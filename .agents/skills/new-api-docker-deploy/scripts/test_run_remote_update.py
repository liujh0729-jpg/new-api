#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import unittest
from pathlib import Path
from unittest import mock
from urllib import error


SCRIPT_PATH = Path(__file__).with_name("run_remote_update.py")
SPEC = importlib.util.spec_from_file_location("run_remote_update", SCRIPT_PATH)
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)

INSTANCE_ID = "11111111-2222-4333-8444-555555555555"
OTHER_INSTANCE_ID = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"


class RemoteUpdateIdentityTest(unittest.TestCase):
    def test_update_finish_never_invents_instance_identity(self) -> None:
        source = SCRIPT_PATH.read_text(encoding="utf-8")
        self.assertNotIn('"instanceId": instance_id or str(uuid.uuid4())', source)
        self.assertIn("if deployment_id and instance_id:", source)

    def test_update_refuses_to_invent_instance_identity(self) -> None:
        with (
            mock.patch.object(MODULE, "REGISTERED_INSTANCE_ID", ""),
            mock.patch.object(MODULE, "run", return_value=(0, "", "")),
            mock.patch.object(MODULE, "sftp_read", return_value="AIPDD_API_KEY=masked\n"),
            self.assertRaises(SystemExit),
        ):
            MODULE.resolve_instance_id(object())

    def test_update_recovers_missing_file_from_env(self) -> None:
        writes: list[tuple[str, str, int]] = []

        def record_write(_client: object, path: str, data: str, mode: int = 0o600) -> None:
            writes.append((path, data, mode))

        with (
            mock.patch.object(MODULE, "REGISTERED_INSTANCE_ID", ""),
            mock.patch.object(MODULE, "run", return_value=(0, "", "")),
            mock.patch.object(
                MODULE,
                "sftp_read",
                return_value=f"AIPDD_API_KEY=masked\nAIPDD_INSTANCE_ID={INSTANCE_ID}\n",
            ),
            mock.patch.object(MODULE, "sftp_write", side_effect=record_write),
        ):
            resolved = MODULE.resolve_instance_id(object())

        self.assertEqual(INSTANCE_ID, resolved)
        self.assertEqual(
            [(f"{MODULE.DEPLOY_DIR}/.aipdd-instance-id", INSTANCE_ID + "\n", 0o600)],
            writes,
        )

    def test_update_rejects_conflicting_file_and_env_identity(self) -> None:
        with (
            mock.patch.object(MODULE, "REGISTERED_INSTANCE_ID", ""),
            mock.patch.object(MODULE, "run", return_value=(0, INSTANCE_ID + "\n", "")),
            mock.patch.object(
                MODULE,
                "sftp_read",
                return_value=f"AIPDD_INSTANCE_ID={OTHER_INSTANCE_ID}\n",
            ),
            self.assertRaises(SystemExit),
        ):
            MODULE.resolve_instance_id(object())

    def test_env_and_compose_mutation_scripts_are_valid_python(self) -> None:
        commands: list[str] = []

        def capture_run(_client: object, command: str, **_kwargs: object) -> tuple[int, str, str]:
            commands.append(command)
            marker = "ENV_FLAGS_UPDATED" if len(commands) == 1 else "COMPOSE_UPDATED"
            return 0, marker, ""

        with mock.patch.object(MODULE, "run", side_effect=capture_run):
            MODULE.update_env_flags(object(), INSTANCE_ID)

        self.assertEqual(2, len(commands))
        for command in commands:
            prefix = "python3 - <<'PY'\n"
            self.assertIn(prefix, command)
            source = command.split(prefix, 1)[1].rsplit("\nPY", 1)[0]
            compile(source, "<remote-update-snippet>", "exec")
            self.assertIn("AIPDD_INSTANCE_ID", source)

    def test_site_identity_probe_accepts_authenticated_not_found(self) -> None:
        not_found = error.HTTPError(
            "https://api.aipdd.work/probe", 404, "not found", {}, None
        )
        with mock.patch.object(MODULE.request, "urlopen", side_effect=not_found) as call:
            MODULE.probe_aipdd_site_identity("site-key", INSTANCE_ID, "https://api.aipdd.work")

        sent = call.call_args.args[0]
        self.assertEqual("site-key", sent.get_header("X-api-key"))
        self.assertEqual(INSTANCE_ID, sent.get_header("X-aipdd-instance-id"))

    def test_site_identity_probe_rejects_forbidden_key_or_instance(self) -> None:
        forbidden = error.HTTPError(
            "https://api.aipdd.work/probe", 403, "forbidden", {}, None
        )
        with (
            mock.patch.object(MODULE.request, "urlopen", side_effect=forbidden),
            self.assertRaises(SystemExit),
        ):
            MODULE.probe_aipdd_site_identity("site-key", INSTANCE_ID, "https://api.aipdd.work")


if __name__ == "__main__":
    unittest.main()
