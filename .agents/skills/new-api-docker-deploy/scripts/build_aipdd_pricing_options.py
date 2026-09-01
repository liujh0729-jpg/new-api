#!/usr/bin/env python3
"""Build reversible New API option updates from an authenticated AIPDD catalog.

The script is intentionally offline. It reads catalog and pricing-only exports,
never API keys, SSH credentials, cookies, or the deployment .env file.
"""

from __future__ import annotations

import argparse
import json
import math
import re
from decimal import Decimal, InvalidOperation
from pathlib import Path
from typing import Any


OPTION_KEYS = (
    "ModelPrice",
    "ModelRatio",
    "billing_setting.billing_expr",
    "billing_setting.task_pricing",
    "billing_setting.billing_mode",
)

# Read-only site options required for RMB-anchored USD conversion.
RATE_OPTION_KEYS = ("USDExchangeRate",)


def load_json(path: Path) -> Any:
    with path.open("r", encoding="utf-8") as handle:
        value = json.load(handle)
    if isinstance(value, str):
        value = json.loads(value)
    return value


def catalog_object(value: Any) -> dict[str, Any]:
    if isinstance(value, dict) and isinstance(value.get("payload"), str):
        value = json.loads(value["payload"])
    if not isinstance(value, dict):
        raise ValueError("catalog must be a JSON object")
    if not isinstance(value.get("capabilities"), list) or not isinstance(value.get("models"), list):
        raise ValueError("catalog must contain capabilities and models arrays")
    return value


def option_values(value: Any) -> dict[str, Any]:
    accepted = set(OPTION_KEYS) | set(RATE_OPTION_KEYS)
    if isinstance(value, dict) and "data" in value:
        value = value["data"]
    if isinstance(value, list):
        return {
            str(item.get("key")): item.get("value")
            for item in value
            if isinstance(item, dict) and item.get("key") in accepted
        }
    if isinstance(value, dict):
        return {key: value.get(key) for key in accepted}
    raise ValueError("options must be GET /api/option/ output or an option map")


def managed_model_names(value: Any) -> set[str]:
    if isinstance(value, dict) and "data" in value:
        value = value["data"]
    if isinstance(value, dict) and isinstance(value.get("items"), list):
        value = value["items"]
    if isinstance(value, list) and all(isinstance(item, str) for item in value):
        return {item.strip() for item in value if item.strip()}
    if isinstance(value, dict):
        value = [value]
    if isinstance(value, list):
        names: set[str] = set()
        for channel in value:
            if not isinstance(channel, dict) or int(channel.get("type", 0) or 0) != 58:
                continue
            models = channel.get("models", [])
            if isinstance(models, str):
                models = models.split(",")
            if isinstance(models, list):
                names.update(str(item).strip() for item in models if str(item).strip())
        return names
    raise ValueError("managed models must be a string array or AIPDD channel export")


def parse_map(raw: Any, key: str) -> dict[str, Any]:
    if raw is None or raw == "":
        return {}
    if isinstance(raw, str):
        raw = json.loads(raw)
    if not isinstance(raw, dict):
        raise ValueError(f"{key} must be a JSON object")
    return dict(raw)


def decimal_value(
    value: Any,
    label: str,
    *,
    positive: bool = False,
    allowed_values: tuple[Decimal, ...] = (),
) -> Decimal:
    if isinstance(value, bool) or value is None:
        raise ValueError(f"{label} must be numeric")
    try:
        number = Decimal(str(value))
    except (InvalidOperation, ValueError) as exc:
        raise ValueError(f"{label} must be numeric") from exc
    if not number.is_finite():
        relation = "positive" if positive else "non-negative"
        raise ValueError(f"{label} must be finite and {relation}")
    if number in allowed_values:
        return number
    if number < 0 or (positive and number <= 0):
        relation = "positive" if positive else "non-negative"
        raise ValueError(f"{label} must be finite and {relation}")
    return number


def is_seedance_25_model_name(value: Any) -> bool:
    if not isinstance(value, str):
        return False
    parts = [part for part in re.split(r"[\s._-]+", value.strip().casefold()) if part]
    return any(parts[index : index + 3] == ["seedance", "2", "5"] for index in range(len(parts) - 2))


