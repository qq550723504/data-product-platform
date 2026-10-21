"""Renewal assembly tests use private synthetic files and no retained runtime."""
from contextlib import ExitStack, redirect_stdout, redirect_stderr
import copy
import io
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import uuid

import activation as a
import bootstrap as b
import business
import business_renewal as renewal
import resume as c
import runtime as r
import trial
import test_readonly_storage as native


def snapshot(root):
    return {str(p.relative_to(root)): (p.read_bytes(), p.stat().st_mtime_ns)
            for p in root.rglob("*") if p.is_file()}


def fixture(root, stack, state=None, mock_verify=True):
    private = root / "private"; private.mkdir(exist_ok=True)
    (private / "business-tools").mkdir()
    tool = private / "business-tools/trial-business.exe"; tool.write_bytes(b"admitted helper")
    admission = root / "admission.json"; b.put(admission, {"head": "a" * 40})
    s = state or {"phase": "RUNNING", "runId": "a" * 32, "workspaceId": str(uuid.uuid4()),
                  "actorId": str(uuid.uuid4()), "root": str(root), "private": str(private)}
    s.update(activation={"status": "PASS"}, activationBinding={"path": "accepted", "sha256": "b" * 64})
    identity = {k: s[k] for k in ("runId", "workspaceId", "actorId")}
    keys = ["authorization", "contractVersion", "profile"] + [kind + "-" + str(i) for kind in ("declaration", "verification", "binding") for i in range(3)]
    old_objects = {key: str(uuid.uuid4()) for key in keys}
    old = {"contract": business.CONTRACT, "status": "PREPARED", **identity,
           "principalRef": "trial-operator-" + s["runId"], "deliveryAuthorized": False,
           "planSha256": "c" * 64, "objects": old_objects}
    folder = private / "business-preparations/accepted"; folder.mkdir(parents=True)
    manifest = {"contract": business.CONTRACT, "status": "PREPARED", **identity, "root": s["root"],
                "activationBinding": s["activationBinding"], "configuration": business.configuration(s),
                "receipt": old, "corePlanSha256": old["planSha256"]}
    b.put(folder / "manifest.json", manifest)
    s["businessPrerequisiteBinding"] = b.file(folder / "manifest.json")
    spec = private / "renewal.json"
    b.put(spec, {"contract": renewal.CONTRACT, **identity, "renewalId": str(uuid.uuid4()),
                "preparationSha256": s["businessPrerequisiteBinding"]["sha256"],
                "validFrom": "2026-10-21T08:00:00Z", "validTo": "2026-10-21T10:00:00Z",
                "verificationReason": "Synthetic finite successor authority"})
    if mock_verify: stack.enter_context(patch.object(r, "verify"))
    stack.enter_context(patch.object(c, "verify_origin"))
    stack.enter_context(patch.object(a, "verify_binding"))
    stack.enter_context(patch.object(b, "target", return_value={"head": "a" * 40, "sources": {}}))

    def helper(s, binary, args):
        if args[0] == "renewal-plan":
            assert r.READ_ONLY_OBSERVATION.get()
            value = r.read(args[2])
            parent = old["planSha256"]
            if s.get("businessRightsRenewalBinding"):
                latest = renewal.verify_binding(s)
                parent = latest["corePlanSha256"]
            plan = {"contract": renewal.CONTRACT, "spec": value,
                    "preparationBinding": s["businessPrerequisiteBinding"], "parent": {"planSha256": parent}}
            return {"plan": plan, "planSha256": r.digest(b.encoded(plan))}
        assert args[0] == "renew-rights"
        plan = r.read(args[4]); value = plan["spec"]
        objects = {key: str(uuid.uuid5(uuid.UUID(value["renewalId"]), key)) for key in old_objects}
        objects.update({key: old_objects[key] for key in ("contractVersion", "profile")})
        return {"contract": renewal.CONTRACT, "status": "RENEWED", **identity,
                "principalRef": old["principalRef"], "deliveryAuthorized": False,
                "objects": objects, "renewalId": value["renewalId"], "planSha256": args[6],
                "parentPlanSha256": plan["parent"]["planSha256"]}
    call = stack.enter_context(patch.object(business, "helper", side_effect=helper))
    return s, spec, admission, tool, call


def review(s, spec, admission, tool, destination):
    value = renewal.plan(s, spec, admission, tool)
    b.put(destination, value)
    return value, r.digest(b.encoded(value))


