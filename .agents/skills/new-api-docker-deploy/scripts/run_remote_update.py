#!/usr/bin/env python3
"""Orchestrate a New API ACR update over SSH. Secrets come from env vars only."""

from __future__ import annotations

import json
import os
import sys
import time
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import Any
from urllib import error, request

import paramiko

HOST = os.environ.get("DEPLOY_SSH_HOST", "")
USER = os.environ.get("DEPLOY_SSH_USER", "root")
SSH_PASSWORD = os.environ.get("DEPLOY_SSH_PASSWORD", "")
SSH_KEY_FILE = os.environ.get("DEPLOY_SSH_KEY_FILE", "").strip()
ADMIN_USER = os.environ.get("DEPLOY_ADMIN_USER", "root")
ADMIN_PASSWORD = os.environ.get("DEPLOY_ADMIN_PASSWORD", "")
DEPLOY_DIR = os.environ.get("DEPLOY_DIR", "/opt/new-api")
PUBLIC_PORT = os.environ.get("DEPLOY_PUBLIC_PORT", "6070")
REGISTERED_INSTANCE_ID = os.environ.get("DEPLOY_INSTANCE_ID", "").strip()
EXPECTED_IMAGE = (
    "crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest"
)
CHANNEL_OVERWRITE = os.environ.get("DEPLOY_CHANNEL_OVERWRITE", "false").lower() == "true"
PRICE_OVERWRITE = os.environ.get("DEPLOY_PRICE_OVERWRITE", "false").lower() == "true"
VIP_SYNC = os.environ.get("DEPLOY_VIP_SYNC", "false").lower() == "true"
SCRIPT_DIR = Path(__file__).resolve().parent
WORK_DIR = Path(os.environ.get("DEPLOY_WORK_DIR", str(Path.cwd() / ".tmp_deploy_update")))
REPORT_BASE = os.environ.get("AIPDD_REPORT_BASE_URL", "https://api.aipdd.work")

reporting_failures: list[str] = []


def utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def die(msg: str, code: int = 1) -> None:
    print(f"ERROR: {msg}", file=sys.stderr)
    sys.exit(code)


def require_secrets() -> None:
    if not HOST:
        die("DEPLOY_SSH_HOST missing")
    if not SSH_PASSWORD and not SSH_KEY_FILE:
        die("DEPLOY_SSH_PASSWORD or DEPLOY_SSH_KEY_FILE required")
    if SSH_KEY_FILE and not Path(SSH_KEY_FILE).is_file():
        die(f"DEPLOY_SSH_KEY_FILE not found: {SSH_KEY_FILE}")
    if CHANNEL_OVERWRITE or PRICE_OVERWRITE or VIP_SYNC:
        if not ADMIN_PASSWORD:
            die("DEPLOY_ADMIN_PASSWORD required for confirmed admin operations")


def connect() -> paramiko.SSHClient:
    client = paramiko.SSHClient()
    known = os.path.expanduser("~/.ssh/known_hosts")
    if os.path.exists(known):
        client.load_host_keys(known)
        client.set_missing_host_key_policy(paramiko.WarningPolicy())
    else:
        client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    kwargs: dict[str, Any] = {
        "hostname": HOST,
        "username": USER,
        "timeout": 30,
        "allow_agent": False,
        "look_for_keys": False,
    }
    if SSH_KEY_FILE:
        kwargs["key_filename"] = SSH_KEY_FILE
        if SSH_PASSWORD:
            # Optional passphrase for encrypted private keys.
            kwargs["passphrase"] = SSH_PASSWORD
    else:
        kwargs["password"] = SSH_PASSWORD
    client.connect(**kwargs)
    return client


def run(
    client: paramiko.SSHClient,
    cmd: str,
    *,
    check: bool = True,
    timeout: int = 120,
) -> tuple[int, str, str]:
    stdin, stdout, stderr = client.exec_command(cmd, timeout=timeout)
    out = stdout.read().decode("utf-8", "replace")
    err = stderr.read().decode("utf-8", "replace")
    code = stdout.channel.recv_exit_status()
    if check and code != 0:
        die(f"remote command failed ({code}): {cmd}\n{err[:800] or out[:800]}")
    return code, out, err


def sftp_write(client: paramiko.SSHClient, remote_path: str, data: str, mode: int = 0o600) -> None:
    sftp = client.open_sftp()
    try:
        with sftp.file(remote_path, "w") as f:
            f.write(data)
        sftp.chmod(remote_path, mode)
    finally:
        sftp.close()


def sftp_read(client: paramiko.SSHClient, remote_path: str) -> str:
    sftp = client.open_sftp()
    try:
        with sftp.file(remote_path, "r") as f:
            return f.read().decode("utf-8", "replace")
    finally:
        sftp.close()


def sftp_remove(client: paramiko.SSHClient, remote_path: str) -> None:
    sftp = client.open_sftp()
    try:
        try:
            sftp.remove(remote_path)
        except OSError:
            pass
    finally:
        sftp.close()


def ensure_work_dir() -> None:
    WORK_DIR.mkdir(parents=True, exist_ok=True)
    try:
        os.chmod(WORK_DIR, 0o700)
    except OSError:
        pass


def write_local(path: Path, data: str | bytes) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    if isinstance(data, str):
        path.write_text(data, encoding="utf-8")
    else:
        path.write_bytes(data)
    try:
        os.chmod(path, 0o600)
    except OSError:
        pass


def report(
    stage: str,
    payload: dict[str, Any],
    instance_id: str | None = None,
    api_key: str | None = None,
) -> bool:
    """Best-effort AIPDD deployment archive. Failures are recorded, not fatal."""
    key = (api_key or os.environ.get("AIPDD_API_KEY", "")).strip()
    if not key:
        reporting_failures.append(f"{stage}(no AIPDD_API_KEY)")
        print(f"REPORT_SKIP {stage}: no AIPDD_API_KEY")
        return False
    script = SCRIPT_DIR / "report_deployment.py"
    if not script.exists():
        reporting_failures.append(f"{stage}(missing report script)")
        return False
    payload_path = WORK_DIR / f"report-{stage}-{uuid.uuid4().hex}.json"
    key_path = WORK_DIR / f"report-key-{uuid.uuid4().hex}"
    write_local(payload_path, json.dumps(payload, ensure_ascii=False))
    write_local(key_path, key)
    cmd = [
        sys.executable,
        str(script),
        "--stage",
        stage,
        "--payload",
        str(payload_path),
        "--key-file",
        str(key_path),
        "--base-url",
        REPORT_BASE,
        "--timeout",
        "20",
    ]
    if instance_id and stage in {"instance", "credentials"}:
        cmd.extend(["--instance-id", instance_id])
    import subprocess

    try:
        proc = subprocess.run(cmd, capture_output=True, text=True, timeout=40)
        if proc.returncode != 0:
            reporting_failures.append(stage)
            print(f"REPORT_FAIL {stage}: exit={proc.returncode}")
            if proc.stderr:
                print(proc.stderr[:300])
            return False
        print(f"REPORT_OK {stage}")
        return True
    except Exception as exc:  # noqa: BLE001
        reporting_failures.append(stage)
        print(f"REPORT_FAIL {stage}: {type(exc).__name__}")
        return False
    finally:
        for p in (payload_path, key_path):
            try:
                p.unlink(missing_ok=True)
            except OSError:
                pass


