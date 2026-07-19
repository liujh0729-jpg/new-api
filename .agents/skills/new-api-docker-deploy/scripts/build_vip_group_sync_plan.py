#!/usr/bin/env python3
"""Build an idempotent VIP group and AIPDD channel-group sync plan.

This helper never reads or writes model pricing. It merges the fixed VIP1-VIP5
ratios into GroupRatio, keeps those VIP groups out of the global user-selectable
map, links each same-name user group to its private price group, and appends the
five groups to every supplied AIPDD channel while preserving the channel's
existing group order.
"""

from __future__ import annotations

import argparse
import json
import math
from collections import OrderedDict
from pathlib import Path
from typing import Any


VIP_GROUPS = OrderedDict(
    (
        ("VIP1", 0.78),
        ("VIP2", 0.80),
        ("VIP3", 0.85),
        ("VIP4", 0.90),
        ("VIP5", 0.95),
    )
)
SPECIAL_USABLE_GROUP_OPTION = "group_ratio_setting.group_special_usable_group"
VIP_PRIVATE_RULE_KEY = "-:default"
OPTION_KEYS = (
    "GroupRatio",
    "UserUsableGroups",
    SPECIAL_USABLE_GROUP_OPTION,
)
CHANNEL_TYPE_AIPDD = 58
MAX_CHANNEL_GROUP_LENGTH = 64


class PlanError(ValueError):
    pass


def load_json(path: Path) -> Any:
    with path.open("r", encoding="utf-8") as handle:
        return json.load(handle)


def canonical_json(value: dict[str, Any]) -> str:
    return json.dumps(
        value,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )


def option_values(response: Any) -> dict[str, Any]:
    value = (
        response.get("data")
        if isinstance(response, dict) and "data" in response
        else response
    )
    if isinstance(value, list):
        return {
            str(item.get("key")): item.get("value")
            for item in value
            if isinstance(item, dict) and item.get("key") in OPTION_KEYS
        }
    if isinstance(value, dict):
        return {key: value.get(key) for key in OPTION_KEYS}
    raise PlanError("option response must contain a list or object")


def parse_option_map(value: Any, key: str) -> dict[str, Any]:
    if value is None or value == "":
        return {}
    if isinstance(value, str):
        try:
            value = json.loads(value)
        except json.JSONDecodeError as exc:
            raise PlanError(f"{key} is not valid JSON: {exc}") from exc
    if not isinstance(value, dict):
        raise PlanError(f"{key} must be a JSON object")
    return dict(value)


def rollback_value(value: Any, key: str) -> str:
    if value is None or value == "":
        return "{}"
    if isinstance(value, str):
        parse_option_map(value, key)
        return value
    return canonical_json(parse_option_map(value, key))


def validate_group_ratios(values: dict[str, Any]) -> None:
    for name, ratio in values.items():
        if (
            isinstance(ratio, bool)
            or not isinstance(ratio, (int, float))
            or not math.isfinite(float(ratio))
            or float(ratio) < 0
        ):
            raise PlanError(f"GroupRatio[{name!r}] must be finite and non-negative")


def validate_usable_groups(values: dict[str, Any]) -> None:
    for name, description in values.items():
        if not isinstance(name, str) or not name.strip():
            raise PlanError("UserUsableGroups contains an empty group name")
        if not isinstance(description, str):
            raise PlanError(
                f"UserUsableGroups[{name!r}] description must be a string"
            )


def validate_special_usable_groups(values: dict[str, Any]) -> None:
    for user_group, rules in values.items():
        if not isinstance(user_group, str) or not user_group.strip():
            raise PlanError(
                f"{SPECIAL_USABLE_GROUP_OPTION} contains an empty user group"
            )
        if not isinstance(rules, dict):
            raise PlanError(
                f"{SPECIAL_USABLE_GROUP_OPTION}[{user_group!r}] must be an object"
            )
        for rule, description in rules.items():
            if not isinstance(rule, str) or not rule.strip():
                raise PlanError(
                    f"{SPECIAL_USABLE_GROUP_OPTION}[{user_group!r}] contains an empty rule"
                )
            if not isinstance(description, str):
                raise PlanError(
                    f"{SPECIAL_USABLE_GROUP_OPTION}[{user_group!r}]"
                    f"[{rule!r}] description must be a string"
                )


