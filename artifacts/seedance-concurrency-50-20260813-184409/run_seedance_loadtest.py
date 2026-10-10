import concurrent.futures
import json
import statistics
import threading
import time
from datetime import datetime, timezone
from pathlib import Path

import paramiko
import requests


ARTIFACT_DIR = Path(__file__).resolve().parent
BASE_URL = "https://susciyuan.com"
MODEL = "AP Seedance-2.0 轻量版"
GROUP = "default"
RESOLUTION = "480p"
RATIO = "16:9"
DURATION_SECONDS = 4
CONCURRENT_NEW_REQUESTS = 49
WARMUP_TASK_ID = "task_LRZF8U9egvAYJeup7qOctFmavuQahfWg"
WARMUP_INDEX = 1
TOKEN_ID = 21
POLL_INTERVAL_SECONDS = 5
POLL_TIMEOUT_SECONDS = 1200
PROMPT_BASE = (
    "Minimal abstract scene: a small white sphere slowly drifting across a plain "
    "black background, stable camera, simple lighting. Load test batch 20260813-184409"
)


def utc_now():
    return datetime.now(timezone.utc).isoformat()


def append_jsonl(path, payload, lock):
    line = json.dumps(payload, ensure_ascii=False, separators=(",", ":"))
    with lock:
        with path.open("a", encoding="utf-8", newline="\n") as handle:
            handle.write(line + "\n")


def get_api_token():
    sql = f"SELECT key FROM tokens WHERE id={TOKEN_ID} AND status=1 AND deleted_at IS NULL LIMIT 1;"
    remote_command = (
        "docker exec new-api-postgres sh -lc "
        "'psql -U \"$POSTGRES_USER\" -d \"$POSTGRES_DB\" -At -c \""
        + sql
        + "\"'"
    )
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect(
        "14.103.100.4",
        username="root",
        key_filename=r"D:\Documents\zhuimi.pem",
        timeout=10,
        banner_timeout=10,
        auth_timeout=10,
    )
    _, stdout, stderr = client.exec_command(remote_command, timeout=15)
    token = stdout.read().decode("utf-8", "replace").strip()
    error_text = stderr.read().decode("utf-8", "replace").strip()
    client.close()
    if not token:
        raise RuntimeError(f"Unable to retrieve token id {TOKEN_ID}: {error_text}")
    return token


def build_headers(token):
    rendered = token if token.startswith("sk-") else f"sk-{token}"
    return {
        "Authorization": f"Bearer {rendered}",
        "Content-Type": "application/json",
        "User-Agent": "seedance-loadtest-20260813-184409/1.0",
    }


def preflight(headers):
    started = time.perf_counter()
    response = requests.get(f"{BASE_URL}/v1/models", headers=headers, timeout=30)
    elapsed_ms = round((time.perf_counter() - started) * 1000, 3)
    response.raise_for_status()
    payload = response.json()
    model_ids = {
        item.get("id")
        for item in payload.get("data", [])
        if isinstance(item, dict) and isinstance(item.get("id"), str)
    }
    if MODEL not in model_ids:
        raise RuntimeError(f"Model unavailable to token {TOKEN_ID}: {MODEL}")
    return {
        "captured_at_utc": utc_now(),
        "latency_ms": elapsed_ms,
        "model_found": True,
        "model_count": len(model_ids),
    }


def build_payload(index):
    prompt = f"{PROMPT_BASE} #{index:02d}"
    content = [{"type": "text", "text": prompt}]
    return {
        "model": MODEL,
        "group": GROUP,
        "prompt": prompt,
        "duration": DURATION_SECONDS,
        "seconds": str(DURATION_SECONDS),
        "size": "1280x720",
        "content": content,
        "ratio": RATIO,
        "resolution": RESOLUTION,
        "metadata": {
            "content": content,
            "ratio": RATIO,
            "resolution": RESOLUTION,
        },
    }


def extract_task_id(payload):
    if not isinstance(payload, dict):
        return None
    candidates = [payload]
    for key in ("data", "task"):
        value = payload.get(key)
        if isinstance(value, dict):
            candidates.append(value)
    for candidate in candidates:
        for key in ("task_id", "id"):
            value = candidate.get(key)
            if isinstance(value, str) and value.strip():
                return value.strip()
    return None


def normalize_status(payload):
    if not isinstance(payload, dict):
        return "unknown"
    candidates = [payload]
    for key in ("data", "task"):
        value = payload.get(key)
        if isinstance(value, dict):
            candidates.append(value)
    for candidate in candidates:
        for key in ("status", "state"):
            value = candidate.get(key)
            if isinstance(value, str) and value.strip():
                return value.strip().lower()
    return "unknown"


def submit_one(index, headers, barrier, result_path, write_lock):
    payload = build_payload(index)
    barrier.wait()
    request_started_epoch = time.time()
    request_started = time.perf_counter()
    record = {
        "index": index,
        "prompt": payload["prompt"],
        "request_started_at_utc": utc_now(),
        "request_started_epoch": request_started_epoch,
    }
    try:
        response = requests.post(
            f"{BASE_URL}/v1/videos",
            headers=headers,
            json=payload,
            timeout=90,
        )
        record["response_at_utc"] = utc_now()
        record["response_epoch"] = time.time()
        record["latency_ms"] = round((time.perf_counter() - request_started) * 1000, 3)
        record["http_status"] = response.status_code
        try:
            body = response.json()
        except ValueError:
            body = {"raw_text": response.text[:2000]}
        record["response"] = body
        record["task_id"] = extract_task_id(body)
        record["accepted"] = response.ok and bool(record["task_id"])
    except Exception as exc:
        record.update(
            {
                "response_at_utc": utc_now(),
                "response_epoch": time.time(),
                "latency_ms": round((time.perf_counter() - request_started) * 1000, 3),
                "accepted": False,
                "error": repr(exc),
            }
        )
    append_jsonl(result_path, record, write_lock)
    return record


