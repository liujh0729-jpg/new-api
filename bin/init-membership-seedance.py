#!/usr/bin/env python3
"""Idempotently initialize NewAPI membership levels and Seedance catalog.

The bundled production snapshot contains no credentials. This command is a dry
run unless --apply is supplied. Authentication and all provider credentials are
kept in memory and are never written to the plan or backup files.
"""

from __future__ import annotations

import argparse
import getpass
import hashlib
import http.cookiejar
import json
import os
import re
import sys
import urllib.error
import urllib.parse
import urllib.request
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterable


if os.name == "nt":
    for stream in (sys.stdout, sys.stderr):
        if hasattr(stream, "reconfigure"):
            stream.reconfigure(encoding="utf-8", errors="replace")


DEFAULT_CONFIG = Path(__file__).with_name(
    "membership-seedance-bootstrap.production.json"
)
DEFAULT_BACKUP_DIR = Path(__file__).resolve().parent.parent / "data" / "bootstrap-backups"
SEEDANCE_CHANNEL_TYPE = 59
ROOT_ROLE = 100
RESOLUTIONS = {"480p", "720p", "1080p", "2k", "4k"}
FORBIDDEN_PUBLIC_NAME_TOKENS = (
    "enhancement",
    "enhance",
    "super_resolution",
    "super-resolution",
    "upscale",
    "超分",
    "增强",
    "provider",
    "byok",
)


class BootstrapFailure(RuntimeError):
    pass


def canonical_json(value: Any) -> str:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"), sort_keys=True)


def decode_json(value: Any, label: str, expected_type: type) -> Any:
    if isinstance(value, expected_type):
        return value
    if value in (None, ""):
        return expected_type()
    if not isinstance(value, str):
        raise BootstrapFailure(f"{label} 不是合法 JSON {expected_type.__name__}")
    try:
        decoded = json.loads(value)
    except json.JSONDecodeError as exc:
        raise BootstrapFailure(f"{label} 不是合法 JSON：{exc}") from exc
    if not isinstance(decoded, expected_type):
        raise BootstrapFailure(f"{label} 必须是 JSON {expected_type.__name__}")
    return decoded


def require_list(config: dict[str, Any], key: str) -> list[dict[str, Any]]:
    value = config.get(key)
    if not isinstance(value, list) or not all(isinstance(item, dict) for item in value):
        raise BootstrapFailure(f"配置 {key} 必须是对象数组")
    return value


def unique_index(items: Iterable[dict[str, Any]], key: str, label: str) -> dict[str, dict[str, Any]]:
    result: dict[str, dict[str, Any]] = {}
    for item in items:
        value = str(item.get(key, "")).strip()
        if not value:
            raise BootstrapFailure(f"{label} 的 {key} 不能为空")
        if value in result:
            raise BootstrapFailure(f"{label} 的 {key} 重复：{value}")
        result[value] = item
    return result


def load_config(path: Path) -> dict[str, Any]:
    try:
        config = json.loads(path.read_text(encoding="utf-8"))
    except OSError as exc:
        raise BootstrapFailure(f"无法读取配置 {path}：{exc}") from exc
    except json.JSONDecodeError as exc:
        raise BootstrapFailure(f"配置 {path} 不是合法 JSON：{exc}") from exc
    if not isinstance(config, dict) or config.get("schema_version") != 1:
        raise BootstrapFailure("仅支持 schema_version=1 的初始化配置")
    validate_config(config)
    return config


def _positive_int(value: Any, label: str, *, allow_zero: bool = False) -> int:
    if isinstance(value, bool) or not isinstance(value, int):
        raise BootstrapFailure(f"{label} 必须是整数")
    minimum = 0 if allow_zero else 1
    if value < minimum:
        relation = "不小于 0" if allow_zero else "大于 0"
        raise BootstrapFailure(f"{label} 必须{relation}")
    return value


def _base_cost_map(item: dict[str, Any]) -> dict[tuple[str, bool], int]:
    rows = item.get("cost_matrix")
    if not isinstance(rows, list):
        raise BootstrapFailure(f"基础模型 {item.get('code')} 缺少 cost_matrix")
    result: dict[tuple[str, bool], int] = {}
    for row in rows:
        if not isinstance(row, dict):
            raise BootstrapFailure(f"基础模型 {item.get('code')} 的成本行必须是对象")
        resolution = str(row.get("source_resolution", "")).strip().lower()
        has_reference = row.get("has_reference_video")
        if resolution not in RESOLUTIONS or not isinstance(has_reference, bool):
            raise BootstrapFailure(f"基础模型 {item.get('code')} 的成本键无效")
        key = (resolution, has_reference)
        if key in result:
            raise BootstrapFailure(f"基础模型 {item.get('code')} 的成本键重复：{key}")
        result[key] = _positive_int(
            row.get("cost_micro_rmb_per_second"),
            f"基础模型 {item.get('code')} 成本",
            allow_zero=True,
        )
    return result


def _enhancement_cost_map(item: dict[str, Any]) -> dict[tuple[str, str], int]:
    rows = item.get("cost_matrix")
    if not isinstance(rows, list):
        raise BootstrapFailure(f"处理模型 {item.get('code')} 缺少 cost_matrix")
    result: dict[tuple[str, str], int] = {}
    for row in rows:
        if not isinstance(row, dict):
            raise BootstrapFailure(f"处理模型 {item.get('code')} 的成本行必须是对象")
        resolution = str(row.get("target_resolution", "")).strip().lower()
        bucket = str(row.get("fps_bucket", "")).strip().upper()
        if resolution not in RESOLUTIONS or bucket not in {"LE_30", "GT_30"}:
            raise BootstrapFailure(f"处理模型 {item.get('code')} 的成本键无效")
        key = (resolution, bucket)
        if key in result:
            raise BootstrapFailure(f"处理模型 {item.get('code')} 的成本键重复：{key}")
        result[key] = _positive_int(
            row.get("cost_micro_rmb_per_second"),
            f"处理模型 {item.get('code')} 成本",
            allow_zero=True,
        )
    return result


def is_ap_seedance_480p_membership_exempt(
    origin_model_name: str, target_resolution: str
) -> bool:
    if target_resolution.strip().lower() != "480p":
        return False
    normalized_name = re.sub(r"[\s_-]+", "", origin_model_name.strip().lower())
    return "apseedance" in normalized_name


