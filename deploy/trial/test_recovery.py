"""Bounded recovery contracts; temporary fixtures only, no retained environment."""
from contextlib import ExitStack, contextmanager, nullcontext, redirect_stdout
import copy
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import uuid

import activation as a
import bootstrap as b
import recovery as recovery
import runtime as r
import resume as c
import trial as t
import test_bootstrap as fixtures


class RecoveryContracts(unittest.TestCase):
    @contextmanager
    def fixture(self, *, noncanonical_root=False):
        with tempfile.TemporaryDirectory() as name, ExitStack() as stack:
            root = Path(name)
            if noncanonical_root:
                (root / "alias").mkdir()
                root = root / "alias" / ".."
            stack.enter_context(patch.object(r, "STATE", root / "state"))
            stack.enter_context(patch.object(t, "STATE", root / "state"))
            helper = fixtures.LedgerStages()
            s, facts, admission, old_business = helper.fixture(root)
            helper.mocks(stack, old_business)
            owner = {"pid": 123, "created": 45, "exe": "original.exe"}
            s["processes"] = {"api": owner}; r.save(s)
            native = {"containers": {}, "processes": {"api": owner}}
            b.native.side_effect = lambda s, hosts=False: native if hosts else {"containers": {}, "processes": {"api": None}}
            old_source = {"deploy/trial/bootstrap.py": "old"}
            target = {"head": recovery.PREDECESSOR, "sources": old_source, "admission": {"head": recovery.PREDECESSOR, "ciRunId": 1}}
            b.target.return_value = target
            r.write(admission, target["admission"])
            original_plan = b.plan(s, "bootstrap", facts=facts, admission=admission)
            identity = uuid.uuid4().hex
            journal = r.STATE / "bootstrap-attempts" / identity; journal.mkdir(parents=True)
            private = Path(s["private"]) / "bootstrap-attempts" / identity; private.mkdir(parents=True)
            b.put(journal / "before-state.json", s); b.put(journal / "plan.json", original_plan)
            for action in ("build", "private-admission"):
                b.put(journal / (action + "-started.json"), {"action": action})
                b.put(journal / (action + "-result.json"), {"status": "PASS"})
            b.put(journal / "failed.json", {"status": "UNKNOWN", "retained": True, "failureType": "Refused"})
            for tool in ("trial-activate", "migrate"): (private / (tool + ".exe")).write_bytes(b"old-tool")
            b.put(private / (uuid.uuid4().hex + "-input.json"), {"configFile": str(Path(s["private"]) / "config.json")})
            s.update(phase="UNKNOWN", bootstrap={"attemptId": identity, "status": "UNKNOWN", "receipts": str(journal), "planSha256": r.digest(b.encoded(original_plan))}); r.save(s)
            stack.enter_context(patch.object(c, "committed_sources", return_value=old_source))
            new_admission = root / "new-admission.json"; b.put(new_admission, {"head": "b" * 40, "ciRunId": 2})
            b.target.return_value = {"head": "b" * 40, "sources": {"deploy/trial/recovery.py": "new"}, "admission": r.read(new_admission)}
            yield root, s, journal, private, new_admission, original_plan, stack

    def snapshot(self, root):
        return {p.relative_to(root).as_posix(): (p.read_bytes(), p.stat().st_mtime_ns) for p in root.rglob("*") if p.is_file()}

    def test_registry_uses_the_existing_canonical_fingerprint_path(self):
        # Windows CI may spell the temp directory RUNNER~1 while fingerprint()
        # resolves its long name. Reproduce differing path spellings on any OS.
        with self.fixture(noncanonical_root=True) as (root, s, _, _, admission, old, _):
            self.assertNotEqual(old["registry"]["path"], str(r.STATE / "state.json"))
            self.assertEqual(old["registry"]["path"], str((r.STATE / "state.json").resolve()))
            before = self.snapshot(root)
            self.assertEqual(recovery.plan(s, admission)["attempt"], s["bootstrap"])
            self.assertEqual(before, self.snapshot(root))

    def test_real_cli_recovery_plan_is_repeatable_and_readonly(self):
        with self.fixture() as (root, s, journal, private, admission, old, stack):
            before = self.snapshot(root)
            for obj, name in ((r, "save"), (r, "command"), (r, "docker"), (r, "spawn"), (b, "build"), (b, "migrate")):
                stack.enter_context(patch.object(obj, name, side_effect=AssertionError("plan effect")))
            result = []
            for _ in range(2):
                output = io.StringIO()
                with patch.object(t.sys, "argv", ["trial.py", "bootstrap-recovery-plan", "--admission", str(admission)]), redirect_stdout(output):
                    self.assertEqual(t.main(), 0)
                result.append(json.loads(output.getvalue()))
            self.assertEqual(result[0], result[1]); self.assertEqual(before, self.snapshot(root))
            self.assertEqual(result[0]["plan"]["attempt"], s["bootstrap"])
            self.assertNotEqual(result[0]["plan"]["bootstrapPlan"]["target"], old["target"])

    def test_any_later_marker_additional_file_or_orphan_refuses(self):
        cases = [("public", name) for name in ("stop-host-api-started.json", "start-core-started.json", "before-admission.json", "backup-started.json", "migration-started.json", "core-admission.json", "other.json")]
        cases += [("private", name) for name in ("before.sql", "before-inventory.json", "migrations", "extra.exe")]
        cases += [("public", "recoveries/orphan"), ("private", "recoveries/orphan"), ("sibling", "orphan")]
        for location, name in cases:
            with self.subTest(location=location, name=name), self.fixture() as (_, s, journal, private, admission, _, _):
                base = journal if location == "public" else private if location == "private" else journal.parent
                path = base / name
                if "/" in name or name in ("migrations", "orphan"): path.mkdir(parents=True)
                else: path.write_bytes(b"later")
                with self.assertRaises(r.Refused): recovery.plan(s, admission)

    def test_checkpoint_identity_inputs_and_pinned_source_fail_closed(self):
        for invalid in ("source", "source-map", "registry-path", "digest", "before-state", "config", "facts", "private-input", "failed", "partial-build", "private-pass", "native", "binding"):
            with self.subTest(invalid=invalid), self.fixture() as (root, s, journal, private, admission, old, stack):
                if invalid in ("source", "source-map"):
                    changed = copy.deepcopy(old)
                    changed["target"]["head" if invalid == "source" else "sources"] = "foreign"
                    r.write(journal / "plan.json", changed); s["bootstrap"]["planSha256"] = r.digest(b.encoded(changed))
                elif invalid == "registry-path":
                    r.write(root / "foreign.json", s)
                    changed = copy.deepcopy(old); changed["registry"]["path"] = str(root / "foreign.json")
                    r.write(journal / "plan.json", changed); s["bootstrap"]["planSha256"] = r.digest(b.encoded(changed))
                elif invalid == "digest": s["bootstrap"]["planSha256"] = "0" * 64
                elif invalid == "before-state": value = r.read(journal / "before-state.json"); value["actorId"] = str(uuid.uuid4()); r.write(journal / "before-state.json", value)
                elif invalid == "config": (Path(s["private"]) / "config.json").write_bytes(b"changed")
                elif invalid == "facts": (root / "state/facts.json").write_bytes(b"changed")
                elif invalid == "private-input": r.write(next(private.glob("*-input.json")), {"configFile": "foreign", "schema": 68})
                elif invalid == "failed": r.write(journal / "failed.json", {"status": "UNKNOWN", "failureType": "OSError", "retained": True})
                elif invalid in ("partial-build", "private-pass"): r.write(journal / ("build-result.json" if invalid == "partial-build" else "private-admission-result.json"), {"status": "UNKNOWN"})
                elif invalid == "native": b.native.side_effect = lambda *args, **kw: {"containers": {"foreign": {}}, "processes": {}}
                else: s["bootstrapBinding"] = {"path": "later"}
                r.save(s)
                with self.assertRaises((r.Refused, ValueError)): recovery.plan(s, admission)

    def execution_mocks(self, stack, order, *, interrupted=False, tamper=None):
        def build(directory, names):
            order.append("build")
            tools = {}
            for name in names:
                p = directory / (name + ".exe"); p.write_bytes(b"fresh-tool"); tools[name] = b.file(p)
            if tamper: tamper()
            return tools
        stack.enter_context(patch.object(b, "build", side_effect=build))
        inventory = {"tables": {"business": {"columns": ["id"], "rows": ['{"id":1}']}}}
        stack.enter_context(patch.object(a, "tool", side_effect=lambda s, directory, name, mode, **kw: order.append("private" if mode == "private" else "inventory") or b.encoded({"status": "PASS", "privateACL": True} if mode == "private" else inventory)))
        stack.enter_context(patch.object(b, "owned", side_effect=lambda s, role, op: order.append(op + "-" + role)))
        stack.enter_context(patch.object(r, "docker", side_effect=lambda *args, **kw: order.append("backup" if "pg_dump" in args else "ready") or b"backup"))
        stack.enter_context(patch.object(b, "inspect", side_effect=lambda s, directory, expected, **kw: order.append("admit-" + str(kw.get("schema", 71))) or {"schema": kw.get("schema", 71), "versions": expected["versions"]}))
        stack.enter_context(patch.object(r, "process_identity", side_effect=lambda pid, terminate=False, **kw: order.append("stop-host") if terminate else None))
        def migrate(*args):
            order.append("migration")
            if interrupted: raise r.Refused("interrupted after69")
        stack.enter_context(patch.object(b, "migrate", side_effect=migrate))
        for obj, name in ((r, "http"), (r, "seed_source"), (c, "refresh_identity"), (c, "launch_hosts")):
            stack.enter_context(patch.object(obj, name, side_effect=AssertionError("forbidden recovery effect")))

    def test_same_attempt_resolution_shared_bootstrap_order_and_metadata_only_replay(self):
        with self.fixture() as (root, s, journal, private, admission, old, stack):
            expected = recovery.plan(s, admission)
            path = root / "reviewed.json"; b.put(path, expected); digest = r.digest(b.encoded(expected))
            old_files = self.snapshot(journal); old_private = self.snapshot(private); original = copy.deepcopy(s)
            order = []; self.execution_mocks(stack, order)
            result = recovery.run(s, path, digest)
            self.assertEqual(order, ["build", "private", "stop-host", "start-core", "ready", "admit-68", "backup", "inventory", "migration", "inventory", "admit-71", "stop-core"])
            self.assertEqual(result["attemptId"], original["bootstrap"]["attemptId"])
            self.assertEqual(s["bootstrap"]["planSha256"], original["bootstrap"]["planSha256"])
            self.assertEqual(s["bootstrap"]["receipts"], str(journal)); self.assertEqual(s["phase"], "STOPPED")
            for name, value in old_files.items(): self.assertEqual(self.snapshot(journal)[name], value)
            for name, value in old_private.items(): self.assertEqual(self.snapshot(private)[name], value)
            accepted = b.binding(s, "bootstrap")
            self.assertEqual(accepted["plan"], expected["bootstrapPlan"])
            self.assertEqual(result["acceptedBootstrapPlanSha256"], r.digest(b.encoded(accepted["plan"])))
            self.assertNotEqual(result["acceptedBootstrapPlanSha256"], s["bootstrap"]["planSha256"])
            resolution = r.read(Path(s["bootstrap"]["recovery"]["receipts"]) / "resolution.json")
            self.assertEqual(resolution["firstFailure"], b.file(journal / "failed.json"))
            with self.assertRaisesRegex(r.Refused, "ordinary resume"): b.resume_guard(s)
            before_replay = self.snapshot(root); previous_order = list(order)
            with patch.object(b, "target", side_effect=AssertionError("replay API")), patch.object(b, "bootstrap_work", side_effect=AssertionError("replay effects")):
                self.assertTrue(recovery.run(s, path, digest)["replay"])
            self.assertEqual(previous_order, order); self.assertEqual(before_replay, self.snapshot(root))

    def test_failed_recovery_retains_original_unknown_and_refuses_further_recovery(self):
        with self.fixture() as (root, s, journal, private, admission, old, stack):
            expected = recovery.plan(s, admission); path = root / "reviewed.json"; b.put(path, expected)
            first_failure = (journal / "failed.json").read_bytes()
            order = []; self.execution_mocks(stack, order, interrupted=True)
            with self.assertRaisesRegex(r.Refused, "after69"): recovery.run(s, path, r.digest(b.encoded(expected)))
            self.assertEqual(s["phase"], "UNKNOWN"); self.assertEqual((journal / "failed.json").read_bytes(), first_failure)
            self.assertTrue((Path(s["bootstrap"]["recovery"]["receipts"]) / "failed.json").exists())
            with self.assertRaises(r.Refused): recovery.plan(s, admission)
            with self.assertRaises(r.Refused): b.guard(s)
            self.assertNotIn("stop-core", order)

    def test_real_cli_dispatches_explicit_recovery_with_same_attempt(self):
        with self.fixture() as (root, s, journal, private, admission, old, stack):
            expected = recovery.plan(s, admission); path = root / "reviewed.json"; b.put(path, expected)
            order = []; self.execution_mocks(stack, order)
            output = io.StringIO()
            with patch.object(t, "exclusive", return_value=nullcontext()), patch.object(t.sys, "argv", ["trial.py", "bootstrap-recover", "--plan", str(path), "--plan-sha256", r.digest(b.encoded(expected))]), redirect_stdout(output):
                self.assertEqual(t.main(), 0)
            self.assertEqual(json.loads(output.getvalue())["attemptId"], s["bootstrap"]["attemptId"])
            self.assertEqual(r.read(r.STATE / "state.json")["phase"], "STOPPED")

    def test_input_native_source_private_or_registry_drift_before_effects_refuses(self):
        for invalid in ("input", "native", "source", "private", "registry", "orphan"):
            with self.subTest(invalid=invalid), self.fixture() as (root, s, journal, private, admission, old, stack):
                expected = recovery.plan(s, admission); path = root / "reviewed.json"; b.put(path, expected)
                def tamper():
                    if invalid == "input": (private / "migrate.exe").write_bytes(b"tampered")
                    elif invalid == "native": b.native.side_effect = lambda *args, **kw: {"containers": {}, "processes": {}}
                    elif invalid == "source": b.target.return_value = {"head": "foreign"}
                    elif invalid == "registry": value = r.read(r.STATE / "state.json"); value["actorId"] = "foreign"; r.write(r.STATE / "state.json", value)
                    elif invalid == "orphan": (private / "recoveries/foreign").mkdir()
                order = []; self.execution_mocks(stack, order, tamper=tamper)
                if invalid == "private": stack.enter_context(patch.object(a, "tool", side_effect=r.Refused("private ACL rejected")))
                with self.assertRaises(r.Refused): recovery.run(s, path, r.digest(b.encoded(expected)))
                self.assertFalse(any(x in order for x in ("stop-host", "start-core", "backup", "migration")))
                self.assertEqual(s["phase"], "UNKNOWN")

    def test_cli_rejects_mixed_recovery_options_before_io(self):
        for args in (["bootstrap-recovery-plan", "--facts", "forbidden", "--admission", "file"], ["bootstrap-recover", "--admission", "file"], ["bootstrap-recover", "--plan", "p", "--plan-sha256", "hash", "--output", "forbidden"]):
            with self.subTest(args=args), patch.object(t.sys, "argv", ["trial.py", *args]), patch.object(t, "read", side_effect=AssertionError("unexpected IO")), patch.object(t, "exclusive", side_effect=AssertionError("unexpected lock")), patch("sys.stderr", io.StringIO()):
                with self.assertRaises(SystemExit): t.main()


if __name__ == "__main__": unittest.main()
