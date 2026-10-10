import json
from pathlib import Path

import paramiko


ARTIFACT_DIR = Path(__file__).resolve().parent


def run_psql_csv(client, sql):
    remote_command = (
        "docker exec new-api-postgres sh -lc "
        "'psql -U \"$POSTGRES_USER\" -d \"$POSTGRES_DB\" --csv -c \""
        + sql.replace('"', '\\"')
        + "\"'"
    )
    _, stdout, stderr = client.exec_command(remote_command, timeout=30)
    output = stdout.read().decode("utf-8", "replace")
    error_text = stderr.read().decode("utf-8", "replace").strip()
    if not output.strip():
        raise RuntimeError(error_text or "empty psql output")
    return output


def main():
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect(
        "14.103.100.4",
        username="root",
        key_filename=r"D:\Documents\zhuimi.pem",
        timeout=10,
        banner_timeout=10,
        auth_timeout=10,
    )
    task_sql = (
        "SELECT id,task_id,status,channel_id,quota,submit_time,start_time,"
        "finish_time,progress,properties::text FROM tasks WHERE id>=193 "
        "AND user_id=1 ORDER BY id"
    )
    account_sql = (
        "SELECT u.id,u.username,u.quota,u.used_quota,u.request_count,"
        "COUNT(t.id) AS batch_task_count,COALESCE(SUM(t.quota),0) AS batch_quota "
        "FROM users u LEFT JOIN tasks t ON t.user_id=u.id AND t.id>=193 "
        "WHERE u.id=1 GROUP BY u.id,u.username,u.quota,u.used_quota,u.request_count"
    )
    tasks_csv = run_psql_csv(client, task_sql)
    account_csv = run_psql_csv(client, account_sql)
    client.close()
    (ARTIFACT_DIR / "database-task-evidence.csv").write_text(tasks_csv, encoding="utf-8")
    (ARTIFACT_DIR / "database-account-after.csv").write_text(account_csv, encoding="utf-8")
    print(
        json.dumps(
            {
                "task_rows": max(0, len(tasks_csv.splitlines()) - 1),
                "task_file": str(ARTIFACT_DIR / "database-task-evidence.csv"),
                "account_file": str(ARTIFACT_DIR / "database-account-after.csv"),
            },
            ensure_ascii=False,
        )
    )


if __name__ == "__main__":
    main()
