#!/usr/bin/env python3
"""Recompose API/web against retained isolated B3 providers and business facts.

The original runtime owns the containers. This process owns only its new API,
standalone Next server and loopback browser gateway; neither creates business
facts nor removes another runtime's providers.
"""
import argparse
import hashlib
import json
import os
import shutil
import subprocess
import time
from pathlib import Path
from urllib.parse import urlparse
from b2_smoke import REPO, port, require
from b3_web_gateway import start


def run(runtime, output, api_name, dist_dir):
    old = json.loads(runtime.read_text(encoding="utf-8"))
    require(old["coreContainer"].startswith("dpp-pgi-b3-core-"), "isolated Core required")
    require(urlparse(old["api"]).hostname == "127.0.0.1", "loopback Core required")
    private = Path(old["private"]).resolve()
    require(private.name.startswith("dpp-pgi-b3-"), "private B3 config required")
    envs = json.loads((private / "environment.json").read_text(encoding="utf-8"))
    require(not (output / "runtime.json").exists(), "new output directory required")
    output.mkdir(parents=True, exist_ok=True)
    api_port, next_port, web_port = port(), port(), port()
    api_env = dict(envs["api"], APP_HTTP_ADDR=f"127.0.0.1:{api_port}")
    web_env = dict(envs["web"], PLATFORM_API_BASE_URL=f"http://127.0.0.1:{api_port}",
                   PORT=str(next_port), HOSTNAME="127.0.0.1")
    require(api_name.startswith("pgi-b3-api") and Path(api_name).name == api_name, "B3 binary required")
    require(dist_dir.startswith(".next-b3") and Path(dist_dir).name == dist_dir, "isolated build directory required")
    build = REPO / "apps/web" / dist_dir
    server = build / "standalone"
    require((server / "server.js").is_file(), "standalone B3 build required")
    shutil.copytree(build / "static", server / dist_dir / "static", dirs_exist_ok=True)
    if (REPO / "apps/web/public").exists():
        shutil.copytree(REPO / "apps/web/public", server / "public", dirs_exist_ok=True)
    processes, gateway = [], None
    try:
        def spawn(args, cwd, env, log):
            with (output / log).open("wb") as stream:
                p = subprocess.Popen(args, cwd=cwd, env=env, stdout=stream,
                                     stderr=subprocess.STDOUT,
                                     creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0))
            processes.append(p)
        api_binary = REPO / ".data" / api_name
        spawn([str(api_binary)], REPO / "apps/platform", api_env, "api.log")
        spawn(["node", str(server / "server.js")], server, web_env, "web.log")
        gateway = start(web_port, next_port, "b3-browser-operator", web_env["DELIVERY_WEB_GATEWAY_TOKEN"])
        state = dict(old, api=f"http://127.0.0.1:{api_port}", web=f"http://127.0.0.1:{web_port}",
                     retainedRuntime=str(runtime), apiSha256=hashlib.sha256(api_binary.read_bytes()).hexdigest())
        (output / "runtime.json").write_text(json.dumps(state, indent=2), encoding="utf-8")
        print(json.dumps({"web": state["web"], "api": state["api"]}), flush=True)
        while not (output / "STOP").exists():
            require(all(p.poll() is None for p in processes), "recomposed process stopped; inspect local logs")
            time.sleep(2)
    finally:
        if gateway:
            gateway.shutdown()
            gateway.server_close()
        for p in reversed(processes):
            if p.poll() is None:
                subprocess.run(["taskkill", "/PID", str(p.pid), "/T", "/F"],
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--runtime", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--api-binary",default="pgi-b3-api-v2.exe")
    parser.add_argument("--dist-dir",default=".next-b3")
    args = parser.parse_args()
    run(args.runtime.resolve(), args.output.resolve(),args.api_binary,args.dist_dir)
