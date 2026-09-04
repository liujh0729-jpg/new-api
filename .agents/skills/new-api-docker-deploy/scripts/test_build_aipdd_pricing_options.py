#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import json
import unittest
from pathlib import Path


SCRIPT_PATH = Path(__file__).with_name("build_aipdd_pricing_options.py")
SPEC = importlib.util.spec_from_file_location("build_aipdd_pricing_options", SCRIPT_PATH)
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)

RMB_PER_AWCOIN = 0.1
USD_EXCHANGE_RATE = 7.5


def catalog(*, capabilities: list[dict] | None = None, models: list[dict] | None = None) -> dict:
    return {
        "revision": "revision-1",
        "awcoinRate": {"rmbPerAwcoin": RMB_PER_AWCOIN, "usdPerAwcoin": 999},
        "capabilities": capabilities or [],
        "models": models or [],
    }


def site_options(**overrides: object) -> dict:
    values: dict[str, object] = {
        "ModelPrice": {},
        "ModelRatio": {},
        "billing_setting.billing_expr": {},
        "billing_setting.task_pricing": {},
        "billing_setting.billing_mode": {},
        "USDExchangeRate": USD_EXCHANGE_RATE,
    }
    values.update(overrides)
    return values


def per_call_capability(name: str = "aipdd-image", amount: float = 15) -> dict:
    return {
        "id": name,
        "adapterCode": "generic",
        "available": True,
        "pricing": {
            "pricingModel": "per_call",
            "currency": "awcoin",
            "enabled": True,
            "chargeConfig": {"amount": amount},
        },
    }


def per_unit_capability(name: str = "aipdd-ltx", *, unit: str = "second", amount: float = 18) -> dict:
    return {
        "id": name,
        "adapterCode": "generic",
        "available": True,
        "pricing": {
            "pricingModel": "per_unit",
            "currency": "awcoin",
            "enabled": True,
            "chargeConfig": {"unit": unit, "amount": amount},
        },
    }


def token_market_duration_capability(name: str = "aipdd-video", *, available: bool = True) -> dict:
    return {
        "id": name,
        "adapterCode": "token_market_media",
        "available": available,
        "pricing": {
            "pricingModel": "per_second",
            "pricingBasis": "display",
            "currency": "awcoin",
            "enabled": True,
            "byResolution": {
                "720p": {"targetResolution": "720p"},
                "1080p": {"targetResolution": "1080p"},
            },
        },
    }


def seedance_capability(name: str = "AP Seedance-2.5") -> dict:
    # Deliberately incomplete: deployment pricing must ignore independent
    # Seedance data before attempting to validate its old AIPDD price shape.
    return {"id": name, "adapterCode": "seedance", "pricing": None}


