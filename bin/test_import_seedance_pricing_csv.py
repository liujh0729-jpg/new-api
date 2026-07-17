from __future__ import annotations

import importlib.util
import json
import tempfile
import unittest
from decimal import Decimal
from pathlib import Path


SCRIPT_PATH = Path(__file__).with_name("import-seedance-pricing-csv.py")
SPEC = importlib.util.spec_from_file_location("import_seedance_pricing_csv", SCRIPT_PATH)
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


HEADERS = [
    "平台模型",
    "输出规格",
    "能力类型",
    "计费单位",
    "对比原生价",
    "VIP1",
    "VIP2",
    "VIP3",
    "VIP4",
    "VIP5",
]


def row(resolution: str, kind: str, native: str, ratios: list[str]) -> dict[str, str]:
    values = ["AP Seedance", resolution, kind, "条/5秒", native, *ratios]
    return dict(zip(HEADERS, values, strict=True))


class ImportSeedancePricingCSVTest(unittest.TestCase):
    def test_builds_native_matrix_and_exempts_all_one_resolution(self) -> None:
        rows = [
            row("480P", "输入含视频+平超", "4", ["1", "1", "1", "1", "1"]),
            row("480P", "不含视频+平超", "3", ["1", "1", "1", "1", "1"]),
            row("720p", "输入含视频 超一档", "6.7", [".78", ".8", ".85", ".9", ".95"]),
            row("720p", "不含视频 超一档", "4.97", [".78", ".8", ".85", ".9", ".95"]),
        ]

        pricing, summary = MODULE.build_task_pricing(rows, Decimal("7.3"))

        tiers = pricing["AP Seedance"]["by_resolution"]
        self.assertEqual("none", tiers["480p"]["group_ratio_policy"])
        self.assertNotIn("group_ratio_policy", tiers["720p"])
        self.assertAlmostEqual(3 / 5 / 7.3, tiers["480p"]["no_reference_video_unit_price"])
        self.assertAlmostEqual(4 / 5 / 7.3, tiers["480p"]["reference_video_unit_price"])
        self.assertEqual(["AP Seedance/480p"], summary["exempt_resolutions"])

    def test_rejects_nonstandard_group_ratios(self) -> None:
        rows = [
            row("1080p", "输入含视频", "10.5", [".75", ".75", ".8", ".8", ".85"]),
            row("1080p", "不含视频", "8", [".75", ".75", ".8", ".8", ".85"]),
        ]

        with self.assertRaisesRegex(MODULE.ImportFailure, "第 2 行.*严格为 VIP1=0.78"):
            MODULE.build_task_pricing(rows, Decimal("7.3"))

    def test_plan_creates_fixed_groups_and_preserves_unrelated_entries(self) -> None:
        pricing = {
            "AP Seedance": {
                "unit": "second",
                "by_resolution": {
                    "480p": {
                        "no_reference_video_unit_price": 1,
                        "reference_video_policy": "same",
                        "group_ratio_policy": "none",
                    }
                },
            }
        }
        options = {
            "billing_setting.task_pricing": json.dumps({"other": {"keep": True}}),
            "billing_setting.billing_mode": json.dumps({"other": "ratio"}),
            "GroupRatio": json.dumps({"default": 1}),
            "UserUsableGroups": json.dumps({"default": "默认分组"}),
        }

        plan = MODULE.build_plan(
            options,
            pricing,
            {
                "models": ["AP Seedance"],
                "resolution_tiers": 1,
                "source_rows": 2,
                "exempt_resolutions": ["AP Seedance/480p"],
            },
        )
        updates = {item["key"]: json.loads(item["value"]) for item in plan["updates"]}

        self.assertTrue(updates["billing_setting.task_pricing"]["other"]["keep"])
        self.assertEqual("task_pricing", updates["billing_setting.billing_mode"]["AP Seedance"])
        self.assertEqual(
            {"VIP1": 0.78, "VIP2": 0.8, "VIP3": 0.85, "VIP4": 0.9, "VIP5": 0.95},
            {key: updates["GroupRatio"][key] for key in MODULE.GROUPS},
        )
        self.assertEqual("默认分组", updates["UserUsableGroups"]["default"])

    def test_rejects_mismatched_exemption_between_video_variants(self) -> None:
        rows = [
            row("480p", "输入含视频", "4", ["1", "1", "1", "1", "1"]),
            row("480p", "不含视频", "3", [".78", ".8", ".85", ".9", ".95"]),
        ]

        with self.assertRaisesRegex(MODULE.ImportFailure, "分组豁免定义不一致"):
            MODULE.build_task_pricing(rows, Decimal("7.3"))

    def test_reads_utf8_bom_csv(self) -> None:
        content = ",".join(HEADERS) + "\n" + ",".join(
            ["AP Seedance", "480p", "不含视频", "条/5秒", "3", "1", "1", "1", "1", "1"]
        )
        with tempfile.TemporaryDirectory() as temp_dir:
            path = Path(temp_dir) / "pricing.csv"
            path.write_text("\ufeff" + content, encoding="utf-8")
            rows = MODULE.read_csv_rows(path)

        self.assertEqual("平台模型", next(iter(rows[0])))
        self.assertEqual("AP Seedance", rows[0]["平台模型"])

    def test_reads_gbk_compatible_csv(self) -> None:
        content = ",".join(HEADERS) + "\n" + ",".join(
            ["AP Seedance", "480p", "不含视频", "条/5秒", "3", "1", "1", "1", "1", "1"]
        )
        with tempfile.TemporaryDirectory() as temp_dir:
            path = Path(temp_dir) / "pricing-gbk.csv"
            path.write_bytes(content.encode("gb18030"))
            rows = MODULE.read_csv_rows(path)

        self.assertEqual("AP Seedance", rows[0]["平台模型"])

    def test_rejects_legacy_discount_headers(self) -> None:
        legacy_headers = [*HEADERS[:5], "78档", "80档", "85档", "90档", "95档"]
        content = ",".join(legacy_headers) + "\n" + ",".join(
            ["AP Seedance", "480p", "不含视频", "条/5秒", "3", "1", "1", "1", "1", "1"]
        )
        with tempfile.TemporaryDirectory() as temp_dir:
            path = Path(temp_dir) / "pricing-legacy.csv"
            path.write_text(content, encoding="utf-8")
            with self.assertRaisesRegex(MODULE.ImportFailure, "CSV 缺少列.*VIP1"):
                MODULE.read_csv_rows(path)

if __name__ == "__main__":
    unittest.main()
