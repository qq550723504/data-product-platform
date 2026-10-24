"""Real CLI and business validators with temporary Git; native/provider I/O simulated."""
from contextlib import ExitStack, contextmanager, redirect_stdout, redirect_stderr
import copy
import io
from itertools import product
import json
import os
from pathlib import Path
import socket
import tempfile
import unittest
from unittest.mock import patch
import uuid

import runtime as r
import resume as c
import bootstrap as b
import activation as a
import protected_resume as p
import stop_receipts as stops
import stop_recovery as recovery
import trial
import test_protected_resume as fixtures


class StopReceiptContracts(unittest.TestCase):
    @contextmanager
    def fixture(self, *, collision=False, processes=None):
        with tempfile.TemporaryDirectory() as name, ExitStack() as stack:
            root = Path(name).resolve(); state = root / "state"; private = root / "private"
            state.mkdir(); private.mkdir()
            for module, field, value in ((r, "ROOT", root), (r, "STATE", state), (trial, "STATE", state)):
                stack.enter_context(patch.object(module, field, value))
            # exclusive() captured its default STATE at import. Bind the real
            # lock explicitly so parallel suites never lock a real checkout.
            stack.enter_context(patch.object(trial, "exclusive", side_effect=lambda: r.exclusive(state)))
            real_command = r.command
            def git_io(args, **kw):
                # command() captures its default cwd at import; only redirect
                # real Git I/O into this temporary repository, never fake it.
                self.assertEqual(str(args[0]), "git")
                kw.setdefault("cwd", root)
                return real_command(args, **kw)
            stack.enter_context(patch.object(r, "command", side_effect=git_io))
            r.command(["git", "init", "--quiet"])
            for key, value in (("user.name", "Trial fixture"), ("user.email", "fixture@example.invalid"),
                               ("commit.gpgsign", "false"), ("core.hooksPath", str(root / "no-hooks")),
                               ("core.autocrlf", "false")):
                r.command(["git", "config", key, value])
            for relative in r.SOURCE_PATHS:
                path = root / relative
                if relative.endswith(".ps1"):
                    path.parent.mkdir(parents=True, exist_ok=True); path.write_text("# fixture\n")
                else:
                    path.mkdir(parents=True, exist_ok=True); (path / ".fixture").write_text("fixture\n")
            business_source = root / "apps/platform/fixture.go"
            business_source.write_text("package fixture\n")
            controller_source = root / "deploy/trial/fixture.py"
            controller_source.write_text("# original controller\n")
            r.command(["git", "add", "--", *r.SOURCE_PATHS])
            r.command(["git", "commit", "--quiet", "-m", "Original business sources"])
            business_head = r.command(["git", "rev-parse", "HEAD"]).decode().strip()
            business_sources = c.committed_sources(business_head)
            controller_source.write_text("# recovery controller\n")
            r.command(["git", "add", "deploy/trial/fixture.py"])
            r.command(["git", "commit", "--quiet", "-m", "Controller only"])
            s, before, after = fixtures.ProtectedResumeContracts().fixture()
            s.pop("activationBinding")
            s.update(contract="issue325/stage1", root=str(root), private=str(private),
                     head=business_head, sources=business_sources, project="fixture-project",
                     host={"daemonId": "daemon"}, processes=copy.deepcopy(processes or {}), ports={}, subnets=[], networks={},
                     images={"core": {"id": "same-image"}}, minio={}, resourceIds={},
                     api="http://127.0.0.1:1", web="http://127.0.0.1:2", webBuild={},
                     binaries={}, privateHashes={}, secretReferences=[], volumes={"core": "retained-volume"})
            native = {"Id": "c" * 64, "Name": "/same-core", "Image": "same-image",
                      "Config": {"Labels": {r.LABEL: s["runId"]}}, "HostConfig": {}, "Mounts": [],
                      "NetworkSettings": {"Networks": {"private": {}}}, "State": {"Status": "running"}}
            s["containers"] = {"core": {"id": native["Id"], "name": "same-core", "configHash": r.container_hash(native)}}
            r.write(private / "compose.json", {})
            s["composeSha256"] = r.digest((private / "compose.json").read_bytes())
            b.put(state / "resume-origin.json", s)
            s["originSnapshot"] = {"sha256": r.digest((state / "resume-origin.json").read_bytes())}
            human = private / "human-token"; human.write_text("synthetic-only-token")
            s["privateHashes"][str(human)] = r.digest(human.read_bytes())
            def artifacts(folder):
                folder.mkdir(parents=True)
                binary = folder / "api.exe"; binary.write_bytes(b"synthetic binary")
                web = folder / "web"; web.mkdir(); (web / "server.js").write_bytes(b"synthetic web")
                return {"binaries": {"api": b.file(binary)},
                        "web": {"root": str(web), "files": {"server.js": r.digest((web / "server.js").read_bytes())}}}
            business_folder = private / "upgrades" / uuid.uuid4().hex
            business = {"workflowVersionId": str(uuid.uuid4()), "outputs": {"standardized": "same-standardized", "curated": "same-curated"}}
            business_artifacts = artifacts(business_folder)
            b.put(business_folder / "manifest.json", {**stops.identity(s), "head": business_head, "sources": business_sources,
                   "business": business, "humanTokenFile": str(human), "artifacts": business_artifacts})
            s["businessBinding"] = b.file(business_folder / "manifest.json")
            profile = private / "object-profile.json"
            r.write(profile, {"Endpoint": "127.0.0.1:3", "Secure": False, "CredentialFile": str(private / "credential"),
                              "Scope": {"WorkspaceID": s["workspaceId"], "ActorID": s["actorId"],
                                        "PrincipalRef": p.identity(s)["principalRef"], "Bucket": "raw",
                                        "ExpiresAt": "2099-01-01T00:00:00+00:00"}})
            s["workspaceObjectProfile"] = str(profile)
            s["privateHashes"][str(profile)] = r.digest(profile.read_bytes())
            legacy = state / "before-stop-facts.json"
            r.write(legacy, {"workspaceId": s["workspaceId"], "historical": "five pre-bootstrap versions"})
            original = legacy.read_bytes(); historical = state / "retained-original.json"; historical.write_bytes(original)
            for stage in b.STAGES:
                attempt = uuid.uuid4().hex; folder = private / (stage + "-attempts") / attempt
                folder.mkdir(parents=True)
                evidence = folder / "evidence.json"; b.put(evidence, {"stage": stage})
                document = {"contract": b.CONTRACT, "stage": stage, "status": "PASS", **stops.identity(s),
                            "previousBusinessBinding": s["businessBinding"], "origin": s["originSnapshot"],
                            "files": [*([b.file(legacy)] if stage == "bootstrap" else []), b.file(evidence)]}
                b.put(folder / "manifest.json", document)
                s[stage] = {"attemptId": attempt, "status": "PASS"}; s[stage + "Binding"] = b.file(folder / "manifest.json")
            folder = private / "activations" / uuid.uuid4().hex
            activated_artifacts = artifacts(folder)
            b.put(folder / "manifest.json", {"contract": a.CONTRACT, **stops.identity(s),
                   "previousBusinessBinding": s["businessBinding"], "business": business, "humanTokenFile": str(human),
                   "head": business_head, "sources": business_sources, "artifacts": activated_artifacts,
                   "protectedIdentity": a.profile_identity(r.read(profile)), "ledger": {"bootstrap": s["bootstrapBinding"],
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
            def invoke(*args, **kw):
                self.assertEqual(args[1], "resume-verify"); calls.append(("protected-probe",))
                observed[0] = after
                return {"status": "PASS", "verifiedVersions": 5, "runtimeRootFallback": False}
            stack.enter_context(patch.object(r, "docker", side_effect=docker))
            stack.enter_context(patch.object(r, "local_docker_identity", return_value={"daemonId": "daemon"}))
            stack.enter_context(patch.object(r, "preflight", return_value={"daemonId": "daemon"}))
            stack.enter_context(patch.object(r, "storage_check"))  # native PowerShell adapter only
            stack.enter_context(patch.object(p, "observe", side_effect=lambda _: copy.deepcopy(observed[0])))
            stack.enter_context(patch.object(p, "invoke", side_effect=invoke))
            stack.enter_context(patch.object(c, "wait_containers"))
            renew = stack.enter_context(patch.object(c, "refresh_identity", return_value={"nativeSTSInvocations": 1, "expiresAt": "2099-01-01T00:00:00+00:00"}))
            hosts = stack.enter_context(patch.object(c, "launch_hosts"))
            stack.enter_context(patch.object(r, "http", return_value=True))
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

    def snapshot(self, root, *, include_lock=False):
        return {str(v.relative_to(root)): (v.read_bytes(), v.stat().st_mtime_ns)
                for v in root.rglob("*") if v.is_file() and (include_lock or v.name != "operator.lock")}

    def reviewed(self, s, historical, root):
        reviewed = recovery.plan(s, historical); path = root / "reviewed.json"; b.put(path, reviewed)
        return path, r.digest(b.encoded(reviewed))

    @unittest.skipUnless(os.name == "nt", "Windows full process identity contract")
    def test_stopped_owner_reused_live_pid_real_cli_plan_recover_and_replay(self):
        actual = r.process_identity(os.getpid())
        for field in ("created", "exe"):
            owner = copy.deepcopy(actual)
            if field == "created": owner[field] += 1
            else: owner[field] += ".different-owner"
            with self.subTest(field=field), self.fixture(collision=True, processes={"worker": owner}) as (root, s, current, historical, original, calls, renew, hosts, _):
                # The native Windows adapter is real: this PID is alive, but
                # its creation/executable tuple is not the recorded worker.
                with patch.object(r, "process_handle", wraps=r.process_handle) as handles:
                    for live_hosts in (False, True):
                        with self.assertRaisesRegex(r.Refused, "identity drift"):
                            b.native(s, hosts=live_hosts)
                    before = self.snapshot(root, include_lock=True)
                    code, output = self.cli("stop-recovery-plan", "--facts", historical)
                    self.assertEqual(code, 0, output)
                    self.assertEqual(output["plan"]["native"]["processes"], {"worker": None})
                    self.assertEqual(before, self.snapshot(root, include_lock=True))
                    path = root / "reviewed.json"; b.put(path, output["plan"])
                    args = ("stop-recover", "--plan", path, "--plan-sha256", output["planSha256"])
                    completed = self.cli(*args); self.assertEqual(completed[0], 0, completed)
                    self.assertEqual(stops.read(r.read(r.STATE / "state.json")), current)
                    self.assertEqual((r.STATE / "before-stop-facts.json").read_bytes(), original)
                    retained = self.snapshot(root); observations = handles.call_count
                    self.assertEqual(completed, self.cli(*args))
                    self.assertEqual(retained, self.snapshot(root)); self.assertEqual(handles.call_count, observations)
                    self.assertTrue(handles.call_args_list)
                    self.assertTrue(all(not call.args[1] for call in handles.call_args_list))
                self.assertEqual(r.process_identity(os.getpid()), actual)
                self.assertEqual(r.read(r.STATE / "state.json")["processes"], {"worker": owner})
                self.assertFalse(any(call[0] in ("start", "stop", "protected-probe") for call in calls))
                renew.assert_not_called(); hosts.assert_not_called()

    @unittest.skipUnless(os.name == "nt", "Windows full process identity contract")
    def test_stopped_owner_matching_live_identity_refuses_readonly_cli_plan(self):
        owner = r.process_identity(os.getpid())
        with self.fixture(collision=True, processes={"worker": owner}) as (root, s, _, historical, _, calls, renew, hosts, _):
            with self.assertRaisesRegex(r.Refused, "Recorded host still live"): b.native(s)
            self.assertEqual(b.native(s, hosts=True)["processes"], {"worker": owner})
            before = self.snapshot(root, include_lock=True)
            output = root / "must-not-exist.json"
            code, error = self.cli("stop-recovery-plan", "--facts", historical, "--output", output)
            self.assertEqual(code, 1, error); self.assertIn("Recorded host still live", error["reason"])
            self.assertEqual(before, self.snapshot(root, include_lock=True))
            self.assertFalse(output.exists()); self.assertFalse((r.STATE / "stop-recoveries").exists())
            self.assertFalse(any(call[0] in ("start", "stop", "protected-probe") for call in calls))
            renew.assert_not_called(); hosts.assert_not_called()

    @unittest.skipUnless(os.name == "nt", "Windows process absence adapter contract")
    def test_stopped_owner_absent_or_unreadable_native_identity_real_cli_plan(self):
        owner = r.process_identity(os.getpid())
        for unavailable in (False, True):
            with self.subTest(unavailable=unavailable), self.fixture(collision=True, processes={"worker": owner}) as (root, _, _, historical, _, calls, renew, hosts, _):
                before = self.snapshot(root, include_lock=True)
                # Only native handle I/O is simulated; real process_identity,
                # CLI, committed sources and every business validator execute.
                native = {"side_effect": r.Refused("Process identity unavailable")} if unavailable else {"return_value": (None, None)}
                with patch.object(r, "process_handle", **native) as handles:
                    code, output = self.cli("stop-recovery-plan", "--facts", historical)
                self.assertEqual(code, 1 if unavailable else 0, output)
                if unavailable: self.assertIn("Process identity unavailable", output["reason"])
                else: self.assertEqual(output["plan"]["native"]["processes"], {"worker": None})
                self.assertEqual(before, self.snapshot(root, include_lock=True))
                self.assertTrue(all(not call.args[1] for call in handles.call_args_list))
                self.assertFalse(any(call[0] in ("start", "stop", "protected-probe") for call in calls))
                renew.assert_not_called(); hosts.assert_not_called()

    @unittest.skipUnless(os.name == "nt", "Windows process retry contract")
    def test_stopped_owner_retry_rechecks_identity_before_claim_or_archive_writes(self):
        owner = r.process_identity(os.getpid()); owner["created"] += 1
        for claimed in (False, True):
            with self.subTest(claimed=claimed), self.fixture(collision=True, processes={"worker": owner}) as (root, s, _, historical, _, calls, renew, hosts, _):
                path, sha = self.reviewed(s, historical, root); save = r.save
                def crash(value):
                    if value.get("stopRecovery", {}).get("status") == "UNKNOWN":
                        if claimed: save(value)
                        raise r.Refused("Interrupted at recovery claim")
                    save(value)
                args = ("stop-recover", "--plan", path, "--plan-sha256", sha)
                with patch.object(r, "save", side_effect=crash):
                    self.assertEqual(self.cli(*args)[0], 1)
                self.assertTrue((r.STATE / "stop-recoveries" / sha / "started.json").exists())
                before = self.snapshot(root)
                with patch.object(r, "process_handle", side_effect=r.Refused("Process identity unavailable")):
                    code, error = self.cli(*args)
                self.assertEqual(code, 1, error); self.assertIn("Process identity unavailable", error["reason"])
                self.assertEqual(before, self.snapshot(root), "Refused retry changed its claim or recovery archives")
                completed = self.cli(*args); self.assertEqual(completed[0], 0, completed)
                retained = self.snapshot(root)
                self.assertEqual(completed, self.cli(*args)); self.assertEqual(retained, self.snapshot(root))
                self.assertEqual(r.read(r.STATE / "state.json")["phase"], "STOPPED")
                self.assertFalse(any(call[0] in ("start", "stop", "protected-probe") for call in calls))
                renew.assert_not_called(); hosts.assert_not_called()

    @unittest.skipUnless(os.name == "nt", "Windows stopped-owner port contract")
    def test_stopped_owner_reused_pid_still_requires_original_port_available(self):
        owner = r.process_identity(os.getpid()); owner["created"] += 1
        with self.fixture(collision=True, processes={"worker": owner}) as (root, s, _, historical, _, calls, renew, hosts, _), socket.socket() as occupied:
            occupied.bind(("127.0.0.1", 0)); s["ports"] = {"api": occupied.getsockname()[1]}; r.save(s)
            before = self.snapshot(root, include_lock=True)
            code, error = self.cli("stop-recovery-plan", "--facts", historical)
            self.assertEqual(code, 1, error); self.assertIn("Original ledger port occupied", error["reason"])
            self.assertEqual(before, self.snapshot(root, include_lock=True))
            self.assertFalse(any(call[0] in ("start", "stop", "protected-probe") for call in calls))
            renew.assert_not_called(); hosts.assert_not_called()

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
            self.assertEqual(first[0], 0, first); self.assertEqual(first, self.cli("stop-recovery-plan", "--facts", historical))
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

    def test_real_business_chain_stays_strict_while_plan_is_readonly_with_one_explicit_output(self):
        with self.fixture(collision=True) as (root, s, _, historical, _, calls, renew, hosts, _):
            for ordinary in (lambda: a.verify_binding(s), lambda: r.active_business(s),
                             lambda: c.check_sources(s, r.sources()), lambda: r.verify(s, source=False, running=False)):
                with self.assertRaisesRegex(r.Refused, "Ledger artifact drift"):
                    ordinary()
            before = self.snapshot(root, include_lock=True)
            code, first = self.cli("stop-recovery-plan", "--facts", historical)
            self.assertEqual(code, 0, first)
            self.assertEqual(before, self.snapshot(root, include_lock=True))
            output = root / "reviewed-plan.json"
            code, second = self.cli("stop-recovery-plan", "--facts", historical, "--output", output)
            self.assertEqual(code, 0, second); self.assertEqual(first, second)
            after = self.snapshot(root, include_lock=True)
            self.assertEqual(set(after) - set(before), {"reviewed-plan.json"})
            self.assertEqual(before, {k: after[k] for k in before})
            self.assertEqual(r.read(output), first["plan"])
            self.assertEqual(r.digest(b.encoded(r.read(output))), first["planSha256"])
            self.assertEqual(self.cli("stop-recover", "--plan", output, "--plan-sha256", first["planSha256"])[0], 0)
            recovered = r.read(r.STATE / "state.json")
            self.assertEqual(r.active_business(recovered), a.verify_binding(recovered))
            self.assertEqual(c.check_sources(recovered, r.sources()), ["deploy/trial/fixture.py"])
            self.assertFalse(any(call[0] in ("start", "stop", "protected-probe") for call in calls))
            renew.assert_not_called(); hosts.assert_not_called()

    def test_recovery_source_admission_checks_real_git_and_all_frozen_business_artifacts(self):
        cases = {"activation-hash": "Activation manifest drift", "activation-identity": "Activation identity",
                 "previous-business": "Activation identity", "ledger-reference": "Activation ledger",
                 "activated-binary": "Activated binary drift", "activated-web": "Activated production Web drift",
                 "business-binary": "Upgraded binary drift", "business-web": "Upgraded production Web drift",
                 "business-sources": "Business source manifest drift", "activation-sources": "Activation source manifest drift",
                 "profile-scope": "Protected runtime scope drift", "profile-binding": "Unbound protected profile",
                 "dirty-controller": "Committed recovery controller required", "committed-business": "Business source drift",
                 "bootstrap-extra": "Unrelated ledger artifact drift", "custody-extra": "Unrelated ledger artifact drift"}
        for mutation, reason in cases.items():
            with self.subTest(mutation=mutation), self.fixture(collision=True) as (root, s, _, historical, _, calls, renew, hosts, _):
                activation_path = Path(s["activationBinding"]["path"])
                activated = r.read(activation_path)
                business = r.read(s["businessBinding"]["path"])
                if mutation == "activation-hash": activation_path.write_bytes(b"changed")
                elif mutation in ("activation-identity", "previous-business", "ledger-reference", "activation-sources"):
                    if mutation == "activation-identity": activated["actorId"] = str(uuid.uuid4())
                    elif mutation == "previous-business": activated["previousBusinessBinding"] = {"path": "foreign", "sha256": "0" * 64}
                    elif mutation == "ledger-reference": activated["ledger"]["custody"] = {"path": "foreign", "sha256": "0" * 64}
                    else: activated["sources"] = {}
                    r.write(activation_path, activated); s["activationBinding"] = b.file(activation_path)
                elif mutation in ("activated-binary", "business-binary"):
                    value = activated if mutation == "activated-binary" else business
                    Path(value["artifacts"]["binaries"]["api"]["path"]).write_bytes(b"changed")
                elif mutation in ("activated-web", "business-web"):
                    value = activated if mutation == "activated-web" else business
                    (Path(value["artifacts"]["web"]["root"]) / "server.js").write_bytes(b"changed")
                elif mutation == "business-sources":
                    business["sources"] = {}
                    path = Path(s["businessBinding"]["path"]); r.write(path, business)
                    s["businessBinding"] = b.file(path)
                    activated["previousBusinessBinding"] = s["businessBinding"]
                    r.write(activation_path, activated); s["activationBinding"] = b.file(activation_path)
                elif mutation in ("profile-scope", "profile-binding"):
                    profile = r.read(s["workspaceObjectProfile"])
                    if mutation == "profile-scope": profile["Scope"]["WorkspaceID"] = str(uuid.uuid4())
                    else: profile["CredentialFile"] += "-unbound"
                    r.write(s["workspaceObjectProfile"], profile)
                elif mutation in ("dirty-controller", "committed-business"):
                    path = root / ("deploy/trial/fixture.py" if mutation == "dirty-controller" else "apps/platform/fixture.go")
                    path.write_text("changed\n")
                    if mutation == "committed-business":
                        r.command(["git", "add", "apps/platform/fixture.go"])
                        r.command(["git", "commit", "--quiet", "-m", "Business source mutation"])
                else:
                    stage = "bootstrap" if mutation == "bootstrap-extra" else "custody"
                    document = r.read(s[stage + "Binding"]["path"])
                    Path(document["files"][-1]["path"]).write_bytes(b"changed")
                r.save(s)
                before = self.snapshot(root, include_lock=True)
                code, error = self.cli("stop-recovery-plan", "--facts", historical)
                self.assertEqual(code, 1, error); self.assertIn(reason, error["reason"])
                self.assertEqual(before, self.snapshot(root, include_lock=True))
                self.assertFalse(any(call[0] in ("start", "stop", "protected-probe") for call in calls))
                renew.assert_not_called(); hosts.assert_not_called()

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

    def test_recovery_retry_rechecks_ledger_before_any_write(self):
        for cut, stage, kind in product(("journal", "archived", "restored", "manifest"), b.STAGES, ("file", "manifest")):
            with self.subTest(cut=cut, stage=stage, kind=kind), self.fixture(collision=True) as (root, s, _, historical, _, calls, renew, hosts, _):
                path, sha = self.reviewed(s, historical, root)
                save, restore = r.save, recovery.restore
                def crash_save(value):
                    if value.get("stopRecovery", {}).get("status") == ("UNKNOWN" if cut == "journal" else "PASS"):
                        raise r.Refused("interrupted registry commit")
                    save(value)
                def crash_restore(*args):
                    if cut == "restored": restore(*args)
                    raise r.Refused("interrupted restoration")
                module, name, crash = (r, "save", crash_save) if cut in ("journal", "manifest") else (recovery, "restore", crash_restore)
                with patch.object(module, name, side_effect=crash):
                    self.assertEqual(self.cli("stop-recover", "--plan", path, "--plan-sha256", sha)[0], 1)
                self.assertTrue((r.STATE / "stop-recoveries" / sha / "started.json").exists())
                manifest = Path(s[stage + "Binding"]["path"])
                artifact = manifest if kind == "manifest" else Path(r.read(manifest)["files"][-1]["path"])
                clean = artifact.read_bytes(); artifact.write_bytes(clean + b"\nchanged")
                target = r.STATE / "before-stop-facts.json"; target_bytes = target.read_bytes()
                # A mutating CLI acquires the existing operator lock; compare
                # all recovery/business files, excluding only that lock file.
                before = self.snapshot(root)
                code, error = self.cli("stop-recover", "--plan", path, "--plan-sha256", sha)
                self.assertEqual(code, 1, error)
                self.assertEqual(target.read_bytes(), target_bytes, "Refused retry restored the target before checking ledger drift")
                self.assertEqual(before, self.snapshot(root))
                self.assertIn("Ledger binding drift" if kind == "manifest" else "Unrelated ledger artifact drift", error["reason"])
                # Removing only the injected drift lets this exact journal finish.
                artifact.write_bytes(clean)
                completed = self.cli("stop-recover", "--plan", path, "--plan-sha256", sha)
                self.assertEqual(completed[0], 0, completed)
                retained = self.snapshot(root)
                self.assertEqual(completed, self.cli("stop-recover", "--plan", path, "--plan-sha256", sha))
                self.assertEqual(retained, self.snapshot(root))
                self.assertEqual(r.read(r.STATE / "state.json")["phase"], "STOPPED")
                self.assertFalse(any(v[0] in ("start", "stop", "protected-probe") for v in calls))
                renew.assert_not_called(); hosts.assert_not_called()

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
