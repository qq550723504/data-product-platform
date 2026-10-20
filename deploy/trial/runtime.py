"""Normal Windows/Docker assembly. No acceptance runner or fault controls."""
import base64
import csv
import ctypes
from contextvars import ContextVar
from datetime import datetime, timezone
import hashlib
import io
import ipaddress
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import sys
import tarfile
import time
import urllib.request
import uuid
import xml.etree.ElementTree as ET
from contextlib import contextmanager

ROOT = Path(__file__).resolve().parents[2]
FIXED = ROOT / "examples/enterprise-activity/postgres-ingestion"
STATE = ROOT / ".data/trial"
SOURCE_PATHS = ("apps/platform", "apps/web", "migrations", "industry-packs", "deploy/trial",
                "examples/enterprise-activity/data", "examples/enterprise-activity/postgres-ingestion",
                "examples/lifecycle-d1/durable-storage.ps1")
LABEL = "dpp.trial.run"
READ_ONLY_OBSERVATION = ContextVar("trial_read_only_observation", default=False)
PG = "postgres:16-alpine@sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea"
HOP = "apache/hop:2.19.0@sha256:5f285c86523439ab3859f1b75e1fab1b4d7d958daa0349e609ef45be403a854d"
REDIS = "redis:7-alpine@sha256:858f009f9709ce576febc734aa78b8f6d624b82571f9ddb6bda4377c833b3499"
CADDY = "caddy:2.11.7-alpine@sha256:d8542f48d34a9cf4e4c11a478865229840e87e4c96ea3f439101f31a5d35f75f"
ALPINE = "alpine:3.22@sha256:5291449c3df73caf6ed85e649dec1b9e818b39a5d8c871e97afc13e9cd5e8fa8"
MINIO_COMMIT = "9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a"
MINIO_RELEASE = "RELEASE.2025-10-15T17-29-55Z"
JDBC_SHA = "73914527305a40cce504b0d3d90b23caf565136912d607ae8ae7c5895512332c"
JDBC_URL = "https://repo.maven.apache.org/maven2/org/postgresql/postgresql/42.7.14/postgresql-42.7.14.jar"
ROLES = {"source": "/var/lib/postgresql/data", "core": "/var/lib/postgresql/data",
         "objects": "/data", "redis": "/data", "hop": "/files"}


class Refused(RuntimeError):
    pass


def need(ok, message):
    if not ok:
        raise Refused(message)


def digest(data):
    return hashlib.sha256(data).hexdigest()


def read(path):
    return json.loads(Path(path).read_text(encoding="utf-8"))


def write(path, value):
    need(not READ_ONLY_OBSERVATION.get(), "Read-only observation cannot persist state or diagnostics")
    path = Path(path)
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_text(json.dumps(value, ensure_ascii=False, indent=2), encoding="utf-8")
    temporary.replace(path)


def environment():
    # Do not inherit application/provider tokens, .env, npm hooks or Compose overrides.
    keys = {"PATH", "SYSTEMROOT", "WINDIR", "COMSPEC", "TEMP", "TMP", "USERPROFILE", "LOCALAPPDATA",
            "APPDATA", "PROGRAMFILES", "PROGRAMFILES(X86)", "PROGRAMDATA", "GOPATH", "GOCACHE", "GOMODCACHE"}
    return {k: v for k, v in os.environ.items() if k.upper() in keys}


@contextmanager
def read_only_observation():
    token = READ_ONLY_OBSERVATION.set(True)
    try: yield
    finally: READ_ONLY_OBSERVATION.reset(token)


def command(args, *, data=None, cwd=ROOT, env=None, timeout=180):
    try:
        r = subprocess.run([str(a) for a in args], input=data, cwd=cwd, env=env or environment(),
                           stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=timeout,
                           creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0))
    except (OSError, subprocess.TimeoutExpired):
        raise Refused(f"{Path(str(args[0])).name} unavailable/timeout; outcome UNKNOWN, retain this run") from None
    if r.returncode:
        # Only sanitized diagnostics are persisted, including on startup failure.
        raw = r.stdout + r.stderr
        diagnostic = "diagnostic not persisted"
        if not READ_ONLY_OBSERVATION.get() and (STATE / "state.json").exists():
            s = read(STATE / "state.json")
            for name in s.get("secretReferences", []):
                f = Path(name)
                if not f.is_file(): continue
                value = f.read_bytes().strip()
                values = [value]
                try: values += [v.encode() for v in json.loads(value).values() if isinstance(v, str)]
                except (ValueError, AttributeError): pass
                for value in values:
                    if len(value) >= 8: raw = raw.replace(value, b"[REDACTED]")
            error_path = Path(s["private"]) / ("command-error-" + uuid.uuid4().hex + ".log")
            with error_path.open("xb") as log:
                log.write(raw)
            diagnostic = "sanitized private " + error_path.name
        raise Refused(f"{Path(str(args[0])).name} failed; {diagnostic}, retain this run")
    return r.stdout


def docker(*args, data=None, timeout=180):
    return command(["docker", *args], data=data, timeout=timeout)


