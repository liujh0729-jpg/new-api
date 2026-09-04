#!/usr/bin/env python3
"""Migrate legacy local material uploads into AIPDD digital assets.

The utility is intentionally operational rather than part of application
startup. It reads legacy rows through psql inside the PostgreSQL container,
uploads the original files, creates private AIPDD digital assets, and stores
the durable AIPDD identifiers back on the legacy rows. Original files and
legacy URL/path fields are never removed or overwritten.
"""

from __future__ import annotations

import argparse
import csv
import io
import json
import mimetypes
import os
import pathlib
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from dataclasses import dataclass
from typing import Any


SCHEMA_SQL = """
ALTER TABLE materials ADD COLUMN IF NOT EXISTS aipdd_file_id TEXT;
ALTER TABLE materials ADD COLUMN IF NOT EXISTS aipdd_asset_id BIGINT;
ALTER TABLE materials ADD COLUMN IF NOT EXISTS aipdd_channel_id BIGINT;
ALTER TABLE materials ADD COLUMN IF NOT EXISTS aipdd_migrated_at BIGINT;
"""

PENDING_ROWS_SQL = """
COPY (
    SELECT id, user_id, name, type, source_type, mime_type, file_name,
           file_path, file_size
    FROM materials
    WHERE deleted_at IS NULL
      AND source_type = 'material'
      AND type = 'image'
      AND COALESCE(aipdd_asset_id, 0) = 0
    ORDER BY id
) TO STDOUT WITH (FORMAT CSV, HEADER TRUE);
"""

ALL_ROWS_SQL = """
COPY (
    SELECT id, user_id, name, type, source_type, mime_type, file_name,
           file_path, file_size
    FROM materials
    WHERE deleted_at IS NULL
      AND source_type = 'material'
      AND type = 'image'
    ORDER BY id
) TO STDOUT WITH (FORMAT CSV, HEADER TRUE);
"""

MAPPED_ROWS_SQL = """
COPY (
    SELECT id, user_id, name, type, source_type, mime_type, file_name,
           file_path, file_size, aipdd_file_id, aipdd_asset_id
    FROM materials
    WHERE deleted_at IS NULL
      AND source_type = 'material'
      AND type = 'image'
      AND aipdd_asset_id > 0
      AND LENGTH(aipdd_file_id) > 0
    ORDER BY id
) TO STDOUT WITH (FORMAT CSV, HEADER TRUE);
"""


@dataclass(frozen=True)
class LegacyMaterial:
    material_id: int
    user_id: int
    name: str
    media_type: str
    source_type: str
    mime_type: str
    file_name: str
    file_path: str
    file_size: int


@dataclass(frozen=True)
class MigratedMaterial:
    legacy: LegacyMaterial
    file_id: str
    asset_id: int


class MigrationError(RuntimeError):
    pass


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Migrate uploaded legacy images to private AIPDD digital assets."
    )
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--apply", action="store_true", help="perform uploads and database writes")
    mode.add_argument("--verify", action="store_true", help="verify database mappings against AIPDD")
    parser.add_argument("--key-file", help="root-readable file containing AIPDD_API_KEY")
    parser.add_argument("--base-url", default="https://api.aipdd.work")
    parser.add_argument("--db-container", default="new-api-postgres")
    parser.add_argument("--host-data-root", default="/opt/new-api/data")
    parser.add_argument("--container-data-root", default="/data")
    parser.add_argument("--journal", default="/opt/new-api/backups/material-aipdd-migration.jsonl")
    parser.add_argument("--channel-id", type=int, default=0)
    parser.add_argument("--limit", type=int, default=0, help="migrate at most this many pending rows")
    return parser.parse_args()


def run_psql(container: str, sql: str) -> str:
    command = [
        "docker",
        "exec",
        "-i",
        container,
        "sh",
        "-lc",
        'exec psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"',
    ]
    result = subprocess.run(
        command,
        input=sql,
        text=True,
        capture_output=True,
        check=False,
    )
    if result.returncode != 0:
        message = result.stderr.strip() or result.stdout.strip() or "unknown psql failure"
        raise MigrationError(f"database command failed: {message}")
    return result.stdout


def load_rows(container: str, pending_only: bool) -> list[LegacyMaterial]:
    output = run_psql(container, PENDING_ROWS_SQL if pending_only else ALL_ROWS_SQL)
    rows: list[LegacyMaterial] = []
    for row in csv.DictReader(io.StringIO(output)):
        rows.append(
            LegacyMaterial(
                material_id=int(row["id"]),
                user_id=int(row["user_id"]),
                name=(row["name"] or "").strip(),
                media_type=(row["type"] or "").strip(),
                source_type=(row["source_type"] or "").strip(),
                mime_type=(row["mime_type"] or "").strip(),
                file_name=(row["file_name"] or "").strip(),
                file_path=(row["file_path"] or "").strip(),
                file_size=int(row["file_size"] or 0),
            )
        )
    return rows


