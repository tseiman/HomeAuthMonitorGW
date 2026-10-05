#!/usr/bin/env python3
"""Emit one flat Honeycomb-friendly health KPI JSON event per gateway collector."""

from __future__ import annotations

import argparse
import datetime as dt
import json
import os
import pathlib
import stat
import sys
import urllib.parse
import urllib.request

MAX_RESPONSE_BYTES = 8 << 20
WAGO_PREFIX = "1.3.6.1.4.1.13576."
WAGO_OIDS = {
    "uptime": "1.3.6.1.2.1.1.3.0",
    "rtc_battery": "1.3.6.1.4.1.13576.10.1.11.5.0",
    "error_group": "1.3.6.1.4.1.13576.10.1.20.1.0",
    "error_code": "1.3.6.1.4.1.13576.10.1.20.2.0",
    "error_description": "1.3.6.1.4.1.13576.10.1.20.4.0",
    "task_status": "1.3.6.1.4.1.13576.10.1.30.9.1.3.1",
    "task_cycle": "1.3.6.1.4.1.13576.10.1.30.9.1.9.1",
    "task_cycle_max": "1.3.6.1.4.1.13576.10.1.30.9.1.11.1",
    "modbus_max": "1.3.6.1.4.1.13576.10.1.40.6.3.0",
    "modbus_free": "1.3.6.1.4.1.13576.10.1.40.6.7.0",
    "module_count": "1.3.6.1.4.1.13576.10.1.50.1.0",
    "firmware": "1.3.6.1.4.1.13576.10.1.10.4.0",
}


def build_events(snapshot: dict, expected_wago_run_status: int | None = None,
                 battery_warning: float = 50, battery_critical: float = 20,
                 temperature_warning: float = 45, temperature_critical: float = 55) -> list[dict]:
    """Return independently scored, source-name-sorted events for all collectors."""
    if not isinstance(snapshot, dict):
        raise ValueError("snapshot root must be an object")
    generated_at = dt.datetime.now(dt.timezone.utc).isoformat().replace("+00:00", "Z")
    events = []
    for source_name in sorted(snapshot):
        source = snapshot[source_name]
        if not isinstance(source, dict):
            raise ValueError(f"collector {source_name!r} must be an object")
        metrics = source.get("metrics", [])
        if not isinstance(metrics, list):
            raise ValueError(f"collector {source_name!r} metrics must be an array")
        by_name, by_oid = _index_metrics(metrics)
        driver = str(source.get("driver", "unknown"))
        kind = _collector_kind(driver, by_name, by_oid)
        event = {
            "schema_version": 1,
            "generated_at": generated_at,
            "collector_name": source_name,
            "collector_driver": driver,
            "collector_kind": kind,
            "available": bool(source.get("available", False)),
            "stale": bool(source.get("stale", True)),
            "last_success": str(source.get("last_success", "")),
            "poll_duration_ms": _number(source.get("poll_duration_ns"), 0) / 1_000_000,
            "collector_error": str(source.get("error", "")),
        }
        critical, warning = [], []
        if not event["available"]:
            critical.append("collector_unavailable")
        if event["stale"]:
            warning.append("collector_stale")
        if kind == "wago-750-880":
            _add_wago(event, by_name, by_oid, expected_wago_run_status, critical, warning)
        elif driver == "nut":
            _add_ups(event, by_name, battery_warning, battery_critical,
                     temperature_warning, temperature_critical, critical, warning)
        _finish_health(event, critical, warning)
        events.append(event)
    return events


def _index_metrics(metrics: list) -> tuple[dict, dict]:
    by_name, by_oid = {}, {}
    for metric in metrics:
        if not isinstance(metric, dict) or "value" not in metric:
            continue
        name = metric.get("name")
        if isinstance(name, str):
            by_name[name] = metric["value"]
        labels = metric.get("labels")
        if isinstance(labels, dict) and isinstance(labels.get("oid"), str):
            by_oid[labels["oid"].lstrip(".")] = metric["value"]
    return by_name, by_oid