def fixture_tables():
    result = []
    for name, count in (("enterprise", 6), ("lease", 10), ("energy", 14)):
        with (FIXED.parent / "data" / f"{name}.csv").open(encoding="utf-8-sig", newline="") as f:
            records = list(csv.reader(f, strict=True))
        need(len(records) == count + 1 and all(len(r) == len(records[0]) for r in records), "fixed fixture mismatch")
        result.append({"Code": name + "_raw", "Schema": "dpp_source", "Name": name,
                       "Columns": records[0], "Rows": count})
    return result


def sources():
    paths = command(["git", "ls-files", "-co", "--exclude-standard", "--", *SOURCE_PATHS])
    return {p: digest((ROOT / p).read_bytes().replace(b"\r\n", b"\n"))
            for p in sorted(set(paths.decode().splitlines()))
            if p != "apps/web/next-env.d.ts" and (ROOT / p).is_file()}


def local_docker_identity():
    need(os.name == "nt", "This trial supports Windows host + Docker Linux only")
    need(shutil.which("docker"), "missing prerequisite: docker")
    context = command(["docker", "context", "show"]).decode().strip()
    info = json.loads(command(["docker", "context", "inspect", context]))[0]
    endpoint = os.environ.get("DOCKER_HOST") if os.environ.get("DOCKER_HOST") and not os.environ.get("DOCKER_CONTEXT") else info["Endpoints"]["docker"]["Host"]
    need(endpoint.startswith("npipe://"), "Only a local named-pipe Docker daemon is permitted")
    # Pin the actual context for every subsequent call, including child builds.
    need(not os.environ.get("DOCKER_HOST") or endpoint == info["Endpoints"]["docker"]["Host"], "Docker host override differs from selected context")
    daemon = json.loads(docker("info", "--format", "{{json .}}"))
    need(daemon["OSType"] == "linux", "Docker Linux containers required")
    return {"context": context, "endpoint": endpoint, "daemonId": daemon["ID"]}


def preflight():
    host = local_docker_identity()
    for name in ("go", "node", "npm.cmd", "pwsh", "git"):
        need(shutil.which(name), f"missing prerequisite: {name}")
    version = docker("compose", "version", "--short").decode().strip()
    need(int(version.lstrip("v").split(".")[0]) >= 2, "Compose v2 required")
    return {**host, "compose": version,
            "go": command(["go", "version"]).decode().strip(), "node": command(["node", "--version"]).decode().strip(),
            "tables": fixture_tables()}


@contextmanager
def exclusive(directory=STATE):
    import msvcrt
    directory.mkdir(parents=True, exist_ok=True)
    with (directory / "operator.lock").open("a+b") as f:
        f.seek(0); f.write(b"0"); f.flush(); f.seek(0)
        try:
            msvcrt.locking(f.fileno(), msvcrt.LK_NBLCK, 1)
        except OSError:
            raise Refused("Another trial command owns this run; do not start a second writer") from None
        try:
            yield
        finally:
            f.seek(0); msvcrt.locking(f.fileno(), msvcrt.LK_UNLCK, 1)


def free_ports():
    sockets = []
    try:
        for _ in range(7):
            s = socket.socket(); s.bind(("127.0.0.1", 0)); sockets.append(s)
        return dict(zip(("source", "core", "storage", "redis", "hop", "api", "web"),
                        (s.getsockname()[1] for s in sockets)))
    finally:
        for s in sockets: s.close()


def subnets():
    ids = docker("network", "ls", "-q").decode().split()
    used = [ipaddress.ip_network(c["Subnet"]) for n in json.loads(docker("network", "inspect", *ids))
            for c in (n.get("IPAM", {}).get("Config") or []) if c.get("Subnet")]
    routes = command(["pwsh", "-NoProfile", "-Command", "Get-NetRoute -AddressFamily IPv4 | Select-Object -ExpandProperty DestinationPrefix"]).decode("utf-8-sig").split()
    used += [ipaddress.ip_network(r) for r in routes if r != "0.0.0.0/0"]
    candidates = [n for n in ipaddress.ip_network("10.249.0.0/16").subnets(new_prefix=28)
                  if not any(n.overlaps(u) for u in used)]
    need(len(candidates) >= 2, "No isolated trial subnet available; existing networks are retained")
    return [str(n) for n in candidates[:2]]


def private_directory(run_id):
    path = Path(os.environ["LOCALAPPDATA"]) / "Codex/trials" / ("dpp-trial-" + run_id)
    path.mkdir(parents=True, exist_ok=False)
    sid = list(csv.reader(io.StringIO(command(["whoami", "/user", "/fo", "csv", "/nh"]).decode())))[0][1]
    command(["icacls", path, "/inheritance:r", "/grant:r", "*" + sid + ":(OI)(CI)F", "*S-1-5-18:(OI)(CI)F"])
    return path