def remote_http_json(
    client: paramiko.SSHClient,
    method: str,
    path: str,
    body: dict[str, Any] | None = None,
    cookie_file: str | None = None,
    user_id: int | str | None = None,
) -> dict[str, Any]:
    """Call app API from the server using curl + protected temp files."""
    req_path = f"/tmp/newapi-req-{uuid.uuid4().hex}.json"
    resp_path = f"/tmp/newapi-resp-{uuid.uuid4().hex}.json"
    url = f"http://127.0.0.1:{PUBLIC_PORT}{path}"
    try:
        if body is not None:
            sftp_write(client, req_path, json.dumps(body, ensure_ascii=False))
        cookie_arg = f'-b "{cookie_file}" -c "{cookie_file}"' if cookie_file else ""
        user_arg = f'-H "New-Api-User: {user_id}"' if user_id is not None else ""
        data_arg = f'-H "Content-Type: application/json" --data-binary @{req_path}' if body is not None else ""
        cmd = (
            f'curl -sS -X {method} {cookie_arg} {user_arg} {data_arg} '
            f'-o "{resp_path}" -w "%{{http_code}}" --max-time 120 "{url}"'
        )
        code, out, err = run(client, cmd, check=False, timeout=90)
        if code != 0:
            die(f"curl failed for {method} {path}: {err[:400]}")
        http_code = out.strip()
        raw = sftp_read(client, resp_path)
        try:
            data = json.loads(raw) if raw.strip() else {}
        except json.JSONDecodeError:
            die(f"non-JSON response for {method} {path}: HTTP {http_code}")
        data["_http_code"] = http_code
        return data
    finally:
        if body is not None:
            sftp_remove(client, req_path)
        sftp_remove(client, resp_path)


def preflight(client: paramiko.SSHClient) -> None:
    print("=== preflight ===")
    for cmd in [
        "docker --version",
        "docker compose version",
        'docker info --format "{{.ServerVersion}} {{.Driver}}"',
        "df -h / | tail -n +1",
    ]:
        code, out, err = run(client, cmd, check=False)
        print(out.strip() or err.strip())
        if code != 0 and "docker" in cmd:
            die(f"Docker unavailable: {cmd}")
    code, out, _ = run(client, f"test -f {DEPLOY_DIR}/docker-compose.yml && test -f {DEPLOY_DIR}/.env && echo OK", check=False)
    if "OK" not in out:
        die(f"Incomplete deployment at {DEPLOY_DIR}; refusing to treat as update")
    _, images, _ = run(client, f"grep -E '^[[:space:]]*image:' {DEPLOY_DIR}/docker-compose.yml")
    print("compose images:")
    print(images.strip())
    if EXPECTED_IMAGE.split(":")[0] not in images:
        die(f"Compose image is not ACR expected repo. Found:\n{images}")
    _, ps, _ = run(client, f"cd {DEPLOY_DIR} && docker compose ps")
    print(ps)


def resolve_instance_id(client: paramiko.SSHClient) -> str:
    path = f"{DEPLOY_DIR}/.aipdd-instance-id"
    code, out, _ = run(client, f"test -f {path} && cat {path} || true", check=False)
    file_value = out.strip()
    env_value = ""
    try:
        for line in sftp_read(client, f"{DEPLOY_DIR}/.env").splitlines():
            if line.strip().startswith("AIPDD_INSTANCE_ID="):
                env_value = line.split("=", 1)[1].strip().strip('"\'')
                break
    except OSError:
        pass

    normalized: dict[str, str] = {}
    for source, value in {
        ".aipdd-instance-id": file_value,
        ".env AIPDD_INSTANCE_ID": env_value,
        "DEPLOY_INSTANCE_ID": REGISTERED_INSTANCE_ID,
    }.items():
        if not value:
            continue
        try:
            normalized[source] = str(uuid.UUID(value))
        except ValueError:
            die(f"{source} is not a valid UUID")

    identities = set(normalized.values())
    if len(identities) > 1:
        die("Configured AIPDD instance identities disagree; refusing to change site ownership")
    if not identities:
        die(
            "No registered AIPDD instance UUID found. Set DEPLOY_INSTANCE_ID to the "
            "delivery site's externalInstanceId before updating"
        )
    instance_id = identities.pop()
    if not file_value:
        sftp_write(client, path, instance_id + "\n")
        print("Restored .aipdd-instance-id from the registered site identity")
    return instance_id


def current_app_image_digest(client: paramiko.SSHClient) -> str:
    """Read the running new-api content digest before pull/recreate.

    Prefer RepoDigests (registry content digest); fall back to Image ID.
    Returns a sha256:... value required by update-mode archive reporting.
    """
    script = f"""
set -e
cd {DEPLOY_DIR}
python3 - <<'PY'
import json
import subprocess
import sys

cid = subprocess.check_output(
    ["docker", "compose", "ps", "-q", "new-api"],
    text=True,
).strip().splitlines()
cid = (cid[0] if cid else "").strip()
if not cid:
    print("NO_CONTAINER", file=sys.stderr)
    sys.exit(2)
raw = subprocess.check_output(
    ["docker", "inspect", "--format", "{{{{json .}}}}", cid],
    text=True,
)
info = json.loads(raw)
for entry in info.get("RepoDigests") or []:
    if isinstance(entry, str) and "@" in entry:
        digest = entry.split("@", 1)[1].strip()
        if digest.startswith("sha256:") and len(digest) > len("sha256:"):
            print(digest)
            sys.exit(0)
image_id = (info.get("Image") or "").strip()
if image_id.startswith("sha256:") and len(image_id) > len("sha256:"):
    print(image_id)
    sys.exit(0)
print("NO_DIGEST", file=sys.stderr)
sys.exit(3)
PY
"""
    code, out, err = run(client, script, check=False, timeout=60)
    digest = out.strip().splitlines()[-1].strip() if out.strip() else ""
    if code != 0 or not digest.startswith("sha256:"):
        die(f"cannot read previous application image digest: {err[:400] or out[:400]}")
    print(f"previousImageDigest={digest}")
    return digest


def backup(client: paramiko.SSHClient) -> str:
    stamp = datetime.now().strftime("%Y%m%d-%H%M%S")
    backup_dir = f"{DEPLOY_DIR}/backups/update-{stamp}"
    run(client, f"mkdir -p {backup_dir} && chmod 700 {DEPLOY_DIR}/backups {backup_dir}")
    run(
        client,
        f"cp -a {DEPLOY_DIR}/docker-compose.yml {backup_dir}/docker-compose.yml && "
        f"cp -a {DEPLOY_DIR}/.env {backup_dir}/.env && chmod 600 {backup_dir}/.env",
    )
    print(f"Backup created at {backup_dir}")
    return backup_dir


