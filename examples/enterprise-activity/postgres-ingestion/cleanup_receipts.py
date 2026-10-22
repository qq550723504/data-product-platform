"""Observe finite harness cleanup attempts without losing their business receipt."""
import subprocess


class CleanupReceipts:
    def __init__(self, report, components):
        self.report = report
        report["cleanup"] = "FAIL"
        report["cleanup_detail"] = {name: {"status": "NOT_RUN", "attempts": []} for name in components}

    def attempt(self, component, subject, action):
        group = self.report["cleanup_detail"][component]
        item = {"subject": subject, "status": "UNKNOWN"}
        group["attempts"].append(item)
        group["status"] = "UNKNOWN"
        try:
            value = action()
            if isinstance(value, subprocess.CompletedProcess):
                item["status"] = "PASS" if value.returncode == 0 else "FAILED"
                item["exit_code"] = value.returncode
            elif isinstance(value, dict) and "status" in value:
                item["status"] = "FAILED" if value["status"] == "FAIL" else value["status"]
                item["result"] = value
            else:
                item["status"] = "PASS"
        except Exception as error:
            item["failure_type"] = type(error).__name__
        statuses = [attempt["status"] for attempt in group["attempts"]]
        group["status"] = self.aggregate(statuses)
        return item

    def retain(self, component):
        self.report["cleanup_detail"][component]["status"] = "RETAINED"

    @staticmethod
    def aggregate(statuses):
        for state in ("UNKNOWN", "FAILED", "BLOCKED", "NOT_RUN"):
            if state in statuses:
                return state
        if all(state == "RETAINED" for state in statuses):
            return "RETAINED"
        return "PASS" if all(state in ("PASS", "RETAINED") for state in statuses) else "UNKNOWN"

    def finish(self):
        status = self.aggregate([item["status"] for item in self.report["cleanup_detail"].values()])
        self.report["cleanup"] = status if status in ("PASS", "RETAINED") else "FAIL"
