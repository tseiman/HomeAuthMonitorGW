#!/usr/bin/env python3
"""Tests for the per-collector health KPI event generator."""

import importlib.util
import pathlib
import unittest

SCRIPT = pathlib.Path(__file__).with_name("health-kpi.py")


class HealthKPITest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        spec = importlib.util.spec_from_file_location("health_kpi", SCRIPT)
        cls.module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(cls.module)

    def test_build_events_returns_one_independent_event_per_collector(self):
        snapshot = {
            "wago-main": {
                "driver": "snmp",
                "available": True,
                "stale": False,
                "last_success": "2026-10-04T23:04:42Z",
                "poll_duration_ns": 2_000_000,
                "metrics": [
                    metric("sysUpTime", 18_993_191, "1.3.6.1.2.1.1.3.0"),
                    metric("wioRtcBatteryStatus", 0, "1.3.6.1.4.1.13576.10.1.11.5.0"),
                    metric("wioErrorGroup", 0, "1.3.6.1.4.1.13576.10.1.20.1.0"),
                    metric("wioErrorCode", 0, "1.3.6.1.4.1.13576.10.1.20.2.0"),
                    metric("wioErrorDescription", "Coupler running, OK", "1.3.6.1.4.1.13576.10.1.20.4.0", "string"),
                    metric("wioFreeModbusSockets", 15, "1.3.6.1.4.1.13576.10.1.40.6.7.0"),
                ],
            },
            "ups-main": {
                "driver": "nut",
                "available": True,
                "stale": False,
                "last_success": "2026-10-04T23:04:42Z",
                "poll_duration_ns": 1_000_000,
                "metrics": [
                    metric("device.mfr", "Phoenix Contact", value_type="string"),
                    metric("ups.status", "OL", value_type="string"),
                    metric("battery.charge", 100),
                    metric("battery.runtime", 3600),
                    metric("battery.temperature", 28),
                    metric("output.voltage", 24.1),
                ],
            },
        }

        events = self.module.build_events(snapshot, expected_wago_run_status=None)

        self.assertEqual([event["collector_name"] for event in events], ["ups-main", "wago-main"])
        self.assertNotIn("overall", {event["collector_name"] for event in events})
        by_name = {event["collector_name"]: event for event in events}
        self.assertEqual(by_name["wago-main"]["collector_kind"], "wago-750-880")
        self.assertEqual(by_name["wago-main"]["health_status"], "healthy")
        self.assertEqual(by_name["wago-main"]["wago_diagnostic"], "Coupler running, OK")
        self.assertAlmostEqual(by_name["wago-main"]["uptime_seconds"], 189931.91)
        self.assertEqual(by_name["ups-main"]["collector_kind"], "phoenix-contact-ups")
        self.assertEqual(by_name["ups-main"]["ups_status"], "OL")
        self.assertEqual(by_name["ups-main"]["battery_charge_percent"], 100)

    def test_collector_failures_do_not_change_other_collector_health(self):
        snapshot = {
            "wago-main": {
                "driver": "snmp", "available": True, "stale": False, "metrics": [
                    metric("wioErrorCode", 7, "1.3.6.1.4.1.13576.10.1.20.2.0"),
                    metric("wioErrorDescription", "K-bus fault", "1.3.6.1.4.1.13576.10.1.20.4.0", "string"),
                ],
            },
            "ups-main": {
                "driver": "nut", "available": True, "stale": False, "metrics": [
                    metric("ups.status", "OL", value_type="string"), metric("battery.charge", 100),
                    metric("battery.temperature", 28),
                ],
            },
        }

        events = {event["collector_name"]: event for event in self.module.build_events(snapshot, None)}

        self.assertEqual(events["wago-main"]["health_status"], "critical")
        self.assertEqual(events["ups-main"]["health_status"], "healthy")
        self.assertIn("wago_error_code", events["wago-main"]["health_reasons"])

    def test_missing_health_metrics_are_warning_not_healthy(self):
        snapshot = {
            "wago-main": {"driver": "snmp", "available": True, "stale": False, "metrics": [
                metric("wioErrorDescription", "Coupler running, OK", "1.3.6.1.4.1.13576.10.1.20.4.0", "string"),
            ]},
            "ups-main": {"driver": "nut", "available": True, "stale": False, "metrics": [
                metric("ups.status", "OL", value_type="string"),
            ]},
        }

        events = {event["collector_name"]: event for event in self.module.build_events(snapshot, None)}

        self.assertEqual(events["wago-main"]["health_status"], "warning")
        self.assertEqual(events["ups-main"]["health_status"], "warning")
        self.assertIn("wago_health_metrics_missing", events["wago-main"]["health_reasons"])
        self.assertIn("ups_health_metrics_missing", events["ups-main"]["health_reasons"])


def metric(name, value, oid=None, value_type="number"):
    result = {"name": name, "value": value, "value_type": value_type, "timestamp": "2026-10-04T23:04:42Z"}
    if oid:
        result["labels"] = {"oid": oid}
    return result


if __name__ == "__main__":
    unittest.main()