def build_images(s):
    private, run_id = Path(s["private"]), s["runId"]
    build = private / "build"; build.mkdir()
    for image in (PG, HOP, REDIS, CADDY, ALPINE):
        try: docker("image", "inspect", image)
        except Refused: docker("pull", image, timeout=600)
    with urllib.request.urlopen(JDBC_URL, timeout=60) as r: driver = r.read()
    need(digest(driver) == JDBC_SHA, "Pinned JDBC checksum mismatch")
    (build / "postgresql-42.7.14.jar").write_bytes(driver)
    hop = "dpp-trial-" + run_id + ":hop"
    docker("build", "--label", LABEL + "=" + run_id, "-t", hop, "-f", FIXED / "Dockerfile.hop", build, timeout=600)
    src = build / "minio-source"
    command(["git", "clone", "--depth", "1", "--branch", MINIO_RELEASE, "https://github.com/minio/minio.git", src], timeout=300)
    need(command(["git", "rev-parse", "HEAD"], cwd=src).decode().strip() == MINIO_COMMIT, "Pinned MinIO source mismatch")
    command(["go", "build", "-trimpath", "-o", build / "minio", "-ldflags",
             f"-s -w -X github.com/minio/minio/cmd.Version={MINIO_RELEASE} -X github.com/minio/minio/cmd.CommitID={MINIO_COMMIT}", "."],
            cwd=src, env=dict(environment(), GOOS="linux", GOARCH="amd64", CGO_ENABLED="0", GOTOOLCHAIN="local"), timeout=1200)
    shutil.copyfile(src / "LICENSE", build / "LICENSE")
    (build / ".dockerignore").write_text("*\n!minio\n!LICENSE\n!Dockerfile\n", encoding="utf-8")
    (build / "Dockerfile").write_text(f"FROM {ALPINE}\nCOPY minio /usr/local/bin/minio\nCOPY LICENSE /licenses/minio/LICENSE\n", encoding="utf-8")
    minio = "dpp-trial-" + run_id + ":minio"
    docker("build", "--label", LABEL + "=" + run_id, "-t", minio, build, timeout=300)
    s["images"] = {role: {"ref": image, "id": json.loads(docker("image", "inspect", image))[0]["Id"]}
                   for role, image in {"source": PG, "core": PG, "hop": hop, "redis": REDIS, "objects": minio, "gateway": CADDY}.items()}
    s["minio"] = {"source": MINIO_COMMIT, "release": MINIO_RELEASE, "binarySha256": digest((build / "minio").read_bytes()), "license": "AGPL-3.0"}


def compose_config(s):
    p, ports, run_id = Path(s["private"]), s["ports"], s["runId"]
    labels = {LABEL: run_id}
    mount = lambda source, target, kind="bind", ro=True: {"type": kind, "source": str(source), "target": target, "read_only": ro}
    services, volumes = {}, {}
    for role in ("source", "core", "objects", "redis", "hop", "gateway"):
        item = {"container_name": s["containers"][role]["name"], "image": s["images"][role]["id"], "labels": labels,
                "networks": ["engine" if role in ("source", "hop") else "storage"], "volumes": []}
        if role in ROLES:
            volume = s["volumes"][role]
            volumes[role] = {"name": volume, "labels": labels}
            item["volumes"].append(mount(role, ROLES[role], "volume", False))
        if role in ("source", "core"):
            item["environment"] = {"POSTGRES_DB": "dpp_ingestion_source" if role == "source" else "dpp_trial_core", "POSTGRES_PASSWORD_FILE": "/run/credentials/password"}
            item["volumes"].append(mount(p / (role + "-password"), "/run/credentials/password"))
            item["ports"] = [f"127.0.0.1:{ports[role]}:5432"]
        elif role == "objects":
            item["entrypoint"] = ["sh", "/run/credentials/start.sh"]
            item["volumes"].append(mount(p / "storage", "/run/credentials"))
            item["networks"] = {"storage": {"aliases": ["minio"]}}
        elif role == "redis":
            item["command"] = ["redis-server", "--appendonly", "yes", "--appendfsync", "always"]
            item["ports"] = [f"127.0.0.1:{ports['redis']}:6379"]
        elif role == "hop":
            item["entrypoint"] = ["sh", "/start-hop.sh"]
            item["volumes"] += [mount(p / "hop", "/run/secrets"), mount(FIXED / "metadata", "/metadata"),
                                mount(FIXED / "start-hop.sh", "/start-hop.sh"), mount(p / "staging", "/staging", ro=False)]
            volumes["hop-tmp"] = {"name": s["volumes"]["hop-tmp"], "labels": labels}
            item["volumes"].append(mount("hop-tmp", "/tmp", "volume", False))
            item["ports"] = [f"127.0.0.1:{ports['hop']}:8080"]
        else:
            item["volumes"].append(mount(FIXED / "storage.Caddyfile", "/etc/caddy/Caddyfile"))
            item["ports"] = [f"127.0.0.1:{ports['storage']}:9000"]
        services[role] = item
    return {"name": s["project"], "services": services, "volumes": volumes,
            "networks": {name: {"name": s["networks"][name], "labels": labels,
                                "ipam": {"config": [{"subnet": subnet}]}}
                         for name, subnet in zip(("engine", "storage"), s["subnets"])}}


def compose(s, *args):
    return docker("compose", "--project-name", s["project"], "--project-directory", ROOT,
                  "--env-file", Path(s["private"]) / "empty.env", "-f", Path(s["private"]) / "compose.json", *args, timeout=300)


