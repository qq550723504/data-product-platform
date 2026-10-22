"""Real controller stop/precheck/resume/repair paths; native/provider I/O mocked."""
from contextlib import ExitStack, contextmanager, redirect_stdout, redirect_stderr
import copy
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import uuid

import runtime as r
import resume as c
import bootstrap as b
import protected_resume as p
import stop_receipts as stops
import stop_recovery as recovery
import trial
import test_protected_resume as fixtures


class StopReceiptContracts(unittest.TestCase):
    @contextmanager
    def fixture(self, *, collision=False):
        with tempfile.TemporaryDirectory() as name, ExitStack() as stack:
            root = Path(name).resolve(); state = root / "state"; private = root / "private"
            state.mkdir(); private.mkdir()
            for module, field, value in ((r, "ROOT", root), (r, "STATE", state), (trial, "STATE", state)):
                stack.enter_context(patch.object(module, field, value))
            # exclusive() captured its default STATE at import. Bind the real
            # lock explicitly so parallel suites never lock a real checkout.
            stack.enter_context(patch.object(trial, "exclusive", side_effect=lambda: r.exclusive(state)))
            s, before, after = fixtures.ProtectedResumeContracts().fixture()
            s.update(root=str(root), private=str(private), head="a" * 40, sources={},
                     host={"daemonId": "daemon"}, businessBinding={"path": "original-business", "sha256": "f" * 64},
                     originSnapshot={"sha256": "original-origin"}, processes={}, ports={},
                     images={"core": {"id": "same-image"}}, binaries={}, privateHashes={}, secretReferences=[],
                     volumes={"core": "retained-volume"}, workspaceObjectProfile="same-profile")
            native = {"Id": "c" * 64, "Name": "/same-core", "Image": "same-image",
                      "Config": {"Labels": {r.LABEL: s["runId"]}}, "HostConfig": {}, "Mounts": [],
                      "NetworkSettings": {"Networks": {"private": {}}}, "State": {"Status": "running"}}
            s["containers"] = {"core": {"id": native["Id"], "name": "same-core", "configHash": r.container_hash(native)}}
            legacy = state / "before-stop-facts.json"
            r.write(legacy, {"workspaceId": s["workspaceId"], "historical": "five pre-bootstrap versions"})
            original = legacy.read_bytes(); historical = state / "retained-original.json"; historical.write_bytes(original)
            for stage in b.STAGES:
                attempt = uuid.uuid4().hex; folder = private / (stage + "-attempts") / attempt
                folder.mkdir(parents=True)
                document = {"contract": b.CONTRACT, "stage": stage, "status": "PASS", **stops.identity(s),
                            "previousBusinessBinding": s["businessBinding"], "origin": s["originSnapshot"],
                            "files": [b.file(legacy)] if stage == "bootstrap" else []}
                b.put(folder / "manifest.json", document)
                s[stage] = {"attemptId": attempt, "status": "PASS"}; s[stage + "Binding"] = b.file(folder / "manifest.json")
            folder = private / "activations" / uuid.uuid4().hex; folder.mkdir(parents=True)
            b.put(folder / "manifest.json", {**stops.identity(s), "ledger": {"bootstrap": s["bootstrapBinding"],
                   "custody": s["custodyBinding"], "proof": before["expected"]}})
            s["activationBinding"] = b.file(folder / "manifest.json")
            calls = []; observed = [before]
            def docker(*args, **kw):
                calls.append(args)
                if args[0] == "ps": return native["Id"].encode()
                if args[0] == "inspect": return json.dumps([native]).encode()
                if args[0] in ("start", "stop"):
                    self.assertEqual(args[-1], native["Id"])
                    native["State"]["Status"] = "running" if args[0] == "start" else "exited"
                    return native["Id"].encode()
                raise AssertionError("unexpected native call")
            def verify(*args, **kw):
                # Keep the real historical ledger validator in both the normal
                # precheck and the live collision reproducer.
                b.binding(args[0], "bootstrap"); b.binding(args[0], "custody")
            def invoke(*args, **kw):
                self.assertEqual(args[1], "resume-verify"); calls.append(("protected-probe",))
                observed[0] = after
                return {"status": "PASS", "verifiedVersions": 5, "runtimeRootFallback": False}
            stack.enter_context(patch.object(r, "docker", side_effect=docker))
            stack.enter_context(patch.object(r, "local_docker_identity", return_value={"daemonId": "daemon"}))
            stack.enter_context(patch.object(r, "verify", side_effect=verify))
            stack.enter_context(patch.object(c, "verify_origin"))
            stack.enter_context(patch.object(r, "sources", return_value={}))
            stack.enter_context(patch.object(c, "committed_sources", return_value={}))
            stack.enter_context(patch.object(r, "active_business", return_value={"sources": {}}))
            stack.enter_context(patch.object(r, "command", return_value=("b" * 40).encode()))
            stack.enter_context(patch.object(r, "inspect_facts", side_effect=lambda _: copy.deepcopy(observed[0])))
            stack.enter_context(patch.object(p, "expected", return_value=before["expected"]))
            stack.enter_context(patch.object(p, "observe", side_effect=lambda _: copy.deepcopy(observed[0])))
            stack.enter_context(patch.object(p, "invoke", side_effect=invoke))
            stack.enter_context(patch.object(c, "wait_containers"))
            renew = stack.enter_context(patch.object(c, "refresh_identity", return_value={"nativeSTSInvocations": 1, "expiresAt": "future"}))
            hosts = stack.enter_context(patch.object(c, "launch_hosts"))
            stack.enter_context(patch.object(r, "status", side_effect=lambda value: {"phase": value["phase"]}))
            for function in ("seed_source", "provision_config", "compose", "spawn"):
                stack.enter_context(patch.object(r, function, side_effect=AssertionError("forbidden environment effect")))
            if collision:
                r.write(legacy, before); s["phase"] = "STOPPED"; native["State"]["Status"] = "exited"
                s["stopInspection"] = {"runId": s["runId"], "status": "PASS", "receipt": str(legacy)}
            r.save(s)
            yield root, s, before, historical, original, calls, renew, hosts, stack

    def cli(self, *args):
        out, err = io.StringIO(), io.StringIO()
        with patch.object(trial.sys, "argv", ["trial.py", *map(str, args)]), redirect_stdout(out), redirect_stderr(err):
            code = trial.main()
        return code, json.loads(out.getvalue()) if code == 0 else json.loads(err.getvalue())

    def snapshot(self, root):
        return {str(v.relative_to(root)): (v.read_bytes(), v.stat().st_mtime_ns)
                for v in root.rglob("*") if v.is_file() and v.name != "operator.lock"}

    def reviewed(self, s, historical, root):
        reviewed = recovery.plan(s, historical); path = root / "reviewed.json"; b.put(path, reviewed)
        return path, r.digest(b.encoded(reviewed))

    def test_real_cli_stop_to_resume_keeps_frozen_ledger_and_reads_new_stop_binding(self):
        with self.fixture() as (root, s, before, _, original, calls, renew, hosts, _):
            old_bindings = {key: s[key] for key in ("bootstrapBinding", "custodyBinding", "activationBinding")}
            self.assertEqual(self.cli("stop")[0], 0)
            stopped = r.read(r.STATE / "state.json"); baseline = stops.read(stopped)
            self.assertEqual(baseline, before)
            self.assertEqual((r.STATE / "before-stop-facts.json").read_bytes(), original)
            b.binding(stopped, "bootstrap")
            self.assertEqual(self.cli("resume")[0], 0)
            running = r.read(r.STATE / "state.json")
            self.assertEqual(running["phase"], "RUNNING")
            self.assertEqual({key: running[key] for key in old_bindings}, old_bindings)
            self.assertEqual((r.STATE / "before-stop-facts.json").read_bytes(), original)
            self.assertEqual(len(running["resumeHistory"]), 1)
            self.assertEqual(self.cli("resume")[0], 0)
            renew.assert_called_once(); hosts.assert_called_once()
            self.assertEqual(sum(v[0] == "stop" for v in calls), 1)
            self.assertEqual(sum(v[0] == "start" for v in calls), 1)
            self.assertEqual(sum(v[0] == "protected-probe" for v in calls), 1)

    def test_cli_uses_the_real_lock_in_its_temporary_state_directory(self):
        with self.fixture() as (_, _, _, _, _, calls, renew, hosts, _):
            with r.exclusive(r.STATE):
                code, error = self.cli("stop")
                self.assertEqual(code, 1); self.assertIn("Another trial command", error["reason"])
            self.assertEqual(calls, []); renew.assert_not_called(); hosts.assert_not_called()
            self.assertEqual(self.cli("stop")[0], 0)

    def test_repeat_stop_is_readonly_and_missing_or_tampered_receipt_never_starts(self):
        for mutation in ("facts", "receipt-path", "observation", "identity", "unknown"):
            with self.subTest(mutation=mutation), self.fixture() as (root, s, _, _, _, calls, renew, hosts, _):
                self.assertEqual(self.cli("stop")[0], 0); before = self.snapshot(root)
                self.assertEqual(self.cli("stop")[0], 0); self.assertEqual(before, self.snapshot(root))
                state = r.read(r.STATE / "state.json"); ref = state["stopInspection"]
                if mutation == "facts": Path(ref["receipt"]).write_text("changed")
                elif mutation == "receipt-path": ref["receipt"] = str(root / "foreign.json")
                elif mutation == "observation": Path(ref["observation"]["path"]).write_text("changed")
                elif mutation == "identity": state["workspaceId"] = str(uuid.uuid4())
                else: ref["status"] = "UNKNOWN"
                r.save(state); code, _ = self.cli("resume"); self.assertEqual(code, 1)
                self.assertFalse(any(v[0] == "start" for v in calls)); renew.assert_not_called(); hosts.assert_not_called()

    def test_recovery_real_cli_readonly_plan_execute_replay_and_then_resume(self):
        with self.fixture(collision=True) as (root, s, current, historical, original, calls, renew, hosts, _):
            code, error = self.cli("resume"); self.assertEqual(code, 1); self.assertIn("Ledger artifact drift", error["reason"])
            before = self.snapshot(root)
            first = self.cli("stop-recovery-plan", "--facts", historical)
            self.assertEqual(first[0], 0); self.assertEqual(first, self.cli("stop-recovery-plan", "--facts", historical))
            self.assertEqual(before, self.snapshot(root)); self.assertFalse(any(v[0] in ("start", "stop") for v in calls))
            path, sha = self.reviewed(s, historical, root)
            code, result = self.cli("stop-recover", "--plan", path, "--plan-sha256", sha)
            self.assertEqual(code, 0); self.assertEqual(result["status"], "PASS")
            recovered = r.read(r.STATE / "state.json")
            self.assertEqual(stops.read(recovered), current)
            self.assertEqual((r.STATE / "before-stop-facts.json").read_bytes(), original)
            retained = self.snapshot(root)
            self.assertEqual((code, result), self.cli("stop-recover", "--plan", path, "--plan-sha256", sha))
            self.assertEqual(retained, self.snapshot(root)); renew.assert_not_called(); hosts.assert_not_called()
            self.assertEqual(self.cli("resume")[0], 0); renew.assert_called_once(); hosts.assert_called_once()
            self.assertEqual((r.STATE / "before-stop-facts.json").read_bytes(), original)

    def test_restore_interruption_keeps_unknown_and_same_plan_resumes_without_second_restore(self):
        with self.fixture(collision=True) as (root, s, current, historical, original, calls, renew, hosts, _):
            path, sha = self.reviewed(s, historical, root); restore = recovery.restore
            def interrupted(*args): restore(*args); raise r.Refused("interrupted after atomic restoration")
            with patch.object(recovery, "restore", side_effect=interrupted):
                self.assertEqual(self.cli("stop-recover", "--plan", path, "--plan-sha256", sha)[0], 1)
            state = r.read(r.STATE / "state.json"); self.assertEqual(state["stopRecovery"]["status"], "UNKNOWN")
            self.assertEqual(self.cli("resume")[0], 1); renew.assert_not_called()
            historical_meta = ((r.STATE / "before-stop-facts.json").read_bytes(), (r.STATE / "before-stop-facts.json").stat().st_mtime_ns)
            self.assertEqual(self.cli("stop-recover", "--plan", path, "--plan-sha256", sha)[0], 0)
            self.assertEqual(historical_meta, ((r.STATE / "before-stop-facts.json").read_bytes(), (r.STATE / "before-stop-facts.json").stat().st_mtime_ns))
            self.assertEqual(stops.read(r.read(r.STATE / "state.json")), current)
            self.assertEqual(historical_meta[0], original)
            self.assertFalse(any(v[0] in ("start", "stop", "protected-probe") for v in calls)); renew.assert_not_called(); hosts.assert_not_called()

    def test_manifest_before_registry_commit_replay_finishes_same_claim(self):
        with self.fixture(collision=True) as (root, s, _, historical, _, _, renew, _, _):
            path, sha = self.reviewed(s, historical, root); save = r.save
            def crash(value):
                if value.get("stopRecovery", {}).get("status") == "PASS": raise r.Refused("commit interrupted")
                save(value)
            with patch.object(r, "save", side_effect=crash):
                self.assertEqual(self.cli("stop-recover", "--plan", path, "--plan-sha256", sha)[0], 1)
            self.assertEqual(self.cli("stop-recover", "--plan", path, "--plan-sha256", sha)[0], 0)
            self.assertEqual(len(r.read(r.STATE / "state.json")["stopRecoveryHistory"]), 1); renew.assert_not_called()

    def test_completed_recovery_replay_does_not_rebind_a_later_stop(self):
        with self.fixture(collision=True) as (root, s, _, historical, original, calls, renew, hosts, _):
            path, sha = self.reviewed(s, historical, root)
            completed = self.cli("stop-recover", "--plan", path, "--plan-sha256", sha)
            self.assertEqual(completed[0], 0)
            self.assertEqual(self.cli("resume")[0], 0)
            self.assertEqual(self.cli("stop")[0], 0)
            current = r.read(r.STATE / "state.json")["stopInspection"]
            self.assertNotEqual(current, completed[1]["stopInspection"])
            before = self.snapshot(root)
            self.assertEqual(completed, self.cli("stop-recover", "--plan", path, "--plan-sha256", sha))
            self.assertEqual(before, self.snapshot(root))
            self.assertEqual(r.read(r.STATE / "state.json")["stopInspection"], current)
            self.assertEqual((r.STATE / "before-stop-facts.json").read_bytes(), original)
            renew.assert_called_once(); hosts.assert_called_once()

    def test_recovery_refuses_unsupported_drift_and_changed_plan_before_mutations(self):
        for mutation in ("history-bytes", "foreign-copy", "identity", "current-facts", "other-artifact", "hash", "running", "orphan"):
            with self.subTest(mutation=mutation), self.fixture(collision=True) as (root, s, _, historical, _, calls, renew, hosts, _):
                if mutation == "other-artifact":
                    artifact = Path(s["custodyBinding"]["path"]); value = r.read(artifact)
                    extra = root / "extra.json"; extra.write_bytes(b"old"); value["files"] = [b.file(extra)]
                    r.write(artifact, value); s["custodyBinding"] = b.file(artifact); r.save(s); extra.write_bytes(b"changed")
                    with self.assertRaises(r.Refused): recovery.plan(s, historical)
                    continue
                path, sha = self.reviewed(s, historical, root)
                if mutation == "history-bytes": historical.write_bytes(b"changed")
                elif mutation == "foreign-copy":
                    outside = Path(name := tempfile.mkdtemp()) / "copy.json"
                    try:
                        outside.write_bytes(historical.read_bytes())
                        with self.assertRaises(r.Refused): recovery.plan(s, outside)
                    finally: outside.unlink(); Path(name).rmdir()
                    continue
                elif mutation == "identity": s["actorId"] = str(uuid.uuid4()); r.save(s)
                elif mutation == "current-facts": (r.STATE / "before-stop-facts.json").write_bytes(b"changed")
                elif mutation == "hash": sha = "0" * 64
                elif mutation == "running": s["phase"] = "RUNNING"; r.save(s)
                else: (r.STATE / "stop-recoveries" / "orphan").mkdir(parents=True)
                before = self.snapshot(root); self.assertEqual(self.cli("stop-recover", "--plan", path, "--plan-sha256", sha)[0], 1)
                self.assertEqual(before, self.snapshot(root)); renew.assert_not_called(); hosts.assert_not_called()
                self.assertFalse(any(v[0] in ("start", "stop") for v in calls))


if __name__ == "__main__": unittest.main()
