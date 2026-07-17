from __future__ import annotations

import importlib.util
import json
import unittest
from pathlib import Path


SCRIPT_PATH = Path(__file__).with_name("build_vip_group_sync_plan.py")
SPEC = importlib.util.spec_from_file_location("build_vip_group_sync_plan", SCRIPT_PATH)
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def channel(channel_id: int, group: str, *, channel_type: int = 58) -> dict:
    return {
        "id": channel_id,
        "name": f"AIPDD-{channel_id}",
        "type": channel_type,
        "group": group,
        "models": "AP Seedance",
    }


class BuildVIPGroupSyncPlanTest(unittest.TestCase):
    def test_merges_fixed_groups_and_preserves_unrelated_values(self) -> None:
        group_ratio_raw = '{"default":1,"custom":1.2,"VIP1":0.5}'
        usable_raw = '{"default":"默认分组","VIP1":"保留现有说明"}'
        plan = MODULE.build_plan(
            {
                "data": [
                    {"key": "GroupRatio", "value": group_ratio_raw},
                    {"key": "UserUsableGroups", "value": usable_raw},
                    {"key": "ModelPrice", "value": '{"must":"not-change"}'},
                ]
            },
            {
                "data": {
                    "items": [
                        channel(1, "default,VIP1,default"),
                        channel(2, "custom"),
                    ]
                }
            },
        )

        updates = {
            item["key"]: json.loads(item["value"])
            for item in plan["option_updates"]
        }
        self.assertEqual(1, updates["GroupRatio"]["default"])
        self.assertEqual(1.2, updates["GroupRatio"]["custom"])
        self.assertEqual(
            {"VIP1": 0.78, "VIP2": 0.8, "VIP3": 0.85, "VIP4": 0.9, "VIP5": 0.95},
            {name: updates["GroupRatio"][name] for name in MODULE.VIP_GROUPS},
        )
        self.assertEqual("保留现有说明", updates["UserUsableGroups"]["VIP1"])
        self.assertEqual("VIP2（80档）", updates["UserUsableGroups"]["VIP2"])
        self.assertEqual(
            [
                {
                    "id": 1,
                    "name": "AIPDD-1",
                    "type": 58,
                    "previous_group": "default,VIP1,default",
                    "group": "default,VIP1,VIP2,VIP3,VIP4,VIP5",
                },
                {
                    "id": 2,
                    "name": "AIPDD-2",
                    "type": 58,
                    "previous_group": "custom",
                    "group": "custom,VIP1,VIP2,VIP3,VIP4,VIP5",
                },
            ],
            plan["channel_updates"],
        )
        self.assertEqual(
            ["UserUsableGroups", "GroupRatio"],
            [item["key"] for item in plan["option_rollback"]],
        )
        self.assertEqual(
            usable_raw,
            plan["option_rollback"][0]["value"],
        )
        self.assertEqual(
            group_ratio_raw,
            plan["option_rollback"][1]["value"],
        )
        self.assertEqual(
            group_ratio_raw,
            plan["option_updates"][0]["previous_value"],
        )
        self.assertEqual(
            usable_raw,
            plan["option_updates"][1]["previous_value"],
        )
        self.assertNotIn("ModelPrice", json.dumps(plan, ensure_ascii=False))
        self.assertIn("preserve unrelated", plan["summary"]["contract"])

    def test_already_synchronized_input_produces_no_writes(self) -> None:
        plan = MODULE.build_plan(
            {
                "GroupRatio": json.dumps(dict(MODULE.VIP_GROUPS)),
                "UserUsableGroups": json.dumps(MODULE.VIP_DESCRIPTIONS),
            },
            [
                channel(1, "default,VIP1,VIP2,VIP3,VIP4,VIP5"),
            ],
        )

        self.assertEqual([], plan["option_updates"])
        self.assertEqual([], plan["option_rollback"])
        self.assertEqual([], plan["channel_updates"])
        self.assertEqual([], plan["channel_rollback"])

    def test_rejects_non_aipdd_or_duplicate_channels(self) -> None:
        with self.assertRaisesRegex(MODULE.PlanError, "not an AIPDD"):
            MODULE.build_plan({}, [channel(1, "default", channel_type=1)])
        with self.assertRaisesRegex(MODULE.PlanError, "duplicate channel id"):
            MODULE.build_plan({}, [channel(1, "default"), channel(1, "vip")])

    def test_rejects_missing_aipdd_channels(self) -> None:
        with self.assertRaisesRegex(MODULE.PlanError, "at least one AIPDD"):
            MODULE.build_plan({}, [])

    def test_rejects_invalid_existing_options(self) -> None:
        with self.assertRaisesRegex(MODULE.PlanError, "finite and non-negative"):
            MODULE.build_plan(
                {"GroupRatio": '{"broken":-1}'},
                [channel(1, "default")],
            )
        with self.assertRaisesRegex(MODULE.PlanError, "description must be a string"):
            MODULE.build_plan(
                {"UserUsableGroups": '{"broken":7}'},
                [channel(1, "default")],
            )

    def test_rejects_channel_group_that_would_exceed_database_limit(self) -> None:
        with self.assertRaisesRegex(MODULE.PlanError, "exceeds 64"):
            MODULE.build_plan(
                {},
                [channel(1, "x" * 50)],
            )


if __name__ == "__main__":
    unittest.main()
