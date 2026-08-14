import csv
import json
import math
import re
import statistics
from collections import Counter, defaultdict
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont


ARTIFACT_DIR = Path(__file__).resolve().parent
CHART_DIR = ARTIFACT_DIR / "charts"
CHART_DIR.mkdir(exist_ok=True)


def load_jsonl(name):
    path = ARTIFACT_DIR / name
    if not path.exists():
        return []
    rows = []
    for line in path.read_text(encoding="utf-8").splitlines():
        if line.strip():
            rows.append(json.loads(line))
    return rows


def percentile(values, quantile):
    values = sorted(values)
    if not values:
        return None
    if len(values) == 1:
        return values[0]
    position = (len(values) - 1) * quantile
    lower = math.floor(position)
    upper = math.ceil(position)
    if lower == upper:
        return values[lower]
    return values[lower] + (values[upper] - values[lower]) * (position - lower)


def metric_summary(values):
    values = [float(value) for value in values if value is not None and math.isfinite(float(value))]
    if not values:
        return {"count": 0, "min": None, "p50": None, "p95": None, "p99": None, "mean": None, "max": None}
    return {
        "count": len(values),
        "min": min(values),
        "p50": percentile(values, 0.50),
        "p95": percentile(values, 0.95),
        "p99": percentile(values, 0.99),
        "mean": statistics.fmean(values),
        "max": max(values),
    }


def parse_byte_value(value):
    match = re.match(r"\s*([0-9.]+)\s*([KMGT]?i?B)\s*$", value or "", re.I)
    if not match:
        return None
    number = float(match.group(1))
    unit = match.group(2).lower()
    multipliers = {
        "b": 1,
        "kb": 1_000,
        "mb": 1_000_000,
        "gb": 1_000_000_000,
        "tb": 1_000_000_000_000,
        "kib": 1024,
        "mib": 1024**2,
        "gib": 1024**3,
        "tib": 1024**4,
    }
    return number * multipliers[unit]