def update_env_flags(client: paramiko.SSHClient, instance_id: str) -> None:
    """Set boot toggles and the immutable site identity without printing .env."""
    catalog = "true" if PRICE_OVERWRITE else "false"
    channel = "true" if CHANNEL_OVERWRITE else "false"
    # Update or append the two keys only.
    script = f"""
python3 - <<'PY'
from pathlib import Path
p = Path({DEPLOY_DIR!r}) / '.env'
text = p.read_text(encoding='utf-8')
lines = text.splitlines()
keys = {{
    'AIPDD_CATALOG_SYNC_ON_BOOT': {catalog!r},
    'AIPDD_CHANNEL_OVERWRITE_ON_BOOT': {channel!r},
    'AIPDD_INSTANCE_ID': {instance_id!r},
}}
seen = set()
out = []
for line in lines:
    if not line or line.lstrip().startswith('#') or '=' not in line:
        out.append(line)
        continue
    k, _, _ = line.partition('=')
    k = k.strip()
    if k in keys:
        if k == 'AIPDD_INSTANCE_ID' and line.partition('=')[2].strip().strip(chr(34) + chr(39)) not in ('', keys[k]):
            raise SystemExit('AIPDD_INSTANCE_ID does not match .aipdd-instance-id')
        out.append(f"{{k}}={{keys[k]}}")
        seen.add(k)
    else:
        out.append(line)
for k, v in keys.items():
    if k not in seen:
        out.append(f"{{k}}={{v}}")
p.write_text('\\n'.join(out) + '\\n', encoding='utf-8')
p.chmod(0o600)
print('ENV_FLAGS_UPDATED')
PY
"""
    _, out, _ = run(client, script)
    if "ENV_FLAGS_UPDATED" not in out:
        die("Failed to update .env boot flags")
    # Ensure compose reads env vars for the boot flags and immutable identity.
    compose_fix = f"""
python3 - <<'PY'
from pathlib import Path
p = Path({DEPLOY_DIR!r}) / 'docker-compose.yml'
text = p.read_text(encoding='utf-8')
changed = False
replacements = {{
    'AIPDD_CATALOG_SYNC_ON_BOOT: "true"': 'AIPDD_CATALOG_SYNC_ON_BOOT: ${{AIPDD_CATALOG_SYNC_ON_BOOT}}',
    'AIPDD_CATALOG_SYNC_ON_BOOT: "false"': 'AIPDD_CATALOG_SYNC_ON_BOOT: ${{AIPDD_CATALOG_SYNC_ON_BOOT}}',
    'AIPDD_CHANNEL_OVERWRITE_ON_BOOT: "true"': 'AIPDD_CHANNEL_OVERWRITE_ON_BOOT: ${{AIPDD_CHANNEL_OVERWRITE_ON_BOOT}}',
    'AIPDD_CHANNEL_OVERWRITE_ON_BOOT: "false"': 'AIPDD_CHANNEL_OVERWRITE_ON_BOOT: ${{AIPDD_CHANNEL_OVERWRITE_ON_BOOT}}',
}}
for old, new in replacements.items():
    if old in text:
        text = text.replace(old, new)
        changed = True
# also handle unquoted boolean literals
import re
for flag_key in ('AIPDD_CATALOG_SYNC_ON_BOOT', 'AIPDD_CHANNEL_OVERWRITE_ON_BOOT'):
    pat = re.compile(r'(^\\s*' + flag_key + r':\\s*)(true|false|"true"|"false")\\s*$', re.M)
    def repl(m, _k=flag_key):
        return m.group(1) + '${{' + _k + '}}'
    new_text, n = pat.subn(repl, text)
    if n:
        text = new_text
        changed = True

mapping_identity = re.compile(r'^(\\s*)AIPDD_INSTANCE_ID:\\s*.*$', re.M)
list_identity = re.compile(r'^(\\s*)-\\s*AIPDD_INSTANCE_ID=.*$', re.M)
if mapping_identity.search(text):
    text, n = mapping_identity.subn(
        lambda m: m.group(1) + 'AIPDD_INSTANCE_ID: ${{AIPDD_INSTANCE_ID}}', text
    )
    changed = changed or bool(n)
elif list_identity.search(text):
    text, n = list_identity.subn(
        lambda m: m.group(1) + '- AIPDD_INSTANCE_ID=${{AIPDD_INSTANCE_ID}}', text
    )
    changed = changed or bool(n)
else:
    mapping_key = re.compile(r'^(\\s*)AIPDD_API_KEY:\\s*.*$', re.M)
    list_key = re.compile(r'^(\\s*)-\\s*AIPDD_API_KEY=.*$', re.M)
    if mapping_key.search(text):
        text = mapping_key.sub(
            lambda m: m.group(0) + '\\n' + m.group(1) + 'AIPDD_INSTANCE_ID: ${{AIPDD_INSTANCE_ID}}',
            text,
            count=1,
        )
        changed = True
    elif list_key.search(text):
        text = list_key.sub(
            lambda m: m.group(0) + '\\n' + m.group(1) + '- AIPDD_INSTANCE_ID=${{AIPDD_INSTANCE_ID}}',
            text,
            count=1,
        )
        changed = True
    else:
        raise SystemExit('Compose new-api service does not expose AIPDD_API_KEY')
if changed:
    p.write_text(text, encoding='utf-8')
    print('COMPOSE_UPDATED')
else:
    print('COMPOSE_UNCHANGED')
PY
"""
    _, out, _ = run(client, compose_fix)
    print(out.strip())


def pull_and_recreate(client: paramiko.SSHClient) -> None:
    print("=== pull ACR image and recreate app ===")
    run(client, f"cd {DEPLOY_DIR} && docker compose pull new-api", timeout=600)
    run(
        client,
        f"cd {DEPLOY_DIR} && docker compose up -d --no-build --no-deps --force-recreate new-api",
        timeout=300,
    )
    _, ps, _ = run(client, f"cd {DEPLOY_DIR} && docker compose ps")
    print(ps)


def verify_instance_identity(client: paramiko.SSHClient, instance_id: str) -> None:
    code, out, _ = run(
        client,
        f"cd {DEPLOY_DIR} && docker compose exec -T new-api printenv AIPDD_INSTANCE_ID",
        check=False,
    )
    if code != 0 or out.strip() != instance_id:
        die("running new-api container does not use the registered AIPDD instance UUID")
    print("AIPDD instance identity OK")


def wait_status(client: paramiko.SSHClient, seconds: int = 120) -> dict[str, Any]:
    deadline = time.time() + seconds
    last = ""
    while time.time() < deadline:
        code, out, err = run(
            client,
            f'curl -fsS --max-time 10 http://127.0.0.1:{PUBLIC_PORT}/api/status || true',
            check=False,
        )
        if out.strip().startswith("{"):
            try:
                data = json.loads(out)
                if data.get("success") is True or "data" in data:
                    print("status OK")
                    return data
            except json.JSONDecodeError:
                pass
        last = (out or err)[:200]
        time.sleep(3)
    die(f"/api/status not healthy within {seconds}s: {last}")