def json_number(value: Decimal) -> int | float:
    if value == value.to_integral_value():
        return int(value)
    number = float(format(value.normalize(), "f"))
    if not math.isfinite(number):
        raise ValueError("price exceeds the supported numeric range")
    return number


def decimal_text(value: Decimal) -> str:
    text = format(value.normalize(), "f")
    return "0" if text in {"", "-0"} else text


def task_awcoin_price(pricing: dict[str, Any]) -> Decimal:
    charge = pricing.get("chargeConfig")
    if isinstance(charge, dict):
        for key in ("priceAWcoin", "chargeAwcoin", "amountAwcoin", "amount", "awcoin"):
            if key not in charge:
                continue
            amount = decimal_value(charge[key], f"chargeConfig.{key}")
            if amount > 0:
                return amount

    return Decimal(0)


def normalize_resolution(value: Any, capability_id: str) -> str:
    if not isinstance(value, str):
        raise ValueError(f"{capability_id}: resolution key must be a string")
    resolution = value.strip().lower()
    if not resolution:
        raise ValueError(f"{capability_id}: resolution key must not be empty")
    if len(resolution) > 128:
        raise ValueError(f"{capability_id}/{resolution}: resolution key exceeds 128 characters")
    return resolution


def resolution_task_pricing(
    capability: dict[str, Any],
    usd_per_awcoin: Decimal,
) -> dict[str, Any]:
    model_name = str(capability.get("id", "")).strip()
    pricing = capability.get("pricing")
    if not isinstance(pricing, dict) or not isinstance(pricing.get("byResolution"), dict):
        raise ValueError(f"{capability.get('id')}: per-second pricing matrix is missing")
    if (
        str(pricing.get("pricingModel", "")).strip().lower() != "per_second"
        or str(pricing.get("currency", "")).strip().lower() != "awcoin"
        or pricing.get("enabled") is not True
    ):
        raise ValueError(f"{capability.get('id')}: invalid per-second pricing metadata")
    pricing_basis = str(pricing.get("pricingBasis", "")).strip().lower()
    if pricing_basis != "display":
        raise ValueError(
            f"{capability.get('id')}: pricingBasis must be 'display'"
        )
    if not pricing["byResolution"]:
        raise ValueError(f"{capability.get('id')}: per-second pricing matrix is empty")

    by_resolution: dict[str, dict[str, Any]] = {}
    for raw_resolution, item in pricing["byResolution"].items():
        resolution = normalize_resolution(raw_resolution, str(capability.get("id")))
        if resolution in by_resolution:
            raise ValueError(
                f"{capability.get('id')}/{resolution}: duplicate resolution after normalization"
            )
        if not isinstance(item, dict):
            raise ValueError(f"{capability.get('id')}/{resolution}: pricing must be an object")
        target_resolution = normalize_resolution(
            item.get("targetResolution", ""), str(capability.get("id"))
        )
        if target_resolution != resolution:
            raise ValueError(
                f"{capability.get('id')}/{resolution}: targetResolution must match the resolution key"
            )
        decimal_value(
            item.get("defaultDurationSeconds"),
            f"{capability.get('id')}/{resolution}.defaultDurationSeconds",
            positive=True,
            allowed_values=(Decimal("-1"),) if is_seedance_25_model_name(model_name) else (),
        )
        decimal_value(
            item.get("defaultFramesPerSecond"),
            f"{capability.get('id')}/{resolution}.defaultFramesPerSecond",
            positive=True,
        )
        # AIPDD still publishes platform display/settlement prices; New API sale
        # prices must come from suggested retail (对比原生价 / MSRP).
        decimal_value(
            item.get("displayAmountAwcoinPerSecond"),
            f"{capability.get('id')}/{resolution}.displayAmountAwcoinPerSecond",
            positive=True,
        )
        decimal_value(
            item.get("displayVideoInputAwcoinPerSecond"),
            f"{capability.get('id')}/{resolution}.displayVideoInputAwcoinPerSecond",
            positive=True,
        )
        no_reference_rate = decimal_value(
            item.get("suggestedRetailAwcoinPerSecond"),
            f"{capability.get('id')}/{resolution}.suggestedRetailAwcoinPerSecond",
            positive=True,
        )
        video_rate = decimal_value(
            item.get("suggestedRetailVideoInputAwcoinPerSecond"),
            f"{capability.get('id')}/{resolution}.suggestedRetailVideoInputAwcoinPerSecond",
            positive=True,
        )
        no_reference_price = no_reference_rate * usd_per_awcoin
        reference_price = video_rate * usd_per_awcoin
        policy = "same" if reference_price == no_reference_price else "custom"
        tier = {
            "no_reference_video_unit_price": json_number(no_reference_price),
            "reference_video_policy": policy,
        }
        if policy == "custom":
            tier["reference_video_unit_price"] = json_number(reference_price)
        if resolution == "480p":
            tier["group_ratio_policy"] = "none"
        by_resolution[resolution] = tier

    return {
        "unit": "second",
        "by_resolution": by_resolution,
    }


