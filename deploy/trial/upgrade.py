"""Explicit, versioned business upgrade of the retained synthetic trial.

Uses existing HTTP commands and the existing stop/resume controller. No bootstrap,
migration, SQL business write, replacement container or replacement RAW path.
"""
import copy
import json
from pathlib import Path
import secrets
import shutil
import urllib.error
import urllib.request
import uuid

import runtime as r
import resume as c

WORKFLOW = "examples/enterprise-activity/workflow/workflow-v1.yaml"


def allowed(path):
    return c.controller_only(path) or path in {
        "apps/web/.gitignore", "apps/web/next.config.mjs", "apps/web/app/batches/actions.ts",
        "apps/web/app/batches/[id]/page.tsx"}


def target(s):
    r.need(s["phase"] == "RUNNING", "Business upgrade requires known RUNNING original trial")
    r.need("businessBinding" not in s, "Business upgrade already bound; use status/restart, never duplicate preparation")
    r.verify(s, source=False)
    c.verify_origin(s)
    old = s["controllerBinding"]
    r.need(c.committed_sources(old["head"]) == old["sources"], "Previous controller manifest drift")
    current = r.sources()
    head = r.command(["git", "rev-parse", "HEAD"]).decode().strip()
    r.need(c.committed_sources(head) == current, "Upgrade sources must be committed")
    changed = sorted(p for p in set(current) | set(old["sources"]) if current.get(p) != old["sources"].get(p))
    r.need(all(allowed(p) for p in changed), "Upgrade changes shared/out-of-scope source; produce ownership impact first")
    definition = (r.ROOT / WORKFLOW).read_bytes()
    committed = r.command(["git", "show", head + ":" + WORKFLOW])
    r.need(definition.replace(b"\r\n", b"\n") == committed.replace(b"\r\n", b"\n"), "Workflow source drift")
    # This admission-time input is outside the original runtime SOURCE_PATHS.
    # Fingerprint it separately; expanding the old manifest would invalidate
    # the retained origin. A committed change is still outside this fixed slice.
    previous = r.command(["git", "show", old["head"] + ":" + WORKFLOW])
    canonical = committed.replace(b"\r\n", b"\n")
    r.need(canonical == previous.replace(b"\r\n", b"\n"), "Fixed workflow change is outside business upgrade scope")
    workflow_source = {"path": WORKFLOW, "normalizedSha256": r.digest(canonical), "baselineHead": old["head"]}
    return {"head": head, "sources": current, "deltaPaths": changed, "workflowSource": workflow_source}, definition


def sql(s, query, source=False):
    # Callers use fixed SQL and validated UUIDs only. Database access is READ ONLY.
    db = "dpp_ingestion_source" if source else "dpp_trial_core"
    owner = s["containers"]["source" if source else "core"]["id"]
    data = r.docker("exec", owner, "psql", "-XqAt", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", db,
                    "-c", "BEGIN READ ONLY; " + query + "; COMMIT;")
    return json.loads(data)


def retained(s):
    ws = str(uuid.UUID(s["workspaceId"]))
    tables = ("data_resource", "dataset", "audit_event", "evidence", "cost_event")
    core = {t: sql(s, f"SELECT coalesce(json_agg(to_jsonb(t) ORDER BY id),'[]') FROM {t} t WHERE workspace_id='{ws}'") for t in tables}
    source = {t["Name"]: sql(s, "SELECT json_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text) FROM dpp_source." + t["Name"] + " t", source=True) for t in r.fixture_tables()}
    return {"core": core, "source": source}


def retained_equal(before, after):
    r.need(before["source"] == after["source"], "Original PostgreSQL source rows changed")
    for table, rows in before["core"].items():
        actual = {row["id"]: row for row in after["core"][table]}
        r.need(all(actual.get(row["id"]) == row for row in rows), "Original " + table + " facts changed")


def raw_equal(before, after):
    r.need(before["workspaceId"] == after["workspaceId"] and before["objects"] == after["objects"], "Original workspace/RAW bytes changed")
    for table in ("operations", "receipts", "bindings"):
        r.need(before["facts"][table] == after["facts"][table], "Original ingestion facts changed")
    versions = {v["id"]: v for v in after["facts"]["versions"]}
    r.need(all(versions.get(v["id"]) == v for v in before["facts"]["versions"]), "Original DatasetVersions changed")


def api(s, path, body):
    token = (Path(s["private"]) / "token").read_text().strip()
    request = urllib.request.Request(s["api"] + "/api/v1" + path, method="POST", data=json.dumps(body).encode(),
        headers={"Content-Type": "application/json", "Authorization": "Bearer " + token, "X-Actor-ID": s["actorId"]})
    try:
        with urllib.request.urlopen(request, timeout=60) as response:
            return json.load(response)
    except (urllib.error.URLError, TimeoutError):
        raise r.Refused("Business prerequisite command failed/UNKNOWN; inspect existing facts, never blindly repeat") from None


