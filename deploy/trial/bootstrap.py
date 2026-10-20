"""One-attempt retained ledger bootstrap and bounded custody; no ordinary resume.

Plans only inspect metadata. Each executable stage is a separate operator command.
The existing migration runner, pg_dump, custody SDK/ledger and private ACLs own
their respective capabilities; this module only binds the retained composition.
"""
import copy
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import re
import socket
import urllib.request
import urllib.parse
import uuid

import runtime as r
import resume as c
import upgrade as u

BASELINE = "6e5c973174404b8f3ea47d8c25b3327d809acaa3"
CONTRACT = "issue325/retained-ledger-v1"
STAGES = ("bootstrap", "custody")


def encoded(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode()


def file(path, *, below=None):
    from activation import fingerprint
    return fingerprint(path, below=below)


def action(receipts, name, operation):
    put(receipts / (name + "-started.json"), {"action": name})
    result = operation()
    put(receipts / (name + "-result.json"), {"status": "PASS"})
    return result


def put(path, value):
    """Exclusive durable journal; a partial file is retained as an incomplete attempt."""
    r.need(not r.READ_ONLY_OBSERVATION.get(), "Read-only plan cannot persist a journal")
    with Path(path).open("xb") as output:
        output.write(encoded(value)); output.flush(); os.fsync(output.fileno())
    if os.name != "nt":
        fd = os.open(Path(path).parent, os.O_RDONLY)
        try: os.fsync(fd)
        finally: os.close(fd)


def github(path):
    # Public repository metadata only; never a retained storage/provider request.
    try:
        request = urllib.request.Request("https://api.github.com/repos/qq550723504/data-product-platform/" + path,
                                         headers={"Accept": "application/vnd.github+json"})
        with urllib.request.urlopen(request, timeout=30) as response:
            data = response.read((2 << 20) + 1)
        r.need(len(data) <= 2 << 20, "Bounded source admission required")
        return json.loads(data)
    except Exception:
        raise r.Refused("Reviewed merged source/main CI admission unavailable") from None


def target(s, admission):
    head = r.command(["git", "rev-parse", "HEAD"]).decode().strip()
    current = r.sources()
    r.need(c.committed_sources(head) == current, "Bootstrap sources must be committed")
    base = c.committed_sources(BASELINE)
    delta = sorted(p for p in set(base) | set(current) if base.get(p) != current.get(p))
    r.need(all(c.controller_only(p) for p in delta), "Changes outside reviewed #335 controller baseline")
    r.need(admission.get("head") == head and type(admission.get("ciRunId")) is int and admission["ciRunId"] > 0,
           "Exact merged source and main CI run required")
    commit = github("commits/" + head)
    main = github("branches/main")
    r.need(commit.get("sha") == head, "Reviewed controller commit unavailable")
    comparison = github("compare/" + head + "..." + main["commit"]["sha"])
    r.need(comparison.get("merge_base_commit", {}).get("sha") == head and comparison.get("status") in ("ahead", "identical"),
           "Reviewed controller must be merged into main")
    run = github("actions/runs/" + str(admission["ciRunId"]))
    r.need(run.get("head_sha") == head and run.get("event") == "push" and run.get("head_branch") == "main"
           and run.get("status") == "completed" and run.get("conclusion") == "success"
           and run.get("path") == ".github/workflows/ci.yml", "Exact merge-SHA push/main CI must succeed")
    jobs = github("actions/runs/" + str(admission["ciRunId"]) + "/jobs?per_page=100")
    r.need(jobs.get("total_count") == len(jobs.get("jobs", [])) and any(j.get("name") == "required" and j.get("conclusion") == "success" for j in jobs["jobs"])
           and all(j.get("status") == "completed" and j.get("conclusion") in ("success", "skipped") for j in jobs["jobs"]), "Complete required main CI proof needed")
    r.need(r.command(["git", "show", BASELINE + ":.github/workflows/ci.yml"]).replace(b"\r\n", b"\n") ==
           r.command(["git", "show", head + ":.github/workflows/ci.yml"]).replace(b"\r\n", b"\n"), "Reviewed main CI contract changed")
    workflow = (r.ROOT / u.WORKFLOW).read_bytes().replace(b"\r\n", b"\n")
    for old in (BASELINE, u.verify_business(s)["head"]):
        r.need(r.command(["git", "show", old + ":" + u.WORKFLOW]).replace(b"\r\n", b"\n") == workflow,
               "Frozen workflow source drift")
    return {"head": head, "sources": current, "deltaPaths": delta, "workflowSha256": r.digest(workflow),
            "admission": copy.deepcopy(admission)}


def migrations():
    result = {}
    for path in sorted((r.ROOT / "migrations").glob("*.sql")):
        match = re.fullmatch(r"([0-9]{6})_.+\.(up|down)\.sql", path.name)
        r.need(match is not None, "Unexpected migration filename")
        if int(match[1]) <= 71: result[path.name] = r.digest(path.read_bytes())
    up = [int(n[:6]) for n in result if n.endswith(".up.sql")]
    # Repository history intentionally has no 50/54/56. Never synthesize them.
    r.need(up and up[0] == 1 and up[-3:] == [69, 70, 71] and len(up) == len(set(up)), "Exact complete migration manifest through 71 required")
    return result


def guard(s):
    """Also covers journal allocation before registry update; never auto-adopt."""
    for stage in STAGES:
        attempt = s.get(stage, {})
        binding = s.get(stage + "Binding")
        for root in (r.STATE / (stage + "-attempts"), Path(s.get("private", r.STATE)) / (stage + "-attempts")):
            if root.exists():
                r.need(all(p.name == attempt.get("attemptId") and attempt.get("status") == "PASS" and binding
                           for p in root.iterdir()), "Unfinished/orphan ledger stage; explicit recovery required")
        r.need(not attempt or attempt.get("status") == "PASS" and binding, "Unfinished ledger stage; explicit recovery required")


def resume_guard(s):
    guard(s)
    r.need(not s.get("bootstrapBinding") or bool(s.get("activationBinding")),
           "Bootstrapped ledger requires explicit custody/activation; ordinary resume is blocked")


def binding(s, stage):
    ref = s[stage + "Binding"]
    source = file(ref["path"], below=Path(s["private"]) / (stage + "-attempts"))
    r.need(source["sha256"] == ref["sha256"], "Ledger binding drift")
    value = r.read(source["path"])
    r.need(value["contract"] == CONTRACT and value["stage"] == stage and value["status"] == "PASS"
           and all(value[k] == s[k] for k in ("runId", "workspaceId", "actorId", "root"))
           and value["previousBusinessBinding"] == s["businessBinding"] and value["origin"] == s["originSnapshot"],
           "Ledger identity/original binding drift")
    for artifact in value["files"]:
        r.need(file(artifact["path"])["sha256"] == artifact["sha256"], "Ledger artifact drift")
    return value


def native(s, *, hosts=False):
    result = {}
    for role, owner in s["containers"].items():
        item = json.loads(r.docker("inspect", owner["id"]))[0]; r.verify_container(s, role, item)
        r.need(item["State"]["Status"] == "exited", "Ledger plan requires same stopped containers")
        result[role] = {"id": item["Id"], "image": item["Image"], "configHash": r.container_hash(item)}
    processes = {}
    for role, owner in s["processes"].items():
        observed = r.process_identity(owner["pid"], expected=owner, allow_missing=True)
        r.need(hosts or observed is None, "Recorded host still live")
        processes[role] = observed
    for name, port in s.get("ports", {}).items():
        if name in ("api", "web") and any(processes.values()): continue
        with socket.socket() as sock:
            try: sock.bind(("127.0.0.1", port))
            except OSError: raise r.Refused("Original ledger port occupied; no replacement port") from None
    return {"containers": result, "processes": processes}


def version_facts(s, facts):
    r.need(facts["workspaceId"] == s["workspaceId"], "Same workspace facts required")
    versions = facts["facts"]["versions"]
    outputs = u.verify_business(s)["business"]["outputs"]
    derived = [v for name in ("standardized", "curated") for v in versions if v["dataset_id"] == outputs[name]]
    raw = [str(uuid.UUID(b["version_id"])) for b in facts["facts"]["bindings"]]
    r.need(len(versions) == len({v["id"] for v in versions}) == 5 and len(raw) == len(set(raw)) == 3 and len(derived) == 2
           and len({v["dataset_id"] for v in derived}) == 2 and set(raw).isdisjoint(v["id"] for v in derived)
           and set(raw) | {v["id"] for v in derived} == {v["id"] for v in versions}, "Exact original five version membership required")
    expected = []
    for v in versions:
        identity = str(uuid.UUID(v["id"]))
        r.need(v["status"] == "READY" and v["checksum_algorithm"] == "SHA256" and re.fullmatch("[0-9a-f]{64}", v["checksum_value"])
               and type(v["byte_size"]) is int and 0 < v["byte_size"] <= 128 << 20 and v["storage_uri"].startswith("s3://raw/"),
               "Immutable five-version content identity required")
        expected.append({"id": identity, "sha256": v["checksum_value"], "bytes": v["byte_size"], "uri": v["storage_uri"]})
    return sorted(expected, key=lambda v: v["id"]), derived


def base_plan(s, stage, admission):
    guard(s)
    r.need(s["phase"] in ("RUNNING", "STOPPED") and not s.get("activation"), "Known unactivated retained environment required")
    r.need(not s.get(stage) and not s.get(stage + "Binding"), "Stage already attempted; no retry")
    registry = file(r.STATE / "state.json")
    r.need(r.read(registry["path"]) == s, "Registry drift")
    r.verify(s, source=False, running=False); c.verify_origin(s)
    configuration = file(Path(s["private"]) / "config.json")
    cfg = r.read(configuration["path"]); dsn = urllib.parse.urlsplit(cfg["DSN"])
    r.need(cfg["Private"] == s["private"] and cfg["Repo"] == s["root"] and cfg["Workspace"] == s["workspaceId"] and cfg["Actor"] == s["actorId"]
           and dsn.scheme == "postgres" and dsn.hostname == "127.0.0.1" and dsn.port == s["ports"]["core"] and dsn.path == "/dpp_trial_core"
           and cfg["Profile"]["Subject"] == "trial-operator-" + s["runId"] and cfg["Profile"]["Bucket"] == "raw"
           and cfg["Profile"]["StorageEndpoint"] == "127.0.0.1:" + str(s["ports"]["storage"]) and not cfg["Profile"].get("StorageTLS", False),
           "Original native Core/config/principal composition drift")
    result = {"contract": CONTRACT, "stage": stage, **{k: s[k] for k in ("runId", "workspaceId", "actorId", "root")},
              "registry": registry, "origin": copy.deepcopy(s["originSnapshot"]), "previousBusinessBinding": copy.deepcopy(s["businessBinding"]),
              "target": target(s, admission), "native": native(s, hosts=stage == "bootstrap"), "config": configuration}
    result["providerWitness"] = r.digest(encoded(result["native"]["containers"]))
    return result


def plan(s, stage, *, facts=None, admission=None, profile=None):
    with r.read_only_observation():
        r.need(stage in STAGES, "Explicit ledger stage required")
        result = base_plan(s, stage, r.read(admission))
        result["admissionFile"] = file(admission)
        if stage == "bootstrap":
            r.need(not s.get("bootstrapBinding"), "Bootstrap already completed")
            result["facts"] = file(facts, below=r.STATE)
            result["versions"], _ = version_facts(s, r.read(result["facts"]["path"]))
            result["migrations"] = migrations()
            result["migrationVersions"] = [int(n[:6]) for n in result["migrations"] if n.endswith(".up.sql")]
            result["expectedSchema"] = {"from": 68, "to": 71}
        else:
            before = binding(s, "bootstrap")
            r.need(before["target"] == result["target"] and s["phase"] == "STOPPED", "Same accepted bootstrap source required")
            result["bootstrap"] = copy.deepcopy(s["bootstrapBinding"])
            result["versions"] = before["plan"]["versions"]
            result["migrationVersions"] = before["plan"]["migrationVersions"]
            result["profile"] = file(profile, below=s["private"])
            p = r.read(result["profile"]["path"])
            cfg = r.read(Path(s["private"]) / "config.json")
            scope = p["plan"]
            witness = result["providerWitness"]
            r.need(scope["workspaceId"] == s["workspaceId"] and scope["actorId"] == s["actorId"] and scope["runId"] == s["runId"]
                   and scope["principalRef"] == cfg["Profile"]["Subject"] == "trial-operator-" + s["runId"]
                   and scope["endpoint"] == cfg["Profile"]["StorageEndpoint"] and scope["providerWitness"] == witness
                   and scope["planSha256"] == s["bootstrap"]["planSha256"]
                   and p["credentialFile"] == str(Path(s["private"]) / "config.json")
                   and file(p["credentialFile"])["sha256"] == p["credentialFileSha256"], "Custody profile/native binding drift")
            _, derived = version_facts(s, r.read(before["plan"]["facts"]["path"]))
            for name, v in zip(("standardized", "curated"), derived):
                r.need(scope[name] == {"versionId": v["id"], "uri": v["storage_uri"], "sha256": v["checksum_value"], "bytes": v["byte_size"]},
                       "Custody must retain exact two original source objects")
            expiry = datetime.fromisoformat(scope["expiresAt"].replace("Z", "+00:00"))
            r.need(0 < (expiry - datetime.now(timezone.utc)).total_seconds() <= 3600, "Fresh bounded custody authority required")
            destination = Path(scope["outputDirectory"])
            r.need(destination.is_absolute() and destination.is_dir() and not any(destination.iterdir())
                   and destination.resolve().is_relative_to(Path(s["private"]).resolve()) and destination.resolve() != Path(s["private"]).resolve(),
                   "Existing empty retained-private custody directory required")
        r.need(file(result["registry"]["path"]) == result["registry"], "Registry changed during plan")
        return result


def build(directory, names):
    result = {}
    for name in names:
        output = directory / (name + ".exe")
        r.command(["go", "build", "-trimpath", "-o", output, "./cmd/" + name], cwd=r.ROOT / "apps/platform", timeout=600)
        result[name] = file(output)
    return result


def owned(s, role, operation):
    with r.read_only_observation():
        r.verify(s, source=False, running=False)
    r.need(r.local_docker_identity()["daemonId"] == s["host"]["daemonId"], "Daemon changed before action")
    item = json.loads(r.docker("inspect", s["containers"][role]["id"]))[0]; r.verify_container(s, role, item)
    r.need(item["State"]["Status"] == ("exited" if operation == "start" else "running"), "Container state changed before action")
    r.docker(operation, *(["--time", "30"] if operation == "stop" else []), item["Id"], timeout=180)


def migrate(s, directory, expected):
    r.need(migrations() == expected, "Migration manifest changed")
    stage = directory / "migrations"; stage.mkdir()
    for name, sha in expected.items():
        value = (r.ROOT / "migrations" / name).read_bytes()
        r.need(r.digest(value) == sha, "Migration changed before staging")
        with (stage / name).open("xb") as f: f.write(value); f.flush(); os.fsync(f.fileno())
    cfg = r.read(Path(s["private"]) / "config.json")
    r.command([directory / "migrate.exe", "-direction", "up", "-dir", stage],
              env=dict(r.environment(), POSTGRES_DSN=cfg["DSN"]), timeout=300)


def inspect(s, directory, expected, *, schema=71, custody=None):
    from activation import tool
    return json.loads(tool(s, directory, "trial-activate", "admit", versions=expected["versions"], schema=schema,
                           schemaVersions=[n for n in expected["migrationVersions"] if n <= schema], **(custody or {})))


def execute(s, expected):
    from activation import preserve, tool
    stage = expected["stage"]; attempt = uuid.uuid4().hex
    directory = Path(s["private"]) / (stage + "-attempts") / attempt
    receipts = r.STATE / (stage + "-attempts") / attempt
    directory.mkdir(parents=True, exist_ok=False); receipts.mkdir(parents=True, exist_ok=False)
    put(receipts / "before-state.json", s); put(receipts / "plan.json", expected)
    s[stage] = {"attemptId": attempt, "status": "IN_PROGRESS", "receipts": str(receipts), "planSha256": r.digest(encoded(expected))}
    s["phase"] = "LEDGER_STAGE"; r.save(s)
    try:
        names = ("trial-activate", "migrate") if stage == "bootstrap" else ("trial-activate", "operator-custody")
        tools = action(receipts, "build", lambda: build(directory, names))
        private = action(receipts, "private-admission", lambda: json.loads(tool(s, directory, "trial-activate", "private")))
        r.need(private == {"status": "PASS", "privateACL": True}, "Operator-private stage required before native/SQL actions")
        r.need(target(s, expected["target"]["admission"]) == expected["target"], "Source admission changed during build")
        r.verify(s, source=False, running=False); c.verify_origin(s)
        r.need(native(s, hosts=stage == "bootstrap") == expected["native"], "Native identity changed during build")
        inputs = [expected[k] for k in ("facts", "config", "profile", "admissionFile") if k in expected]
        for source in inputs:
            r.need(file(source["path"]) == source, "Reviewed ledger input changed during build")
        files = list(tools.values()) + inputs + [file(receipts / "before-state.json"), file(receipts / "plan.json")]
        if stage == "bootstrap":
            for role, owner in reversed(list(s["processes"].items())):
                action(receipts, "stop-host-" + role, lambda owner=owner: r.process_identity(owner["pid"], terminate=True, expected=owner, allow_missing=True))
            for owner in s["processes"].values():
                r.wait_for(lambda owner=owner: r.process_identity(owner["pid"], expected=owner, allow_missing=True) is None, "exact host exit")
        action(receipts, "start-core", lambda: owned(s, "core", "start"))
        r.wait_for(lambda: r.docker("exec", s["containers"]["core"]["id"], "pg_isready", "-U", "postgres"), "Core")
        proof = inspect(s, directory, expected, schema=68 if stage == "bootstrap" else 71)
        put(receipts / "before-admission.json", proof)
        if stage == "bootstrap":
            backup = action(receipts, "backup", lambda: r.docker("exec", s["containers"]["core"]["id"], "pg_dump", "-U", "postgres", "-d", "dpp_trial_core", timeout=180))
            backup_path = directory / "before.sql"
            with backup_path.open("xb") as f: f.write(backup); f.flush(); os.fsync(f.fileno())
            files.append(file(backup_path))
            inventory = json.loads(tool(s, directory, "trial-activate", "snapshot"))
            baseline = directory / "before-inventory.json"; put(baseline, inventory); files.append(file(baseline))
            action(receipts, "migration", lambda: migrate(s, directory, expected["migrations"]))
            preserve(inventory, json.loads(tool(s, directory, "trial-activate", "snapshot", baseline=str(baseline))))
            proof = inspect(s, directory, expected)
        else:
            for role in ("objects", "gateway"): action(receipts, "start-" + role, lambda role=role: owned(s, role, "start"))
            # Deliberately no generic wait_containers, health GET, HEAD or STS.
            cfg = r.read(Path(s["private"]) / "config.json")
            p = r.read(expected["profile"]["path"])
            r.need(file(expected["profile"]["path"]) == expected["profile"], "Custody profile changed")
            result = action(receipts, "custody", lambda: json.loads(r.command([directory / "operator-custody.exe", "-profile", expected["profile"]["path"],
                "-profile-sha256", expected["profile"]["sha256"], "-idempotency-key", "trial-custody:" + s["runId"] + ":" + attempt],
                env=dict(r.environment(), POSTGRES_DSN=cfg["DSN"]), timeout=180)))
            manifest = file(Path(p["plan"]["outputDirectory"]) / "custody.json")
            r.need(result["status"] == "ACCEPTED" and result["manifestSha256"] == manifest["sha256"], "Core custody acceptance required")
            proof = inspect(s, directory, expected, custody={"custodyId": result["acquisitionId"], "custodyPlan": p["plan"], "manifestSha256": manifest["sha256"]})
            files += [expected["profile"], manifest] + [file(x["file"]) for x in r.read(manifest["path"])]
            for role in ("gateway", "objects"): action(receipts, "stop-" + role, lambda role=role: owned(s, role, "stop"))
        put(receipts / "core-admission.json", proof); files.append(file(receipts / "core-admission.json"))
        action(receipts, "stop-core", lambda: owned(s, "core", "stop"))
        native(s)
        manifest = {"contract": CONTRACT, "stage": stage, "status": "PASS", **{k: s[k] for k in ("runId", "workspaceId", "actorId", "root")},
                    "previousBusinessBinding": copy.deepcopy(s["businessBinding"]), "origin": copy.deepcopy(s["originSnapshot"]),
                    "target": expected["target"], "plan": expected, "files": files, "coreAdmission": proof}
        path = directory / "manifest.json"; put(path, manifest)
        s[stage + "Binding"] = file(path); s[stage]["status"] = "PASS"; s["phase"] = "STOPPED"; r.save(s)
        return {"status": "PASS", "stage": stage, "runId": s["runId"], "workspaceId": s["workspaceId"], "binding": s[stage + "Binding"]}
    except BaseException as error:
        put(receipts / "failed.json", {"status": "UNKNOWN", "failureType": type(error).__name__, "retained": True})
        s[stage]["status"] = "UNKNOWN"; s["phase"] = "UNKNOWN"; r.save(s)
        raise


def run(s, stage, plan_path, digest):
    expected = r.read(plan_path)
    r.need(expected.get("stage") == stage and r.digest(encoded(expected)) == digest, "Stage/plan digest mismatch")
    with r.read_only_observation():
        guard(s)
        if s.get(stage + "Binding"):
            old = binding(s, stage)
            r.need(r.digest(encoded(old["plan"])) == digest, "Different stage already accepted")
            r.verify(s, source=False, running=False); c.verify_origin(s)
            return {"status": "PASS", "stage": stage, "replay": True, "runId": s["runId"]}
        # Reconstruct against the exact same files; admission is public metadata.
        actual = plan(s, stage, admission=expected["admissionFile"]["path"],
                      facts=expected.get("facts", {}).get("path"), profile=expected.get("profile", {}).get("path"))
        r.need(actual == expected, "Stage admission drift; review a fresh plan")
    return execute(s, expected)
