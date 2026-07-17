#!/usr/bin/env python3
"""Import Seedance task prices and fixed VIP groups from a retail CSV.

The CSV's "对比原生价" is treated as RMB per the duration declared by
"计费单位" (for example 条/5秒). It is converted to NewAPI's USD/second
task-pricing base. VIP1..VIP5 use fixed global ratios. A resolution whose five
tier cells are all 1 is marked to keep its native price for every group.

The command is a dry run unless --apply is supplied. Authentication uses an
in-memory cookie jar and an interactive password prompt; secrets are not saved.
"""

from __future__ import annotations

import argparse
import csv
import getpass
import http.cookiejar
import io
import json
import math
import os
import re
import sys
import urllib.error
import urllib.parse
import urllib.request
from collections import OrderedDict
from datetime import datetime, timezone
from decimal import Decimal, InvalidOperation, ROUND_HALF_UP
from pathlib import Path
from typing import Any


if os.name == "nt":
    # Keep Chinese summaries readable when launched from Windows PowerShell/cmd.
    for stream in (sys.stdout, sys.stderr):
        if hasattr(stream, "reconfigure"):
            stream.reconfigure(encoding="utf-8", errors="replace")


GROUPS = OrderedDict(
    (
        ("VIP1", Decimal("0.78")),
        ("VIP2", Decimal("0.80")),
        ("VIP3", Decimal("0.85")),
        ("VIP4", Decimal("0.90")),
        ("VIP5", Decimal("0.95")),
    )
)

REQUIRED_COLUMNS = {
    "平台模型",
    "输出规格",
    "能力类型",
    "计费单位",
    "对比原生价",
} | set(GROUPS)

OPTION_KEYS = (
    "billing_setting.task_pricing",
    "billing_setting.billing_mode",
    "GroupRatio",
    "UserUsableGroups",
    "USDExchangeRate",
)

UPDATE_ORDER = (
    "billing_setting.task_pricing",
    "billing_setting.billing_mode",
    "GroupRatio",
    "UserUsableGroups",
)

MAX_RESOLUTION_LENGTH = 128


class ImportFailure(RuntimeError):
    pass


def decimal_value(value: Any, label: str, *, positive: bool = False) -> Decimal:
    if isinstance(value, bool) or value is None:
        raise ImportFailure(f"{label} 必须是数字")
    text = str(value).strip().replace(",", "")
    if text.endswith("%"):
        text = text[:-1].strip()
        divisor = Decimal(100)
    else:
        divisor = Decimal(1)
    try:
        result = Decimal(text) / divisor
    except (InvalidOperation, ValueError, ZeroDivisionError) as exc:
        raise ImportFailure(f"{label} 必须是数字，实际为 {value!r}") from exc
    if not result.is_finite() or result < 0 or (positive and result <= 0):
        relation = "大于 0" if positive else "不小于 0"
        raise ImportFailure(f"{label} 必须有限且{relation}")
    return result


def json_number(value: Decimal) -> int | float:
    rounded = value.quantize(Decimal("0.000000000001"), rounding=ROUND_HALF_UP)
    if rounded == rounded.to_integral_value():
        return int(rounded)
    number = float(format(rounded.normalize(), "f"))
    if not math.isfinite(number):
        raise ImportFailure("价格超出支持的数字范围")
    return number


def normalize_resolution(value: Any, row_number: int) -> str:
    resolution = str(value or "").strip().lower()
    if not resolution:
        raise ImportFailure(f"第 {row_number} 行：输出规格不能为空")
    if len(resolution) > MAX_RESOLUTION_LENGTH:
        raise ImportFailure(
            f"第 {row_number} 行：输出规格超过 {MAX_RESOLUTION_LENGTH} 个字符"
        )
    return resolution


def parse_duration_seconds(value: Any, row_number: int) -> Decimal:
    text = str(value or "").strip()
    matches = re.findall(r"(\d+(?:\.\d+)?)\s*秒", text, flags=re.IGNORECASE)
    if len(matches) != 1:
        raise ImportFailure(
            f"第 {row_number} 行：无法从计费单位 {text!r} 唯一解析秒数"
        )
    return decimal_value(matches[0], f"第 {row_number} 行计费秒数", positive=True)


