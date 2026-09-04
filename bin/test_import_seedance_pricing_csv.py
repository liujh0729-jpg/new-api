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


HEADERS = ["平台模型", "输出规格", "能力类型", "计费单位", "对比原生价"]


def row(resolution: str, kind: str, native: str) -> dict[str, str]:
    values = ["AP Seedance", resolution, kind, "条/5秒", native]
    return dict(zip(HEADERS, values, strict=True))


class ImportSeedancePricingCSVTest(unittest.TestCase):
    def test_imports_base_prices_without_group_policy(self) -> None:
        rows = [
            row("480P", "输入含视频+平超", "4"),
            row("480P", "不含视频+平超", "3"),
            row("720p", "输入含视频 超一档", "6.7"),
            row("720p", "不含视频 超一档", "4.97"),
        ]

        pricing, summary = MODULE.build_task_pricing(rows, Decimal("7.3"))

        tiers = pricing["AP Seedance"]["by_resolution"]
        self.assertNotIn("group_ratio_policy", tiers["480p"])
        self.assertNotIn("group_ratio_policy", tiers["720p"])
        self.assertAlmostEqual(3 / 7.3, tiers["480p"]["no_reference_video_unit_price"])
        self.assertAlmostEqual(4 / 7.3, tiers["480p"]["reference_video_unit_price"])
        self.assertEqual(2, summary["resolution_tiers"])

    def test_billing_unit_does_not_scale_per_second_prices(self) -> None:
        one_second_rows = [
            row("720p", "输入含视频", "6"),
            row("720p", "不含视频", "4"),
        ]
        five_second_rows = [dict(item) for item in one_second_rows]
        for item in one_second_rows:
            item["计费单位"] = "条/1秒"
        for item in five_second_rows:
            item["计费单位"] = "条/5秒"

        one_second, _ = MODULE.build_task_pricing(one_second_rows, Decimal("2"))
        five_seconds, _ = MODULE.build_task_pricing(five_second_rows, Decimal("2"))

        self.assertEqual(one_second, five_seconds)
        tier = one_second["AP Seedance"]["by_resolution"]["720p"]
        self.assertEqual(2, tier["no_reference_video_unit_price"])
        self.assertEqual(3, tier["reference_video_unit_price"])

    def test_plan_only_updates_task_pricing_and_billing_mode(self) -> None:
        pricing = {
            "AP Seedance": {
                "unit": "second",
                "by_resolution": {
                    "480p": {
                        "no_reference_video_unit_price": 1,
                        "reference_video_policy": "same",
                    }
                },
            }
        }
        options = {
            "billing_setting.task_pricing": json.dumps({"other": {"keep": True}}),
            "billing_setting.billing_mode": json.dumps({"other": "ratio"}),
            "GroupRatio": json.dumps({"default": 1, "custom": 0.7}),
            "UserUsableGroups": json.dumps({"default": "默认分组"}),
        }

        plan = MODULE.build_plan(
            options,
            pricing,
            {"models": ["AP Seedance"], "resolution_tiers": 1, "source_rows": 2},
        )
        updates = {item["key"]: json.loads(item["value"]) for item in plan["updates"]}

        self.assertEqual(
            {"billing_setting.task_pricing", "billing_setting.billing_mode"},
            set(updates),
        )
        self.assertTrue(updates["billing_setting.task_pricing"]["other"]["keep"])
        self.assertEqual(
            "task_pricing", updates["billing_setting.billing_mode"]["AP Seedance"]
        )

    def test_reads_legacy_extra_columns_but_ignores_them(self) -> None:
        headers = [*HEADERS, "VIP1", "VIP2"]
        content = ",".join(headers) + "\n" + ",".join(
            ["AP Seedance", "480p", "不含视频", "条/5秒", "3", "0.1", "0.2"]
        )
        with tempfile.TemporaryDirectory() as temp_dir:
            path = Path(temp_dir) / "pricing.csv"
            path.write_text("\ufeff" + content, encoding="utf-8")
            rows = MODULE.read_csv_rows(path)

        self.assertEqual("AP Seedance", rows[0]["平台模型"])

    def test_reads_gbk_compatible_csv(self) -> None:
        content = ",".join(HEADERS) + "\n" + ",".join(
            ["AP Seedance", "480p", "不含视频", "条/5秒", "3"]
        )
        with tempfile.TemporaryDirectory() as temp_dir:
            path = Path(temp_dir) / "pricing-gbk.csv"
            path.write_bytes(content.encode("gb18030"))
            rows = MODULE.read_csv_rows(path)

        self.assertEqual("AP Seedance", rows[0]["平台模型"])

    def test_rejects_missing_base_price_column(self) -> None:
        headers = HEADERS[:-1]
        content = ",".join(headers) + "\n" + ",".join(
            ["AP Seedance", "480p", "不含视频", "条/5秒"]
        )
        with tempfile.TemporaryDirectory() as temp_dir:
            path = Path(temp_dir) / "pricing-invalid.csv"
            path.write_text(content, encoding="utf-8")
            with self.assertRaisesRegex(MODULE.ImportFailure, "CSV 缺少列.*对比原生价"):
                MODULE.read_csv_rows(path)


if __name__ == "__main__":
    unittest.main()
