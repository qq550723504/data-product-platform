"""Native verifier regressions; synthetic Docker only, no original runtime I/O.

Keep plan, verify, storage_check, command and the PowerShell storage validator
real. Only external Docker/tool replies and unrelated source/origin admission
are replaced. In particular, do not mock verify or storage_check.
"""
from contextlib import ExitStack, contextmanager, redirect_stdout
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch
import uuid

import activation as a
import resume as c
import runtime as r
import trial as t
import upgrade as u
import bootstrap as b
import recovery as recovery

REAL_RUN = subprocess.run
PWSH = shutil.which("pwsh")
SECRET = "synthetic-private-metadata-never-on-argv"


class NativeReadonlyStorage(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if os.name != "nt":
            raise unittest.SkipTest("Native Windows retained-trial verifier")
        if not PWSH:
            if os.name == "nt":
                raise RuntimeError("Native PowerShell required by Windows retained-trial CI")
            raise unittest.SkipTest("PowerShell 7 is not installed")

    def fixture(self, root):
        state = root / "state"; state.mkdir()
        private = root / "private"; private.mkdir()
        (private / "compose.json").write_bytes(b"original-compose")
        (private / "storage-inspect.json").write_bytes(b"original private diagnostic snapshot")
        credential = private / "credential.json"; credential.write_text(json.dumps({"SecretKey": SECRET}))
        (private / "command-error-old.log").write_bytes(b"old diagnostic evidence")
        run_id = "retained-run-\u6d4b\u8bd5"
        s = {"contract": "issue325/stage1", "root": str(r.ROOT), "private": str(private), "runId": run_id,
             "workspaceId": str(uuid.uuid4()), "actorId": str(uuid.uuid4()), "phase": "STOPPED",
             "host": {"daemonId": "synthetic-daemon"}, "containers": {}, "images": {}, "volumes": {},
             "processes": {}, "ports": {}, "resourceIds": {}, "binaries": {}, "sources": {},
             "composeSha256": r.digest((private / "compose.json").read_bytes()),
             "privateHashes": {str(credential): r.digest(credential.read_bytes())}, "secretReferences": [str(credential)],
             "originSnapshot": {"sha256": "old-origin"}, "businessBinding": {"path": "old", "sha256": "old"}}
        containers = {}
        for role, data_path in r.ROLES.items():
            name, volume = "retained-" + role, "volume-" + role
            item = {"Id": "cid-" + role, "Name": "/" + name, "Image": "image-" + role,
                    "Config": {"Labels": {r.LABEL: run_id}, "Env": ["PRIVATE=" + SECRET]},
                    "HostConfig": {"PortBindings": {}, "Tmpfs": {}}, "State": {"Status": "exited"},
                    "NetworkSettings": {"Networks": {"retained": {}}},
                    "Mounts": [{"Type": "volume", "Name": volume, "Destination": data_path, "RW": True}]}
            containers[name] = item
            s["containers"][role] = {"id": item["Id"], "name": name, "configHash": r.container_hash(item)}
            s["images"][role] = {"id": item["Image"]}; s["volumes"][role] = volume
            s["resourceIds"]["volume:" + volume] = "created-" + role
        outputs = {"standardized": "dataset-3", "curated": "dataset-4"}
        versions = [{"id": str(uuid.uuid4()), "dataset_id": "dataset-" + str(i), "status": "READY",
                     "checksum_algorithm": "SHA256", "checksum_value": r.digest(bytes([i])), "byte_size": 1, "storage_uri": "s3://raw/datasets/" + str(i)} for i in range(5)]
        facts = {"workspaceId": s["workspaceId"], "facts": {"versions": versions, "bindings": [{"version_id": v["id"]} for v in versions[:3]]}}
        facts_path = state / "facts.json"; c.write_once(facts_path, facts)
        custody = []
        for i in (3, 4):
            file = private / (str(i) + ".csv"); file.write_bytes(bytes([i]))
            custody.append({"versionId": versions[i]["id"], "file": str(file)})
        custody_path = root / "custody.json"; c.write_once(custody_path, custody)
        c.write_once(state / "state.json", s)
        return s, containers, facts_path, custody_path, {"business": {"outputs": outputs}}

    def snapshot(self, root):
        return {p.relative_to(root).as_posix(): (p.read_bytes(), p.stat().st_mtime_ns)
                for p in root.rglob("*") if p.is_file()}

    @contextmanager
    def external_replies(self, s, containers, *, docker_failure=False):
        calls = []

        def run(args, **kwargs):
            args = [str(x) for x in args]; calls.append((args, kwargs.get("input")))
            if args[0] == "pwsh":
                self.assertNotIn(SECRET, " ".join(args))
                return REAL_RUN([PWSH, *args[1:]], **kwargs)
            if args[:3] == ["docker", "context", "show"]: data = b"synthetic-context"
            elif args[:3] == ["docker", "context", "inspect"]:
                data = json.dumps([{"Endpoints": {"docker": {"Host": "npipe://synthetic-local"}}}]).encode()
            elif args[:2] == ["docker", "info"]: data = b'{"OSType":"linux","ID":"synthetic-daemon"}'
            elif args[:3] == ["docker", "compose", "version"]: data = b"2.99.0"
            elif args == ["go", "version"]: data = b"go synthetic"
            elif args == ["node", "--version"]: data = b"v22.synthetic"
            elif args[:2] == ["docker", "inspect"]:
                if docker_failure: return subprocess.CompletedProcess(args, 17, SECRET.encode(), SECRET.encode())
                identity = args[2]
                item = containers.get(identity) or next(x for x in containers.values() if x["Id"] == identity)
                data = json.dumps([item]).encode()
            elif args[:3] == ["docker", "volume", "inspect"]:
                role = args[3].removeprefix("volume-")
                data = json.dumps([{"Labels": {r.LABEL: s["runId"]}, "CreatedAt": "created-" + role}]).encode()
            else:
                self.fail("Unexpected external action: " + repr(args))
            return subprocess.CompletedProcess(args, 0, data, b"")

        with patch.object(r.subprocess, "run", side_effect=run), patch.object(r.shutil, "which", return_value="synthetic-tool"), patch.dict(r.os.environ, {"DOCKER_HOST": "", "DOCKER_CONTEXT": ""}):
            yield calls

    @contextmanager
    def admission(self, old):
        # Source/origin admission has separate tamper tests. All runtime
        # verification and storage validation remain real in this test graph.
        def accepted(s):
            root = Path(s["private"]).parent
            return {"facts": a.fingerprint(root / "state/facts.json"), "manifest": a.fingerprint(root / "custody.json")}
        with patch.object(c, "verify_origin"), patch.object(u, "verify_business", return_value=old), patch.object(a, "readiness", side_effect=accepted), patch.object(a, "target", return_value={"head": "a" * 40, "sources": {}, "deltaPaths": []}):
            yield

    def test_bootstrap_cli_plan_real_verify_and_storage_check_are_readonly_even_with_live_hosts(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d); s, containers, facts, _, old = self.fixture(root)
            admission = root / "admission.json"; c.write_once(admission, {"head": "a" * 40, "ciRunId": 1})
            owner = {"pid": 123, "created": 9, "exe": "original.exe"}; s["processes"] = {"api": owner}
            s["ports"] = r.free_ports()
            b.put(root / "private/config.json", {"DSN": f"postgres://postgres:synthetic@127.0.0.1:{s['ports']['core']}/dpp_trial_core", "Private": s["private"], "Repo": s["root"], "Workspace": s["workspaceId"], "Actor": s["actorId"], "Profile": {"Subject": "trial-operator-" + s["runId"], "Bucket": "raw", "StorageEndpoint": "127.0.0.1:" + str(s["ports"]["storage"])}})
            r.write(root / "state/state.json", s); before = self.snapshot(root)
            with patch.object(r, "STATE", root / "state"), patch.object(t, "STATE", root / "state"), self.admission(old), self.external_replies(s, containers) as calls, patch.object(b, "target", return_value={"head": "a" * 40}), patch.object(r, "process_identity", return_value=owner), patch.object(t.sys, "argv", ["trial.py", "bootstrap-plan", "--facts", str(facts), "--admission", str(admission)]), redirect_stdout(io.StringIO()):
                self.assertEqual(t.main(), 0)
                self.assertEqual(t.main(), 0)
            self.assertEqual(self.snapshot(root), before)
            self.assertEqual(len([x for x in calls if x[0][0] == "pwsh"]), 2)

    def test_real_plan_and_native_storage_check_preserve_existing_files(self):
        for existing_snapshot in (True, False):
            with self.subTest(existing_snapshot=existing_snapshot), tempfile.TemporaryDirectory() as d:
                root = Path(d); s, containers, facts, custody, old = self.fixture(root)
                if not existing_snapshot: (root / "private/storage-inspect.json").unlink()
                before = self.snapshot(root)
                with patch.object(r, "STATE", root / "state"), patch.object(t, "STATE", root / "state"), self.admission(old), self.external_replies(s, containers) as calls:
                    results = []
                    for _ in range(2):
                        output = io.StringIO()
                        with patch.object(t.sys, "argv", ["trial.py", "activation-plan", "--facts", str(facts), "--custody", str(custody)]), redirect_stdout(output):
                            self.assertEqual(t.main(), 0)
                        results.append(json.loads(output.getvalue()))
                self.assertEqual(results[0], results[1])
                self.assertEqual(self.snapshot(root), before)
                checks = [x for x in calls if x[0][0] == "pwsh"]
                self.assertEqual(len(checks), 2)
                for args, payload in checks:
                    self.assertIn("-InputFromStdin", args); self.assertNotIn("-InputPath", args)
                    self.assertEqual(json.loads(payload)["run_id"], s["runId"])
                self.assertFalse(r.READ_ONLY_OBSERVATION.get())

    def test_recovery_cli_uses_real_readonly_verify_storage_and_powershell(self):
        with tempfile.TemporaryDirectory() as d, ExitStack() as stack:
            root = Path(d); s, containers, facts, _, old = self.fixture(root)
            s["ports"] = r.free_ports()
            config = root / "private/config.json"
            b.put(config, {"DSN": f"postgres://postgres:synthetic@127.0.0.1:{s['ports']['core']}/dpp_trial_core", "Private": s["private"], "Repo": s["root"], "Workspace": s["workspaceId"], "Actor": s["actorId"], "Profile": {"Subject": "trial-operator-" + s["runId"], "Bucket": "raw", "StorageEndpoint": "127.0.0.1:" + str(s["ports"]["storage"])}})
            r.write(root / "state/state.json", s)
            old_source = {"deploy/trial/bootstrap.py": "pinned"}
            admission = root / "old-admission.json"; b.put(admission, {"head": recovery.PREDECESSOR, "ciRunId": 1})
            for obj, name, value in ((r, "STATE", root / "state"), (t, "STATE", root / "state")):
                stack.enter_context(patch.object(obj, name, value))
            stack.enter_context(self.admission(old))
            calls = stack.enter_context(self.external_replies(s, containers))
            target = stack.enter_context(patch.object(b, "target", return_value={"head": recovery.PREDECESSOR, "sources": old_source, "admission": r.read(admission)}))
            stack.enter_context(patch.object(c, "committed_sources", return_value=old_source))
            original = b.plan(s, "bootstrap", facts=facts, admission=admission)
            identity = uuid.uuid4().hex
            journal = r.STATE / "bootstrap-attempts" / identity; journal.mkdir(parents=True)
            private = root / "private/bootstrap-attempts" / identity; private.mkdir(parents=True)
            b.put(journal / "before-state.json", s); b.put(journal / "plan.json", original)
            for action in ("build", "private-admission"):
                b.put(journal / (action + "-started.json"), {"action": action})
                b.put(journal / (action + "-result.json"), {"status": "PASS"})
            b.put(journal / "failed.json", {"status": "UNKNOWN", "failureType": "Refused", "retained": True})
            for name in ("trial-activate", "migrate"): (private / (name + ".exe")).write_bytes(b"original-tool")
            b.put(private / (uuid.uuid4().hex + "-input.json"), {"configFile": str(config)})
            s.update(phase="UNKNOWN", bootstrap={"attemptId": identity, "status": "UNKNOWN", "receipts": str(journal), "planSha256": r.digest(b.encoded(original))}); r.save(s)
            new_admission = root / "new-admission.json"; b.put(new_admission, {"head": "a" * 40, "ciRunId": 2})
            target.return_value = {"head": "a" * 40, "admission": r.read(new_admission)}
            before, first_call = self.snapshot(root), len(calls)
            for _ in range(2):
                with patch.object(t.sys, "argv", ["trial.py", "bootstrap-recovery-plan", "--admission", str(new_admission)]), redirect_stdout(io.StringIO()):
                    self.assertEqual(t.main(), 0)
            self.assertEqual(before, self.snapshot(root))
            checks = [(argv, data) for argv, data in calls[first_call:] if argv[0] == "pwsh"]
            self.assertEqual(len(checks), 2)
            for argv, data in checks:
                self.assertIn("-InputFromStdin", argv); self.assertNotIn("-InputPath", argv)
                self.assertEqual(json.loads(data)["run_id"], s["runId"])

    def test_native_rejection_through_real_verify_preserves_cache_state_and_logs(self):
        for invalid in ("foreign-volume", "bind-data", "tmpfs-wal", "wrong-state"):
            with self.subTest(invalid=invalid), tempfile.TemporaryDirectory() as d:
                root = Path(d); s, containers, facts, custody, old = self.fixture(root)
                item = containers["retained-core"]
                if invalid == "foreign-volume": item["Mounts"][0]["Name"] = "foreign"
                elif invalid == "bind-data": item["Mounts"][0]["Type"] = "bind"
                elif invalid == "tmpfs-wal": item["HostConfig"]["Tmpfs"] = {"/var/lib/postgresql/data/pg_wal": "rw"}
                else: item["State"]["Status"] = "paused"
                s["containers"]["core"]["configHash"] = r.container_hash(item)
                r.write(root / "state/state.json", s); before = self.snapshot(root)
                with patch.object(r, "STATE", root / "state"), self.admission(old), self.external_replies(s, containers) as calls:
                    with self.assertRaises(r.Refused) as error: a.plan(s, facts, custody)
                self.assertIn("pwsh failed", str(error.exception)); self.assertNotIn(SECRET, str(error.exception))
                self.assertEqual(self.snapshot(root), before)
                self.assertEqual(len([x for x in calls if x[0][0] == "pwsh"]), 1)

    def test_live_host_refusal_after_real_verify_does_not_touch_snapshot(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d); s, containers, facts, custody, old = self.fixture(root)
            owner = {"pid": 123, "created": 9, "exe": "original.exe"}; s["processes"] = {"api": owner}
            r.write(root / "state/state.json", s); before = self.snapshot(root)
            with patch.object(r, "STATE", root / "state"), self.admission(old), self.external_replies(s, containers) as calls, patch.object(r, "process_identity", return_value=owner):
                with self.assertRaisesRegex(r.Refused, "host process still running"): a.plan(s, facts, custody)
            self.assertEqual(self.snapshot(root), before)
            self.assertEqual(len([x for x in calls if x[0][0] == "pwsh"]), 1)

    def test_docker_error_in_real_verify_never_persists_diagnostics(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d); s, containers, facts, custody, old = self.fixture(root); before = self.snapshot(root)
            with patch.object(r, "STATE", root / "state"), self.admission(old), self.external_replies(s, containers, docker_failure=True):
                with self.assertRaises(r.Refused) as error: a.plan(s, facts, custody)
            self.assertNotIn(SECRET, str(error.exception)); self.assertEqual(self.snapshot(root), before)

    def test_completed_replay_uses_real_readonly_verify_storage_path(self):
        for existing_lock in (True, False):
            with self.subTest(existing_lock=existing_lock), tempfile.TemporaryDirectory() as d:
                root = Path(d); s, containers, _, _, old = self.fixture(root)
                plan = root / "plan.json"; c.write_once(plan, {}); digest = r.digest(a.encoded({}))
                s.update(activationBinding={"path": "accepted"}, activation={"status": "PASS"})
                s["controllerBinding"] = {"head": "accepted", "sources": {}}
                r.write(root / "state/state.json", s)
                lock = root / "state/operator.lock"
                if existing_lock: lock.write_bytes(b"0")
                before = self.snapshot(root); before.pop("state/operator.lock", None)
                output = io.StringIO()
                # Keep the actual CLI/operator lock; redirect only its default
                # directory so no lock can be opened outside this fixture.
                with patch.object(r, "STATE", root / "state"), patch.object(t, "STATE", root / "state"), patch.object(t, "exclusive", side_effect=lambda: r.exclusive(root / "state")), patch.object(t.sys, "argv", ["trial.py", "activate", "--plan", str(plan), "--plan-sha256", digest]), redirect_stdout(output), patch.object(a, "verify_binding", return_value={"head": "accepted", "planSha256": digest}), patch.object(c, "verify_binding"), patch.object(u, "verify_business", return_value=old), self.external_replies(s, containers) as calls:
                    self.assertEqual(t.main(), 0)
                self.assertTrue(json.loads(output.getvalue())["replay"])
                after = self.snapshot(root)
                self.assertIn("state/operator.lock", after)
                after.pop("state/operator.lock")
                self.assertEqual(after, before)
                self.assertIn("-InputFromStdin", next(args for args, _ in calls if args[0] == "pwsh"))

    def test_normal_verify_still_persists_snapshot_and_failure_diagnostic(self):
        for fail in (False, True):
            with self.subTest(fail=fail), tempfile.TemporaryDirectory() as d:
                root = Path(d); s, containers, _, _, old = self.fixture(root)
                if fail: containers["retained-core"]["Mounts"][0]["Name"] = "foreign"
                s["containers"]["core"]["configHash"] = r.container_hash(containers["retained-core"])
                with patch.object(r, "STATE", root / "state"), patch.object(u, "verify_business", return_value=old), self.external_replies(s, containers) as calls:
                    if fail:
                        with self.assertRaises(r.Refused): r.verify(s, source=False, running=False)
                    else: r.verify(s, source=False, running=False)
                payload = r.read(root / "private/storage-inspect.json")
                self.assertEqual(payload["run_id"], s["runId"])
                args, data = next(x for x in calls if x[0][0] == "pwsh")
                self.assertIn("-InputPath", args); self.assertNotIn("-InputFromStdin", args); self.assertIsNone(data)
                self.assertEqual((root / "private/command-error-old.log").read_bytes(), b"old diagnostic evidence")
                self.assertEqual(len(list((root / "private").glob("command-error-*.log"))), 2 if fail else 1)

    def test_native_stdin_rejects_malformed_json_and_ambiguous_inputs(self):
        for extra, data in (([], b"{malformed"), (["-InputPath", "must-not-be-read"], b"{}")):
            with self.subTest(extra=extra), tempfile.TemporaryDirectory() as d, patch.object(r, "STATE", Path(d)):
                before = self.snapshot(Path(d))
                with r.read_only_observation():
                    with self.assertRaises(r.Refused):
                        r.command([PWSH, "-NoProfile", "-File", r.ROOT / "deploy/trial/storage-check.ps1", "-AllowedStates", "exited", "-InputFromStdin", *extra], data=data)
                self.assertEqual(self.snapshot(Path(d)), before)

    def test_persistence_guard_refuses_before_creating_or_replacing_files(self):
        with tempfile.TemporaryDirectory() as d, patch.object(r, "STATE", Path(d)):
            existing = Path(d) / "state.json"; r.write(existing, {"original": True}); before = self.snapshot(Path(d))
            with r.read_only_observation():
                for path in (existing, Path(d) / "new.json"):
                    with self.assertRaisesRegex(r.Refused, "cannot persist"): r.write(path, {"bad": True})
                with self.assertRaises(r.Refused): r.save({"bad": True})
            self.assertEqual(self.snapshot(Path(d)), before)
            r.write(existing, {"legitimate": True}); self.assertEqual(r.read(existing), {"legitimate": True})


if __name__ == "__main__": unittest.main()
