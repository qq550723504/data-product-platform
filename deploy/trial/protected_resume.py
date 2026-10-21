"""Bounded continuation of an accepted protected activation; no new Core semantics."""
from collections import Counter
from decimal import Decimal
from datetime import datetime
import copy
import json
from pathlib import Path
import uuid

import runtime as r
import resume as c

CONTRACT = "issue325/protected-resume-v1"
OUTBOX_MUTABLE = {"status", "attempts", "claimed_at", "claimed_by", "claim_token", "last_error", "available_at", "published_at"}


def active(s):
    if not s.get("activationBinding"):
        return False
    # The explicit activation's historical final validator owns its own path.
    if s.get("phase") == "ACTIVATING" and s.get("activation", {}).get("status") == "ACTIVATING":
        return False
    r.need(s.get("activation", {}).get("status") == "PASS", "Unfinished protected activation; no legacy inspection fallback")
    return True


def tool(s):
    """Only operator commands call this; advisory plans never build or allocate."""
    with r.read_only_observation():
        head = r.command(["git", "rev-parse", "HEAD"]).decode().strip()
        r.need(c.committed_sources(head) == r.sources(), "Committed protected observer source required")
        c.check_sources(s, r.sources())
    old = s.get("protectedObserver")
    if old and old["head"] == head:
        path = Path(old["path"])
        r.need(path.is_relative_to(Path(s["private"]) / "resume-observations") and r.digest(path.read_bytes()) == old["sha256"], "Protected observer binding drift")
        return path
    folder = Path(s["private"]) / "resume-observations" / uuid.uuid4().hex
    folder.mkdir(parents=True, exist_ok=False)
    path = folder / "trial-activate.exe"
    c.write_once(folder / "build-started.json", {"head": head})
    r.command(["go", "build", "-trimpath", "-o", path, "./cmd/trial-activate"], cwd=r.ROOT / "apps/platform", timeout=600)
    value = {"head": head, "path": str(path), "sha256": r.digest(path.read_bytes())}
    c.write_once(folder / "build-result.json", value)
    s["protectedObserver"] = value
    s["binaries"]["protected-observer-" + folder.name] = {k: value[k] for k in ("path", "sha256")}
    r.save(s)
    return path


def invoke(s, mode, **values):
    binary = tool(s)
    path = binary.parent / (uuid.uuid4().hex + "-input.json")
    c.write_once(path, {"configFile": str(Path(s["private"]) / "config.json"), **values})
    return json.loads(r.command([binary, mode], env=dict(r.environment(), DPP_TRIAL_ACTIVATION_INPUT=str(path)), timeout=300))


def rows(observation):
    return {name: [json.loads(row, parse_float=Decimal) for row in table["rows"]]
            for name, table in observation["inventory"]["tables"].items()}


def identity(s):
    return {"runId": s["runId"], "workspaceId": s["workspaceId"], "actorId": s["actorId"], "principalRef": "trial-operator-" + s["runId"]}


def expected(s):
    from activation import verify_binding
    return copy.deepcopy(verify_binding(s)["ledger"]["proof"])


def validate(s, value):
    r.need(value.get("contract") == CONTRACT and value.get("identity") == identity(s), "Matching protected SQL baseline required")
    data = rows(value)
    versions = value["expected"]["versions"]
    r.need(len(versions) == 5 and len({v["id"] for v in versions}) == 5, "Five original protected versions required")
    datasets = {v["id"]: v for v in data["dataset"] if v["workspace_id"] == s["workspaceId"]}
    actual = {v["id"]: v for v in data["dataset_version"] if v["dataset_id"] in datasets}
    r.need(value["expected"]["schema"] == 71 and sorted(v["version"] for v in data["schema_migration"]) == value["expected"]["schemaVersions"], "Exact accepted schema71 history required")
    r.need(set(actual) == {v["id"] for v in versions}, "Original version membership drift")
    r.need(Counter(datasets[v["dataset_id"]]["dataset_type"] for v in actual.values()) == {"RAW": 3, "STANDARDIZED": 1, "CURATED": 1}, "Original version kinds required")
    for v in versions:
        row = actual[v["id"]]
        r.need(row["status"] == "READY" and row["checksum_algorithm"] == "SHA256" and
               (row["checksum_value"], row["byte_size"], row["storage_uri"]) == (v["sha256"], v["bytes"], v["uri"]), "Original immutable version drift")
    return data


def observe(s):
    proof = expected(s)
    inventory = invoke(s, "snapshot")
    # Source data is independent of Core. Use its existing native PostgreSQL CLI,
    # a repeatable-read READ ONLY transaction, and fixed table names only.
    query = "SELECT jsonb_build_object(" + ",".join("'" + name + "',(SELECT jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text) FROM dpp_source." + name + " t)" for name in ("enterprise", "lease", "energy")) + ")"
    with r.read_only_observation():
        source = r.command(["docker", "exec", "-i", s["containers"]["source"]["id"], "psql", "-X", "-qAt", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "dpp_ingestion_source"], data=("BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;\n" + query + ";\nCOMMIT;\n").encode()).decode().strip()
    r.need({k: len(v) for k,v in json.loads(source).items()} == {"enterprise":6,"lease":10,"energy":14}, "Original source rows required")
    value = {"contract": CONTRACT, "workspaceId": s["workspaceId"], "identity": identity(s), "expected": proof, "inventory": inventory, "source": source}
    validate(s, value)
    return value


