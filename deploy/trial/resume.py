"""Explicit continuation of the same retained trial; never bootstraps data."""
import base64
import copy
import io
import json
from pathlib import Path
import socket
import tarfile
import uuid

import runtime as r


def write_once(path, value):
    with Path(path).open("x", encoding="utf-8") as f:
        json.dump(value, f, ensure_ascii=False, indent=2)


def committed_sources(head):
    r.need(len(head) == 40 and all(c in "0123456789abcdef" for c in head), "Recorded Git commit required")
    data = r.command(["git", "archive", "--format=tar", head, "--", *r.SOURCE_PATHS])
    with tarfile.open(fileobj=io.BytesIO(data)) as archive:
        return {m.name: r.digest(archive.extractfile(m).read().replace(b"\r\n", b"\n"))
                for m in archive.getmembers() if m.isfile() and m.name != "apps/web/next-env.d.ts"}


def controller_only(path):
    return path.startswith(("deploy/trial/", "apps/platform/cmd/trial-resume/", "apps/platform/cmd/trial-activate/"))


def check_sources(s, current):
    baseline = s["sources"]
    if "businessBinding" in s:
        baseline = r.active_business(s)["sources"]
    changed = sorted(p for p in set(current) | set(baseline) if current.get(p) != baseline.get(p))
    r.need(all(controller_only(p) for p in changed), "Business source drift; this resume only reuses original business artifacts")
    return changed


def verify_origin(s):
    binding = s["originSnapshot"]
    path = r.STATE / "resume-origin.json"
    r.need(r.digest(path.read_bytes()) == binding["sha256"], "Original registry snapshot drift")
    original = r.read(path)
    for key in ("contract", "runId", "root", "private", "project", "host", "head", "sources", "workspaceId", "actorId",
                "ports", "subnets", "containers", "volumes", "networks", "images", "minio", "composeSha256", "resourceIds", "api", "webBuild", "web"):
        r.need(s[key] == original[key], "Original trial identity/artifact binding drift")
    for key in ("binaries", "privateHashes"):
        r.need(all(s[key].get(name) == value for name, value in original[key].items()), "Original private artifact binding drift")
    r.need(set(original["secretReferences"]).issubset(s["secretReferences"]), "Original secret references must be retained")
    if original.get("workspaceObjectProfile"):
        r.need(bool(s.get("workspaceObjectProfile")), "Protected object profile binding removed")
        old, current = r.read(original["workspaceObjectProfile"]), r.read(s["workspaceObjectProfile"])
        for key in ("Endpoint", "Secure"):
            r.need(old.get(key) == current.get(key), "Protected storage endpoint drift")
        old_scope, current_scope = copy.deepcopy(old["Scope"]), copy.deepcopy(current["Scope"])
        old_scope.pop("ExpiresAt", None); current_scope.pop("ExpiresAt", None)
        r.need(old_scope == current_scope, "Protected object scope/identity drift")


def verify_binding(s):
    verify_origin(s)
    binding = s["controllerBinding"]
    check_sources(s, binding["sources"])
    r.need(committed_sources(binding["head"]) == binding["sources"] == r.sources(), "Controller/source drift; explicit stopped resume required")


def precheck(s):
    r.need(s["phase"] == "STOPPED", f"Run {s['runId']} is {s['phase']}; resume requires known STOPPED, never reseeds")
    # Verify actual retained artifacts before accepting any controller update.
    r.verify(s, source=False, running=False)
    if "originSnapshot" in s:
        verify_origin(s)
    r.need(committed_sources(s["head"]) == s["sources"], "Original source manifest does not match its recorded commit")
    current = r.sources()
    head = r.command(["git", "rev-parse", "HEAD"]).decode().strip()
    r.need(committed_sources(head) == current, "Resume controller/source must be committed; retain this run")
    changes = check_sources(s, current)
    for item in s["containers"].values():
        actual = json.loads(r.docker("inspect", item["id"]))[0]
        r.need(actual["State"]["Status"] == "exited", "Resume requires the same stopped containers")
    for expected in s["processes"].values():
        actual = r.process_identity(expected["pid"], allow_missing=True)
        # A reused PID belongs to another process; it is never a stop target.
        r.need(actual != expected, "Recorded host process still running; stop exact owners first")
    sockets = []
    try:
        for port in s["ports"].values():
            sock = socket.socket(); sockets.append(sock)
            try: sock.bind(("127.0.0.1", port))
            except OSError: raise r.Refused("Original trial port unavailable; no replacement port or environment") from None
    finally:
        for sock in sockets: sock.close()
    baseline = r.read(r.STATE / "before-stop-facts.json")
    r.need(baseline["workspaceId"] == s["workspaceId"], "Same-workspace stopped facts required")
    return {"head": head, "sources": current, "deltaPaths": changes}, baseline