def analyze_tasks(summary):
    submissions = load_jsonl("submission-results.jsonl")
    polling = load_jsonl("polling-events.jsonl")
    poll_by_task = defaultdict(list)
    for event in polling:
        poll_by_task[event["task_id"]].append(event)
    rows = []
    for submission in sorted(submissions, key=lambda row: row["index"]):
        task_id = submission.get("task_id")
        events = sorted(poll_by_task.get(task_id, []), key=lambda row: row["captured_epoch"])
        terminal = next(
            (event for event in events if event.get("status") in {"completed", "succeeded", "success", "failed", "failure"}),
            None,
        )
        in_progress = next((event for event in events if event.get("status") == "in_progress"), None)
        queued = next((event for event in events if event.get("status") == "queued"), None)
        response_epoch = submission.get("response_epoch")
        request_epoch = submission.get("request_started_epoch")
        terminal_epoch = terminal.get("captured_epoch") if terminal else None
        in_progress_epoch = in_progress.get("captured_epoch") if in_progress else None
        queue_seconds = (
            max(0.0, in_progress_epoch - response_epoch)
            if in_progress_epoch is not None and response_epoch is not None
            else 0.0 if terminal_epoch is not None else None
        )
        generation_seconds = (
            max(0.0, terminal_epoch - in_progress_epoch)
            if terminal_epoch is not None and in_progress_epoch is not None
            else max(0.0, terminal_epoch - response_epoch)
            if terminal_epoch is not None and response_epoch is not None
            else None
        )
        total_seconds = (
            max(0.0, terminal_epoch - request_epoch)
            if terminal_epoch is not None and request_epoch is not None
            else None
        )
        rows.append(
            {
                "index": submission["index"],
                "task_id": task_id,
                "accepted": submission.get("accepted"),
                "http_status": submission.get("http_status"),
                "submission_latency_seconds": submission.get("latency_ms", 0) / 1000,
                "first_polled_status": events[0].get("status") if events else None,
                "queued_observed": queued is not None,
                "queue_seconds_approx": queue_seconds,
                "generation_seconds_approx": generation_seconds,
                "total_seconds": total_seconds,
                "generation_to_video_ratio": generation_seconds / summary["duration_seconds"] if generation_seconds is not None else None,
                "final_status": terminal.get("status") if terminal else None,
                "poll_events": len(events),
            }
        )

    csv_path = ARTIFACT_DIR / "task-performance.csv"
    with csv_path.open("w", encoding="utf-8-sig", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=list(rows[0].keys()))
        writer.writeheader()
        writer.writerows(rows)

    successful = [row for row in rows if row["final_status"] in {"completed", "succeeded", "success"}]
    start_epoch = min(row["request_started_epoch"] for row in submissions)
    terminal_epochs = []
    for submission in submissions:
        events = poll_by_task.get(submission.get("task_id"), [])
        terminal_epochs.extend(
            event["captured_epoch"]
            for event in events
            if event.get("status") in {"completed", "succeeded", "success", "failed", "failure"}
        )
    makespan_seconds = max(terminal_epochs) - start_epoch

    timeline = []
    if polling:
        poll_start = min(event["captured_epoch"] for event in polling)
        poll_end = max(event["captured_epoch"] for event in polling)
        task_event_lists = {
            task_id: sorted(events, key=lambda event: event["captured_epoch"])
            for task_id, events in poll_by_task.items()
        }
        tick = poll_start
        while tick <= poll_end + 0.001:
            counts = Counter()
            for task_id, events in task_event_lists.items():
                applicable = [event for event in events if event["captured_epoch"] <= tick + 2.5]
                status = applicable[-1].get("status") if applicable else "accepted"
                counts[status] += 1
            timeline.append({"elapsed_seconds": tick - poll_start, **counts})
            tick += 5

    return rows, timeline, {
        "concurrent_submission_attempts": len(submissions),
        "concurrent_submission_accepted": sum(1 for row in submissions if row.get("accepted")),
        "concurrent_terminal_success": len(successful),
        "concurrent_terminal_failure": len(rows) - len(successful),
        "submission_latency_seconds": metric_summary([row["submission_latency_seconds"] for row in rows]),
        "queue_seconds_approx": metric_summary([row["queue_seconds_approx"] for row in successful]),
        "generation_seconds_approx": metric_summary([row["generation_seconds_approx"] for row in successful]),
        "total_seconds": metric_summary([row["total_seconds"] for row in successful]),
        "generation_to_video_ratio": metric_summary([row["generation_to_video_ratio"] for row in successful]),
        "concurrent_makespan_seconds": makespan_seconds,
        "effective_tasks_per_minute": len(successful) / makespan_seconds * 60,
        "effective_output_video_seconds_per_wall_second": (
            len(successful) * summary["duration_seconds"] / makespan_seconds
        ),
        "max_observed_queued": max((row.get("queued", 0) for row in timeline), default=0),
        "max_observed_in_progress": max((row.get("in_progress", 0) for row in timeline), default=0),
        "polling_events": len(polling),
    }


