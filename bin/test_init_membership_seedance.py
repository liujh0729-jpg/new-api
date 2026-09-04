from __future__ import annotations

import copy
import contextlib
import importlib.util
import io
import json
import os
import re
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch


SCRIPT_PATH = Path(__file__).with_name("init-membership-seedance.py")
CONFIG_PATH = Path(__file__).with_name(
    "membership-seedance-bootstrap.production.json"
)
SPEC = importlib.util.spec_from_file_location("init_membership_seedance", SCRIPT_PATH)
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class FakeNewAPIClient:
    def __init__(self) -> None:
        self.memberships = []
        self.providers = []
        self.base_models = []
        self.enhancement_models = []
        self.offerings = []
        self.config = None
        self.billing_credential_configured = False
        self.credentials = []
        self.next_id = 1
        self.write_count = 0

    def _id(self) -> int:
        value = self.next_id
        self.next_id += 1
        return value

    def list_memberships(self):
        return copy.deepcopy(self.memberships)

    def list_resource(self, name, channel_id=None):
        values = {
            "providers": self.providers,
            "base-models": self.base_models,
            "enhancement-models": self.enhancement_models,
            "offerings": self.offerings,
        }[name]
        if channel_id is not None and name == "offerings":
            values = [item for item in values if item["channel_id"] == channel_id]
        return copy.deepcopy(values)

    def overview(self, channel_id):
        return {
            "config": copy.deepcopy(self.config),
            "configured": self.config is not None,
            "billing_credential_configured": self.billing_credential_configured,
            "site_instance_id": "11111111-1111-4111-8111-111111111111",
            "credentials": copy.deepcopy(self.credentials),
            "providers": copy.deepcopy(self.providers),
            "offerings": copy.deepcopy(self.offerings),
        }

    def _upsert(self, values, payload):
        item = copy.deepcopy(payload)
        item.pop("mediakit_api_key", None)
        item["id"] = int(item.get("id") or self._id())
        for index, existing in enumerate(values):
            if existing["id"] == item["id"]:
                values[index] = item
                return copy.deepcopy(item)
        values.append(item)
        return copy.deepcopy(item)

    def data(self, method, path, payload=None):
        if method in {"POST", "PUT", "DELETE"}:
            self.write_count += 1
        payload = copy.deepcopy(payload or {})
        if path == "/api/membership/admin/levels" and method == "POST":
            return self._upsert(self.memberships, payload)
        if path.startswith("/api/membership/admin/levels/") and method == "PUT":
            item_id = int(path.rsplit("/", 1)[1])
            existing = next(item for item in self.memberships if item["id"] == item_id)
            payload["id"] = item_id
            payload["code"] = existing["code"]
            return self._upsert(self.memberships, payload)
        if path == "/api/seedance-admin/providers":
            existing = next(
                (item for item in self.providers if item["id"] == payload.get("id")),
                None,
            )
            configured = bool(payload.get("mediakit_api_key")) or bool(
                existing and existing.get("credential_configured")
            )
            saved = self._upsert(self.providers, payload)
            saved["credential_configured"] = configured
            return self._upsert(self.providers, saved)
        if path.endswith("/config") and method == "PUT":
            previous_verified = int((self.config or {}).get("last_verified_at", 0))
            if payload.get("aipdd_billing_api_key"):
                self.billing_credential_configured = True
            self.config = {
                key: value
                for key, value in payload.items()
                if key != "aipdd_billing_api_key"
            }
            self.config["volcengine_bill_product_codes"] = json.dumps(
                self.config.pop("volcengine_bill_product_codes")
            )
            self.config["volcengine_bill_configuration_codes"] = json.dumps(
                self.config.pop("volcengine_bill_configuration_codes")
            )
            self.config["last_verified_at"] = previous_verified
            return copy.deepcopy(self.config)
        if path.endswith("/credentials") and method == "POST":
            credential = {
                "id": self._id(),
                "channel_id": 1,
                "status": "PENDING",
            }
            self.credentials.append(credential)
            return copy.deepcopy(credential)
        if "/credentials/" in path and path.endswith("/validate"):
            credential_id = int(path.split("/")[-2])
            for item in self.credentials:
                item["status"] = "ACTIVE" if item["id"] == credential_id else item["status"]
            self.config["last_verified_at"] = 1
            return {"validated": True, "credential_id": credential_id}
        if path == "/api/seedance-admin/base-models":
            return self._upsert(self.base_models, payload)
        if path.startswith("/api/seedance-admin/base-models/") and method == "DELETE":
            item_id = int(path.rsplit("/", 1)[1])
            self.base_models = [
                item for item in self.base_models if item["id"] != item_id
            ]
            return {"id": item_id, "archived": True}
        if path == "/api/seedance-admin/enhancement-models":
            return self._upsert(self.enhancement_models, payload)
        if path.startswith("/api/seedance-admin/enhancement-models/") and method == "DELETE":
            item_id = int(path.rsplit("/", 1)[1])
            self.enhancement_models = [
                item for item in self.enhancement_models if item["id"] != item_id
            ]
            return {"id": item_id, "archived": True}
        if path == "/api/seedance-admin/offerings":
            payload["archived_at"] = 0
            return self._upsert(self.offerings, payload)
        raise AssertionError(f"unexpected fake request: {method} {path}")


