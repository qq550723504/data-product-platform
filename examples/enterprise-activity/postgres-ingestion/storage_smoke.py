#!/usr/bin/env python3
"""B2 storage gate: official source build, real MinIO/Caddy, scoped STS identities.

No existing service, volume, long-lived credential or Core database is touched.
The upstream OSS authentication defect is mitigated at the sole gateway as
documented by its vendor; this is not a patched MinIO or production acceptance.
"""
import argparse
import hashlib
import json
import os
import re
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import tempfile
import time
import urllib.request
import uuid

from smoke import command, require, source_sha
from trial_images import TrialImages
from cleanup_receipts import CleanupReceipts

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[2]
SOURCE_COMMIT = "9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a"
SOURCE_RELEASE = "RELEASE.2025-10-15T17-29-55Z"
ALPINE = "alpine:3.22@sha256:5291449c3df73caf6ed85e649dec1b9e818b39a5d8c871e97afc13e9cd5e8fa8"
CADDY = "caddy:2.11.7-alpine@sha256:d8542f48d34a9cf4e4c11a478865229840e87e4c96ea3f439101f31a5d35f75f"


def prepare_binary(cache, image_tag, *, owner):
    require(re.fullmatch(r"dpp-pgi-minio:trial-[0-9a-f]{32}", image_tag),
            "a unique trial image tag is required")
    cache.mkdir(parents=True, exist_ok=True)
    source = cache / "source"
    if not source.exists():
        command(["git", "clone", "--depth", "1", "--branch", SOURCE_RELEASE,
                 "https://github.com/minio/minio.git", str(source)])
    result = command(["git", "-C", str(source), "rev-parse", "HEAD"])
    require(result.stdout.decode().strip() == SOURCE_COMMIT, "MinIO source commit mismatch")
    require(not command(["git", "-C", str(source), "status", "--porcelain"]).stdout,
            "MinIO source must be unmodified")
    binary = cache / "minio"
    env = dict(os.environ, GOOS="linux", GOARCH="amd64", CGO_ENABLED="0", GOTOOLCHAIN="local")
    result = subprocess.run(["go", "build", "-trimpath", "-o", str(binary), "-ldflags",
        f"-s -w -X github.com/minio/minio/cmd.Version={SOURCE_RELEASE} "
        f"-X github.com/minio/minio/cmd.CommitID={SOURCE_COMMIT}", "."],
        cwd=source, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=1200)
    require(result.returncode == 0, "official MinIO source build failed (raw diagnostics withheld)")
    shutil.copyfile(source / "LICENSE", cache / "LICENSE")
    (cache / ".dockerignore").write_text("*\n!minio\n!LICENSE\n!Dockerfile\n", encoding="utf-8")
    (cache / "Dockerfile").write_text(f"FROM {ALPINE}\nCOPY minio /usr/local/bin/minio\n"
        "COPY LICENSE /licenses/minio/LICENSE\n", encoding="utf-8")
    image_id = owner.build(image_tag, [str(cache)], timeout=300)
    return binary, image_id


