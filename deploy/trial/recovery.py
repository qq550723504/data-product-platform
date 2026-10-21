"""Explicit same-attempt recovery of the deployed #336 pre-native checkpoint."""
import copy
import json
from pathlib import Path
import re
import uuid

import bootstrap as b
import runtime as r
import resume as c

CONTRACT = "issue325/bootstrap-pre-native-recovery-v1"
PREDECESSOR = "bfafe8775b48f427aa9d0051b24da7cd78fbc1ba"
PUBLIC = {"before-state.json", "plan.json", "build-started.json", "build-result.json",
          "private-admission-started.json", "private-admission-result.json", "failed.json"}


def members(root, names, recovery=None):
    r.need(root.is_dir() and not root.is_symlink() and not getattr(root, "is_junction", lambda: False)(), "Exact recovery directory required")
    r.need({p.name for p in root.iterdir()} == names | ({"recoveries"} if recovery else set()),
           "Later action, additional member or orphan recovery; recovery refused")
    for name in names:
        path = root / name
        r.need(path.is_file() and not path.is_symlink(), "Regular checkpoint files required")
    if recovery:
        child = root / "recoveries"
        r.need(not child.is_symlink() and not getattr(child, "is_junction", lambda: False)()
               and {p.name for p in child.iterdir()} == {recovery}, "Foreign/orphan recovery")


def checkpoint(s):
    attempt = s.get("bootstrap", {})
    r.need(s["phase"] == "UNKNOWN" and set(attempt) == {"attemptId", "status", "receipts", "planSha256"}
           and attempt["status"] == "UNKNOWN" and re.fullmatch("[0-9a-f]{32}", attempt["attemptId"]),
           "Only an un-recovered UNKNOWN bootstrap checkpoint is recoverable")
    journal = r.STATE / "bootstrap-attempts" / attempt["attemptId"]
    private = Path(s["private"]) / "bootstrap-attempts" / attempt["attemptId"]
    r.need(Path(attempt["receipts"]) == journal, "Foreign bootstrap receipts")
    # Do not admit unregistered sibling attempts in either ownership root.
    for parent in (journal.parent, private.parent):
        r.need(parent.is_dir() and {p.name for p in parent.iterdir()} == {attempt["attemptId"]}, "Foreign/orphan bootstrap attempt")
    members(journal, PUBLIC)
    private_names = {p.name for p in private.iterdir()} if private.is_dir() else set()
    inputs = private_names - {"trial-activate.exe", "migrate.exe"}
    r.need(len(inputs) == 1 and re.fullmatch("[0-9a-f]{32}-input.json", next(iter(inputs))), "Only the old private-mode input and two tools are admitted")
    members(private, {"trial-activate.exe", "migrate.exe"} | inputs)
    r.need(r.read(journal / "failed.json") == {"failureType": "Refused", "retained": True, "status": "UNKNOWN"}, "Unqualified first failure")
    for name in ("build", "private-admission"):
        r.need(r.read(journal / (name + "-started.json")) == {"action": name}
               and r.read(journal / (name + "-result.json")) == {"status": "PASS"}, "Required pre-native checkpoint missing")
    before, old = r.read(journal / "before-state.json"), r.read(journal / "plan.json")
    r.need(before["phase"] in ("RUNNING", "STOPPED") and "bootstrap" not in before
           and {k: v for k, v in s.items() if k not in ("phase", "bootstrap")} == {k: v for k, v in before.items() if k != "phase"},
           "Original pre-attempt facts changed")
    r.need(not any(s.get(k) for k in ("bootstrapBinding", "custody", "custodyBinding", "activation", "activationBinding")), "Later lifecycle is not recoverable")
    r.need(old["contract"] == b.CONTRACT and old["stage"] == "bootstrap" and r.digest(b.encoded(old)) == attempt["planSha256"]
           and old["target"]["head"] == PREDECESSOR and old["target"]["sources"] == c.committed_sources(PREDECESSOR),
           "Only the deployed #336 original plan/source is recoverable")
    admission = old["target"]["admission"]
    r.need(admission.get("head") == PREDECESSOR and type(admission.get("ciRunId")) is int and admission["ciRunId"] > 0,
           "Original exact merge/main-CI admission required")
    r.need(all(old[k] == s[k] for k in ("root", "runId", "workspaceId", "actorId"))
           and old["origin"] == s["originSnapshot"] and old["previousBusinessBinding"] == s["businessBinding"], "Original identity/binding changed")
    r.need(old["registry"]["path"] == str(r.STATE / "state.json") and old["expectedSchema"] == {"from": 68, "to": 71}, "Original registry/schema contract drift")
    config = Path(s["private"]) / "config.json"
    r.need(r.read(private / next(iter(inputs))) == {"configFile": str(config)} and old["config"] == b.file(config), "Original private-mode input/config changed")
    for name in ("facts", "admissionFile"):
        r.need(b.file(old[name]["path"]) == old[name], "Original plan input changed")
    r.need(r.read(old["admissionFile"]["path"]) == old["target"]["admission"], "Original source admission changed")
    r.need(old["versions"] == b.version_facts(s, r.read(old["facts"]["path"]))[0]
           and old["migrations"] == b.migrations()
           and old["migrationVersions"] == [int(n[:6]) for n in old["migrations"] if n.endswith(".up.sql")], "Original five-version/migration facts changed")
    return journal, private, private_names, old


