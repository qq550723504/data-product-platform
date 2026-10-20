"""Activation contracts: mocks only; no original SQL, daemon or provider calls."""
from contextlib import ExitStack
import copy
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import Mock, patch
import uuid

import activation as a
import runtime as r
import resume as c
import upgrade as u


class ActivationContracts(unittest.TestCase):
    def fixture(self, folder):
        r.STATE.mkdir()
        private = folder / "private"; private.mkdir()
        s = {"phase": "RUNNING", "runId": "kept", "workspaceId": str(uuid.uuid4()), "actorId": str(uuid.uuid4()),
             "root": str(folder), "private": str(private), "originSnapshot": {"sha256": "old-origin"}, "businessBinding": {"path": "old-manifest", "sha256": "old"},
             "containers": {"core": {"id": "same-core"}}, "ports": {}, "processes": {}, "privateHashes": {}, "binaries": {}}
        versions = [{"id": str(uuid.uuid4()), "dataset_id": str(i), "status": "READY", "checksum_algorithm": "SHA256", "checksum_value": r.digest(bytes([i])), "byte_size": 1} for i in range(5)]
        facts = {"workspaceId": s["workspaceId"], "objects": [], "facts": {"versions": versions, "bindings": [{"version_id": v["id"]} for v in versions[:3]], "receipts": [], "operations": []}}
        facts_path = r.STATE / "facts.json"; c.write_once(facts_path, facts)
        custody = []
        for i in (3, 4):
            p = folder / (str(i) + ".csv"); p.write_bytes(bytes([i])); custody.append({"versionId": versions[i]["id"], "file": str(p)})
        custody_path = folder / "custody.json"; c.write_once(custody_path, custody)
        old = {"head": "a" * 40, "business": {"outputs": {"standardized": "3", "curated": "4"}}, "humanTokenFile": "old-human-token"}
        r.save(s)
        return s, facts_path, custody_path, old

    def plan_mocks(self, stack, old):
        for obj, name in ((r, "verify"), (c, "verify_origin")):
            stack.enter_context(patch.object(obj, name))
        stack.enter_context(patch.object(u, "verify_business", return_value=old))
        stack.enter_context(patch.object(a, "target", return_value={"head": "b" * 40, "sources": {}, "deltaPaths": []}))
        stack.enter_context(patch.object(a, "stopped", return_value={"core": "exited"}))

    def test_plan_is_stable_and_never_writes_or_invokes_mutations(self):
        with tempfile.TemporaryDirectory() as d, patch.object(r, "STATE", Path(d) / "state"), ExitStack() as stack:
            s, facts, custody, old = self.fixture(Path(d)); self.plan_mocks(stack, old)
            originals = {str(p): p.read_bytes() for p in Path(d).rglob("*") if p.is_file()}
            for name in ("save", "docker", "spawn", "seed_source", "provision_config", "command"):
                stack.enter_context(patch.object(r, name, side_effect=AssertionError("plan mutation")))
            first = a.plan(s, facts, custody); second = a.plan(s, facts, custody)
            self.assertEqual(a.encoded(first), a.encoded(second)); self.assertEqual(first["recordedPhase"], "RUNNING")
            self.assertEqual(originals, {str(p): p.read_bytes() for p in Path(d).rglob("*") if p.is_file()})

    def test_custody_and_version_membership_fail_closed(self):
        with tempfile.TemporaryDirectory() as d, patch.object(r, "STATE", Path(d) / "state"):
            s, facts, custody, old = self.fixture(Path(d))
            with patch.object(u, "verify_business", return_value=old):
                for broken in ([], r.read(custody)[:1], r.read(custody) * 2):
                    with self.assertRaises(r.Refused): a.versions(s, r.read(facts), broken)
                Path(r.read(custody)[0]["file"]).write_bytes(b"wrong")
                with self.assertRaisesRegex(r.Refused, "Custody bytes"): a.versions(s, r.read(facts), r.read(custody))

    def test_unknown_attempt_or_orphan_prevents_any_new_native_action(self):
        with tempfile.TemporaryDirectory() as d, patch.object(r, "STATE", Path(d) / "state"), patch.object(r, "verify") as verify:
            s, facts, custody, _ = self.fixture(Path(d))
            for phase in ("UNKNOWN", "ACTIVATING", "RESUMING"):
                with self.assertRaises(r.Refused): a.plan({**s, "phase": phase}, facts, custody)
            with self.assertRaises(r.Refused): a.plan({**s, "activation": {"status": "UNKNOWN"}}, facts, custody)
            (Path(s["private"]) / "activations" / "orphan").mkdir(parents=True)
            with self.assertRaisesRegex(r.Refused, "orphan"): a.plan(s, facts, custody)
            verify.assert_not_called()
            with self.assertRaisesRegex(r.Refused, "explicit recovery"): c.resume({"phase": "STOPPED", "activation": {"status": "UNKNOWN"}})

    def test_native_live_or_reused_pid_and_occupied_port_are_read_only_refusals(self):
        owner = {"pid": 22, "created": 1, "exe": "original.exe"}
        s = {"containers": {}, "processes": {"api": owner}, "ports": {}}
        with patch.object(r, "process_identity", return_value=owner) as identity:
            with self.assertRaises(r.Refused): a.stopped(s)
            identity.assert_called_once_with(22, expected=owner, allow_missing=True)
        sock = Mock(); sock.bind.side_effect = OSError()
        with patch.object(a.socket, "socket", return_value=sock):
            with self.assertRaisesRegex(r.Refused, "port unavailable"): a.stopped({"containers": {}, "processes": {}, "ports": {"api": 1234}})
        sock.close.assert_called_once()

    def test_original_row_multiset_and_columns_cannot_be_rewritten_or_lost(self):
        before = {"tables": {"old": {"columns": ["id", "amount"], "rows": ['{"id":1,"amount":9007199254740993}'] * 2}}}
        after = copy.deepcopy(before); after["tables"]["old"]["rows"].append('{"id":2}')
        after["tables"]["new"] = {"columns": ["id"], "rows": []}; a.preserve(before, after)
        for change in (lambda t: t["old"]["rows"].pop(0), lambda t: t["old"].update(columns=["id"]), lambda t: t.pop("old")):
            broken = copy.deepcopy(before); change(broken["tables"])
            with self.assertRaises(r.Refused): a.preserve(before, broken)

    def test_plan_tamper_or_native_admission_drift_never_executes(self):
        with tempfile.TemporaryDirectory() as d:
            p = Path(d) / "plan.json"; expected = {"facts": {"path": "facts"}, "custody": {"path": "custody"}}
            c.write_once(p, expected); digest = r.digest(a.encoded(expected))
            with patch.object(a, "execute") as execute, patch.object(a, "plan", return_value={"drift": True}):
                with self.assertRaisesRegex(r.Refused, "digest"): a.activate({}, p, "wrong")
                with self.assertRaisesRegex(r.Refused, "admission drift"): a.activate({}, p, digest)
            execute.assert_not_called()

    def test_completed_replay_only_verifies_and_preserves_every_file(self):
        with tempfile.TemporaryDirectory() as d:
            p = Path(d) / "plan.json"; c.write_once(p, {})
            digest = r.digest(a.encoded({})); s = {"phase": "STOPPED", "runId": "same", "workspaceId": "same", "activationBinding": {"path": "accepted"}, "activation": {"status": "PASS"}}
            with patch.object(a, "verify_binding", return_value={"planSha256": digest, "head": "accepted"}), patch.object(r, "verify") as verify, patch.object(a, "plan") as plan, patch.object(r, "status") as status, patch.object(a, "execute") as execute:
                self.assertTrue(a.activate(s, p, digest)["replay"])
            verify.assert_called_once_with(s, running=False); plan.assert_not_called(); execute.assert_not_called(); status.assert_not_called()

    def test_failed_build_keeps_before_state_and_blocks_migration_sts_and_hosts(self):
        with tempfile.TemporaryDirectory() as d, patch.object(r, "STATE", Path(d) / "state"), ExitStack() as stack:
            s, facts, custody, old = self.fixture(Path(d)); self.plan_mocks(stack, old)
            expected = a.plan(s, facts, custody); original = (r.STATE / "state.json").read_bytes()
            stack.enter_context(patch.object(r, "docker", return_value=b"private backup"))
            stack.enter_context(patch.object(c, "wait_containers"))
            stack.enter_context(patch.object(r, "inspect_facts", return_value=r.read(facts)))
            stack.enter_context(patch.object(u, "retained", return_value={}))
            stack.enter_context(patch.object(u, "sql", return_value=68))
            stack.enter_context(patch.object(a, "build", side_effect=r.Refused("build failed")))
            migrate = stack.enter_context(patch.object(a, "migrate")); refresh = stack.enter_context(patch.object(c, "refresh_identity")); launch = stack.enter_context(patch.object(c, "launch_hosts"))
            with self.assertRaisesRegex(r.Refused, "build failed"): a.execute(s, expected)
            self.assertEqual(s["phase"], "UNKNOWN"); self.assertEqual(s["businessBinding"], expected["previousBusinessBinding"])
            receipts = Path(s["activation"]["receipts"])
            self.assertEqual((receipts / "before-state.bytes").read_bytes(), original)
            self.assertTrue((receipts / "build-started.json").exists()); self.assertFalse((receipts / "build-result.json").exists())
            migrate.assert_not_called(); refresh.assert_not_called(); launch.assert_not_called()

    def test_profile_renewal_changes_only_credential_and_expiry(self):
        with tempfile.TemporaryDirectory() as d:
            directory = Path(d); old = {"Endpoint": "127.0.0.1:123", "Secure": False, "CredentialFile": "old", "Scope": {"WorkspaceID": "same", "ActorID": "actor", "ExpiresAt": "old", "PrincipalRef": "same", "NamespaceID": "same"}}
            p = directory / "old.json"; c.write_once(p, old)
            s = {"workspaceId": "same", "actorId": "actor", "workspaceObjectProfile": str(p), "privateHashes": {}}
            c.refresh_workspace_profile(s, directory, directory / "new-credential", "future")
            self.assertEqual(a.profile_identity(r.read(s["workspaceObjectProfile"])), a.profile_identity(old)); self.assertEqual(r.read(p), old)

    def test_ordered_success_preserves_binding_and_uses_one_narrow_renewal(self):
        with tempfile.TemporaryDirectory() as d, patch.object(r, "STATE", Path(d) / "state"), ExitStack() as stack:
            s, facts, custody, old = self.fixture(Path(d)); self.plan_mocks(stack, old)
            s["runId"] = uuid.uuid4().hex; r.save(s)
            c.write_once(Path(s["private"]) / "config.json", {"DSN": "private-dsn", "Private": s["private"], "Workspace": s["workspaceId"], "Actor": s["actorId"],
                         "Profile": {"Subject": "trial-operator-" + s["runId"], "Bucket": "raw", "StorageEndpoint": "127.0.0.1:1234"}})
            expected = a.plan(s, facts, custody); order = []
            stack.enter_context(patch.object(r, "docker", side_effect=lambda *args, **kw: order.append(args[0]) or b"backup"))
            stack.enter_context(patch.object(c, "wait_containers"))
            stack.enter_context(patch.object(r, "inspect_facts", return_value=r.read(facts)))
            retained = {"core": {}, "source": {"enterprise": ["same"]}}
            stack.enter_context(patch.object(u, "retained", return_value=retained)); stack.enter_context(patch.object(u, "sql", return_value=68))
            stack.enter_context(patch.object(a, "build", side_effect=lambda *args: order.append("build") or {"new": "artifacts"}))
            inventory = {"tables": {"old": {"columns": ["id"], "rows": ['{"id":1}']}}}
            stack.enter_context(patch.object(a, "tool", side_effect=lambda s, folder, name, mode, **kw: order.append(mode) or a.encoded(inventory if mode == "snapshot" else {"status": "PASS"})))
            stack.enter_context(patch.object(a, "migrate", side_effect=lambda *args: order.append("migration")))
            def renew(s, directory, receipts):
                self.assertEqual(directory.parent, Path(s["private"]) / "resume")
                self.assertTrue((receipts / "before-facts.json").exists())
                order.append("renew")
                s["resume"]["credentialExpiresAt"] = "2099-01-01T00:00:00Z"
                return {"expiresAt": "2099-01-01T00:00:00Z", "nativeSTSInvocations": 1}
            refresh = stack.enter_context(patch.object(c, "refresh_identity", side_effect=renew))
            copy_command = stack.enter_context(patch.object(r, "command", side_effect=lambda *args, **kw: order.append("copy") or b"{}"))
            stack.enter_context(patch.object(c, "launch_hosts", side_effect=lambda *args: order.append("hosts")))
            stack.enter_context(patch.object(r, "status", return_value={"phase": "RUNNING"}))
            a.execute(s, expected)
            self.assertEqual(order, ["start", "exec", "build", "snapshot", "migration", "snapshot", "renew", "copy", "copy", "verify", "snapshot", "hosts"])
            self.assertEqual(s["businessBinding"], expected["previousBusinessBinding"]); self.assertEqual(s["phase"], "RUNNING")
            self.assertEqual(s["activation"]["status"], "PASS"); refresh.assert_called_once(); self.assertEqual(copy_command.call_count, 2)
            for call in copy_command.call_args_list:
                self.assertNotIn("RootKey", call.kwargs["env"])
            manifest = r.read(s["activationBinding"]["path"])
            self.assertEqual(manifest["humanTokenFile"], old["humanTokenFile"]); self.assertEqual(manifest["business"], old["business"])

    def test_runner_stages_only_original_migrations_and_carries_no_storage_credentials(self):
        with tempfile.TemporaryDirectory() as d, patch.object(r, "ROOT", Path(d)):
            root = Path(d); migrations = root / "migrations"; migrations.mkdir()
            private = root / "private"; private.mkdir(); directory = root / "attempt"; directory.mkdir()
            for number in (68, 69, 70, 71): (migrations / f"{number:06d}_test.up.sql").write_text(str(number))
            c.write_once(private / "config.json", {"DSN": "private-dsn", "RootKey": "never", "RootSecret": "never"})
            with patch.object(r, "command") as command, patch.object(u, "sql", return_value=70): a.migrate({"private": str(private)}, directory)
            self.assertEqual(sorted(p.name for p in (directory / "migrations").iterdir()), [f"{n:06d}_test.up.sql" for n in (68, 69, 70)])
            self.assertEqual(command.call_args.kwargs["env"], dict(r.environment(), POSTGRES_DSN="private-dsn"))
            self.assertNotIn("down", command.call_args.args[0])

    def test_controller_baseline_never_admits_shared_business_changes(self):
        for name in ("migrations/000071_new.up.sql", "apps/platform/internal/lifecycle/writer.go", "apps/web/app/batches/actions.ts"):
            self.assertFalse(c.controller_only(name))
        self.assertTrue(c.controller_only("apps/platform/cmd/trial-activate/main.go"))


if __name__ == "__main__": unittest.main()