class RenewalContracts(unittest.TestCase):
    def test_readonly_cli_repeat_failure_and_invalid_options(self):
        with tempfile.TemporaryDirectory() as d, ExitStack() as stack:
            root = Path(d); s, spec, admission, tool, helper = fixture(root, stack)
            state = root / "state"; state.mkdir(); b.put(state / "state.json", s)
            stack.enter_context(patch.object(trial, "STATE", state))
            lock = stack.enter_context(patch.object(trial, "exclusive", side_effect=AssertionError("plan lock")))
            stack.enter_context(patch.object(trial.sys, "argv", ["trial.py", "business-rights-renewal-plan", "--profile", str(spec), "--admission", str(admission), "--tool", str(tool)]))
            before = snapshot(root)
            outputs = []
            for _ in range(2):
                output = io.StringIO()
                with redirect_stdout(output): self.assertEqual(trial.main(), 0)
                outputs.append(json.loads(output.getvalue()))
            self.assertEqual(outputs[0], outputs[1])
            helper.side_effect = r.Refused("readonly helper refused")
            with redirect_stderr(io.StringIO()): self.assertEqual(trial.main(), 1)
            self.assertEqual(before, snapshot(root)); lock.assert_not_called()
        for args in (["business-rights-renewal-plan", "--profile", "x"],
                     ["renew-business-rights", "--plan", "x", "--plan-sha256", "y", "--tool", "z"]):
            with patch.object(trial.sys, "argv", ["trial.py", *args]), patch.object(trial, "exclusive") as lock, redirect_stderr(io.StringIO()):
                with self.assertRaises(SystemExit) as error: trial.main()
                self.assertEqual(error.exception.code, 2); lock.assert_not_called()

    def test_unknown_same_plan_recovery_and_completed_replay_preserve_history(self):
        with tempfile.TemporaryDirectory() as d, ExitStack() as stack:
            root = Path(d); s, spec, admission, tool, helper = fixture(root, stack)
            stack.enter_context(patch.object(r, "save"))
            original = copy.deepcopy(s["businessPrerequisiteBinding"])
            original_bytes = Path(original["path"]).read_bytes()
            reviewed, sha = review(s, spec, admission, tool, root / "plan.json")
            successful = helper.side_effect
            def unknown(s, tool, args):
                if args[0] == "renew-rights": raise r.Refused("response unknown")
                return successful(s, tool, args)
            helper.side_effect = unknown
            with self.assertRaises(r.Refused): renewal.renew(s, root / "plan.json", sha)
            failed = next(Path(s["private"]).rglob("unknown.json")); failure = failed.read_bytes()
            changed = copy.deepcopy(reviewed); changed["core"]["plan"]["spec"]["renewalId"] = str(uuid.uuid4())
            b.put(root / "changed.json", changed)
            with self.assertRaisesRegex(r.Refused, "same reviewed plan"):
                renewal.renew(s, root / "changed.json", r.digest(b.encoded(changed)))
            helper.side_effect = successful
            result = renewal.renew(s, root / "plan.json", sha)
            self.assertEqual(renewal.verify_binding(s), result)
            helper.reset_mock(); before = snapshot(root)
            self.assertEqual(renewal.renew(s, root / "plan.json", sha), result)
            helper.assert_not_called(); self.assertEqual(snapshot(root), before)
            self.assertEqual(failed.read_bytes(), failure)
            self.assertEqual(s["businessPrerequisiteBinding"], original)
            self.assertEqual(Path(original["path"]).read_bytes(), original_bytes)
            self.assertEqual(len(s["businessRightsRenewalAttempts"]), 2)

    def test_next_window_appends_binding_and_tamper_is_refused(self):
        with tempfile.TemporaryDirectory() as d, ExitStack() as stack:
            root = Path(d); s, spec, admission, tool, helper = fixture(root, stack)
            stack.enter_context(patch.object(r, "save"))
            first, sha = review(s, spec, admission, tool, root / "first.json")
            renewal.renew(s, root / "first.json", sha)
            first_binding = copy.deepcopy(s["businessRightsRenewalBinding"])
            prior = snapshot(Path(first_binding["path"]).parent)
            value = r.read(spec); value["renewalId"] = str(uuid.uuid4()); value["validFrom"] = value["validTo"]; value["validTo"] = "2026-10-21T11:00:00Z"
            r.write(spec, value)
            second, sha2 = review(s, spec, admission, tool, root / "second.json")
            self.assertEqual(second["parentBinding"], first_binding)
            self.assertEqual(second["core"]["plan"]["parent"]["planSha256"], first["core"]["planSha256"])
            renewal.renew(s, root / "second.json", sha2)
            self.assertNotEqual(s["businessRightsRenewalBinding"], first_binding)
            self.assertEqual(snapshot(Path(first_binding["path"]).parent), prior)
            bound = Path(s["businessRightsRenewalBinding"]["path"])
            plan_path = bound.parent / "plan.json"; edited = r.read(plan_path); edited["tool"]["sha256"] = "f" * 64; r.write(plan_path, edited)
            with self.assertRaisesRegex(r.Refused, "plan drift"): renewal.verify_binding(s)

    def test_digest_tool_or_orphan_refused_before_new_commands(self):
        for changed in ("digest", "tool", "orphan"):
            with self.subTest(changed=changed), tempfile.TemporaryDirectory() as d, ExitStack() as stack:
                root = Path(d); s, spec, admission, tool, helper = fixture(root, stack)
                _, sha = review(s, spec, admission, tool, root / "plan.json")
                if changed == "digest": sha = "f" * 64
                elif changed == "tool": tool.write_bytes(b"changed")
                else:
                    orphan = Path(s["private"]) / "business-rights-renewals/foreign/attempt"; orphan.mkdir(parents=True); b.put(orphan / "started.json", {})
                with self.assertRaises(r.Refused): renewal.renew(s, root / "plan.json", sha)
                self.assertFalse(any(call.args[2][0] == "renew-rights" for call in helper.call_args_list))

    def test_false_receipt_cannot_be_adopted(self):
        with tempfile.TemporaryDirectory() as d, ExitStack() as stack:
            root = Path(d); s, spec, admission, tool, helper = fixture(root, stack)
            stack.enter_context(patch.object(r, "save"))
            _, sha = review(s, spec, admission, tool, root / "plan.json")
            original = helper.side_effect
            def false(s, tool, args):
                result = original(s, tool, args)
                if args[0] == "renew-rights": result["objects"]["authorization"] = business.verify_binding(s)["receipt"]["objects"]["authorization"]
                return result
            helper.side_effect = false
            with self.assertRaisesRegex(r.Refused, "Exact new rights"): renewal.renew(s, root / "plan.json", sha)
            self.assertNotIn("businessRightsRenewalBinding", s)
            self.assertEqual(s["businessRightsRenewal"]["status"], "UNKNOWN")