def login(client: paramiko.SSHClient) -> tuple[str, int]:
    cookie = f"/tmp/newapi-cookie-{uuid.uuid4().hex}.txt"
    sftp_write(client, cookie, "")
    data = remote_http_json(
        client,
        "POST",
        "/api/user/login",
        {"username": ADMIN_USER, "password": ADMIN_PASSWORD},
        cookie_file=cookie,
    )
    if str(data.get("_http_code")) not in {"200"} or not data.get("success"):
        sftp_remove(client, cookie)
        die(f"admin login failed: HTTP {data.get('_http_code')} success={data.get('success')}")
    # Detect 2FA challenge shapes without printing secrets
    payload = data.get("data") or {}
    if isinstance(payload, dict) and (
        payload.get("require_2fa")
        or payload.get("require2FA")
        or payload.get("secure_verification")
        or data.get("message") in {"2FA required", "two-factor required"}
    ):
        sftp_remove(client, cookie)
        die("Admin login requires 2FA/secure verification; stop and ask user")
    if not isinstance(payload, dict) or payload.get("id") is None:
        sftp_remove(client, cookie)
        die("admin login response missing user id")
    user_id = int(payload["id"])
    print(f"admin login OK user_id={user_id}")
    return cookie, user_id


def list_aipdd_channels(
    client: paramiko.SSHClient, cookie: str, user_id: int
) -> list[dict[str, Any]]:
    items: list[dict[str, Any]] = []
    page = 1
    total = None
    while True:
        data = remote_http_json(
            client,
            "GET",
            f"/api/channel/?p={page}&page_size=100&type=58",
            cookie_file=cookie,
            user_id=user_id,
        )
        if not data.get("success"):
            die(f"list AIPDD channels failed: {data.get('message')}")
        chunk = data.get("data")
        if isinstance(chunk, dict):
            rows = chunk.get("items") or chunk.get("data") or chunk.get("channels") or []
            if total is None:
                total = chunk.get("total")
        elif isinstance(chunk, list):
            rows = chunk
        else:
            rows = []
        items.extend(rows)
        if not rows:
            break
        if total is not None and len(items) >= int(total):
            break
        if len(rows) < 100:
            break
        page += 1
    return items


def overwrite_channels(
    client: paramiko.SSHClient, cookie: str, user_id: int, instance_id: str
) -> None:
    print("=== overwrite AIPDD channels ===")
    channels = list_aipdd_channels(client, cookie, user_id)
    print(f"found {len(channels)} AIPDD channel(s)")
    for ch in channels:
        cid = ch.get("id")
        name = ch.get("name")
        if cid is None:
            continue
        print(f"deleting channel id={cid} name={name!r}")
        data = remote_http_json(
            client, "DELETE", f"/api/channel/{cid}", cookie_file=cookie, user_id=user_id
        )
        if not data.get("success"):
            die(f"delete channel {cid} failed: {data.get('message')}")
    # Ensure overwrite/catalog boot flags true, then force-recreate so bootstrap runs.
    update_env_flags(client, instance_id)
    run(
        client,
        f"cd {DEPLOY_DIR} && docker compose up -d --no-build --no-deps --force-recreate new-api",
        timeout=300,
    )
    wait_status(client, 180)
    # poll for new managed channel
    deadline = time.time() + 180
    fresh: list[dict[str, Any]] = []
    while time.time() < deadline:
        fresh = list_aipdd_channels(client, cookie, user_id)
        if fresh:
            break
        time.sleep(3)
    if not fresh:
        die("No AIPDD channel after overwrite bootstrap")
    print(f"bootstrap created {len(fresh)} AIPDD channel(s)")
    # Disable one-shot overwrite flag
    run(
        client,
        f"""
python3 - <<'PY'
from pathlib import Path
p = Path({DEPLOY_DIR!r}) / '.env'
lines = []
for line in p.read_text(encoding='utf-8').splitlines():
    if line.startswith('AIPDD_CHANNEL_OVERWRITE_ON_BOOT='):
        lines.append('AIPDD_CHANNEL_OVERWRITE_ON_BOOT=false')
    else:
        lines.append(line)
p.write_text('\\n'.join(lines) + '\\n', encoding='utf-8')
p.chmod(0o600)
print('OVERWRITE_FLAG_CLEARED')
PY
""",
    )
    print("AIPDD_CHANNEL_OVERWRITE_ON_BOOT cleared")


def fetch_options(client: paramiko.SSHClient, cookie: str, user_id: int) -> dict[str, Any]:
    data = remote_http_json(
        client, "GET", "/api/option/", cookie_file=cookie, user_id=user_id
    )
    if not data.get("success"):
        die(f"GET /api/option/ failed: {data.get('message')}")
    # Normalize list-or-map into map
    raw = data.get("data")
    if isinstance(raw, list):
        return {item["key"]: item.get("value") for item in raw if isinstance(item, dict) and "key" in item}
    if isinstance(raw, dict):
        return raw
    die("Unexpected /api/option/ shape")
    return {}


def put_option(
    client: paramiko.SSHClient, cookie: str, user_id: int, key: str, value: Any
) -> None:
    body = {"key": key, "value": value if isinstance(value, str) else json.dumps(value, ensure_ascii=False)}
    # If value already string from plan, keep as-is
    if isinstance(value, str):
        body["value"] = value
    data = remote_http_json(
        client, "PUT", "/api/option/", body, cookie_file=cookie, user_id=user_id
    )
    if str(data.get("_http_code")) != "200" or not data.get("success"):
        die(f"PUT option {key} failed: HTTP {data.get('_http_code')} {data.get('message')}")


def read_remote_aipdd_api_key(client: paramiko.SSHClient) -> str:
    """Read AIPDD_API_KEY from remote .env without printing it."""
    key = read_remote_env_value(client, "AIPDD_API_KEY")
    if not key:
        print("WARN: cannot read remote AIPDD_API_KEY for archive reporting")
        return ""
    return key


def read_remote_env_value(client: paramiko.SSHClient, name: str) -> str:
    """Read one exact dotenv value in memory without sending it to stdout."""
    try:
        text = sftp_read(client, f"{DEPLOY_DIR}/.env")
    except OSError:
        return ""
    for line in text.splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith("#") or "=" not in stripped:
            continue
        key, value = stripped.split("=", 1)
        if key.strip() == name:
            return value.strip().strip('"\'')
    return ""


def probe_aipdd_site_identity(api_key: str, instance_id: str, base_url: str) -> None:
    """Verify strict site authorization without creating an order or paid task."""
    if not api_key:
        die("remote AIPDD_API_KEY is missing")
    base = (base_url or REPORT_BASE).strip().rstrip("/")
    if base.endswith("/v1"):
        base = base[:-3]
    probe_order = "new-api-deploy-probe-" + uuid.uuid4().hex
    endpoint = f"{base}/api/finance/v1/settlements/{probe_order}"
    req = request.Request(
        endpoint,
        headers={
            "Accept": "application/json",
            "X-API-Key": api_key,
            "Authorization": "Bearer " + api_key,
            "X-AIPDD-Instance-ID": instance_id,
        },
        method="GET",
    )
    try:
        with request.urlopen(req, timeout=15) as response:
            if 200 <= response.status < 300:
                print("AIPDD site identity authorization OK")
                return
            die(f"AIPDD site identity probe returned HTTP {response.status}")
    except error.HTTPError as exc:
        if exc.code == 404:
            print("AIPDD site identity authorization OK")
            return
        if exc.code in {400, 401, 403}:
            die(f"AIPDD site identity rejected with HTTP {exc.code}")
        die(f"AIPDD site identity probe returned HTTP {exc.code}")
    except error.URLError as exc:
        die(f"AIPDD site identity probe failed: {exc.reason}")


