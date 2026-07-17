from __future__ import annotations

import importlib.util
import json
import unittest
from decimal import Decimal
from pathlib import Path


SCRIPT_PATH = Path(__file__).with_name("build_aipdd_pricing_options.py")
SPEC = importlib.util.spec_from_file_location("build_aipdd_pricing_options", SCRIPT_PATH)
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def resolution(name: str, amount: float, video: float) -> dict:
    return {
        "targetResolution": name,
        "displayAmountAwcoinPerSecond": amount,
        "displayVideoInputAwcoinPerSecond": video,
        "defaultDurationSeconds": 5,
        "defaultFramesPerSecond": 24,
    }


def seedance_capability(by_resolution: dict) -> dict:
    return {
        "id": "AP Seedance",
        "adapterCode": "seedance",
        "pricing": {
            "pricingModel": "per_second",
            "currency": "awcoin",
            "pricingBasis": "display",
            "enabled": True,
            "byResolution": by_resolution,
        },
    }


def duration_capability(name: str = "aipdd_ltx_2.3", *, unit: str = "second", amount: float = 1800) -> dict:
    return {
        "id": name,
        "adapterCode": "comfyui",
        "pricing": {
            "pricingModel": "per_unit",
            "currency": "awcoin",
            "enabled": True,
            "chargeConfig": {"unit": unit, "amount": amount},
        },
    }