def compare_facts(s, baseline):
    actual = r.inspect_facts(s)
    r.need(actual == baseline, "Retained SQL/native object facts differ or are incomplete; resume outcome UNKNOWN")
    return actual


def wait_containers(s):
    for role in ("source", "core"):
        r.wait_for(lambda role=role: r.docker("exec", s["containers"][role]["id"], "pg_isready", "-U", "postgres"), role)
    r.wait_for(lambda: r.http(f"http://127.0.0.1:{s['ports']['storage']}/minio/health/live"), "storage")
    auth = "Basic " + base64.b64encode(("trial:" + (Path(s["private"]) / "hop-password").read_text()).encode()).decode()
    r.wait_for(lambda: r.http(f"http://127.0.0.1:{s['ports']['hop']}/hop/status?xml=Y", {"Authorization": auth}), "Hop")
    r.storage_check(s, ["running"])


def refresh_identity(s, directory, receipts):
    binary, output = directory / "trial-resume.exe", directory / "core-identity.json"
    r.command(["go", "build", "-trimpath", "-o", binary, "./cmd/trial-resume"], cwd=r.ROOT / "apps/platform", timeout=600)
    s["binaries"]["resume-" + directory.name] = {"path": str(binary), "sha256": r.digest(binary.read_bytes())}
    s["secretReferences"].append(str(output)); r.save(s)
    write_once(receipts / "sts-started.json", {"resumeAttemptId": directory.name, "operation": "native STS AssumeRole", "scope": "original workspace, same Core policy"})
    result = json.loads(r.command([binary, "renew-core"], env=dict(r.environment(), DPP_TRIAL_CONFIG=str(Path(s["private"]) / "config.json"), DPP_TRIAL_RENEW_OUTPUT=str(output))))
    write_once(receipts / "sts-result.json", result)
    # The original profile and credential are retained byte for byte. Existing
    # Core code requires a fresh process when its cached credential changes.
    profile = r.read(Path(s["private"]) / "profile.json")
    profile["CoreCredentialFile"] = str(output)
    profile_path = directory / "profile.json"
    write_once(profile_path, profile)
    for path in (output, profile_path): s["privateHashes"][str(path)] = r.digest(path.read_bytes())
    s["activeProfile"] = str(profile_path)
    refresh_workspace_profile(s, directory, output, result["expiresAt"])
    s["resume"]["credentialExpiresAt"] = result["expiresAt"]; r.save(s)
    probe = json.loads(r.command([binary, "verify-core"], env=dict(r.environment(), DPP_TRIAL_CONFIG=str(Path(s["private"]) / "config.json"),
                                 DPP_TRIAL_RENEW_OUTPUT=str(output), DPP_TRIAL_VERIFY_RECEIPT=str(receipts / "before-facts.json"))))
    write_once(receipts / "scoped-object-probe.json", probe)
    return result


def refresh_workspace_profile(s, directory, credential, expires_at):
    # Only already-bound protected runtimes renew this profile. Real retained
    # pre-330 business artifacts keep their old explicit binding until upgrade.
    if not s.get("workspaceObjectProfile"): return
    old = r.read(s["workspaceObjectProfile"])
    r.need(old["Scope"]["WorkspaceID"] == s["workspaceId"] and old["Scope"]["ActorID"] == s["actorId"], "Workspace object identity drift")
    updated = copy.deepcopy(old)
    updated["CredentialFile"] = str(credential)
    updated["Scope"]["ExpiresAt"] = expires_at
    path = directory / "workspace-object-profile.json"
    write_once(path, updated)
    s["privateHashes"][str(path)] = r.digest(path.read_bytes())
    s["workspaceObjectProfile"] = str(path)