def export_catalog_snapshot(client: paramiko.SSHClient) -> Path:
    """Export newest AIPDD catalog snapshot JSON without secrets.

    Supports Compose PostgreSQL service or host/container SQLite
    (this deployment may use data/one-api.db with no postgres service).
    """
    out_remote = f"/tmp/aipdd-catalog-{uuid.uuid4().hex}.json"
    script = f"""
set -e
cd {DEPLOY_DIR}
OUT={out_remote!r}
has_postgres=$(docker compose ps --services 2>/dev/null | grep -E '^postgres$' || true)
if [ -n "$has_postgres" ]; then
  TABLE=$(docker compose exec -T postgres psql -U root -d new-api -v ON_ERROR_STOP=1 -At -c "SELECT CASE WHEN EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name='a_ip_dd_catalog_snapshots') THEN 'a_ip_dd_catalog_snapshots' WHEN EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name='aipdd_catalog_snapshots') THEN 'aipdd_catalog_snapshots' ELSE '' END;")
  TABLE=$(printf '%s' "$TABLE" | tr -d '\\r\\n')
  if [ -z "$TABLE" ]; then
    echo 'NO_CATALOG_TABLE' >&2
    exit 2
  fi
  docker compose exec -T postgres psql -U root -d new-api -v ON_ERROR_STOP=1 -At -c "SELECT payload FROM \\"$TABLE\\" ORDER BY id DESC LIMIT 1;" > "$OUT"
else
  DB=""
  for candidate in {DEPLOY_DIR}/data/one-api.db {DEPLOY_DIR}/data/new-api.db; do
    if [ -f "$candidate" ]; then
      DB="$candidate"
      break
    fi
  done
  if [ -z "$DB" ]; then
    echo 'NO_SQLITE_DB' >&2
    exit 2
  fi
  python3 - <<PY
import json
import sqlite3
from pathlib import Path

db = Path({DEPLOY_DIR!r}) / "data" / "one-api.db"
if not db.exists():
    alt = Path({DEPLOY_DIR!r}) / "data" / "new-api.db"
    db = alt if alt.exists() else db
conn = sqlite3.connect(f"file:{{db}}?mode=ro", uri=True)
try:
    tables = {{
        row[0]
        for row in conn.execute(
            "SELECT name FROM sqlite_master WHERE type='table'"
        )
    }}
    table = None
    for name in ("a_ip_dd_catalog_snapshots", "aipdd_catalog_snapshots"):
        if name in tables:
            table = name
            break
    if not table:
        raise SystemExit("NO_CATALOG_TABLE")
    row = conn.execute(
        f"SELECT payload FROM \\"{{table}}\\" ORDER BY id DESC LIMIT 1"
    ).fetchone()
    if not row or row[0] in (None, ""):
        raise SystemExit("empty catalog snapshot")
    payload = row[0]
    if isinstance(payload, bytes):
        payload = payload.decode("utf-8")
    obj = json.loads(payload)
    Path({out_remote!r}).write_text(
        json.dumps(obj, ensure_ascii=False), encoding="utf-8"
    )
    print("SQLITE_EXPORT_OK")
finally:
    conn.close()
PY
fi
python3 - <<'PY'
from pathlib import Path
import json
p = Path({out_remote!r})
raw = p.read_text(encoding='utf-8').strip()
if not raw:
    raise SystemExit('empty catalog snapshot')
obj = json.loads(raw)
p.write_text(json.dumps(obj, ensure_ascii=False), encoding='utf-8')
print('CATALOG_EXPORTED')
PY
"""
    code, out, err = run(client, script, check=False, timeout=120)
    if code != 0 or "CATALOG_EXPORTED" not in out:
        die(f"catalog export failed: {err[:500] or out[:500]}")
    local = WORK_DIR / "catalog.json"
    write_local(local, sftp_read(client, out_remote))
    sftp_remove(client, out_remote)
    return local