def analyze_host(label, start_epoch, end_epoch):
    rows = [row for row in load_jsonl(f"server-metrics-{label}.jsonl") if "error" not in row]
    rows.sort(key=lambda row: row["remote_epoch"])
    derived = []
    for previous, current in zip(rows, rows[1:]):
        total_delta = current["cpu_total_ticks"] - previous["cpu_total_ticks"]
        idle_delta = current["cpu_idle_ticks"] - previous["cpu_idle_ticks"]
        cpu_percent = 100 * (1 - idle_delta / total_delta) if total_delta > 0 else None
        derived.append(
            {
                **current,
                "cpu_percent": cpu_percent,
                "memory_used_bytes": current["mem_total_bytes"] - current["mem_available_bytes"],
                "memory_used_percent": 100
                * (current["mem_total_bytes"] - current["mem_available_bytes"])
                / current["mem_total_bytes"],
            }
        )
    active = [row for row in derived if start_epoch <= row["remote_epoch"] <= end_epoch]
    baseline = [row for row in derived if start_epoch - 60 <= row["remote_epoch"] < start_epoch]
    if not active:
        raise RuntimeError(f"No active metric samples for {label}")
    first = active[0]
    last = active[-1]

    container_summary = {}
    for row in active:
        for container in row.get("docker", []):
            name = container.get("Name")
            if not name:
                continue
            entry = container_summary.setdefault(name, {"cpu_percent": [], "memory_bytes": [], "memory_percent": [], "pids": []})
            try:
                entry["cpu_percent"].append(float(container.get("CPUPerc", "0").rstrip("%")))
            except ValueError:
                pass
            usage = (container.get("MemUsage") or "").split("/")[0].strip()
            parsed_usage = parse_byte_value(usage)
            if parsed_usage is not None:
                entry["memory_bytes"].append(parsed_usage)
            try:
                entry["memory_percent"].append(float(container.get("MemPerc", "0").rstrip("%")))
            except ValueError:
                pass
            try:
                entry["pids"].append(int(container.get("PIDs", "0")))
            except ValueError:
                pass
    rendered_containers = {}
    for name, values in container_summary.items():
        rendered_containers[name] = {
            "cpu_percent": metric_summary(values["cpu_percent"]),
            "memory_bytes": metric_summary(values["memory_bytes"]),
            "memory_percent": metric_summary(values["memory_percent"]),
            "pids_max": max(values["pids"], default=None),
        }

    result = {
        "host_label": label,
        "host": active[0].get("host"),
        "sample_count_total": len(rows),
        "sample_count_active": len(active),
        "sample_errors": len(load_jsonl(f"server-metrics-{label}.jsonl")) - len(rows),
        "sample_latency_ms": metric_summary([row.get("sample_latency_ms") for row in active]),
        "cpu_percent": metric_summary([row.get("cpu_percent") for row in active]),
        "baseline_cpu_percent_mean": statistics.fmean([row["cpu_percent"] for row in baseline]) if baseline else None,
        "memory_used_bytes": metric_summary([row.get("memory_used_bytes") for row in active]),
        "memory_used_percent": metric_summary([row.get("memory_used_percent") for row in active]),
        "baseline_memory_used_percent_mean": statistics.fmean([row["memory_used_percent"] for row in baseline]) if baseline else None,
        "load_1m": metric_summary([row.get("load_1m") for row in active]),
        "baseline_load_1m_mean": statistics.fmean([row["load_1m"] for row in baseline]) if baseline else None,
        "load_5m": metric_summary([row.get("load_5m") for row in active]),
        "swap_used_bytes_max": max(row["swap_total_bytes"] - row["swap_free_bytes"] for row in active),
        "network_rx_bytes_delta": last["network_rx_bytes"] - first["network_rx_bytes"],
        "network_tx_bytes_delta": last["network_tx_bytes"] - first["network_tx_bytes"],
        "disk_read_bytes_delta": last["disk_read_bytes"] - first["disk_read_bytes"],
        "disk_write_bytes_delta": last["disk_write_bytes"] - first["disk_write_bytes"],
        "containers": rendered_containers,
    }
    chart_rows = [
        {
            "elapsed_seconds": row["remote_epoch"] - start_epoch,
            "cpu_percent": row["cpu_percent"],
            "memory_used_percent": row["memory_used_percent"],
        }
        for row in active
    ]
    return result, chart_rows


def font(size=24, bold=False):
    candidates = [
        "C:/Windows/Fonts/msyhbd.ttc" if bold else "C:/Windows/Fonts/msyh.ttc",
        "C:/Windows/Fonts/simhei.ttf",
        "C:/Windows/Fonts/arialbd.ttf" if bold else "C:/Windows/Fonts/arial.ttf",
        "C:/Windows/Fonts/segoeuib.ttf" if bold else "C:/Windows/Fonts/segoeui.ttf",
    ]
    for candidate in candidates:
        try:
            return ImageFont.truetype(candidate, size=size)
        except OSError:
            continue
    return ImageFont.load_default()


def draw_axes(draw, box, x_max, y_max, x_label, y_label):
    x0, y0, x1, y1 = box
    draw.line((x0, y1, x1, y1), fill="#64748b", width=2)
    draw.line((x0, y0, x0, y1), fill="#64748b", width=2)
    for fraction in (0, 0.25, 0.5, 0.75, 1):
        x = x0 + (x1 - x0) * fraction
        y = y1 - (y1 - y0) * fraction
        draw.line((x, y0, x, y1), fill="#e2e8f0", width=1)
        draw.line((x0, y, x1, y), fill="#e2e8f0", width=1)
        draw.text((x - 18, y1 + 8), f"{x_max*fraction:.0f}", fill="#475569", font=font(16))
        draw.text((x0 - 50, y - 10), f"{y_max*fraction:.0f}", fill="#475569", font=font(16))
    draw.text(((x0 + x1) / 2 - 50, y1 + 36), x_label, fill="#334155", font=font(18))
    draw.text((8, y0 - 2), y_label, fill="#334155", font=font(18))