def duration_task_pricing(
    capability: dict[str, Any],
    usd_per_awcoin: Decimal,
) -> dict[str, Any]:
    pricing = capability.get("pricing")
    model_name = str(capability.get("id", "")).strip()
    if not isinstance(pricing, dict):
        raise ValueError(f"{model_name}: pricing is missing")
    charge = pricing.get("chargeConfig")
    if (
        str(pricing.get("pricingModel", "")).strip().lower() != "per_unit"
        or str(pricing.get("currency", "")).strip().lower() != "awcoin"
        or pricing.get("enabled") is not True
        or not isinstance(charge, dict)
        or str(charge.get("unit", "")).strip().lower() != "second"
    ):
        raise ValueError(f"{model_name}: invalid per-unit second pricing metadata")
    amount = task_awcoin_price(pricing)
    if amount <= 0:
        raise ValueError(f"{model_name}: no positive per-second catalog price")
    return {
        "unit": "second",
        "no_reference_video_unit_price": json_number(amount * usd_per_awcoin),
        "reference_video_policy": "same",
    }


def is_token_market_duration_capability(capability: dict[str, Any]) -> bool:
    pricing = capability.get("pricing")
    return (
        isinstance(pricing, dict)
        and str(capability.get("adapterCode", "")).strip().lower() == "token_market_media"
        and str(pricing.get("pricingModel", "")).strip().lower() == "per_second"
    )


def validate_builtin_token_market_task_pricing(
    capability: dict[str, Any],
    task_pricing: Any,
    billing_mode: Any,
) -> None:
    """Validate the post-sync option shape without rebuilding its display prices."""
    model_name = str(capability.get("id", "")).strip()
    pricing = capability.get("pricing")
    if not isinstance(pricing, dict) or not isinstance(pricing.get("byResolution"), dict):
        raise ValueError(f"{model_name}: Token Market per-second pricing matrix is missing")
    if (
        str(pricing.get("currency", "")).strip().lower() != "awcoin"
        or str(pricing.get("pricingBasis", "")).strip().lower() != "display"
        or pricing.get("enabled") is not True
    ):
        raise ValueError(f"{model_name}: invalid Token Market display pricing metadata")
    if capability.get("available") is False:
        return
    if str(billing_mode).strip().lower() != "task_pricing":
        raise ValueError(
            f"{model_name}: built-in catalog sync did not set billing_mode=task_pricing"
        )
    if not isinstance(task_pricing, dict):
        raise ValueError(f"{model_name}: built-in catalog sync task pricing is missing")
    if str(task_pricing.get("unit", "")).strip().lower() != "second":
        raise ValueError(f"{model_name}: built-in catalog sync task pricing must use seconds")
    by_resolution = task_pricing.get("by_resolution")
    if not isinstance(by_resolution, dict) or not by_resolution:
        raise ValueError(
            f"{model_name}: built-in catalog sync task pricing has no resolution matrix"
        )

    catalog_resolutions = {
        normalize_resolution(raw_resolution, model_name)
        for raw_resolution in pricing["byResolution"]
    }
    option_resolutions = {
        normalize_resolution(raw_resolution, model_name)
        for raw_resolution in by_resolution
    }
    if option_resolutions != catalog_resolutions:
        raise ValueError(
            f"{model_name}: built-in catalog sync resolution matrix does not match the catalog"
        )
    for resolution, tier in by_resolution.items():
        normalized = normalize_resolution(resolution, model_name)
        if not isinstance(tier, dict):
            raise ValueError(
                f"{model_name}/{normalized}: built-in task pricing tier must be an object"
            )
        decimal_value(
            tier.get("no_reference_video_unit_price"),
            f"{model_name}/{normalized}.no_reference_video_unit_price",
            positive=True,
        )
        policy = str(tier.get("reference_video_policy", "")).strip().lower()
        if policy not in {"same", "custom"}:
            raise ValueError(
                f"{model_name}/{normalized}: invalid built-in reference video policy"
            )
        if policy == "custom":
            decimal_value(
                tier.get("reference_video_unit_price"),
                f"{model_name}/{normalized}.reference_video_unit_price",
                positive=True,
            )