def _collector_kind(driver: str, by_name: dict, by_oid: dict) -> str:
    if driver == "snmp" and (any(oid.startswith(WAGO_PREFIX) for oid in by_oid) or any(name.startswith("wio") for name in by_name)):
        return "wago-750-880"
    if driver == "nut" and "phoenix" in str(by_name.get("device.mfr", "")).lower():
        return "phoenix-contact-ups"
    if driver == "nut":
        return "nut-ups"
    return driver


def _value(by_name: dict, by_oid: dict, name: str, oid_key: str):
    if name in by_name:
        return by_name[name]
    indexed = name + "[1]"
    if indexed in by_name:
        return by_name[indexed]
    return by_oid.get(WAGO_OIDS[oid_key])


def _add_wago(event: dict, by_name: dict, by_oid: dict, expected_run: int | None,
              critical: list[str], warning: list[str]) -> None:
    uptime = _value(by_name, by_oid, "sysUpTime", "uptime")
    event["uptime_seconds"] = _number(uptime) / 100 if uptime is not None else None
    event["wago_rtc_battery_status"] = _value(by_name, by_oid, "wioRtcBatteryStatus", "rtc_battery")
    event["wago_error_group"] = _value(by_name, by_oid, "wioErrorGroup", "error_group")
    event["wago_error_code"] = _value(by_name, by_oid, "wioErrorCode", "error_code")
    event["wago_diagnostic"] = _value(by_name, by_oid, "wioErrorDescription", "error_description")
    event["wago_iec_task_status"] = _value(by_name, by_oid, "wioIecTaskStatus", "task_status")
    event["wago_iec_cycle_time"] = _value(by_name, by_oid, "wioIecTaskCycleTime", "task_cycle")
    event["wago_iec_cycle_time_max"] = _value(by_name, by_oid, "wioIecTaskCycleTimeMax", "task_cycle_max")
    event["wago_modbus_connections_max"] = _value(by_name, by_oid, "wioMaxConnections", "modbus_max")
    event["wago_modbus_connections_free"] = _value(by_name, by_oid, "wioFreeModbusSockets", "modbus_free")
    event["wago_kbus_module_count"] = _value(by_name, by_oid, "wioModulCount", "module_count")
    event["wago_firmware_version"] = _value(by_name, by_oid, "wioFirmwareVersion", "firmware")
    required = (event["wago_error_group"], event["wago_error_code"], event["wago_diagnostic"], event["wago_rtc_battery_status"])
    if any(value is None for value in required):
        warning.append("wago_health_metrics_missing")
    if ((event["wago_error_code"] is not None and _number(event["wago_error_code"]) != 0)
            or (event["wago_error_group"] is not None and _number(event["wago_error_group"]) != 0)):
        critical.append("wago_error_code")
    if event["wago_rtc_battery_status"] is not None and _number(event["wago_rtc_battery_status"]) != 0:
        warning.append("wago_rtc_battery")
    free = event["wago_modbus_connections_free"]
    if free is not None and _number(free) <= 0:
        warning.append("wago_modbus_connections_exhausted")
    if expected_run is not None and event["wago_iec_task_status"] is not None and int(_number(event["wago_iec_task_status"])) != expected_run:
        critical.append("wago_iec_task_not_running")


def _add_ups(event: dict, by_name: dict, battery_warning: float, battery_critical: float,
             temperature_warning: float, temperature_critical: float,
             critical: list[str], warning: list[str]) -> None:
    status = str(by_name.get("ups.status", ""))
    tokens = set(status.split())
    event.update({
        "ups_status": status,
        "ups_model": by_name.get("device.model"),
        "battery_charge_percent": by_name.get("battery.charge"),
        "battery_runtime_seconds": by_name.get("battery.runtime"),
        "battery_temperature_celsius": by_name.get("battery.temperature"),
        "battery_voltage": by_name.get("battery.voltage"),
        "output_voltage": by_name.get("output.voltage"),
        "output_current": by_name.get("output.current"),
    })
    if not status or event["battery_charge_percent"] is None or event["battery_temperature_celsius"] is None:
        warning.append("ups_health_metrics_missing")
    if tokens & {"LB", "FSD", "OFF"}:
        critical.append("ups_status_critical")
    elif status and "OL" not in tokens:
        warning.append("ups_not_on_line")
    charge = event["battery_charge_percent"]
    if charge is not None and _number(charge) < battery_critical:
        critical.append("ups_battery_charge_critical")
    elif charge is not None and _number(charge) < battery_warning:
        warning.append("ups_battery_charge_low")
    temperature = event["battery_temperature_celsius"]
    if temperature is not None and _number(temperature) >= temperature_critical:
        critical.append("ups_battery_temperature_critical")
    elif temperature is not None and _number(temperature) >= temperature_warning:
        warning.append("ups_battery_temperature_high")