def validate_config(config: dict[str, Any]) -> None:
    snapshot = config.get("snapshot")
    if not isinstance(snapshot, dict):
        raise BootstrapFailure("配置缺少 snapshot")
    namespace = str(snapshot.get("pricing_namespace", "")).strip()
    if not re.fullmatch(r"[A-Za-z0-9._-]{1,40}", namespace):
        raise BootstrapFailure("snapshot.pricing_namespace 格式无效")

    memberships = require_list(config, "membership_levels")
    membership_by_code = unique_index(memberships, "code", "会员等级")
    expected_memberships = ("VIP-T1", "VIP1", "VIP2", "VIP3", "VIP4", "VIP5")
    if set(membership_by_code) != set(expected_memberships):
        raise BootstrapFailure("会员等级必须恰好包含 VIP-T1、VIP1 至 VIP5")
    ranks: set[int] = set()
    multipliers: list[int] = []
    for code, item in membership_by_code.items():
        multiplier = _positive_int(item.get("multiplier_ppm"), f"{code} multiplier_ppm")
        if multiplier > 1_000_000:
            raise BootstrapFailure(f"{code} multiplier_ppm 不能大于 1000000")
        rank = _positive_int(item.get("rank"), f"{code} rank")
        if rank in ranks:
            raise BootstrapFailure("会员等级 rank 不能重复")
        ranks.add(rank)
        multipliers.append(multiplier)
        if not isinstance(item.get("enabled"), bool):
            raise BootstrapFailure(f"{code} enabled 必须是布尔值")
    if not all(
        membership_by_code[left]["rank"] > membership_by_code[right]["rank"]
        for left, right in zip(expected_memberships, expected_memberships[1:])
    ):
        raise BootstrapFailure("VIP-T1、VIP1 至 VIP5 的 rank 必须依次降低")

    provider = config.get("provider")
    if not isinstance(provider, dict):
        raise BootstrapFailure("配置缺少 provider")
    if provider.get("provider_type") != "DIRECT_EXTERNAL" or provider.get("adapter_type") != "VOLCENGINE_MEDIAKIT":
        raise BootstrapFailure("本脚本只支持 DIRECT_EXTERNAL/VOLCENGINE_MEDIAKIT")

    base_models = require_list(config, "base_models")
    enhancement_models = require_list(config, "enhancement_models")
    offerings = require_list(config, "offerings")
    base_by_code = unique_index(base_models, "code", "基础模型")
    retired_base_model_codes = config.get("retired_base_model_codes", [])
    if not isinstance(retired_base_model_codes, list) or not all(
        isinstance(code, str) and code.strip() for code in retired_base_model_codes
    ):
        raise BootstrapFailure("配置 retired_base_model_codes 必须是非空字符串数组")
    retired_base_model_codes = [code.strip() for code in retired_base_model_codes]
    if len(set(retired_base_model_codes)) != len(retired_base_model_codes):
        raise BootstrapFailure("retired_base_model_codes 不能重复")
    if set(retired_base_model_codes) & set(base_by_code):
        raise BootstrapFailure("待归档基础模型不能同时出现在 base_models")
    enhancement_by_code = unique_index(enhancement_models, "code", "处理模型")
    retired_enhancement_model_codes = config.get(
        "retired_enhancement_model_codes", []
    )
    if not isinstance(retired_enhancement_model_codes, list) or not all(
        isinstance(code, str) and code.strip()
        for code in retired_enhancement_model_codes
    ):
        raise BootstrapFailure(
            "配置 retired_enhancement_model_codes 必须是非空字符串数组"
        )
    retired_enhancement_model_codes = [
        code.strip() for code in retired_enhancement_model_codes
    ]
    if len(set(retired_enhancement_model_codes)) != len(
        retired_enhancement_model_codes
    ):
        raise BootstrapFailure("retired_enhancement_model_codes 不能重复")
    if set(retired_enhancement_model_codes) & set(enhancement_by_code):
        raise BootstrapFailure("待归档处理模型不能同时出现在 enhancement_models")
    unique_index(offerings, "display_name", "售卖模型")
    base_costs = {code: _base_cost_map(item) for code, item in base_by_code.items()}
    enhancement_costs = {
        code: _enhancement_cost_map(item) for code, item in enhancement_by_code.items()
    }
    lowest_multiplier = min(multipliers)

    for item in enhancement_models:
        specification = item.get("specification")
        if not isinstance(specification, dict):
            raise BootstrapFailure(f"处理模型 {item.get('code')} specification 必须是对象")
        if specification.get("scene", "aigc") != "aigc" or specification.get("tool_version") not in {"standard", "professional"}:
            raise BootstrapFailure(f"处理模型 {item.get('code')} MediaKit 规格无效")
        if str(specification.get("resolution", "")).lower() not in RESOLUTIONS:
            raise BootstrapFailure(f"处理模型 {item.get('code')} 分辨率无效")

    for item in offerings:
        name = str(item.get("display_name", "")).strip()
        normalized_name = name.lower()
        if not name or "," in name or any(token in normalized_name for token in FORBIDDEN_PUBLIC_NAME_TOKENS):
            raise BootstrapFailure(f"公开模型名含禁用内部词或逗号：{name}")
        if any(part == "sr" for part in re.split(r"[-_. ]+", normalized_name)):
            raise BootstrapFailure(f"公开模型名含禁用内部词 sr：{name}")
        base_code = str(item.get("base_model_code", ""))
        enhancement_code = str(item.get("enhancement_model_code") or "").strip()
        if base_code not in base_by_code:
            raise BootstrapFailure(f"{name} 引用了不存在的基础模型")
        if enhancement_code and enhancement_code not in enhancement_by_code:
            raise BootstrapFailure(f"{name} 引用了不存在的处理模型")
        source = str(item.get("source_resolution", "")).lower()
        target = str(item.get("target_resolution", "")).lower()
        fps = _positive_int(item.get("output_fps"), f"{name} output_fps")
        if source not in RESOLUTIONS or target not in RESOLUTIONS or fps > 240:
            raise BootstrapFailure(f"{name} 的分辨率或帧率无效")
        if not enhancement_code and source != target:
            raise BootstrapFailure(f"{name} 未配置处理模型时输入输出分辨率必须相同")
        bucket = "LE_30" if fps <= 30 else "GT_30"
        try:
            enhance_cost = (
                enhancement_costs[enhancement_code][(target, bucket)]
                if enhancement_code
                else 0
            )
            no_reference_total = base_costs[base_code][(source, False)] + enhance_cost
            reference_total = base_costs[base_code][(source, True)] + enhance_cost
        except KeyError as exc:
            raise BootstrapFailure(f"{name} 缺少对应成本矩阵行：{exc}") from exc
        expected_no_ref = _positive_int(
            item.get("expected_no_reference_total_cost_micro_rmb"),
            f"{name} 预期无参考视频总成本",
            allow_zero=True,
        )
        expected_ref = _positive_int(
            item.get("expected_reference_total_cost_micro_rmb"),
            f"{name} 预期含参考视频总成本",
            allow_zero=True,
        )
        if (no_reference_total, reference_total) != (expected_no_ref, expected_ref):
            raise BootstrapFailure(
                f"{name} 成本拆分不守恒：计算 {no_reference_total}/{reference_total}，"
                f"预期 {expected_no_ref}/{expected_ref}"
            )
        no_ref_sale = _positive_int(
            item.get("no_reference_unit_price_micro_rmb"), f"{name} 无参考视频售价"
        )
        ref_sale = _positive_int(
            item.get("reference_unit_price_micro_rmb"), f"{name} 含参考视频售价"
        )
        membership_multiplier = (
            1_000_000
            if is_ap_seedance_480p_membership_exempt(name, target)
            else lowest_multiplier
        )
        price_policy = (
            "480p 会员豁免规则"
            if membership_multiplier == 1_000_000
            else "最低会员倍率"
        )
        if no_ref_sale * membership_multiplier // 1_000_000 < no_reference_total:
            raise BootstrapFailure(f"{name} 在{price_policy}下无参考视频售价低于成本")
        if ref_sale * membership_multiplier // 1_000_000 < reference_total:
            raise BootstrapFailure(f"{name} 在{price_policy}下含参考视频售价低于成本")


