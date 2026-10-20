"""Explicit, single-attempt activation of the retained #325 five-version trial.

Planning is read-only. Execution is a separate command; UNKNOWN never retries.
All business commands, seeding, IAM changes and legacy object reads are excluded.
"""
from collections import Counter
import copy
import json
from pathlib import Path
import re
import socket
import uuid

import runtime as r
import resume as c
import upgrade as u

CONTRACT = "issue325/protected-activation-v2"
LIMIT = 128 << 20


def encoded(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode()


def fingerprint(path, *, below=None):
    path = Path(path)
    r.need(path.is_absolute() and path.is_file() and not path.is_symlink(), "Absolute regular activation input required")
    if below:
        r.need(path.resolve().is_relative_to(Path(below).resolve()), "Activation input escapes retained directory")
    r.need(path.stat().st_size <= LIMIT, "Synthetic activation input exceeds bound")
    return {"path": str(path.resolve()), "sha256": r.digest(path.read_bytes())}


def target(s):
    import bootstrap as b
    accepted = b.binding(s, "bootstrap")
    return b.target(s, accepted["target"]["admission"])


def readiness(s):
    import bootstrap as b
    b.guard(s)
    bootstrap, custody = b.binding(s, "bootstrap"), b.binding(s, "custody")
    r.need(bootstrap["coreAdmission"]["schema"] == custody["coreAdmission"]["schema"] == 71
           and custody["plan"]["bootstrap"] == s["bootstrapBinding"]
           and custody["coreAdmission"].get("custodyId") and custody["coreAdmission"].get("manifestSha256"),
           "Schema-71 bootstrap and Core accepted custody required")
    profile = r.read(custody["plan"]["profile"]["path"])
    manifest = fingerprint(Path(profile["plan"]["outputDirectory"]) / "custody.json")
    r.need(manifest["sha256"] == custody["coreAdmission"]["manifestSha256"], "Accepted custody manifest drift")
    return {"bootstrap": copy.deepcopy(s["bootstrapBinding"]), "custody": copy.deepcopy(s["custodyBinding"]),
            "facts": bootstrap["plan"]["facts"], "manifest": manifest, "proof": custody["coreAdmission"]}


def pending(s):
    r.need(not s.get("activationBinding") and not s.get("activation"), "Activation already attempted; inspect existing journal")
    # Include the allocation-before-registry crash window. Never discard or reuse.
    for base in (r.STATE / "storage-activations", Path(s["private"]) / "activations"):
        r.need(not base.exists() or not any(base.iterdir()), "Unfinished/orphan activation journal; explicit recovery required")


def stopped(s):
    observations = {}
    for role, owner in s["containers"].items():
        actual = json.loads(r.docker("inspect", owner["id"]))[0]
        r.verify_container(s, role, actual)
        r.need(actual["State"]["Status"] == "exited", "Activation requires the same stopped containers")
        observations[role] = {"id": actual["Id"], "image": actual["Image"], "state": "exited", "configHash": r.container_hash(actual)}
    for owner in s["processes"].values():
        # expected rejects a reused live PID, too; never terminate during plan.
        r.need(r.process_identity(owner["pid"], expected=owner, allow_missing=True) is None,
               "Recorded host process still running")
    sockets = []
    try:
        for port in s["ports"].values():
            sock = socket.socket(); sockets.append(sock)
            try: sock.bind(("127.0.0.1", port))
            except OSError: raise r.Refused("Original trial port unavailable; no replacement environment") from None
    finally:
        for sock in sockets: sock.close()
    return observations


def versions(s, facts, custody):
    r.need(facts["workspaceId"] == s["workspaceId"], "Same-workspace facts required")
    vs = facts["facts"]["versions"]
    bindings = facts["facts"]["bindings"]
    raw = {str(uuid.UUID(b["version_id"])) for b in bindings}
    r.need(len(bindings) == len(raw) == 3 and len(vs) == 5, "Exactly original three RAW and two processed versions required")
    outputs = u.verify_business(s)["business"]["outputs"]
    derived = {v["id"] for v in vs if v["dataset_id"] in (outputs["standardized"], outputs["curated"])}
    r.need(len(derived) == 2 and len({v["dataset_id"] for v in vs if v["id"] in derived}) == 2,
           "Original processing output membership required")
    result = []
    for v in vs:
        identity = str(uuid.UUID(v["id"]))
        r.need(v["status"] == "READY" and v["checksum_algorithm"] == "SHA256" and
               bool(re.fullmatch("[0-9a-f]{64}", v["checksum_value"])) and isinstance(v["byte_size"], int) and 0 < v["byte_size"] <= LIMIT,
               "Ready immutable version content identity required")
        result.append({"id": identity, "sha256": v["checksum_value"], "bytes": v["byte_size"]})
    r.need(len({v["id"] for v in result}) == 5 and {v["id"] for v in result} == raw | derived and not raw & derived,
           "Original five-version membership conflict")
    r.need(isinstance(custody, list) and len(custody) == 2 and {x["versionId"] for x in custody} == derived,
           "Exactly two explicit processed custody files required")
    files = []
    for item in custody:
        f = fingerprint(item["file"])
        v = next(v for v in result if v["id"] == item["versionId"])
        r.need(f["sha256"] == v["sha256"] and Path(f["path"]).stat().st_size == v["bytes"], "Custody bytes differ from immutable version")
        files.append({"versionId": v["id"], **f})
    return sorted(result, key=lambda v: v["id"]), sorted(files, key=lambda f: f["versionId"])


def plan(s, facts_path, custody_path):
    with r.read_only_observation():
        return inspect_plan(s, facts_path, custody_path)


def inspect_plan(s, facts_path, custody_path):
    r.need(s["phase"] in ("STOPPED", "RUNNING"), "Known retained stopped owners required; UNKNOWN cannot activate")
    pending(s)
    registry = fingerprint(r.STATE / "state.json")
    r.need(r.read(registry["path"]) == s, "Registry changed before plan")
    r.verify(s, source=False, running=False); c.verify_origin(s)
    binding = target(s)
    accepted = readiness(s)
    native = stopped(s)
    facts = fingerprint(facts_path, below=r.STATE)
    custody = fingerprint(custody_path)
    r.need(facts == accepted["facts"] and custody == accepted["manifest"], "Activation must use Core accepted custody and original facts")
    expected, files = versions(s, r.read(facts["path"]), r.read(custody["path"]))
    r.need(fingerprint(registry["path"]) == registry, "Registry changed during plan")
    # No timestamps: re-derivation of unchanged admission facts has the same digest.
    return {"contract": CONTRACT, **{k: s[k] for k in ("runId", "workspaceId", "actorId", "root")},
            "target": binding, "registry": registry, "origin": copy.deepcopy(s["originSnapshot"]),
            "previousBusinessBinding": copy.deepcopy(s["businessBinding"]), "recordedPhase": s["phase"],
            "native": native, "facts": facts, "custody": custody, "files": files, "versions": expected, "ledger": accepted}


def preserve(before, after):
    for name, old in before["tables"].items():
        new = after["tables"].get(name)
        r.need(new is not None and old["columns"] == new["columns"] and not (Counter(old["rows"]) - Counter(new["rows"])),
               "Original public table rows changed: " + name)


def verify_binding(s):
    binding = s["activationBinding"]
    f = fingerprint(binding["path"], below=Path(s["private"]) / "activations")
    r.need(f["sha256"] == binding["sha256"], "Activation manifest drift")
    manifest = r.read(f["path"])
    r.need(manifest["contract"] == CONTRACT and all(manifest[k] == s[k] for k in ("runId", "workspaceId", "actorId", "root"))
           and manifest["previousBusinessBinding"] == s["businessBinding"], "Activation identity/previous binding drift")
    r.need(manifest["ledger"]["bootstrap"] == s["bootstrapBinding"] and manifest["ledger"]["custody"] == s["custodyBinding"],
           "Activation ledger binding drift")
    import bootstrap as b
    b.binding(s, "bootstrap"); b.binding(s, "custody")
    old = u.verify_business(s)
    r.need(manifest["business"] == old["business"] and manifest["humanTokenFile"] == old["humanTokenFile"], "Frozen business identity drift")
    r.need(c.committed_sources(manifest["head"]) == manifest["sources"], "Activation source manifest drift")
    folder = Path(f["path"]).parent
    for item in manifest["artifacts"]["binaries"].values():
        r.need(fingerprint(item["path"], below=folder)["sha256"] == item["sha256"], "Activated binary drift")
    web = manifest["artifacts"]["web"]
    for name, sha in web["files"].items():
        r.need(fingerprint(Path(web["root"]) / name, below=folder)["sha256"] == sha, "Activated production Web drift")
    profile = r.read(s["workspaceObjectProfile"])
    r.need(profile_identity(profile) == manifest["protectedIdentity"], "Protected runtime scope drift")
    r.need(r.digest(Path(s["workspaceObjectProfile"]).read_bytes()) == s["privateHashes"].get(s["workspaceObjectProfile"]), "Unbound protected profile")
    return manifest


def profile_identity(profile):
    value = copy.deepcopy(profile)
    value.pop("CredentialFile", None)
    value["Scope"].pop("ExpiresAt", None)
    return value


def make_profile(s, credential, expires):
    cfg = r.read(Path(s["private"]) / "config.json")
    r.need(str(cfg["Workspace"]) == s["workspaceId"] and str(cfg["Actor"]) == s["actorId"] and
           cfg["Profile"]["Subject"] == "trial-operator-" + s["runId"] and cfg["Profile"]["Bucket"] == "raw", "Original principal composition drift")
    return {"Endpoint": cfg["Profile"]["StorageEndpoint"], "Secure": False, "CredentialFile": str(credential),
            "Scope": {"WorkspaceID": s["workspaceId"], "ActorID": s["actorId"], "NamespaceID": str(uuid.uuid5(uuid.UUID(s["workspaceId"]), "core-object-namespace")),
                      "ProviderInstanceID": str(uuid.uuid5(uuid.NAMESPACE_URL, cfg["Private"] + "/objects")),
                      "IncarnationID": str(uuid.uuid5(uuid.NAMESPACE_URL, cfg["Private"] + "/objects/data")),
                      "PrincipalRef": cfg["Profile"]["Subject"], "Bucket": "raw", "ExpiresAt": expires}}


def action(receipts, name, operation):
    c.write_once(receipts / (name + "-started.json"), {"action": name})
    value = operation()
    c.write_once(receipts / (name + "-result.json"), {"status": "PASS"})
    return value


def tool(s, directory, name, mode=None, **inputs):
    path = directory / (uuid.uuid4().hex + "-input.json")
    c.write_once(path, {"configFile": str(Path(s["private"]) / "config.json"), **inputs})
    return r.command([directory / (name + ".exe"), *([mode] if mode else [])],
                     env=dict(r.environment(), DPP_TRIAL_ACTIVATION_INPUT=str(path)), timeout=300)


def build(s, directory, attempt):
    artifacts = u.build(s, directory, attempt)
    for name in ("workspace-object-copy", "trial-resume", "trial-activate"):
        r.command(["go", "build", "-trimpath", "-o", directory / (name + ".exe"), "./cmd/" + name], cwd=r.ROOT / "apps/platform", timeout=600)
    return artifacts


def execute(s, expected):
    import bootstrap as b
    attempt = uuid.uuid4().hex
    directory = Path(s["private"]) / "activations" / attempt
    receipts = r.STATE / "storage-activations" / attempt
    directory.mkdir(parents=True, exist_ok=False); receipts.mkdir(parents=True, exist_ok=False)
    (receipts / "before-state.bytes").write_bytes((r.STATE / "state.json").read_bytes())
    c.write_once(receipts / "plan.json", expected)
    s["activation"] = {"attemptId": attempt, "status": "ACTIVATING", "receipts": str(receipts), "planSha256": r.digest(encoded(expected))}
    s["phase"] = "ACTIVATING"; r.save(s)
    try:
        artifacts = action(receipts, "build", lambda: build(s, directory, attempt))
        private = json.loads(tool(s, directory, "trial-activate", "private"))
        r.need(private == {"status": "PASS", "privateACL": True}, "Operator-private activation required before native/SQL actions")
        r.need(target(s) == expected["target"] and readiness(s) == expected["ledger"], "Source/ledger changed during activation build")
        action(receipts, "core", lambda: b.owned(s, "core", "start"))
        r.wait_for(lambda: r.docker("exec", s["containers"]["core"]["id"], "pg_isready", "-U", "postgres"), "Core")
        proof = expected["ledger"]["proof"]
        live = json.loads(tool(s, directory, "trial-activate", "admit", schema=71, schemaVersions=proof["schemaVersions"], versions=proof["versions"],
                          custodyId=proof["custodyId"], custodyPlan=proof["custodyPlan"], manifestSha256=proof["manifestSha256"]))
        r.need(live == proof, "Core accepted custody/schema changed; no credential or copy released")
        c.write_once(receipts / "ledger-admission.json", live)
        for role in s["containers"]:
            if role != "core": action(receipts, "start-" + role, lambda role=role: b.owned(s, role, "start"))
        c.wait_containers(s)
        before = r.inspect_facts(s); u.raw_equal(r.read(expected["facts"]["path"]), before)
        r.need(len(before["facts"]["versions"]) == 5, "Unexpected version appeared since plan")
        c.write_once(receipts / "before-facts.json", before)
        retained = u.retained(s); c.write_once(directory / "retained.json", retained)
        r.need(u.sql(s, "SELECT max(version) FROM schema_migration") == 71, "Accepted bootstrap migration head must be 71")
        backup = action(receipts, "backup", lambda: r.docker("exec", s["containers"]["core"]["id"], "pg_dump", "-U", "postgres", "-d", "dpp_trial_core", timeout=180))
        with (directory / "before.sql").open("xb") as f: f.write(backup)
        c.write_once(receipts / "backup.json", {"sha256": r.digest(backup), "bytes": len(backup)})
        inventory = json.loads(tool(s, directory, "trial-activate", "snapshot"))
        baseline = directory / "before-inventory.json"; c.write_once(baseline, inventory)
        # Bootstrap alone owns migrations. Activation only consumes schema 71.
        # Do not widen the existing STS helper's output path or policy.
        renewal = Path(s["private"]) / "resume" / attempt; renewal.mkdir(parents=True, exist_ok=False)
        s["resume"] = {"attemptId": attempt, "status": "RESUMING", "receipts": str(receipts)}; r.save(s)
        refreshed = c.refresh_identity(s, renewal, receipts)
        profile = make_profile(s, renewal / "core-identity.json", refreshed["expiresAt"])
        profile_path = directory / "workspace-object-profile.json"; c.write_once(profile_path, profile)
        s["workspaceObjectProfile"] = str(profile_path)
        s["privateHashes"][str(profile_path)] = r.digest(profile_path.read_bytes()); r.save(s)
        cfg = r.read(Path(s["private"]) / "config.json")
        for item in expected["files"]:
            # Rehash immediately before custody handoff, not just during plan.
            r.need(fingerprint(item["path"])["sha256"] == item["sha256"], "Custody file changed")
            def copy_object(item=item):
                return r.command([directory / "workspace-object-copy.exe", "-profile", profile_path, "-version", item["versionId"],
                                  "-content-file", item["path"], "-idempotency-key", "trial-activation:" + s["runId"] + ":" + item["versionId"]],
                                 env=dict(r.environment(), POSTGRES_DSN=cfg["DSN"]), timeout=300)
            action(receipts, "copy-" + item["versionId"], copy_object)
        probe = action(receipts, "protected-probe", lambda: json.loads(tool(s, directory, "trial-activate", "verify", profile=str(profile_path), versions=expected["versions"])))
        c.write_once(receipts / "protected-probe.json", probe)
        preserve(inventory, json.loads(tool(s, directory, "trial-activate", "snapshot", baseline=str(baseline))))
        c.write_once(receipts / "preservation.json", {"status": "PASS", "originalPublicTables": len(inventory["tables"]), "beforeHostWriters": True})
        old = u.verify_business(s)
        manifest = {"contract": CONTRACT, **{k: s[k] for k in ("runId", "workspaceId", "actorId", "root")}, **expected["target"],
                    "previousBusinessBinding": copy.deepcopy(s["businessBinding"]), "planSha256": s["activation"]["planSha256"],
                    "artifacts": artifacts, "business": old["business"], "humanTokenFile": old["humanTokenFile"], "protectedIdentity": profile_identity(profile),
                    "ledger": expected["ledger"]}
        path = directory / "manifest.json"; c.write_once(path, manifest)
        s["activationBinding"] = {"path": str(path), "sha256": r.digest(path.read_bytes())}
        s["controllerBinding"] = expected["target"]; s["processes"] = {}; r.save(s)
        action(receipts, "hosts", lambda: c.launch_hosts(s))
        u.raw_equal(before, r.inspect_facts(s)); u.retained_equal(retained, u.retained(s))
        c.write_once(receipts / "completed.json", {"status": "PASS", "runId": s["runId"], "workspaceId": s["workspaceId"], "actorId": s["actorId"],
                     "planSha256": s["activation"]["planSha256"], "verifiedVersions": 5, "nativeSTSInvocations": refreshed["nativeSTSInvocations"]})
        s["activation"]["status"] = "PASS"; s["resume"]["status"] = "PASS"; s["phase"] = "RUNNING"; r.save(s)
        return r.status(s)
    except BaseException as error:
        c.write_once(receipts / "failed.json", {"status": "UNKNOWN", "failureType": type(error).__name__, "retained": True})
        s["phase"] = "UNKNOWN"; s["activation"]["status"] = "UNKNOWN"; r.save(s)
        raise


def activate(s, plan_path, digest):
    expected = r.read(plan_path)
    r.need(r.digest(encoded(expected)) == digest, "Activation plan digest mismatch")
    if s.get("activationBinding") and s.get("activation", {}).get("status") == "PASS":
        # Read only, including registry/status files; do not call runtime.status.
        with r.read_only_observation():
            manifest = verify_binding(s)
            r.need(manifest["planSha256"] == digest, "Different activation already accepted")
            r.verify(s, running=s["phase"] == "RUNNING")
        return {"status": "PASS", "replay": True, "runId": s["runId"], "workspaceId": s["workspaceId"], "activationHead": manifest["head"]}
    actual = plan(s, expected["facts"]["path"], expected["custody"]["path"])
    r.need(actual == expected, "Activation admission drift; generate and review a fresh plan")
    return execute(s, expected)