def chart_task_latency(task_rows):
    width, height = 1500, 820
    image = Image.new("RGB", (width, height), "white")
    draw = ImageDraw.Draw(image)
    draw.text((60, 28), "Seedance 并发任务端到端耗时", fill="#0f172a", font=font(34, True))
    draw.text((60, 76), "49 个同时提交的 API 任务；输出规格为 480p / 4 秒", fill="#475569", font=font(20))
    box = (95, 130, 1440, 720)
    values = sorted((row["total_seconds"], row["index"]) for row in task_rows if row["total_seconds"] is not None)
    y_max = math.ceil(max(value for value, _ in values) / 30) * 30
    draw_axes(draw, box, len(values), y_max, "按耗时排序的任务", "耗时（秒）")
    x0, y0, x1, y1 = box
    points = []
    for position, (value, index) in enumerate(values, start=1):
        x = x0 + (x1 - x0) * position / len(values)
        y = y1 - (y1 - y0) * value / y_max
        points.append((x, y))
    draw.line(points, fill="#2563eb", width=4)
    for x, y in points:
        draw.ellipse((x - 3, y - 3, x + 3, y + 3), fill="#2563eb")
    p50 = percentile([value for value, _ in values], 0.5)
    p95 = percentile([value for value, _ in values], 0.95)
    for value, color, label in ((p50, "#16a34a", "P50 中位数"), (p95, "#dc2626", "P95")):
        y = y1 - (y1 - y0) * value / y_max
        draw.line((x0, y, x1, y), fill=color, width=3)
        draw.text((x1 - 150, y - 28), f"{label} {value:.1f}s", fill=color, font=font(18, True))
    image.save(CHART_DIR / "01-task-latency.png")


def chart_resources(host_rows):
    width, height = 1500, 920
    image = Image.new("RGB", (width, height), "white")
    draw = ImageDraw.Draw(image)
    draw.text((60, 28), "压测活动期双服务器资源利用率", fill="#0f172a", font=font(34, True))
    colors = {"newapi": "#2563eb", "upstream-aipdd": "#f97316"}
    for panel_index, metric in enumerate(("cpu_percent", "memory_used_percent")):
        top = 120 + panel_index * 370
        box = (95, top, 1440, top + 285)
        x_max = max(row["elapsed_seconds"] for rows in host_rows.values() for row in rows)
        y_max = max(100, math.ceil(max(row[metric] for rows in host_rows.values() for row in rows) / 10) * 10)
        draw_axes(draw, box, x_max, y_max, "压测开始后（秒）", "CPU 占用（%）" if metric == "cpu_percent" else "内存占用（%）")
        x0, y0, x1, y1 = box
        for label, rows in host_rows.items():
            points = [
                (
                    x0 + (x1 - x0) * row["elapsed_seconds"] / x_max,
                    y1 - (y1 - y0) * row[metric] / y_max,
                )
                for row in rows
            ]
            draw.line(points, fill=colors[label], width=3)
        draw.text((105, top + 10), "New API 服务器", fill=colors["newapi"], font=font(18, True))
        draw.text((285, top + 10), "上游 AIPDD", fill=colors["upstream-aipdd"], font=font(18, True))
    image.save(CHART_DIR / "02-server-resources.png")


def chart_states(timeline):
    width, height = 1500, 820
    image = Image.new("RGB", (width, height), "white")
    draw = ImageDraw.Draw(image)
    draw.text((60, 28), "任务状态变化时间线", fill="#0f172a", font=font(34, True))
    draw.text((60, 76), "基于每 5 秒一次的并发任务轮询统计", fill="#475569", font=font(20))
    box = (95, 130, 1440, 720)
    x_max = max(row["elapsed_seconds"] for row in timeline)
    draw_axes(draw, box, x_max, 49, "压测开始后（秒）", "任务数量")
    x0, y0, x1, y1 = box
    series = (("queued", "#eab308"), ("in_progress", "#2563eb"), ("completed", "#16a34a"))
    for name, color in series:
        points = [
            (
                x0 + (x1 - x0) * row["elapsed_seconds"] / x_max,
                y1 - (y1 - y0) * row.get(name, 0) / 49,
            )
            for row in timeline
        ]
        draw.line(points, fill=color, width=4)
    draw.text((105, 145), "排队中", fill="#eab308", font=font(18, True))
    draw.text((210, 145), "生成中", fill="#2563eb", font=font(18, True))
    draw.text((315, 145), "已完成", fill="#16a34a", font=font(18, True))
    image.save(CHART_DIR / "03-task-states.png")


