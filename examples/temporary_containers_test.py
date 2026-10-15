"""Host-independent destructive-boundary tests; no daemon or provider access."""
import copy
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import temporary_containers as tc


RUN = "d98080ae-a114-4a9b-9972-e8bc9c896cfe"
ID = "a" * 64


class Docker:
    def __init__(self):
        self.item = None
        self.calls = []
        self.fail_logs = False
        self.fail_remove = False
        self.before_inspect = None

    def __call__(self, args):
        self.calls.append(args)
        output, error = b"", b""
        if args[:2] == ["docker", "create"]:
            labels = {args[i + 1].split("=", 1)[0]: args[i + 1].split("=", 1)[1]
                      for i, part in enumerate(args) if part == "--label"}
            self.item = dict(Id=ID, Name="/owned-helper", Image="sha256:" + "b" * 64,
                Created="2026-10-15T01:00:00Z", RestartCount=0,
                State=dict(Status="exited", Running=False, ExitCode=0, Error="", OOMKilled=False,
                           StartedAt="2026-10-15T01:00:01Z", FinishedAt="2026-10-15T01:00:02Z"),
                Config=dict(Labels=labels, Env=["CONTROLLED_SECRET=never print this"]),
                HostConfig=dict(ReadonlyRootfs=True, RestartPolicy=dict(Name="no"),
                                PortBindings={}, AutoRemove=False, Privileged=False), Mounts=[])
            output = (ID + "\n").encode()
        elif args[:3] == ["docker", "container", "inspect"]:
            if self.before_inspect:
                callback, self.before_inspect = self.before_inspect, None
                callback()
            if self.item is None:
                raise tc.CleanupError("controlled missing container")
            output = json.dumps([self.item]).encode()
        elif args[:2] == ["docker", "logs"]:
            if self.fail_logs:
                raise tc.CleanupError("controlled log driver failure")
            output, error = b"controlled stdout\n", b"controlled stderr\n"
        elif args[:3] == ["docker", "container", "ls"]:
            output = (ID + "\n").encode() if self.item else b""
        elif args[:3] == ["docker", "container", "rm"]:
            if self.fail_remove or self.item["State"]["Running"]:
                raise tc.CleanupError("controlled remove refused")
            self.item = None
        else:
            raise AssertionError("Unexpected operation")
        return subprocess.CompletedProcess(args, 0, output, error)