def provision_config(s):
    p, ports = Path(s["private"]), s["ports"]
    for folder in ("hop", "storage", "staging", "bin"): (p / folder).mkdir()
    for name in ("source-password", "core-password", "reader-password", "hop-password", "token", "storage/access", "storage/secret"):
        (p / name).write_text(secrets.token_hex(24), encoding="utf-8")
    shutil.copyfile(p / "reader-password", p / "hop/source-password")
    server = ET.Element("hop-server-config"); hs = ET.SubElement(server, "hop-server")
    for k, v in (("name", "trial"), ("hostname", "0.0.0.0"), ("port", "8080"), ("username", "trial"), ("password", (p / "hop-password").read_text())):
        ET.SubElement(hs, k).text = v
    ET.SubElement(server, "metadata_folder").text = "/metadata"
    ET.ElementTree(server).write(p / "hop/hop-server.xml", encoding="UTF-8")
    (p / "storage/start.sh").write_text("#!/bin/sh\nset -eu\nexport MINIO_ROOT_USER=$(cat /run/credentials/access)\nexport MINIO_ROOT_PASSWORD=$(cat /run/credentials/secret)\nexec /usr/local/bin/minio server /data --address :9000\n", encoding="utf-8", newline="\n")
    (p / "empty.env").write_text("", encoding="utf-8")
    metadata = {"rdbms": [read(FIXED / "metadata/rdbms/dpp-source.json")], "pipeline-run-configuration": [read(FIXED / "metadata/pipeline-run-configuration/local.json")]}
    pg = metadata["rdbms"][0]["rdbms"]["POSTGRESQL"]
    pg["hostname"], pg["port"] = "host.docker.internal", str(ports["source"])
    write(p / "metadata.json", metadata)
    profile = {"Enabled": True, "TokenFile": str(p / "token"), "Subject": "trial-operator-" + s["runId"],
               "ConnectionRef": "dpp-source-reader", "InstanceRef": s["project"], "Database": "dpp_ingestion_source", "Reader": "dpp_reader",
               "SourceDSN": f"postgres://dpp_reader@host.docker.internal:{ports['source']}/dpp_ingestion_source?sslmode=disable",
               "SourcePasswordFile": str(p / "reader-password"), "TemplateFile": str(FIXED / "extract.hpl"), "MetadataFile": str(p / "metadata.json"),
               "Tables": fixture_tables(), "HopURL": f"http://127.0.0.1:{ports['hop']}", "HopUser": "trial", "HopPasswordFile": str(p / "hop-password"),
               "LocalRoot": str(p / "staging"), "EngineRoot": "/staging", "Bucket": "raw", "StorageEndpoint": f"127.0.0.1:{ports['storage']}",
               "CoreCredentialFile": str(p / "core-identity.json"), "PublisherCredentialFile": str(p / "publisher-identity.json")}
    cfg = {"DSN": f"postgres://postgres:{(p / 'core-password').read_text()}@127.0.0.1:{ports['core']}/dpp_trial_core?sslmode=disable",
           "Private": str(p), "Repo": str(ROOT), "RootKey": (p / "storage/access").read_text(), "RootSecret": (p / "storage/secret").read_text(),
           "Workspace": s["workspaceId"], "Actor": s["actorId"], "Profile": profile}
    write(p / "config.json", cfg)
    s["secretReferences"] = [str(p / name) for name in ("source-password", "core-password", "reader-password", "hop-password", "token", "storage/access", "storage/secret", "core-identity.json", "publisher-identity.json")]


def save(s):
    write(STATE / "state.json", s)


def storage_check(s, allowed):
    items = []
    for role, target in ROLES.items():
        name = s["containers"][role]["name"]
        item = json.loads(docker("inspect", name))[0]
        items.append({"role": role, "name": name, "path": target, "volume": s["volumes"][role], "container": item})
    payload = {"run_id": s["runId"], "volumes": [s["volumes"][role] for role in ROLES], "services": items}
    args = ["pwsh", "-NoProfile", "-File", ROOT / "deploy/trial/storage-check.ps1", "-AllowedStates", ",".join(allowed)]
    if READ_ONLY_OBSERVATION.get():
        # Docker metadata can contain secrets. Keep it off argv and disk,
        # including temporary files; reuse the same retained-storage validator.
        result = command([*args, "-InputFromStdin"], data=json.dumps(payload, ensure_ascii=False).encode("utf-8"))
    else:
        path = Path(s["private"]) / "storage-inspect.json"
        write(path, payload)
        result = command([*args, "-InputPath", path])
    return json.loads(result.decode("utf-8-sig"))


def wait_for(probe, label, seconds=90):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        try:
            if probe(): return
        except (OSError, Refused): pass
        time.sleep(0.5)
    raise Refused(label + " readiness UNKNOWN; retain run and diagnose")


def http(url, headers=None):
    with urllib.request.urlopen(urllib.request.Request(url, headers=headers or {}), timeout=3) as r:
        return r.status == 200