def launch_hosts(s):
    logs = Path(s["private"]) / "resume" / s["resume"]["attemptId"]
    artifacts = None
    if "businessBinding" in s:
        artifacts = r.active_business(s)["artifacts"]
    for role in ("api", "worker"):
        binary = (artifacts["binaries"] if artifacts else s["binaries"])[role]["path"]
        r.spawn(s, role, [binary], r.core_environment(s), log_directory=logs)
    r.wait_for(lambda: r.http(s["api"] + "/health/ready"), "API")
    server = Path(artifacts["web"]["root"]) / "server.js" if artifacts else r.ROOT / "apps/web/.next/standalone/server.js"
    r.spawn(s, "web", [s["binaries"]["node"]["path"], server], dict(r.web_environment(s), HOSTNAME="127.0.0.1", PORT=str(s["ports"]["web"])), log_directory=logs)
    r.wait_for(lambda: r.http(s["web"] + "/batches"), "Web", seconds=120)


def resume(s):
    from bootstrap import resume_guard
    resume_guard(s)
    r.need(not s.get("activation") or s["activation"]["status"] == "PASS", "Unfinished activation requires explicit recovery; ordinary resume cannot retry it")
    if s["phase"] == "RUNNING": return r.status(s)
    binding, baseline = precheck(s)
    attempt = uuid.uuid4().hex
    receipts = r.STATE / "resumes" / attempt
    private = Path(s["private"])
    base = private / "resume"
    r.need(base.resolve().is_relative_to(private.resolve()), "Private resume directory escapes original trial")
    directory = base / attempt
    receipts.mkdir(parents=True, exist_ok=False); directory.mkdir(parents=True, exist_ok=False)
    # Keep a permanent original plus a separate snapshot for every continuation.
    if "originSnapshot" not in s:
        path = r.STATE / "resume-origin.json"
        write_once(path, s)
        s["originSnapshot"] = {"sha256": r.digest(path.read_bytes())}
    write_once(receipts / "before-state.json", copy.deepcopy(s))
    write_once(receipts / "before-facts.json", baseline)
    write_once(receipts / "started.json", {"runId": s["runId"], "workspaceId": s["workspaceId"], "resumeAttemptId": attempt, "controllerHead": binding["head"], "sourceDelta": binding["deltaPaths"]})
    s["controllerBinding"] = binding
    s["resume"] = {"attemptId": attempt, "status": "RESUMING", "receipts": str(receipts)}
    s["phase"] = "RESUMING"; s["processes"] = {}; r.save(s)
    try:
        r.docker("start", *[c["id"] for c in s["containers"].values()], timeout=180)
        wait_containers(s)
        write_once(receipts / "containers-facts.json", compare_facts(s, baseline))
        refreshed = refresh_identity(s, directory, receipts)
        launch_hosts(s)
        write_once(receipts / "running-facts.json", compare_facts(s, baseline))
        s["phase"] = "RUNNING"; s["resume"]["status"] = "PASS"; r.save(s)
        result = r.status(s)
        observation = {"runId": s["runId"], "workspaceId": s["workspaceId"], "resumeAttemptId": attempt, "status": "PASS",
                       "sameSQLAndNativeFacts": True, "nativeSTSInvocations": refreshed["nativeSTSInvocations"], "credentialExpiresAt": refreshed["expiresAt"]}
        write_once(receipts / "completed.json", observation)
        s.setdefault("resumeHistory", []).append(observation); r.save(s)
        return result
    except BaseException as e:
        observation = {"runId": s["runId"], "resumeAttemptId": attempt, "status": "UNKNOWN", "failureType": type(e).__name__,
                       "credentialRefresh": "PASS" if (receipts / "sts-result.json").exists() else "UNKNOWN" if (receipts / "sts-started.json").exists() else "NOT_RUN", "retained": True}
        write_once(receipts / "failed.json", observation)
        s["phase"] = "UNKNOWN"; s["resume"]["status"] = "UNKNOWN"
        s.setdefault("resumeHistory", []).append(observation); r.save(s)
        raise


def restart(s):
    from bootstrap import resume_guard
    resume_guard(s)
    r.need(s["phase"] == "RUNNING", "Restart requires known RUNNING; use resume for STOPPED")
    r.verify(s)
    r.stop(s)
    return resume(s)