def channel_items(response: Any) -> list[dict[str, Any]]:
    value = (
        response.get("data")
        if isinstance(response, dict) and "data" in response
        else response
    )
    if isinstance(value, dict) and isinstance(value.get("items"), list):
        value = value["items"]
    if not isinstance(value, list):
        raise PlanError("channel response must contain an items list")

    result: list[dict[str, Any]] = []
    seen_ids: set[int] = set()
    for item in value:
        if not isinstance(item, dict):
            raise PlanError("channel item must be an object")
        channel_id = item.get("id")
        if isinstance(channel_id, bool) or not isinstance(channel_id, int) or channel_id <= 0:
            raise PlanError("channel id must be a positive integer")
        if channel_id in seen_ids:
            raise PlanError(f"duplicate channel id: {channel_id}")
        seen_ids.add(channel_id)
        if item.get("type") != CHANNEL_TYPE_AIPDD:
            raise PlanError(f"channel {channel_id} is not an AIPDD channel")
        if not isinstance(item.get("group", ""), str):
            raise PlanError(f"channel {channel_id} group must be a string")
        result.append(item)
    return result


def merge_channel_groups(value: str) -> str:
    groups: list[str] = []
    seen: set[str] = set()
    for raw_group in value.strip(",").split(","):
        group = raw_group.strip()
        if not group or group in seen:
            continue
        groups.append(group)
        seen.add(group)
    for group in VIP_GROUPS:
        if group not in seen:
            groups.append(group)
            seen.add(group)
    merged = ",".join(groups)
    if len(merged) > MAX_CHANNEL_GROUP_LENGTH:
        raise PlanError(
            f"merged channel group exceeds {MAX_CHANNEL_GROUP_LENGTH} characters"
        )
    return merged


def build_plan(options_response: Any, channels_response: Any) -> dict[str, Any]:
    options = option_values(options_response)
    channels = channel_items(channels_response)
    if not channels:
        raise PlanError("at least one AIPDD channel is required")
    original_ratios = parse_option_map(options.get("GroupRatio"), "GroupRatio")
    original_usable = parse_option_map(
        options.get("UserUsableGroups"),
        "UserUsableGroups",
    )
    original_special = parse_option_map(
        options.get(SPECIAL_USABLE_GROUP_OPTION),
        SPECIAL_USABLE_GROUP_OPTION,
    )
    validate_group_ratios(original_ratios)
    validate_usable_groups(original_usable)
    validate_special_usable_groups(original_special)

    next_ratios = dict(original_ratios)
    next_ratios.update(VIP_GROUPS)
    next_usable = dict(original_usable)
    for group in VIP_GROUPS:
        next_usable.pop(group, None)
    next_special = {
        user_group: dict(rules)
        for user_group, rules in original_special.items()
    }
    for group in VIP_GROUPS:
        rules = next_special.setdefault(group, {})
        rules.setdefault(VIP_PRIVATE_RULE_KEY, f"仅使用 {group}")

    option_updates: list[dict[str, str]] = []
    option_rollback: list[dict[str, str]] = []
    for key, original, updated in (
        ("GroupRatio", original_ratios, next_ratios),
        ("UserUsableGroups", original_usable, next_usable),
        (SPECIAL_USABLE_GROUP_OPTION, original_special, next_special),
    ):
        if updated == original:
            continue
        previous_value = rollback_value(options.get(key), key)
        option_updates.append(
            {
                "key": key,
                "previous_value": previous_value,
                "value": canonical_json(updated),
            }
        )
        option_rollback.insert(
            0,
            {"key": key, "value": previous_value},
        )

    channel_updates: list[dict[str, Any]] = []
    for channel in channels:
        previous_group = channel.get("group", "")
        next_group = merge_channel_groups(previous_group)
        if next_group == previous_group:
            continue
        channel_updates.append(
            {
                "id": channel["id"],
                "name": str(channel.get("name", "")),
                "type": CHANNEL_TYPE_AIPDD,
                "previous_group": previous_group,
                "group": next_group,
            }
        )

    channel_rollback = [
        {
            "id": item["id"],
            "name": item["name"],
            "type": CHANNEL_TYPE_AIPDD,
            "expected_group": item["group"],
            "group": item["previous_group"],
        }
        for item in reversed(channel_updates)
    ]

    return {
        "option_updates": option_updates,
        "option_rollback": option_rollback,
        "channel_updates": channel_updates,
        "channel_rollback": channel_rollback,
        "summary": {
            "fixed_groups": dict(VIP_GROUPS),
            "private_user_groups": list(VIP_GROUPS),
            "private_rule": VIP_PRIVATE_RULE_KEY,
            "option_updates": [item["key"] for item in option_updates],
            "channel_updates": len(channel_updates),
            "channel_count": len(channels),
            "contract": (
                "merge fixed VIP groups, keep them out of global user-selectable "
                "groups, link same-name user groups with a private default-removal "
                "rule, and append AIPDD channel groups; preserve unrelated groups, "
                "channels, keys, models, users, and model prices"
            ),
        },
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--options", type=Path, required=True)
    parser.add_argument("--channels", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()

    result = build_plan(load_json(args.options), load_json(args.channels))
    args.output.parent.mkdir(parents=True, exist_ok=True)
    with args.output.open("w", encoding="utf-8", newline="\n") as handle:
        json.dump(result, handle, ensure_ascii=False, indent=2)
        handle.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