def seed_source(s):
    password = (Path(s["private"]) / "reader-password").read_text()
    sql = f"CREATE ROLE dpp_reader LOGIN NOINHERIT PASSWORD '{password}';\n" + (FIXED / "source.sql").read_text(encoding="utf-8")
    for table in fixture_tables():
        with (FIXED.parent / "data" / (table["Name"] + ".csv")).open(encoding="utf-8-sig", newline="") as f: records = list(csv.reader(f))[1:]
        for row in records:
            values = ",".join("'" + v.replace("'", "''") + "'" for v in row)
            sql += f"\nINSERT INTO dpp_source.{table['Name']} ({','.join(table['Columns'])}) VALUES ({values});"
    docker("exec", "-i", s["containers"]["source"]["name"], "psql", "-Xq", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "dpp_ingestion_source", data=sql.encode())


def process_handle(pid, terminate=False, allow_missing=False):
    from ctypes import wintypes as w
    kernel = ctypes.WinDLL("kernel32", use_last_error=True)
    kernel.OpenProcess.restype = w.HANDLE
    kernel.OpenProcess.argtypes = [w.DWORD, w.BOOL, w.DWORD]
    handle = kernel.OpenProcess(0x1000 | (1 if terminate else 0) | (0x100000 if allow_missing else 0), False, pid)
    if not handle and allow_missing and ctypes.get_last_error() == 87: return kernel, None
    need(handle, f"Recorded process {pid} unavailable; identity UNKNOWN")
    kernel.CloseHandle.argtypes = [w.HANDLE]
    return kernel, handle


def process_identity(pid, terminate=False, expected=None, allow_missing=False):
    from ctypes import wintypes as w
    k, h = process_handle(pid, terminate, allow_missing)
    if not h: return None
    try:
        if allow_missing:
            # An exited process object may remain open while another observer
            # holds a handle. Its image name is then unavailable; the native
            # signaled state proves exit without trusting a reused live PID.
            k.WaitForSingleObject.argtypes = [w.HANDLE, w.DWORD]
            k.WaitForSingleObject.restype = w.DWORD
            state = k.WaitForSingleObject(h, 0)
            need(state in (0, 258), "Cannot verify process exit")
            if state == 0: return None
        k.GetProcessTimes.argtypes = [w.HANDLE, ctypes.POINTER(w.FILETIME), ctypes.POINTER(w.FILETIME), ctypes.POINTER(w.FILETIME), ctypes.POINTER(w.FILETIME)]
        times = [w.FILETIME() for _ in range(4)]
        need(k.GetProcessTimes(h, *[ctypes.byref(t) for t in times]), "Cannot verify process creation time")
        size, buf = w.DWORD(32768), ctypes.create_unicode_buffer(32768)
        k.QueryFullProcessImageNameW.argtypes = [w.HANDLE, w.DWORD, w.LPWSTR, ctypes.POINTER(w.DWORD)]
        need(k.QueryFullProcessImageNameW(h, 0, buf, ctypes.byref(size)), "Cannot verify process executable")
        value = {"pid": pid, "created": (times[0].dwHighDateTime << 32) | times[0].dwLowDateTime, "exe": str(Path(buf.value).resolve())}
        need(not expected or value == expected, f"PID {pid} identity drift; no process stopped")
        if terminate:
            k.TerminateProcess.argtypes = [w.HANDLE, w.UINT]
            need(k.TerminateProcess(h, 0), "Exact process stop failed")
        return value
    finally:
        k.CloseHandle(h)


def spawn(s, role, args, env, *, log_directory=None):
    log_path = Path(log_directory or s["private"]) / (role + ".log")
    with log_path.open("xb") as log:
        p = subprocess.Popen([str(a) for a in args], cwd=ROOT / "apps" / ("web" if role == "web" else "platform"),
                             env=env, stdin=subprocess.DEVNULL, stdout=log, stderr=log,
                             creationflags=subprocess.CREATE_NO_WINDOW | subprocess.DETACHED_PROCESS)
    s["processes"][role] = process_identity(p.pid)
    s.setdefault("processLogs", {})[role] = str(log_path)
    save(s)


def inspect_facts(s):
    return json.loads(command([Path(s["private"]) / "bin/trial-prepare.exe", "inspect"],
                              env=dict(environment(), DPP_TRIAL_CONFIG=str(Path(s["private"]) / "config.json"))))


def verify_container(s, role, item, *, configuration=True):
    expected = s["containers"][role]
    need(item["Name"].lstrip("/") == expected["name"] and item["Config"]["Labels"].get(LABEL) == s["runId"]
         and item["Image"] == s["images"][role]["id"] and (not expected.get("id") or item["Id"] == expected["id"]),
         f"{role} identity drift; no mutation allowed")
    # Exact owned containers can always be stopped for containment. Do not
    # require their configuration to remain healthy in order to stop them.
    if not configuration: return
    for bindings in (item["HostConfig"].get("PortBindings") or {}).values():
        need(all(b["HostIp"] == "127.0.0.1" for b in bindings), "Non-loopback publication refused")
    if expected.get("configHash"):
        need(container_hash(item) == expected["configHash"], role + " mounts/network/configuration drift")