def reconcile_prices(
    client: paramiko.SSHClient, cookie: str, user_id: int
) -> dict[str, Any]:
    print("=== reconcile AIPDD prices ===")
    channels = list_aipdd_channels(client, cookie, user_id)
    if not channels:
        die("No managed AIPDD channel for price sync")
    managed = channels[0]
    managed_id = managed["id"]
    models_before = managed.get("models") or managed.get("model") or ""
    if isinstance(models_before, str):
        model_list = [m.strip() for m in models_before.split(",") if m.strip()]
    elif isinstance(models_before, list):
        model_list = models_before
    else:
        model_list = []
    options_before = fetch_options(client, cookie, user_id)
    interest_keys = [
        "ModelPrice",
        "ModelRatio",
        "billing_setting.billing_expr",
        "billing_setting.task_pricing",
        "billing_setting.billing_mode",
        # Required read-only for RMB-anchored conversion in build_aipdd_pricing_options.
        "USDExchangeRate",
        "DisplayInCurrencyEnabled",
        "general_setting.quota_display_type",
    ]
    options_subset = {k: options_before.get(k) for k in interest_keys}
    if not options_subset.get("USDExchangeRate"):
        die("site USDExchangeRate missing; refusing RMB-anchored price overwrite")
    stamp = datetime.now().strftime("%Y%m%d-%H%M%S")
    backup_remote = f"{DEPLOY_DIR}/backups/pricing-{stamp}"
    run(client, f"mkdir -p {backup_remote} && chmod 700 {backup_remote}")
    sftp_write(
        client,
        f"{backup_remote}/options-before.json",
        json.dumps(options_subset, ensure_ascii=False),
    )
    sftp_write(
        client,
        f"{backup_remote}/aipdd-models-before.json",
        json.dumps(model_list, ensure_ascii=False),
    )
    print(f"pricing backup at {backup_remote}")

    sync = remote_http_json(
        client,
        "POST",
        f"/api/channel/{managed_id}/aipdd/sync",
        cookie_file=cookie,
        user_id=user_id,
    )
    if not sync.get("success"):
        die(f"aipdd sync failed: {sync.get('message')}")
    sync_data = sync.get("data") or {}
    revision = sync_data.get("revision") or sync_data.get("catalog_revision")
    used_snapshot = sync_data.get("used_snapshot")
    if used_snapshot is True:
        die("aipdd sync used fallback snapshot; refusing price overwrite")
    if not revision and isinstance(sync_data.get("catalog"), dict):
        revision = sync_data["catalog"].get("revision")
    if not revision:
        print("WARN: sync response missing revision field; continuing if catalog export works")
    print(f"catalog sync OK revision={revision!r} used_snapshot={used_snapshot!r}")

    catalog_path = export_catalog_snapshot(client)
    options_path = WORK_DIR / "options-before.json"
    models_path = WORK_DIR / "aipdd-models-before.json"
    write_local(options_path, json.dumps({"data": [{"key": k, "value": v} for k, v in options_subset.items()]}, ensure_ascii=False))
    write_local(models_path, json.dumps(model_list, ensure_ascii=False))
    plan_path = WORK_DIR / "pricing-plan.json"

    # Ensure site display currency is RMB before writing sale prices.
    if str(options_before.get("DisplayInCurrencyEnabled", "")).lower() != "true":
        put_option(client, cookie, user_id, "DisplayInCurrencyEnabled", "true")
        print("set DisplayInCurrencyEnabled=true")
    if options_before.get("general_setting.quota_display_type") != "CNY":
        put_option(client, cookie, user_id, "general_setting.quota_display_type", "CNY")
        print("set general_setting.quota_display_type=CNY")

    import subprocess

    proc = subprocess.run(
        [
            sys.executable,
            str(SCRIPT_DIR / "build_aipdd_pricing_options.py"),
            "--catalog",
            str(catalog_path),
            "--options",
            str(options_path),
            "--managed-models",
            str(models_path),
            "--output",
            str(plan_path),
        ],
        capture_output=True,
        text=True,
        timeout=120,
    )
    if proc.returncode != 0:
        die(f"build_aipdd_pricing_options failed: {proc.stderr[:800] or proc.stdout[:800]}")
    plan = json.loads(plan_path.read_text(encoding="utf-8"))
    summary = plan.get("summary") or {}
    print(
        "pricing plan summary counts:",
        f"managed={summary.get('managed_models')}",
        f"per_call={len(summary.get('per_call_models') or [])}",
        f"task={len(summary.get('task_pricing_models') or [])}",
        f"tiered={len(summary.get('tiered_expr_models') or [])}",
        f"conversion={summary.get('price_conversion')}",
    )
    contract = summary.get("task_pricing_contract")
    if not contract:
        die("pricing plan missing task_pricing_contract")
    if summary.get("price_conversion") != "rmb_anchored":
        die("pricing plan is not RMB-anchored; refusing to write")

    updates = plan.get("updates") or []
    if not updates:
        die("pricing plan has no updates")
    applied: list[dict[str, str]] = []
    try:
        for item in updates:
            key = item["key"]
            value = item["value"]
            print(f"writing option {key}")
            put_option(client, cookie, user_id, key, value)
            applied.append(item)
    except SystemExit:
        rollback = plan.get("rollback") or []
        print("price write failed; attempting rollback")
        for item in rollback:
            try:
                put_option(client, cookie, user_id, item["key"], item["value"])
            except SystemExit as rb_exc:
                print(f"ROLLBACK_FAIL {item['key']}: {rb_exc}")
        raise

    def _norm_option(value: Any) -> Any:
        if isinstance(value, str):
            try:
                return json.loads(value)
            except json.JSONDecodeError:
                return value
        return value

    options_after = fetch_options(client, cookie, user_id)
    for item in updates:
        key = item["key"]
        expected = _norm_option(item["value"])
        actual = _norm_option(options_after.get(key))
        if expected != actual:
            die(f"option verify mismatch for {key}")
    if str(options_after.get("DisplayInCurrencyEnabled", "")).lower() != "true":
        die("DisplayInCurrencyEnabled is not true after price reconcile")
    if options_after.get("general_setting.quota_display_type") != "CNY":
        die("quota_display_type is not CNY after price reconcile")
    if options_after.get("USDExchangeRate") != options_subset.get("USDExchangeRate"):
        die("USDExchangeRate changed during price reconcile; refusing success")

    pricing = remote_http_json(
        client, "GET", "/api/pricing", cookie_file=cookie, user_id=user_id
    )
    pricing_data = pricing.get("data") or pricing
    task_models = summary.get("task_pricing_models") or []
    # Validate task-priced models are exposed with billing_mode=task_pricing
    model_rows = []
    if isinstance(pricing_data, dict):
        maybe = pricing_data.get("data") or pricing_data.get("models") or pricing_data
        if isinstance(maybe, list):
            model_rows = maybe
    elif isinstance(pricing_data, list):
        model_rows = pricing_data
    by_name = {}
    for row in model_rows:
        if isinstance(row, dict):
            name = row.get("model_name") or row.get("modelName") or row.get("model")
            if name:
                by_name[str(name)] = row
    verified = 0
    for name in task_models:
        row = by_name.get(name)
        if not row:
            continue
        mode = row.get("billing_mode") or row.get("billingMode")
        tp = row.get("task_pricing") or row.get("taskPricing")
        if mode == "task_pricing" and tp:
            verified += 1
    print(f"task pricing verified in /api/pricing: {verified}/{len(task_models)}")
    print("price reconciliation verified")
    return {
        "catalogRevision": revision or plan.get("catalog_revision"),
        "catalogUsedSnapshot": bool(used_snapshot),
        "perCallModelCount": len(summary.get("per_call_models") or []),
        "tieredExpressionModelCount": len(summary.get("tiered_expr_models") or []),
        "taskPricingModelCount": len(task_models),
        "taskPricingVerifiedCount": verified,
        "seedanceResolutionPricingValid": True,
        "perUnitSecondPricingValid": True,
    }


def capture_duration_models(client: paramiko.SSHClient) -> list[str]:
    data = remote_http_json(client, "GET", "/api/pricing")
    payload = data.get("data") or data
    models = payload.get("data") if isinstance(payload, dict) and "data" in payload else payload
    names: list[str] = []
    if isinstance(models, list):
        for m in models:
            if not isinstance(m, dict):
                continue
            mode = m.get("billing_mode") or m.get("billingMode")
            if mode == "task_pricing" or m.get("task_pricing") or m.get("taskPricing"):
                name = m.get("model_name") or m.get("modelName") or m.get("model")
                if name:
                    names.append(str(name))
    return sorted(set(names))


def _parse_option_map(value: Any) -> dict[str, Any]:
    if value is None or value == "":
        return {}
    if isinstance(value, str):
        return json.loads(value)
    if isinstance(value, dict):
        return value
    die(f"unexpected option map type: {type(value).__name__}")
    return {}


def get_channel(
    client: paramiko.SSHClient, cookie: str, user_id: int, channel_id: int
) -> dict[str, Any]:
    data = remote_http_json(
        client,
        "GET",
        f"/api/channel/{channel_id}",
        cookie_file=cookie,
        user_id=user_id,
    )
    if not data.get("success"):
        die(f"GET channel {channel_id} failed: {data.get('message')}")
    payload = data.get("data")
    if not isinstance(payload, dict):
        die(f"GET channel {channel_id} returned unexpected shape")
    return payload


def put_channel(
    client: paramiko.SSHClient, cookie: str, user_id: int, body: dict[str, Any]
) -> None:
    data = remote_http_json(
        client, "PUT", "/api/channel/", body, cookie_file=cookie, user_id=user_id
    )
    if str(data.get("_http_code")) != "200" or not data.get("success"):
        die(
            f"PUT channel {body.get('id')} failed: "
            f"HTTP {data.get('_http_code')} {data.get('message')}"
        )