class BuildAIPDDPricingOptionsTest(unittest.TestCase):
    def test_resolution_task_pricing_uses_only_display_fields(self) -> None:
        capability = seedance_capability({
            "720p": resolution("720p", 10, 15),
            "1080p": resolution("1080p", 20, 25),
        })

        pricing = MODULE.resolution_task_pricing(capability, Decimal("0.01"))

        self.assertEqual(
            {
                "unit": "second",
                "by_resolution": {
                    "720p": {
                        "no_reference_video_unit_price": 0.1,
                        "reference_video_policy": "custom",
                        "reference_video_unit_price": 0.15,
                    },
                    "1080p": {
                        "no_reference_video_unit_price": 0.2,
                        "reference_video_policy": "custom",
                        "reference_video_unit_price": 0.25,
                    },
                },
            },
            pricing,
        )

    def test_resolution_task_pricing_uses_display_fields_and_ignores_byok_fields(self) -> None:
        item = resolution("720p", 600, 1670)
        item.update({
            "displayAmountAwcoinPerSecond": 4620,
            "byokAmountAwcoinPerSecond": 600,
            "displayVideoInputAwcoinPerSecond": 12770,
            "byokVideoInputAwcoinPerSecond": 1670,
        })
        capability = seedance_capability({"720p": item})

        pricing = MODULE.resolution_task_pricing(capability, Decimal("0.01"))

        self.assertEqual(
            {
                "no_reference_video_unit_price": 46.2,
                "reference_video_policy": "custom",
                "reference_video_unit_price": 127.7,
            },
            pricing["by_resolution"]["720p"],
        )

    def test_resolution_task_pricing_rejects_legacy_catalog_fields(self) -> None:
        capability = seedance_capability({
            "720p": {
                "targetResolution": "720p",
                "amountAwcoinPerSecond": 10,
                "videoInputAwcoinPerSecond": 15,
                "defaultDurationSeconds": 5,
                "defaultFramesPerSecond": 24,
            },
        })

        with self.assertRaisesRegex(ValueError, "displayAmountAwcoinPerSecond"):
            MODULE.resolution_task_pricing(capability, Decimal("0.01"))

    def test_resolution_keys_are_canonical_and_same_policy_omits_custom_price(self) -> None:
        capability = seedance_capability({
            " 4K ": resolution("4k", 30, 30),
        })

        pricing = MODULE.resolution_task_pricing(capability, Decimal("0.01"))

        self.assertEqual(
            {
                "no_reference_video_unit_price": 0.3,
                "reference_video_policy": "same",
            },
            pricing["by_resolution"]["4k"],
        )

    def test_duplicate_resolution_after_normalization_is_rejected(self) -> None:
        capability = seedance_capability({
            "4K": resolution("4k", 30, 30),
            "4k ": resolution("4k", 30, 30),
        })

        with self.assertRaisesRegex(ValueError, "duplicate resolution"):
            MODULE.resolution_task_pricing(capability, Decimal("0.01"))

    def test_non_string_target_resolution_is_rejected(self) -> None:
        capability = seedance_capability({
            "720p": resolution(None, 10, 15),
        })

        with self.assertRaisesRegex(ValueError, "resolution key must be a string"):
            MODULE.resolution_task_pricing(capability, Decimal("0.01"))

    def test_legacy_price_variants_are_rejected(self) -> None:
        capability = seedance_capability({
            "720p": {
                "targetResolution": "720p",
                "defaultDurationSeconds": 5,
                "defaultFramesPerSecond": 24,
                "priceVariants": [
                    {"hasReferenceVideo": False, "amountAwcoinPerSecond": 10},
                    {"hasReferenceVideo": True, "amountAwcoinPerSecond": 15},
                ],
            }
        })

        with self.assertRaisesRegex(ValueError, "displayAmountAwcoinPerSecond"):
            MODULE.resolution_task_pricing(capability, Decimal("0.01"))

    def test_existing_model_price_is_never_used_as_a_fallback(self) -> None:
        catalog = {
            "revision": "revision-new-contract",
            "awcoinRate": {"usdPerAwcoin": 0.01},
            "capabilities": [seedance_capability({
                "720p": {
                    "targetResolution": "720p",
                    "defaultDurationSeconds": 5,
                    "defaultFramesPerSecond": 24,
                }
            })],
            "models": [],
        }
        current = {"ModelPrice": {"AP Seedance": 99}}

        with self.assertRaisesRegex(ValueError, "displayAmountAwcoinPerSecond"):
            MODULE.build_updates(catalog, current, {"AP Seedance"})

    def test_plan_reports_strict_new_contract(self) -> None:
        catalog = {
            "revision": "revision-new-contract",
            "awcoinRate": {"usdPerAwcoin": 0.01},
            "capabilities": [seedance_capability({
                "720p": resolution("720p", 10, 15),
                "1080p": resolution("1080p", 20, 25),
            })],
            "models": [],
        }
        result = MODULE.build_updates(
            catalog,
            {
                "ModelPrice": {"AP Seedance": 99},
                "billing_setting.task_pricing": {
                    "AP Seedance": {
                        "unit": "second",
                        "no_reference_video_unit_price": 99,
                        "reference_video_policy": "same",
                    },
                    "unrelated-task": {
                        "unit": "second",
                        "no_reference_video_unit_price": 1,
                        "reference_video_policy": "same",
                    },
                },
            },
            {"AP Seedance"},
        )
        updates = {item["key"]: json.loads(item["value"]) for item in result["updates"]}

        self.assertNotIn("AP Seedance", updates["ModelPrice"])
        self.assertEqual(
            {
                "unit": "second",
                "by_resolution": {
                    "720p": {
                        "no_reference_video_unit_price": 0.1,
                        "reference_video_policy": "custom",
                        "reference_video_unit_price": 0.15,
                    },
                    "1080p": {
                        "no_reference_video_unit_price": 0.2,
                        "reference_video_policy": "custom",
                        "reference_video_unit_price": 0.25,
                    },
                },
            },
            updates["billing_setting.task_pricing"]["AP Seedance"],
        )
        self.assertEqual(
            {
                "unit": "second",
                "no_reference_video_unit_price": 1,
                "reference_video_policy": "same",
            },
            updates["billing_setting.task_pricing"]["unrelated-task"],
        )
        self.assertNotIn(
            "no_reference_video_unit_price",
            updates["billing_setting.task_pricing"]["AP Seedance"],
        )
        self.assertIn("by_resolution matrix", result["summary"]["task_pricing_contract"])
        self.assertIn("requires explicit display prices", result["summary"]["task_pricing_contract"])
        self.assertIn("rejects legacy catalog pricing", result["summary"]["task_pricing_contract"])
        self.assertIn("no legacy ModelPrice fallback", result["summary"]["task_pricing_contract"])

    def test_per_unit_second_model_uses_flat_task_pricing(self) -> None:
        catalog = {
            "revision": "revision-duration",
            "awcoinRate": {"usdPerAwcoin": 0.01},
            "capabilities": [duration_capability()],
            "models": [],
        }
        result = MODULE.build_updates(
            catalog,
            {"ModelPrice": {"aipdd_ltx_2.3": 99}},
            {"aipdd_ltx_2.3"},
        )
        updates = {item["key"]: json.loads(item["value"]) for item in result["updates"]}

        self.assertNotIn("aipdd_ltx_2.3", updates["ModelPrice"])
        self.assertEqual(
            {
                "unit": "second",
                "no_reference_video_unit_price": 18,
                "reference_video_policy": "same",
            },
            updates["billing_setting.task_pricing"]["aipdd_ltx_2.3"],
        )
        self.assertEqual(
            "task_pricing",
            updates["billing_setting.billing_mode"]["aipdd_ltx_2.3"],
        )

    def test_per_unit_non_second_model_is_rejected(self) -> None:
        catalog = {
            "revision": "revision-invalid-duration",
            "awcoinRate": {"usdPerAwcoin": 0.01},
            "capabilities": [duration_capability(unit="minute")],
            "models": [],
        }

        with self.assertRaisesRegex(ValueError, "per-unit second"):
            MODULE.build_updates(catalog, {}, set())


if __name__ == "__main__":
    unittest.main()