def json_semantically_equal(left: Any, right: Any) -> bool:
    try:
        left_value = json.loads(left) if isinstance(left, str) else left
        right_value = json.loads(right) if isinstance(right, str) else right
    except json.JSONDecodeError:
        return False
    return left_value == right_value


def classify_action(
    existing: dict[str, Any] | None,
    desired: dict[str, Any],
    fields: Iterable[str],
    json_fields: Iterable[str] = (),
) -> str:
    if existing is None:
        return "create"
    json_field_set = set(json_fields)
    for field in fields:
        left = existing.get(field)
        right = desired.get(field)
        if field in json_field_set:
            if not json_semantically_equal(left, right):
                return "update"
        elif left != right:
            return "update"
    return "noop"


def pricing_version(
    config: dict[str, Any],
    offering: dict[str, Any],
    base_model: dict[str, Any],
    enhancement_model: dict[str, Any] | None,
    identity: dict[str, int | None] | None = None,
) -> str:
    material = {
        "offering": offering,
        "base_model": base_model,
        "enhancement_model": enhancement_model,
        "provider": config["provider"],
        # Revision row IDs are part of the offering snapshot. Including them
        # avoids reusing a pricing_version when an archived row is recreated
        # from otherwise identical declarative data.
        "identity": identity or {},
    }
    digest = hashlib.sha256(canonical_json(material).encode("utf-8")).hexdigest()[:12]
    namespace = str(config["snapshot"]["pricing_namespace"])
    return f"{namespace}-{digest}"


