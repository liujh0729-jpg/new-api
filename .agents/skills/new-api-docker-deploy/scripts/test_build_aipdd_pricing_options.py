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


def resolution(name: str, amount: float, video: float, *, image: float | None = None) -> dict:
    return {
        "targetResolution": name,
        "amountAwcoinPerSecond": amount,
        "textInputAwcoinPerSecond": amount,
        "imageInputAwcoinPerSecond": amount if image is None else image,
        "audioInputAwcoinPerSecond": amount,
        "videoInputAwcoinPerSecond": video,
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
            "enabled": True,
            "byResolution": by_resolution,
        },
    }


class BuildAIPDDPricingOptionsTest(unittest.TestCase):
    def test_flat_task_pricing_uses_only_new_modality_fields(self) -> None:
        capability = seedance_capability({
            "720p": resolution("720p", 10, 15, image=12),
            "1080p": resolution("1080p", 20, 25),
        })

        pricing = MODULE.flat_task_pricing(capability, Decimal("0.01"))

        self.assertEqual(0.2, pricing["no_reference_video_unit_price"])
        self.assertEqual("custom", pricing["reference_video_policy"])
        self.assertEqual(0.25, pricing["reference_video_unit_price"])

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

        with self.assertRaisesRegex(ValueError, "amountAwcoinPerSecond"):
            MODULE.flat_task_pricing(capability, Decimal("0.01"))

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

        with self.assertRaisesRegex(ValueError, "amountAwcoinPerSecond"):
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
        result = MODULE.build_updates(catalog, {"ModelPrice": {"AP Seedance": 99}}, {"AP Seedance"})
        updates = {item["key"]: json.loads(item["value"]) for item in result["updates"]}

        self.assertNotIn("AP Seedance", updates["ModelPrice"])
        self.assertEqual(
            {
                "unit": "second",
                "no_reference_video_unit_price": 0.2,
                "reference_video_policy": "custom",
                "reference_video_unit_price": 0.25,
            },
            updates["billing_setting.task_pricing"]["AP Seedance"],
        )
        self.assertIn("no priceVariants or legacy ModelPrice fallback", result["summary"]["task_pricing_contract"])


if __name__ == "__main__":
    unittest.main()