def container_hash(item):
    config = {k:item["Config"].get(k) for k in ("Image","Entrypoint","Cmd","Env","User","WorkingDir","Labels")}
    host = {k:item["HostConfig"].get(k) for k in ("PortBindings","Privileged","ReadonlyRootfs","CapAdd","CapDrop","SecurityOpt","ExtraHosts","PidMode")}
    mounts = [{k:m.get(k) for k in ("Type","Destination","Name","RW","Source")} for m in item["Mounts"]]
    return digest(json.dumps({"Config":config,"HostConfig":host,"Mounts":sorted(mounts,key=lambda m:m["Destination"]),
                              "Networks": sorted(item["NetworkSettings"]["Networks"])}, sort_keys=True).encode())


def verify(s, *, source=True, running=True):
    need(s["root"] == str(ROOT) and s["contract"] == "issue325/stage1", "Foreign trial registry")
    need(preflight()["daemonId"] == s["host"]["daemonId"], "Docker daemon drift")
    if source:
        if "controllerBinding" in s:
            from resume import verify_binding
            verify_binding(s)
        else:
            need(sources() == s["sources"], "Source drift; retain original run identity and diagnose")
    need(digest((Path(s["private"]) / "compose.json").read_bytes()) == s["composeSha256"], "Compose configuration drift")
    for name, expected in s.get("privateHashes", {}).items():
        need(digest(Path(name).read_bytes()) == expected, "Private profile/credential drift; retain this run")
    for role, value in s["binaries"].items():
        need(digest(Path(value["path"]).read_bytes()) == value["sha256"], role + " binary drift")
    if "businessBinding" in s:
        from upgrade import verify_business
        verify_business(s)
    if "activationBinding" in s:
        from activation import verify_binding
        verify_binding(s)
    for role, expected in s["containers"].items():
        item = json.loads(docker("inspect", expected["name"]))[0]
        verify_container(s, role, item)
        if running: need(item["State"]["Status"] == "running", role + " not running; outcome UNKNOWN")
    for role, expected in s["processes"].items():
        if running: process_identity(expected["pid"], expected=expected)
    if s.get("webBuild"):
        for name, expected in s["webBuild"]["files"].items():
            need(digest((ROOT / "apps/web/.next" / name).read_bytes()) == expected, "Production web build drift")
    for name, expected in s.get("resourceIds", {}).items():
        kind, target = name.split(":", 1)
        item = json.loads(docker(kind, "inspect", target))[0]
        need(item.get("Labels", {}).get(LABEL) == s["runId"] and (item.get("Id") or item.get("CreatedAt")) == expected,
             "Volume/network identity drift")
    storage_check(s, ["running"] if running else ["exited", "running", "created"])


def active_business(s):
    if "activationBinding" in s:
        from activation import verify_binding
        return verify_binding(s)
    from upgrade import verify_business
    return verify_business(s)


def core_environment(s):
    p, cfg = Path(s["private"]), read(Path(s["private"]) / "config.json")
    env = dict(environment(), APP_ENV="synthetic-trial", APP_HTTP_ADDR=f"127.0.0.1:{s['ports']['api']}", POSTGRES_DSN=cfg["DSN"],
                REDIS_ADDR=f"127.0.0.1:{s['ports']['redis']}", REDIS_DB="0", OBJECT_STORAGE_ENDPOINT=cfg["Profile"]["StorageEndpoint"],
                OBJECT_STORAGE_BUCKET="raw", OBJECT_STORAGE_USE_SSL="false",
                INDUSTRY_PACK_ROOT=str(ROOT / "industry-packs"), INGESTION_PROFILE=s.get("activeProfile", str(p / "profile.json")),
                OPENMETADATA_ENABLED="false", HOP_ENABLED="false", SPLINK_ENABLED="false", GX_ENABLED="false", LABEL_STUDIO_ENABLED="false",
                DELIVERY_API_ENABLED="false", HUMAN_DECISION_API_ENABLED="false")
    if s.get("workspaceObjectProfile"):
        env["OBJECT_STORAGE_WORKSPACE_PROFILE"] = s["workspaceObjectProfile"]
    else:
        # The real retained pre-330 business artifact still owns this binding.
        # New binaries require an explicit protected profile and cannot fall back.
        env.update(OBJECT_STORAGE_ACCESS_KEY=cfg["RootKey"], OBJECT_STORAGE_SECRET_KEY=cfg["RootSecret"])
    if "businessBinding" in s:
        manifest = active_business(s)
        env.update(HUMAN_DECISION_API_ENABLED="true", HUMAN_DECISION_API_TOKEN=Path(manifest["humanTokenFile"]).read_text().strip(),
                   HUMAN_DECISION_API_SUBJECT="trial-operator-" + s["runId"], HUMAN_DECISION_API_ACTOR_ID=s["actorId"],
                   HUMAN_DECISION_API_WORKSPACE_IDS=s["workspaceId"], RIGHTS_API_TOKEN=(p / "token").read_text().strip(),
                   RIGHTS_API_ACTOR_ID=s["actorId"], RIGHTS_API_WORKSPACE_IDS=s["workspaceId"])
    return env


