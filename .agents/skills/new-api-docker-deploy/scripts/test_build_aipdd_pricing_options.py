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

# rmbPerAwcoin / USDExchangeRate = 0.073 / 7.3 = 0.01 — keeps expected USD
# prices identical to the previous usdPerAwcoin=0.01 fixtures.
RMB_PER_AWCOIN = 0.073
USD_EXCHANGE_RATE = "7.3"
USD_PER_AWCOIN = Decimal("0.01")


def awcoin_rate(*, rmb: float | None = RMB_PER_AWCOIN, usd: float | None = 0.01) -> dict:
    rate: dict = {}
    if rmb is not None:
        rate["rmbPerAwcoin"] = rmb
    if usd is not None:
        rate["usdPerAwcoin"] = usd
    return rate


def site_options(**extra: object) -> dict:
    options: dict = {"USDExchangeRate": USD_EXCHANGE_RATE}
    options.update(extra)
    return options


def resolution(
    name: str,
    retail: float,
    retail_video: float,
    *,
    display: float | None = None,
    display_video: float | None = None,
) -> dict:
    # display* = AIPDD platform/channel settlement; suggestedRetail* = New API sale.
    platform = display if display is not None else retail
    platform_video = display_video if display_video is not None else retail_video
    return {
        "targetResolution": name,
        "displayAmountAwcoinPerSecond": platform,
        "displayVideoInputAwcoinPerSecond": platform_video,
        "suggestedRetailAwcoinPerSecond": retail,
        "suggestedRetailVideoInputAwcoinPerSecond": retail_video,
        "defaultDurationSeconds": 5,
        "defaultFramesPerSecond": 24,
    }


