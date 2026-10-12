"""Restart only a registered retained trial, without bootstrap or cleanup.

STOP terminates this composition and stops its containers; it preserves every
container, volume, network, credential/config file and staging path.
"""
import argparse,base64,json,os,subprocess,threading,time
from pathlib import Path
from urllib.parse import urlparse
from smoke import FaultProxy,command,require,HERE,REPO
from retained_runtime import DurableStorage
from b3_web_gateway import start

def run(runtime,output,services_only=False):
    state=json.loads(runtime.read_text());private=Path(state["private"])
    require(state.get("run_id") and state["coreContainer"].startswith("dpp-pgi-b3-core-"),"retained runtime registry required")
    output.mkdir(parents=True,exist_ok=False)
    envs=json.loads((private/"environment.json").read_text())
    services=json.loads((runtime.parent/"storage-registry.json").read_text())["services"]
    durable=DurableStorage(output,private,Path(state["durableHelper"]))
    # Inspect all mounts before starting any data service; real DTOs only.
    for service in services:
        status=json.loads(command(["docker","inspect",service["name"]]).stdout)[0]["State"]["Status"]
        require(status in ("running","exited","created"),"unexpected retained service state")
        durable.check([service],state["run_id"],status)
    for name in state["containers"]:
        item=json.loads(command(["docker","inspect",name]).stdout)[0]
        require(item["Config"].get("Labels",{}).get("dpp.trial.run")==state["run_id"],"retained container ownership mismatch")
        command(["docker","start",name])
    durable.check(services,state["run_id"])
    for name in (state["coreContainer"],state["source"]):
        for _ in range(60):
            if command(["docker","exec",name,"pg_isready","-h","127.0.0.1","-U","postgres"],check=False).returncode==0:break
            time.sleep(.5)
        else:raise AssertionError("retained PostgreSQL final TCP server unavailable")
    if services_only:
        (output/"runtime.json").write_text(json.dumps(state,indent=2),encoding="utf-8")
        print(json.dumps({"services":"running","applicationProcesses":0,"bootstrap":False,"cleanup":False}),flush=True)
        return  # Quiescent native services for backup; no business worker starts.
    processes=[];proxy=gateway=None
    try:
        token=(private/"hop-password").read_text().strip()
        proxy=FaultProxy(state["hopUpstream"],"Basic "+base64.b64encode(("b1:"+token).encode()).decode(),urlparse(state["hopProxy"]).port)
        threading.Thread(target=proxy.serve_forever,daemon=True).start()
        def spawn(args,cwd,env,log):
            with (output/log).open("wb") as f:
                p=subprocess.Popen(args,cwd=cwd,env=env,stdout=f,stderr=subprocess.STDOUT,creationflags=getattr(subprocess,"CREATE_NO_WINDOW",0))
            processes.append(p)
        for app in ("api","worker"):
            spawn([str(REPO/f'.data/{state["binaryPrefix"]}-{app}.exe')],REPO/"apps/platform",envs["api"],app+".log")
        spawn(["cmd.exe","/d","/s","/c",f'npm.cmd run start -- --hostname 127.0.0.1 --port {state["nextPort"]}'],REPO/"apps/web",envs["web"],"web.log")
        gateway=start(urlparse(state["web"]).port,state["nextPort"],"b3-browser-operator",envs["web"]["DELIVERY_WEB_GATEWAY_TOKEN"])
        (output/"runtime.json").write_text(json.dumps(state,indent=2),encoding="utf-8")
        print(json.dumps({"web":state["web"],"api":state["api"],"bootstrap":False,"cleanup":False}),flush=True)
        while not (output/"STOP").exists():
            require(all(p.poll() is None for p in processes),"resumed process stopped; inspect private local logs")
            time.sleep(2)
    finally:
        for server in (gateway,proxy):
            if server:server.shutdown();server.server_close()
        for p in reversed(processes):
            if p.poll() is None:
                if os.name=="nt":subprocess.run(["taskkill","/PID",str(p.pid),"/T","/F"],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
                else:p.terminate()
        for name in reversed(state["containers"]):command(["docker","stop","--time","20",name],check=False)

if __name__=="__main__":
    p=argparse.ArgumentParser();p.add_argument("--runtime",type=Path,required=True);p.add_argument("--output",type=Path,required=True)
    p.add_argument("--services-only",action="store_true",help="start retained providers for quiescent backup, without API/web/worker")
    a=p.parse_args();run(a.runtime.resolve(),a.output.resolve(),a.services_only)