def prerequisites(s, definition, receipts):
    ws, actor = str(uuid.UUID(s["workspaceId"])), str(uuid.UUID(s["actorId"]))
    outputs = {}
    for suffix, kind in (("standardized", "STANDARDIZED"), ("curated", "CURATED")):
        code = "dpp-source-reader-" + suffix
        rows = sql(s, f"SELECT coalesce(json_agg(to_jsonb(d)),'[]') FROM dataset d WHERE workspace_id='{ws}' AND code='{code}'")
        if not rows:
            c.write_once(receipts / (suffix + "-command-started.json"), {"command": "CreateDataset", "workspaceId": ws, "code": code})
            api(s, "/datasets", {"workspaceId": ws, "code": code, "name": "PostgreSQL " + kind, "datasetType": kind, "ownerId": actor})
            rows = sql(s, f"SELECT coalesce(json_agg(to_jsonb(d)),'[]') FROM dataset d WHERE workspace_id='{ws}' AND code='{code}'")
        r.need(len(rows) == 1 and rows[0]["dataset_type"] == kind and rows[0]["owner_id"] == actor and rows[0]["created_by"] == actor
               and rows[0]["source_resource_id"] is None, "Output dataset ownership/type conflict")
        outputs[suffix] = rows[0]["id"]
        c.write_once(receipts / (suffix + "-dataset.json"), rows[0])
    query = f"SELECT coalesce(json_agg(to_jsonb(v)),'[]') FROM workflow_version v JOIN workflow w ON w.id=v.workflow_id WHERE w.workspace_id='{ws}' AND w.code='TRIAL-ACTIVITY' AND v.version='1.0.0'"
    rows = sql(s, query)
    if not rows:
        c.write_once(receipts / "workflow-command-started.json", {"command": "CreateWorkflowVersion", "definitionSha256": r.digest(definition)})
        api(s, "/workflow-versions", {"workspaceId": ws, "code": "TRIAL-ACTIVITY", "name": "原三份 RAW 的企业活动加工", "version": "1.0.0", "definitionRef": WORKFLOW, "definitionYaml": definition.decode("utf-8")})
        rows = sql(s, query)
    r.need(len(rows) == 1 and rows[0]["definition_sha256"] == r.digest(definition) and rows[0]["definition_ref"] == WORKFLOW
           and rows[0]["created_by"] == actor, "Frozen workflow source/actor conflict")
    c.write_once(receipts / "frozen-workflow.json", rows[0])
    return {"outputs": outputs, "workflowVersionId": rows[0]["id"], "workflowSha256": r.digest(definition)}


def build(s, directory, attempt):
    binaries = {}
    for role in ("api", "worker"):
        binary = directory / (role + ".exe")
        r.command(["go", "build", "-trimpath", "-o", binary, "./cmd/" + role], cwd=r.ROOT / "apps/platform", timeout=600)
        binaries[role] = {"path": str(binary), "sha256": r.digest(binary.read_bytes())}
    # Next's own distDir support isolates builds from the running production tree.
    dist = ".next-trial-" + attempt
    config_name = dist + "-tsconfig.json"
    with (r.ROOT / "apps/web" / config_name).open("xb") as f:
        f.write((r.ROOT / "apps/web/tsconfig.json").read_bytes())
    env = dict(r.web_environment(s), NEXT_DIST_DIR=dist, NEXT_TSCONFIG_PATH=config_name)
    r.command(["cmd.exe", "/d", "/c", "npm.cmd run build"], cwd=r.ROOT / "apps/web", env=env, timeout=900)
    built = r.ROOT / "apps/web" / dist
    web = directory / "web"
    shutil.copytree(built / "standalone", web)
    shutil.copytree(built / "static", web / dist / "static")
    shutil.copytree(r.ROOT / "apps/web/public", web / "public")
    return {"binaries": binaries, "web": {"root": str(web), "buildId": (built / "BUILD_ID").read_text().strip(),
            "files": {str(p.relative_to(web)): r.digest(p.read_bytes()) for p in web.rglob("*") if p.is_file()}}}