def web_environment(s):
    p = Path(s["private"])
    env = dict(environment(), NODE_ENV="production", PLATFORM_API_BASE_URL=s["api"], POC_WORKSPACE_ID=s["workspaceId"],
                POC_ENABLE_INGESTION_ACTIONS="true", POC_INGESTION_TOKEN_FILE=str(p / "token"), POC_INGEST_ACTOR_ID=s["actorId"],
                POC_TRIAL_RAW_ONLY="true")
    if "businessBinding" in s:
        manifest = active_business(s)
        env.update(POC_TRIAL_RAW_ONLY="false", POC_ENABLE_REVIEW_ACTIONS="true", POC_RIGHTS_TOKEN_FILE=str(p / "token"),
                   HUMAN_DECISION_API_TOKEN=Path(manifest["humanTokenFile"]).read_text().strip(),
                   POC_INGESTION_WORKFLOW_ID=manifest["business"]["workflowVersionId"])
    return env


def start():
    if (STATE / "state.json").exists():
        s = read(STATE / "state.json")
        need(s["phase"] == "RUNNING", f"Existing run {s['runId']} is {s['phase']}; no reseed/replacement. Use explicit resume after stop")
        return status(s)
    host = preflight(); run_id = uuid.uuid4().hex
    p = private_directory(run_id); project = "dpp-trial-" + run_id
    s = {"contract": "issue325/stage1", "phase": "STARTING", "runId": run_id, "root": str(ROOT), "private": str(p),
         "project": project, "host": host, "head": command(["git", "rev-parse", "HEAD"]).decode().strip(), "sources": sources(),
         "workspaceId": str(uuid.uuid4()), "actorId": str(uuid.uuid4()), "ports": free_ports(), "subnets": subnets(),
         "containers": {r: {"name": project + "-" + r} for r in ("source", "core", "objects", "redis", "hop", "gateway")},
         "volumes": {r: project + "-" + r for r in (*ROLES, "hop-tmp")},
         "networks": {r: project + "-" + r for r in ("engine", "storage")}, "processes": {}, "binaries": {}}
    save(s)  # No external creation before a stable identity exists.
    try:
        print(json.dumps({"phase": "BUILDING", "runId": run_id}), flush=True)
        provision_config(s); save(s); build_images(s); save(s)
        compose_path = p / "compose.json"; write(compose_path, compose_config(s))
        s["composeSha256"] = digest(compose_path.read_bytes()); save(s)
        compose(s, "create", "--no-build", "--pull", "never")
        storage_check(s, ["created"])
        for role, expected in s["containers"].items():
            item = json.loads(docker("inspect", expected["name"]))[0]; verify_container(s, role, item); expected["id"] = item["Id"]
            expected["configHash"] = container_hash(item)
        s["resourceIds"] = {}
        for kind, names in (("volume", s["volumes"].values()), ("network", s["networks"].values())):
            for name in names:
                item = json.loads(docker(kind, "inspect", name))[0]
                s["resourceIds"][kind + ":" + name] = item.get("Id") or item["CreatedAt"]
        save(s)
        compose(s, "start")
        for role in ("source", "core"):
            wait_for(lambda r=role: docker("exec", s["containers"][r]["name"], "pg_isready", "-U", "postgres"), role)
        wait_for(lambda: http(f"http://127.0.0.1:{s['ports']['storage']}/minio/health/live"), "storage")
        auth = "Basic " + base64.b64encode(("trial:" + (p / "hop-password").read_text()).encode()).decode()
        wait_for(lambda: http(f"http://127.0.0.1:{s['ports']['hop']}/hop/status?xml=Y", {"Authorization": auth}), "Hop")
        storage_check(s, ["running"])
        seed_source(s)
        print(json.dumps({"phase": "BUILDING_CORE", "runId": run_id}), flush=True)
        for role in ("api", "worker", "trial-prepare"):
            binary = p / "bin" / (role + ".exe")
            command(["go", "build", "-trimpath", "-o", binary, "./cmd/" + role], cwd=ROOT / "apps/platform", timeout=600)
            s["binaries"][role] = {"path": str(binary), "sha256": digest(binary.read_bytes())}; save(s)
        command([p / "bin/trial-prepare.exe", "prepare"], env=dict(environment(), DPP_TRIAL_CONFIG=str(p / "config.json")))
        s["workspaceObjectProfile"] = str(p / "workspace-object-profile.json")
        s["privateHashes"] = {str(f): digest(f.read_bytes()) for f in [p / "config.json", p / "profile.json", p / "metadata.json", Path(s["workspaceObjectProfile"]), *[Path(n) for n in s["secretReferences"]]]}
        save(s)
        baseline = inspect_facts(s)
        need(baseline["counts"]["dataset_version"] == baseline["counts"]["ingestion_operation"] == 0, "First start must not create business versions/batches")
        write(STATE / "initial-facts.json", baseline)
        core_env = core_environment(s)
        for role in ("api", "worker"): spawn(s, role, [p / "bin" / (role + ".exe")], core_env)
        s["api"] = f"http://127.0.0.1:{s['ports']['api']}"
        wait_for(lambda: http(s["api"] + "/health/ready"), "API")
        web_env = web_environment(s)
        print(json.dumps({"phase": "BUILDING_PRODUCTION_WEB", "runId": run_id}), flush=True)
        command(["cmd.exe", "/d", "/c", "npm.cmd install --no-audit --no-fund --package-lock=false"], cwd=ROOT / "apps/web", timeout=600)
        command(["cmd.exe", "/d", "/c", "npm.cmd run build"], cwd=ROOT / "apps/web", env=web_env, timeout=900)
        manifest = ROOT / "apps/web/.next/BUILD_ID"
        standalone = manifest.parent / "standalone"
        shutil.copytree(manifest.parent / "static", standalone / ".next/static")
        shutil.copytree(ROOT / "apps/web/public", standalone / "public")
        s["webBuild"] = {"buildId": manifest.read_text().strip(), "files": {str(f.relative_to(manifest.parent)): digest(f.read_bytes()) for f in manifest.parent.rglob("*") if f.is_file() and "cache" not in f.relative_to(manifest.parent).parts}}
        node = Path(shutil.which("node"))
        s["binaries"]["node"] = {"path": str(node), "sha256": digest(node.read_bytes())}
        s["web"] = f"http://127.0.0.1:{s['ports']['web']}"
        spawn(s, "web", [node, standalone / "server.js"], dict(web_env, HOSTNAME="127.0.0.1", PORT=str(s["ports"]["web"])))
        wait_for(lambda: http(s["web"] + "/batches"), "Web", seconds=120)
        s["phase"] = "RUNNING"; save(s)
        return status(s)
    except BaseException:
        s["phase"] = "UNKNOWN"; save(s)
        print(json.dumps({"phase": "UNKNOWN", "runId": run_id, "registry": str(STATE / "state.json"), "retained": True}), flush=True)
        raise