def parse_video_variant(value: Any, row_number: int) -> bool:
    text = str(value or "").strip()
    if "不含视频" in text:
        return False
    if "输入含视频" in text or "含视频" in text:
        return True
    raise ImportFailure(
        f"第 {row_number} 行：能力类型必须能识别为“输入含视频”或“不含视频”，实际为 {text!r}"
    )


def resolution_sort_key(value: str) -> tuple[int, float | str]:
    match = re.fullmatch(r"(\d+(?:\.\d+)?)([pk])", value.lower())
    if not match:
        return (1, value.lower())
    number = float(match.group(1))
    if match.group(2) == "k":
        number *= 1000
    return (0, number)


def read_csv_rows(path: Path) -> list[dict[str, str]]:
    try:
        content = path.read_bytes()
    except OSError as exc:
        raise ImportFailure(f"无法读取 CSV：{exc}") from exc
    decoded = None
    for encoding in ("utf-8-sig", "gb18030"):
        try:
            decoded = content.decode(encoding)
            break
        except UnicodeDecodeError:
            continue
    if decoded is None:
        raise ImportFailure("CSV 编码无法识别，仅支持 UTF-8 或 GB18030/GBK")

    reader = csv.DictReader(io.StringIO(decoded, newline=""))
    if not reader.fieldnames:
        raise ImportFailure("CSV 缺少表头")
    reader.fieldnames = [str(name or "").strip() for name in reader.fieldnames]
    missing = sorted(REQUIRED_COLUMNS - set(reader.fieldnames))
    if missing:
        raise ImportFailure(f"CSV 缺少列：{', '.join(missing)}")
    rows: list[dict[str, str]] = []
    for row in reader:
        if not any(str(value or "").strip() for value in row.values()):
            continue
        normalized = {
            str(key or "").strip(): str(value or "").strip()
            for key, value in row.items()
        }
        rows.append(normalized)
    return rows


