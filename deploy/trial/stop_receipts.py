"""Attempt-scoped stop facts; frozen ledger artifacts are never current slots."""
import uuid
from pathlib import Path

import runtime as r
import bootstrap as b

CONTRACT = "issue325/stop-receipt-v1"


def identity(s):
    return {key: s[key] for key in ("runId", "workspaceId", "actorId", "root")}


def store(s, directory, facts=None, *, reason=None):
    observation = {"contract": CONTRACT, **identity(s), "attemptId": directory.name,
                   "status": "UNKNOWN" if facts is None else "PASS", "facts": None}
    if facts is not None:
        r.need(facts.get("workspaceId") == s["workspaceId"], "Stop facts workspace drift")
        b.put(directory / "facts.json", facts)
        observation["facts"] = b.file(directory / "facts.json", below=r.STATE / "stops")
    else:
        observation["reason"] = reason
    b.put(directory / "observation.json", observation)
    return {"status": observation["status"], "runId": s["runId"], "attemptId": directory.name,
            "observation": b.file(directory / "observation.json", below=r.STATE / "stops"),
            **({"receipt": observation["facts"]["path"], "sha256": observation["facts"]["sha256"]}
               if facts is not None else {})}


def capture(s):
    directory = r.STATE / "stops" / uuid.uuid4().hex
    directory.mkdir(parents=True, exist_ok=False)
    b.put(directory / "started.json", {"contract": CONTRACT, **identity(s), "attemptId": directory.name})
    try:
        facts = r.inspect_facts(s)
    except (r.Refused, OSError, ValueError):
        return store(s, directory, reason="Business facts unavailable; retain prior observations")
    return store(s, directory, facts)


def read(s):
    guard(s)
    bound = s.get("stopInspection", {})
    r.need(bound.get("status") == "PASS", "Successful bound stop receipt required")
    attempt = bound["attemptId"]
    r.need(uuid.UUID(attempt).hex == attempt, "Canonical stop attempt required")
    directory = r.STATE / "stops" / attempt
    observation = b.file(directory / "observation.json", below=r.STATE / "stops")
    r.need(observation == bound["observation"], "Stop observation binding drift")
    value = r.read(observation["path"])
    facts = b.file(directory / "facts.json", below=r.STATE / "stops")
    r.need(value["contract"] == CONTRACT and all(value[k] == s[k] for k in identity(s)) and
           value["attemptId"] == attempt and value["status"] == "PASS" and
           value["facts"] == facts and bound["receipt"] == facts["path"] and
           bound["sha256"] == facts["sha256"] and bound["runId"] == s["runId"],
           "Stop facts identity/hash drift")
    result = r.read(facts["path"])
    r.need(result.get("workspaceId") == s["workspaceId"], "Stop facts workspace drift")
    return result


def guard(s):
    recovery = s.get("stopRecovery", {})
    r.need(not recovery or recovery["status"] == "PASS", "Unfinished stop recovery; use same reviewed plan")
    registered = {Path(v["path"]).resolve() for v in s.get("stopRecoveryHistory", [])}
    root = r.STATE / "stop-recoveries"
    actual = {v.resolve() for v in root.iterdir()} if root.exists() else set()
    r.need(actual == registered, "Unregistered stop recovery journal; preserve for inspection")