def verify_business(s):
    binding = s["businessBinding"]
    # Windows TEMP/private paths may contain an 8.3 alias. Compare canonical
    # paths on both sides; a long child is not lexically below its short parent.
    path = Path(binding["path"]).resolve()
    private = Path(s["private"]).resolve()
    r.need(path.resolve().is_relative_to(private / "upgrades"), "Upgrade manifest escapes retained private directory")
    r.need(r.digest(path.read_bytes()) == binding["sha256"], "Business upgrade manifest drift")
    manifest = r.read(path)
    r.need(all(manifest[k] == s[k] for k in ("runId", "workspaceId", "actorId", "root")), "Business upgrade identity drift")
    r.need(manifest["sources"] == c.committed_sources(manifest["head"]), "Business source manifest drift")
    for item in manifest["artifacts"]["binaries"].values():
        p = Path(item["path"])
        r.need(p.resolve().is_relative_to(path.parent) and r.digest(p.read_bytes()) == item["sha256"], "Upgraded binary drift")
    web = manifest["artifacts"]["web"]
    root = Path(web["root"]).resolve()
    r.need(root.resolve().is_relative_to(path.parent), "Upgraded Web escapes manifest directory")
    for name, expected in web["files"].items():
        file = root / name
        r.need(file.resolve().is_relative_to(root.resolve()) and r.digest(file.read_bytes()) == expected, "Upgraded production Web drift")
    return manifest


def upgrade(s):
    if "businessBinding" in s:
        return r.status(s)  # A duplicate upgrade is read-only, including business commands.
    binding, definition = target(s)
    attempt = uuid.uuid4().hex
    directory = Path(s["private"]) / "upgrades" / attempt
    receipts = r.STATE / "upgrades" / attempt
    directory.mkdir(parents=True, exist_ok=False); receipts.mkdir(parents=True, exist_ok=False)
    with (receipts / "before-state.bytes").open("xb") as f:
        f.write((r.STATE / "state.json").read_bytes())
    c.write_once(receipts / "before-state.json", copy.deepcopy(s))
    baseline, preserved = r.inspect_facts(s), retained(s)
    c.write_once(receipts / "before-facts.json", baseline)
    c.write_once(receipts / "retained-rows.json", preserved)
    c.write_once(receipts / "started.json", {"attemptId": attempt, "runId": s["runId"], "targetHead": binding["head"], "sourceDelta": binding["deltaPaths"]})
    s["upgrade"] = {"attemptId": attempt, "status": "UPGRADING", "receipts": str(receipts)}
    s["phase"] = "UPGRADING"; r.save(s)
    try:
        print(json.dumps({"phase": "BUILDING_UPGRADE", "runId": s["runId"], "upgradeAttemptId": attempt}), flush=True)
        artifacts = build(s, directory, attempt)
        # Recheck the source after builds and before any business mutation.
        r.need(binding["sources"] == r.sources(), "Source changed during upgrade build")
        business = prerequisites(s, definition, receipts)
        prepared = r.inspect_facts(s)
        raw_equal(baseline, prepared); retained_equal(preserved, retained(s))
        c.write_once(receipts / "prepared-facts.json", prepared)
        token = directory / "human-token"
        with token.open("x", encoding="utf-8") as f: f.write(secrets.token_hex(32))
        s["secretReferences"].append(str(token)); s["privateHashes"][str(token)] = r.digest(token.read_bytes())
        manifest = {"contract": "issue325/business-upgrade-v1", **{k: s[k] for k in ("runId", "workspaceId", "actorId", "root")},
                    **binding, "artifacts": artifacts, "business": business, "humanTokenFile": str(token)}
        path = directory / "manifest.json"; c.write_once(path, manifest)
        c.write_once(receipts / "manifest.json", manifest)
        # Stop the exact old owners before installing a distinct active binding.
        s["phase"] = "RUNNING"; r.save(s)
        r.stop(s)
        s["businessBinding"] = {"path": str(path), "sha256": r.digest(path.read_bytes())}
        s["controllerBinding"] = binding; r.save(s)
        result = c.resume(s)
        after = r.inspect_facts(s)
        r.need(after == prepared, "Business facts changed during activation")
        raw_equal(baseline, after); retained_equal(preserved, retained(s))
        c.write_once(receipts / "after-facts.json", after)
        completed = {"status": "PASS", "attemptId": attempt, "runId": s["runId"], "workspaceId": s["workspaceId"],
                     "actorId": s["actorId"], "businessHead": binding["head"], "sameOriginalRAW": True, "retainedOriginalRows": True,
                     "workflowVersionId": business["workflowVersionId"], "businessProcessing": "NOT_RUN"}
        c.write_once(receipts / "completed.json", completed)
        s["upgrade"]["status"] = "PASS"; r.save(s)
        return {**result, "upgrade": completed}
    except BaseException as e:
        c.write_once(receipts / "failed.json", {"status": "UNKNOWN", "failureType": type(e).__name__, "retained": True})
        s["phase"] = "UNKNOWN"; s["upgrade"]["status"] = "UNKNOWN"; r.save(s)
        raise