def build_task_pricing(
    rows: list[dict[str, str]], rmb_per_usd: Decimal
) -> tuple[dict[str, Any], dict[str, Any]]:
    records: dict[tuple[str, str], dict[str, Any]] = {}

    for index, row in enumerate(rows, start=2):
        model = row["平台模型"].strip()
        if not model:
            raise ImportFailure(f"第 {index} 行：平台模型不能为空")
        resolution = normalize_resolution(row["输出规格"], index)
        has_reference_video = parse_video_variant(row["能力类型"], index)
        duration = parse_duration_seconds(row["计费单位"], index)
        native_rmb = decimal_value(
            row["对比原生价"], f"第 {index} 行对比原生价", positive=True
        )

        source_ratios: dict[str, Decimal] = {}
        for group_name in GROUPS:
            ratio = decimal_value(row[group_name], f"第 {index} 行{group_name}")
            source_ratios[group_name] = ratio
        keeps_native_price = all(
            value == Decimal(1) for value in source_ratios.values()
        )
        uses_fixed_groups = all(
            source_ratios[group_name] == fixed_ratio
            for group_name, fixed_ratio in GROUPS.items()
        )
        if not keeps_native_price and not uses_fixed_groups:
            expected = ", ".join(
                f"{group_name}={fixed_ratio}"
                for group_name, fixed_ratio in GROUPS.items()
            )
            actual = ", ".join(
                f"{group_name}={source_ratios[group_name]}" for group_name in GROUPS
            )
            raise ImportFailure(
                f"第 {index} 行：VIP 分组倍率必须全部为 1，或严格为 {expected}；"
                f"实际为 {actual}"
            )
        group_ratio_policy = "none" if keeps_native_price else "global"

        key = (model, resolution)
        record = records.setdefault(
            key,
            {
                "duration": duration,
                "group_ratio_policy": group_ratio_policy,
                "prices_rmb": {},
                "rows": [],
            },
        )
        if record["duration"] != duration:
            raise ImportFailure(
                f"{model}/{resolution} 的计费单位秒数不一致："
                f"{record['duration']} 与 {duration}"
            )
        if record["group_ratio_policy"] != group_ratio_policy:
            raise ImportFailure(
                f"{model}/{resolution} 的含视频和不含视频行对分组豁免定义不一致"
            )
        if has_reference_video in record["prices_rmb"]:
            variant = "输入含视频" if has_reference_video else "不含视频"
            raise ImportFailure(f"{model}/{resolution} 存在重复的{variant}行")
        record["prices_rmb"][has_reference_video] = native_rmb
        record["rows"].append(index)

    models: dict[str, dict[str, Any]] = {}
    exempt_resolutions: list[str] = []
    for (model, resolution), record in sorted(
        records.items(), key=lambda item: (item[0][0], resolution_sort_key(item[0][1]))
    ):
        prices = record["prices_rmb"]
        if False not in prices or True not in prices:
            missing = "不含视频" if False not in prices else "输入含视频"
            raise ImportFailure(f"{model}/{resolution} 缺少{missing}价格行")
        duration = record["duration"]
        no_reference = prices[False] / duration / rmb_per_usd
        reference = prices[True] / duration / rmb_per_usd
        tier: dict[str, Any] = {
            "no_reference_video_unit_price": json_number(no_reference),
            "reference_video_policy": "same" if reference == no_reference else "custom",
        }
        if reference != no_reference:
            tier["reference_video_unit_price"] = json_number(reference)
        if record["group_ratio_policy"] == "none":
            tier["group_ratio_policy"] = "none"
            exempt_resolutions.append(f"{model}/{resolution}")
        model_config = models.setdefault(model, {"unit": "second", "by_resolution": {}})
        model_config["by_resolution"][resolution] = tier

    return models, {
        "models": sorted(models),
        "resolution_tiers": len(records),
        "source_rows": len(rows),
        "exempt_resolutions": exempt_resolutions,
    }


def parse_json_map(raw: Any, key: str) -> dict[str, Any]:
    if raw is None or raw == "":
        return {}
    if isinstance(raw, str):
        try:
            raw = json.loads(raw)
        except json.JSONDecodeError as exc:
            raise ImportFailure(f"线上选项 {key} 不是合法 JSON：{exc}") from exc
    if not isinstance(raw, dict):
        raise ImportFailure(f"线上选项 {key} 必须是 JSON 对象")
    return dict(raw)


def option_values(response: Any) -> dict[str, Any]:
    value = response.get("data") if isinstance(response, dict) and "data" in response else response
    if isinstance(value, list):
        return {
            str(item.get("key")): item.get("value")
            for item in value
            if isinstance(item, dict) and item.get("key") in OPTION_KEYS
        }
    if isinstance(value, dict):
        return {key: value.get(key) for key in OPTION_KEYS}
    raise ImportFailure("GET /api/option/ 返回结构无法识别")


def canonical_json(value: dict[str, Any]) -> str:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"), sort_keys=True)


def build_plan(
    options: dict[str, Any], imported_task_pricing: dict[str, Any], summary: dict[str, Any]
) -> dict[str, Any]:
    task_pricing = parse_json_map(
        options.get("billing_setting.task_pricing"), "billing_setting.task_pricing"
    )
    billing_mode = parse_json_map(
        options.get("billing_setting.billing_mode"), "billing_setting.billing_mode"
    )
    group_ratio = parse_json_map(options.get("GroupRatio"), "GroupRatio")
    usable_groups = parse_json_map(options.get("UserUsableGroups"), "UserUsableGroups")

    for model, config in imported_task_pricing.items():
        task_pricing[model] = config
        billing_mode[model] = "task_pricing"
    for group_name, ratio in GROUPS.items():
        group_ratio[group_name] = json_number(ratio)
        usable_groups.setdefault(
            group_name,
            f"{group_name}（{int(ratio * 100)}档）",
        )

    next_values = {
        "billing_setting.task_pricing": canonical_json(task_pricing),
        "billing_setting.billing_mode": canonical_json(billing_mode),
        "GroupRatio": canonical_json(group_ratio),
        "UserUsableGroups": canonical_json(usable_groups),
    }
    previous_values = {
        key: str(options.get(key) or "{}")
        for key in UPDATE_ORDER
    }
    return {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "summary": {
            **summary,
            "groups": {name: json_number(ratio) for name, ratio in GROUPS.items()},
        },
        "updates": [{"key": key, "value": next_values[key]} for key in UPDATE_ORDER],
        "rollback": [
            {"key": key, "value": previous_values[key]}
            for key in reversed(UPDATE_ORDER)
        ],
    }


