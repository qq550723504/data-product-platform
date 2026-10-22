"""Ownership/lifecycle regressions. No Docker mutation or real credentials."""
import copy
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch, Mock
import runtime as r


class TrialContracts(unittest.TestCase):
    def test_generated_next_types_are_not_executable_source_identity(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d);(root/"apps/web").mkdir(parents=True)
            (root/"apps/web/next-env.d.ts").write_text("generated")
            (root/"apps/web/page.tsx").write_text("runtime source")
            with patch.object(r,"ROOT",root),patch.object(r,"command",return_value=b"apps/web/next-env.d.ts\napps/web/page.tsx\n"):
                before=r.sources();(root/"apps/web/next-env.d.ts").write_text("regenerated")
                self.assertEqual(before,r.sources())
                (root/"apps/web/page.tsx").write_text("changed runtime")
                self.assertNotEqual(before,r.sources())
    def test_repeat_start_reads_existing_run_and_cannot_seed(self):
        with tempfile.TemporaryDirectory() as d, patch.object(r, "STATE", Path(d)):
            r.write(Path(d) / "state.json", {"runId":"original", "phase":"RUNNING"})
            with patch.object(r, "status", return_value={"runId":"original"}) as status, patch.object(r,"seed_source") as seed:
                self.assertEqual(r.start(), {"runId":"original"}); status.assert_called_once(); seed.assert_not_called()

    def test_unknown_or_stopped_run_cannot_be_replaced(self):
        for phase in ("STARTING","UNKNOWN","STOPPED","STOPPING"):
            with tempfile.TemporaryDirectory() as d, patch.object(r,"STATE",Path(d)), patch.object(r,"preflight") as probe:
                r.write(Path(d)/"state.json",{"runId":"kept","phase":phase})
                with self.assertRaisesRegex(r.Refused,"kept"): r.start()
                probe.assert_not_called()

    def test_source_drift_refused_before_inspecting_or_mutating_containers(self):
        s={"root":str(r.ROOT),"contract":"issue325/stage1","host":{"daemonId":"local"},"sources":{"old":"hash"}}
        with patch.object(r,"preflight",return_value={"daemonId":"local"}), patch.object(r,"sources",return_value={"new":"hash"}), patch.object(r,"docker") as docker:
            with self.assertRaisesRegex(r.Refused,"Source drift"): r.verify(s)
            docker.assert_not_called()

    def container(self):
        return {"Id":"sha-container","Name":"/mine-core","Image":"sha-image","Config":{"Labels":{r.LABEL:"mine"}},
                "HostConfig":{"PortBindings":{"5432/tcp":[{"HostIp":"127.0.0.1","HostPort":"12345"}]}},"Mounts":[],"NetworkSettings":{"Networks":{"private":{}}},"State":{"Status":"running"}}

    def state(self):
        item=self.container()
        return {"runId":"mine","workspaceId":"workspace","actorId":"actor","root":str(r.ROOT),"phase":"UNKNOWN","host":{"daemonId":"local"},"containers":{"core":{"name":"mine-core","id":"sha-container","configHash":r.container_hash(item)}},"images":{"core":{"id":"sha-image"}},"processes":{},"volumes":{"core":"retained"}}

    def test_foreign_or_replaced_container_cannot_be_stopped(self):
        for field,value in (("Id","replaced"),("Image","foreign-image"),("Name","/foreign")):
            item=self.container();item[field]=value
            calls=[]
            def docker(*args,**kw):
                calls.append(args)
                if args[0]=="ps":return b"sha-container"
                if args[0]=="inspect":return json.dumps([item]).encode()
                self.fail("foreign container reached stop")
            with patch.object(r,"local_docker_identity",return_value={"daemonId":"local"}),patch.object(r,"docker",side_effect=docker):
                with self.assertRaises(r.Refused):r.stop(self.state())
            self.assertFalse(any(c[0]=="stop" for c in calls))

    def test_network_or_nonloopback_drift_refused(self):
        for mutation in (lambda x:x["NetworkSettings"]["Networks"].update({"bypass":{}}),lambda x:x["HostConfig"]["PortBindings"]["5432/tcp"][0].update({"HostIp":"0.0.0.0"})):
            item=self.container();mutation(item)
            with self.assertRaises(r.Refused):r.verify_container(self.state(),"core",item)

    def test_daemon_runtime_defaults_do_not_change_authored_contract(self):
        item=self.container();before=r.container_hash(item)
        item["HostConfig"].update({"Runtime":"runc","MemorySwappiness":-1,"ConsoleSize":[0,0]})
        self.assertEqual(before,r.container_hash(item))
        item["Mounts"].append({"Type":"bind","Source":"foreign","Destination":"/data","RW":True})
        self.assertNotEqual(before,r.container_hash(item))

    def test_stop_uses_exact_container_id_and_retains_all_resources(self):
        calls=[];s=self.state();item=self.container()
        def docker(*args,**kw):
            calls.append(args)
            if args[0]=="ps":return b"sha-container"
            if args[0]=="inspect":return json.dumps([item]).encode()
            if args[0]=="stop":return b"sha-container"
            self.fail("unexpected resource mutation")
        with patch.object(r,"local_docker_identity",return_value={"daemonId":"local"}),patch.object(r,"docker",side_effect=docker),patch.object(r,"save"):
            result=r.stop(s)
        self.assertEqual(calls[-1],("stop","--time","30","sha-container"));self.assertTrue(result["retained"])

    def test_running_stop_retains_unknown_snapshot_and_stops_exact_owner(self):
        for failure in (r.Refused("Core/object unavailable"), OSError("unavailable"), ValueError("invalid response")):
            with self.subTest(failure=type(failure).__name__), tempfile.TemporaryDirectory() as d:
                calls=[];s=self.state();s["phase"]="RUNNING";item=self.container()
                previous={"counts":{"dataset_version":3},"proof":"last-good"}
                r.write(Path(d)/"before-stop-facts.json",previous)
                def docker(*args,**kw):
                    calls.append(args)
                    if args[0]=="ps":return b"sha-container"
                    if args[0]=="inspect":return json.dumps([item]).encode()
                    if args[0]=="stop":return b"sha-container"
                    self.fail("unexpected resource mutation")
                with patch.object(r,"STATE",Path(d)),patch.object(r,"local_docker_identity",return_value={"daemonId":"local"}),patch.object(r,"docker",side_effect=docker),patch.object(r,"inspect_facts",side_effect=failure),patch.object(r,"save"):
                    result=r.stop(s)
                self.assertEqual(calls[-1],("stop","--time","30","sha-container"))
                self.assertEqual(result["phase"],"STOPPED")
                self.assertTrue(result["retained"])
                observation=r.read(s["stopInspection"]["observation"]["path"])
                self.assertEqual(observation["status"],"UNKNOWN")
                self.assertEqual(observation["runId"],"mine")
                self.assertEqual(r.read(Path(d)/"before-stop-facts.json"),previous)
                self.assertEqual(s["stopInspection"]["status"],observation["status"])

    def test_stop_does_not_require_build_tools_or_fixture_health(self):
        s=self.state();item=self.container();calls=[]
        def docker(*args,**kw):
            calls.append(args)
            if args[0]=="ps":return b"sha-container"
            if args[0]=="inspect":return json.dumps([item]).encode()
            if args[0]=="stop":return b"sha-container"
            self.fail("unexpected resource mutation")
        with patch.object(r,"preflight",side_effect=r.Refused("missing build tools/fixture")) as preflight,patch.object(r,"local_docker_identity",return_value={"daemonId":"local"}),patch.object(r,"docker",side_effect=docker),patch.object(r,"save"):
            result=r.stop(s)
        preflight.assert_not_called()
        self.assertEqual(result["phase"],"STOPPED")
        self.assertEqual(calls[-1],("stop","--time","30","sha-container"))

    def test_stop_still_refuses_daemon_drift_before_any_target_mutation(self):
        with patch.object(r,"local_docker_identity",return_value={"daemonId":"foreign"}),patch.object(r,"docker") as docker,patch.object(r,"inspect_facts") as inspect:
            with self.assertRaisesRegex(r.Refused,"Docker daemon drift"):r.stop(self.state())
        docker.assert_not_called();inspect.assert_not_called()

    def test_unknown_commands_rejected_before_docker(self):
        for action in ("reset","cleanup","down","delete","source-revoke"):
            result=subprocess.run([sys.executable,str(r.ROOT/"deploy/trial/trial.py"),action],capture_output=True)
            self.assertEqual(result.returncode,2)

    def test_no_application_secrets_or_compose_overrides_inherited(self):
        with patch.dict(os.environ,{"RIGHTS_API_TOKEN":"private","NEXT_PUBLIC_TOKEN":"private","COMPOSE_FILE":"foreign","DOCKER_HOST":"tcp://remote","INGESTION_PROFILE":"foreign"}):
            env=r.environment()
        self.assertFalse(any(k in env for k in ("RIGHTS_API_TOKEN","NEXT_PUBLIC_TOKEN","COMPOSE_FILE","DOCKER_HOST","INGESTION_PROFILE")))

    def test_failed_commands_preserve_existing_diagnostics_and_redact_each_new_log(self):
        with tempfile.TemporaryDirectory() as d, patch.object(r,"STATE",Path(d)/"state"):
            r.STATE.mkdir();private=Path(d)/"private";private.mkdir()
            old=private/"last-command-error.log";old.write_bytes(b"original diagnostic evidence")
            secret=private/"identity.json";secret.write_text(json.dumps({"SecretKey":"synthetic-secret-value"}),encoding="utf-8")
            r.write(r.STATE/"state.json",{"private":str(private),"secretReferences":[str(secret)]})
            observations=[]
            for failure in (b"first synthetic-secret-value failure",b"second synthetic-secret-value failure"):
                with patch.object(r.subprocess,"run",return_value=Mock(returncode=1,stdout=b"",stderr=failure)):
                    with self.assertRaises(r.Refused) as refused:r.command(["synthetic-command"])
                observations.append(str(refused.exception))
            logs=sorted(private.glob("command-error-*.log"))
            self.assertEqual(old.read_bytes(),b"original diagnostic evidence")
            self.assertEqual(len(logs),2)
            self.assertEqual({log.read_bytes() for log in logs},{b"first [REDACTED] failure",b"second [REDACTED] failure"})
            for log in logs:self.assertTrue(any(log.name in value for value in observations))
            self.assertTrue(all("synthetic-secret-value" not in value for value in observations))

    def test_diagnostic_name_collision_cannot_overwrite_prior_log(self):
        with tempfile.TemporaryDirectory() as d, patch.object(r,"STATE",Path(d)):
            old=Path(d)/"command-error-collision.log";old.write_bytes(b"prior failure evidence")
            r.write(Path(d)/"state.json",{"private":d,"secretReferences":[]})
            with patch.object(r.uuid,"uuid4",return_value=Mock(hex="collision")),patch.object(r.subprocess,"run",return_value=Mock(returncode=1,stdout=b"",stderr=b"new failure")):
                with self.assertRaises(FileExistsError):r.command(["synthetic-command"])
            self.assertEqual(old.read_bytes(),b"prior failure evidence")

    @unittest.skipUnless(os.name=="nt","Windows identity/lock contract")
    def test_operator_lock_and_pid_creation_identity(self):
        with tempfile.TemporaryDirectory() as d:
            with r.exclusive(Path(d)):
                with self.assertRaises(r.Refused):
                    with r.exclusive(Path(d)):self.fail("second writer acquired lock")
        identity=r.process_identity(os.getpid())
        wrong=copy.deepcopy(identity);wrong["created"]+=1
        with self.assertRaises(r.Refused):r.process_identity(os.getpid(),expected=wrong)

    @unittest.skipUnless(os.name=="nt","Windows retained exited-process contract")
    def test_exited_process_with_held_handle_is_absent_for_stop_and_resume(self):
        with subprocess.Popen([sys.executable,"-c","import time; time.sleep(60)"],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL) as child:
            expected=r.process_identity(child.pid)
            kernel,held=r.process_handle(child.pid,allow_missing=True)
            try:
                child.terminate();child.wait(timeout=10)
                self.assertIsNone(r.process_identity(child.pid,allow_missing=True))
                self.assertIsNone(r.process_identity(child.pid,terminate=True,expected=expected,allow_missing=True))
                with self.assertRaises(r.Refused):r.process_identity(child.pid)
            finally:kernel.CloseHandle(held)

    @unittest.skipUnless(os.name=="nt","Windows live-process ownership contract")
    def test_live_foreign_creation_identity_is_not_stopped(self):
        with subprocess.Popen([sys.executable,"-c","import time; time.sleep(60)"],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL) as child:
            try:
                wrong=r.process_identity(child.pid);wrong["created"]+=1
                with self.assertRaisesRegex(r.Refused,"identity drift"):
                    r.process_identity(child.pid,terminate=True,expected=wrong,allow_missing=True)
                self.assertIsNone(child.poll())
            finally:
                child.terminate();child.wait(timeout=10)

    def test_unknown_process_wait_state_refuses_before_identity_or_termination(self):
        kernel=Mock();kernel.WaitForSingleObject.return_value=0xffffffff
        with patch.object(r,"process_handle",return_value=(kernel,123)):
            with self.assertRaisesRegex(r.Refused,"process exit"):
                r.process_identity(42,terminate=True,expected={"pid":42},allow_missing=True)
        kernel.GetProcessTimes.assert_not_called();kernel.QueryFullProcessImageNameW.assert_not_called()
        kernel.TerminateProcess.assert_not_called();kernel.CloseHandle.assert_called_once_with(123)


if __name__=="__main__":unittest.main()