class BootstrapConfigTests(unittest.TestCase):
    def setUp(self) -> None:
        self.config = json.loads(CONFIG_PATH.read_text(encoding="utf-8"))

    def test_production_snapshot_is_valid_and_complete(self) -> None:
        MODULE.validate_config(self.config)
        self.assertEqual(6, len(self.config["membership_levels"]))
        self.assertEqual(4, len(self.config["base_models"]))
        self.assertEqual(4, len(self.config["enhancement_models"]))
        self.assertEqual(13, len(self.config["offerings"]))

    def test_vip_contract_is_frozen(self) -> None:
        actual = {
            item["code"]: item["multiplier_ppm"]
            for item in self.config["membership_levels"]
        }
        self.assertEqual(
            {
                "VIP-T1": 730000,
                "VIP1": 780000,
                "VIP2": 800000,
                "VIP3": 850000,
                "VIP4": 900000,
                "VIP5": 950000,
            },
            actual,
        )

    def test_base_models_use_stable_official_ids_and_readable_names(self) -> None:
        actual = {
            item["code"]: (item["display_name"], item["provider_model_id"])
            for item in self.config["base_models"]
        }
        self.assertEqual(
            {
                "seedance-2.0-vip-prod": (
                    "seedance-2-0",
                    "doubao-seedance-2-0",
                ),
                "seedance-2.0-standard-prod": (
                    "seedance-2-0-fast",
                    "doubao-seedance-2-0-fast",
                ),
                "seedance-2.0-light-prod": (
                    "seedance-2-0-mini",
                    "doubao-seedance-2-0-mini",
                ),
                "seedance-2.5-standard-prod": (
                    "seedance-2-5",
                    "doubao-seedance-2-5",
                ),
            },
            actual,
        )
        self.assertTrue(
            all(
                re.search(r"-\d{6}$", item["provider_model_id"]) is None
                for item in self.config["base_models"]
            )
        )

    def test_cost_split_must_reconcile_to_aipdd_total(self) -> None:
        invalid = copy.deepcopy(self.config)
        invalid["base_models"][0]["cost_matrix"][0][
            "cost_micro_rmb_per_second"
        ] += 1
        with self.assertRaisesRegex(MODULE.BootstrapFailure, "成本拆分不守恒"):
            MODULE.validate_config(invalid)

    def test_lowest_membership_sale_must_stay_above_cost(self) -> None:
        invalid = copy.deepcopy(self.config)
        offering = next(
            item
            for item in invalid["offerings"]
            if item["target_resolution"] != "480p"
        )
        offering["no_reference_unit_price_micro_rmb"] = 1
        with self.assertRaisesRegex(MODULE.BootstrapFailure, "售价低于成本"):
            MODULE.validate_config(invalid)

    def test_ap_seedance_480p_uses_membership_exemption_without_allowing_loss(self) -> None:
        valid = copy.deepcopy(self.config)
        offering = next(
            item
            for item in valid["offerings"]
            if item["display_name"] == "AP Seedance-2.0 VIP 480p"
        )
        offering["no_reference_unit_price_micro_rmb"] = offering[
            "expected_no_reference_total_cost_micro_rmb"
        ]
        offering["reference_unit_price_micro_rmb"] = offering[
            "expected_reference_total_cost_micro_rmb"
        ]
        MODULE.validate_config(valid)

        offering["no_reference_unit_price_micro_rmb"] -= 1
        with self.assertRaisesRegex(MODULE.BootstrapFailure, "480p 会员豁免规则"):
            MODULE.validate_config(valid)

    def test_public_name_cannot_expose_processing_detail(self) -> None:
        invalid = copy.deepcopy(self.config)
        invalid["offerings"][0]["display_name"] = "Seedance 超分 720p"
        with self.assertRaisesRegex(MODULE.BootstrapFailure, "公开模型名"):
            MODULE.validate_config(invalid)

    def test_native_2_5_480p_does_not_require_processing_model(self) -> None:
        offering = next(
            item
            for item in self.config["offerings"]
            if item["display_name"] == "AP Seedance-2.5 标准版 480p"
        )
        self.assertIsNone(offering["enhancement_model_code"])
        MODULE.validate_config(self.config)