def run(output, cache):
    output.mkdir(parents=True, exist_ok=True)
    owner = TrialImages(output, str(uuid.uuid4()), False, command)
    report = {"gate":"B2-storage", "status":"NOT_RUN", "source_release":SOURCE_RELEASE,
        "source_commit":SOURCE_COMMIT, "license":"AGPL-3.0", "gateway_image":CADDY,
        "official_mitigation":"GHSA-hv4r-mvr4-25vw unsigned-trailer rejection",
        "core_admission":"NOT_RUN", "user_flow":"NOT_RUN", "checks":[]}
    report["sources"] = {str(path.relative_to(REPO)).replace("\\", "/"):source_sha(path)
        for path in [HERE/"storage_smoke.py", HERE/"trial_images.py", HERE/"cleanup_receipts.py", HERE/"storage.Caddyfile",
            REPO/"apps/platform/internal/platform/storage/minio.go",
            REPO/"apps/platform/internal/platform/storage/minio_integration_test.go",
            REPO/"apps/platform/go.mod",
            REPO/".github/workflows/ci.yml"]}
    private = Path(tempfile.mkdtemp(prefix="dpp-pgi-b2-secret-"))
    suffix = uuid.uuid4().hex[:12]
    network = "dpp-pgi-b2-private-"+suffix
    engine_network = "dpp-pgi-b2-engine-"+suffix
    server = "dpp-pgi-b2-minio-"+suffix
    gateway = "dpp-pgi-b2-gateway-"+suffix
    names = []
    networks = []
    secret = secrets.token_hex(32)
    access = secrets.token_hex(12)
    try:
        binary, image_id = prepare_binary(cache, owner.tag("minio"), owner=owner)
        report["binary_sha256"] = hashlib.sha256(binary.read_bytes()).hexdigest()
        report["build_go"] = command(["go","version"]).stdout.decode().strip()
        sdk=subprocess.run(["go","list","-m","-f","{{.Version}}","github.com/minio/minio-go/v7"],
            cwd=REPO/"apps/platform",stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=60)
        require(sdk.returncode==0,"read storage client SDK identity")
        report["client_sdk"]="github.com/minio/minio-go/v7@"+sdk.stdout.decode().strip()
        report["image_id"] = image_id
        for net in [network, engine_network]:
            command(["docker","network","create",net]); networks.append(net)
        (private/"access").write_text(access,encoding="utf-8")
        (private/"secret").write_text(secret,encoding="utf-8")
        # No configured secret env/argument. Only the private storage process
        # reads this mount; Hop/the gateway do not receive root credentials.
        (private/"start.sh").write_text("#!/bin/sh\nset -eu\n"
            "export MINIO_ROOT_USER=$(cat /run/credentials/access)\n"
            "export MINIO_ROOT_PASSWORD=$(cat /run/credentials/secret)\n"
            "exec /usr/local/bin/minio server /data --address :9000\n",encoding="utf-8",newline="\n")
        command(["docker","run","-d","--name",server,"--network",network,
            "--network-alias","minio","--mount",f"type=bind,src={private},dst=/run/credentials,readonly",
            "--tmpfs","/data:rw,size=128m","--entrypoint","sh",image_id,
            "/run/credentials/start.sh"]); names.append(server)
        with socket.socket() as sock:
            sock.bind(("127.0.0.1",0)); port=sock.getsockname()[1]
        command(["docker","run","-d","--name",gateway,"--network",network,"-p",f"127.0.0.1:{port}:9000",
            "--mount",f"type=bind,src={HERE/'storage.Caddyfile'},dst=/etc/caddy/Caddyfile,readonly",CADDY]); names.append(gateway)
        command(["docker","network","connect",engine_network,gateway])
        deadline = time.monotonic()+45
        while time.monotonic()<deadline:
            try:
                with urllib.request.urlopen(f"http://127.0.0.1:{port}/minio/health/live", timeout=2) as response:
                    if response.status==200: break
            except Exception:
                state=command(["docker","inspect",server,"--format","{{.State.Status}}"]).stdout.decode().strip()
                require(state=="running","private storage process stopped")
                time.sleep(0.5)
        else:
            raise AssertionError("storage gateway unavailable")
        report["checks"].append({"name":"real storage gateway health","status":"PASS"})
        inspect = json.loads(command(["docker","inspect",server,gateway]).stdout)
        require(not inspect[0]["NetworkSettings"]["Ports"], "MinIO private port must not be published")
        require(engine_network not in inspect[0]["NetworkSettings"]["Networks"],"engine can bypass gateway")
        bypass=command(["docker","run","--rm","--network",engine_network,ALPINE,
            "sh","-c","wget -T 2 -qO- http://minio:9000/minio/health/live"],check=False)
        require(bypass.returncode!=0,"engine network reached private MinIO")
        private_ip=inspect[0]["NetworkSettings"]["Networks"][network]["IPAddress"]
        bypass=command(["docker","run","--rm","--network",engine_network,ALPINE,
            "wget","-T","2","-qO-",f"http://{private_ip}:9000/minio/health/live"],check=False)
        require(bypass.returncode!=0,"engine network reached private MinIO by IP")
        report["checks"].append({"name":"engine network has no private storage bypass","status":"PASS"})
        config=private/"client.json"
        config.write_text(json.dumps({"Endpoint":f"127.0.0.1:{port}","AccessKey":access,"SecretKey":secret}),encoding="utf-8")
        env=dict(os.environ,TEST_INGESTION_STORAGE_CONFIG=str(config))
        result=subprocess.run(["go","test","-count=1","-v","./internal/platform/storage"],
            cwd=REPO/"apps/platform",env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=120)
        # Test output contains test names only; don't persist provider diagnostics.
        require(result.returncode==0,"live MinIO isolation tests failed (raw diagnostics withheld)")
        denials=[line.split("DENIED:",1)[1].strip() for line in result.stdout.decode().splitlines() if "DENIED:" in line]
        require(len(denials)==10,"all ten storage mutation denials must execute")
        report["checks"].append({"name":"scoped STS staging and Core isolation; provider conditional race; unsigned-trailer rejection","status":"PASS", "denials":denials})
        for name in names:
            logs=command(["docker","logs",name],check=False)
            wire=logs.stdout+logs.stderr+command(["docker","inspect",name]).stdout
            require(secret.encode() not in wire and access.encode() not in wire,"credential exposed in configured container metadata/logs")
        require(secret.encode() not in result.stdout+result.stderr,"credential exposed in test output")
        report["checks"].append({"name":"private credentials absent from Docker metadata/logs and test output","status":"PASS"})
        report["status"]="PASS"
    except Exception as error:
        report["status"]="FAIL"
        report["failure"]=str(error)
        raise
    finally:
        receipt=CleanupReceipts(report,("containers","networks","temporary_credentials","image_refs"))
        try:
            for name in reversed(names):
                receipt.attempt("containers",name,lambda n=name:command(["docker","rm","-f",n],check=False))
            if not names:receipt.attempt("containers","not created",lambda:None)
            for net in reversed(networks):
                receipt.attempt("networks",net,lambda n=net:command(["docker","network","rm",n],check=False))
            if not networks:receipt.attempt("networks","not created",lambda:None)
            def remove_private():
                require(private.is_absolute() and private.parent==Path(tempfile.gettempdir()).resolve()
                        and private.name.startswith("dpp-pgi-b2-secret-"),"unsafe credential cleanup target")
                shutil.rmtree(private)
            receipt.attempt("temporary_credentials","ephemeral private directory",remove_private)
            receipt.attempt("image_refs","owned image refs",owner.cleanup)
        finally:
            receipt.finish()
            (output/"storage-report.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n",encoding="utf-8")
    require(report["cleanup"]=="PASS","storage cleanup incomplete; see component receipt")
    print("B2 storage gate PASS; Core admission and user flow NOT_RUN")


if __name__=="__main__":
    parser=argparse.ArgumentParser()
    parser.add_argument("--output",type=Path,required=True)
    parser.add_argument("--cache",type=Path,default=REPO/".data/pgi-b2-storage-cache")
    args=parser.parse_args()
    run(args.output.resolve(),args.cache.resolve())
