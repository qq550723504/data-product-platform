#!/usr/bin/env python3
"""Independent C1 PostgreSQL/HTTP oracle; successes must already exist in Edge.

Only same-key recovery/rejection commands are sent. No synthetic engine outcome,
certification or delivery success is fabricated. Private credentials stay local.
"""
import argparse, hashlib, json, subprocess, urllib.request, urllib.error, uuid
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

class Oracle:
    def __init__(self, runtime):
        self.state = json.loads(runtime.read_text(encoding="utf-8"))
        self.container = self.state["coreContainer"]
        assert self.container.startswith("dpp-pgi-b3-core-")
        assert self.state["api"].startswith("http://127.0.0.1:")
        self.ws = str(uuid.UUID(self.state["workspaceId"]))
        self.private = Path(self.state["private"])
        self.token = (self.private / "token").read_text().strip()
        self.human_token = json.loads((self.private / "environment.json").read_text())["api"]["HUMAN_DECISION_API_TOKEN"]
    def rows(self, table, predicate="true"):
        q = f'SELECT row_to_json(t) FROM "{table}" t WHERE {predicate}'
        r = subprocess.run(["docker", "exec", self.container, "psql", "-U", "postgres", "-d", "dpp_ingestion_core", "-At", "-v", "ON_ERROR_STOP=1", "-c", q], capture_output=True, encoding="utf-8", check=True)
        return sorted([json.loads(line) for line in r.stdout.splitlines() if line.strip()], key=lambda v: json.dumps(v, sort_keys=True))
    def api(self, path, body, key, authorized=True):
        h = {"Content-Type":"application/json", "Idempotency-Key":key}
        if authorized:
            token = self.human_token if "/quality-remediations" in path else self.token
            h["Authorization"] = "Bearer " + token
        req = urllib.request.Request(self.state["api"] + "/api/v1" + path, data=json.dumps(body).encode(), headers=h, method="POST")
        try: response = urllib.request.urlopen(req, timeout=30)
        except urllib.error.HTTPError as error: response = error
        with response:
            payload = response.read()
            assert b"source_company_id" not in payload
            return response.status, json.loads(payload) if payload else None

def digest(rows):
    return hashlib.sha256(json.dumps(rows, sort_keys=True, separators=(",",":"), ensure_ascii=False).encode()).hexdigest()

def history(o, prior=None):
    tables = ("dataset_version", "dataset_version_lineage", "quality_result", "quality_finding", "compliance_result", "dataset_certification", "evidence_snapshot", "evidence_snapshot_item", "rights_snapshot", "rights_snapshot_authorization", "rights_snapshot_declaration", "rights_snapshot_provenance_binding", "effective_rights_snapshot", "product_version", "product_asset", "product_release", "product_release_dataset")
    frozen = {}
    for table in tables:
        if prior:
            predicate = prior[table]["predicate"]
        else:
            original = o.rows(table)
            identities = []
            for row in original:
                keys = ["id"] if "id" in row else [k for k in row if k.endswith("_id")]
                assert keys, table
                identity = {k:row[k] for k in keys}
                literal = json.dumps(identity, separators=(",",":")).replace("'", "''")
                identities.append(f"to_jsonb(t) @> '{literal}'::jsonb")
            predicate = " OR ".join(identities) or "false"
        values = o.rows(table, predicate)
        value_hash = digest(values)
        if prior and table == "dataset_version" and value_hash != prior[table]["sha256"]:
            # SetReady explicitly supersedes an earlier READY version. Prove
            # exactly that audited lifecycle change; all other bytes must match.
            restored = []
            for value in values:
                value = dict(value)
                if value["status"] == "SUPERSEDED":
                    assert o.rows("outbox_event", f"aggregate_id='{value['id']}' AND event_type='DatasetVersionSuperseded'")
                    assert o.rows("audit_event", f"object_id='{value['id']}' AND action='DATASET_VERSION_SUPERSEDED'")
                    value["status"] = "READY"
                restored.append(value)
            assert digest(restored) == prior[table]["sha256"], "version content changed beyond explicit supersession"
            value_hash = prior[table]["sha256"]
        frozen[table] = dict(predicate=predicate, count=len(values), sha256=value_hash)
        if prior: assert frozen[table] == prior[table], (table, "history changed")
    assert frozen["product_release"]["count"] > 0 and frozen["product_release_dataset"]["count"] > 0
    assert frozen["dataset_certification"]["count"] > 0 and frozen["quality_result"]["count"] > 0
    return frozen

