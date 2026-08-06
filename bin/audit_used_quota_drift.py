#!/usr/bin/env python3
"""Read-only audit for user used_quota inflation caused by refunds.

Before the refund used_quota rollback fix, async-task refunds restored wallet
quota but left users.used_quota unchanged. The admin UI then shows
Total = Remaining + Used, which double-counts each refund.

This script aggregates LogTypeRefund (type=6) per user and prints:

  user_id / username / current_used / refund_sum / suggested_used / drift

It never writes to the database.

Usage examples:

  python bin/audit_used_quota_drift.py --sqlite ./data/one-api.db
  python bin/audit_used_quota_drift.py --dsn "$SQL_DSN"
  SQLITE_PATH=./data/one-api.db python bin/audit_used_quota_drift.py

Optional:

  --before <unix_ts>   only count refunds created before this timestamp
                       (useful after the fix is deployed so new refunds
                       that already rolled back used_quota are excluded)
  --json               emit JSON instead of a table
"""

from __future__ import annotations

import argparse
import json
import os
import sqlite3
import sys
from dataclasses import asdict, dataclass
from typing import Any, Iterable, Optional
from urllib.parse import unquote, urlparse


if os.name == "nt":
    for stream in (sys.stdout, sys.stderr):
        if hasattr(stream, "reconfigure"):
            stream.reconfigure(encoding="utf-8", errors="replace")


LOG_TYPE_REFUND = 6


@dataclass
class DriftRow:
    user_id: int
    username: str
    current_used: int
    refund_sum: int
    suggested_used: int
    drift: int


class AuditError(RuntimeError):
    pass


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Audit used_quota drift from refund logs (read-only)."
    )
    parser.add_argument(
        "--sqlite",
        help="Path to SQLite database file (also reads SQLITE_PATH).",
    )
    parser.add_argument(
        "--dsn",
        help="SQL DSN for MySQL or PostgreSQL (also reads SQL_DSN).",
    )
    parser.add_argument(
        "--before",
        type=int,
        default=None,
        help="Only count refunds with created_at < this unix timestamp.",
    )
    parser.add_argument(
        "--json",
        action="store_true",
        help="Print JSON instead of a human-readable table.",
    )
    return parser.parse_args()


def resolve_sqlite(args: argparse.Namespace) -> Optional[str]:
    path = args.sqlite or os.environ.get("SQLITE_PATH")
    if not path:
        return None
    if not os.path.isfile(path):
        raise AuditError(f"SQLite file not found: {path}")
    return path


def resolve_dsn(args: argparse.Namespace) -> Optional[str]:
    return args.dsn or os.environ.get("SQL_DSN") or None


def connect_sqlite(path: str) -> sqlite3.Connection:
    conn = sqlite3.connect(f"file:{path}?mode=ro", uri=True)
    conn.row_factory = sqlite3.Row
    return conn


def connect_mysql(dsn: str):
    try:
        import pymysql  # type: ignore
    except ImportError as exc:
        raise AuditError(
            "pymysql is required for MySQL DSNs. Install with: pip install pymysql"
        ) from exc

    # Accept both URL form and Go-style user:pass@tcp(host:port)/db
    if "://" in dsn:
        parsed = urlparse(dsn)
        if parsed.scheme not in ("mysql", "mysql+pymysql"):
            raise AuditError(f"Unsupported MySQL scheme: {parsed.scheme}")
        user = unquote(parsed.username or "")
        password = unquote(parsed.password or "")
        host = parsed.hostname or "127.0.0.1"
        port = parsed.port or 3306
        database = (parsed.path or "/").lstrip("/")
        return pymysql.connect(
            host=host,
            port=port,
            user=user,
            password=password,
            database=database,
            charset="utf8mb4",
            cursorclass=pymysql.cursors.DictCursor,
            autocommit=True,
        )

    # Go DSN: user:pass@tcp(host:port)/dbname?params
    import re

    m = re.match(
        r"^(?P<user>[^:]+):(?P<pass>[^@]*)@tcp\((?P<host>[^:]+):(?P<port>\d+)\)/(?P<db>[^?]+)",
        dsn,
    )
    if not m:
        raise AuditError(
            "Unrecognized MySQL DSN. Use mysql://user:pass@host:port/db "
            "or user:pass@tcp(host:port)/db"
        )
    return pymysql.connect(
        host=m.group("host"),
        port=int(m.group("port")),
        user=m.group("user"),
        password=m.group("pass"),
        database=m.group("db"),
        charset="utf8mb4",
        cursorclass=pymysql.cursors.DictCursor,
        autocommit=True,
    )


