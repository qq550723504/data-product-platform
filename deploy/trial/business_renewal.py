"""Explicit finite rights successors; original preparation/configuration stay bound."""
import copy
import json
from pathlib import Path
import uuid

import runtime as r
import resume as c
import bootstrap as b
import activation as a
import business

CONTRACT = "issue325/business-rights-renewal-v1"


def receipt_objects(original, result):
    old, new = original["receipt"]["objects"], result["objects"]
    reused = {"contractVersion", "profile"}
    r.need(set(old) == set(new) and len(new) == 12 and
           all(str(uuid.UUID(v)) == v and uuid.UUID(v).int != 0 for v in new.values()) and
           len(set(new.values())) == len(new) and
           all(new[k] == old[k] for k in reused) and
           not (set(new[k] for k in new.keys() - reused) & set(old.values())),
           "Exact new rights identities and reused contract/profile required")


def verify_binding(s):
    original = business.verify_binding(s)
    ref = s["businessRightsRenewalBinding"]
    bound = b.file(ref["path"], below=Path(s["private"]) / "business-rights-renewals")
    r.need(bound["sha256"] == ref["sha256"], "Rights renewal manifest drift")
    manifest = r.read(bound["path"])
    receipt = manifest["receipt"]
    r.need(manifest["contract"] == CONTRACT and manifest["status"] == "RENEWED" and
           all(manifest[k] == s[k] for k in ("runId", "workspaceId", "actorId", "root")) and
           manifest["preparationBinding"] == s["businessPrerequisiteBinding"] and
           receipt["contract"] == CONTRACT and receipt["status"] == "RENEWED" and
           all(receipt[k] == s[k] for k in ("runId", "workspaceId", "actorId")) and
           receipt["principalRef"] == "trial-operator-" + s["runId"] and
           receipt["deliveryAuthorized"] is False and receipt["planSha256"] == manifest["corePlanSha256"] and
           receipt["renewalId"] == manifest["renewalId"] and
           all(receipt["objects"][k] == original["receipt"]["objects"][k] for k in ("contractVersion", "profile")),
           "Rights renewal identity/reused coverage drift")
    receipt_objects(original, receipt)
    folder = Path(bound["path"]).parent
    core = r.read(folder / "core-plan.json")
    reviewed = r.read(folder / "plan.json")
    # Go's digest is returned by the admitted helper; compare all references and
    # the original saved plan rather than inventing a second JSON encoding.
    r.need(r.digest(b.encoded(reviewed)) == manifest["planSha256"] and
           reviewed["core"]["planSha256"] == manifest["corePlanSha256"] and
           core == reviewed["core"]["plan"] and
           core["spec"]["renewalId"] == receipt["renewalId"] and
           core["parent"]["planSha256"] == receipt["parentPlanSha256"] and
           core["preparationBinding"] == s["businessPrerequisiteBinding"], "Bound renewal plan drift")
    return manifest


def plan(s, spec, admission, tool):
    with r.read_only_observation():
        r.need(s["phase"] == "RUNNING" and s.get("activation", {}).get("status") == "PASS",
               "Same activated RUNNING environment required")
        r.verify(s, source=False, running=True)
        c.verify_origin(s)
        a.verify_binding(s)
        business.verify_binding(s)
        if s.get("businessRightsRenewalBinding"):
            verify_binding(s)
        source = b.target(s, r.read(admission))
        private = Path(s["private"])
        specification = b.file(spec, below=private)
        binary = b.file(tool, below=private / "business-tools")
        core = business.helper(s, binary["path"], ["renewal-plan", "-spec", specification["path"]])
        value = core["plan"]["spec"]
        r.need(core["plan"]["contract"] == CONTRACT and value["contract"] == CONTRACT and
               all(value[k] == s[k] for k in ("runId", "workspaceId", "actorId")) and
               str(uuid.UUID(value["renewalId"])) == value["renewalId"] and
               core["plan"]["preparationBinding"] == s["businessPrerequisiteBinding"],
               "Core renewal scope/identity drift")
        return {"contract": CONTRACT, **{k: s[k] for k in ("runId", "workspaceId", "actorId", "root")},
                "preparationBinding": copy.deepcopy(s["businessPrerequisiteBinding"]),
                "parentBinding": copy.deepcopy(s.get("businessRightsRenewalBinding")),
                "source": source, "admission": b.file(admission), "specification": specification,
                "tool": binary, "core": core}