def rmb_anchored_usd_per_awcoin(catalog: dict[str, Any], current: dict[str, Any]) -> Decimal:
    """Return the USD/AWCoin factor that makes site display RMB equal AIPDD RMB.

    storedUSD = AWCoin × rmbPerAwcoin ÷ siteUSDExchangeRate
    displayRMB = storedUSD × siteUSDExchangeRate = AWCoin × rmbPerAwcoin
    """
    rate_data = catalog.get("awcoinRate")
    if not isinstance(rate_data, dict):
        raise ValueError("catalog awcoinRate is missing")
    rmb_per_awcoin = decimal_value(
        rate_data.get("rmbPerAwcoin"), "rmbPerAwcoin", positive=True
    )
    usd_exchange_rate = decimal_value(
        current.get("USDExchangeRate"), "USDExchangeRate", positive=True
    )
    return rmb_per_awcoin / usd_exchange_rate


def build_updates(
    catalog: dict[str, Any], current: dict[str, Any], previous_models: set[str]
) -> dict[str, Any]:
    # RMB-anchored conversion: do not use catalog usdPerAwcoin (≈1/6.75), which
    # would diverge from site display when USDExchangeRate is 7.3.
    usd_per_awcoin = rmb_anchored_usd_per_awcoin(catalog, current)

    capabilities = catalog["capabilities"]
    llm_models = catalog["models"]
    current_ids = {
        str(item.get("id", "")).strip()
        for item in capabilities + llm_models
        if isinstance(item, dict) and str(item.get("id", "")).strip()
    }
    if len(current_ids) != len(capabilities) + len(llm_models):
        raise ValueError("catalog contains an empty or duplicate model id")
    managed = previous_models | current_ids

    maps = {key: parse_map(current.get(key), key) for key in OPTION_KEYS}
    builtin_sync_models = {
        str(capability.get("id", "")).strip()
        for capability in capabilities
        if isinstance(capability, dict)
        and is_token_market_duration_capability(capability)
        and capability.get("available") is not False
    }
    for key, values in maps.items():
        removable = managed
        if key in {"billing_setting.task_pricing", "billing_setting.billing_mode"}:
            # These entries were just written atomically by New API's built-in
            # token_market_media display-price sync. Preserve their values.
            removable = managed - builtin_sync_models
        for name in removable:
            values.pop(name, None)

    per_call_models: list[str] = []
    task_models: list[str] = []
    builtin_task_models: list[str] = []
    builtin_removed_models: list[str] = []
    llm_names: list[str] = []

    for capability in capabilities:
        if not isinstance(capability, dict):
            raise ValueError("catalog capability must be an object")
        model_name = str(capability.get("id", "")).strip()
        pricing = capability.get("pricing")
        if not isinstance(pricing, dict):
            raise ValueError(f"{model_name}: pricing is missing")
        pricing_model = str(pricing.get("pricingModel", "")).strip().lower()
        adapter_code = str(capability.get("adapterCode", "")).strip().lower()
        if adapter_code == "seedance":
            task_pricing = resolution_task_pricing(capability, usd_per_awcoin)
            maps["billing_setting.task_pricing"][model_name] = task_pricing
            maps["billing_setting.billing_mode"][model_name] = "task_pricing"
            task_models.append(model_name)
            continue
        if is_token_market_duration_capability(capability):
            if capability.get("available") is False:
                builtin_removed_models.append(model_name)
                continue
            validate_builtin_token_market_task_pricing(
                capability,
                maps["billing_setting.task_pricing"].get(model_name),
                maps["billing_setting.billing_mode"].get(model_name),
            )
            builtin_task_models.append(model_name)
            continue
        if pricing_model == "per_second":
            raise ValueError(
                f"{model_name}: per-second pricing is supported only for Seedance "
                "or token_market_media"
            )
        if pricing_model == "per_unit":
            task_pricing = duration_task_pricing(capability, usd_per_awcoin)
            maps["billing_setting.task_pricing"][model_name] = task_pricing
            maps["billing_setting.billing_mode"][model_name] = "task_pricing"
            task_models.append(model_name)
            continue

        amount = task_awcoin_price(pricing)
        if amount <= 0:
            raise ValueError(f"{model_name}: no positive per-call catalog price")
        maps["ModelPrice"][model_name] = json_number(amount * usd_per_awcoin)
        per_call_models.append(model_name)

    for item in llm_models:
        if not isinstance(item, dict):
            raise ValueError("catalog model must be an object")
        model_name = str(item.get("id", "")).strip()
        pricing = item.get("pricing")
        if not isinstance(pricing, dict):
            raise ValueError(f"{model_name}: pricing is missing")
        prompt = decimal_value(pricing.get("promptPerMillion"), f"{model_name}.promptPerMillion")
        completion = decimal_value(
            pricing.get("completionPerMillion"), f"{model_name}.completionPerMillion"
        )
        prompt_usd = prompt * usd_per_awcoin
        completion_usd = completion * usd_per_awcoin
        maps["billing_setting.billing_expr"][model_name] = (
            f'tier("aipdd", p * {decimal_text(prompt_usd)} + c * {decimal_text(completion_usd)})'
        )
        maps["billing_setting.billing_mode"][model_name] = "tiered_expr"
        llm_names.append(model_name)

    compact = {
        key: json.dumps(maps[key], ensure_ascii=False, separators=(",", ":"), sort_keys=True)
        for key in OPTION_KEYS
    }
    previous = {
        key: json.dumps(
            parse_map(current.get(key), key),
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        )
        for key in OPTION_KEYS
    }
    update_order = (
        "billing_setting.task_pricing",
        "billing_setting.billing_expr",
        "ModelPrice",
        "ModelRatio",
        "billing_setting.billing_mode",
    )
    rollback_order = (
        "billing_setting.billing_mode",
        "ModelRatio",
        "ModelPrice",
        "billing_setting.billing_expr",
        "billing_setting.task_pricing",
    )
    return {
        "catalog_revision": str(catalog.get("revision", "")),
        "updates": [{"key": key, "value": compact[key]} for key in update_order],
        "rollback": [{"key": key, "value": previous[key]} for key in rollback_order],
        "summary": {
            "managed_models": len(current_ids),
            "per_call_models": sorted(per_call_models),
            "task_pricing_models": sorted(task_models + builtin_task_models),
            "manual_task_pricing_models": sorted(task_models),
            "builtin_synced_task_pricing_models": sorted(builtin_task_models),
            "builtin_sync_removed_models": sorted(builtin_removed_models),
            "tiered_expr_models": sorted(llm_names),
            "task_pricing_contract": "Only Seedance by_resolution matrix pricing requires suggested retail prices for New API sale; Seedance still requires AIPDD display settlement fields, accepts only the -1 auto-duration sentinel for Seedance 2.5 model names, fixes 480p group ratio at 1, and rejects legacy catalog pricing. Eligible token_market_media/per_second models do not require suggestedRetail fields: their display-price task_pricing and billing_mode must already exist from New API built-in catalog sync and are preserved without changing their values. per_unit/second tasks use flat USD/second task pricing; manually rebuilt AIPDD prices convert as AWCoin × rmbPerAwcoin ÷ site USDExchangeRate; no legacy ModelPrice fallback",
            "task_pricing_policy": "RMB-anchored manual pricing: Seedance uses per-resolution suggested retail with group_ratio_policy=none for 480p, per-unit duration uses chargeConfig, and catalog usdPerAwcoin is unused. Token Market per-second models remain owned by built-in display-price sync and are validated/preserved instead of rebuilt",
            "price_conversion": "rmb_anchored",
        },
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--catalog", type=Path, required=True)
    parser.add_argument("--options", type=Path, required=True)
    parser.add_argument("--managed-models", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()

    result = build_updates(
        catalog_object(load_json(args.catalog)),
        option_values(load_json(args.options)),
        managed_model_names(load_json(args.managed_models)),
    )
    args.output.parent.mkdir(parents=True, exist_ok=True)
    with args.output.open("w", encoding="utf-8", newline="\n") as handle:
        json.dump(result, handle, ensure_ascii=False, indent=2)
        handle.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