def connect_postgres(dsn: str):
    try:
        import psycopg2  # type: ignore
        import psycopg2.extras  # type: ignore
    except ImportError as exc:
        raise AuditError(
            "psycopg2 is required for PostgreSQL DSNs. "
            "Install with: pip install psycopg2-binary"
        ) from exc

    if dsn.startswith("postgres://"):
        dsn = "postgresql://" + dsn[len("postgres://") :]
    conn = psycopg2.connect(dsn)
    conn.set_session(readonly=True, autocommit=True)
    return conn


def detect_driver(dsn: str) -> str:
    lower = dsn.lower()
    if lower.startswith("postgres://") or lower.startswith("postgresql://"):
        return "postgres"
    if lower.startswith("mysql://") or "@tcp(" in dsn:
        return "mysql"
    raise AuditError(
        "Cannot detect DSN type. Use --sqlite, mysql://..., postgresql://..., "
        "or Go-style user:pass@tcp(host:port)/db"
    )


def fetch_rows_sqlite(conn: sqlite3.Connection, before: Optional[int]) -> list[DriftRow]:
    sql = """
        SELECT
            u.id AS user_id,
            u.username AS username,
            u.used_quota AS current_used,
            COALESCE(r.refund_sum, 0) AS refund_sum
        FROM users u
        INNER JOIN (
            SELECT user_id, SUM(quota) AS refund_sum
            FROM logs
            WHERE type = ?
              AND (? IS NULL OR created_at < ?)
            GROUP BY user_id
            HAVING SUM(quota) > 0
        ) r ON r.user_id = u.id
        ORDER BY r.refund_sum DESC, u.id ASC
    """
    cur = conn.execute(sql, (LOG_TYPE_REFUND, before, before))
    return [
        DriftRow(
            user_id=int(row["user_id"]),
            username=str(row["username"] or ""),
            current_used=int(row["current_used"] or 0),
            refund_sum=int(row["refund_sum"] or 0),
            suggested_used=max(0, int(row["current_used"] or 0) - int(row["refund_sum"] or 0)),
            drift=int(row["refund_sum"] or 0),
        )
        for row in cur.fetchall()
    ]


def fetch_rows_sql(conn: Any, before: Optional[int], placeholder: str = "%s") -> list[DriftRow]:
    sql = f"""
        SELECT
            u.id AS user_id,
            u.username AS username,
            u.used_quota AS current_used,
            COALESCE(r.refund_sum, 0) AS refund_sum
        FROM users u
        INNER JOIN (
            SELECT user_id, SUM(quota) AS refund_sum
            FROM logs
            WHERE type = {placeholder}
              AND ({placeholder} IS NULL OR created_at < {placeholder})
            GROUP BY user_id
            HAVING SUM(quota) > 0
        ) r ON r.user_id = u.id
        ORDER BY r.refund_sum DESC, u.id ASC
    """
    with conn.cursor() as cur:
        cur.execute(sql, (LOG_TYPE_REFUND, before, before))
        rows = cur.fetchall()
    out: list[DriftRow] = []
    for row in rows:
        if not isinstance(row, dict):
            # psycopg2 default tuples -> wrap with RealDictCursor preferred
            raise AuditError("Expected dict rows from SQL driver")
        current_used = int(row["current_used"] or 0)
        refund_sum = int(row["refund_sum"] or 0)
        out.append(
            DriftRow(
                user_id=int(row["user_id"]),
                username=str(row["username"] or ""),
                current_used=current_used,
                refund_sum=refund_sum,
                suggested_used=max(0, current_used - refund_sum),
                drift=refund_sum,
            )
        )
    return out