@unittest.skipUnless(os.name == "nt" and native.PWSH, "native Windows PowerShell verifier")
class RenewalNativeReadonly(unittest.TestCase):
    def test_real_verify_storage_check_through_new_cli_keeps_bytes_mtimes_and_logs(self):
        for failure in (False, True):
            with self.subTest(failure=failure), tempfile.TemporaryDirectory() as d, ExitStack() as stack:
                root = Path(d); harness = native.NativeReadonlyStorage()
                s, containers, _, _, old = harness.fixture(root)
                s["phase"] = "RUNNING"
                for item in containers.values(): item["State"]["Status"] = "running"
                s, spec, admission, tool, helper = fixture(root, stack, s, mock_verify=False)
                r.write(root / "state/state.json", s)
                stack.enter_context(patch.object(r, "STATE", root / "state"))
                stack.enter_context(patch.object(trial, "STATE", root / "state"))
                stack.enter_context(patch.object(native.u, "verify_business", return_value=old))
                calls = stack.enter_context(harness.external_replies(s, containers, docker_failure=failure))
                stack.enter_context(patch.object(trial, "exclusive", side_effect=AssertionError("readonly lock")))
                stack.enter_context(patch.object(trial.sys, "argv", ["trial.py", "business-rights-renewal-plan", "--profile", str(spec), "--admission", str(admission), "--tool", str(tool)]))
                before = snapshot(root)
                for _ in range(2):
                    with redirect_stdout(io.StringIO()), redirect_stderr(io.StringIO()):
                        self.assertEqual(trial.main(), int(failure))
                self.assertEqual(before, snapshot(root))
                if not failure:
                    checks = [(args, data) for args, data in calls if args[0] == "pwsh"]
                    self.assertEqual(len(checks), 2)
                    for args, data in checks:
                        self.assertIn("-InputFromStdin", args); self.assertNotIn("-InputPath", args)
                        self.assertEqual(json.loads(data)["run_id"], s["runId"])


if __name__ == "__main__": unittest.main()
