from __future__ import annotations

import importlib.util
import io
import json
import os
import tempfile
import unittest
import uuid
from contextlib import redirect_stdout
from pathlib import Path
from unittest import mock


SCRIPT_PATH = Path(__file__).with_name("report_deployment.py")
SPEC = importlib.util.spec_from_file_location("report_deployment", SCRIPT_PATH)
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


INSTANCE_ID = "98ccca72-5132-46aa-80f9-f693dd2a1c14"
DEPLOYMENT_ID = "e599536e-18ac-4577-a620-68bf513fdc91"


def deployment_payload(status: str = "running") -> dict:
    run = {
        "mode": "initial",
        "status": status,
        "triggerSource": "cursor-skill",
        "startedAt": "2026-08-04T01:02:03Z",
        "skillName": "new-api-docker-deploy",
    }
    if status != "running":
        run["finishedAt"] = "2026-08-04T01:03:03Z"
        run["durationMs"] = 60000
    return {
        "schemaVersion": 1,
        "deploymentId": DEPLOYMENT_ID,
        "instance": {
            "instanceId": INSTANCE_ID,
            "instanceLabel": "prod-01",
            "serverIp": "203.0.113.8",
            "sshPassword": "server-secret",
            "deploymentDirectory": "/opt/new-api",
        },
        "run": run,
        "decisions": {
            "aipddChannelOverwrite": False,
            "aipddPriceOverwrite": False,
            "vipGroupSynchronization": False,
        },
    }


class ReportDeploymentTest(unittest.TestCase):
    def test_redaction_is_recursive_and_does_not_mutate_input(self) -> None:
        source = {
            "sshPassword": "ssh-secret",
            "credentials": [
                {"type": "admin_password", "username": "root", "secret": "admin-secret"}
            ],
            "safe": {"value": "visible"},
        }
        result = MODULE.redact(source)
        self.assertEqual("<redacted>", result["sshPassword"])
        self.assertEqual("<redacted>", result["credentials"][0]["secret"])
        self.assertEqual("admin-secret", source["credentials"][0]["secret"])
        self.assertEqual({"value": "visible"}, result["safe"])

    def test_instance_request_has_exact_route_and_body(self) -> None:
        payload = {
            "instanceLabel": "prod-01",
            "serverIp": "203.0.113.8",
            "sshPort": 22,
            "sshUsername": "root",
            "sshPassword": "ssh-secret",
            "domain": "api.example.com",
            "publicUrl": "http://api.example.com:6070",
            "deploymentDirectory": "/opt/new-api",
        }
        method, url, body = MODULE.shape_request(
            "instance", payload, instance_id=INSTANCE_ID
        )
        self.assertEqual("PUT", method)
        self.assertEqual(
            f"https://api.aipdd.work/v1/new-api/instances/{INSTANCE_ID}", url
        )
        self.assertIs(payload, body)
        self.assertNotIn("instanceId", body)

    def test_credentials_validation_rejects_unknown_and_duplicate_types(self) -> None:
        with self.assertRaisesRegex(MODULE.ValidationError, r"unknown field"):
            MODULE.shape_request(
                "credentials",
                {
                    "mode": "initial",
                    "credentials": [
                        {
                            "type": "admin_password",
                            "username": "root",
                            "secret": "secret",
                            "clear": False,
                        }
                    ],
                },
                instance_id=INSTANCE_ID,
            )
        with self.assertRaisesRegex(MODULE.ValidationError, "duplicate"):
            MODULE.shape_request(
                "credentials",
                {
                    "mode": "update",
                    "credentials": [
                        {"type": "redis_password", "secret": "one"},
                        {"type": "redis_password", "secret": "two"},
                    ],
                },
                instance_id=INSTANCE_ID,
            )

    def test_deployment_stages_enforce_status_and_exact_dto_fields(self) -> None:
        payload = deployment_payload()
        _, url, _ = MODULE.shape_request("deployment-start", payload)
        self.assertTrue(url.endswith(f"/v1/new-api/deployments/{DEPLOYMENT_ID}"))

        with self.assertRaisesRegex(MODULE.ValidationError, "terminal"):
            MODULE.shape_request("deployment-finish", payload)

        finished = deployment_payload("succeeded")
        MODULE.shape_request("deployment-finish", finished)
        finished["verification"] = {"applicationHealthy": True, "extra": True}
        with self.assertRaisesRegex(MODULE.ValidationError, r"unknown field"):
            MODULE.shape_request("deployment-finish", finished)

    def test_dry_run_prints_redacted_shaped_request_without_key(self) -> None:
        payload = {
            "mode": "initial",
            "credentials": [
                {
                    "type": "admin_password",
                    "username": "root",
                    "secret": "never-print-this",
                }
            ],
        }
        output = io.StringIO()
        with (
            mock.patch.object(MODULE.sys, "stdin", io.StringIO(json.dumps(payload))),
            mock.patch.dict(os.environ, {}, clear=True),
            redirect_stdout(output),
        ):
            result = MODULE.main(
                [
                    "--stage",
                    "credentials",
                    "--instance-id",
                    INSTANCE_ID,
                    "--dry-run",
                ]
            )
        preview = output.getvalue()
        self.assertEqual(0, result)
        self.assertNotIn("never-print-this", preview)
        parsed = json.loads(preview)
        self.assertEqual("<redacted>", parsed["payload"]["credentials"][0]["secret"])
        self.assertTrue(parsed["url"].endswith(f"/{INSTANCE_ID}/credentials"))

    def test_resolve_instance_id_securely_creates_and_reuses_uuid(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            deployment_dir = Path(temporary) / "deployment"
            created = MODULE.resolve_instance_id(deployment_dir, create_if_missing=True)
            self.assertIsInstance(created, uuid.UUID)
            self.assertEqual(created, MODULE.resolve_instance_id(deployment_dir))
            id_path = deployment_dir / MODULE.INSTANCE_ID_FILE
            self.assertEqual(f"{created}\n", id_path.read_text(encoding="utf-8"))
            if os.name != "nt":
                self.assertEqual(0o600, id_path.stat().st_mode & 0o777)

    def test_invalid_instance_id_returns_validation_exit_code(self) -> None:
        output = io.StringIO()
        with (
            mock.patch.object(
                MODULE.sys,
                "stdin",
                io.StringIO(json.dumps({"instanceLabel": "prod-01"})),
            ),
            mock.patch.object(MODULE.sys, "stderr", output),
        ):
            result = MODULE.main(
                ["--stage", "instance", "--instance-id", "not-a-uuid", "--dry-run"]
            )
        self.assertEqual(2, result)
        self.assertIn("validation error", output.getvalue())


if __name__ == "__main__":
    unittest.main()