def plan(s, admission):
    with r.read_only_observation():
        journal, private, private_names, old = checkpoint(s)
        registry = b.file(r.STATE / "state.json")
        r.need(r.read(registry["path"]) == s, "Registry changed during recovery plan")
        r.verify(s, source=False, running=False); c.verify_origin(s)
        native = b.native(s, hosts=True)
        r.need(native == old["native"], "Original native checkpoint drift")
        target = b.target(s, r.read(admission))
        files = [b.file(journal / n, below=journal) for n in sorted(PUBLIC)]
        files += [b.file(private / n, below=private) for n in sorted(private_names)]
        files += [b.file(old[n]["path"]) for n in ("facts", "config", "admissionFile")]
        new_admission = b.file(admission); files.append(new_admission)
        effective = copy.deepcopy(old)
        effective.update(registry=registry, target=target, admissionFile=new_admission)
        effective["recovery"] = {"attemptId": s["bootstrap"]["attemptId"], "originalPlanSha256": s["bootstrap"]["planSha256"],
                                 "firstFailure": b.file(journal / "failed.json")}
        result = {"contract": CONTRACT, **{k: s[k] for k in ("root", "runId", "workspaceId", "actorId")},
                  "registry": registry, "attempt": copy.deepcopy(s["bootstrap"]), "journal": str(journal), "private": str(private),
                  "privateMembers": sorted(private_names), "inputs": files, "admissionFile": new_admission, "bootstrapPlan": effective}
        r.need(b.file(registry["path"]) == registry, "Registry changed during recovery plan")
        return result


def run(s, path, digest):
    expected = r.read(path)
    r.need(expected.get("contract") == CONTRACT and r.digest(b.encoded(expected)) == digest, "Reviewed recovery plan/digest required")
    recovery = s.get("bootstrap", {}).get("recovery", {})
    if recovery.get("status") == "PASS" and s.get("bootstrapBinding"):
        with r.read_only_observation():
            accepted = b.binding(s, "bootstrap")
            r.need(accepted.get("recoveryPlanSha256") == digest and recovery["planSha256"] == digest, "Different accepted recovery")
            r.verify(s, source=False, running=False); c.verify_origin(s); b.native(s)
        return {"status": "PASS", "replay": True, "attemptId": s["bootstrap"]["attemptId"], "binding": s["bootstrapBinding"]}
    actual = plan(s, expected["admissionFile"]["path"])
    r.need(actual == expected, "Recovery admission drift; review a new plan")
    return execute(s, expected, digest)