def run(runtime, output, previous=None, download=None):
    o = Oracle(runtime)
    prior = json.loads(previous.read_text()) if previous else None
    report = dict(status="PASS", workspaceId=o.ws, history=history(o, prior["history"] if prior else None), checks=[])
    def check(name, condition, detail=None):
        assert condition, (name,detail)
        report["checks"].append(dict(name=name,status="PASS",detail=detail))
    if previous:
        cases = o.rows("quality_remediation_case", f"workspace_id='{o.ws}'")
        check("one actual negative finding case", len(cases)==1)
        c = cases[0]; cid = c["id"]
        findings = o.rows("quality_finding", f"id='{c['finding_id']}'")
        check("actual negative RAW detection is 1/14", findings[0]["status"]=="FAIL" and findings[0]["observed"]["affectedCount"]==1 and findings[0]["observed"]["total"]==14)
        links = o.rows("quality_remediation_execution", f"case_id='{cid}'")
        links.sort(key=lambda v:v["ordinal"])
        executions = [o.rows("execution", f"id='{v['execution_id']}'")[0] for v in links]
        check("real failure and explicit retry preserve two attempts", [e["status"] for e in executions]==["FAILED","SUCCEEDED"] and executions[1]["retry_of_execution_id"]==executions[0]["id"])
        target = executions[-1]["output_dataset_version_id"]
        check("output is a new immutable version", target!=c["version_id"] and target not in prior.get("versions",[]))
        quarantine = o.rows("execution_quarantine_record", f"execution_id='{executions[-1]['id']}'")
        check("quarantine preserves negative original", len(quarantine)==1 and quarantine[0]["reason_code"]=="NEGATIVE_ENERGY_KWH" and quarantine[0]["payload"]["energy_kwh"]=="-999")
        m = executions[-1]["metrics"]
        check("energy grain conserved separately from enterprise aggregation", m["energyRecords"]==14 and m["energyMapped"]==13 and m["quarantineCount"]==1 and m["outputRows"]==5)
        rechecks = o.rows("quality_remediation_recheck",f"case_id='{cid}'")
        check("one independent recheck",len(rechecks)==1)
        outcome = o.rows("quality_remediation_recheck_outcome", f"id='{rechecks[0]['id']}'")[0]
        quality = o.rows("quality_result",f"id='{outcome['assessment_id']}'")[0]
        check("full existing ruleset assessed new output PASS", quality["dataset_version_id"]==target and quality["gate_decision"]=="PASS" and quality["rule_set_ref"]==c["recheck_rule_ref"])
        costs = o.rows("cost_event",f"workspace_id='{o.ws}'")
        check("all three real native invocations charged incl failure",sum(e["cost_type"]=="NATIVE_ENGINE_INVOCATION" for e in costs)==3)
        before = {t:digest(o.rows(t)) for t in ("execution","quality_result","cost_event","quality_remediation_execution","quality_remediation_recheck","audit_event","outbox_event")}
        base = f"/workspaces/{o.ws}/quality-remediations/{cid}"
        retry = links[-1]
        closed = o.rows("quality_remediation_close", f"case_id='{cid}'")
        with ThreadPoolExecutor(max_workers=8) as workers:
            replies = list(workers.map(lambda _:o.api(base+"/retry",dict(reason=retry["reason"]),retry["request_key"]),range(8)))
        check("eight HTTP same-key retries recover same execution including after closure",all(s==200 and b["executionId"]==retry["execution_id"] for s,b in replies))
        if closed:
            check("closed case blocks new execution admission",o.api(base+"/retry",dict(reason=retry["reason"]),"closed-new-execution")[0]==409)
        rc = rechecks[0]
        response = o.api(base+"/recheck",dict(executionLinkId=rc["execution_link_id"]),rc["request_key"])
        check("accepted recheck recovers same identity after close",response[0]==200 and response[1]["recheckId"]==rc["id"])
        if closed:
            check("closed case blocks new recheck admission",o.api(base+"/recheck",dict(executionLinkId=rc["execution_link_id"]),"closed-new-recheck")[0]==409)
            cl=closed[0]
            check("manual close same-key recovery is stable",o.api(base+"/close",dict(reason=cl["reason"]),cl["request_key"])[0]==200)
        check("changed same-key handling reason rejects",o.api(base+"/retry",dict(reason="different"),retry["request_key"])[0]==409)
        check("forged actor without credential rejects",o.api(base+"/retry",dict(reason=retry["reason"]),retry["request_key"],False)[0]==403)
        check("foreign workspace rejects",o.api(base.replace(o.ws,str(uuid.uuid4()))+"/retry",dict(reason=retry["reason"]),retry["request_key"])[0]==403)
        check("replays create no facts or cost",before=={t:digest(o.rows(t)) for t in before})
        if download:
            v=o.rows("dataset_version",f"id='{target}'")[0]
            check("real browser payload matches output hash",len(download.read_bytes())==v["byte_size"]==640 and hashlib.sha256(download.read_bytes()).hexdigest()==v["checksum_value"])
        certs=o.rows("dataset_certification",f"workspace_id='{o.ws}'")
        source_certs=[v for v in certs if v["dataset_version_id"]==c["version_id"]]
        check("manual closure did not turn RAW failure into certification",source_certs and all(v["decision"]=="REJECTED" and any(b["code"]=="QUALITY_GATE_NOT_PASSED" for b in v["blockers"]) for v in source_certs))
        check("new output has independent certification",any(v["dataset_version_id"]==target and v["decision"]=="CERTIFIED" for v in certs))
        deliveries=o.rows("delivery_operation",f"workspace_id='{o.ws}'")
        issued=[v for v in deliveries if v["status"]=="ISSUED"]
        blocked=[v for v in deliveries if v["status"]=="BLOCKED"]
        check("issued history retained and stale ALLOWED request blocked",len(issued)==len(blocked)==1 and blocked[0]["terminal_reason"]=="CURRENT_ENTITLEMENT_BLOCKED")
        facts=json.loads((o.private/"facts.json").read_text())
        check("actual source authorization revoked",o.rows("data_authorization",f"id='{facts['Authorization']}'")[0]["status"]=="REVOKED")
        payload=dict(profileId=facts["Profile"],consumer="LICENSED_BANK",purpose=blocked[0]["purpose"],action=blocked[0]["action"],scopeType="ALL_RESOURCE",scopeRef=blocked[0]["scope_ref"])
        delivery_path=f"/workspaces/{o.ws}/dataset-versions/{target}/deliveries"
        status,_=o.api(delivery_path,payload,blocked[0]["idempotency_key"])
        check("blocked same-key recovery outputs zero dataset payload",status in (403,409))
        check("old direct-data issued recovery never resends bytes",o.api(delivery_path,payload,issued[0]["idempotency_key"])[0]==409)
        config=json.loads((o.private/"test.json").read_text())
        secrets=[o.token,config["RootSecret"],config["RootKey"]]+[(o.private/n).read_text().strip() for n in ("source-password","hop-password","core-password")]
        public_facts=json.dumps(dict(cases=cases,links=links,quality=quality,certifications=certs,events=o.rows("outbox_event"),audits=o.rows("audit_event")),ensure_ascii=False)
        check("business and audit facts contain no configured secrets",all(v not in public_facts for v in secrets))
        source=o.rows("dataset_version",f"id='{c['version_id']}'")[0]
        rendered=[]
        for path in (f"/datasets/{source['dataset_id']}/versions/{source['id']}/remediation",f"/datasets/{v['dataset_id']}/versions/{target}?view=eligibility" if download else "/batches"):
            with urllib.request.urlopen(o.state["web"]+path,timeout=20) as response:rendered.append(response.read().decode())
        check("final production browser responses contain no configured secrets",all(secret not in page for secret in secrets for page in rendered))
        report["caseId"],report["outputVersion"] = cid,target
        report["apiSha256"],report["webMode"]=o.state["apiSha256"],o.state["webMode"]
    report["versions"] = [v["id"] for v in o.rows("dataset_version")]
    output.write_text(json.dumps(report,ensure_ascii=False,indent=2),encoding="utf-8")
    print(json.dumps(dict(status="PASS",checks=len(report["checks"]),historyTables=len(report["history"]),report=str(output))))

if __name__=="__main__":
    p=argparse.ArgumentParser();p.add_argument("--runtime",type=Path,required=True);p.add_argument("--output",type=Path,required=True);p.add_argument("--previous",type=Path);p.add_argument("--download",type=Path)
    a=p.parse_args();run(a.runtime,a.output,a.previous,a.download)
