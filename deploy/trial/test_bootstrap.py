"""Retained stage boundaries and crash windows; only isolated synthetic fixtures."""
from contextlib import ExitStack
import copy
from datetime import datetime, timedelta, timezone
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import uuid

import activation as a
import bootstrap as b
import resume as c
import runtime as r


class LedgerStages(unittest.TestCase):
    def fixture(self, root):
        r.STATE.mkdir(); private = root / "private"; private.mkdir()
        versions = [{"id": str(uuid.uuid4()), "dataset_id": str(i), "status": "READY", "storage_uri": "s3://raw/datasets/" + str(i),
                     "checksum_algorithm": "SHA256", "checksum_value": r.digest(bytes([i])), "byte_size": 1} for i in range(5)]
        s = {"phase": "RUNNING", "runId": "retained", "workspaceId": str(uuid.uuid4()), "actorId": str(uuid.uuid4()), "root": str(root),
             "private": str(private), "processes": {}, "containers": {"core": {"id": "same-core"}}, "privateHashes": {}, "binaries": {}, "originSnapshot": {"sha256": "old"},
             "businessBinding": {"path": "old", "sha256": "old"}}
        s["ports"] = {"core": 54321, "storage": 9000}
        b.put(private / "config.json", {"DSN": "postgres://postgres:synthetic@127.0.0.1:54321/dpp_trial_core", "Private": str(private), "Repo": s["root"],
            "Workspace": s["workspaceId"], "Actor": s["actorId"], "Profile": {"Subject": "trial-operator-retained", "Bucket": "raw", "StorageEndpoint": "127.0.0.1:9000"}})
        facts = r.STATE / "facts.json"; b.put(facts, {"workspaceId": s["workspaceId"], "facts": {"versions": versions, "bindings": [{"version_id": v["id"]} for v in versions[:3]]}})
        old = {"business": {"outputs": {"standardized": "3", "curated": "4"}}}
        admission = root / "admission.json"; b.put(admission, {"head": "b" * 40, "ciRunId": 1})
        r.save(s)
        return s, facts, admission, old

    def mocks(self, stack, old):
        stack.enter_context(patch.object(r, "verify"))
        stack.enter_context(patch.object(c, "verify_origin"))
        stack.enter_context(patch.object(b.u, "verify_business", return_value=old))
        stack.enter_context(patch.object(b, "target", return_value={"head": "b" * 40, "admission": {"head": "b" * 40, "ciRunId": 1}}))
        stack.enter_context(patch.object(b, "native", return_value={"containers": {}, "processes": {}}))

    def snapshot(self, root):
        return {p.relative_to(root).as_posix(): (p.read_bytes(), p.stat().st_mtime_ns) for p in root.rglob("*") if p.is_file()}

    def test_bootstrap_plan_is_repeatable_and_has_no_custody_or_service_dependency(self):
        with tempfile.TemporaryDirectory() as d, patch.object(r, "STATE", Path(d) / "state"), ExitStack() as stack:
            root = Path(d); s, facts, admission, old = self.fixture(root); self.mocks(stack, old)
            before = self.snapshot(root)
            for name in ("save", "docker", "spawn", "seed_source", "provision_config"):
                stack.enter_context(patch.object(r, name, side_effect=AssertionError("read-only plan mutation")))
            first = b.plan(s, "bootstrap", facts=facts, admission=admission)
            self.assertEqual(first, b.plan(s, "bootstrap", facts=facts, admission=admission))
            self.assertEqual(first["expectedSchema"], {"from": 68, "to": 71})
            self.assertEqual(first["migrationVersions"][-3:], [69, 70, 71])
            self.assertNotIn(50, first["migrationVersions"])
            self.assertEqual(before, self.snapshot(root))

    def test_partial_or_orphan_attempt_blocks_plan_resume_and_repeat_start(self):
        with tempfile.TemporaryDirectory() as d, patch.object(r, "STATE", Path(d) / "state"):
            s, _, _, _ = self.fixture(Path(d))
            (r.STATE / "bootstrap-attempts/orphan").mkdir(parents=True)
            with patch.object(r, "docker", side_effect=AssertionError("native action")), patch.object(r, "status", side_effect=AssertionError("status")):
                for operation in (lambda: b.guard(s), lambda: c.resume(s), r.start):
                    with self.assertRaisesRegex(r.Refused, "orphan"): operation()
            unknown = {**s, "phase": "STOPPED", "bootstrap": {"attemptId": "orphan", "status": "UNKNOWN"}}
            with self.assertRaisesRegex(r.Refused, "explicit recovery"): c.resume(unknown)

    def test_accepted_bootstrap_cannot_resume_old_hosts_before_explicit_activation(self):
        s = {"phase": "STOPPED", "bootstrapBinding": {"path": "accepted"}}
        with patch.object(c, "precheck", side_effect=AssertionError("old resume")):
            with self.assertRaisesRegex(r.Refused, "ordinary resume is blocked"): c.resume(s)

    def test_bootstrap_order_and_migration_interruption_are_single_attempt(self):
        for interrupted in (False, True):
            with self.subTest(interrupted=interrupted), tempfile.TemporaryDirectory() as d, patch.object(r, "STATE", Path(d) / "state"), ExitStack() as stack:
                root = Path(d); s, facts, admission, old = self.fixture(root); self.mocks(stack, old)
                expected = b.plan(s, "bootstrap", facts=facts, admission=admission)
                original = copy.deepcopy(s); order = []
                stack.enter_context(patch.object(b, "build", side_effect=lambda *args: order.append("build") or {}))
                stack.enter_context(patch.object(b, "owned", side_effect=lambda s, role, op: order.append(op + "-" + role)))
                stack.enter_context(patch.object(r, "docker", side_effect=lambda *args, **kw: order.append("backup" if "pg_dump" in args else "ready") or b"backup"))
                stack.enter_context(patch.object(b, "inspect", side_effect=lambda *args, **kw: order.append("admit-" + str(kw.get("schema", 71))) or {"schema": kw.get("schema", 71)}))
                inventory = {"tables": {"old": {"columns": ["id"], "rows": ['{"id":1}']}}}
                stack.enter_context(patch.object(a, "tool", side_effect=lambda *args, **kw: order.append("inventory") or a.encoded(inventory)))
                def migration(*args):
                    order.append("migration")
                    if interrupted: raise r.Refused("loss after migration 69")
                stack.enter_context(patch.object(b, "migrate", side_effect=migration))
                for obj, name in ((c, "wait_containers"), (c, "refresh_identity"), (c, "launch_hosts"), (r, "seed_source"), (r, "http")):
                    stack.enter_context(patch.object(obj, name, side_effect=AssertionError("forbidden stage effect")))
                if interrupted:
                    with self.assertRaisesRegex(r.Refused, "migration 69"): b.execute(s, expected)
                    self.assertEqual(s["phase"], "UNKNOWN")
                    with self.assertRaises(r.Refused): c.resume({**s, "phase": "STOPPED"})
                    self.assertNotIn("stop-core", order)
                    receipts = Path(s["bootstrap"]["receipts"])
                    self.assertTrue((receipts / "migration-started.json").exists())
                    self.assertFalse((receipts / "migration-result.json").exists())
                    self.assertEqual(r.read(receipts / "before-state.json"), original)
                else:
                    b.execute(s, expected)
                    self.assertEqual(order, ["build", "start-core", "ready", "admit-68", "backup", "inventory", "migration", "inventory", "admit-71", "stop-core"])
                    self.assertEqual(s["phase"], "STOPPED")
                    self.assertEqual(s["businessBinding"], original["businessBinding"])
                    manifest = b.binding(s, "bootstrap")
                    self.assertEqual(manifest["plan"], expected)
                    before = self.snapshot(root)
                    p = root / "reviewed.json"; b.put(p, expected); before = self.snapshot(root)
                    self.assertTrue(b.run(s, "bootstrap", p, r.digest(a.encoded(expected)))["replay"])
                    self.assertEqual(before, self.snapshot(root))

    def test_five_version_identity_and_custody_plan_are_exact(self):
        with tempfile.TemporaryDirectory() as d, patch.object(r, "STATE", Path(d) / "state"), ExitStack() as stack:
            root = Path(d); s, facts, admission, old = self.fixture(root); self.mocks(stack, old)
            baseline = b.plan(s, "bootstrap", facts=facts, admission=admission)
            s.update(phase="STOPPED", bootstrap={"status": "PASS", "planSha256": "a" * 64}, bootstrapBinding={"path": "accepted"}); r.save(s)
            stack.enter_context(patch.object(b, "binding", return_value={"target": baseline["target"], "plan": baseline}))
            config = Path(s["private"]) / "config.json"
            output = Path(s["private"]) / "output"; output.mkdir()
            scope = {"workspaceId": s["workspaceId"], "actorId": s["actorId"], "runId": s["runId"], "principalRef": "trial-operator-retained", "endpoint": "127.0.0.1:9000",
                     "providerWitness": r.digest(a.encoded({})), "planSha256": "a" * 64, "expiresAt": (datetime.now(timezone.utc) + timedelta(minutes=20)).isoformat(), "outputDirectory": str(output)}
            for name, v in zip(("standardized", "curated"), r.read(facts)["facts"]["versions"][3:]):
                scope[name] = {"versionId": v["id"], "uri": v["storage_uri"], "sha256": v["checksum_value"], "bytes": v["byte_size"]}
            profile = Path(s["private"]) / "profile.json"
            value = {"plan": scope, "credentialFile": str(config), "credentialFileSha256": b.file(config)["sha256"]}; b.put(profile, value)
            expected = b.plan(s, "custody", admission=admission, profile=profile)
            self.assertEqual(expected["bootstrap"], s["bootstrapBinding"])
            for field in ("workspaceId", "runId", "providerWitness", "planSha256"):
                broken = copy.deepcopy(value); broken["plan"][field] = "foreign"; r.write(profile, broken)
                with self.assertRaises(r.Refused): b.plan(s, "custody", admission=admission, profile=profile)
            r.write(profile, value); (output / "partial").write_bytes(b"keep")
            with self.assertRaisesRegex(r.Refused, "empty"): b.plan(s, "custody", admission=admission, profile=profile)

    def test_readonly_source_ci_admission_rejects_source_or_actual_run_substitution(self):
        head = "b" * 40; source = {"deploy/trial/controller.py": "new"}
        with tempfile.TemporaryDirectory() as d, patch.object(r, "ROOT", Path(d)), patch.object(r, "sources", return_value=source), patch.object(c, "committed_sources", side_effect=lambda h: source if h == head else {}), patch.object(b.u, "verify_business", return_value={"head": "c" * 40}):
            workflow = Path(d) / b.u.WORKFLOW; workflow.parent.mkdir(parents=True); workflow.write_bytes(b"workflow")
            with patch.object(r, "command", side_effect=lambda args: head.encode() if args[1] == "rev-parse" else b"workflow"):
                for invalid in (None, "sha", "event", "branch", "status", "required"):
                    run = {"head_sha": head, "event": "push", "head_branch": "main", "status": "completed", "conclusion": "success", "path": ".github/workflows/ci.yml"}
                    if invalid and invalid != "required": run[{"sha": "head_sha", "event": "event", "branch": "head_branch", "status": "status"}[invalid]] = "foreign"
                    def reply(path):
                        if path.startswith("commits/"): return {"sha": head}
                        if path == "branches/main": return {"commit": {"sha": head}}
                        if path.startswith("compare/"): return {"merge_base_commit": {"sha": head}, "status": "identical"}
                        if path.endswith("per_page=100"): return {"total_count": 1, "jobs": [{"name": "required", "status": "completed", "conclusion": "failure" if invalid == "required" else "success"}]}
                        return run
                    with patch.object(b, "github", side_effect=reply):
                        if invalid:
                            with self.assertRaises(r.Refused): b.target({}, {"head": head, "ciRunId": 1})
                        else: self.assertEqual(b.target({}, {"head": head, "ciRunId": 1})["head"], head)

    def test_custody_has_one_sdk_entry_no_health_sts_or_host_launch_and_unknown_cannot_retry(self):
        for failed_observation in (False, True):
            with self.subTest(failed_observation=failed_observation), tempfile.TemporaryDirectory() as d, patch.object(r, "STATE", Path(d) / "state"), ExitStack() as stack:
                root = Path(d); s, facts, admission, old = self.fixture(root); self.mocks(stack, old)
                baseline = b.plan(s, "bootstrap", facts=facts, admission=admission)
                s.update(phase="STOPPED", bootstrap={"status": "PASS", "planSha256": "a" * 64}, bootstrapBinding={"path": "accepted"}); r.save(s)
                stack.enter_context(patch.object(b, "binding", return_value={"target": baseline["target"], "plan": baseline}))
                output = Path(s["private"]) / "output"; output.mkdir()
                p = {"outputDirectory": str(output)}
                profile = Path(s["private"]) / "profile.json"; b.put(profile, {"plan": p})
                expected = {**baseline, "stage": "custody", "profile": b.file(profile)}
                order = []
                stack.enter_context(patch.object(b, "build", side_effect=lambda *args: order.append("build") or {}))
                stack.enter_context(patch.object(b, "owned", side_effect=lambda s, role, op: order.append(op + "-" + role)))
                stack.enter_context(patch.object(r, "docker", side_effect=lambda *args, **kw: order.append("core-ready") or b"ready"))
                def sdk(args, **kwargs):
                    order.append("sdk"); self.assertEqual(args[1], "-profile"); self.assertIn("-profile-sha256", args)
                    members = []
                    for i in range(2):
                        path = output / (str(i) + ".csv"); path.write_bytes(bytes([i])); members.append({"versionId": str(i), "file": str(path)})
                    b.put(output / "custody.json", members)
                    return a.encoded({"status": "ACCEPTED", "acquisitionId": str(uuid.uuid4()), "manifestSha256": b.file(output / "custody.json")["sha256"]})
                command = stack.enter_context(patch.object(r, "command", side_effect=sdk))
                def inspect(*args, **kwargs):
                    order.append("admit-custody" if kwargs.get("custody") else "admit-71")
                    if kwargs.get("custody") and failed_observation: raise r.Refused("lost custody SQL observation")
                    return {"schema": 71}
                stack.enter_context(patch.object(b, "inspect", side_effect=inspect))
                for obj, name in ((c, "wait_containers"), (c, "refresh_identity"), (c, "launch_hosts"), (r, "seed_source"), (r, "http"), (b, "migrate")):
                    stack.enter_context(patch.object(obj, name, side_effect=AssertionError("forbidden custody effect")))
                if failed_observation:
                    with self.assertRaisesRegex(r.Refused, "lost custody"): b.execute(s, expected)
                    self.assertEqual(s["phase"], "UNKNOWN")
                    with self.assertRaises(r.Refused): c.resume({**s, "phase": "STOPPED"})
                else:
                    b.execute(s, expected)
                    self.assertEqual(s["phase"], "STOPPED")
                    self.assertEqual(order, ["build", "start-core", "core-ready", "admit-71", "start-objects", "start-gateway", "sdk", "admit-custody", "stop-gateway", "stop-objects", "stop-core"])
                command.assert_called_once()

    def test_journal_is_exclusive_and_cannot_write_in_readonly_context(self):
        with tempfile.TemporaryDirectory() as d:
            path = Path(d) / "fact.json"; b.put(path, {"status": "UNKNOWN"})
            with self.assertRaises(FileExistsError): b.put(path, {"status": "PASS"})
            with r.read_only_observation(), self.assertRaises(r.Refused): b.put(Path(d) / "forbidden", {})


if __name__ == "__main__": unittest.main()
