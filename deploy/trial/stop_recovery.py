"""Explicit exact-byte repair of the one observed legacy stop/ledger collision."""
import copy
import os
import uuid
from pathlib import Path

import runtime as r
import resume as c
import bootstrap as b
import activation as a
import protected_resume as p
import stop_receipts as stops

CONTRACT = "issue325/stop-recovery-v1"


def controller(s):
    head = r.command(["git", "rev-parse", "HEAD"]).decode().strip()
    sources = r.sources()
    r.need(c.committed_sources(head) == sources, "Committed recovery controller required")
    # Full activation validation rejects the known historical stop collision.
    # Verify its frozen business/source baseline here; plan() independently
    # validates every ledger file and admits only the exact legacy collision.
    c.check_controller_delta(a.binding_document(s)["sources"], sources)
    return {"head": head, "sources": sources}


def check_ledger(s, current, historical):
    """Validate every ledger artifact, allowing only the reviewed target states."""
    frozen = []
    for stage in b.STAGES:
        for artifact in b.binding_document(s, stage)["files"]:
            actual = b.file(artifact["path"])
            if actual["path"] == current["path"]:
                r.need(stage == "bootstrap" and current["sha256"] != historical["sha256"] == artifact["sha256"]
                       and actual["sha256"] in (current["sha256"], artifact["sha256"]),
                       "Unrelated ledger artifact drift; cannot recover")
                frozen.append(artifact)
            else:
                r.need(actual["sha256"] == artifact["sha256"], "Unrelated ledger artifact drift; cannot recover")
    r.need(len(frozen) == 1, "Exactly one frozen bootstrap artifact collision required")
    return frozen[0]


def plan(s, historical):
    with r.read_only_observation():
        r.need(s["phase"] == "STOPPED" and s["root"] == str(r.ROOT) and
               s.get("activation", {}).get("status") == "PASS" and not s.get("stopRecovery"),
               "Known activated STOPPED run without recovery required")
        stops.guard(s); b.guard(s)
        r.need(r.local_docker_identity()["daemonId"] == s["host"]["daemonId"], "Docker daemon drift")
        native = b.stopped_native(s)
        source = controller(s)
        registry = b.file(r.STATE / "state.json")
        r.need(r.read(registry["path"]) == s, "Recovery registry drift")
        target = b.file(r.STATE / "before-stop-facts.json", below=r.STATE)
        current = s.get("stopInspection", {})
        r.need(current.get("status") == "PASS" and current.get("runId") == s["runId"] and
               Path(current.get("receipt", "")).resolve() == Path(target["path"]),
               "Only the observed legacy stop receipt collision can recover")
        original = b.file(historical, below=r.STATE)
        r.need(original["path"] != target["path"], "Distinct retained historical copy required")
        frozen = check_ledger(s, target, original)
        activation = b.file(s["activationBinding"]["path"], below=Path(s["private"]) / "activations")
        r.need(activation == s["activationBinding"], "Activation manifest drift")
        accepted = r.read(activation["path"])
        r.need(all(accepted[k] == s[k] for k in stops.identity(s)) and
               accepted["ledger"]["bootstrap"] == s["bootstrapBinding"] and
               accepted["ledger"]["custody"] == s["custodyBinding"], "Accepted activation ledger drift")
        facts = r.read(target["path"]); p.validate(s, facts)
        r.need(facts["expected"] == accepted["ledger"]["proof"], "Current stop facts do not match accepted activation")
        return {"contract": CONTRACT, **stops.identity(s), "registry": registry,
                "controller": source, "native": native,
                "historical": original, "current": target, "frozen": frozen,
                "bootstrapBinding": copy.deepcopy(s["bootstrapBinding"]),
                "custodyBinding": copy.deepcopy(s["custodyBinding"]),
                "activationBinding": copy.deepcopy(s["activationBinding"])}


def write_bytes(path, data):
    with Path(path).open("xb") as output:
        output.write(data); output.flush(); os.fsync(output.fileno())


def restore(target, historical, current_sha, expected_sha, folder):
    actual = b.file(target, below=r.STATE)
    r.need(actual["sha256"] in (current_sha, expected_sha), "Recovery target drift; do not overwrite")
    if actual["sha256"] == expected_sha:
        return
    data = Path(historical).read_bytes()
    r.need(r.digest(data) == expected_sha, "Historical restoration bytes drift")
    temporary = folder / "restore-payload.tmp"
    if temporary.exists():
        r.need(temporary.read_bytes() == data, "Partial restoration payload drift")
    else:
        write_bytes(temporary, data)
    r.need(b.file(target)["sha256"] == current_sha, "Recovery target changed before restoration")
    os.replace(temporary, target)
    r.need(b.file(target)["sha256"] == expected_sha, "Restored historical bytes mismatch")