def poll_one(task_record, headers):
    task_id = task_record["task_id"]
    started = time.perf_counter()
    try:
        response = requests.get(
            f"{BASE_URL}/v1/videos/{task_id}",
            headers=headers,
            timeout=30,
        )
        try:
            body = response.json()
        except ValueError:
            body = {"raw_text": response.text[:2000]}
        return {
            "index": task_record["index"],
            "task_id": task_id,
            "captured_at_utc": utc_now(),
            "captured_epoch": time.time(),
            "latency_ms": round((time.perf_counter() - started) * 1000, 3),
            "http_status": response.status_code,
            "status": normalize_status(body),
            "response": body,
        }
    except Exception as exc:
        return {
            "index": task_record["index"],
            "task_id": task_id,
            "captured_at_utc": utc_now(),
            "captured_epoch": time.time(),
            "latency_ms": round((time.perf_counter() - started) * 1000, 3),
            "status": "poll_error",
            "error": repr(exc),
        }


def main():
    submission_path = ARTIFACT_DIR / "submission-results.jsonl"
    polling_path = ARTIFACT_DIR / "polling-events.jsonl"
    summary_path = ARTIFACT_DIR / "loadtest-summary.json"
    write_lock = threading.Lock()

    token = get_api_token()
    headers = build_headers(token)
    preflight_result = preflight(headers)
    test_started_epoch = time.time()
    indices = list(range(2, 51))
    barrier = threading.Barrier(len(indices) + 1)

    with concurrent.futures.ThreadPoolExecutor(max_workers=len(indices)) as executor:
        futures = [
            executor.submit(
                submit_one,
                index,
                headers,
                barrier,
                submission_path,
                write_lock,
            )
            for index in indices
        ]
        barrier_released_at_utc = utc_now()
        barrier_released_epoch = time.time()
        barrier.wait()
        submissions = [future.result() for future in futures]

    accepted = [record for record in submissions if record.get("accepted")]
    terminal_statuses = {
        "completed",
        "succeeded",
        "success",
        "failed",
        "failure",
        "cancelled",
        "canceled",
    }
    final_events = {}
    poll_started_epoch = time.time()

    while accepted and time.time() - poll_started_epoch < POLL_TIMEOUT_SECONDS:
        pending = [record for record in accepted if record["task_id"] not in final_events]
        if not pending:
            break
        with concurrent.futures.ThreadPoolExecutor(max_workers=min(32, len(pending))) as executor:
            events = list(executor.map(lambda row: poll_one(row, headers), pending))
        for event in events:
            append_jsonl(polling_path, event, write_lock)
            if event.get("status") in terminal_statuses:
                final_events[event["task_id"]] = event
        if len(final_events) < len(accepted):
            time.sleep(POLL_INTERVAL_SECONDS)

    latencies = [record["latency_ms"] for record in submissions if "latency_ms" in record]
    accepted_start_epochs = [record["request_started_epoch"] for record in accepted]
    accepted_response_epochs = [record["response_epoch"] for record in accepted]
    summary = {
        "batch_id": "20260813-184409",
        "created_at_utc": utc_now(),
        "base_url": BASE_URL,
        "model": MODEL,
        "group": GROUP,
        "resolution": RESOLUTION,
        "ratio": RATIO,
        "duration_seconds": DURATION_SECONDS,
        "warmup_task": {"index": WARMUP_INDEX, "task_id": WARMUP_TASK_ID},
        "concurrent_new_requests": CONCURRENT_NEW_REQUESTS,
        "total_batch_tasks": CONCURRENT_NEW_REQUESTS + 1,
        "preflight": preflight_result,
        "test_started_epoch": test_started_epoch,
        "barrier_released_at_utc": barrier_released_at_utc,
        "barrier_released_epoch": barrier_released_epoch,
        "submission_attempts": len(submissions),
        "submission_accepted": len(accepted),
        "submission_rejected": len(submissions) - len(accepted),
        "submission_window_seconds": (
            max(accepted_response_epochs) - min(accepted_start_epochs)
            if accepted_start_epochs and accepted_response_epochs
            else None
        ),
        "submission_latency_ms": {
            "min": min(latencies) if latencies else None,
            "max": max(latencies) if latencies else None,
            "mean": statistics.fmean(latencies) if latencies else None,
            "median": statistics.median(latencies) if latencies else None,
        },
        "terminal_observed": len(final_events),
        "terminal_status_counts": {
            status: sum(1 for event in final_events.values() if event.get("status") == status)
            for status in sorted({event.get("status") for event in final_events.values()})
        },
        "poll_timeout_seconds": POLL_TIMEOUT_SECONDS,
        "test_finished_epoch": time.time(),
    }
    summary_path.write_text(
        json.dumps(summary, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    print(json.dumps(summary, ensure_ascii=False))


if __name__ == "__main__":
    main()
