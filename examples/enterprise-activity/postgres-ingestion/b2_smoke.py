#!/usr/bin/env python3
"""#298/B2: real PostgreSQL + Hop + isolated MinIO gateway + Core HTTP commands.

Synthetic local fixtures only. The Go suite creates native scoped identities,
executes all Commands through HTTP, and verifies facts independently in Core.
"""
import argparse
import csv
import hashlib
import http.server
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import tempfile
import threading
import time
import urllib.request

from smoke import Trial, PG_IMAGE, SLOTS, command, require, source_sha, fixture
from storage_smoke import prepare_binary, CADDY, SOURCE_COMMIT, SOURCE_RELEASE

HERE=Path(__file__).resolve().parent
REPO=HERE.parents[2]

def port():
    with socket.socket() as s:
        s.bind(("127.0.0.1",0));return s.getsockname()[1]

class Control(http.server.BaseHTTPRequestHandler):
    def log_message(self,*_): pass
    def do_POST(self):
        owner=self.server
        if self.headers.get("Authorization")!="Bearer "+owner.token:
            self.send_error(403);return
        action=self.path.lstrip("/")
        trial=owner.trial
        if action=="drop-register": trial.proxy.drop="/registerPipeline"
        elif action=="drop-start": trial.proxy.drop="/startPipeline"
        elif action=="status-unavailable": trial.proxy.unavailable=True
        elif action=="status-without-identity": trial.proxy.strip_status_identity=True
        elif action=="reset": trial.proxy.unavailable=False;trial.proxy.drop=None;trial.proxy.strip_status_identity=False
        elif action=="source-revoke": trial.psql("REVOKE SELECT ON dpp_source.lease FROM dpp_reader;")
        elif action=="source-restore": trial.psql("GRANT SELECT ON dpp_source.lease TO dpp_reader;")
        elif action=="fail-start": trial.proxy.before_start=lambda:trial.psql("REVOKE SELECT ON dpp_source.lease FROM dpp_reader;")
        elif action=="restart-hop": trial.docker("restart","-t","5",trial.hop);trial.wait_hop()
        else: self.send_error(400);return
        self.send_response(204);self.end_headers()