def run(s, plan_path, expected_sha):
    reviewed = r.read(plan_path)
    r.need(r.digest(b.encoded(reviewed)) == expected_sha and reviewed["contract"] == CONTRACT and
           all(reviewed[k] == s[k] for k in stops.identity(s)), "Exact reviewed stop recovery required")
    folder = r.STATE / "stop-recoveries" / expected_sha
    receipt = folder / "manifest.json"
    previous = s.get("stopRecovery", {})
    r.need(not previous or previous["planSha256"] == expected_sha and previous["path"] == str(folder),
           "Stop recovery claim conflict")
    if previous.get("status") == "PASS":
        r.need(b.file(receipt) == previous["binding"], "Recovery manifest drift")
        result = r.read(receipt)
        r.need(result["planSha256"] == expected_sha and
               b.file(reviewed["frozen"]["path"])["sha256"] == reviewed["frozen"]["sha256"], "Completed restoration drift")
        stops.read(dict(s, stopInspection=result["stopInspection"]))
        return result
    r.need(s["phase"] == "STOPPED", "Stop recovery cannot operate a running/unknown environment")
    with r.read_only_observation():
        r.need(controller(s) == reviewed["controller"], "Recovery controller drift")
        # A saved journal skips plan(), but never the ledger precondition.
        # A prior attempt may already have restored the one reviewed target.
        r.need(check_ledger(s, reviewed["current"], reviewed["historical"]) == reviewed["frozen"],
               "Reviewed frozen ledger artifact drift")
        r.need(r.local_docker_identity()["daemonId"] == s["host"]["daemonId"] and
               b.stopped_native(s) == reviewed["native"], "Stopped native identity drift")
    if not folder.exists():
        r.need(plan(s, reviewed["historical"]["path"]) == reviewed, "Stop recovery plan drift")
        folder.mkdir(parents=True, exist_ok=False)
        b.put(folder / "before-state.json", s)
        b.put(folder / "plan.json", reviewed)
        b.put(folder / "started.json", {"planSha256": expected_sha})
    r.need(r.read(folder / "plan.json") == reviewed and
           r.read(folder / "started.json") == {"planSha256": expected_sha}, "Unrecognized recovery journal")
    before = r.read(folder / "before-state.json")
    r.need({k:v for k,v in s.items() if k not in {"stopRecovery", "stopInspection", "stopRecoveryHistory"}} ==
           {k:v for k,v in before.items() if k not in {"stopRecovery", "stopInspection", "stopRecoveryHistory"}},
           "Unrelated retained state drift during recovery")
    registered = s.get("stopRecoveryHistory", [])
    r.need(registered == before.get("stopRecoveryHistory", []) and
           s["stopInspection"] == before["stopInspection"], "Recovery current binding/history drift")
    if not previous:
        s["stopRecovery"] = {"status": "UNKNOWN", "planSha256": expected_sha, "path": str(folder)}
        r.save(s)
    for name, source in (("current-facts.json", reviewed["current"]), ("historical-facts.json", reviewed["historical"])):
        destination = folder / name
        if not destination.exists():
            r.need(b.file(source["path"])["sha256"] == source["sha256"], "Recovery source drift")
            write_bytes(destination, Path(source["path"]).read_bytes())
        r.need(b.file(destination)["sha256"] == source["sha256"], "Recovery archived facts drift")
    directory = r.STATE / "stops" / uuid.uuid5(uuid.NAMESPACE_URL, expected_sha).hex
    if not directory.exists(): directory.mkdir(parents=True, exist_ok=False)
    if not (directory / "observation.json").exists():
        facts = r.read(folder / "current-facts.json")
        if (directory / "facts.json").exists():
            r.need(r.read(directory / "facts.json") == facts, "Partial recovered stop facts drift")
            # Complete the same observation after a crash following the facts write.
            value = {"contract": stops.CONTRACT, **stops.identity(s), "attemptId": directory.name,
                     "status": "PASS", "facts": b.file(directory / "facts.json")}
            b.put(directory / "observation.json", value)
        else:
            stops.store(s, directory, facts)
    observation = r.read(directory / "observation.json")
    stop_binding = {"status": "PASS", "runId": s["runId"], "attemptId": directory.name,
                    "observation": b.file(directory / "observation.json"),
                    "receipt": observation["facts"]["path"], "sha256": observation["facts"]["sha256"]}
    candidate = dict(s, stopInspection=stop_binding, stopRecoveryHistory=[*registered, {"path": str(folder)}],
                     stopRecovery={"status": "PASS"})
    facts = stops.read(candidate); p.validate(s, facts)
    r.need(facts == r.read(folder / "current-facts.json"), "Recovered stop receipt differs from reviewed facts")
    restore(reviewed["frozen"]["path"], folder / "historical-facts.json",
            reviewed["current"]["sha256"], reviewed["frozen"]["sha256"], folder)
    b.binding(s, "bootstrap"); b.binding(s, "custody")
    result = {"contract": CONTRACT, "status": "PASS", **stops.identity(s),
              "planSha256": expected_sha, "restored": reviewed["frozen"], "stopInspection": stop_binding}
    if receipt.exists(): r.need(r.read(receipt) == result, "Recovery result drift")
    else: b.put(receipt, result)
    s["stopInspection"] = stop_binding
    s["stopRecovery"] = {"status": "PASS", "planSha256": expected_sha, "path": str(folder), "binding": b.file(receipt)}
    s.setdefault("stopRecoveryHistory", []).append({"path": str(folder), "planSha256": expected_sha, "binding": b.file(receipt)})
    r.save(s)
    return result
