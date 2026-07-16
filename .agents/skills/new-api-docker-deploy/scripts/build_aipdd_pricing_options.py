#!/usr/bin/env python3
"""Build reversible New API option updates from an authenticated AIPDD catalog.

The script is intentionally offline. It reads catalog and pricing-only exports,
never API keys, SSH credentials, cookies, or the deployment .env file.
"""

from __future__ import annotations

import argparse
import json
import math
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
    if isinstance(value, dict) and "data" in value:
        value = value["data"]
    if isinstance(value, list):
        return {
            str(item.get("key")): item.get("value")
            for item in value
            if isinstance(item, dict) and item.get("key") in OPTION_KEYS
        }
    if isinstance(value, dict):
        return {key: value.get(key) for key in OPTION_KEYS}
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


def decimal_value(value: Any, label: str, *, positive: bool = False) -> Decimal:
    if isinstance(value, bool) or value is None:
        raise ValueError(f"{label} must be numeric")
    try:
        number = Decimal(str(value))
    except (InvalidOperation, ValueError) as exc:
        raise ValueError(f"{label} must be numeric") from exc
    if not number.is_finite() or number < 0 or (positive and number <= 0):
        relation = "positive" if positive else "non-negative"
        raise ValueError(f"{label} must be finite and {relation}")
    return number


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


def flat_task_pricing(
    capability: dict[str, Any],
    usd_per_awcoin: Decimal,
) -> dict[str, Any]:
    pricing = capability.get("pricing")
    if not isinstance(pricing, dict) or not isinstance(pricing.get("byResolution"), dict):
        raise ValueError(f"{capability.get('id')}: per-second pricing matrix is missing")
    if (
        str(pricing.get("pricingModel", "")).strip().lower() != "per_second"
        or str(pricing.get("currency", "")).strip().lower() != "awcoin"
        or pricing.get("enabled") is not True
    ):
        raise ValueError(f"{capability.get('id')}: invalid per-second pricing metadata")
    if not pricing["byResolution"]:
        raise ValueError(f"{capability.get('id')}: per-second pricing matrix is empty")

    no_reference_prices: list[Decimal] = []
    reference_prices: list[Decimal] = []
    for resolution, item in pricing["byResolution"].items():
        if not isinstance(item, dict):
            raise ValueError(f"{capability.get('id')}/{resolution}: pricing must be an object")
        target_resolution = str(item.get("targetResolution", "")).strip()
        if not target_resolution or target_resolution.lower() != str(resolution).strip().lower():
            raise ValueError(
                f"{capability.get('id')}/{resolution}: targetResolution must match the resolution key"
            )
        decimal_value(
            item.get("defaultDurationSeconds"),
            f"{capability.get('id')}/{resolution}.defaultDurationSeconds",
            positive=True,
        )
        decimal_value(
            item.get("defaultFramesPerSecond"),
            f"{capability.get('id')}/{resolution}.defaultFramesPerSecond",
            positive=True,
        )
        non_video_rates = [
            decimal_value(
                item.get(field),
                f"{capability.get('id')}/{resolution}.{field}",
                positive=True,
            )
            for field in (
                "amountAwcoinPerSecond",
                "textInputAwcoinPerSecond",
                "imageInputAwcoinPerSecond",
                "audioInputAwcoinPerSecond",
            )
        ]
        video_rate = decimal_value(
            item.get("videoInputAwcoinPerSecond"),
            f"{capability.get('id')}/{resolution}.videoInputAwcoinPerSecond",
            positive=True,
        )
        no_reference_prices.append(max(non_video_rates) * usd_per_awcoin)
        reference_prices.append(video_rate * usd_per_awcoin)

    no_reference_price = max(no_reference_prices)
    reference_price = max(reference_prices)
    policy = "same" if reference_price == no_reference_price else "custom"

    return {
        "unit": "second",
        "no_reference_video_unit_price": json_number(no_reference_price),
        "reference_video_policy": policy,
        "reference_video_unit_price": json_number(reference_price),
    }


def build_updates(
    catalog: dict[str, Any], current: dict[str, Any], previous_models: set[str]
) -> dict[str, Any]:
    rate_data = catalog.get("awcoinRate")
    if not isinstance(rate_data, dict):
        raise ValueError("catalog awcoinRate is missing")
    usd_per_awcoin = decimal_value(rate_data.get("usdPerAwcoin"), "usdPerAwcoin", positive=True)

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
    for values in maps.values():
        for name in managed:
            values.pop(name, None)

    per_call_models: list[str] = []
    task_models: list[str] = []
    llm_names: list[str] = []

    for capability in capabilities:
        if not isinstance(capability, dict):
            raise ValueError("catalog capability must be an object")
        model_name = str(capability.get("id", "")).strip()
        pricing = capability.get("pricing")
        if not isinstance(pricing, dict):
            raise ValueError(f"{model_name}: pricing is missing")
        is_task_pricing = (
            str(capability.get("adapterCode", "")).strip().lower() == "seedance"
            or str(pricing.get("pricingModel", "")).strip().lower() == "per_second"
        )
        if is_task_pricing:
            task_pricing = flat_task_pricing(capability, usd_per_awcoin)
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
            "task_pricing_models": sorted(task_models),
            "tiered_expr_models": sorted(llm_names),
            "task_pricing_contract": "AIPDD modality pricing fields only; no priceVariants or legacy ModelPrice fallback",
            "task_pricing_policy": "maximum non-video modality USD/second and maximum video-input USD/second across catalog resolutions",
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