def build_capacity_forecast(task_summary):
    measured_rate = task_summary["effective_tasks_per_minute"]
    conservative_rate = 6.5
    optimistic_rate = 9.0
    scenarios = {
        "保守情景": {"rate": conservative_rate, "floor_minutes": 170.19 / 60},
        "实测基准": {"rate": measured_rate, "floor_minutes": 142.47 / 60},
        "乐观情景": {"rate": optimistic_rate, "floor_minutes": 120 / 60},
    }
    rows = []
    for concurrency in range(10, 121, 5):
        row = {"concurrency": concurrency}
        for name, scenario in scenarios.items():
            row[name] = max(scenario["floor_minutes"], concurrency / scenario["rate"])
        rows.append(row)
    forecast = {
        "model": "饱和吞吐排队模型：批次完成时间 = max(生成时间下限, 并发任务数 / 完成吞吐)",
        "measured_throughput_tasks_per_minute": measured_rate,
        "estimated_service_slots": {
            "lower": 18,
            "upper": 22,
            "basis": "实测吞吐 × 生成阶段平均/P95耗时",
        },
        "recommended_peak_concurrency": 45,
        "predicted_max_concurrency": 65,
        "prediction_assumption": "将 10 分钟作为批次清空上限，并采用 6.5 任务/分钟的保守吞吐",
        "risk_boundary_concurrency": 100,
        "risk_boundary_minutes": {
            "conservative": 100 / conservative_rate,
            "baseline": 100 / measured_rate,
            "optimistic": 100 / optimistic_rate,
        },
        "slo_capacity": {
            "5_minutes": {"conservative": 32, "baseline": 39, "optimistic": 45},
            "6_minutes": {"conservative": 39, "baseline": 47, "optimistic": 54},
            "10_minutes": {"conservative": 65, "baseline": 78, "optimistic": 90},
        },
        "scenarios": scenarios,
    }
    return forecast, rows


def chart_capacity_forecast(forecast, rows):
    width, height = 1500, 860
    image = Image.new("RGB", (width, height), "white")
    draw = ImageDraw.Draw(image)
    draw.text((60, 28), "最大并发预测（按批次完成时限）", fill="#0f172a", font=font(34, True))
    draw.text((60, 76), "基于实测吞吐 7.81 任务/分钟；保守与乐观情景用于反映吞吐波动", fill="#475569", font=font(20))
    box = (105, 145, 1435, 740)
    x_max = 120
    y_max = 20
    draw_axes(draw, box, x_max, y_max, "同时提交的任务数", "批次完成时间（分钟）")
    colors = {"保守情景": "#dc2626", "实测基准": "#2563eb", "乐观情景": "#16a34a"}
    x0, y0, x1, y1 = box
    for name in ("保守情景", "实测基准", "乐观情景"):
        points = [
            (
                x0 + (x1 - x0) * row["concurrency"] / x_max,
                y1 - (y1 - y0) * row[name] / y_max,
            )
            for row in rows
        ]
        draw.line(points, fill=colors[name], width=4)
    for minutes, label in ((6, "6 分钟目标"), (10, "10 分钟上限")):
        y = y1 - (y1 - y0) * minutes / y_max
        draw.line((x0, y, x1, y), fill="#94a3b8", width=2)
        draw.text((x1 - 165, y - 27), label, fill="#475569", font=font(17, True))
    actual_x = x0 + (x1 - x0) * 49 / x_max
    actual_y = y1 - (y1 - y0) * (376.58 / 60) / y_max
    draw.ellipse((actual_x - 8, actual_y - 8, actual_x + 8, actual_y + 8), fill="#0f172a")
    draw.text((actual_x + 12, actual_y - 32), "实测：49 路 / 6.28 分钟", fill="#0f172a", font=font(18, True))
    max_x = x0 + (x1 - x0) * forecast["predicted_max_concurrency"] / x_max
    max_y = y1 - (y1 - y0) * 10 / y_max
    draw.ellipse((max_x - 8, max_y - 8, max_x + 8, max_y + 8), fill="#f97316")
    draw.text((max_x + 12, max_y + 10), "预测可用上限：65 路", fill="#c2410c", font=font(18, True))
    legend_x = 125
    for name in ("保守情景", "实测基准", "乐观情景"):
        draw.line((legend_x, 165, legend_x + 36, 165), fill=colors[name], width=4)
        draw.text((legend_x + 45, 151), name, fill="#334155", font=font(17, True))
        legend_x += 170
    image.save(CHART_DIR / "04-capacity-forecast.png")


