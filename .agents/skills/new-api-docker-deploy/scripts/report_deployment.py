#!/usr/bin/env python3
"""Safely report New API instances, credentials, and deployment archives."""

from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.request
import uuid
from pathlib import Path
from typing import Any


DEFAULT_BASE_URL = "https://api.aipdd.work"
INSTANCE_ID_FILE = ".aipdd-instance-id"
STAGES = ("instance", "credentials", "deployment-start", "deployment-finish")
TERMINAL_STATUSES = {
    "succeeded",
    "failed",
    "rolled_back",
    "rollback_failed",
    "abandoned",
}
CREDENTIAL_TYPES = {
    "ssh_password",
    "admin_password",
    "admin_api_token",
    "postgres_password",
    "redis_password",
    "session_secret",
    "crypto_secret",
}
INSTANCE_FIELDS = {
    "instanceLabel",
    "serverIp",
    "sshPort",
    "sshUsername",
    "sshPassword",
    "domain",
    "publicUrl",
    "deploymentDirectory",
}
DEPLOYMENT_FIELDS = {
    "schemaVersion",
    "deploymentId",
    "instance",
    "run",
    "release",
    "decisions",
    "aipdd",
    "verification",
    "recovery",
    "error",
}
DEPLOYMENT_NESTED_FIELDS = {
    "instance": {
        "instanceId",
        "instanceLabel",
        "publicUrl",
        "deploymentDirectory",
        "serverIp",
        "sshPort",
        "sshUsername",
        "sshPassword",
        "clearSshPassword",
        "domain",
    },
    "run": {
        "mode",
        "status",
        "triggerSource",
        "startedAt",
        "finishedAt",
        "durationMs",
        "skillName",
        "skillRevision",
    },
    "release": {
        "imageRef",
        "previousImageDigest",
        "imageDigest",
        "applicationVersion",
    },
    "decisions": {
        "aipddChannelOverwrite",
        "aipddPriceOverwrite",
        "vipGroupSynchronization",
    },
    "aipdd": {
        "catalogRevision",
        "catalogUsedSnapshot",
        "perCallModelCount",
        "tieredExpressionModelCount",
        "taskPricingModelCount",
        "taskPricingVerifiedCount",
        "seedanceResolutionPricingValid",
        "perUnitSecondPricingValid",
        "vipChangedChannelCount",
        "vipSynchronizationNoop",
    },
    "verification": {
        "applicationHealthy",
        "postgresHealthy",
        "redisHealthy",
        "statusEndpointHealthy",
        "environmentPreserved",
        "databasePreserved",
    },
    "recovery": {
        "backupCreated",
        "backupReference",
        "rollbackAttempted",
        "rollbackSucceeded",
        "localRollbackImageDigest",
    },
    "error": {"errorStage", "errorCode", "errorSummary"},
}


class ValidationError(ValueError):
    """Input does not satisfy the reporting contract."""


def _uuid(value: Any, label: str) -> str:
    try:
        return str(uuid.UUID(str(value)))
    except (ValueError, TypeError, AttributeError) as exc:
        raise ValidationError(f"{label} must be a UUID") from exc


