#!/usr/bin/env python3
"""Independent live Core oracle after the Edge B3 business operations.

Reads the isolated PostgreSQL directly and exercises bounded HTTP rejection /
same-request recovery. It does not create extraction, quality, certification or
delivery successes. Business facts must already have been produced in Edge.
"""
import argparse
from concurrent.futures import ThreadPoolExecutor
import hashlib
import json
from pathlib import Path
import subprocess
import urllib.error
import urllib.parse
import urllib.request
import uuid


def verify(runtime, download, output, metadata_status="UNKNOWN"):
    state = json.loads(runtime.read_text(encoding="utf-8"))
    container, ws = state["coreContainer"], str(uuid.UUID(state["workspaceId"]))
    assert container.startswith("dpp-pgi-b3-core-")
    assert urllib.parse.urlparse(state["api"]).hostname == "127.0.0.1"
    private = Path(state["private"]).resolve()
    assert private.name.startswith("dpp-pgi-b3-")
    facts = json.loads((private / "facts.json").read_text(encoding="utf-8"))
    token = (private / "token").read_text(encoding="utf-8").strip()
    checks = []

    def check(name, condition, detail=None):
        assert condition, (name, detail)
        checks.append(dict(name=name, status="PASS", detail=detail))

    def sql(query):
        r = subprocess.run(["docker", "exec", container, "psql", "-U", "postgres",
                            "-d", "dpp_ingestion_core", "-At", "-v", "ON_ERROR_STOP=1",
                            "-c", query], capture_output=True, encoding="utf-8", check=True)
        return [json.loads(line) for line in r.stdout.splitlines() if line.strip()]

    def rows(table, predicate="true"):
        return sql(f'SELECT row_to_json(t) FROM "{table}" t WHERE {predicate}')

    def api(path, method="GET", body=None, key=None, authorized=True):
        headers = {"Content-Type": "application/json", "X-Actor-ID": state["actorId"]}
        if authorized:
            headers["Authorization"] = "Bearer " + token
        if key:
            headers["Idempotency-Key"] = key
        request = urllib.request.Request(state["api"] + "/api/v1" + path,
                    data=None if body is None else json.dumps(body).encode(),
                    headers=headers, method=method)
        try:
            response = urllib.request.urlopen(request, timeout=15)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            content = response.read()
            assert b"source_company_id" not in content, "unexpected dataset payload during oracle"
            return response.status, json.loads(content) if content else None

    check("isolated browser acceptance uses production web", state.get("webMode") == "production")
    datasets = rows("dataset", f"workspace_id='{ws}'")
    versions = rows("dataset_version", f"dataset_id IN (SELECT id FROM dataset WHERE workspace_id='{ws}')")
    by_type = {kind: [v for v in versions if v["dataset_id"] in
                {d["id"] for d in datasets if d["dataset_type"] == kind}]
                for kind in ("RAW", "STANDARDIZED", "CURATED")}
    check("same Core has three RAW, one STANDARDIZED and one CURATED", [len(by_type[k]) for k in by_type] == [3, 1, 1])
    check("RAW remains READY with original 6/10/14 rows", sorted(v["row_count"] for v in by_type["RAW"]) == [6, 10, 14] and all(v["status"] == "READY" for v in by_type["RAW"]))
    operations = rows("ingestion_operation", f"workspace_id='{ws}'")
    receipts = rows("ingestion_receipt", f"operation_id IN (SELECT id FROM ingestion_operation WHERE workspace_id='{ws}')")
    check("browser batch accepted with exactly one persisted manifest receipt", len(operations) == len(receipts) == 1 and operations[0]["state"] == "ACCEPTED" and operations[0]["manifest_hash"] == receipts[0]["manifest_hash"])
    slots = rows("ingestion_receipt_slot", f"receipt_id='{receipts[0]['id']}'")
    raw_ids = {v["id"] for v in by_type["RAW"]}
    check("receipt references exact three immutable RAW content hashes", {s["version_id"] for s in slots} == raw_ids and all(s["content_hash"] == next(v["checksum_value"] for v in versions if v["id"] == s["version_id"]) for s in slots))
    executions = rows("execution", f"workspace_id='{ws}'")
    curated = by_type["CURATED"][0]
    version = curated["id"]
    check("one actual native execution succeeded", len(executions) == 1 and executions[0]["status"] == "SUCCEEDED" and executions[0]["output_dataset_version_id"] == version)
    quarantine = rows("execution_quarantine_record", f"execution_id='{executions[0]['id']}'")
    check("negative energy quarantined with original -999 value", len(quarantine) == 1 and quarantine[0]["reason_code"] == "NEGATIVE_ENERGY_KWH" and quarantine[0]["payload"]["energy_kwh"] == "-999")
    check("real browser download equals immutable 5-row 640-byte output", curated["row_count"] == 5 and curated["byte_size"] == len(download.read_bytes()) == 640 and hashlib.sha256(download.read_bytes()).hexdigest() == curated["checksum_value"], curated["checksum_value"])
    quality = rows("quality_result", f"workspace_id='{ws}'")
    positive = [q for q in quality if q["dataset_version_id"] == version]
    negative = [q for q in quality if q["dataset_version_id"] in raw_ids]
    check("actual quality PASS and RAW FAIL are separate immutable facts", len(positive) == len(negative) == 1 and positive[0]["gate_decision"] == "PASS" and negative[0]["gate_decision"] == "FAIL")
    certs = rows("dataset_certification", f"workspace_id='{ws}'")
    failed = [c for c in certs if c["dataset_version_id"] == negative[0]["dataset_version_id"]]
    check("failed quality never produced CERTIFIED", len(failed) >= 1 and all(c["decision"] == "REJECTED" and any(b["code"] == "QUALITY_GATE_NOT_PASSED" for b in c["blockers"]) for c in failed))
    commands = rows("command_idempotency", f"workspace_id='{ws}'")
    prepared = [c for c in commands if c["command_type"] == "CERTIFICATION.PREPARE_INPUTS"]
    assert len(prepared) == 1
    prep = prepared[0]
    snapshots = rows("evidence_snapshot", f"id='{prep['object_id']}'")
    snapshot = snapshots[0]
    members = rows("evidence_snapshot_item", f"snapshot_id='{snapshot['id']}'")
    check("browser prepared FINALIZED snapshot of four actual engine/quality/compliance evidence items", snapshot["status"] == "FINALIZED" and len(members) == 4 and snapshot["manifest"]["qualityAssessmentId"] == positive[0]["id"] and snapshot["manifest"]["profileId"] == facts["Profile"])
    dispositions = rows("certification_disposition", f"workspace_id='{ws}'")
    check("explicit supersede preserves both CERTIFIED history facts", len(dispositions) == 1 and dispositions[0]["disposition"] == "SUPERSEDED" and all(next(c for c in certs if c["id"] == identity)["decision"] == "CERTIFIED" for identity in (dispositions[0]["certification_id"], dispositions[0]["superseded_by_certification_id"])))
    allocations = rows("cost_allocation", f"dataset_certification_id IN (SELECT id FROM dataset_certification WHERE workspace_id='{ws}') OR certification_disposition_id IN (SELECT id FROM certification_disposition WHERE workspace_id='{ws}')")
    check("each actual certification evaluation and disposition has allocated cost", len(allocations) == len(certs) + len(dispositions))
    deliveries = rows("delivery_operation", f"workspace_id='{ws}'")
    check("actual browser delivery issued and revoked retry blocked", sum(d["status"] == "ISSUED" for d in deliveries) == 2 and sum(d["status"] == "BLOCKED" for d in deliveries) == 2 and all(d["retry_of_delivery_operation_id"] for d in deliveries if d["status"] == "BLOCKED"))
    check("revocation retained original issued history", all(d["terminal_reason"] == "CURRENT_ENTITLEMENT_BLOCKED" for d in deliveries if d["status"] == "BLOCKED") and rows("data_authorization", f"id='{facts['Authorization']}'")[0]["status"] == "REVOKED")
    base = f"/workspaces/{ws}/dataset-versions/{version}"
    scope = next(d["scope_ref"] for d in deliveries if d["status"] == "ISSUED")
    query = urllib.parse.urlencode(dict(profileId=facts["Profile"], consumer="LICENSED_BANK", purpose="ENTERPRISE_CREDIT_RISK_SUPPORT", action="READ", delivery="DIRECT_DATA", scopeType="ALL_RESOURCE", scopeRef=scope))
    status, gate = api(base + "/delivery-eligibility?" + query)
    check("fresh gate separates valid certification from revoked entitlement", status == 200 and not gate["allowed"] and gate["certification"]["allowed"] and not gate["entitlement"]["allowed"])
    event = rows("outbox_event", "event_type='DatasetCertificationInputsPrepared'")
    check("preparation domain event published with explicit no-task obligation", len(event) == 1 and event[0]["status"] == "PUBLISHED" and event[0]["required_handlers"] == [])
    audit = rows("audit_event", f"workspace_id='{ws}' AND action='CERTIFICATION_INPUTS_PREPARED'")
    check("preparation audit references exact frozen evidence snapshot", len(audit) == 1 and audit[0]["object_id"] == snapshot["id"])
    effect = snapshot["manifest"]["references"]["effectiveRightsSnapshotId"]
    check("rights preparation has a real allocated cost", len(rows("cost_allocation", f"effective_rights_snapshot_id='{effect}'")) == 1)

    def counts():
        return sql("SELECT jsonb_build_object(" + ",".join(f"'{t}',(SELECT count(*) FROM {t})" for t in
                   ("dataset_certification", "certification_disposition", "evidence_snapshot", "effective_rights_snapshot", "cost_event", "outbox_event", "audit_event")) + ")")[0]
    before = counts()
    body = dict(profileId=facts["Profile"], qualityAssessmentId=positive[0]["id"])
    with ThreadPoolExecutor(max_workers=6) as workers:
        replay = list(workers.map(lambda _: api(base + "/certification-inputs", "POST", body, prep["idempotency_key"]), range(6)))
    check("six concurrent preparation replays recover or report lock contention", any(s == 200 for s,b in replay) and all((s == 200 and b["evidenceSnapshotId"] == snapshot["id"]) or (s == 409 and b["code"] == "CERTIFICATION_INPUTS_NOT_PREPARED") for s,b in replay), {"recovered":sum(s == 200 for s,b in replay),"contended":sum(s == 409 for s,b in replay)})
    recovery = api(base + "/certification-inputs", "POST", body, prep["idempotency_key"])
    check("retry after contention recovers exact snapshot", recovery[0] == 200 and recovery[1]["evidenceSnapshotId"] == snapshot["id"])
    changed = dict(body, qualityAssessmentId=negative[0]["id"])
    check("same preparation key with different quality fails closed", api(base + "/certification-inputs", "POST", changed, prep["idempotency_key"])[0] == 409)
    disposition = dispositions[0]
    key = next(c["idempotency_key"] for c in commands if c["command_type"] == "CERTIFICATION.DISPOSITION")
    response = api(f"/workspaces/{ws}/certifications/{disposition['certification_id']}/supersede", "POST", dict(replacementId=disposition["superseded_by_certification_id"], reason=disposition["reason"], effectiveAt=disposition["effective_at"]), key)
    check("supersede replay returns same disposition", response[0] == 200 and response[1]["id"] == disposition["id"])
    check("forged CERTIFIED decision is rejected", api(base + "/certifications", "POST", dict(body, decision="CERTIFIED"), str(uuid.uuid4()))[0] == 400)
    check("replays/rejections add no snapshots, costs, decisions, audits or events", before == counts())

    resource = next(d["source_resource_id"] for d in datasets if d["dataset_type"] == "RAW")
    paths = [f"/workspaces/{ws}/certification-profiles", base + "/certification-inputs?" + urllib.parse.urlencode(body),
             f"/workspaces/{ws}/executions/{executions[0]['id']}/quarantine",
             f"/data-resources/{resource}/metadata-observation?workspaceId={ws}",
             f"/workspaces/{ws}/entity-match-jobs?inputVersionId={by_type['RAW'][0]['id']}"]
    check("new read endpoints reject forged actor without trusted credential", all(api(p, authorized=False)[0] == 403 for p in paths))
    other = str(uuid.uuid4())
    check("trusted token cannot cross workspace", all(api(p.replace(ws, other))[0] == 403 for p in paths))
    check("quarantine rejects negative page offset", api(paths[2] + "?offset=-1")[0] == 400)
    observation = api(paths[3])
    check("metadata observation is " + metadata_status, observation[0] == 200 and observation[1]["status"] == metadata_status)
    # Same-key direct-data recovery returns JSON; it must never resend the CSV.
    payload = dict(profileId=facts["Profile"], consumer="LICENSED_BANK", purpose="ENTERPRISE_CREDIT_RISK_SUPPORT", action="READ", scopeType="ALL_RESOURCE", scopeRef=scope)
    issued = next(d for d in deliveries if d["status"] == "ISSUED")
    result = api(base + "/deliveries", "POST", payload, issued["idempotency_key"])
    check("issued direct-data same-key recovery is non-payload", result[0] == 409)
    private_config = json.loads((private / "test.json").read_text(encoding="utf-8"))
    secret_values = [token, private_config["RootKey"], private_config["RootSecret"]] + [(private / name).read_text(encoding="utf-8").strip() for name in ("source-password", "hop-password", "core-password")]
    identities = [json.loads((private / name).read_text(encoding="utf-8")) for name in ("core-identity.json", "publisher-identity.json")]
    secret_values += [value for identity in identities for key, value in identity.items() if key.lower() in ("secretkey", "sessiontoken") and value]
    environment = json.loads((private / "environment.json").read_text(encoding="utf-8"))
    secret_values += [environment["api"].get("OPENMETADATA_TOKEN", ""), environment["web"].get("DELIVERY_WEB_GATEWAY_TOKEN", "")]
    secret_values = [value for value in secret_values if value]
    public_wire = json.dumps(dict(operations=operations, receipts=receipts, certs=certs, snapshot=snapshot,
                     events=rows("outbox_event"), audits=rows("audit_event")), ensure_ascii=False)
    check("public business/audit/event facts contain no known credential secrets", all(value not in public_wire for value in secret_values))
    html = []
    for path in (f"/resources/{resource}", f"/batches/{operations[0]['id']}",
                 f"/datasets/{curated['dataset_id']}/versions/{version}?view=certification"):
        with urllib.request.urlopen(state["web"] + path, timeout=15) as response:
            html.append(response.read().decode("utf-8"))
    check("rendered browser pages contain no known credential secrets", all(value not in page for value in secret_values for page in html))
    report = dict(status="PASS", workspaceId=ws, api=state["api"], web=state["web"], webMode=state.get("webMode"), apiSha256=state.get("apiSha256"),
                  acceptance={"PGI-08":"PASS", "metadata-observation":metadata_status, "PGI-09-human-explanation":"NOT_RUN"},
                  checks=checks, facts=dict(batch=operations[0]["id"], outputVersion=version, outputHash=curated["checksum_value"],
                  preparedSnapshot=snapshot["id"], currentCertification=disposition["superseded_by_certification_id"]),
                  notRun=["metadata outage/reconnect not assessed by this B3 oracle", "human operator explanation - explicitly deferred by user", "production IAM/provider acceptance"])
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps(dict(status="PASS", checks=len(checks), report=str(output))))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--runtime", type=Path, required=True)
    parser.add_argument("--download", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--metadata-status", choices=("UNKNOWN", "OBSERVED", "STALE"), default="UNKNOWN")
    args = parser.parse_args()
    verify(args.runtime, args.download, args.output, args.metadata_status)