def fetch_rows_postgres(conn: Any, before: Optional[int]) -> list[DriftRow]:
    import psycopg2.extras  # type: ignore

    sql = """
        SELECT
            u.id AS user_id,
            u.username AS username,
            u.used_quota AS current_used,
            COALESCE(r.refund_sum, 0) AS refund_sum
        FROM users u
        INNER JOIN (
            SELECT user_id, SUM(quota) AS refund_sum
            FROM logs
            WHERE type = %s
              AND (%s IS NULL OR created_at < %s)
            GROUP BY user_id
            HAVING SUM(quota) > 0
        ) r ON r.user_id = u.id
        ORDER BY r.refund_sum DESC, u.id ASC
    """
    with conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as cur:
        cur.execute(sql, (LOG_TYPE_REFUND, before, before))
        rows = cur.fetchall()
    out: list[DriftRow] = []
    for row in rows:
        current_used = int(row["current_used"] or 0)
        refund_sum = int(row["refund_sum"] or 0)
        out.append(
            DriftRow(
                user_id=int(row["user_id"]),
                username=str(row["username"] or ""),
                current_used=current_used,
                refund_sum=refund_sum,
                suggested_used=max(0, current_used - refund_sum),
                drift=refund_sum,
            )
        )
    return out


def format_table(rows: Iterable[DriftRow]) -> str:
    header = (
        f"{'user_id':>8}  {'username':<24}  {'current_used':>14}  "
        f"{'refund_sum':>12}  {'suggested_used':>14}  {'drift':>12}"
    )
    lines = [header, "-" * len(header)]
    total_drift = 0
    count = 0
    for row in rows:
        count += 1
        total_drift += row.drift
        lines.append(
            f"{row.user_id:>8}  {row.username:<24}  {row.current_used:>14}  "
            f"{row.refund_sum:>12}  {row.suggested_used:>14}  {row.drift:>12}"
        )
    lines.append("-" * len(header))
    lines.append(f"users_with_drift={count}  total_drift_quota={total_drift}")
    lines.append(
        "NOTE: read-only estimate assuming refunds did not roll back used_quota. "
        "After the fix is live, pass --before <deploy_unix_ts> to exclude new refunds."
    )
    return "\n".join(lines)


def main() -> int:
    args = parse_args()
    sqlite_path = resolve_sqlite(args)
    dsn = resolve_dsn(args)

    if sqlite_path and dsn:
        raise AuditError("Provide either --sqlite/SQLITE_PATH or --dsn/SQL_DSN, not both")
    if not sqlite_path and not dsn:
        raise AuditError(
            "No database configured. Use --sqlite PATH, --dsn DSN, "
            "or set SQLITE_PATH / SQL_DSN"
        )

    if sqlite_path:
        conn = connect_sqlite(sqlite_path)
        try:
            rows = fetch_rows_sqlite(conn, args.before)
        finally:
            conn.close()
    else:
        assert dsn is not None
        driver = detect_driver(dsn)
        if driver == "mysql":
            conn = connect_mysql(dsn)
            try:
                rows = fetch_rows_sql(conn, args.before)
            finally:
                conn.close()
        else:
            conn = connect_postgres(dsn)
            try:
                rows = fetch_rows_postgres(conn, args.before)
            finally:
                conn.close()

    if args.json:
        payload = {
            "before": args.before,
            "count": len(rows),
            "total_drift_quota": sum(r.drift for r in rows),
            "items": [asdict(r) for r in rows],
        }
        print(json.dumps(payload, ensure_ascii=False, indent=2))
    else:
        if not rows:
            print("No users with refund-related used_quota drift found.")
            return 0
        print(format_table(rows))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except AuditError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        raise SystemExit(2)