class BuildAIPDDPricingOptionsTest(unittest.TestCase):
    def test_seedance_catalog_and_existing_prices_are_ignored(self) -> None:
        seedance_name = "AP Seedance-2.5"
        current = site_options(
            ModelPrice={seedance_name: 12.5, "removed-aipdd": 3},
            ModelRatio={seedance_name: 1.2, "removed-aipdd": 1},
            **{
                "billing_setting.task_pricing": {
                    seedance_name: {"unit": "second", "legacy": True},
                    "removed-aipdd": {"unit": "second"},
                },
                "billing_setting.billing_mode": {
                    seedance_name: "task_pricing",
                    "removed-aipdd": "task_pricing",
                },
            },
        )

        result = MODULE.build_updates(
            catalog(capabilities=[seedance_capability(), per_call_capability()]),
            current,
            {seedance_name, "removed-aipdd"},
        )
        updates = {item["key"]: json.loads(item["value"]) for item in result["updates"]}

        self.assertEqual(12.5, updates["ModelPrice"][seedance_name])
        self.assertEqual(1.2, updates["ModelRatio"][seedance_name])
        self.assertTrue(updates["billing_setting.task_pricing"][seedance_name]["legacy"])
        self.assertEqual("task_pricing", updates["billing_setting.billing_mode"][seedance_name])
        self.assertNotIn("removed-aipdd", updates["ModelPrice"])

    def test_seedance_name_is_ignored_even_without_legacy_adapter_code(self) -> None:
        named_seedance = {
            "id": "legacy-seedance-model",
            "adapterCode": "generic",
            "pricing": {"pricingModel": "unsupported"},
        }
        result = MODULE.build_updates(
            catalog(capabilities=[named_seedance]),
            site_options(ModelPrice={"legacy-seedance-model": 8}),
            {"legacy-seedance-model"},
        )
        updates = {item["key"]: json.loads(item["value"]) for item in result["updates"]}
        self.assertEqual(8, updates["ModelPrice"]["legacy-seedance-model"])

    def test_prices_only_models_in_synchronized_aipdd_channel(self) -> None:
        result = MODULE.build_updates(
            catalog(
                capabilities=[
                    per_call_capability("published-model", 15),
                    per_call_capability("hidden-legacy-model", 30),
                ]
            ),
            site_options(ModelPrice={"hidden-legacy-model": 9}),
            {"hidden-legacy-model"},
            {"published-model"},
        )
        updates = {item["key"]: json.loads(item["value"]) for item in result["updates"]}
        self.assertIn("published-model", updates["ModelPrice"])
        self.assertNotIn("hidden-legacy-model", updates["ModelPrice"])

    def test_per_call_and_llm_prices_are_rmb_anchored(self) -> None:
        result = MODULE.build_updates(
            catalog(
                capabilities=[per_call_capability(amount=15)],
                models=[{
                    "id": "aipdd-chat",
                    "pricing": {"promptPerMillion": 30, "completionPerMillion": 60},
                }],
            ),
            site_options(),
            set(),
        )
        updates = {item["key"]: json.loads(item["value"]) for item in result["updates"]}
        factor = RMB_PER_AWCOIN / USD_EXCHANGE_RATE
        self.assertAlmostEqual(15 * factor, updates["ModelPrice"]["aipdd-image"])
        expression = updates["billing_setting.billing_expr"]["aipdd-chat"]
        self.assertEqual('tier("aipdd", p * 0.4 + c * 0.8)', expression)
        self.assertEqual("tiered_expr", updates["billing_setting.billing_mode"]["aipdd-chat"])
        self.assertEqual("rmb_anchored", result["summary"]["price_conversion"])

    def test_token_market_duration_price_from_builtin_sync_is_preserved(self) -> None:
        name = "aipdd-video"
        existing = {
            "unit": "second",
            "by_resolution": {
                "720p": {"no_reference_video_unit_price": 1, "reference_video_policy": "same"},
                "1080p": {"no_reference_video_unit_price": 2, "reference_video_policy": "custom", "reference_video_unit_price": 3},
            },
        }
        result = MODULE.build_updates(
            catalog(capabilities=[token_market_duration_capability(name)]),
            site_options(**{
                "billing_setting.task_pricing": {name: existing},
                "billing_setting.billing_mode": {name: "task_pricing"},
            }),
            set(),
        )
        updates = {item["key"]: json.loads(item["value"]) for item in result["updates"]}
        self.assertEqual(existing, updates["billing_setting.task_pricing"][name])
        self.assertEqual([name], result["summary"]["builtin_synced_task_pricing_models"])

    def test_token_market_duration_requires_post_sync_options(self) -> None:
        with self.assertRaisesRegex(ValueError, "built-in catalog sync"):
            MODULE.build_updates(
                catalog(capabilities=[token_market_duration_capability()]),
                site_options(),
                set(),
            )

    def test_unavailable_token_market_duration_removes_stale_price(self) -> None:
        name = "aipdd-video"
        result = MODULE.build_updates(
            catalog(capabilities=[token_market_duration_capability(name, available=False)]),
            site_options(**{
                "billing_setting.task_pricing": {name: {"unit": "second"}},
                "billing_setting.billing_mode": {name: "task_pricing"},
            }),
            set(),
        )
        updates = {item["key"]: json.loads(item["value"]) for item in result["updates"]}
        self.assertNotIn(name, updates["billing_setting.task_pricing"])
        self.assertNotIn(name, updates["billing_setting.billing_mode"])
        self.assertEqual([name], result["summary"]["builtin_sync_removed_models"])

    def test_per_unit_second_uses_flat_task_pricing(self) -> None:
        result = MODULE.build_updates(
            catalog(capabilities=[per_unit_capability(amount=18)]),
            site_options(),
            set(),
        )
        updates = {item["key"]: json.loads(item["value"]) for item in result["updates"]}
        price = updates["billing_setting.task_pricing"]["aipdd-ltx"]
        self.assertEqual("second", price["unit"])
        self.assertNotIn("by_resolution", price)
        self.assertAlmostEqual(18 * RMB_PER_AWCOIN / USD_EXCHANGE_RATE, price["no_reference_video_unit_price"])
        self.assertEqual("task_pricing", updates["billing_setting.billing_mode"]["aipdd-ltx"])

    def test_per_unit_non_second_is_rejected(self) -> None:
        with self.assertRaisesRegex(ValueError, "per-unit second"):
            MODULE.build_updates(
                catalog(capabilities=[per_unit_capability(unit="minute")]),
                site_options(),
                set(),
            )

    def test_missing_exchange_rates_abort(self) -> None:
        bad_catalog = catalog(capabilities=[per_call_capability()])
        bad_catalog["awcoinRate"] = {}
        with self.assertRaisesRegex(ValueError, "rmbPerAwcoin"):
            MODULE.build_updates(bad_catalog, site_options(), set())
        with self.assertRaisesRegex(ValueError, "USDExchangeRate"):
            MODULE.build_updates(
                catalog(capabilities=[per_call_capability()]),
                site_options(USDExchangeRate=None),
                set(),
            )

    def test_option_values_keeps_usd_exchange_rate(self) -> None:
        parsed = MODULE.option_values({
            "data": [
                {"key": "USDExchangeRate", "value": 7.5},
                {"key": "unrelated", "value": "ignored"},
            ]
        })
        self.assertEqual(7.5, parsed["USDExchangeRate"])
        self.assertNotIn("unrelated", parsed)


if __name__ == "__main__":
    unittest.main()