def execute(s, expected, digest):
    from activation import tool
    identity = uuid.uuid4().hex
    journal, private = Path(expected["journal"]), Path(expected["private"])
    receipts, directory = journal / "recoveries" / identity, private / "recoveries" / identity
    # A failure between these allocations and the registry write is an orphan;
    # the exact-member checkpoint refuses it rather than adopting it.
    receipts.mkdir(parents=True, exist_ok=False); directory.mkdir(parents=True, exist_ok=False)
    b.put(receipts / "before-recovery-state.json", s); b.put(receipts / "recovery-plan.json", expected)
    s["bootstrap"]["recovery"] = {"recoveryId": identity, "status": "IN_PROGRESS", "receipts": str(receipts), "planSha256": digest}
    s["phase"] = "LEDGER_STAGE"; r.save(s)
    effective = expected["bootstrapPlan"]
    try:
        tools = b.action(receipts, "build", lambda: b.build(directory, ("trial-activate", "migrate")))
        private_proof = b.action(receipts, "private-admission", lambda: json.loads(tool(s, directory, "trial-activate", "private")))
        r.need(private_proof == {"status": "PASS", "privateACL": True}, "Fresh private ACL admission required")
        with r.read_only_observation():
            r.verify(s, source=False, running=False); c.verify_origin(s)
            r.need(r.read(r.STATE / "state.json") == s, "Recovery registry drift")
            r.need(b.target(s, effective["target"]["admission"]) == effective["target"], "Recovery source changed during build")
            r.need(b.native(s, hosts=True) == effective["native"] and b.migrations() == effective["migrations"], "Recovery native/migration drift")
            members(journal, PUBLIC, identity); members(private, set(expected["privateMembers"]), identity)
            for item in expected["inputs"]:
                r.need(b.file(item["path"]) == item, "Frozen recovery input changed during build")
        b.put(receipts / "readmission.json", {"status": "PASS", "firstFailure": effective["recovery"]["firstFailure"], "originalPlanSha256": s["bootstrap"]["planSha256"]})
        files = list(tools.values()) + expected["inputs"] + [b.file(receipts / n) for n in ("before-recovery-state.json", "recovery-plan.json", "readmission.json")]
        proof = b.bootstrap_work(s, effective, directory, receipts, files)
        b.stop_core(s, receipts, proof, files)
        resolution = {"contract": CONTRACT, "status": "PASS", "attemptId": s["bootstrap"]["attemptId"], "recoveryId": identity,
                      "originalPlanSha256": s["bootstrap"]["planSha256"], "firstFailure": effective["recovery"]["firstFailure"],
                      "recoveryPlanSha256": digest, "acceptedBootstrapPlanSha256": r.digest(b.encoded(effective)), "coreAdmission": b.file(receipts / "core-admission.json")}
        b.put(receipts / "resolution.json", resolution); files.append(b.file(receipts / "resolution.json"))
        manifest = {"contract": b.CONTRACT, "stage": "bootstrap", "status": "PASS", **{k: s[k] for k in ("root", "runId", "workspaceId", "actorId")},
                    "previousBusinessBinding": s["businessBinding"], "origin": s["originSnapshot"], "target": effective["target"],
                    "plan": effective, "files": files, "coreAdmission": proof, "recoveryPlanSha256": digest}
        manifest_path = directory / "manifest.json"; b.put(manifest_path, manifest)
        s["bootstrapBinding"] = b.file(manifest_path)
        s["bootstrap"]["status"] = s["bootstrap"]["recovery"]["status"] = "PASS"
        s["phase"] = "STOPPED"; r.save(s)
        return {"status": "PASS", "attemptId": s["bootstrap"]["attemptId"], "recoveryId": identity,
                "acceptedBootstrapPlanSha256": resolution["acceptedBootstrapPlanSha256"], "binding": s["bootstrapBinding"]}
    except BaseException as error:
        b.put(receipts / "failed.json", {"status": "UNKNOWN", "failureType": type(error).__name__, "retained": True})
        s["bootstrap"]["status"] = s["bootstrap"]["recovery"]["status"] = "UNKNOWN"
        s["phase"] = "UNKNOWN"; r.save(s)
        raise