class Contracts(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name).resolve()
        # Test private filesystem behavior independently of Windows account ACLs.
        self.private = patch.object(tc, "private_root", return_value=self.root)
        self.private.start()
        self.addCleanup(self.private.stop)
        self.docker = Docker()
        self.helpers = tc.TemporaryContainers(self.root, self.docker)
        self.handle = self.helpers.create(RUN, ["--read-only", "controlled-image"])

    def ticket(self):
        return Path(self.handle["ticket"])

    def removals(self):
        return [a for a in self.docker.calls if a[:3] == ["docker", "container", "rm"]]

    def test_success_archives_both_streams_before_exact_id_removal(self):
        result = self.helpers.complete(self.ticket())
        self.assertEqual(result["status"], "REMOVED")
        folder = self.ticket().parent / "diagnostics"
        self.assertEqual((folder / "stderr.log").read_bytes(), b"controlled stderr\n")
        self.assertEqual((folder / "stdout.log").read_bytes(), b"controlled stdout\n")
        self.assertEqual(json.loads((folder / "metadata.json").read_text())["Id"], ID)
        self.assertEqual(self.removals(), [["docker", "container", "rm", ID]])
        self.assertEqual(result["volumes_removed"], 0)
        self.assertNotIn("CONTROLLED_SECRET", json.dumps(result))
        self.assertEqual(self.helpers.complete(self.ticket())["status"], "ALREADY_REMOVED")

    def test_failed_or_running_or_data_or_foreign_containers_are_not_removed(self):
        pristine = copy.deepcopy(self.docker.item)
        variants = []
        for key, value in (("Status", "running"), ("Running", True), ("ExitCode", 7),
                           ("OOMKilled", True), ("Error", "controlled-error")):
            item = copy.deepcopy(pristine); item["State"][key] = value; variants.append(item)
        for key, value in (("ReadonlyRootfs", False), ("PortBindings", {"80/tcp": [{}]}),
                           ("RestartPolicy", {"Name": "always"}), ("AutoRemove", True), ("Privileged", True)):
            item = copy.deepcopy(pristine); item["HostConfig"][key] = value; variants.append(item)
        item = copy.deepcopy(pristine); item["Mounts"] = [{"Type": "volume", "Name": "retained-db"}]; variants.append(item)
        item = copy.deepcopy(pristine); item["Config"]["Labels"][tc.PREFIX + "owner"] = "other-session"; variants.append(item)
        item = copy.deepcopy(pristine); item["Config"]["Labels"]["com.docker.compose.project"] = "retained"; variants.append(item)
        for item in variants:
            with self.subTest(item=item):
                self.docker.item = item
                with self.assertRaises(tc.CleanupError):
                    self.helpers.complete(self.ticket())
                self.assertFalse(self.removals())

    def test_intentional_nonzero_exit_requires_exact_expected_code(self):
        self.docker.item["State"]["ExitCode"] = 95
        with self.assertRaises(tc.CleanupError):
            self.helpers.complete(self.ticket())
        self.assertEqual(self.helpers.complete(self.ticket(), expected=95)["status"], "REMOVED")

    def test_incomplete_archive_can_be_retried_without_repeating_business_work(self):
        self.docker.fail_logs = True
        with self.assertRaises(tc.CleanupError):
            self.helpers.complete(self.ticket())
        self.assertFalse(self.removals())
        self.docker.fail_logs = False
        self.assertEqual(self.helpers.collect(self.ticket())["status"], "REMOVED")
        self.assertEqual(sum(a[:2] == ["docker", "create"] for a in self.docker.calls), 1)

    def test_changed_completion_and_corrupt_archive_block_retry(self):
        self.docker.fail_remove = True
        with self.assertRaises(tc.CleanupError):
            self.helpers.complete(self.ticket())
        before = len(self.removals())
        pristine = copy.deepcopy(self.docker.item)
        self.docker.item["State"]["FinishedAt"] = "2026-10-15T02:00:00Z"
        self.docker.fail_remove = False
        with self.assertRaises(tc.CleanupError):
            self.helpers.collect(self.ticket())
        self.assertEqual(len(self.removals()), before)
        self.docker.item = pristine
        (self.ticket().parent / "diagnostics/stdout.log").write_text("tampered")
        with self.assertRaises(tc.CleanupError):
            self.helpers.collect(self.ticket())
        self.assertEqual(len(self.removals()), before)

    def test_active_and_explicitly_retained_tickets_are_not_collected(self):
        self.assertEqual(self.helpers.collect(self.ticket())["status"], "RETAINED")
        self.helpers.complete(self.ticket(), retain=True)
        self.assertEqual(self.helpers.collect(self.ticket())["status"], "RETAINED")
        self.assertFalse(self.removals())

    def test_absence_after_lost_remove_response_is_recovered_from_archive(self):
        self.docker.fail_remove = True
        with self.assertRaises(tc.CleanupError):
            self.helpers.complete(self.ticket())
        self.docker.item = None
        self.assertEqual(self.helpers.collect(self.ticket())["status"], "ALREADY_ABSENT")

    def test_inspection_failure_does_not_mean_absence(self):
        self.docker.fail_remove = True
        with self.assertRaises(tc.CleanupError):
            self.helpers.complete(self.ticket())
        def fail():
            raise tc.CleanupError("daemon unavailable")
        self.docker.before_inspect = fail
        with self.assertRaises(tc.CleanupError):
            self.helpers.collect(self.ticket())
        self.assertEqual(json.loads(self.ticket().read_text())["state"], "COMPLETED")

    def test_foreign_ticket_directory_and_forged_owner_are_rejected(self):
        with self.assertRaises(tc.CleanupError):
            self.helpers.load(self.root / "foreign/ticket.json")
        record = json.loads(self.ticket().read_text()); record["owner"] = "f" * 32
        tc.write_json(self.ticket(), record)
        with self.assertRaises(tc.CleanupError):
            self.helpers.complete(self.ticket())


if __name__ == "__main__":
    unittest.main()