def load_mapped_rows(container: str) -> list[MigratedMaterial]:
    output = run_psql(container, MAPPED_ROWS_SQL)
    rows: list[MigratedMaterial] = []
    for row in csv.DictReader(io.StringIO(output)):
        rows.append(
            MigratedMaterial(
                legacy=LegacyMaterial(
                    material_id=int(row["id"]),
                    user_id=int(row["user_id"]),
                    name=(row["name"] or "").strip(),
                    media_type=(row["type"] or "").strip(),
                    source_type=(row["source_type"] or "").strip(),
                    mime_type=(row["mime_type"] or "").strip(),
                    file_name=(row["file_name"] or "").strip(),
                    file_path=(row["file_path"] or "").strip(),
                    file_size=int(row["file_size"] or 0),
                ),
                file_id=(row["aipdd_file_id"] or "").strip(),
                asset_id=int(row["aipdd_asset_id"] or 0),
            )
        )
    return rows


def resolve_source_path(
    row: LegacyMaterial,
    host_data_root: pathlib.Path,
    container_data_root: pathlib.PurePosixPath,
) -> pathlib.Path:
    candidates: list[pathlib.Path] = []
    stored = pathlib.PurePosixPath(row.file_path.replace("\\", "/"))
    try:
        relative = stored.relative_to(container_data_root)
        candidates.append(host_data_root / pathlib.Path(*relative.parts))
    except ValueError:
        pass
    candidates.append(host_data_root / "materials" / pathlib.Path(row.file_name).name)
    candidates.append(host_data_root / "materials" / stored.name)

    resolved_root = host_data_root.resolve()
    for candidate in candidates:
        try:
            resolved = candidate.resolve()
            resolved.relative_to(resolved_root)
        except (OSError, ValueError):
            continue
        if resolved.is_file():
            return resolved
    raise MigrationError(f"source file is missing for material {row.material_id}")


def api_request(
    base_url: str,
    api_key: str,
    method: str,
    path: str,
    *,
    body: bytes | None = None,
    content_type: str | None = None,
) -> dict[str, Any]:
    headers = {
        "Accept": "application/json",
        "X-API-Key": api_key,
        "Authorization": f"Bearer {api_key}",
    }
    if content_type:
        headers["Content-Type"] = content_type
    request = urllib.request.Request(
        base_url + path,
        data=body,
        headers=headers,
        method=method,
    )
    try:
        with urllib.request.urlopen(request, timeout=180) as response:
            payload = response.read(4 * 1024 * 1024 + 1)
    except urllib.error.HTTPError as error:
        detail = error.read(4096).decode("utf-8", "replace").strip()
        raise MigrationError(f"AIPDD returned HTTP {error.code}: {detail[:500]}") from error
    except urllib.error.URLError as error:
        raise MigrationError(f"AIPDD request failed: {error.reason}") from error
    if len(payload) > 4 * 1024 * 1024:
        raise MigrationError("AIPDD response exceeded 4 MiB")
    try:
        envelope = json.loads(payload)
    except json.JSONDecodeError as error:
        raise MigrationError("AIPDD returned invalid JSON") from error
    if not isinstance(envelope, dict) or envelope.get("code") != 0:
        message = envelope.get("message") if isinstance(envelope, dict) else None
        raise MigrationError(str(message or "AIPDD application error"))
    data = envelope.get("data")
    return data if isinstance(data, dict) else {}


def upload_file(base_url: str, api_key: str, source: pathlib.Path, mime_type: str) -> str:
    boundary = "----new-api-material-" + uuid.uuid4().hex
    safe_name = source.name.replace('"', "")
    prefix = (
        f"--{boundary}\r\n"
        f'Content-Disposition: form-data; name="file"; filename="{safe_name}"\r\n'
        f"Content-Type: {mime_type}\r\n\r\n"
    ).encode("utf-8")
    suffix = f"\r\n--{boundary}--\r\n".encode("ascii")
    body = prefix + source.read_bytes() + suffix
    query = urllib.parse.urlencode(
        {
            "prefix": "new-api/legacy-materials",
            "is_private": "true",
            "valid_time": "259200",
        }
    )
    data = api_request(
        base_url,
        api_key,
        "POST",
        "/oss/upload?" + query,
        body=body,
        content_type=f"multipart/form-data; boundary={boundary}",
    )
    file_id = str(data.get("file_id") or "").strip()
    if not file_id:
        raise MigrationError("AIPDD upload returned an empty file_id")
    return file_id