def _finish_health(event: dict, critical: list[str], warning: list[str]) -> None:
    event["critical_count"] = len(critical)
    event["warning_count"] = len(warning)
    event["health_status"] = "critical" if critical else ("warning" if warning else "healthy")
    event["health_score"] = max(0, 100 - 35 * len(critical) - 10 * len(warning))
    event["health_reasons"] = critical + warning


def _number(value, default=None) -> float:
    try:
        return float(value)
    except (TypeError, ValueError):
        if default is None:
            raise ValueError(f"expected numeric value, got {value!r}")
        return float(default)


def _read_token(path: str) -> str:
    target = pathlib.Path(path)
    info = target.lstat()
    if stat.S_ISLNK(info.st_mode) or not stat.S_ISREG(info.st_mode):
        raise ValueError("token file must be a regular file, not a symlink")
    if info.st_mode & 0o007:
        raise ValueError("token file must not be accessible to other users")
    token = target.read_text(encoding="utf-8").strip()
    if not token or "\n" in token or "\r" in token:
        raise ValueError("token file must contain exactly one non-empty line")
    return token


def _load_snapshot(args) -> dict:
    if args.input:
        if args.input == "-":
            return json.load(sys.stdin)
        with open(args.input, encoding="utf-8") as stream:
            return json.load(stream)
    parsed = urllib.parse.urlparse(args.url)
    if parsed.scheme != "https" or not parsed.netloc or parsed.query or parsed.fragment:
        raise ValueError("--url must be an HTTPS origin or /api/v1/metrics URL without query or fragment")
    url = args.url.rstrip("/")
    if not url.endswith("/api/v1/metrics"):
        url += "/api/v1/metrics"
    token = _read_token(args.token_file)
    request = urllib.request.Request(url, headers={"Authorization": "Bearer " + token, "Accept": "application/json"})
    with urllib.request.urlopen(request, timeout=args.timeout) as response:
        data = response.read(MAX_RESPONSE_BYTES + 1)
    if len(data) > MAX_RESPONSE_BYTES:
        raise ValueError("gateway response exceeds 8 MiB")
    return json.loads(data)


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument("--input", help="gateway /api/v1/metrics JSON path, or - for stdin")
    source.add_argument("--url", help="gateway HTTPS origin or complete /api/v1/metrics URL")
    parser.add_argument("--token-file", help="protected bearer-token file; required with --url")
    parser.add_argument("--timeout", type=float, default=10)
    parser.add_argument("--wago-run-status", type=int, help="expected numeric IEC task RUN value; omit until verified")
    parser.add_argument("--battery-warning", type=float, default=50)
    parser.add_argument("--battery-critical", type=float, default=20)
    parser.add_argument("--temperature-warning", type=float, default=45)
    parser.add_argument("--temperature-critical", type=float, default=55)
    args = parser.parse_args(argv)
    if args.url and not args.token_file:
        parser.error("--token-file is required with --url")
    try:
        events = build_events(_load_snapshot(args), args.wago_run_status, args.battery_warning,
                              args.battery_critical, args.temperature_warning, args.temperature_critical)
        for event in events:
            print(json.dumps(event, sort_keys=True, separators=(",", ":"), ensure_ascii=False))
    except (OSError, ValueError, json.JSONDecodeError) as error:
        print(f"health KPI failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
