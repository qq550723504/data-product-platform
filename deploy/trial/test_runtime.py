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
        return {"runId":"mine","root":str(r.ROOT),"phase":"UNKNOWN","host":{"daemonId":"local"},"containers":{"core":{"name":"mine-core","id":"sha-container","configHash":r.container_hash(item)}},"images":{"core":{"id":"sha-image"}},"processes":{},"volumes":{"core":"retained"}}

    def test_foreign_or_replaced_container_cannot_be_stopped(self):
        for field,value in (("Id","replaced"),("Image","foreign-image"),("Name","/foreign")):
            item=self.container();item[field]=value
            calls=[]
            def docker(*args,**kw):
                calls.append(args)
                if args[0]=="ps":return b"sha-container"
                if args[0]=="inspect":return json.dumps([item]).encode()
                self.fail("foreign container reached stop")
            with patch.object(r,"preflight",return_value={"daemonId":"local"}),patch.object(r,"docker",side_effect=docker):
                with self.assertRaises(r.Refused):r.stop(self.state())
            self.assertFalse(any(c[0]=="stop" for c in calls))

    def test_network_or_nonloopback_drift_refused(self):
        for mutation in (lambda x:x["NetworkSettings"]["Networks"].update({"bypass":{}}),lambda x:x["HostConfig"]["PortBindings"]["5432/tcp"][0].update({"HostIp":"0.0.0.0"})):
            item=self.container();mutation(item)
            with self.assertRaises(r.Refused):r.verify_container(self.state(),"core",item)

    def test_stop_uses_exact_container_id_and_retains_all_resources(self):
        calls=[];s=self.state();item=self.container()
        def docker(*args,**kw):
            calls.append(args)
            if args[0]=="ps":return b"sha-container"
            if args[0]=="inspect":return json.dumps([item]).encode()
            if args[0]=="stop":return b"sha-container"
            self.fail("unexpected resource mutation")
        with patch.object(r,"preflight",return_value={"daemonId":"local"}),patch.object(r,"docker",side_effect=docker),patch.object(r,"save"):
            result=r.stop(s)
        self.assertEqual(calls[-1],("stop","--time","30","sha-container"));self.assertTrue(result["retained"])

    def test_unknown_commands_rejected_before_docker(self):
        for action in ("reset","cleanup","down","delete","source-revoke"):
            result=subprocess.run([sys.executable,str(r.ROOT/"deploy/trial/trial.py"),action],capture_output=True)
            self.assertEqual(result.returncode,2)

    def test_no_application_secrets_or_compose_overrides_inherited(self):
        with patch.dict(os.environ,{"RIGHTS_API_TOKEN":"private","NEXT_PUBLIC_TOKEN":"private","COMPOSE_FILE":"foreign","DOCKER_HOST":"tcp://remote","INGESTION_PROFILE":"foreign"}):
            env=r.environment()
        self.assertFalse(any(k in env for k in ("RIGHTS_API_TOKEN","NEXT_PUBLIC_TOKEN","COMPOSE_FILE","DOCKER_HOST","INGESTION_PROFILE")))

    @unittest.skipUnless(os.name=="nt","Windows identity/lock contract")
    def test_operator_lock_and_pid_creation_identity(self):
        with tempfile.TemporaryDirectory() as d:
            with r.exclusive(Path(d)):
                with self.assertRaises(r.Refused):
                    with r.exclusive(Path(d)):self.fail("second writer acquired lock")
        identity=r.process_identity(os.getpid())
        wrong=copy.deepcopy(identity);wrong["created"]+=1
        with self.assertRaises(r.Refused):r.process_identity(os.getpid(),expected=wrong)


if __name__=="__main__":unittest.main()