def journal_members(s):
    root = Path(s["private"]) / "business-rights-renewals"
    actual = {str(p.parent) for p in root.glob("*/*/started.json")} if root.exists() else set()
    registered = {v["path"] for v in s.get("businessRightsRenewalAttempts", [])}
    r.need(actual == registered, "Orphan rights renewal journal; preserve it for explicit inspection")


def renew(s, plan_path, expected_sha):
    reviewed = r.read(plan_path)
    r.need(r.digest(b.encoded(reviewed)) == expected_sha, "Reviewed renewal plan digest mismatch")
    if s.get("businessRightsRenewalBinding"):
        completed = verify_binding(s)
        if completed["renewalId"] == reviewed["core"]["plan"]["spec"]["renewalId"]:
            r.need(completed["planSha256"] == expected_sha, "Completed renewal input conflict")
            return completed  # Historical receipt only; no Commands/reactivation.
    journal_members(s)
    previous = s.get("businessRightsRenewal", {})
    r.need(not previous or previous["status"] == "PASS" or previous["planSha256"] == expected_sha,
           "Unfinished rights renewal requires the same reviewed plan")
    current = plan(s, reviewed["specification"]["path"], reviewed["admission"]["path"], reviewed["tool"]["path"])
    r.need(current == reviewed, "Rights renewal plan/predecessor drift")
    renewal_id = reviewed["core"]["plan"]["spec"]["renewalId"]
    attempt = uuid.uuid4().hex
    folder = Path(s["private"]) / "business-rights-renewals" / renewal_id / attempt
    folder.mkdir(parents=True, exist_ok=False)
    b.put(folder / "started.json", {"planSha256": expected_sha, "renewalId": renewal_id, "attemptId": attempt})
    b.put(folder / "plan.json", reviewed)
    b.put(folder / "core-plan.json", reviewed["core"]["plan"])
    s.setdefault("businessRightsRenewalAttempts", []).append({"path": str(folder), "planSha256": expected_sha,
                                                            "renewalId": renewal_id, "attemptId": attempt})
    s["businessRightsRenewal"] = {"status": "UNKNOWN", "planSha256": expected_sha,
                                  "renewalId": renewal_id, "attemptId": attempt}
    r.save(s)
    try:
        result = business.helper(s, reviewed["tool"]["path"], ["renew-rights", "-spec", reviewed["specification"]["path"],
                                  "-plan", str(folder / "core-plan.json"), "-sha256", reviewed["core"]["planSha256"]])
        original = business.verify_binding(s)
        r.need(result["contract"] == CONTRACT and result["status"] == "RENEWED" and
               all(result[k] == s[k] for k in ("runId", "workspaceId", "actorId")) and
               result["principalRef"] == "trial-operator-" + s["runId"] and result["deliveryAuthorized"] is False and
               result["renewalId"] == renewal_id and result["planSha256"] == reviewed["core"]["planSha256"] and
               result["parentPlanSha256"] == reviewed["core"]["plan"]["parent"]["planSha256"] and
               all(result["objects"][k] == original["receipt"]["objects"][k] for k in ("contractVersion", "profile")),
               "Core renewal receipt/reused coverage drift")
        receipt_objects(original, result)
        manifest = {"contract": CONTRACT, "status": "RENEWED", **{k: s[k] for k in ("runId", "workspaceId", "actorId", "root")},
                    "renewalId": renewal_id, "planSha256": expected_sha, "corePlanSha256": reviewed["core"]["planSha256"],
                    "preparationBinding": copy.deepcopy(s["businessPrerequisiteBinding"]),
                    "parentBinding": copy.deepcopy(reviewed["parentBinding"]), "receipt": result, "source": reviewed["source"]}
        b.put(folder / "manifest.json", manifest)
        s["businessRightsRenewalBinding"] = b.file(folder / "manifest.json", below=Path(s["private"]))
        s["businessRightsRenewal"]["status"] = "PASS"
        r.save(s)
        return manifest
    except Exception:
        b.put(folder / "unknown.json", {"status": "UNKNOWN", "planSha256": expected_sha,
                                       "recovery": "same reviewed window/UUID/predecessor; Core audit restoration only"})
        raise
