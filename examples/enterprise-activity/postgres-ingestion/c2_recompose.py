"""Recompose final C2 code against retained, isolated B3/native providers.

Builds a production Next bundle. The original runtime keeps provider ownership;
STOP only terminates this API/web/worker composition. Private env files are never
copied into the public report. Build binaries before calling this script.
"""
import argparse, json, subprocess
from pathlib import Path
from b3_recompose import run
from b2_smoke import REPO, require

def recompose(runtime, output, reuse_build=False):
    state=json.loads(runtime.read_text()); private=Path(state["private"])
    require(state["coreContainer"].startswith("dpp-pgi-b3-core-"),"isolated Core required")
    envs=json.loads((private/"environment.json").read_text())
    require(envs["api"].get("OPENMETADATA_SCHEMA_PROFILE"),"native schema profile required")
    output.mkdir(parents=True,exist_ok=True)
    if not reuse_build:
        with (output/"web-build.log").open("wb") as log:
            result=subprocess.run(["cmd.exe","/d","/s","/c","npm.cmd run build"],cwd=REPO/"apps/web",
                env=dict(envs["web"],NEXT_DIST_DIR=".next-b3-c2",NODE_ENV="production"),stdout=log,stderr=subprocess.STDOUT)
        require(result.returncode==0,"production build failed; inspect local build log")
    with (output/"worker.log").open("wb") as log:
        worker=subprocess.Popen([str(REPO/".data/pgi-b3-c2-final-worker.exe")],cwd=REPO/"apps/platform",
            env=envs["api"],stdout=log,stderr=subprocess.STDOUT,creationflags=subprocess.CREATE_NO_WINDOW)
    (output/"worker-process.json").write_text(json.dumps({"pid":worker.pid}))
    try:
        run(runtime,output,"pgi-b3-api-c2-final.exe",".next-b3-c2")
    finally:
        worker.terminate()

if __name__=="__main__":
    p=argparse.ArgumentParser();p.add_argument("--runtime",type=Path,required=True);p.add_argument("--output",type=Path,required=True)
    p.add_argument("--reuse-build",action="store_true",help="reuse an unchanged final production bundle")
    a=p.parse_args();recompose(a.runtime.resolve(),a.output.resolve(),a.reuse_build)