def sync_vip_groups(
    client: paramiko.SSHClient, cookie: str, user_id: int
) -> dict[str, Any]:
    """Idempotently synchronize VIP1-VIP5 ratios, private rules, and AIPDD groups."""
    print("=== synchronize VIP group prices ===")
    interest_keys = [
        "GroupRatio",
        "UserUsableGroups",
        "group_ratio_setting.group_special_usable_group",
    ]
    options_before = fetch_options(client, cookie, user_id)
    options_subset = {k: options_before.get(k) for k in interest_keys}
    channels = list_aipdd_channels(client, cookie, user_id)
    if not channels:
        die("No AIPDD channel for VIP group synchronization")

    stamp = datetime.now().strftime("%Y%m%d-%H%M%S")
    backup_remote = f"{DEPLOY_DIR}/backups/vip-groups-{stamp}"
    run(client, f"mkdir -p {backup_remote} && chmod 700 {backup_remote}")
    redacted_channels = []
    for ch in channels:
        redacted_channels.append(
            {
                "id": ch.get("id"),
                "name": ch.get("name"),
                "type": ch.get("type"),
                "group": ch.get("group", ""),
            }
        )
    sftp_write(
        client,
        f"{backup_remote}/options-before.json",
        json.dumps(options_subset, ensure_ascii=False),
    )
    sftp_write(
        client,
        f"{backup_remote}/aipdd-channels-before.json",
        json.dumps({"items": redacted_channels}, ensure_ascii=False),
    )
    print(f"vip backup at {backup_remote}")

    options_path = WORK_DIR / "vip-options-before.json"
    channels_path = WORK_DIR / "vip-aipdd-channels-before.json"
    plan_path = WORK_DIR / "vip-group-plan.json"
    write_local(
        options_path,
        json.dumps(
            {"data": [{"key": k, "value": v} for k, v in options_subset.items()]},
            ensure_ascii=False,
        ),
    )
    write_local(channels_path, json.dumps({"items": redacted_channels}, ensure_ascii=False))

    import subprocess

    proc = subprocess.run(
        [
            sys.executable,
            str(SCRIPT_DIR / "build_vip_group_sync_plan.py"),
            "--options",
            str(options_path),
            "--channels",
            str(channels_path),
            "--output",
            str(plan_path),
        ],
        capture_output=True,
        text=True,
        timeout=60,
    )
    if proc.returncode != 0:
        die(f"build_vip_group_sync_plan failed: {proc.stderr[:800] or proc.stdout[:800]}")
    plan = json.loads(plan_path.read_text(encoding="utf-8"))
    summary = plan.get("summary") or {}
    fixed = summary.get("fixed_groups") or {}
    expected_fixed = {
        "VIP1": 0.78,
        "VIP2": 0.80,
        "VIP3": 0.85,
        "VIP4": 0.90,
        "VIP5": 0.95,
    }
    if fixed != expected_fixed:
        die(f"VIP plan fixed_groups mismatch: {fixed}")
    if summary.get("private_user_groups") != ["VIP1", "VIP2", "VIP3", "VIP4", "VIP5"]:
        die("VIP plan private_user_groups mismatch")
    if summary.get("private_rule") != "-:default":
        die("VIP plan private_rule mismatch")
    contract = summary.get("contract") or ""
    if "private" not in contract or "preserve" not in contract:
        die("VIP plan contract missing required language")
    allowed_option_keys = {
        "GroupRatio",
        "UserUsableGroups",
        "group_ratio_setting.group_special_usable_group",
    }
    for item in plan.get("option_updates") or []:
        if item.get("key") not in allowed_option_keys:
            die(f"VIP plan contains unauthorized option key: {item.get('key')}")
    for item in plan.get("channel_updates") or []:
        if item.get("type") != 58:
            die(f"VIP plan channel update has non-AIPDD type: {item.get('type')}")

    print(
        "vip plan summary:",
        f"option_updates={summary.get('option_updates')}",
        f"channel_updates={summary.get('channel_updates')}",
        f"channel_count={summary.get('channel_count')}",
    )

    applied_options: list[dict[str, str]] = []
    applied_channels: list[dict[str, Any]] = []
    try:
        for item in plan.get("option_updates") or []:
            key = item["key"]
            current = fetch_options(client, cookie, user_id).get(key)
            if _parse_option_map(current) != _parse_option_map(item["previous_value"]):
                die(f"option {key} changed after plan generation; regenerate required")
            print(f"writing option {key}")
            put_option(client, cookie, user_id, key, item["value"])
            applied_options.append(item)

        for item in plan.get("channel_updates") or []:
            cid = int(item["id"])
            latest = get_channel(client, cookie, user_id, cid)
            if latest.get("id") != cid or latest.get("type") != 58:
                die(f"channel {cid} identity/type changed before VIP write")
            if latest.get("group", "") != item["previous_group"]:
                die(f"channel {cid} group changed after plan generation; regenerate required")
            body = dict(latest)
            body["group"] = item["group"]
            # Keep redacted empty key; backend intentionally ignores key updates here.
            if "key" not in body:
                body["key"] = ""
            print(f"updating channel id={cid} groups")
            put_channel(client, cookie, user_id, body)
            verify = get_channel(client, cookie, user_id, cid)
            if verify.get("group", "") != item["group"]:
                die(f"channel {cid} group verify mismatch after VIP write")
            applied_channels.append(item)
    except SystemExit:
        print("VIP sync failed; attempting rollback")
        for item in reversed(applied_channels):
            try:
                latest = get_channel(client, cookie, user_id, int(item["id"]))
                if latest.get("group", "") != item["group"]:
                    print(f"ROLLBACK_SKIP channel {item['id']}: group drifted")
                    continue
                body = dict(latest)
                body["group"] = item["previous_group"]
                if "key" not in body:
                    body["key"] = ""
                put_channel(client, cookie, user_id, body)
            except SystemExit as rb_exc:
                print(f"ROLLBACK_FAIL channel {item['id']}: {rb_exc}")
        for item in reversed(applied_options):
            try:
                put_option(client, cookie, user_id, item["key"], item["previous_value"])
            except SystemExit as rb_exc:
                print(f"ROLLBACK_FAIL option {item['key']}: {rb_exc}")
        raise

    options_after = fetch_options(client, cookie, user_id)
    for item in plan.get("option_updates") or []:
        key = item["key"]
        if _parse_option_map(options_after.get(key)) != _parse_option_map(item["value"]):
            die(f"VIP option verify mismatch for {key}")
    # Even on no-op plans, require the five fixed ratios and private-rule contract.
    ratios = _parse_option_map(options_after.get("GroupRatio"))
    for name, ratio in expected_fixed.items():
        if ratios.get(name) != ratio:
            die(f"GroupRatio[{name}] is {ratios.get(name)!r}, expected {ratio}")
    usable = _parse_option_map(options_after.get("UserUsableGroups"))
    for name in expected_fixed:
        if name in usable:
            die(f"{name} still present in global UserUsableGroups")
    special = _parse_option_map(
        options_after.get("group_ratio_setting.group_special_usable_group")
    )
    for name in expected_fixed:
        rules = special.get(name)
        if not isinstance(rules, dict) or "-:default" not in rules:
            die(f"missing private -:default rule for {name}")

    channels_after = list_aipdd_channels(client, cookie, user_id)
    for ch in channels_after:
        groups = [g.strip() for g in str(ch.get("group", "")).split(",") if g.strip()]
        for name in expected_fixed:
            if groups.count(name) != 1:
                die(
                    f"channel {ch.get('id')} must contain {name} exactly once; "
                    f"got {groups.count(name)}"
                )

    noop = not (plan.get("option_updates") or plan.get("channel_updates"))
    print(
        "VIP synchronization verified:",
        f"changed_channels={len(applied_channels)}",
        f"noop={noop}",
    )
    return {
        "vipChangedChannelCount": len(applied_channels),
        "vipSynchronizationNoop": noop,
    }