def run(output,cache):
    output.mkdir(parents=True,exist_ok=False)
    trial=Trial(output);trial.source_port=port()
    private=Path(tempfile.mkdtemp(prefix="dpp-pgi-b2-core-"))
    names=[];networks=[];control=None
    report={"contract":"postgres-ingestion-v1/B2","status":"NOT_RUN","core_admission":"NOT_RUN","b3":"NOT_RUN","remote_ci":"NOT_RUN",
            "base_sha":command(["git","-C",str(REPO),"rev-parse","HEAD"]).stdout.decode().strip(),
            "scope":"synthetic fixtures; quiescent source; controlled authenticated HTTP; real local providers",
            "storage_boundary":"official archived MinIO source + mandatory Caddy gateway; no production admission",
            "minio_commit":SOURCE_COMMIT,"minio_release":SOURCE_RELEASE}
    try:
        print("B2: preparing pinned runtimes",flush=True)
        binary,minio_image=prepare_binary(cache,trial.images.tag("minio"),owner=trial.images)
        trial.setup()
        report["runtime"]=trial.report["runtime"]
        report["minio_binary_sha256"]=hashlib.sha256(binary.read_bytes()).hexdigest()
        suffix=trial.identity.removeprefix("dpp-pgi-")
        net="dpp-pgi-b2-storage-private-"+suffix
        command(["docker","network","create",net]);networks.append(net)
        rootkey,rootsecret,coresecret,token=secrets.token_hex(12),secrets.token_hex(32),secrets.token_hex(24),secrets.token_hex(32)
        storage_private=private/"storage";storage_private.mkdir()
        for name,value in [("access",rootkey),("secret",rootsecret)]:
            (storage_private/name).write_text(value,encoding="utf-8")
        for name,value in [("core-password",coresecret),("token",token),("hop-password",trial.passwords[2]),("source-password",trial.passwords[1])]:
            (private/name).write_text(value,encoding="utf-8")
        (storage_private/"start.sh").write_text("#!/bin/sh\nset -eu\nexport MINIO_ROOT_USER=$(cat /run/credentials/access)\nexport MINIO_ROOT_PASSWORD=$(cat /run/credentials/secret)\nexec /usr/local/bin/minio server /data --address :9000\n",encoding="utf-8",newline="\n")
        minio="dpp-pgi-b2-storage-"+suffix
        command(["docker","run","-d","--name",minio,"--network",net,"--network-alias","minio","--mount",f"type=bind,src={storage_private},dst=/run/credentials,readonly","--tmpfs","/data:rw,size=256m","--entrypoint","sh",minio_image,"/run/credentials/start.sh"]);names.append(minio)
        gateway="dpp-pgi-b2-gateway-"+suffix;sp=port()
        command(["docker","run","-d","--name",gateway,"--network",net,"-p",f"127.0.0.1:{sp}:9000","--mount",f"type=bind,src={HERE/'storage.Caddyfile'},dst=/etc/caddy/Caddyfile,readonly",CADDY]);names.append(gateway)
        command(["docker","network","connect",trial.network,gateway])
        deadline=time.monotonic()+45
        while time.monotonic()<deadline:
            try:
                with urllib.request.urlopen(f"http://127.0.0.1:{sp}/minio/health/live",timeout=2) as r:
                    if r.status==200:break
            except Exception:time.sleep(.5)
        else:raise AssertionError("storage unavailable")
        core="dpp-pgi-b2-core-"+suffix;cp=port()
        command(["docker","run","-d","--name",core,"--network",net,"-p",f"127.0.0.1:{cp}:5432","-e","POSTGRES_DB=dpp_ingestion_core","-e","POSTGRES_PASSWORD_FILE=/run/credentials/core-password","--mount",f"type=bind,src={private/'core-password'},dst=/run/credentials/core-password,readonly","--tmpfs","/var/lib/postgresql/data:rw,size=256m",PG_IMAGE]);names.append(core)
        for _ in range(60):
            if command(["docker","exec",core,"pg_isready","-U","postgres"],check=False).returncode==0:break
            time.sleep(.5)
        else:raise AssertionError("Core PostgreSQL unavailable")
        metadata={"rdbms":[json.loads((HERE/"metadata/rdbms/dpp-source.json").read_text())],"pipeline-run-configuration":[json.loads((HERE/"metadata/pipeline-run-configuration/local.json").read_text())]}
        pg_metadata=metadata["rdbms"][0]["rdbms"]["POSTGRESQL"]
        pg_metadata["hostname"]=trial.source_host
        pg_metadata["port"]=str(trial.source_port)
        (private/"metadata.json").write_text(json.dumps(metadata),encoding="utf-8")
        control=http.server.ThreadingHTTPServer(("127.0.0.1",0),Control)
        control.token=secrets.token_hex(24);control.trial=trial
        threading.Thread(target=control.serve_forever,daemon=True).start()
        cfg={"DSN":f"postgres://postgres:{coresecret}@127.0.0.1:{cp}/dpp_ingestion_core?sslmode=disable", "Private":str(private),"Output":str(output),"Migrations":str(REPO/"migrations"),"ControlURL":f"http://127.0.0.1:{control.server_port}","ControlToken":control.token,"RootKey":rootkey,"RootSecret":rootsecret,
             "Profile":{"Enabled":True,"TokenFile":str(private/"token"),"Subject":"b2-controlled-operator","ConnectionRef":"dpp-source-reader","InstanceRef":trial.identity,"Database":"dpp_ingestion_source","Reader":"dpp_reader","SourceDSN":f"postgres://dpp_reader@{trial.source_host}:{trial.source_port}/dpp_ingestion_source?sslmode=disable","SourcePasswordFile":str(private/"source-password"),"TemplateFile":str(HERE/"extract.hpl"),"MetadataFile":str(private/"metadata.json"),"Tables":[{"Code":table+"_raw","Schema":"dpp_source","Name":table,"Columns":columns,"Rows":rows} for table,(columns,_,rows) in SLOTS.items()],"HopURL":trial.url,"HopUser":"b1","HopPasswordFile":str(private/"hop-password"),"LocalRoot":str(trial.staging),"EngineRoot":"/staging","Bucket":"raw","StorageEndpoint":f"127.0.0.1:{sp}","CoreCredentialFile":str(private/"core-identity.json"),"PublisherCredentialFile":str(private/"publisher-identity.json")}}
        config=private/"test.json";config.write_text(json.dumps(cfg),encoding="utf-8")
        print("B2: running authenticated Commands and independent Core assertions",flush=True)
        sources=[*list((REPO/"apps/platform/internal/ingestion").rglob("*.go")),*list((REPO/"apps/platform/cmd/ingestion-pilot").rglob("*.go")),REPO/"apps/platform/go.mod",REPO/"apps/platform/go.sum",REPO/"migrations/000051_ingestion_batch.up.sql",REPO/"migrations/000051_ingestion_batch.down.sql",REPO/"migrations/000052_ingestion_invocation_recovery.up.sql",REPO/"migrations/000052_ingestion_invocation_recovery.down.sql",REPO/"apps/platform/internal/platform/storage/minio.go",REPO/"apps/platform/internal/platform/routing/routing.go",REPO/"apps/platform/internal/workflow/hop/client.go",HERE/"b2_smoke.py",HERE/"smoke.py",HERE/"extract.hpl",HERE/"storage.Caddyfile",HERE/"source.sql",HERE/"start-hop.sh",REPO/".github/workflows/ci.yml"]
        hashes={str(f.relative_to(REPO)).replace("\\","/"):source_sha(f) for f in sources}
        env=dict(os.environ,TEST_INGESTION_B2_CONFIG=str(config))
        result=subprocess.run(["go","test","-count=1","-v","-timeout","12m","./internal/ingestion/..."],cwd=REPO/"apps/platform",env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=780)
        output_text=result.stdout+result.stderr
        known=trial.passwords+[rootkey,rootsecret,coresecret,token,control.token]
        for f in private.glob("*-identity.json"):
            identity=json.loads(f.read_text());known += [identity[k] for k in ("AccessKey","SecretKey","SessionToken") if identity.get(k)]
        require(all(value.encode() not in output_text for value in known),"credential leaked into test output")
        (output/"b2-tests.log").write_bytes(output_text)
        if result.returncode:
            # Print only after the same secret check used for the retained log.
            # CI must preserve the assertion cause instead of only a file path.
            print(b"\n".join(output_text.splitlines()[-80:])[-16384:].decode("utf-8",errors="replace"),flush=True)
        require(result.returncode==0,"Core batch tests failed; see sanitized b2-tests.log")
        wire=json.dumps(trial.proxy.requests,ensure_ascii=False).encode()
        require(all(value.encode() not in wire for value in known),"credential leaked into Hop requests")
        for container in [trial.pg,trial.hop,*names]:
            logs=command(["docker","logs",container],check=False)
            inspect=command(["docker","inspect",container]).stdout
            require(all(value.encode() not in inspect+logs.stdout+logs.stderr for value in known),"credential leaked into container metadata/logs")
        facts=json.loads((output/"b2-facts.json").read_text())
        physical=sum(any(name in r["url"] for name in ("/registerPipeline","/startPipeline","/pipelineStatus")) for r in trial.proxy.requests)
        require(physical==facts["invocations"],"physical Hop calls differ from durable invocation/cost facts")
        report.update(facts)
        require(set(command(["docker","exec",minio,"ls","/run/credentials"]).stdout.decode().split())=={"access","secret","start.sh"},"storage provider can see other providers' credentials")
        require(set(command(["docker","exec",trial.hop,"ls","/run/secrets"]).stdout.decode().split())=={"source-password","hop-server.xml"},"Hop has a Core/admin/storage secret mount")
        report["checks"]["provider_secret_mount_boundaries"]="PASS"
        for manifest in facts["manifests"]:
            for slot in manifest["outputs"]:
                table=slot["code"].removesuffix("_raw")
                with (trial.staging/manifest["attemptId"]/(slot["code"]+".csv")).open(encoding="utf-8",newline="") as f:
                    values=list(csv.reader(f,strict=True))
                require(values[0]==SLOTS[table][0] and values[1:]==fixture(table),"RAW original field fidelity rejected")
        report["checks"]["source_to_hop_to_core_original_values"]="PASS"
        report["checks"]["physical_call_cost_correspondence"]="PASS"
        report["checks"]["secret_isolation"]="PASS"
        require(hashes=={str(f.relative_to(REPO)).replace("\\","/"):source_sha(f) for f in sources},"implementation changed during real run")
        report["implementation_sha256"]=hashes
        report["status"]=report["core_admission"]="PASS"
    except Exception as error:
        report["status"]="FAIL";report["failure"]=str(error);raise
    finally:
        if control:control.shutdown();control.server_close()
        container_cleanup=[];network_cleanup=[];trial_cleanup_error=None
        for name in reversed(names):container_cleanup.append(command(["docker","rm","-f",name],check=False).returncode==0)
        # Disconnect/remove the gateway before Trial removes its engine network.
        try:trial.cleanup()
        except Exception as error:trial_cleanup_error=type(error).__name__
        container_cleanup += [command(["docker","inspect",name],check=False).returncode!=0 for name in (trial.pg,trial.hop)]
        network_cleanup.append(command(["docker","network","inspect",trial.network],check=False).returncode!=0)
        for net in reversed(networks):network_cleanup.append(command(["docker","network","rm",net],check=False).returncode==0)
        report["cleanup_detail"]={"b2_containers":"PASS" if all(container_cleanup) else "FAIL",
            "b2_networks":"PASS" if all(network_cleanup) else "FAIL",
            "trial":trial.report.get("cleanup_detail",{"status":"UNKNOWN"}),"temporary_credentials":"NOT_RUN",
            "trial_cleanup_error":trial_cleanup_error}
        report["cleanup"]="FAIL"
        try:
            require(private.resolve().parent==Path(tempfile.gettempdir()).resolve() and private.name.startswith("dpp-pgi-b2-core-"),"unsafe credential cleanup")
            shutil.rmtree(private)
            report["cleanup_detail"]["temporary_credentials"]="PASS"
            if all(container_cleanup+network_cleanup) and trial.report.get("cleanup")=="PASS" and trial_cleanup_error is None:
                report["cleanup"]="PASS"
        finally:
            (output/"b2-report.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n",encoding="utf-8")
    require(report["cleanup"]=="PASS","B2 cleanup incomplete; see component receipt")
    print("B2 PASS; B3 NOT_RUN",flush=True)

if __name__=="__main__":
    parser=argparse.ArgumentParser();parser.add_argument("--output",required=True,type=Path);parser.add_argument("--cache",type=Path,default=REPO/".data/pgi-b2-storage-cache")
    args=parser.parse_args();run(args.output.resolve(),args.cache.resolve())
