"""Business composition contracts; no retained services or SQL are accessed."""
from contextlib import ExitStack
import copy
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch, Mock
import uuid
import sys
from contextlib import redirect_stdout, redirect_stderr

import business as business
import runtime as r
import bootstrap as b
import resume as c
import activation as a
import trial


class BusinessContracts(unittest.TestCase):
    def fixture(self, folder, stack):
        private = folder / "private"; private.mkdir()
        (private / "business-tools").mkdir()
        spec = private / "business-spec.json"; spec.write_text("{}")
        tool = private / "business-tools" / "trial-business.exe"; tool.write_bytes(b"reviewed helper")
        admission = folder / "admission.json"; admission.write_text("{}")
        s = {"phase": "RUNNING", "runId": "a" * 32, "workspaceId": str(uuid.uuid4()), "actorId": str(uuid.uuid4()),
             "root": str(folder), "private": str(private), "activation": {"status": "PASS", "attemptId": "same-activation"},
             "activationBinding": {"path": "original-activation", "sha256": "original"}}
        core = {"plan": {"contract": business.CONTRACT, "spec": {**{k: s[k] for k in ("runId", "workspaceId", "actorId")},
                "activationAttemptId": "same-activation"}}, "planSha256": "b" * 64}
        source = {"head": "c" * 40, "sources": {}, "deltaPaths": ["deploy/trial/business.py"]}
        for obj, name in ((r, "verify"), (c, "verify_origin"), (a, "verify_binding")):
            stack.enter_context(patch.object(obj, name))
        stack.enter_context(patch.object(b, "target", return_value=source))
        def call(s, binary, arguments):
            if arguments[0] == "plan":
                self.assertTrue(r.READ_ONLY_OBSERVATION.get())
                return copy.deepcopy(core)
            return {"contract": business.CONTRACT, "status": "PREPARED", **{k: s[k] for k in ("runId", "workspaceId", "actorId")},
                    "principalRef": "trial-operator-" + s["runId"], "planSha256": core["planSha256"],
                    "objects": {"profile": str(uuid.uuid4())}, "deliveryAuthorized": False}
        helper = stack.enter_context(patch.object(business, "helper", side_effect=call))
        return s, spec, admission, tool, helper

    def test_plan_repeat_and_failure_have_no_file_side_effects(self):
        with tempfile.TemporaryDirectory() as d, ExitStack() as stack:
            s, spec, admission, tool, helper = self.fixture(Path(d), stack)
            before = {str(p): (p.read_bytes(), p.stat().st_mtime_ns) for p in Path(d).rglob("*") if p.is_file()}
            for name in ("save", "spawn", "seed_source", "provision_config", "docker"):
                stack.enter_context(patch.object(r, name, side_effect=AssertionError("plan mutation")))
            first = business.plan(s, spec, admission, tool)
            self.assertEqual(first, business.plan(s, spec, admission, tool))
            helper.side_effect = r.Refused("read-only Core unavailable")
            with self.assertRaises(r.Refused): business.plan(s, spec, admission, tool)
            self.assertEqual(before, {str(p): (p.read_bytes(), p.stat().st_mtime_ns) for p in Path(d).rglob("*") if p.is_file()})
            self.assertFalse(r.READ_ONLY_OBSERVATION.get())

    def test_preparation_unknown_recovers_same_core_plan_and_preserves_failure(self):
        with tempfile.TemporaryDirectory() as d, ExitStack() as stack:
            s, spec, admission, tool, helper = self.fixture(Path(d), stack)
            stack.enter_context(patch.object(r, "save"))
            reviewed = business.plan(s, spec, admission, tool)
            plan_file = Path(d) / "plan.json"; c.write_once(plan_file, reviewed)
            sha = r.digest(b.encoded(reviewed)); original = helper.side_effect
            def fail(s, binary, arguments):
                if arguments[0] == "prepare": raise r.Refused("uncertain response")
                return original(s, binary, arguments)
            helper.side_effect = fail
            with self.assertRaises(r.Refused): business.prepare(s, plan_file, sha)
            failures = list(Path(s["private"]).rglob("unknown.json")); self.assertEqual(len(failures), 1)
            failed = failures[0].read_bytes(); self.assertEqual(s["businessPrerequisites"]["status"], "UNKNOWN")
            helper.side_effect = original
            result = business.prepare(s, plan_file, sha)
            self.assertEqual(result["status"], "PREPARED"); self.assertEqual(failures[0].read_bytes(), failed)
            self.assertFalse(result["receipt"]["deliveryAuthorized"])
            count = sum(call.args[2][0] == "prepare" for call in helper.call_args_list)
            self.assertEqual(business.prepare(s, plan_file, sha), result)
            self.assertEqual(count, sum(call.args[2][0] == "prepare" for call in helper.call_args_list))
            self.assertEqual(len(list(Path(s["private"]).rglob("started.json"))), 2)

    def test_digest_drift_or_changed_tool_refused_before_business_command(self):
        with tempfile.TemporaryDirectory() as d, ExitStack() as stack:
            s, spec, admission, tool, helper = self.fixture(Path(d), stack)
            reviewed = business.plan(s, spec, admission, tool); plan_file = Path(d) / "plan.json"; c.write_once(plan_file, reviewed)
            sha = r.digest(b.encoded(reviewed))
            with self.assertRaisesRegex(r.Refused, "digest"): business.prepare(s, plan_file, "f" * 64)
            tool.write_bytes(b"different helper")
            with self.assertRaisesRegex(r.Refused, "drift"): business.prepare(s, plan_file, sha)
            self.assertFalse(any(call.args[2][0] == "prepare" for call in helper.call_args_list))
            self.assertFalse((Path(s["private"]) / "business-preparations").exists())

    def test_binding_checks_identity_configuration_and_delivery_remains_disabled(self):
        with tempfile.TemporaryDirectory() as d, ExitStack() as stack:
            s, spec, admission, tool, helper = self.fixture(Path(d), stack)
            stack.enter_context(patch.object(r, "save"))
            reviewed = business.plan(s, spec, admission, tool); plan_file = Path(d) / "plan.json"; c.write_once(plan_file, reviewed)
            result = business.prepare(s, plan_file, r.digest(b.encoded(reviewed)))
            self.assertEqual(business.verify_binding(s), result)
            self.assertEqual(result["configuration"]["web"]["POC_QUALITY_ACTOR_ID"], s["actorId"])
            self.assertFalse(any("DELIVERY" in key for side in result["configuration"].values() for key in side))
            s["actorId"] = str(uuid.uuid4())
            with self.assertRaisesRegex(r.Refused, "identity"): business.verify_binding(s)

    def test_configuration_is_opt_in_and_never_spawns_or_renews(self):
        s = {"actorId": str(uuid.uuid4())}
        value = business.configuration(s)
        self.assertEqual(set(value["api"]), {"QUALITY_REMEDIATION_PLAN_REF"})
        self.assertEqual(value["web"]["POC_COMPLIANCE_ACTOR_ID"], s["actorId"])
        self.assertNotIn("POC_ENABLE_DATASET_ACTIONS", value["web"])
        self.assertTrue(c.controller_only("apps/platform/cmd/trial-business/main.go"))
        self.assertFalse(c.controller_only("apps/platform/internal/rights/application/service.go"))

    def test_real_cli_plan_does_not_acquire_operator_lock_or_write_registry(self):
        with tempfile.TemporaryDirectory() as d, ExitStack() as stack:
            s, spec, admission, tool, helper = self.fixture(Path(d), stack)
            state = Path(d) / "state"; state.mkdir(); c.write_once(state / "state.json", s)
            before = (state / "state.json").read_bytes()
            stack.enter_context(patch.object(trial, "STATE", state))
            lock = stack.enter_context(patch.object(trial, "exclusive", side_effect=AssertionError("plan acquired lock")))
            stack.enter_context(patch.object(sys, "argv", ["trial.py", "business-plan", "--profile", str(spec), "--admission", str(admission), "--tool", str(tool)]))
            output = io.StringIO()
            with redirect_stdout(output): self.assertEqual(trial.main(), 0)
            self.assertEqual(json.loads(output.getvalue())["plan"]["contract"], business.CONTRACT)
            self.assertEqual((state / "state.json").read_bytes(), before)
            lock.assert_not_called()

    def test_real_cli_rejects_tool_or_execution_options_before_io(self):
        for arguments in (["business-plan", "--profile", "x"], ["status", "--tool", "x"],
                          ["prepare-business", "--plan", "x", "--plan-sha256", "y", "--profile", "z"]):
            with patch.object(sys, "argv", ["trial.py", *arguments]), patch.object(trial, "exclusive") as lock, redirect_stderr(io.StringIO()):
                with self.assertRaises(SystemExit) as exit: trial.main()
                self.assertEqual(exit.exception.code, 2); lock.assert_not_called()

    def test_actual_host_environment_reads_only_successful_bound_configuration(self):
        with tempfile.TemporaryDirectory() as d, ExitStack() as stack:
            s, spec, admission, tool, helper = self.fixture(Path(d), stack)
            stack.enter_context(patch.object(r, "save"))
            reviewed = business.plan(s, spec, admission, tool); plan_file = Path(d) / "plan.json"; c.write_once(plan_file, reviewed)
            private = Path(s["private"])
            c.write_once(private / "config.json", {"DSN": "isolated-not-connected", "Profile": {"StorageEndpoint": "127.0.0.1:1"}})
            (private / "token").write_text("isolated-test-token")
            (private / "human-token").write_text("isolated-human-token")
            s.update(ports={"api": 1, "redis": 2}, api="http://127.0.0.1:1", workspaceObjectProfile="original-protected-profile", businessBinding={})
            stack.enter_context(patch.object(r, "active_business", return_value={"humanTokenFile": str(private / "human-token"), "business": {"workflowVersionId": "original-workflow"}}))
            self.assertNotIn("POC_ENABLE_QUALITY_ACTIONS", r.web_environment(s))
            self.assertNotIn("QUALITY_REMEDIATION_PLAN_REF", r.core_environment(s))
            business.prepare(s, plan_file, r.digest(b.encoded(reviewed)))
            api, web = r.core_environment(s), r.web_environment(s)
            self.assertEqual(api["QUALITY_REMEDIATION_PLAN_REF"], business.REMEDIATION)
            self.assertEqual(api["DELIVERY_API_ENABLED"], "false")
            self.assertEqual(api["OBJECT_STORAGE_WORKSPACE_PROFILE"], "original-protected-profile")
            self.assertNotIn("OBJECT_STORAGE_ACCESS_KEY", api)
            self.assertEqual(web["POC_QUALITY_ACTOR_ID"], s["actorId"])
            self.assertEqual(web["POC_COMPLIANCE_ACTOR_ID"], s["actorId"])
            self.assertNotIn("POC_ENABLE_DELIVERY_ACTIONS", web)


if __name__ == "__main__": unittest.main()