def create_asset(
    base_url: str,
    api_key: str,
    row: LegacyMaterial,
    file_id: str,
    file_size: int,
) -> int:
    body = json.dumps(
        {
            "name": (row.name or row.file_name or f"legacy-material-{row.material_id}")[:191],
            "type": "image",
            "labels": [
                "new-api-legacy-material",
                f"new-api-user-{row.user_id}",
                f"legacy-material-{row.material_id}",
            ],
            "url": file_id,
            "isOpen": False,
            "enabled": True,
            "fileSize": file_size,
        },
        ensure_ascii=False,
        separators=(",", ":"),
    ).encode("utf-8")
    data = api_request(
        base_url,
        api_key,
        "POST",
        "/digital_asset",
        body=body,
        content_type="application/json",
    )
    asset_id = int(data.get("id") or 0)
    if asset_id <= 0:
        raise MigrationError("AIPDD digital asset returned an invalid id")
    return asset_id


def validate_signed_url(base_url: str, api_key: str, file_id: str) -> None:
    escaped_id = urllib.parse.quote(file_id, safe="")
    data = api_request(
        base_url,
        api_key,
        "GET",
        f"/oss/file/{escaped_id}/sign?valid_time=300",
    )
    signed_url = str(data.get("url") or "").strip()
    if not signed_url.startswith(("https://", "http://")):
        raise MigrationError("AIPDD signing returned an invalid URL")


def delete_asset_best_effort(base_url: str, api_key: str, asset_id: int) -> None:
    if asset_id <= 0:
        return
    try:
        api_request(base_url, api_key, "DELETE", f"/digital_asset/{asset_id}")
    except MigrationError:
        pass


def delete_file_best_effort(base_url: str, api_key: str, file_id: str) -> None:
    if not file_id:
        return
    try:
        escaped_id = urllib.parse.quote(file_id, safe="")
        api_request(base_url, api_key, "DELETE", f"/oss/file/{escaped_id}")
    except MigrationError:
        pass