def additions(before, after, name):
    old = Counter(before["inventory"]["tables"][name]["rows"])
    new = Counter(after["inventory"]["tables"][name]["rows"])
    r.need(not old - new, "Original " + name + " historical rows changed")
    return [json.loads(v, parse_float=Decimal) for v in (new - old).elements()]


def compare(s, before, after, *, probe=False):
    old, new = validate(s, before), validate(s, after)
    r.need(before["identity"] == after["identity"] and before["expected"] == after["expected"] and before["source"] == after["source"], "Protected identity/source facts drift")
    bt, at = before["inventory"]["tables"], after["inventory"]["tables"]
    r.need(set(bt) == set(at) and all(bt[t]["columns"] == at[t]["columns"] for t in bt), "Original table/column inventory drift")
    old_events = {v["id"]: v for v in old["outbox_event"]}
    new_events = {v["id"]: v for v in new["outbox_event"]}
    r.need(set(old_events) <= set(new_events), "Original Outbox event removed")
    for key, value in old_events.items():
        r.need({k:v for k,v in value.items() if k not in OUTBOX_MUTABLE} == {k:v for k,v in new_events[key].items() if k not in OUTBOX_MUTABLE}, "Frozen Outbox content drift")
        if value != new_events[key]:
            current = new_events[key]
            r.need(value["status"] in ("PENDING", "PROCESSING") and current["status"] in ("PROCESSING", "PUBLISHED") and value["attempts"] <= current["attempts"] <= value["attempts"] + 1 and current["last_error"] is None and current["claimed_at"] and current["claim_token"] and (current["published_at"] or current["status"] == "PROCESSING") and current["claimed_by"] == "outbox-dispatcher", "Unverified Outbox projection change")
    # Delivery bookkeeping may progress; it cannot create or edit business facts.
    consumption = additions(before, after, "outbox_event_consumption")
    for v in consumption:
        event = new_events.get(v["event_id"], {})
        r.need(v["consumer_name"] in event.get("required_handlers", []), "Unbound Outbox consumption")
    allowed = {"outbox_event", "outbox_event_consumption"}
    delta = {}
    if probe:
        allowed |= {"lifecycle_storage_call", "lifecycle_storage_observation", "cost_event", "cost_allocation", "evidence", "evidence_relation", "audit_event"}
        delta = {name: additions(before, after, name) for name in allowed if name not in {"outbox_event", "outbox_event_consumption"}}
        calls = delta["lifecycle_storage_call"]
        intents = {v["id"]: v for v in old["lifecycle_protected_object_intent"]}
        versions = {v["id"]: v for v in before["expected"]["versions"]}
        r.need(len(calls) == 10 and len({v["id"] for v in calls}) == 10, "Exactly five HEAD and five GET physical calls required")
        seen = set()
        for call in calls:
            intent = intents.get(call["protected_object_intent_id"], {})
            version = versions.get(intent.get("dataset_version_id"), {})
            r.need(call["operation"] in ("PROTECTED_HEAD", "PROTECTED_GET") and call["workspace_id"] == s["workspaceId"] and call["actor_id"] == s["actorId"] and call["principal_ref"] == identity(s)["principalRef"] and call.get("copy_id") is None and call.get("reader_pin_id") is None and version and intent["workspace_id"] == s["workspaceId"] and intent["content_sha256"] == version["sha256"] and intent["byte_size"] == version["bytes"], "Foreign or unbound protected call")
            key = (intent["dataset_version_id"], call["operation"])
            r.need(key not in seen, "Repeated protected version operation")
            seen.add(key)
        observations = delta["lifecycle_storage_observation"]
        costs = delta["cost_event"]
        allocations = delta["cost_allocation"]
        ids = {v["id"] for v in calls}
        r.need(len(observations) == len(costs) == len(allocations) == 10 and {v["call_id"] for v in observations} == ids and {v["activity_id"] for v in costs} == ids and {v["lifecycle_storage_call_id"] for v in allocations} == ids and {v["cost_event_id"] for v in allocations} == {v["id"] for v in costs}, "Ten observed typed costs required")
        r.need(all(all(value is None for key,value in v.items() if key.endswith("_id") and key not in {"cost_event_id", "lifecycle_storage_call_id"}) for v in allocations), "Cost allocated to unrelated business subject")
        for observation in observations:
            call = next(v for v in calls if v["id"] == observation["call_id"])
            intent = intents[call["protected_object_intent_id"]]
            cost = next(v for v in costs if v["activity_id"] == call["id"])
            sha = r.digest(b"") if call["operation"] == "PROTECTED_HEAD" else intent["content_sha256"]
            r.need(observation["workspace_id"] == s["workspaceId"] and observation["invoked_at"] and observation["http_status"] == 200 and observation["failure"] == "" and observation["response_sha256"] == sha, "Successful HEAD/GET hash observation required")
            r.need(datetime.fromisoformat(call["started_at"]) <= datetime.fromisoformat(observation["invoked_at"]) <= datetime.fromisoformat(observation["observed_at"]), "Physical call chronology drift")
            r.need(cost["workspace_id"] == s["workspaceId"] and cost["execution_id"] is None and cost["cost_type"] == "LIFECYCLE_PROVIDER_INVOCATION" and cost["quantity"] == 1 and cost["unit"] == "invocation" and cost["amount"] is None and cost["currency"] is None and cost["pricing_mode"] == "ACTUAL" and cost["occurred_at"] == observation["invoked_at"], "Actual physical-call cost required")
        for version in versions:
            pair = {v["operation"]: v for v in calls if intents[v["protected_object_intent_id"]]["dataset_version_id"] == version}
            head, get = pair["PROTECTED_HEAD"], pair["PROTECTED_GET"]
            head_observation = next(v for v in observations if v["call_id"] == head["id"])
            r.need(head["protected_object_intent_id"] == get["protected_object_intent_id"] and datetime.fromisoformat(head_observation["observed_at"]) <= datetime.fromisoformat(get["started_at"]), "Expected same-intent HEAD before GET")
        facts = {v["id"]:("LifecycleStorageCallStarted", "LIFECYCLE_STORAGE_CALL", v["dependency_revision"], {"operation":v["operation"], "protected_object_intent_id":v["protected_object_intent_id"]}) for v in calls}
        facts.update({v["id"]:("LifecycleStorageCallObserved", "LIFECYCLE_STORAGE_OBSERVATION", next(c["dependency_revision"] for c in calls if c["id"] == v["call_id"]), {"call_id":v["call_id"], "http_status":200, "invocation_confirmed":True, "failure":""}) for v in observations})
        events = [v for k,v in new_events.items() if k not in old_events]
        r.need(len(events) == len(delta["audit_event"]) == len(delta["evidence"]) == len(delta["evidence_relation"]) == 20, "Exact probe fact closure required")
        for fact, (event_type, subject, revision, metadata) in facts.items():
            metadata = dict(metadata, fact_id=fact, workspace_id=s["workspaceId"], dependency_revision=revision)
            matching_events = [v for v in events if v["aggregate_id"] == fact and v["event_type"] == event_type and v["aggregate_type"] == subject]
            audits = [v for v in delta["audit_event"] if v["object_id"] == fact and v["action"] == event_type and v["object_type"] == subject and v["workspace_id"] == s["workspaceId"] and v["actor_id"] == s["actorId"]]
            evidence = [v for v in delta["evidence"] if v["source_id"] == fact and v["title"] == event_type and v["source_type"] == subject and v["workspace_id"] == s["workspaceId"] and v["created_by"] == s["actorId"] and v["evidence_type"] == "LIFECYCLE_OCCUPANCY"]
            r.need(len(matching_events) == len(audits) == len(evidence) == 1, "Missing exact audit/evidence/event")
            r.need(matching_events[0]["payload"] == audits[0]["metadata"] == evidence[0]["metadata"] == metadata, "Probe metadata drift")
            r.need(len([v for v in delta["evidence_relation"] if v["evidence_id"] == evidence[0]["id"] and v["object_id"] == fact and v["object_type"] == subject and v["relation_type"] == "CURRENT_CONTROL_FACT"]) == 1, "Typed probe evidence relation required")
    else:
        r.need(set(new_events) == set(old_events), "Unexpected new Outbox event before protected probe")
    for name in bt:
        if name not in allowed:
            r.need(Counter(bt[name]["rows"]) == Counter(at[name]["rows"]), "Retained business facts changed: " + name)
    return {"status":"PASS", "verifiedVersions":5 if probe else 0, "callIds":sorted(v["id"] for v in delta.get("lifecycle_storage_call", [])), "runtimeRootFallback":False}


def probe(s, baseline, directory, receipts):
    # Require already materialized intents; this entry never repairs/copies data.
    data = validate(s, baseline)
    r.need(all(any(i["dataset_version_id"] == v["id"] and i["workspace_id"] == s["workspaceId"] for i in data["lifecycle_protected_object_intent"]) for v in baseline["expected"]["versions"]), "Existing five protected read intents required")
    c.write_once(receipts / "protected-probe-started.json", {"baselineSha256":r.digest(json.dumps(baseline,sort_keys=True).encode()), "versions":baseline["expected"]["versions"]})
    try:
        result = invoke(s, "resume-verify", profile=s["workspaceObjectProfile"], **baseline["expected"])
    finally:
        # Even a provider failure/unknown must retain its physical-call ledger.
        after = observe(s)
        c.write_once(directory / "after-probe-inventory.json", after)
    r.need(result == {"status":"PASS", "verifiedVersions":5, "runtimeRootFallback":False}, "Protected five-version verifier refused")
    proof = compare(s, baseline, after, probe=True)
    c.write_once(receipts / "protected-probe-result.json", proof)
    return after