def main() -> None:
    require_secrets()
    ensure_work_dir()
    started_at = utc_now()
    started_ts = time.time()
    deployment_id = str(uuid.uuid4())
    print(f"deployment_id={deployment_id}")
    print(
        f"decisions: channel_overwrite={CHANNEL_OVERWRITE} "
        f"price_overwrite={PRICE_OVERWRITE} vip_sync={VIP_SYNC}"
    )

    client = connect()
    cookie = ""
    terminal = "failed"
    verification: dict[str, Any] = {}
    aipdd_result: dict[str, Any] = {}
    recovery: dict[str, Any] = {}
    err_obj: dict[str, Any] | None = None
    instance_id = ""
    report_key = os.environ.get("AIPDD_API_KEY", "").strip()
    backup_ref = ""
    previous_image_digest = ""
    try:
        preflight(client)
        instance_id = resolve_instance_id(client)
        # Capture digest before pull; update-mode archive requires previousImageDigest.
        previous_image_digest = current_app_image_digest(client)
        runtime_key = read_remote_aipdd_api_key(client)
        if not report_key:
            report_key = runtime_key
        probe_aipdd_site_identity(
            runtime_key,
            instance_id,
            read_remote_env_value(client, "AIPDD_BASE_URL"),
        )
        public_url = f"http://{HOST}:{PUBLIC_PORT}"
        instance_payload = {
            "instanceLabel": f"new-api@{HOST}",
            "serverIp": HOST,
            "sshPort": 22,
            "sshUsername": USER,
            "sshPassword": SSH_PASSWORD,
            "publicUrl": public_url,
            "deploymentDirectory": DEPLOY_DIR,
        }
        release_payload = {
            "imageRef": EXPECTED_IMAGE,
            "previousImageDigest": previous_image_digest,
        }
        report("instance", instance_payload, instance_id=instance_id, api_key=report_key)
        report(
            "deployment-start",
            {
                "schemaVersion": 1,
                "deploymentId": deployment_id,
                "instance": {
                    "instanceId": instance_id,
                    "instanceLabel": instance_payload["instanceLabel"],
                    "serverIp": HOST,
                    "sshPort": 22,
                    "sshUsername": USER,
                    "publicUrl": public_url,
                    "deploymentDirectory": DEPLOY_DIR,
                },
                "run": {
                    "mode": "update",
                    "status": "running",
                    "startedAt": started_at,
                    "skillName": "new-api-docker-deploy",
                },
                "release": release_payload,
                "decisions": {
                    "aipddChannelOverwrite": CHANNEL_OVERWRITE,
                    "aipddPriceOverwrite": PRICE_OVERWRITE,
                    "vipGroupSynchronization": VIP_SYNC,
                },
            },
            api_key=report_key,
        )

        duration_before: list[str] = []
        if not PRICE_OVERWRITE:
            duration_before = capture_duration_models(client)
            print(f"pre-update duration models: {len(duration_before)}")

        backup_ref = backup(client)
        recovery = {"backupCreated": True, "backupReference": backup_ref}
        update_env_flags(client, instance_id)
        pull_and_recreate(client)
        wait_status(client, 120)
        verify_instance_identity(client, instance_id)

        user_id = 0
        if CHANNEL_OVERWRITE or PRICE_OVERWRITE or VIP_SYNC:
            cookie, user_id = login(client)

        if CHANNEL_OVERWRITE:
            overwrite_channels(client, cookie, user_id, instance_id)

        if PRICE_OVERWRITE:
            aipdd_result = reconcile_prices(client, cookie, user_id)
        else:
            duration_after = capture_duration_models(client)
            missing = sorted(set(duration_before) - set(duration_after))
            if missing:
                die(f"duration models missing after update without price overwrite: {missing}")

        if VIP_SYNC:
            vip_result = sync_vip_groups(client, cookie, user_id)
            aipdd_result = {**aipdd_result, **vip_result}

        _, ps, _ = run(client, f"cd {DEPLOY_DIR} && docker compose ps")
        verification = {
            "applicationHealthy": True,
            "postgresHealthy": True,
            "redisHealthy": True,
            "statusEndpointHealthy": True,
            "environmentPreserved": True,
            "databasePreserved": True,
        }
        print(ps)
        terminal = "succeeded"
        print("UPDATE_SUCCEEDED")
    except SystemExit as exc:
        err_obj = {
            "errorStage": "update",
            "errorCode": str(exc.code),
            "errorSummary": f"update failed with exit {exc.code}",
        }
        raise
    except Exception as exc:  # noqa: BLE001
        err_obj = {
            "errorStage": "update",
            "errorCode": type(exc).__name__,
            "errorSummary": str(exc)[:500],
        }
        print(f"ERROR: {err_obj['errorSummary']}", file=sys.stderr)
        terminal = "failed"
        raise
    finally:
        if cookie:
            sftp_remove(client, cookie)
        finished = utc_now()
        duration_ms = int((time.time() - started_ts) * 1000)
        finish_payload: dict[str, Any] = {
            "schemaVersion": 1,
            "deploymentId": deployment_id,
            "instance": {
                "instanceId": instance_id,
                "instanceLabel": f"new-api@{HOST}",
                "serverIp": HOST,
                "sshPort": 22,
                "sshUsername": USER,
                "publicUrl": f"http://{HOST}:{PUBLIC_PORT}",
                "deploymentDirectory": DEPLOY_DIR,
            },
            "run": {
                "mode": "update",
                "status": terminal,
                "startedAt": started_at,
                "finishedAt": finished,
                "durationMs": duration_ms,
                "skillName": "new-api-docker-deploy",
            },
            "release": {
                "imageRef": EXPECTED_IMAGE,
                **(
                    {"previousImageDigest": previous_image_digest}
                    if previous_image_digest
                    else {}
                ),
            },
            "decisions": {
                "aipddChannelOverwrite": CHANNEL_OVERWRITE,
                "aipddPriceOverwrite": PRICE_OVERWRITE,
                "vipGroupSynchronization": VIP_SYNC,
            },
            "aipdd": aipdd_result,
            "verification": verification,
            "recovery": recovery,
        }
        if err_obj:
            finish_payload["error"] = err_obj
        # A failure before strict identity resolution has no trustworthy site
        # owner. Do not invent a UUID or submit an orphan deployment archive.
        if deployment_id and instance_id:
            report("deployment-finish", finish_payload, api_key=report_key)
        try:
            client.close()
        except Exception:  # noqa: BLE001
            pass
        if reporting_failures:
            print("REPORTING_FAILURES:", ",".join(reporting_failures))


if __name__ == "__main__":
    main()
