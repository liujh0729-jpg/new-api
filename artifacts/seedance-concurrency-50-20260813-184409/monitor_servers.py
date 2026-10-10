import concurrent.futures
import json
import os
import sys
import time
from datetime import datetime, timezone
from pathlib import Path

import paramiko


ARTIFACT_DIR = Path(__file__).resolve().parent
STOP_FILE = ARTIFACT_DIR / "STOP_MONITORING"
INTERVAL_SECONDS = 2.0
MAX_DURATION_SECONDS = 1800

REMOTE_SAMPLE = r'''python3 - <<'PY'
import json
import os
import subprocess
import time

def read_text(path):
    with open(path, "r", encoding="utf-8", errors="replace") as handle:
        return handle.read()

cpu_fields = read_text("/proc/stat").splitlines()[0].split()
mem = {}
for line in read_text("/proc/meminfo").splitlines():
    if ":" not in line:
        continue
    key, value = line.split(":", 1)
    parts = value.strip().split()
    if parts:
        mem[key] = int(parts[0]) * 1024

rx_bytes = 0
tx_bytes = 0
for line in read_text("/proc/net/dev").splitlines()[2:]:
    if ":" not in line:
        continue
    iface, values = line.split(":", 1)
    if iface.strip() == "lo":
        continue
    fields = values.split()
    rx_bytes += int(fields[0])
    tx_bytes += int(fields[8])

read_sectors = 0
write_sectors = 0
for line in read_text("/proc/diskstats").splitlines():
    fields = line.split()
    if len(fields) < 14:
        continue
    device = fields[2]
    if device.startswith(("loop", "ram", "dm-")) or device[-1:].isdigit():
        continue
    read_sectors += int(fields[5])
    write_sectors += int(fields[9])

docker_rows = []
try:
    result = subprocess.run(
        ["docker", "stats", "--no-stream", "--format", "{{json .}}"],
        check=False,
        capture_output=True,
        text=True,
        timeout=5,
    )
    for line in result.stdout.splitlines():
        try:
            docker_rows.append(json.loads(line))
        except json.JSONDecodeError:
            docker_rows.append({"raw": line})
except Exception as exc:
    docker_rows = [{"error": repr(exc)}]

load_fields = read_text("/proc/loadavg").split()
payload = {
    "remote_epoch": time.time(),
    "cpu_total_ticks": sum(int(value) for value in cpu_fields[1:]),
    "cpu_idle_ticks": int(cpu_fields[4]) + int(cpu_fields[5]),
    "mem_total_bytes": mem.get("MemTotal", 0),
    "mem_available_bytes": mem.get("MemAvailable", 0),
    "swap_total_bytes": mem.get("SwapTotal", 0),
    "swap_free_bytes": mem.get("SwapFree", 0),
    "load_1m": float(load_fields[0]),
    "load_5m": float(load_fields[1]),
    "load_15m": float(load_fields[2]),
    "runnable_processes": load_fields[3],
    "network_rx_bytes": rx_bytes,
    "network_tx_bytes": tx_bytes,
    "disk_read_bytes": read_sectors * 512,
    "disk_write_bytes": write_sectors * 512,
    "docker": docker_rows,
}
print(json.dumps(payload, ensure_ascii=False, separators=(",", ":")))
PY'''


class HostMonitor:
    def __init__(self, label, host, username, key_filename=None, password=None):
        self.label = label
        self.host = host
        self.username = username
        self.key_filename = key_filename
        self.password = password
        self.client = None
        self.output_path = ARTIFACT_DIR / f"server-metrics-{label}.jsonl"

    def connect(self):
        if self.client is not None:
            self.client.close()
        client = paramiko.SSHClient()
        client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        client.connect(
            self.host,
            username=self.username,
            key_filename=self.key_filename,
            password=self.password,
            timeout=10,
            banner_timeout=10,
            auth_timeout=10,
        )
        self.client = client

    def sample(self):
        started = time.perf_counter()
        try:
            if self.client is None:
                self.connect()
            _, stdout, stderr = self.client.exec_command(REMOTE_SAMPLE, timeout=10)
            raw = stdout.read().decode("utf-8", "replace").strip()
            error_text = stderr.read().decode("utf-8", "replace").strip()
            if not raw:
                raise RuntimeError(error_text or "empty remote sample")
            payload = json.loads(raw.splitlines()[-1])
            payload.update(
                {
                    "host_label": self.label,
                    "host": self.host,
                    "captured_at_utc": datetime.now(timezone.utc).isoformat(),
                    "sample_latency_ms": round((time.perf_counter() - started) * 1000, 3),
                }
            )
        except Exception as exc:
            payload = {
                "host_label": self.label,
                "host": self.host,
                "captured_at_utc": datetime.now(timezone.utc).isoformat(),
                "sample_latency_ms": round((time.perf_counter() - started) * 1000, 3),
                "error": repr(exc),
            }
            try:
                self.connect()
            except Exception as reconnect_exc:
                payload["reconnect_error"] = repr(reconnect_exc)
        with self.output_path.open("a", encoding="utf-8", newline="\n") as handle:
            handle.write(json.dumps(payload, ensure_ascii=False, separators=(",", ":")) + "\n")
        return payload

    def close(self):
        if self.client is not None:
            self.client.close()


def main():
    upstream_password = os.environ.get("SEEDANCE_UPSTREAM_SSH_PASSWORD")
    if not upstream_password:
        raise SystemExit("SEEDANCE_UPSTREAM_SSH_PASSWORD is required")

    monitors = [
        HostMonitor(
            "newapi",
            "14.103.100.4",
            "root",
            key_filename=r"D:\Documents\zhuimi.pem",
        ),
        HostMonitor(
            "upstream-aipdd",
            "8.219.81.18",
            "root",
            password=upstream_password,
        ),
    ]

    STOP_FILE.unlink(missing_ok=True)
    started_epoch = time.time()
    with (ARTIFACT_DIR / "monitor-status.jsonl").open("a", encoding="utf-8", newline="\n") as status:
        status.write(json.dumps({"event": "started", "epoch": started_epoch}) + "\n")
        with concurrent.futures.ThreadPoolExecutor(max_workers=len(monitors)) as executor:
            while not STOP_FILE.exists() and time.time() - started_epoch < MAX_DURATION_SECONDS:
                tick_started = time.monotonic()
                futures = [executor.submit(monitor.sample) for monitor in monitors]
                results = [future.result() for future in futures]
                status.write(
                    json.dumps(
                        {
                            "event": "sampled",
                            "epoch": time.time(),
                            "hosts": [
                                {"label": row.get("host_label"), "error": row.get("error")}
                                for row in results
                            ],
                        },
                        ensure_ascii=False,
                    )
                    + "\n"
                )
                status.flush()
                remaining = INTERVAL_SECONDS - (time.monotonic() - tick_started)
                if remaining > 0:
                    time.sleep(remaining)
        for monitor in monitors:
            monitor.close()
        status.write(
            json.dumps(
                {
                    "event": "stopped",
                    "epoch": time.time(),
                    "reason": "stop_file" if STOP_FILE.exists() else "max_duration",
                }
            )
            + "\n"
        )


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(repr(exc), file=sys.stderr)
        raise