def main():
    summary = json.loads((ARTIFACT_DIR / "loadtest-summary.json").read_text(encoding="utf-8"))
    task_rows, task_timeline, task_summary = analyze_tasks(summary)
    host_summaries = {}
    host_chart_rows = {}
    for label in ("newapi", "upstream-aipdd"):
        host_summary, chart_rows = analyze_host(label, summary["barrier_released_epoch"], summary["test_finished_epoch"])
        host_summaries[label] = host_summary
        host_chart_rows[label] = chart_rows

    with (ARTIFACT_DIR / "database-task-evidence.csv").open("r", encoding="utf-8-sig", newline="") as handle:
        db_tasks = list(csv.DictReader(handle))
    db_status_counts = Counter(row["status"] for row in db_tasks)
    batch_quota = sum(int(row["quota"]) for row in db_tasks)

    report_summary = {
        "test": {
            "batch_id": summary["batch_id"],
            "target": summary["base_url"],
            "model": summary["model"],
            "configuration": {
                "group": summary["group"],
                "resolution": summary["resolution"],
                "ratio": summary["ratio"],
                "duration_seconds": summary["duration_seconds"],
            },
            "browser_probe_tasks": 1,
            "simultaneous_api_tasks": 49,
            "total_tasks": 50,
        },
        "outcome": {
            "database_task_rows": len(db_tasks),
            "database_status_counts": dict(db_status_counts),
            "success_rate_percent": 100 * db_status_counts.get("SUCCESS", 0) / len(db_tasks),
            "request_count_before": 35,
            "request_count_after": 85,
            "balance_cny_before": 1359.35,
            "balance_cny_after": 1295.35,
            "actual_cost_cny": 64.00,
            "average_cost_cny_per_task": 64.00 / 50,
            "batch_quota": batch_quota,
        },
        "performance": task_summary,
        "servers": host_summaries,
        "limitations": [
            "The browser playground serializes one video per conversation, so the batch consists of one browser probe plus 49 requests released simultaneously through the same public /v1/videos path.",
            "Queue and generation phase boundaries are approximated from 5-second task polling.",
            "Server metrics cover gateway hosts and containers; GPU workers behind the upstream platform were not exposed on the supplied hosts.",
        ],
    }
    capacity_forecast, capacity_rows = build_capacity_forecast(task_summary)
    report_summary["capacity_forecast"] = capacity_forecast
    (ARTIFACT_DIR / "analysis-summary.json").write_text(
        json.dumps(report_summary, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    (ARTIFACT_DIR / "task-state-timeline.json").write_text(
        json.dumps(task_timeline, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    (ARTIFACT_DIR / "server-analysis.json").write_text(
        json.dumps(host_summaries, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    (ARTIFACT_DIR / "capacity-forecast.json").write_text(
        json.dumps(capacity_forecast, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    with (ARTIFACT_DIR / "capacity-forecast.csv").open("w", encoding="utf-8-sig", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=list(capacity_rows[0].keys()))
        writer.writeheader()
        writer.writerows(capacity_rows)

    chart_task_latency(task_rows)
    chart_resources(host_chart_rows)
    chart_states(task_timeline)
    chart_capacity_forecast(capacity_forecast, capacity_rows)
    print(json.dumps(report_summary, ensure_ascii=False))


if __name__ == "__main__":
    main()