class NewAPIClient:
    def __init__(self, base_url: str) -> None:
        parsed = urllib.parse.urlparse(base_url.strip())
        if parsed.scheme not in {"http", "https"} or not parsed.netloc:
            raise ImportFailure("--base-url 必须是完整的 http:// 或 https:// 地址")
        self.base_url = base_url.rstrip("/")
        self.cookie_jar = http.cookiejar.CookieJar()
        self.opener = urllib.request.build_opener(
            urllib.request.HTTPCookieProcessor(self.cookie_jar)
        )
        self.user_id: int | None = None

    def request(self, method: str, path: str, payload: Any | None = None) -> Any:
        data = None
        headers = {"Accept": "application/json"}
        if payload is not None:
            data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
            headers["Content-Type"] = "application/json"
        if self.user_id is not None:
            headers["New-Api-User"] = str(self.user_id)
        request = urllib.request.Request(
            f"{self.base_url}{path}", data=data, headers=headers, method=method
        )
        try:
            with self.opener.open(request, timeout=30) as response:
                body = response.read().decode("utf-8")
        except urllib.error.HTTPError as exc:
            body = exc.read().decode("utf-8", errors="replace")
            raise ImportFailure(f"{method} {path} 返回 HTTP {exc.code}: {body[:500]}") from exc
        except urllib.error.URLError as exc:
            raise ImportFailure(f"无法连接 NewAPI：{exc}") from exc
        try:
            result = json.loads(body)
        except json.JSONDecodeError as exc:
            raise ImportFailure(f"{method} {path} 没有返回合法 JSON") from exc
        if isinstance(result, dict) and result.get("success") is False:
            raise ImportFailure(f"{method} {path} 失败：{result.get('message', 'unknown error')}")
        return result

    def login(self, username: str, password: str) -> None:
        result = self.request(
            "POST", "/api/user/login", {"username": username, "password": password}
        )
        data = result.get("data") if isinstance(result, dict) else None
        if not isinstance(data, dict):
            raise ImportFailure("登录响应缺少管理员信息")
        if data.get("require_2fa"):
            raise ImportFailure("管理员启用了 2FA；该脚本不绕过二次验证")
        role = int(data.get("role", 0) or 0)
        if role < 100:
            raise ImportFailure("导入选项需要 root 管理员账号")
        self.user_id = int(data.get("id", 0) or 0)
        if self.user_id <= 0:
            raise ImportFailure("登录响应缺少有效用户 ID")

    def get_options(self) -> dict[str, Any]:
        return option_values(self.request("GET", "/api/option/"))

    def put_option(self, key: str, value: str) -> None:
        self.request("PUT", "/api/option/", {"key": key, "value": value})