class BootstrapPlanningTests(unittest.TestCase):
    def setUp(self) -> None:
        self.config = json.loads(CONFIG_PATH.read_text(encoding="utf-8"))

    def test_json_fields_compare_semantically(self) -> None:
        existing = {"cost_matrix": '[{"b":2,"a":1}]', "enabled": True}
        desired = {"cost_matrix": '[{"a":1,"b":2}]', "enabled": True}
        action = MODULE.classify_action(
            existing,
            desired,
            ("cost_matrix", "enabled"),
            ("cost_matrix",),
        )
        self.assertEqual("noop", action)

    def test_classifier_distinguishes_create_update_and_noop(self) -> None:
        desired = {"display_name": "VIP1", "enabled": True}
        fields = ("display_name", "enabled")
        self.assertEqual("create", MODULE.classify_action(None, desired, fields))
        self.assertEqual(
            "noop", MODULE.classify_action(dict(desired), desired, fields)
        )
        self.assertEqual(
            "update",
            MODULE.classify_action(
                {"display_name": "VIP1", "enabled": False}, desired, fields
            ),
        )

    def test_remote_plain_http_is_rejected(self) -> None:
        with self.assertRaisesRegex(MODULE.BootstrapFailure, "必须使用 HTTPS"):
            MODULE.NewAPIClient("http://newapi.example.com")
        local = MODULE.NewAPIClient("http://127.0.0.1:6070")
        self.assertEqual("http://127.0.0.1:6070", local.base_url)

    def test_pricing_version_is_deterministic_and_changes_with_price(self) -> None:
        offering = self.config["offerings"][0]
        bases = {item["code"]: item for item in self.config["base_models"]}
        enhancements = {
            item["code"]: item for item in self.config["enhancement_models"]
        }
        first = MODULE.pricing_version(
            self.config,
            offering,
            bases[offering["base_model_code"]],
            enhancements[offering["enhancement_model_code"]],
        )
        second = MODULE.pricing_version(
            self.config,
            offering,
            bases[offering["base_model_code"]],
            enhancements[offering["enhancement_model_code"]],
        )
        changed = copy.deepcopy(offering)
        changed["reference_unit_price_micro_rmb"] += 1
        third = MODULE.pricing_version(
            self.config,
            changed,
            bases[offering["base_model_code"]],
            enhancements[offering["enhancement_model_code"]],
        )
        self.assertEqual(first, second)
        self.assertNotEqual(first, third)
        self.assertRegex(first, r"^prod-20260905-[0-9a-f]{12}$")

    def test_pricing_version_changes_when_revision_row_identity_changes(self) -> None:
        offering = self.config["offerings"][0]
        bases = {item["code"]: item for item in self.config["base_models"]}
        enhancements = {
            item["code"]: item for item in self.config["enhancement_models"]
        }
        common_args = (
            self.config,
            offering,
            bases[offering["base_model_code"]],
            enhancements[offering["enhancement_model_code"]],
        )
        first = MODULE.pricing_version(
            *common_args, {"base_model_id": 10, "enhancement_model_id": 20}
        )
        second = MODULE.pricing_version(
            *common_args, {"base_model_id": 11, "enhancement_model_id": 20}
        )
        self.assertNotEqual(first, second)