def write_private_json(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(descriptor, "w", encoding="utf-8", newline="\n") as handle:
        json.dump(value, handle, ensure_ascii=False, indent=2)
        handle.write("\n")
    try:
        os.chmod(path, 0o600)
    except OSError:
        pass


class NewAPIClient:
    def __init__(self, base_url: str, timeout: int = 30) -> None:
        parsed = urllib.parse.urlparse(base_url.strip())
        if parsed.scheme not in {"http", "https"} or not parsed.netloc:
            raise BootstrapFailure("--base-url 必须是完整的 http:// 或 https:// 地址")
        if parsed.username or parsed.password or parsed.query or parsed.fragment:
            raise BootstrapFailure("--base-url 不能包含用户信息、查询参数或片段")
        if parsed.scheme == "http" and parsed.hostname not in {
            "localhost",
            "127.0.0.1",
            "::1",
        }:
            raise BootstrapFailure("非本机 NewAPI 必须使用 HTTPS，避免管理员密码和密钥明文传输")
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout
        self.cookie_jar = http.cookiejar.CookieJar()
        self.opener = urllib.request.build_opener(
            urllib.request.HTTPCookieProcessor(self.cookie_jar)
        )
        self.user_id: int | None = None

    def request(self, method: str, path: str, payload: Any | None = None) -> Any:
        body = None
        headers = {"Accept": "application/json"}
        if payload is not None:
            body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
            headers["Content-Type"] = "application/json"
        if self.user_id is not None:
            headers["New-Api-User"] = str(self.user_id)
        request = urllib.request.Request(
            f"{self.base_url}{path}", data=body, headers=headers, method=method
        )
        try:
            with self.opener.open(request, timeout=self.timeout) as response:
                response_body = response.read().decode("utf-8")
        except urllib.error.HTTPError as exc:
            response_body = exc.read().decode("utf-8", errors="replace")
            try:
                detail = json.loads(response_body).get("message", response_body[:500])
            except (json.JSONDecodeError, AttributeError):
                detail = response_body[:500]
            raise BootstrapFailure(f"{method} {path} 返回 HTTP {exc.code}：{detail}") from exc
        except urllib.error.URLError as exc:
            raise BootstrapFailure(f"无法连接 NewAPI：{exc}") from exc
        try:
            result = json.loads(response_body)
        except json.JSONDecodeError as exc:
            raise BootstrapFailure(f"{method} {path} 没有返回合法 JSON") from exc
        if isinstance(result, dict) and result.get("success") is False:
            raise BootstrapFailure(
                f"{method} {path} 失败：{result.get('message', 'unknown error')}"
            )
        return result

    def data(self, method: str, path: str, payload: Any | None = None) -> Any:
        result = self.request(method, path, payload)
        if isinstance(result, dict) and "data" in result:
            return result["data"]
        return result

    def login(self, username: str, password: str) -> None:
        data = self.data(
            "POST", "/api/user/login", {"username": username, "password": password}
        )
        if not isinstance(data, dict):
            raise BootstrapFailure("登录响应缺少管理员信息")
        if data.get("require_2fa"):
            raise BootstrapFailure("管理员启用了 2FA；该脚本不绕过二次验证")
        if int(data.get("role", 0) or 0) != ROOT_ROLE:
            raise BootstrapFailure("初始化 Seedance 需要 root 管理员账号")
        self.user_id = int(data.get("id", 0) or 0)
        if self.user_id <= 0:
            raise BootstrapFailure("登录响应缺少有效用户 ID")

    def list_memberships(self) -> list[dict[str, Any]]:
        value = self.data("GET", "/api/membership/admin/levels?include_archived=true")
        if not isinstance(value, list):
            raise BootstrapFailure("会员等级列表响应结构无效")
        return value

    def list_channels(self) -> list[dict[str, Any]]:
        value = self.data(
            "GET", f"/api/channel/?p=1&page_size=100&type={SEEDANCE_CHANNEL_TYPE}"
        )
        items = value.get("items") if isinstance(value, dict) else None
        if not isinstance(items, list):
            raise BootstrapFailure("Seedance 渠道列表响应结构无效")
        return [item for item in items if item.get("type") == SEEDANCE_CHANNEL_TYPE]

    def overview(self, channel_id: int) -> dict[str, Any]:
        value = self.data("GET", f"/api/seedance-admin/overview?channel_id={channel_id}")
        if not isinstance(value, dict):
            raise BootstrapFailure("Seedance 概览响应结构无效")
        return value

    def list_resource(self, name: str, channel_id: int | None = None) -> list[dict[str, Any]]:
        suffix = ""
        if channel_id is not None:
            suffix = "?" + urllib.parse.urlencode({"channel_id": channel_id})
        value = self.data("GET", f"/api/seedance-admin/{name}{suffix}")
        if not isinstance(value, list):
            raise BootstrapFailure(f"Seedance {name} 响应结构无效")
        return value


def get_secret(env_name: str, prompt: str, non_interactive: bool) -> str:
    value = os.environ.get(env_name, "").strip()
    if value:
        return value
    if non_interactive:
        raise BootstrapFailure(f"非交互模式缺少环境变量 {env_name}")
    value = getpass.getpass(prompt).strip()
    if not value:
        raise BootstrapFailure(f"{env_name} 不能为空")
    return value


def find_single(
    items: Iterable[dict[str, Any]], field: str, value: Any, label: str
) -> dict[str, Any] | None:
    matches = [item for item in items if item.get(field) == value]
    if len(matches) > 1:
        raise BootstrapFailure(f"发现多个 {label}（{field}={value}），请先人工去重")
    return matches[0] if matches else None


def desired_membership(item: dict[str, Any]) -> dict[str, Any]:
    return {
        "code": str(item["code"]).upper(),
        "display_name": str(item["display_name"]),
        "multiplier_ppm": int(item["multiplier_ppm"]),
        "rank": int(item["rank"]),
        "sort_order": int(item["sort_order"]),
        "enabled": bool(item["enabled"]),
    }


def desired_provider(config: dict[str, Any]) -> dict[str, Any]:
    item = config["provider"]
    return {
        "provider_type": item["provider_type"],
        "adapter_type": item["adapter_type"],
        "display_name": item["display_name"],
        "service_endpoint": item["service_endpoint"],
        "service_code": item["service_code"],
        "capabilities": canonical_json(item.get("capabilities", {})),
        "status": item["status"],
        "timeout_policy": canonical_json(item.get("timeout_policy", {})),
        "retry_policy": canonical_json(item.get("retry_policy", {})),
        "fallback_policy": canonical_json(item.get("fallback_policy", {})),
    }


def desired_base_model(item: dict[str, Any]) -> dict[str, Any]:
    return {
        "code": item["code"],
        "display_name": item["display_name"],
        "provider_model_id": item["provider_model_id"],
        "cost_matrix": canonical_json(item["cost_matrix"]),
        "enabled": bool(item.get("enabled", True)),
    }


def desired_enhancement_model(item: dict[str, Any], provider_id: int) -> dict[str, Any]:
    return {
        "code": item["code"],
        "display_name": item["display_name"],
        "provider_id": provider_id,
        "service_code": item["service_code"],
        "quality_tier": item["quality_tier"],
        "specification": canonical_json(item["specification"]),
        "specification_version": item["specification_version"],
        "cost_matrix": canonical_json(item["cost_matrix"]),
        "enabled": bool(item.get("enabled", True)),
    }


def desired_offering(
    config: dict[str, Any],
    item: dict[str, Any],
    channel_id: int,
    base_model_id: int,
    enhancement_model_id: int | None,
    enabled: bool,
) -> dict[str, Any]:
    base_by_code = unique_index(config["base_models"], "code", "基础模型")
    enhancement_by_code = unique_index(
        config["enhancement_models"], "code", "处理模型"
    )
    enhancement_code = str(item.get("enhancement_model_code") or "").strip()
    return {
        "channel_id": channel_id,
        "display_name": item["display_name"],
        "base_model_id": base_model_id,
        "enhancement_model_id": enhancement_model_id,
        "source_resolution": str(item["source_resolution"]).lower(),
        "target_resolution": str(item["target_resolution"]).lower(),
        "output_fps": int(item["output_fps"]),
        "no_reference_unit_price_micro_rmb": int(
            item["no_reference_unit_price_micro_rmb"]
        ),
        "reference_unit_price_micro_rmb": int(
            item["reference_unit_price_micro_rmb"]
        ),
        "pricing_version": pricing_version(
            config,
            item,
            base_by_code[item["base_model_code"]],
            enhancement_by_code.get(enhancement_code),
            {
                "base_model_id": base_model_id,
                "enhancement_model_id": enhancement_model_id,
            },
        ),
        "enabled": enabled,
    }


MEMBERSHIP_FIELDS = (
    "display_name",
    "multiplier_ppm",
    "rank",
    "sort_order",
    "enabled",
)
PROVIDER_FIELDS = (
    "provider_type",
    "adapter_type",
    "display_name",
    "service_endpoint",
    "service_code",
    "capabilities",
    "status",
    "timeout_policy",
    "retry_policy",
    "fallback_policy",
)
PROVIDER_JSON_FIELDS = (
    "capabilities",
    "timeout_policy",
    "retry_policy",
    "fallback_policy",
)
BASE_FIELDS = ("display_name", "provider_model_id", "cost_matrix", "enabled")
ENHANCEMENT_FIELDS = (
    "display_name",
    "provider_id",
    "service_code",
    "quality_tier",
    "specification",
    "specification_version",
    "cost_matrix",
    "enabled",
)
OFFERING_FIELDS = (
    "channel_id",
    "display_name",
    "base_model_id",
    "enhancement_model_id",
    "source_resolution",
    "target_resolution",
    "output_fps",
    "no_reference_unit_price_micro_rmb",
    "reference_unit_price_micro_rmb",
    "pricing_version",
    "enabled",
)


def select_channel(
    channels: list[dict[str, Any]], channel_id: int | None
) -> dict[str, Any] | None:
    if channel_id is not None:
        channel = find_single(channels, "id", channel_id, "Seedance 渠道")
        if channel is None:
            raise BootstrapFailure(f"未找到类型 59 的 Seedance 渠道 #{channel_id}")
        return channel
    if len(channels) == 1:
        return channels[0]
    if len(channels) > 1:
        choices = ", ".join(f"#{item.get('id')} {item.get('name')}" for item in channels)
        raise BootstrapFailure(f"存在多个 Seedance 渠道，请用 --channel-id 指定：{choices}")
    return None


def create_channel(client: NewAPIClient, name: str) -> dict[str, Any]:
    client.data(
        "POST",
        "/api/channel/",
        {
            "mode": "single",
            "channel": {
                "type": SEEDANCE_CHANNEL_TYPE,
                "key": "managed",
                "status": 1,
                "name": name,
                "base_url": "",
                "models": "",
                "group": "default",
                "weight": 0,
                "priority": 0,
                "auto_ban": 1,
            },
        },
    )
    channel = find_single(client.list_channels(), "name", name, "新建 Seedance 渠道")
    if channel is None:
        raise BootstrapFailure("渠道创建成功但无法重新定位，请在管理页检查")
    return channel


def resolve_instance_id(
    args: argparse.Namespace,
    overview: dict[str, Any] | None,
    existing_config: dict[str, Any] | None,
    *,
    required: bool,
) -> str:
    candidates = (
        args.instance_id,
        (existing_config or {}).get("instance_id"),
        os.environ.get("AIPDD_INSTANCE_ID"),
        (overview or {}).get("site_instance_id"),
    )
    value = next((str(candidate).strip() for candidate in candidates if str(candidate or "").strip()), "")
    if not value:
        if required:
            raise BootstrapFailure("缺少实例 UUID：请设置 AIPDD_INSTANCE_ID 或使用 --instance-id")
        return ""
    try:
        uuid.UUID(value)
    except ValueError as exc:
        raise BootstrapFailure(f"实例 ID 不是合法 UUID：{value}") from exc
    return value


def config_request(
    args: argparse.Namespace,
    overview: dict[str, Any],
    provider_id: int,
    billing_api_key: str = "",
) -> dict[str, Any]:
    existing = overview.get("config") if isinstance(overview.get("config"), dict) else {}
    base_url = (
        str(args.aipdd_billing_base_url or "").strip()
        or str(existing.get("aipdd_billing_base_url") or "").strip()
        or os.environ.get("AIPDD_BASE_URL", "").strip()
    )
    return {
        "instance_id": resolve_instance_id(args, overview, existing, required=True),
        "aipdd_billing_base_url": base_url,
        "aipdd_billing_api_key": billing_api_key,
        "volcengine_bill_sync_enabled": bool(
            existing.get("volcengine_bill_sync_enabled", False)
        ),
        "volcengine_bill_product_codes": decode_json(
            existing.get("volcengine_bill_product_codes", "[]"),
            "volcengine_bill_product_codes",
            list,
        ),
        "volcengine_bill_configuration_codes": decode_json(
            existing.get("volcengine_bill_configuration_codes", "[]"),
            "volcengine_bill_configuration_codes",
            list,
        ),
        "default_enhancement_provider_id": provider_id,
        "status": "ACTIVE",
    }


def config_action(existing: dict[str, Any] | None, desired: dict[str, Any]) -> str:
    if existing is None:
        return "create"
    scalar_fields = (
        "instance_id",
        "aipdd_billing_base_url",
        "volcengine_bill_sync_enabled",
        "default_enhancement_provider_id",
        "status",
    )
    if any(existing.get(field) != desired.get(field) for field in scalar_fields):
        return "update"
    for existing_field, desired_field in (
        ("volcengine_bill_product_codes", "volcengine_bill_product_codes"),
        ("volcengine_bill_configuration_codes", "volcengine_bill_configuration_codes"),
    ):
        if decode_json(existing.get(existing_field, "[]"), existing_field, list) != desired[desired_field]:
            return "update"
    return "noop"


def print_action(category: str, name: str, action: str) -> None:
    labels = {
        "create": "新增",
        "update": "更新",
        "archive": "归档",
        "noop": "一致",
        "validate": "验证",
        "missing": "缺失",
    }
    print(f"[{labels.get(action, action)}] {category}：{name}")


def collect_state(client: NewAPIClient, channel: dict[str, Any] | None) -> dict[str, Any]:
    state = {
        "memberships": client.list_memberships(),
        "channels": client.list_channels(),
        "providers": client.list_resource("providers"),
        "base_models": client.list_resource("base-models"),
        "enhancement_models": client.list_resource("enhancement-models"),
        "overview": None,
        "offerings": [],
    }
    if channel is not None:
        overview = client.overview(int(channel["id"]))
        state["overview"] = overview
        state["offerings"] = overview.get("offerings", [])
    return state


def build_plan(
    config: dict[str, Any],
    args: argparse.Namespace,
    state: dict[str, Any],
    channel: dict[str, Any] | None,
) -> list[dict[str, str]]:
    plan: list[dict[str, str]] = []
    memberships = state["memberships"]
    for item in config["membership_levels"]:
        desired = desired_membership(item)
        existing = find_single(memberships, "code", desired["code"], "会员等级")
        if existing and int(existing.get("archived_at", 0) or 0) > 0:
            raise BootstrapFailure(f"会员等级 {desired['code']} 已归档，API 不支持直接恢复")
        plan.append(
            {
                "category": "会员等级",
                "name": desired["code"],
                "action": classify_action(existing, desired, MEMBERSHIP_FIELDS),
            }
        )

    if channel is None:
        plan.append(
            {
                "category": "Seedance 渠道",
                "name": args.channel_name,
                "action": "create" if args.create_channel else "missing",
            }
        )
    else:
        plan.append(
            {
                "category": "Seedance 渠道",
                "name": f"#{channel['id']} {channel.get('name', '')}",
                "action": "noop",
            }
        )

    desired_provider_value = desired_provider(config)
    provider = find_single(
        state["providers"],
        "display_name",
        desired_provider_value["display_name"],
        "MediaKit Provider",
    )
    provider_action = classify_action(
        provider,
        desired_provider_value,
        PROVIDER_FIELDS,
        PROVIDER_JSON_FIELDS,
    )
    if provider and not provider.get("credential_configured"):
        provider_action = "update"
    if args.rotate_secrets and provider:
        provider_action = "update"
    plan.append(
        {
            "category": "处理节点",
            "name": desired_provider_value["display_name"],
            "action": provider_action,
        }
    )

    provider_id = int((provider or {}).get("id", 0) or 0)
    base_by_code = {item.get("code"): item for item in state["base_models"]}
    for item in config["base_models"]:
        desired = desired_base_model(item)
        plan.append(
            {
                "category": "基础模型",
                "name": desired["code"],
                "action": classify_action(
                    base_by_code.get(desired["code"]),
                    desired,
                    BASE_FIELDS,
                    ("cost_matrix",),
                ),
            }
        )

    enhancement_by_code = {
        item.get("code"): item for item in state["enhancement_models"]
    }
    for item in config["enhancement_models"]:
        desired = desired_enhancement_model(item, provider_id)
        existing = enhancement_by_code.get(desired["code"])
        action = "create" if provider_id == 0 and existing is None else classify_action(
            existing,
            desired,
            ENHANCEMENT_FIELDS,
            ("specification", "cost_matrix"),
        )
        plan.append(
            {"category": "处理模型", "name": desired["code"], "action": action}
        )

    overview = state.get("overview")
    if channel is not None and isinstance(overview, dict):
        desired_cfg = config_request(args, overview, provider_id)
        cfg_action = config_action(overview.get("config"), desired_cfg)
        billing_configured = bool(overview.get("billing_credential_configured"))
        billing_key_available = bool(
            os.environ.get("NEW_API_SEEDANCE_AIPDD_BILLING_API_KEY", "").strip()
        )
        if (
            (not billing_configured and (billing_key_available or args.publish))
            or (args.rotate_secrets and billing_key_available)
        ):
            cfg_action = "update" if overview.get("config") else "create"
        plan.append(
            {"category": "渠道配置", "name": str(channel["id"]), "action": cfg_action}
        )
        if billing_configured:
            billing_action = "update" if args.rotate_secrets and billing_key_available else "noop"
        elif billing_key_available or args.publish:
            billing_action = "create"
        else:
            billing_action = "missing"
        plan.append(
            {
                "category": "AIPDD 计费凭证",
                "name": f"渠道 #{channel['id']}",
                "action": billing_action,
            }
        )
        active_credentials = [
            item for item in overview.get("credentials", []) if item.get("status") == "ACTIVE"
        ]
        plan.append(
            {
                "category": "Ark 凭证",
                "name": f"渠道 #{channel['id']}",
                "action": "create" if args.rotate_secrets or not active_credentials else "noop",
            }
        )

    offering_by_name = {item.get("display_name"): item for item in state["offerings"]}
    for item in config["offerings"]:
        existing = offering_by_name.get(item["display_name"])
        base_existing = base_by_code.get(item["base_model_code"])
        enhancement_code = str(item.get("enhancement_model_code") or "").strip()
        enhancement_existing = enhancement_by_code.get(enhancement_code)
        if channel is None or not base_existing or (
            enhancement_code and not enhancement_existing
        ):
            action = "create" if existing is None else "update"
        else:
            enabled = bool(args.publish) if args.publish else bool(
                existing.get("enabled", False) if existing else False
            )
            desired = desired_offering(
                config,
                item,
                int(channel["id"]),
                int(base_existing["id"]),
                int(enhancement_existing["id"]) if enhancement_existing else None,
                enabled,
            )
            action = classify_action(existing, desired, OFFERING_FIELDS)
            if existing and int(existing.get("archived_at", 0) or 0) > 0:
                action = "update"
        plan.append(
            {"category": "售卖模型", "name": item["display_name"], "action": action}
        )
    for code in config.get("retired_base_model_codes", []):
        if base_by_code.get(code):
            plan.append(
                {
                    "category": "历史误建基础模型",
                    "name": code,
                    "action": "archive",
                }
            )
    for code in config.get("retired_enhancement_model_codes", []):
        if enhancement_by_code.get(code):
            plan.append(
                {
                    "category": "历史误建处理模型",
                    "name": code,
                    "action": "archive",
                }
            )
    return plan


def preflight_apply_secrets(
    config: dict[str, Any],
    args: argparse.Namespace,
    state: dict[str, Any],
    channel: dict[str, Any] | None,
) -> None:
    """Resolve every required secret before the first mutating API request."""
    desired = desired_provider(config)
    provider = find_single(
        state["providers"],
        "display_name",
        desired["display_name"],
        "MediaKit Provider",
    )
    if provider is None or not provider.get("credential_configured") or args.rotate_secrets:
        args._mediakit_api_key = get_secret(
            "NEW_API_SEEDANCE_MEDIAKIT_API_KEY",
            "火山 AI MediaKit API Key：",
            args.non_interactive,
        )

    overview = state.get("overview")
    if not isinstance(overview, dict):
        overview = {
            "config": None,
            "site_instance_id": "",
            "billing_credential_configured": False,
            "credentials": [],
        }

    billing_configured = bool(overview.get("billing_credential_configured"))
    billing_api_key = os.environ.get(
        "NEW_API_SEEDANCE_AIPDD_BILLING_API_KEY", ""
    ).strip()
    if args.publish and not billing_configured and not billing_api_key:
        billing_api_key = get_secret(
            "NEW_API_SEEDANCE_AIPDD_BILLING_API_KEY",
            "AIPDD 后付费计费 API Key：",
            args.non_interactive,
        )
    if billing_api_key and (args.rotate_secrets or not billing_configured):
        args._aipdd_billing_api_key = billing_api_key

    provider_id = int((provider or {}).get("id", 0) or 0)
    desired_config = config_request(args, overview, provider_id, billing_api_key)
    if args.publish and not desired_config["aipdd_billing_base_url"]:
        raise BootstrapFailure(
            "--publish 要求配置 AIPDD 财务地址：设置 AIPDD_BASE_URL 或 --aipdd-billing-base-url"
        )

    active_credentials = [
        item for item in overview.get("credentials", []) if item.get("status") == "ACTIVE"
    ]
    if args.rotate_secrets or not active_credentials:
        args._ark_api_key = get_secret(
            "NEW_API_SEEDANCE_ARK_API_KEY",
            "火山 Ark API Key：",
            args.non_interactive,
        )
        args._access_key_id = os.environ.get(
            "NEW_API_SEEDANCE_ACCESS_KEY_ID", ""
        ).strip()
        args._secret_access_key = os.environ.get(
            "NEW_API_SEEDANCE_SECRET_ACCESS_KEY", ""
        ).strip()
        if bool(args._access_key_id) != bool(args._secret_access_key):
            raise BootstrapFailure(
                "NEW_API_SEEDANCE_ACCESS_KEY_ID 与 NEW_API_SEEDANCE_SECRET_ACCESS_KEY 必须同时设置"
            )
        if desired_config["volcengine_bill_sync_enabled"] and not args._access_key_id:
            raise BootstrapFailure("线上已启用火山账单同步，轮换凭证时必须同时提供 AK/SK")


def _save_memberships(
    client: NewAPIClient, config: dict[str, Any]
) -> list[dict[str, Any]]:
    current = client.list_memberships()
    result: list[dict[str, Any]] = []
    for item in config["membership_levels"]:
        desired = desired_membership(item)
        existing = find_single(current, "code", desired["code"], "会员等级")
        action = classify_action(existing, desired, MEMBERSHIP_FIELDS)
        if action == "create":
            saved = client.data("POST", "/api/membership/admin/levels", desired)
        elif action == "update":
            payload = {key: desired[key] for key in MEMBERSHIP_FIELDS}
            saved = client.data(
                "PUT", f"/api/membership/admin/levels/{existing['id']}", payload
            )
        else:
            saved = existing
        print_action("会员等级", desired["code"], action)
        result.append(saved)
    return result


def _save_provider(
    client: NewAPIClient,
    config: dict[str, Any],
    args: argparse.Namespace,
) -> dict[str, Any]:
    desired = desired_provider(config)
    providers = client.list_resource("providers")
    existing = find_single(
        providers, "display_name", desired["display_name"], "MediaKit Provider"
    )
    action = classify_action(
        existing, desired, PROVIDER_FIELDS, PROVIDER_JSON_FIELDS
    )
    need_secret = existing is None or not existing.get("credential_configured") or args.rotate_secrets
    if need_secret:
        desired["mediakit_api_key"] = getattr(args, "_mediakit_api_key", "") or get_secret(
            "NEW_API_SEEDANCE_MEDIAKIT_API_KEY",
            "火山 AI MediaKit API Key：",
            args.non_interactive,
        )
        action = "create" if existing is None else "update"
    if action == "noop":
        saved = existing
    else:
        if existing:
            desired["id"] = int(existing["id"])
        saved = client.data(
            "POST" if existing is None else "PUT",
            "/api/seedance-admin/providers",
            desired,
        )
    print_action("处理节点", desired["display_name"], action)
    if not isinstance(saved, dict) or int(saved.get("id", 0) or 0) <= 0:
        raise BootstrapFailure("处理节点保存响应缺少有效 ID")
    return saved


def _save_channel_config_and_credentials(
    client: NewAPIClient,
    args: argparse.Namespace,
    channel: dict[str, Any],
    provider: dict[str, Any],
) -> dict[str, Any]:
    channel_id = int(channel["id"])
    overview = client.overview(channel_id)
    billing_secret_needed = args.rotate_secrets or not overview.get(
        "billing_credential_configured", False
    )
    billing_api_key = getattr(args, "_aipdd_billing_api_key", "")
    if billing_secret_needed:
        billing_api_key = billing_api_key or os.environ.get(
            "NEW_API_SEEDANCE_AIPDD_BILLING_API_KEY", ""
        ).strip()
        if args.publish and not billing_api_key:
            billing_api_key = get_secret(
                "NEW_API_SEEDANCE_AIPDD_BILLING_API_KEY",
                "AIPDD 后付费计费 API Key：",
                args.non_interactive,
            )
    desired = config_request(args, overview, int(provider["id"]), billing_api_key)
    if args.publish and not desired["aipdd_billing_base_url"]:
        raise BootstrapFailure(
            "--publish 要求配置 AIPDD 财务地址：设置 AIPDD_BASE_URL 或 --aipdd-billing-base-url"
        )
    action = config_action(overview.get("config"), desired)
    if billing_api_key:
        action = "create" if not overview.get("config") else "update"
    if action != "noop":
        client.data(
            "PUT", f"/api/seedance-admin/channels/{channel_id}/config", desired
        )
    print_action("渠道配置", f"#{channel_id}", action)

    overview = client.overview(channel_id)
    active = [
        item for item in overview.get("credentials", []) if item.get("status") == "ACTIVE"
    ]
    rotate = args.rotate_secrets or not active
    if rotate:
        ark_api_key = getattr(args, "_ark_api_key", "") or get_secret(
            "NEW_API_SEEDANCE_ARK_API_KEY",
            "火山 Ark API Key：",
            args.non_interactive,
        )
        access_key_id = getattr(args, "_access_key_id", "") or os.environ.get(
            "NEW_API_SEEDANCE_ACCESS_KEY_ID", ""
        ).strip()
        secret_access_key = getattr(args, "_secret_access_key", "") or os.environ.get(
            "NEW_API_SEEDANCE_SECRET_ACCESS_KEY", ""
        ).strip()
        if bool(access_key_id) != bool(secret_access_key):
            raise BootstrapFailure(
                "NEW_API_SEEDANCE_ACCESS_KEY_ID 与 NEW_API_SEEDANCE_SECRET_ACCESS_KEY 必须同时设置"
            )
        if desired["volcengine_bill_sync_enabled"] and not access_key_id:
            raise BootstrapFailure("线上已启用火山账单同步，轮换凭证时必须同时提供 AK/SK")
        credential = client.data(
            "POST",
            f"/api/seedance-admin/channels/{channel_id}/credentials",
            {
                "ark_api_key": ark_api_key,
                "access_key_id": access_key_id,
                "secret_access_key": secret_access_key,
            },
        )
        if not isinstance(credential, dict) or int(credential.get("id", 0) or 0) <= 0:
            raise BootstrapFailure("Ark 凭证创建响应缺少有效 ID")
        client.data(
            "POST", f"/api/seedance-admin/credentials/{credential['id']}/validate"
        )
        print_action("Ark 凭证", f"渠道 #{channel_id}", "validate")
    else:
        existing_config = overview.get("config") or {}
        if int(existing_config.get("last_verified_at", 0) or 0) <= 0:
            client.data(
                "POST", f"/api/seedance-admin/credentials/{active[0]['id']}/validate"
            )
            print_action("Ark 凭证", f"渠道 #{channel_id}", "validate")
        else:
            print_action("Ark 凭证", f"渠道 #{channel_id}", "noop")

    overview = client.overview(channel_id)
    if args.publish and not overview.get("billing_credential_configured"):
        raise BootstrapFailure("--publish 前必须配置 AIPDD 后付费计费 API Key")
    return overview


def _save_base_models(
    client: NewAPIClient, config: dict[str, Any]
) -> dict[str, dict[str, Any]]:
    current = {item.get("code"): item for item in client.list_resource("base-models")}
    result: dict[str, dict[str, Any]] = {}
    for item in config["base_models"]:
        desired = desired_base_model(item)
        existing = current.get(desired["code"])
        action = classify_action(
            existing, desired, BASE_FIELDS, ("cost_matrix",)
        )
        if action == "noop":
            saved = existing
        else:
            if existing:
                desired["id"] = int(existing["id"])
            saved = client.data(
                "POST" if existing is None else "PUT",
                "/api/seedance-admin/base-models",
                desired,
            )
        print_action("基础模型", desired["code"], action)
        if not isinstance(saved, dict) or int(saved.get("id", 0) or 0) <= 0:
            raise BootstrapFailure(f"基础模型 {desired['code']} 保存响应缺少有效 ID")
        result[desired["code"]] = saved
    return result


def _save_enhancement_models(
    client: NewAPIClient,
    config: dict[str, Any],
    provider_id: int,
) -> dict[str, dict[str, Any]]:
    current = {
        item.get("code"): item for item in client.list_resource("enhancement-models")
    }
    result: dict[str, dict[str, Any]] = {}
    for item in config["enhancement_models"]:
        desired = desired_enhancement_model(item, provider_id)
        existing = current.get(desired["code"])
        action = classify_action(
            existing,
            desired,
            ENHANCEMENT_FIELDS,
            ("specification", "cost_matrix"),
        )
        if action == "noop":
            saved = existing
        else:
            if existing:
                desired["id"] = int(existing["id"])
            saved = client.data(
                "POST" if existing is None else "PUT",
                "/api/seedance-admin/enhancement-models",
                desired,
            )
        print_action("处理模型", desired["code"], action)
        if not isinstance(saved, dict) or int(saved.get("id", 0) or 0) <= 0:
            raise BootstrapFailure(f"处理模型 {desired['code']} 保存响应缺少有效 ID")
        result[desired["code"]] = saved
    return result


def _save_offerings(
    client: NewAPIClient,
    config: dict[str, Any],
    args: argparse.Namespace,
    channel_id: int,
    bases: dict[str, dict[str, Any]],
    enhancements: dict[str, dict[str, Any]],
) -> list[dict[str, Any]]:
    current = {
        item.get("display_name"): item
        for item in client.list_resource("offerings", channel_id)
    }
    result: list[dict[str, Any]] = []
    for item in config["offerings"]:
        existing = current.get(item["display_name"])
        enabled = True if args.publish else bool(
            existing.get("enabled", False) if existing else False
        )
        enhancement_code = str(item.get("enhancement_model_code") or "").strip()
        enhancement = enhancements.get(enhancement_code)
        desired = desired_offering(
            config,
            item,
            channel_id,
            int(bases[item["base_model_code"]]["id"]),
            int(enhancement["id"]) if enhancement else None,
            enabled,
        )
        action = classify_action(existing, desired, OFFERING_FIELDS)
        if existing and int(existing.get("archived_at", 0) or 0) > 0:
            action = "update"
        if action == "noop":
            saved = existing
        else:
            if existing:
                desired["id"] = int(existing["id"])
            saved = client.data(
                "POST" if existing is None else "PUT",
                "/api/seedance-admin/offerings",
                desired,
            )
        print_action("售卖模型", desired["display_name"], action)
        result.append(saved)
    return result


def _archive_retired_base_models(
    client: NewAPIClient, config: dict[str, Any]
) -> None:
    current = {item.get("code"): item for item in client.list_resource("base-models")}
    for code in config.get("retired_base_model_codes", []):
        existing = current.get(code)
        if existing is None:
            continue
        client.data("DELETE", f"/api/seedance-admin/base-models/{int(existing['id'])}")
        print_action("历史误建基础模型", code, "archive")


def _archive_retired_enhancement_models(
    client: NewAPIClient, config: dict[str, Any]
) -> None:
    current = {
        item.get("code"): item
        for item in client.list_resource("enhancement-models")
    }
    for code in config.get("retired_enhancement_model_codes", []):
        existing = current.get(code)
        if existing is None:
            continue
        client.data(
            "DELETE",
            f"/api/seedance-admin/enhancement-models/{int(existing['id'])}",
        )
        print_action("历史误建处理模型", code, "archive")


def verify_result(
    client: NewAPIClient,
    config: dict[str, Any],
    args: argparse.Namespace,
    channel_id: int,
) -> None:
    memberships = {item.get("code"): item for item in client.list_memberships()}
    for item in config["membership_levels"]:
        desired = desired_membership(item)
        if classify_action(memberships.get(desired["code"]), desired, MEMBERSHIP_FIELDS) != "noop":
            raise BootstrapFailure(f"写入后校验失败：会员等级 {desired['code']}")

    providers = client.list_resource("providers")
    desired_provider_value = desired_provider(config)
    provider = find_single(
        providers,
        "display_name",
        desired_provider_value["display_name"],
        "MediaKit Provider",
    )
    if provider is None or not provider.get("credential_configured"):
        raise BootstrapFailure("写入后校验失败：MediaKit Provider 或凭证缺失")

    bases = {item.get("code"): item for item in client.list_resource("base-models")}
    enhancements = {
        item.get("code"): item for item in client.list_resource("enhancement-models")
    }
    for item in config["base_models"]:
        desired = desired_base_model(item)
        if classify_action(bases.get(desired["code"]), desired, BASE_FIELDS, ("cost_matrix",)) != "noop":
            raise BootstrapFailure(f"写入后校验失败：基础模型 {desired['code']}")
    for code in config.get("retired_base_model_codes", []):
        if code in bases:
            raise BootstrapFailure(f"写入后校验失败：错误基础模型尚未归档 {code}")
    for item in config["enhancement_models"]:
        desired = desired_enhancement_model(item, int(provider["id"]))
        if classify_action(
            enhancements.get(desired["code"]),
            desired,
            ENHANCEMENT_FIELDS,
            ("specification", "cost_matrix"),
        ) != "noop":
            raise BootstrapFailure(f"写入后校验失败：处理模型 {desired['code']}")
    for code in config.get("retired_enhancement_model_codes", []):
        if code in enhancements:
            raise BootstrapFailure(f"写入后校验失败：错误处理模型尚未归档 {code}")

    overview = client.overview(channel_id)
    if not any(item.get("status") == "ACTIVE" for item in overview.get("credentials", [])):
        raise BootstrapFailure("写入后校验失败：没有 ACTIVE Ark 凭证")
    if int((overview.get("config") or {}).get("last_verified_at", 0) or 0) <= 0:
        raise BootstrapFailure("写入后校验失败：渠道配置尚未完成凭证验证")

    offerings = {
        item.get("display_name"): item
        for item in client.list_resource("offerings", channel_id)
    }
    for item in config["offerings"]:
        existing = offerings.get(item["display_name"])
        enhancement_code = str(item.get("enhancement_model_code") or "").strip()
        enhancement = enhancements.get(enhancement_code)
        desired = desired_offering(
            config,
            item,
            channel_id,
            int(bases[item["base_model_code"]]["id"]),
            int(enhancement["id"]) if enhancement else None,
            True if args.publish else bool(existing.get("enabled", False) if existing else False),
        )
        if classify_action(existing, desired, OFFERING_FIELDS) != "noop":
            raise BootstrapFailure(f"写入后校验失败：售卖模型 {desired['display_name']}")


def print_snapshot_summary(config: dict[str, Any], publish: bool) -> None:
    print(f"数据快照：{config['snapshot']['name']}")
    print(f"会员等级：{len(config['membership_levels'])} 个")
    print(f"基础模型：{len(config['base_models'])} 个")
    print(f"处理模型：{len(config['enhancement_models'])} 个")
    print(f"售卖模型：{len(config['offerings'])} 个")
    print(f"发布策略：{'写入后启用' if publish else '新模型停用、已有模型保留启停状态'}")


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="初始化最新版 NewAPI 的 VIP-T1、VIP1-VIP5 与独立 Seedance 管理数据"
    )
    parser.add_argument("--base-url", required=True, help="NewAPI 地址，例如 https://api.example.com")
    parser.add_argument("--username", default="root", help="root 用户名，默认 root")
    parser.add_argument("--config", type=Path, default=DEFAULT_CONFIG, help="初始化 JSON 配置")
    parser.add_argument("--channel-id", type=int, help="已有 Seedance(type=59) 渠道 ID")
    parser.add_argument("--create-channel", action="store_true", help="没有 Seedance 渠道时自动创建")
    parser.add_argument("--channel-name", default="Seedance 独立平台", help="自动创建的渠道名")
    parser.add_argument("--instance-id", help="AIPDD 实例 UUID；默认沿用线上或 AIPDD_INSTANCE_ID")
    parser.add_argument("--aipdd-billing-base-url", help="覆盖/首次设置 AIPDD 后付费地址")
    parser.add_argument("--publish", action="store_true", help="写入后启用本快照中的全部售卖模型")
    parser.add_argument("--rotate-secrets", action="store_true", help="轮换 Ark、MediaKit 及已提供的 AIPDD 凭证")
    parser.add_argument("--apply", action="store_true", help="实际写入；不提供时只预演")
    parser.add_argument("--non-interactive", action="store_true", help="禁止密码提示；缺少环境变量时失败")
    parser.add_argument("--plan-output", type=Path, help="把不含密钥的预演计划写入 JSON")
    parser.add_argument("--backup-dir", type=Path, default=DEFAULT_BACKUP_DIR, help="应用前快照目录")
    parser.add_argument("--timeout", type=int, default=30, help="单次 HTTP 超时秒数，默认 30")
    args = parser.parse_args(argv)
    if args.channel_id is not None and args.channel_id <= 0:
        parser.error("--channel-id 必须大于 0")
    if args.timeout <= 0:
        parser.error("--timeout 必须大于 0")
    if args.publish and not args.apply:
        print("提示：当前为预演，--publish 仅展示计划，不会写入。")
    return args


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    config = load_config(args.config)
    password = os.environ.get("NEW_API_ADMIN_PASSWORD", "").strip()
    if not password:
        if args.non_interactive:
            raise BootstrapFailure("非交互模式缺少环境变量 NEW_API_ADMIN_PASSWORD")
        password = getpass.getpass(f"NewAPI root 用户 {args.username} 密码：")
    client = NewAPIClient(args.base_url, args.timeout)
    client.login(args.username, password)

    channels = client.list_channels()
    channel = select_channel(channels, args.channel_id)
    if channel is None and not args.create_channel:
        raise BootstrapFailure(
            "当前没有 Seedance(type=59) 渠道；请先创建，或增加 --create-channel"
        )
    state = collect_state(client, channel)
    plan = build_plan(config, args, state, channel)
    print_snapshot_summary(config, args.publish)
    for item in plan:
        print_action(item["category"], item["name"], item["action"])

    plan_document = {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "base_url": client.base_url,
        "snapshot": config["snapshot"],
        "channel_id": channel.get("id") if channel else None,
        "publish": bool(args.publish),
        "actions": plan,
    }
    if args.plan_output:
        write_private_json(args.plan_output, plan_document)
        print(f"计划文件：{args.plan_output.resolve()}")

    if not args.apply:
        print("预演完成，未写入。确认后增加 --apply 再运行。")
        return 0

    preflight_apply_secrets(config, args, state, channel)
    timestamp = datetime.now().strftime("%Y%m%d-%H%M%S-%f")
    backup_path = args.backup_dir / f"membership-seedance-{timestamp}.json"
    write_private_json(
        backup_path,
        {
            "created_at": datetime.now(timezone.utc).isoformat(),
            "base_url": client.base_url,
            "channel_id": channel.get("id") if channel else None,
            "state": state,
        },
    )
    print(f"应用前快照：{backup_path.resolve()}")

    _save_memberships(client, config)
    if channel is None:
        channel = create_channel(client, args.channel_name)
        print_action("Seedance 渠道", f"#{channel['id']} {channel['name']}", "create")
    provider = _save_provider(client, config, args)
    _save_channel_config_and_credentials(client, args, channel, provider)
    bases = _save_base_models(client, config)
    enhancements = _save_enhancement_models(client, config, int(provider["id"]))
    _save_offerings(
        client, config, args, int(channel["id"]), bases, enhancements
    )
    _archive_retired_base_models(client, config)
    _archive_retired_enhancement_models(client, config)
    verify_result(client, config, args, int(channel["id"]))
    print("初始化成功，写入后校验通过。重复执行将自动跳过一致项。")
    if not args.publish:
        print("本次未要求发布：新建售卖模型保持停用，已有模型保留原启停状态。")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except BootstrapFailure as exc:
        print(f"错误：{exc}", file=sys.stderr)
        raise SystemExit(1)
