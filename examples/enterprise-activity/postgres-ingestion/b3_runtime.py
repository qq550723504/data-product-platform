#!/usr/bin/env python3
"""Isolated B3 runtime. Reuses B1 Trial and B2 native storage gateway.

Only synthetic prerequisites are prepared. Business commands are operated in
Edge against the separately built Core API and its real queue worker.
"""
import hashlib, json, os, re, secrets, subprocess, tempfile, time, urllib.request
from pathlib import Path
from b2_smoke import port, prepare_binary, CADDY, Trial, PG_IMAGE, SLOTS, command, require, HERE, REPO
from b3_web_gateway import start as start_web_gateway

def run(output,cache,binary_prefix="pgi-b3"):
    require(re.fullmatch(r"pgi-b3(?:-[a-z0-9-]+)?",binary_prefix),"invalid local binary prefix")
    output.mkdir(parents=True,exist_ok=False)
    trial=Trial(output);trial.source_port=port()
    private=Path(tempfile.mkdtemp(prefix="dpp-pgi-b3-"))
    names=[];networks=[];processes=[];report={};web_gateway=None
    try:
        binary=prepare_binary(cache)
        trial.setup()
        report["runtime"]=trial.report["runtime"]
        report["minio_binary_sha256"]=hashlib.sha256(binary.read_bytes()).hexdigest()
        suffix=trial.identity.removeprefix("dpp-pgi-")
        net="dpp-pgi-b3-storage-private-"+suffix
        command(["docker","network","create",net]);networks.append(net)
        rootkey,rootsecret,coresecret,token=secrets.token_hex(12),secrets.token_hex(32),secrets.token_hex(24),secrets.token_hex(32)
        storage_private=private/"storage";storage_private.mkdir()
        for name,value in [("access",rootkey),("secret",rootsecret)]:
            (storage_private/name).write_text(value,encoding="utf-8")
        for name,value in [("core-password",coresecret),("token",token),("hop-password",trial.passwords[2]),("source-password",trial.passwords[1])]:
            (private/name).write_text(value,encoding="utf-8")
        (storage_private/"start.sh").write_text("#!/bin/sh\nset -eu\nexport MINIO_ROOT_USER=$(cat /run/credentials/access)\nexport MINIO_ROOT_PASSWORD=$(cat /run/credentials/secret)\nexec /usr/local/bin/minio server /data --address :9000\n",encoding="utf-8",newline="\n")
        minio="dpp-pgi-b3-storage-"+suffix
        command(["docker","run","-d","--name",minio,"--network",net,"--network-alias","minio","--mount",f"type=bind,src={storage_private},dst=/run/credentials,readonly","--tmpfs","/data:rw,size=256m","--entrypoint","sh","dpp-pgi-minio:2025-10-15","/run/credentials/start.sh"]);names.append(minio)
        gateway="dpp-pgi-b3-gateway-"+suffix;sp=port()
        command(["docker","run","-d","--name",gateway,"--network",net,"-p",f"127.0.0.1:{sp}:9000","--mount",f"type=bind,src={HERE/'storage.Caddyfile'},dst=/etc/caddy/Caddyfile,readonly",CADDY]);names.append(gateway)
        command(["docker","network","connect",trial.network,gateway])
        deadline=time.monotonic()+45
        while time.monotonic()<deadline:
            try:
                with urllib.request.urlopen(f"http://127.0.0.1:{sp}/minio/health/live",timeout=2) as r:
                    if r.status==200:break
            except Exception:time.sleep(.5)
        else:raise AssertionError("storage unavailable")
        core="dpp-pgi-b3-core-"+suffix;cp=port()
        command(["docker","run","-d","--name",core,"--network",net,"-p",f"127.0.0.1:{cp}:5432","-e","POSTGRES_DB=dpp_ingestion_core","-e","POSTGRES_PASSWORD_FILE=/run/credentials/core-password","--mount",f"type=bind,src={private/'core-password'},dst=/run/credentials/core-password,readonly","--tmpfs","/var/lib/postgresql/data:rw,size=256m",PG_IMAGE]);names.append(core)
        for _ in range(60):
            if command(["docker","exec",core,"pg_isready","-U","postgres"],check=False).returncode==0:break
            time.sleep(.5)
        else:raise AssertionError("Core PostgreSQL unavailable")
        metadata={"rdbms":[json.loads((HERE/"metadata/rdbms/dpp-source.json").read_text())],"pipeline-run-configuration":[json.loads((HERE/"metadata/pipeline-run-configuration/local.json").read_text())]}
        (private/"metadata.json").write_text(json.dumps(metadata),encoding="utf-8")
        cfg={"DSN":f"postgres://postgres:{coresecret}@127.0.0.1:{cp}/dpp_ingestion_core?sslmode=disable", "Private":str(private),"Output":str(output),"Migrations":str(REPO/"migrations"),"RootKey":rootkey,"RootSecret":rootsecret,
             "Profile":{"Enabled":True,"TokenFile":str(private/"token"),"Subject":"b3-controlled-operator","ConnectionRef":"dpp-source-reader","InstanceRef":trial.identity,"Database":"dpp_ingestion_source","Reader":"dpp_reader","SourceDSN":f"postgres://dpp_reader@127.0.0.1:{trial.source_port}/dpp_ingestion_source?sslmode=disable","SourcePasswordFile":str(private/"source-password"),"TemplateFile":str(HERE/"extract.hpl"),"MetadataFile":str(private/"metadata.json"),"Tables":[{"Code":table+"_raw","Schema":"dpp_source","Name":table,"Columns":columns,"Rows":rows} for table,(columns,_,rows) in SLOTS.items()],"HopURL":trial.url,"HopUser":"b1","HopPasswordFile":str(private/"hop-password"),"LocalRoot":str(trial.staging),"EngineRoot":"/staging","Bucket":"raw","StorageEndpoint":f"127.0.0.1:{sp}","CoreCredentialFile":str(private/"core-identity.json"),"PublisherCredentialFile":str(private/"publisher-identity.json")}}
        config=private/"test.json";config.write_text(json.dumps(cfg),encoding="utf-8")
        cfg["Repo"]=str(REPO)
        config=private/"test.json";config.write_text(json.dumps(cfg),encoding="utf-8")
        env=dict(os.environ,TEST_INGESTION_B3_CONFIG=str(config))
        fixture=str(REPO/f".data/{binary_prefix}-fixture.exe")
        result=subprocess.run([fixture,"bootstrap"],env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
        (private/"bootstrap.log").write_bytes(result.stdout+result.stderr)
        require(result.returncode==0,"B3 bootstrap failed; inspect private bootstrap log")
        public=json.loads((private/"public.json").read_text())
        ap,wp,rp=port(),port(),port()
        command(["docker","run","-d","--name","dpp-pgi-b3-redis-"+suffix,"--network",net,"-p",f"127.0.0.1:{rp}:6379","redis:7-alpine","redis-server","--save","","--appendonly","no"]);names.append("dpp-pgi-b3-redis-"+suffix)
        api_env=dict(env,APP_ENV="pgi-b3-local",APP_HTTP_ADDR=f"127.0.0.1:{ap}",POSTGRES_DSN=cfg["DSN"],REDIS_ADDR=f"127.0.0.1:{rp}",OBJECT_STORAGE_ENDPOINT=f"127.0.0.1:{sp}",OBJECT_STORAGE_BUCKET="raw",OBJECT_STORAGE_ACCESS_KEY=rootkey,OBJECT_STORAGE_SECRET_KEY=rootsecret,OBJECT_STORAGE_USE_SSL="false",INGESTION_PROFILE=str(private/"profile.json"),INDUSTRY_PACK_ROOT=str(REPO/"industry-packs"),RIGHTS_API_TOKEN=token,RIGHTS_API_ACTOR_ID=public["actorId"],RIGHTS_API_WORKSPACE_IDS=public["workspaceId"],DELIVERY_API_ENABLED="true",DELIVERY_API_TOKEN=token,DELIVERY_API_PRINCIPAL_REF="b3-browser-operator",DELIVERY_API_CONSUMER_REF="LICENSED_BANK",DELIVERY_API_WORKSPACE_IDS=public["workspaceId"],HUMAN_DECISION_API_ENABLED="true",HUMAN_DECISION_API_TOKEN=token,HUMAN_DECISION_API_SUBJECT="b3-browser-reviewer",HUMAN_DECISION_API_ACTOR_ID=public["actorId"],HUMAN_DECISION_API_WORKSPACE_IDS=public["workspaceId"],OPENMETADATA_ENABLED="true",OPENMETADATA_BASE_URL=f"http://127.0.0.1:{port()}/api",OPENMETADATA_DOMAIN="SyntheticPark",HOP_ENABLED="false",SPLINK_ENABLED="false",GX_ENABLED="false",LABEL_STUDIO_ENABLED="false")
        web_env=dict(env,PLATFORM_API_BASE_URL=f"http://127.0.0.1:{ap}",POC_WORKSPACE_ID=public["workspaceId"],POC_ENABLE_INGESTION_ACTIONS="true",POC_INGESTION_TOKEN_FILE=str(private/"token"),POC_RIGHTS_TOKEN_FILE=str(private/"token"),POC_INGEST_ACTOR_ID=public["actorId"],POC_INGESTION_WORKFLOW_ID=public["workflowVersionId"],POC_ENABLE_REVIEW_ACTIONS="true",POC_REVIEW_API_TOKEN=token,POC_REVIEW_ACTOR_ID=public["actorId"],POC_ENABLE_QUALITY_ACTIONS="true",POC_QUALITY_ACTOR_ID=public["actorId"],POC_ENABLE_COMPLIANCE_ACTIONS="true",POC_COMPLIANCE_ACTOR_ID=public["actorId"],POC_ENABLE_DELIVERY_ACTIONS="true",HUMAN_DECISION_API_TOKEN=token,DELIVERY_API_TOKEN=token,DELIVERY_API_CONSUMER_REF="LICENSED_BANK",DELIVERY_API_PRINCIPAL_REF="b3-browser-operator",DELIVERY_WEB_GATEWAY_TOKEN=secrets.token_hex(32))
        # Runtime secrets remain private and server-side; no NEXT_PUBLIC secret.
        web_env["NEXT_DIST_DIR"]=".next-"+binary_prefix
        (private/"environment.json").write_text(json.dumps({"api":api_env,"web":web_env}),encoding="utf-8")
        def spawn(args,cwd,env,log):
            f=(output/log).open("wb")
            p=subprocess.Popen(args,cwd=cwd,env=env,stdout=f,stderr=subprocess.STDOUT,creationflags=getattr(subprocess,"CREATE_NO_WINDOW",0));processes.append(p);return p
        for app in ("api","worker"):
            spawn([str(REPO/f".data/{binary_prefix}-{app}.exe")],REPO/"apps/platform",api_env,f"{app}.log")
        spawn(["cmd.exe","/d","/s","/c",f"npm.cmd run dev -- --hostname 127.0.0.1 --port {wp}"],REPO/"apps/web",web_env,"web.log")
        public_port=port();web_gateway=start_web_gateway(public_port,wp,"b3-browser-operator",web_env["DELIVERY_WEB_GATEWAY_TOKEN"])
        state={"private":str(private),"coreContainer":core,"api":f"http://127.0.0.1:{ap}","web":f"http://127.0.0.1:{public_port}","workspaceId":public["workspaceId"],"actorId":public["actorId"],"containers":names,"source":trial.pg,"hop":trial.hop}
        (output/"runtime.json").write_text(json.dumps(state,indent=2),encoding="utf-8")
        print(json.dumps({"web":state["web"],"api":state["api"],"runtime":str(output/"runtime.json")}),flush=True)
        while not (output/"STOP").exists():
            if any(p.poll() is not None for p in processes):raise AssertionError("B3 runtime process stopped; inspect private local logs")
            time.sleep(2)
    finally:
        if web_gateway:web_gateway.shutdown();web_gateway.server_close()
        for p in reversed(processes):
            if p.poll() is None:
                if os.name=="nt":subprocess.run(["taskkill","/PID",str(p.pid),"/T","/F"],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
                else:p.terminate()
        for name in reversed(names):command(["docker","rm","-f",name],check=False)
        trial.cleanup()
        for net in networks:command(["docker","network","rm",net],check=False)
        # Preserve private files until explicit local trial cleanup; they are never committed.

if __name__=="__main__":
    import argparse
    parser=argparse.ArgumentParser();parser.add_argument("--output",type=Path,required=True);parser.add_argument("--cache",type=Path,default=REPO/".data/pgi-b2-storage-cache");parser.add_argument("--binary-prefix",default="pgi-b3")
    args=parser.parse_args();run(args.output.resolve(),args.cache.resolve(),args.binary_prefix)
