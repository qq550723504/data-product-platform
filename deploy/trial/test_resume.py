"""Continuation regressions. All external calls are mocked; no retained data changes."""
import copy
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import Mock, patch

import resume as c
import runtime as r
import trial


class ResumeContracts(unittest.TestCase):
    def state(self, private):
        return {"phase":"STOPPED", "runId":"original-run", "workspaceId":"original-workspace", "private":str(private),
                "head":"a"*40, "sources":{"apps/platform/main.go":"old"}, "processes":{}, "ports":{},
                "containers":{"core":{"id":"exact-core","name":"core"},"objects":{"id":"exact-objects","name":"objects"}},
                "binaries":{}, "privateHashes":{}, "secretReferences":[]}

    def binding(self):
        return {"head":"b"*40,"sources":{"apps/platform/main.go":"old","deploy/trial/resume.py":"new"},"deltaPaths":["deploy/trial/resume.py"]}

    def test_resume_without_registry_never_initializes(self):
        with tempfile.TemporaryDirectory() as d, patch.object(trial,"STATE",Path(d)),patch.object(trial,"exclusive"),patch.object(sys,"argv",["trial.py","resume"]),patch.object(trial,"start") as start,patch.object(trial,"resume") as resume:
            self.assertEqual(trial.main(),1)
        start.assert_not_called();resume.assert_not_called()

    def test_unknown_transitional_states_refused_before_resource_checks(self):
        for phase in ("UNKNOWN","STARTING","STOPPING","RESUMING"):
            with self.subTest(phase=phase),patch.object(r,"verify") as verify:
                with self.assertRaises(r.Refused):c.precheck({"runId":"kept","phase":phase})
                verify.assert_not_called()

    def test_running_resume_only_reads_current_status(self):
        s={"phase":"RUNNING"}
        with patch.object(r,"status",return_value={"phase":"RUNNING"}) as status,patch.object(c,"precheck") as precheck,patch.object(r,"seed_source") as seed:
            self.assertEqual(c.resume(s),{"phase":"RUNNING"})
        status.assert_called_once_with(s);precheck.assert_not_called();seed.assert_not_called()

    def test_expired_session_does_not_report_running_readiness(self):
        s={"phase":"RUNNING","runId":"kept","workspaceId":"kept","head":"a"*40,"resume":{"status":"PASS","credentialExpiresAt":"2000-01-01T00:00:00Z"}}
        with patch.object(r,"verify"),patch.object(r,"http") as http:
            with self.assertRaisesRegex(r.Refused,"session expired"):r.status(s)
        http.assert_not_called()

    def test_business_source_change_cannot_be_rebound(self):
        with tempfile.TemporaryDirectory() as d:
            s=self.state(Path(d));current={"apps/platform/main.go":"changed"}
            with patch.object(r,"verify"),patch.object(c,"committed_sources",side_effect=[s["sources"],current]),patch.object(r,"sources",return_value=current),patch.object(r,"command",return_value=("b"*40).encode()),patch.object(r,"docker") as docker:
                with self.assertRaisesRegex(r.Refused,"Business source drift"):c.precheck(s)
            docker.assert_not_called()

    def test_uncommitted_controller_cannot_be_rebound(self):
        with tempfile.TemporaryDirectory() as d:
            s=self.state(Path(d))
            with patch.object(r,"verify"),patch.object(c,"committed_sources",return_value=s["sources"]),patch.object(r,"sources",return_value={**s["sources"],"deploy/trial/resume.py":"dirty"}),patch.object(r,"command",return_value=("b"*40).encode()),patch.object(r,"docker") as docker:
                with self.assertRaisesRegex(r.Refused,"must be committed"):c.precheck(s)
            docker.assert_not_called()

    def precheck_mocks(self, s):
        return (patch.object(r,"verify"),patch.object(c,"committed_sources",return_value=s["sources"]),
                patch.object(r,"sources",return_value=s["sources"]),patch.object(r,"command",return_value=("b"*40).encode()))

    def test_port_conflict_never_starts_or_replaces_resources(self):
        with tempfile.TemporaryDirectory() as d:
            s=self.state(Path(d));s["ports"]={"api":12345}
            a,b,e,f=self.precheck_mocks(s)
            sock=Mock();sock.bind.side_effect=OSError("occupied")
            with a,b,e,f,patch.object(r,"docker",return_value=b'[{"State":{"Status":"exited"}}]') as docker,patch.object(c.socket,"socket",return_value=sock),patch.object(r,"read") as read:
                with self.assertRaisesRegex(r.Refused,"port unavailable"):c.precheck(s)
            self.assertTrue(all(call.args[0]=="inspect" for call in docker.call_args_list));read.assert_not_called();sock.close.assert_called_once()

    def test_existing_live_host_process_refused_without_termination(self):
        with tempfile.TemporaryDirectory() as d:
            s=self.state(Path(d));expected={"pid":22,"created":100,"exe":"same.exe"};s["processes"]={"api":expected}
            a,b,e,f=self.precheck_mocks(s)
            with a,b,e,f,patch.object(r,"docker",return_value=b'[{"State":{"Status":"exited"}}]'),patch.object(r,"process_identity",return_value=expected) as identity:
                with self.assertRaisesRegex(r.Refused,"still running"):c.precheck(s)
            identity.assert_called_once_with(22,allow_missing=True)

    def test_original_snapshot_mutation_is_refused(self):
        with tempfile.TemporaryDirectory() as d,patch.object(r,"STATE",Path(d)):
            r.write(Path(d)/"resume-origin.json",{"runId":"changed"})
            with self.assertRaisesRegex(r.Refused,"snapshot drift"):c.verify_origin({"originSnapshot":{"sha256":"expected"}})

    def test_write_once_cannot_replace_a_receipt(self):
        with tempfile.TemporaryDirectory() as d:
            path=Path(d)/"completed.json";c.write_once(path,{"status":"UNKNOWN"})
            with self.assertRaises(FileExistsError):c.write_once(path,{"status":"PASS"})
            self.assertEqual(r.read(path),{"status":"UNKNOWN"})

    def test_resume_process_logs_do_not_overwrite_original_or_same_attempt(self):
        with tempfile.TemporaryDirectory() as d:
            private=Path(d);(private/"api.log").write_bytes(b"original runtime evidence")
            logs=private/"resume";logs.mkdir();s={"private":str(private),"processes":{}}
            with patch.object(r.subprocess,"Popen",return_value=Mock(pid=123)) as popen,patch.object(r,"process_identity",return_value={"pid":123}),patch.object(r,"save"):
                r.spawn(s,"api",["unused.exe"],{},log_directory=logs)
                with self.assertRaises(FileExistsError):r.spawn(s,"api",["unused.exe"],{},log_directory=logs)
            popen.assert_called_once();self.assertEqual((private/"api.log").read_bytes(),b"original runtime evidence")
            self.assertEqual(s["processLogs"]["api"],str(logs/"api.log"))

    def test_resume_starts_exact_owners_preserves_origin_and_repeat_is_read_only(self):
        with tempfile.TemporaryDirectory() as d,patch.object(r,"STATE",Path(d)/"state"):
            r.STATE.mkdir();s=self.state(Path(d)/"private");original=copy.deepcopy(s);baseline={"workspaceId":s["workspaceId"],"counts":{"dataset_version":3,"cost_event":6}}
            with patch.object(c,"precheck",return_value=(self.binding(),baseline)),patch.object(r,"save"),patch.object(r,"docker") as docker,patch.object(c,"wait_containers"),patch.object(c,"compare_facts",return_value=baseline) as compare,patch.object(c,"refresh_identity",return_value={"nativeSTSInvocations":1,"expiresAt":"future"}) as refresh,patch.object(c,"launch_hosts"),patch.object(r,"status",return_value={"phase":"RUNNING"}),patch.object(r,"seed_source") as seed,patch.object(r,"provision_config") as provision,patch.object(r,"compose") as compose:
                c.resume(s);attempt=s["resume"]["attemptId"];c.resume(s)
            self.assertEqual(s["runId"],original["runId"]);self.assertEqual(s["workspaceId"],original["workspaceId"])
            self.assertEqual(r.read(r.STATE/"resume-origin.json"),original)
            self.assertEqual(s["head"],original["head"]);self.assertEqual(s["sources"],original["sources"])
            docker.assert_called_once_with("start","exact-core","exact-objects",timeout=180)
            self.assertEqual(compare.call_count,2);refresh.assert_called_once();seed.assert_not_called();provision.assert_not_called();compose.assert_not_called()
            self.assertEqual(len(s["resumeHistory"]),1)
            self.assertEqual(r.read(r.STATE/"resumes"/attempt/"completed.json")["status"],"PASS")

    def test_fact_mismatch_is_unknown_before_credentials_and_hosts(self):
        with tempfile.TemporaryDirectory() as d,patch.object(r,"STATE",Path(d)/"state"):
            r.STATE.mkdir();s=self.state(Path(d)/"private");baseline={"workspaceId":s["workspaceId"],"counts":{"dataset_version":3}}
            with patch.object(c,"precheck",return_value=(self.binding(),baseline)),patch.object(r,"save"),patch.object(r,"docker"),patch.object(c,"wait_containers"),patch.object(r,"inspect_facts",return_value={"counts":{"dataset_version":0}}),patch.object(c,"refresh_identity") as refresh,patch.object(c,"launch_hosts") as launch:
                with self.assertRaisesRegex(r.Refused,"facts differ"):c.resume(s)
            self.assertEqual(s["phase"],"UNKNOWN");refresh.assert_not_called();launch.assert_not_called()
            failed=r.read(Path(s["resume"]["receipts"])/"failed.json")
            self.assertEqual(failed["credentialRefresh"],"NOT_RUN");self.assertTrue(failed["retained"])

    def test_restart_uses_stop_then_resume_for_same_state(self):
        s={"phase":"RUNNING","runId":"kept"};order=[]
        def stopped(value):order.append("stop");value["phase"]="STOPPED"
        with patch.object(r,"verify",side_effect=lambda _:order.append("verify")),patch.object(r,"stop",side_effect=stopped),patch.object(c,"resume",side_effect=lambda value:order.append(("resume",value["runId"],value["phase"]))):c.restart(s)
        self.assertEqual(order,["verify","stop",("resume","kept","STOPPED")])


if __name__=="__main__":unittest.main()