def _object(value: Any, label: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        raise ValidationError(f"{label} must be a JSON object")
    return value


def _reject_unknown(value: dict[str, Any], allowed: set[str], label: str) -> None:
    unknown = sorted(set(value) - allowed)
    if unknown:
        raise ValidationError(f"{label} contains {len(unknown)} unknown field(s)")


def _sensitive_key(key: Any) -> bool:
    normalized = "".join(character for character in str(key).lower() if character.isalnum())
    return any(marker in normalized for marker in ("password", "secret", "apikey", "token"))


def redact(value: Any) -> Any:
    """Return a recursively copied value with secrets replaced."""
    if isinstance(value, dict):
        return {
            key: ("<redacted>" if _sensitive_key(key) else redact(item))
            for key, item in value.items()
        }
    if isinstance(value, list):
        return [redact(item) for item in value]
    return value


def resolve_instance_id(path: Path | str, create_if_missing: bool = False) -> uuid.UUID:
    """Read or securely create <deployment-dir>/.aipdd-instance-id."""
    deployment_dir = Path(path)
    id_path = deployment_dir / INSTANCE_ID_FILE
    if id_path.exists():
        try:
            return uuid.UUID(id_path.read_text(encoding="utf-8").strip())
        except (OSError, ValueError) as exc:
            raise ValidationError(f"{id_path} does not contain a valid UUID") from exc
    if not create_if_missing:
        raise ValidationError(f"{id_path} does not exist")

    deployment_dir.mkdir(parents=True, exist_ok=True)
    instance_id = uuid.uuid4()
    try:
        descriptor = os.open(id_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(descriptor, "w", encoding="utf-8", newline="\n") as handle:
            handle.write(f"{instance_id}\n")
        os.chmod(id_path, 0o600)
    except FileExistsError:
        return resolve_instance_id(deployment_dir)
    return instance_id


def load_payload(path: Path | None) -> dict[str, Any]:
    if path is None:
        source = sys.stdin
        try:
            value = json.load(source)
        except (json.JSONDecodeError, UnicodeError) as exc:
            raise ValidationError("stdin does not contain valid JSON") from exc
    else:
        try:
            if os.name != "nt" and (path.stat().st_mode & 0o077):
                raise ValidationError(f"{path} must not be accessible by group or others")
            with path.open("r", encoding="utf-8") as handle:
                value = json.load(handle)
        except ValidationError:
            raise
        except (OSError, json.JSONDecodeError, UnicodeError) as exc:
            raise ValidationError(f"cannot read JSON payload file {path}") from exc
    return _object(value, "payload")


def _validate_instance(payload: dict[str, Any]) -> None:
    _reject_unknown(payload, INSTANCE_FIELDS, "instance payload")
    label = payload.get("instanceLabel")
    if not isinstance(label, str) or not label.strip():
        raise ValidationError("instanceLabel is required")
    port = payload.get("sshPort")
    if port is not None and (isinstance(port, bool) or not isinstance(port, int) or not 1 <= port <= 65535):
        raise ValidationError("sshPort must be an integer from 1 to 65535")


def _validate_credentials(payload: dict[str, Any]) -> None:
    _reject_unknown(payload, {"mode", "credentials"}, "credentials payload")
    if payload.get("mode") not in {"initial", "update"}:
        raise ValidationError("credentials mode must be initial or update")
    credentials = payload.get("credentials")
    if not isinstance(credentials, list) or not credentials:
        raise ValidationError("credentials must be a non-empty array")
    seen: set[str] = set()
    for index, item in enumerate(credentials):
        item = _object(item, f"credentials[{index}]")
        _reject_unknown(item, {"type", "username", "secret"}, f"credentials[{index}]")
        credential_type = item.get("type")
        if credential_type not in CREDENTIAL_TYPES:
            raise ValidationError(f"credentials[{index}].type is not allowed")
        if credential_type in seen:
            raise ValidationError(f"duplicate credential type: {credential_type}")
        seen.add(credential_type)
        if not isinstance(item.get("secret"), str) or not item["secret"]:
            raise ValidationError(f"credentials[{index}].secret is required")
        if credential_type == "admin_password" and item.get("username") != "root":
            raise ValidationError("admin_password username must be root")


def _validate_deployment(payload: dict[str, Any], stage: str) -> str:
    _reject_unknown(payload, DEPLOYMENT_FIELDS, "deployment payload")
    if payload.get("schemaVersion") != 1:
        raise ValidationError("schemaVersion must be 1")
    deployment_id = _uuid(payload.get("deploymentId"), "deploymentId")
    for required in ("instance", "run"):
        if required not in payload:
            raise ValidationError(f"{required} is required")
    for key, allowed in DEPLOYMENT_NESTED_FIELDS.items():
        if key in payload and payload[key] is not None:
            nested = _object(payload[key], key)
            _reject_unknown(nested, allowed, key)

    instance = payload["instance"]
    _uuid(instance.get("instanceId"), "instance.instanceId")
    if not isinstance(instance.get("instanceLabel"), str) or not instance["instanceLabel"].strip():
        raise ValidationError("instance.instanceLabel is required")
    run = payload["run"]
    if run.get("mode") not in {"initial", "update"}:
        raise ValidationError("run.mode must be initial or update")
    if not isinstance(run.get("startedAt"), str) or not run["startedAt"].strip():
        raise ValidationError("run.startedAt is required")
    status = run.get("status")
    if stage == "deployment-start" and status != "running":
        raise ValidationError("deployment-start requires run.status=running")
    if stage == "deployment-finish" and status not in TERMINAL_STATUSES:
        raise ValidationError("deployment-finish requires a terminal run.status")
    if stage == "deployment-finish" and not run.get("finishedAt"):
        raise ValidationError("deployment-finish requires run.finishedAt")
    return deployment_id


def shape_request(
    stage: str,
    payload: dict[str, Any],
    base_url: str = DEFAULT_BASE_URL,
    instance_id: str | None = None,
) -> tuple[str, str, dict[str, Any]]:
    """Validate input and return (method, URL, exact API body)."""
    base = base_url.rstrip("/")
    if not base.startswith(("https://", "http://")):
        raise ValidationError("base URL must start with http:// or https://")
    if stage == "instance":
        route_id = _uuid(instance_id, "instance ID")
        _validate_instance(payload)
        path = f"/v1/new-api/instances/{route_id}"
    elif stage == "credentials":
        route_id = _uuid(instance_id, "instance ID")
        _validate_credentials(payload)
        path = f"/v1/new-api/instances/{route_id}/credentials"
    elif stage in {"deployment-start", "deployment-finish"}:
        deployment_id = _validate_deployment(payload, stage)
        path = f"/v1/new-api/deployments/{deployment_id}"
    else:
        raise ValidationError(f"unsupported stage: {stage}")
    return "PUT", f"{base}{path}", payload


def _load_key(key_file: Path | None) -> str:
    if key_file is None:
        key = os.environ.get("AIPDD_API_KEY", "").strip()
    else:
        try:
            if os.name != "nt" and (key_file.stat().st_mode & 0o077):
                raise ValidationError(f"{key_file} must not be accessible by group or others")
            key = key_file.read_text(encoding="utf-8").strip()
        except ValidationError:
            raise
        except OSError as exc:
            raise ValidationError(f"cannot read key file {key_file}") from exc
    if not key:
        raise ValidationError("AIPDD_API_KEY or --key-file is required")
    return key


def send_request(
    method: str,
    url: str,
    payload: dict[str, Any],
    key: str,
    timeout: float,
) -> None:
    data = json.dumps(payload, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    request = urllib.request.Request(
        url,
        data=data,
        method=method,
        headers={
            "Authorization": f"Bearer {key}",
            "Content-Type": "application/json",
            "Accept": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            if not 200 <= response.status < 300:
                raise urllib.error.HTTPError(
                    url, response.status, "unexpected response", response.headers, None
                )
            response.read()
    except urllib.error.HTTPError as exc:
        raise RuntimeError(f"API request failed with HTTP {exc.code}") from exc
    except (urllib.error.URLError, TimeoutError, OSError) as exc:
        raise RuntimeError(f"network request failed: {type(exc).__name__}") from exc


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Report a New API deployment stage without exposing secrets."
    )
    parser.add_argument("--stage", choices=STAGES, required=True)
    parser.add_argument("--payload", type=Path, help="mode-600 JSON file; omit to read stdin")
    parser.add_argument("--instance-id", help="required for instance and credentials stages")
    parser.add_argument("--base-url", default=DEFAULT_BASE_URL)
    parser.add_argument("--key-file", type=Path, help="protected file containing AIPDD API key")
    parser.add_argument("--timeout", type=float, default=15.0)
    parser.add_argument("--dry-run", action="store_true")
    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    try:
        if args.timeout <= 0:
            raise ValidationError("timeout must be positive")
        payload = load_payload(args.payload)
        method, url, body = shape_request(
            args.stage, payload, args.base_url, args.instance_id
        )
        if args.dry_run:
            preview = {"method": method, "url": url, "payload": redact(body)}
            print(json.dumps(preview, ensure_ascii=False, indent=2))
            return 0
        key = _load_key(args.key_file)
        send_request(method, url, body, key, args.timeout)
        print(f"reported {args.stage} successfully")
        return 0
    except ValidationError as exc:
        print(f"validation error: {exc}", file=sys.stderr)
        return 2
    except RuntimeError as exc:
        print(str(exc), file=sys.stderr)
        return 3


if __name__ == "__main__":
    raise SystemExit(main())