def sql_literal(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def backfill_row(
    container: str,
    row: LegacyMaterial,
    file_id: str,
    asset_id: int,
    channel_id: int,
) -> None:
    migrated_at = int(time.time())
    sql = f"""
UPDATE materials
SET aipdd_file_id = {sql_literal(file_id)},
    aipdd_asset_id = {asset_id},
    aipdd_channel_id = {channel_id},
    aipdd_migrated_at = {migrated_at},
    updated_time = {migrated_at}
WHERE id = {row.material_id}
  AND deleted_at IS NULL
  AND COALESCE(aipdd_asset_id, 0) = 0;
"""
    output = run_psql(container, sql)
    if "UPDATE 1" not in output:
        raise MigrationError(f"material {row.material_id} was not backfilled")


def append_journal(path: pathlib.Path, event: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a", encoding="utf-8") as handle:
        handle.write(json.dumps(event, ensure_ascii=False, separators=(",", ":")) + "\n")
        handle.flush()
        os.fsync(handle.fileno())


def load_api_key(path_value: str | None) -> str:
    if not path_value:
        raise MigrationError("--key-file is required with --apply")
    key_path = pathlib.Path(path_value)
    api_key = key_path.read_text(encoding="utf-8").strip()
    if not api_key:
        raise MigrationError("AIPDD key file is empty")
    return api_key


def list_migrated_assets(base_url: str, api_key: str) -> list[dict[str, Any]]:
    assets: list[dict[str, Any]] = []
    page = 1
    while True:
        query = urllib.parse.urlencode(
            {
                "page": page,
                "page_size": 100,
                "type": "image",
                "is_mine": "true",
                "label": "new-api-legacy-material",
            }
        )
        data = api_request(
            base_url,
            api_key,
            "GET",
            "/digital_asset/list?" + query,
        )
        page_assets = data.get("list")
        if not isinstance(page_assets, list):
            raise MigrationError("AIPDD digital asset list returned invalid data")
        assets.extend(asset for asset in page_assets if isinstance(asset, dict))
        total = int(data.get("total") or 0)
        if len(assets) >= total or not page_assets:
            return assets
        page += 1


def verify_migration(
    args: argparse.Namespace,
    base_url: str,
    host_data_root: pathlib.Path,
    container_data_root: pathlib.PurePosixPath,
) -> int:
    api_key = load_api_key(args.key_file)
    mapped = load_mapped_rows(args.db_container)
    assets = list_migrated_assets(base_url, api_key)

    database_asset_ids = {row.asset_id for row in mapped}
    database_file_ids = {row.file_id for row in mapped}
    aipdd_asset_ids = {int(asset.get("id") or 0) for asset in assets}
    aipdd_file_ids = {str(asset.get("url") or "").strip() for asset in assets}
    signed_count = sum(
        1
        for asset in assets
        if str(asset.get("previewUrl") or "").startswith(("https://", "http://"))
        and not asset.get("downloadError")
    )
    old_files_present = sum(
        1
        for row in mapped
        if resolve_source_path(row.legacy, host_data_root, container_data_root).is_file()
    )

    checks = {
        "asset_ids_match": database_asset_ids == aipdd_asset_ids,
        "file_ids_match": database_file_ids == aipdd_file_ids,
        "asset_ids_unique": len(database_asset_ids) == len(mapped),
        "file_ids_unique": len(database_file_ids) == len(mapped),
        "signed_previews": signed_count == len(assets),
        "old_files_preserved": old_files_present == len(mapped),
    }
    print(
        "verify "
        f"database_rows={len(mapped)} aipdd_assets={len(assets)} "
        f"signable={signed_count} old_files={old_files_present}"
    )
    for name, passed in checks.items():
        print(f"{name}={'PASS' if passed else 'FAIL'}")
    return 0 if all(checks.values()) else 1


def main() -> int:
    args = parse_args()
    host_data_root = pathlib.Path(args.host_data_root)
    container_data_root = pathlib.PurePosixPath(args.container_data_root)
    base_url = args.base_url.strip().rstrip("/")
    if base_url.endswith("/v1"):
        base_url = base_url[:-3]

    if args.verify:
        return verify_migration(
            args,
            base_url,
            host_data_root,
            container_data_root,
        )

    if args.apply:
        run_psql(args.db_container, SCHEMA_SQL)
    rows = load_rows(args.db_container, pending_only=args.apply)
    if args.limit > 0:
        rows = rows[: args.limit]

    resolved: list[tuple[LegacyMaterial, pathlib.Path]] = []
    missing: list[str] = []
    for row in rows:
        try:
            resolved.append(
                (
                    row,
                    resolve_source_path(row, host_data_root, container_data_root),
                )
            )
        except MigrationError as error:
            missing.append(str(error))

    print(f"eligible={len(rows)} ready={len(resolved)} missing={len(missing)}")
    for message in missing:
        print(f"MISSING {message}")
    if not args.apply:
        return 1 if missing else 0
    if missing:
        raise MigrationError("preflight failed; no uploads were started")

    api_key = load_api_key(args.key_file)
    journal_path = pathlib.Path(args.journal)
    succeeded = 0
    failed = 0
    for index, (row, source) in enumerate(resolved, start=1):
        file_id = ""
        asset_id = 0
        try:
            actual_size = source.stat().st_size
            mime_type = row.mime_type or mimetypes.guess_type(source.name)[0] or "image/jpeg"
            file_id = upload_file(base_url, api_key, source, mime_type)
            asset_id = create_asset(base_url, api_key, row, file_id, actual_size)
            validate_signed_url(base_url, api_key, file_id)
            backfill_row(
                args.db_container,
                row,
                file_id,
                asset_id,
                args.channel_id,
            )
            append_journal(
                journal_path,
                {
                    "status": "migrated",
                    "material_id": row.material_id,
                    "user_id": row.user_id,
                    "file_id": file_id,
                    "asset_id": asset_id,
                    "source_path": str(source),
                    "migrated_at": int(time.time()),
                },
            )
            succeeded += 1
            print(f"[{index}/{len(resolved)}] material={row.material_id} migrated")
        except Exception as error:  # continue so one bad file does not hide the rest
            delete_asset_best_effort(base_url, api_key, asset_id)
            delete_file_best_effort(base_url, api_key, file_id)
            append_journal(
                journal_path,
                {
                    "status": "failed",
                    "material_id": row.material_id,
                    "user_id": row.user_id,
                    "error": str(error),
                    "failed_at": int(time.time()),
                },
            )
            failed += 1
            print(f"[{index}/{len(resolved)}] material={row.material_id} FAILED: {error}")

    print(f"migration_complete succeeded={succeeded} failed={failed}")
    return 1 if failed else 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except MigrationError as error:
        print(f"ERROR: {error}", file=sys.stderr)
        raise SystemExit(1)