def status(s):
    need(s["phase"] in ("RUNNING", "STOPPED"), f"Run {s['runId']} is {s['phase']}; no reseed/replacement, inspect retained registry")
    verify(s, running=s["phase"] == "RUNNING")
    result = {"runId": s["runId"], "phase": s["phase"], "workspaceId": s["workspaceId"], "sourceHead": s["head"],
              "controllerHead": s.get("controllerBinding", {}).get("head", s["head"]),
              "web": s.get("web"), "registry": str(STATE / "state.json"), "OpenMetadata": "NOT_ENABLED",
              "PR2": {"restartResume": s.get("resume", {}).get("status", "NOT_RUN"), "businessDelivery": "NOT_RUN"}, "humanExplanation": "deferred / NOT_RUN"}
    if "businessBinding" in s:
        m = active_business(s)
        result["businessHead"] = m["head"]
        result["PR2"]["businessUpgrade"] = s.get("upgrade", {}).get("status", "UNKNOWN")
        result["workflowVersionId"] = m["business"]["workflowVersionId"]
    if "activation" in s:
        result["PR2"]["protectedActivation"] = s["activation"]["status"]
    if s["phase"] == "RUNNING":
        expiry = s.get("resume", {}).get("credentialExpiresAt")
        if not expiry and s.get("workspaceObjectProfile"):
            expiry = read(s["workspaceObjectProfile"])["Scope"]["ExpiresAt"]
        if expiry:
            need(datetime.fromisoformat(expiry) > datetime.now(timezone.utc), "Core session expired; use explicit restart for same-scope renewal")
        need(http(s["api"] + "/health/ready") and http(s["web"] + "/batches"), "Runtime health UNKNOWN")
        facts = inspect_facts(s); write(STATE / "latest-facts.json", facts)
        result["facts"] = facts
    write(STATE / "status.json", result)
    return result


def stop(s):
    # Source edits cannot remove the ability to safely stop exact recorded owners.
    need(s["root"] == str(ROOT), "Foreign trial registry")
    need(local_docker_identity()["daemonId"] == s["host"]["daemonId"], "Docker daemon drift")
    if s["phase"] == "STOPPED": return {"runId": s["runId"], "phase": "STOPPED", "retained": True}
    owned = []
    for role, expected in s["containers"].items():
        # If Compose never created a resource, it cannot be a stop target.
        found = docker("ps", "-aq", "--filter", "name=^/" + expected["name"] + "$").decode().strip()
        if not found: continue
        item = json.loads(docker("inspect", found))[0]; verify_container(s, role, item, configuration=False); owned.append(item["Id"])
    for expected in s["processes"].values(): process_identity(expected["pid"], expected=expected, allow_missing=True)
    if s["phase"] == "RUNNING":
        try:
            facts = inspect_facts(s)
        except (Refused, OSError, ValueError):
            # Business health is not an ownership condition for containment.
            # Keep prior successful receipts and never turn unreadable facts into zero.
            observation = {"runId": s["runId"], "status": "UNKNOWN", "reason": "Business facts unavailable; retained data requires inspection"}
        else:
            write(STATE / "before-stop-facts.json", facts)
            observation = {"runId": s["runId"], "status": "PASS", "receipt": str(STATE / "before-stop-facts.json")}
        write(STATE / "before-stop-observation.json", observation)
        s["stopInspection"] = observation
    s["phase"] = "STOPPING"; save(s)
    for expected in reversed(list(s["processes"].values())): process_identity(expected["pid"], terminate=True, expected=expected, allow_missing=True)
    if owned: docker("stop", "--time", "30", *owned, timeout=240)
    s["phase"] = "STOPPED"; save(s)
    return {"runId": s["runId"], "phase": s["phase"], "retained": True, "volumes": s["volumes"], "objectsAndEvidence": "retained; no deletion performed"}