def write_private_json(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    flags = os.O_WRONLY | os.O_CREAT | os.O_TRUNC
    descriptor = os.open(path, flags, 0o600)
    with os.fdopen(descriptor, "w", encoding="utf-8", newline="\n") as handle:
        json.dump(value, handle, ensure_ascii=False, indent=2)
        handle.write("\n")
    try:
        os.chmod(path, 0o600)
    except OSError:
        pass


def apply_plan(client: NewAPIClient, plan: dict[str, Any], backup_dir: Path) -> Path:
    timestamp = datetime.now().strftime("%Y%m%d-%H%M%S")
    backup_path = backup_dir / f"seedance-pricing-options-{timestamp}.json"
    write_private_json(
        backup_path,
        {
            "created_at": datetime.now(timezone.utc).isoformat(),
            "rollback": plan["rollback"],
        },
    )

    try:
        for item in plan["updates"]:
            client.put_option(item["key"], item["value"])
        actual = client.get_options()
        for item in plan["updates"]:
            expected = parse_json_map(item["value"], item["key"])
            observed = parse_json_map(actual.get(item["key"]), item["key"])
            if observed != expected:
                raise ImportFailure(f"写入后校验失败：{item['key']} 与计划不一致")
    except Exception as exc:
        rollback_errors: list[str] = []
        for item in plan["rollback"]:
            try:
                client.put_option(item["key"], item["value"])
            except Exception as rollback_exc:  # noqa: BLE001
                rollback_errors.append(f"{item['key']}: {rollback_exc}")
        detail = f"；回滚失败：{' | '.join(rollback_errors)}" if rollback_errors else "；已回滚"
        raise ImportFailure(f"导入失败：{exc}{detail}") from exc
    return backup_path


def print_summary(summary: dict[str, Any], rmb_per_usd: Decimal) -> None:
    print(f"模型数：{len(summary['models'])}")
    print(f"分辨率档位数：{summary['resolution_tiers']}")
    print(f"CSV 数据行：{summary['source_rows']}")
    print(f"人民币/USD：{rmb_per_usd}")
    print("固定分组：" + ", ".join(f"{name}={ratio}" for name, ratio in GROUPS.items()))
    if summary["exempt_resolutions"]:
        print("保持原价：" + ", ".join(summary["exempt_resolutions"]))


def main() -> int:
    parser = argparse.ArgumentParser(
        description="从 Seedance 零售价 CSV 一键导入任务价格矩阵和 VIP1-VIP5 固定分组"
    )
    parser.add_argument("csv_file", type=Path, help="零售价 CSV 文件")
    parser.add_argument("--base-url", required=True, help="NewAPI 地址，例如 https://api.example.com")
    parser.add_argument("--username", default="root", help="root 管理员用户名，默认 root")
    parser.add_argument(
        "--rmb-per-usd",
        type=str,
        help="覆盖线上 USDExchangeRate；默认读取当前 NewAPI 设置",
    )
    parser.add_argument("--apply", action="store_true", help="实际写入；不提供时只预览")
    parser.add_argument(
        "--plan-output", type=Path, help="可选：把不含秘密的导入计划写入 JSON"
    )
    parser.add_argument(
        "--backup-dir",
        type=Path,
        default=Path("pricing-import-backups"),
        help="应用前备份目录",
    )
    args = parser.parse_args()

    password = os.environ.get("NEW_API_ADMIN_PASSWORD") or getpass.getpass(
        f"NewAPI 管理员 {args.username} 密码："
    )
    client = NewAPIClient(args.base_url)
    client.login(args.username, password)
    options = client.get_options()

    if args.rmb_per_usd is not None:
        rmb_per_usd = decimal_value(args.rmb_per_usd, "--rmb-per-usd", positive=True)
    else:
        rmb_per_usd = decimal_value(
            options.get("USDExchangeRate"), "线上 USDExchangeRate", positive=True
        )

    rows = read_csv_rows(args.csv_file)
    imported, summary = build_task_pricing(rows, rmb_per_usd)
    plan = build_plan(options, imported, summary)
    print_summary(summary, rmb_per_usd)

    if args.plan_output:
        write_private_json(args.plan_output, plan)
        print(f"计划文件：{args.plan_output.resolve()}")

    if not args.apply:
        print("预览完成，未写入。确认后增加 --apply 再运行。")
        return 0

    backup_path = apply_plan(client, plan, args.backup_dir)
    print(f"导入成功，回滚备份：{backup_path.resolve()}")
    print("请确认 AIPDD 渠道已启用 VIP1、VIP2、VIP3、VIP4、VIP5 分组。")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except ImportFailure as exc:
        print(f"错误：{exc}", file=sys.stderr)
        raise SystemExit(1)
