"""Explicit retained-run business prerequisites, separate from host lifecycle."""
import copy
import json
from pathlib import Path
import uuid

import runtime as r
import resume as c
import bootstrap as b
import activation as a

CONTRACT = "issue325/business-prerequisites-v1"
REMEDIATION = "park/remediation/energy-quarantine-v1.yaml"


def configuration(s):
    # Delivery requires a separately reviewed trusted consumer/token binding.
    # This preparation cannot mint it, enable delivery or restart a host.
    return {"api": {"QUALITY_REMEDIATION_PLAN_REF": REMEDIATION},
            "web": {"POC_ENABLE_QUALITY_ACTIONS": "true", "POC_QUALITY_ACTOR_ID": s["actorId"],
                    "POC_ENABLE_COMPLIANCE_ACTIONS": "true", "POC_COMPLIANCE_ACTOR_ID": s["actorId"]}}


def helper(s, tool, arguments):
    return json.loads(r.command([tool, *arguments], env=dict(r.environment(),
                      DPP_TRIAL_CONFIG=str(Path(s["private"]) / "config.json")), timeout=180))


def plan(s, spec, admission, tool):
    # Also covers failures: command() cannot append diagnostics in this context.
    with r.read_only_observation():
        r.need(s["phase"] == "RUNNING" and s.get("activation", {}).get("status") == "PASS",
               "Known activated RUNNING original environment required")
        r.verify(s, source=False, running=True)
        c.verify_origin(s)
        a.verify_binding(s)
        source = b.target(s, r.read(admission))
        private = Path(s["private"])
        specification = b.file(spec, below=private)
        binary = b.file(tool, below=private / "business-tools")
        core = helper(s, binary["path"], ["plan", "-spec", specification["path"]])
        value = core["plan"]["spec"]
        r.need(core["plan"]["contract"] == CONTRACT and value["runId"] == s["runId"]
               and value["workspaceId"] == s["workspaceId"] and value["actorId"] == s["actorId"]
               and value["activationAttemptId"] == s["activation"]["attemptId"], "Core plan identity drift")
        return {"contract": CONTRACT, **{k: s[k] for k in ("runId", "workspaceId", "actorId", "root")},
                "activationBinding": copy.deepcopy(s["activationBinding"]), "source": source,
                "admission": b.file(admission), "specification": specification, "tool": binary,
                "core": core, "configuration": configuration(s)}


def verify_binding(s):
    ref = s["businessPrerequisiteBinding"]
    f = b.file(ref["path"], below=Path(s["private"]) / "business-preparations")
    r.need(f["sha256"] == ref["sha256"], "Business prerequisite binding drift")
    manifest = r.read(f["path"])
    r.need(manifest["contract"] == CONTRACT and manifest["status"] == "PREPARED"
           and all(manifest[k] == s[k] for k in ("runId", "workspaceId", "actorId", "root"))
           and manifest["activationBinding"] == s["activationBinding"]
           and manifest["configuration"] == configuration(s), "Business prerequisite identity/configuration drift")
    result = manifest["receipt"]
    r.need(result["status"] == "PREPARED" and result["contract"] == CONTRACT
           and all(result[k] == s[k] for k in ("runId", "workspaceId", "actorId"))
           and result["principalRef"] == "trial-operator-" + s["runId"]
           and result["deliveryAuthorized"] is False
           and result["planSha256"] == manifest["corePlanSha256"], "Core preparation receipt drift")
    return manifest


def prepare(s, plan_path, expected_sha):
    reviewed = r.read(plan_path)
    r.need(r.digest(b.encoded(reviewed)) == expected_sha, "Reviewed business plan digest mismatch")
    current = plan(s, reviewed["specification"]["path"], reviewed["admission"]["path"], reviewed["tool"]["path"])
    r.need(current == reviewed, "Business plan drift; review a new plan, never reset the original run")
    if s.get("businessPrerequisiteBinding"):
        manifest = verify_binding(s)
        r.need(manifest["planSha256"] == expected_sha, "Completed preparation input conflict")
        return manifest  # No Core commands, new attempts, registry writes or credentials.
    previous = s.get("businessPrerequisites", {})
    r.need(not previous or previous["planSha256"] == expected_sha, "Unfinished preparation input conflict")
    # Every invocation gets an append-only local observation; the same Core
    # fingerprint/audit traces recover committed steps after an unknown result.
    attempt = uuid.uuid4().hex
    root = Path(s["private"]) / "business-preparations" / expected_sha / attempt
    root.mkdir(parents=True, exist_ok=False)
    b.put(root / "started.json", {"planSha256": expected_sha, "attemptId": attempt})
    b.put(root / "core-plan.json", reviewed["core"]["plan"])
    s["businessPrerequisites"] = {"status": "UNKNOWN", "planSha256": expected_sha, "attemptId": attempt}
    r.save(s)
    try:
        result = helper(s, reviewed["tool"]["path"], ["prepare", "-spec", reviewed["specification"]["path"],
                        "-plan", str(root / "core-plan.json"), "-sha256", reviewed["core"]["planSha256"]])
        r.need(result["status"] == "PREPARED" and result["contract"] == CONTRACT
               and all(result[k] == s[k] for k in ("runId", "workspaceId", "actorId"))
               and result["principalRef"] == "trial-operator-" + s["runId"]
               and result["planSha256"] == reviewed["core"]["planSha256"]
               and result["deliveryAuthorized"] is False, "Core prerequisite result mismatch")
        manifest = {"contract": CONTRACT, "status": "PREPARED", **{k: s[k] for k in ("runId", "workspaceId", "actorId", "root")},
                    "activationBinding": s["activationBinding"], "planSha256": expected_sha,
                    "corePlanSha256": reviewed["core"]["planSha256"], "receipt": result,
                    "configuration": reviewed["configuration"], "source": reviewed["source"]}
        b.put(root / "manifest.json", manifest)
        s["businessPrerequisiteBinding"] = b.file(root / "manifest.json", below=Path(s["private"]))
        s["businessPrerequisites"]["status"] = "PASS"
        r.save(s)
        return manifest
    except Exception:
        b.put(root / "unknown.json", {"status": "UNKNOWN", "planSha256": expected_sha,
                                     "recovery": "same plan, same Core command fingerprint; retain this attempt"})
        raise