class BootstrapApplyTests(unittest.TestCase):
    def setUp(self) -> None:
        self.config = json.loads(CONFIG_PATH.read_text(encoding="utf-8"))
        self.args = SimpleNamespace(
            rotate_secrets=False,
            non_interactive=True,
            publish=False,
            instance_id="11111111-1111-4111-8111-111111111111",
            aipdd_billing_base_url="https://aipdd.example.com",
        )
        self.channel = {"id": 1, "type": 59, "name": "Seedance 独立平台"}

    def apply_once(self, client: FakeNewAPIClient) -> None:
        MODULE._save_memberships(client, self.config)
        provider = MODULE._save_provider(client, self.config, self.args)
        MODULE._save_channel_config_and_credentials(
            client, self.args, self.channel, provider
        )
        bases = MODULE._save_base_models(client, self.config)
        enhancements = MODULE._save_enhancement_models(
            client, self.config, provider["id"]
        )
        MODULE._save_offerings(
            client,
            self.config,
            self.args,
            self.channel["id"],
            bases,
            enhancements,
        )
        MODULE._archive_retired_base_models(client, self.config)
        MODULE._archive_retired_enhancement_models(client, self.config)
        MODULE.verify_result(
            client,
            self.config,
            self.args,
            self.channel["id"],
        )

    def test_preflight_rejects_missing_mediakit_key_before_any_write(self) -> None:
        client = FakeNewAPIClient()
        state = {
            "providers": [],
            "overview": client.overview(self.channel["id"]),
        }
        secret_names = {
            "NEW_API_SEEDANCE_MEDIAKIT_API_KEY": "",
            "NEW_API_SEEDANCE_ARK_API_KEY": "",
            "NEW_API_SEEDANCE_AIPDD_BILLING_API_KEY": "",
        }
        with patch.dict(os.environ, secret_names, clear=False):
            with self.assertRaisesRegex(
                MODULE.BootstrapFailure,
                "NEW_API_SEEDANCE_MEDIAKIT_API_KEY",
            ):
                MODULE.preflight_apply_secrets(
                    self.config, self.args, state, self.channel
                )
        self.assertEqual(0, client.write_count)

    def test_empty_instance_converges_and_second_apply_is_noop(self) -> None:
        client = FakeNewAPIClient()
        secrets = {
            "NEW_API_SEEDANCE_MEDIAKIT_API_KEY": "mediakit-test-key",
            "NEW_API_SEEDANCE_ARK_API_KEY": "ark-test-key",
            "NEW_API_SEEDANCE_AIPDD_BILLING_API_KEY": "billing-test-key",
        }
        with patch.dict(os.environ, secrets, clear=False), contextlib.redirect_stdout(
            io.StringIO()
        ):
            self.apply_once(client)
            self.assertEqual(6, len(client.memberships))
            self.assertEqual(1, len(client.providers))
            self.assertEqual(4, len(client.base_models))
            self.assertEqual(4, len(client.enhancement_models))
            self.assertEqual(13, len(client.offerings))
            self.assertTrue(client.billing_credential_configured)
            self.assertTrue(all(not item["enabled"] for item in client.offerings))
            native_480p = next(
                item
                for item in client.offerings
                if item["display_name"] == "AP Seedance-2.5 标准版 480p"
            )
            self.assertIsNone(native_480p["enhancement_model_id"])

            client.write_count = 0
            self.apply_once(client)
            self.assertEqual(0, client.write_count)

    def test_retired_cost_profile_base_models_are_archived(self) -> None:
        client = FakeNewAPIClient()
        client.base_models = [
            {
                "id": 9001,
                "code": "seedance-2.0-value-1080p-prod",
                "display_name": "seedance-2-0-fast（1080p 成本档）",
            },
            {
                "id": 9002,
                "code": "seedance-2.0-value-4k-prod",
                "display_name": "seedance-2-0-fast（4K 成本档）",
            },
        ]

        with contextlib.redirect_stdout(io.StringIO()):
            MODULE._archive_retired_base_models(client, self.config)

        self.assertEqual([], client.base_models)
        self.assertEqual(2, client.write_count)

    def test_retired_cost_profile_enhancement_models_are_archived(self) -> None:
        client = FakeNewAPIClient()
        client.enhancement_models = [
            {
                "id": 9101,
                "code": "mediakit-value-1080p-prod",
                "display_name": "MediaKit 高性价比 480p→1080p",
            },
            {
                "id": 9102,
                "code": "mediakit-value-4k-prod",
                "display_name": "MediaKit 高性价比 480p→4K",
            },
        ]

        with contextlib.redirect_stdout(io.StringIO()):
            MODULE._archive_retired_enhancement_models(client, self.config)

        self.assertEqual([], client.enhancement_models)
        self.assertEqual(2, client.write_count)


if __name__ == "__main__":
    unittest.main()