def seedance_capability(by_resolution: dict, name: str = "AP Seedance") -> dict:
    return {
        "id": name,
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
    def test_seedance_25_model_name_variants_accept_auto_duration(self) -> None:
        for model_name in (
            "AP Seedance-2.5 标准版",
            "AP Seedance 2.5 标准版",
            "AP SEEDANCE_2_5 标准版",
        ):
            with self.subTest(model_name=model_name):
                item = resolution("720p", 10, 15)
                item["defaultDurationSeconds"] = -1
                capability = seedance_capability({"720p": item}, model_name)

                pricing = MODULE.resolution_task_pricing(capability, USD_PER_AWCOIN)

                self.assertEqual(0.1, pricing["by_resolution"]["720p"]["no_reference_video_unit_price"])

    def test_auto_duration_remains_rejected_outside_seedance_25(self) -> None:
        for model_name, duration in (
            ("AP Seedance-2.0 标准版", -1),
            ("AP Seedance-2.50 标准版", -1),
            ("AP Seedance-2.5 标准版", -2),
            ("AP Seedance-2.5 标准版", 0),
        ):
            with self.subTest(model_name=model_name, duration=duration):
                item = resolution("720p", 10, 15)
                item["defaultDurationSeconds"] = duration
                capability = seedance_capability({"720p": item}, model_name)

                with self.assertRaisesRegex(ValueError, "positive"):
                    MODULE.resolution_task_pricing(capability, USD_PER_AWCOIN)

    def test_resolution_task_pricing_uses_only_suggested_retail_fields(self) -> None:
        capability = seedance_capability({
            "720p": resolution("720p", 10, 15, display=1, display_video=2),
            "1080p": resolution("1080p", 20, 25, display=3, display_video=4),
        })

        pricing = MODULE.resolution_task_pricing(capability, USD_PER_AWCOIN)

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

    def test_resolution_task_pricing_uses_suggested_retail_and_ignores_display_byok(self) -> None:
        item = resolution("720p", 4620, 12770, display=600, display_video=1670)
        item.update({
            "byokAmountAwcoinPerSecond": 600,
            "byokVideoInputAwcoinPerSecond": 1670,
        })
        capability = seedance_capability({"720p": item})

        pricing = MODULE.resolution_task_pricing(capability, USD_PER_AWCOIN)

        self.assertEqual(
            {
                "no_reference_video_unit_price": 46.2,
                "reference_video_policy": "custom",
                "reference_video_unit_price": 127.7,
            },
            pricing["by_resolution"]["720p"],
        )

    def test_resolution_task_pricing_keeps_480p_at_native_group_ratio(self) -> None:
        capability = seedance_capability({
            "480p": resolution("480p", 8, 12),
            "720p": resolution("720p", 10, 15),
        })

        pricing = MODULE.resolution_task_pricing(capability, USD_PER_AWCOIN)

        self.assertEqual(
            "none",
            pricing["by_resolution"]["480p"]["group_ratio_policy"],
        )
        self.assertNotIn(
            "group_ratio_policy",
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
            MODULE.resolution_task_pricing(capability, USD_PER_AWCOIN)

    def test_resolution_task_pricing_rejects_missing_suggested_retail(self) -> None:
        capability = seedance_capability({
            "720p": {
                "targetResolution": "720p",
                "displayAmountAwcoinPerSecond": 10,
                "displayVideoInputAwcoinPerSecond": 15,
                "defaultDurationSeconds": 5,
                "defaultFramesPerSecond": 24,
            },
        })

        with self.assertRaisesRegex(ValueError, "suggestedRetailAwcoinPerSecond"):
            MODULE.resolution_task_pricing(capability, USD_PER_AWCOIN)

    def test_resolution_keys_are_canonical_and_same_policy_omits_custom_price(self) -> None:
        capability = seedance_capability({
            " 4K ": resolution("4k", 30, 30),
        })

        pricing = MODULE.resolution_task_pricing(capability, USD_PER_AWCOIN)

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
            MODULE.resolution_task_pricing(capability, USD_PER_AWCOIN)

    def test_non_string_target_resolution_is_rejected(self) -> None:
        capability = seedance_capability({
            "720p": resolution(None, 10, 15),
        })

        with self.assertRaisesRegex(ValueError, "resolution key must be a string"):
            MODULE.resolution_task_pricing(capability, USD_PER_AWCOIN)

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
            MODULE.resolution_task_pricing(capability, USD_PER_AWCOIN)

    def test_existing_model_price_is_never_used_as_a_fallback(self) -> None:
        catalog = {
            "revision": "revision-new-contract",
            "awcoinRate": awcoin_rate(),
            "capabilities": [seedance_capability({
                "720p": {
                    "targetResolution": "720p",
                    "defaultDurationSeconds": 5,
                    "defaultFramesPerSecond": 24,
                }
            })],
            "models": [],
        }
        current = site_options(ModelPrice={"AP Seedance": 99})

        with self.assertRaisesRegex(ValueError, "displayAmountAwcoinPerSecond"):
            MODULE.build_updates(catalog, current, {"AP Seedance"})

    def test_plan_reports_strict_new_contract(self) -> None:
        catalog = {
            "revision": "revision-new-contract",
            "awcoinRate": awcoin_rate(),
            "capabilities": [seedance_capability({
                "720p": resolution("720p", 10, 15),
                "1080p": resolution("1080p", 20, 25),
            })],
            "models": [],
        }
        result = MODULE.build_updates(
            catalog,
            site_options(
                ModelPrice={"AP Seedance": 99},
                **{
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
            ),
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
        self.assertIn("suggested retail prices for New API sale", result["summary"]["task_pricing_contract"])
        self.assertIn("AIPDD display settlement fields", result["summary"]["task_pricing_contract"])
        self.assertIn("-1 auto-duration sentinel for Seedance 2.5", result["summary"]["task_pricing_contract"])
        self.assertIn("fixes 480p group ratio at 1", result["summary"]["task_pricing_contract"])
        self.assertIn("rejects legacy catalog pricing", result["summary"]["task_pricing_contract"])
        self.assertIn("no legacy ModelPrice fallback", result["summary"]["task_pricing_contract"])
        self.assertIn("rmbPerAwcoin", result["summary"]["task_pricing_contract"])
        self.assertEqual("rmb_anchored", result["summary"]["price_conversion"])
        self.assertIn("RMB-anchored", result["summary"]["task_pricing_policy"])

    def test_rmb_anchored_conversion_uses_rmb_per_awcoin_over_catalog_usd(self) -> None:
        # Catalog usdPerAwcoin would yield 10 * 0.02 = 0.2; RMB-anchored must
        # use 10 * (0.073 / 7.3) = 0.1 instead.
        catalog = {
            "revision": "revision-rmb-anchor",
            "awcoinRate": awcoin_rate(rmb=0.073, usd=0.02),
            "capabilities": [seedance_capability({
                "720p": resolution("720p", 10, 15),
            })],
            "models": [],
        }
        result = MODULE.build_updates(catalog, site_options(), set())
        updates = {item["key"]: json.loads(item["value"]) for item in result["updates"]}
        tier = updates["billing_setting.task_pricing"]["AP Seedance"]["by_resolution"]["720p"]

        self.assertEqual(0.1, tier["no_reference_video_unit_price"])
        self.assertEqual(0.15, tier["reference_video_unit_price"])
        # Display RMB must equal AWCoin × rmbPerAwcoin.
        self.assertAlmostEqual(
            tier["no_reference_video_unit_price"] * float(USD_EXCHANGE_RATE),
            10 * RMB_PER_AWCOIN,
            places=12,
        )

    def test_missing_rmb_per_awcoin_aborts(self) -> None:
        catalog = {
            "revision": "revision-missing-rmb",
            "awcoinRate": awcoin_rate(rmb=None, usd=0.01),
            "capabilities": [seedance_capability({
                "720p": resolution("720p", 10, 15),
            })],
            "models": [],
        }

        with self.assertRaisesRegex(ValueError, "rmbPerAwcoin"):
            MODULE.build_updates(catalog, site_options(), set())

    def test_missing_usd_exchange_rate_aborts(self) -> None:
        catalog = {
            "revision": "revision-missing-rate",
            "awcoinRate": awcoin_rate(),
            "capabilities": [seedance_capability({
                "720p": resolution("720p", 10, 15),
            })],
            "models": [],
        }

        with self.assertRaisesRegex(ValueError, "USDExchangeRate"):
            MODULE.build_updates(catalog, {}, set())

    def test_option_values_keeps_usd_exchange_rate(self) -> None:
        values = MODULE.option_values([
            {"key": "USDExchangeRate", "value": "7.3"},
            {"key": "ModelPrice", "value": "{}"},
            {"key": "ignored", "value": "x"},
        ])
        self.assertEqual("7.3", values["USDExchangeRate"])
        self.assertEqual("{}", values["ModelPrice"])
        self.assertNotIn("ignored", values)

    def test_per_unit_second_model_uses_flat_task_pricing(self) -> None:
        catalog = {
            "revision": "revision-duration",
            "awcoinRate": awcoin_rate(),
            "capabilities": [duration_capability()],
            "models": [],
        }
        result = MODULE.build_updates(
            catalog,
            site_options(ModelPrice={"aipdd_ltx_2.3": 99}),
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
            "awcoinRate": awcoin_rate(),
            "capabilities": [duration_capability(unit="minute")],
            "models": [],
        }

        with self.assertRaisesRegex(ValueError, "per-unit second"):
            MODULE.build_updates(catalog, site_options(), set())


if __name__ == "__main__":
    unittest.main()
