#!/usr/bin/env python3
"""Read-only, nonempty publication protection oracle for the isolated #298 run.

Capture healthy/outage/projection-failed/recovered phases around real browser
and Command operations. Compare exact historical rows, membership and ancestry;
mutable governance projection status is observed separately, never hashed as a
Core publication fact. No SQL mutation or provider call is performed here.
"""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import uuid
import urllib.error
import urllib.parse
import urllib.request


def capture(runtime, phase, output, previous=None):
    state = json.loads(runtime.read_text(encoding="utf-8"))
    container = state["coreContainer"]
    assert container.startswith("dpp-pgi-b3-core-")
    private = Path(state["private"])
    assert private.name.startswith("dpp-pgi-b3-")
    ws = str(uuid.UUID(state["workspaceId"]))
    assert urllib.parse.urlparse(state["api"]).hostname == "127.0.0.1"
    token = (private / "token").read_text(encoding="utf-8").strip()
    facts = json.loads((private / "facts.json").read_text(encoding="utf-8"))

    def api(path):
        request = urllib.request.Request(state["api"] + "/api/v1" + path, headers={"Authorization": "Bearer " + token})
        with urllib.request.urlopen(request, timeout=20) as response:
            return json.load(response)

    def rows(table, predicate):
        query = f'SELECT row_to_json(t) FROM "{table}" t WHERE {predicate}'
        result = subprocess.run(["docker", "exec", container, "psql", "-U", "postgres", "-d", "dpp_ingestion_core", "-At", "-v", "ON_ERROR_STOP=1", "-c", query], capture_output=True, encoding="utf-8", check=True)
        return sorted((json.loads(line) for line in result.stdout.splitlines() if line.strip()), key=lambda row: json.dumps(row, sort_keys=True))

    def digest(value):
        return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode()).hexdigest()

    histories = {}
    projections = {}
    for label in ("release-healthy", "release-outage"):
        path = private / (label + ".json")
        if not path.exists():
            continue
        release_facts = json.loads(path.read_text(encoding="utf-8"))
        ids = {key: str(uuid.UUID(value)) for key, value in release_facts.items()}
        release = ids["Release"]
        membership = rows("product_release_dataset", f"release_id='{release}'")
        assert membership, "historical publication membership must be nonempty"
        # Recursive closure includes the STANDARDIZED -> RAW input as well as
        # the execution's four frozen dependencies. UNION bounds shared paths.
        closure = f"WITH RECURSIVE ancestry(id) AS (SELECT dataset_version_id FROM product_release_dataset WHERE release_id='{release}' UNION SELECT l.input_version_id FROM dataset_version_lineage l JOIN ancestry a ON a.id=l.output_version_id) SELECT id FROM ancestry"
        snapshots = rows("evidence_snapshot", f"id='{ids['Evidence']}'")
        members = rows("evidence_snapshot_item", f"snapshot_id='{ids['Evidence']}'")
        assert len(snapshots) == 1 and snapshots[0]["status"] == "FINALIZED" and members
        groups = {
            "release": rows("product_release", f"id='{release}'"),
            "releaseMembership": membership,
            "productVersion": rows("product_version", f"id='{ids['Version']}'"),
            "productAssets": rows("product_asset", f"product_version_id='{ids['Version']}'"),
            "ancestryVersions": rows("dataset_version", f"id IN ({closure})"),
            "ancestryLineage": rows("dataset_version_lineage", f"output_version_id IN ({closure})"),
            "quality": rows("quality_result", f"id='{ids['Quality']}'"),
            "compliance": rows("compliance_result", f"id='{ids['Compliance']}'"),
            "rights": rows("rights_snapshot", f"id='{ids['Rights']}'"),
            "rightsAuthorizations": rows("rights_snapshot_authorization", f"rights_snapshot_id='{ids['Rights']}'"),
            "rightsDeclarations": rows("rights_snapshot_declaration", f"rights_snapshot_id='{ids['Rights']}'"),
            "rightsBindings": rows("rights_snapshot_provenance_binding", f"rights_snapshot_id='{ids['Rights']}'"),
            "evidenceSnapshot": snapshots,
            "evidenceMembership": members,
            "evidence": rows("evidence", f"id IN (SELECT evidence_id FROM evidence_snapshot_item WHERE snapshot_id='{ids['Evidence']}')"),
            "publishAudit": rows("audit_event", f"object_id='{release}' AND action='PRODUCT_RELEASE_PUBLISHED'"),
        }
        assert len(groups["release"]) == 1 and groups["release"][0]["status"] == "PUBLISHED"
        assert len(groups["ancestryVersions"]) == 5 and groups["ancestryLineage"]
        assert groups["productAssets"] and len(groups["publishAudit"]) == 1
        histories[label] = dict(ids=ids, hashes={key: digest(value) for key, value in groups.items()}, counts={key: len(value) for key, value in groups.items()}, rootHash=snapshots[0]["root_hash"])
        projection = rows("governance_projection", f"object_id='{release}'")
        assert len(projection) == 1
        projections[label] = {key: projection[0][key] for key in ("object_id", "source_event_id", "status", "attempts", "external_id", "external_fqn", "last_error")}
    assert histories, "zero publications cannot prove protection"
    resources = rows("data_resource", f"workspace_id='{ws}'")
    source_facts = sorted([{key: row[key] for key in ("id", "owner_id", "sensitivity_level", "revision")} for row in resources], key=lambda row: row["id"])
    assert len(source_facts) == 3
    observations = {r["id"]: api(f"/data-resources/{r['id']}/metadata-observation?workspaceId={ws}") for r in resources}
    bound = rows("resource_binding", f"resource_id IN (SELECT id FROM data_resource WHERE workspace_id='{ws}')")
    assert len(bound) == 3
    first_delivery = rows("delivery_operation", f"workspace_id='{ws}' AND status='ISSUED'")[0]
    query = urllib.parse.urlencode(dict(profileId=facts["Profile"], consumer="LICENSED_BANK", purpose="ENTERPRISE_CREDIT_RISK_SUPPORT", action="READ", delivery="DIRECT_DATA", scopeType="ALL_RESOURCE", scopeRef=first_delivery["scope_ref"]))
    gate = api(f"/workspaces/{ws}/dataset-versions/{first_delivery['dataset_version_id']}/delivery-eligibility?{query}")
    decisions = {key: gate[key]["allowed"] for key in ("datasetVersion", "certification", "entitlement")}
    if phase == "post-revocation":
        assert not gate["allowed"] and decisions == dict(datasetVersion=True, certification=True, entitlement=False)
    else:
        assert gate["allowed"] and all(decisions.values()), "directory outage must not invent or remove Core entitlement"
    healthy = phase in ("healthy", "recovered", "post-revocation")
    assert all(o["status"] == ("OBSERVED" if healthy else "STALE") for o in observations.values())
    for observation in observations.values():
        if healthy:
            assert observation["governance"]["owners"] and observation["governance"]["classifications"] == ["PGISynthetic.Internal"] and observation["governance"]["terms"] == ["PGIPark.Activity"]
        else:
            assert not observation.get("governance") and observation["lastConfirmedVersion"] == 0.1
    report = dict(status="PASS", phase=phase, workspaceId=ws, histories=histories, projections=projections, sourceFactsHash=digest(source_facts), bindingHash=digest(bound), observations=observations, gateDecisions=decisions, compared=[])
    if healthy:
        env = json.loads((private / "environment.json").read_text(encoding="utf-8"))["api"]
        endpoint = urllib.parse.urlparse(env["OPENMETADATA_BASE_URL"])
        assert endpoint.scheme == "http" and endpoint.hostname == "127.0.0.1" and not endpoint.username
        request = urllib.request.Request(env["OPENMETADATA_BASE_URL"] + "/v1/dataProducts/name/DP-ENTERPRISE-ACTIVITY?fields=domains", headers={"Authorization": "Bearer " + env["OPENMETADATA_TOKEN"]})
        with urllib.request.urlopen(request, timeout=20) as response:
            native = json.load(response)
        domains = [domain["fullyQualifiedName"] for domain in native["domains"]]
        assert domains == ["SyntheticPark"] and all(p["external_id"] == native["id"] for p in projections.values() if p["status"] == "SUCCEEDED")
        report["nativeDataProduct"] = dict(id=native["id"], fqn=native["fullyQualifiedName"], domains=domains, version=native["version"])
    if previous:
        prior = json.loads(previous.read_text(encoding="utf-8"))
        assert prior["workspaceId"] == ws and prior["sourceFactsHash"] == report["sourceFactsHash"] and prior["bindingHash"] == report["bindingHash"]
        if phase != "post-revocation":
            assert prior["gateDecisions"] == decisions
        for label, history in prior["histories"].items():
            assert histories[label] == history, (label, "immutable publication changed")
            report["compared"].append(label)
    if phase == "healthy":
        assert projections["release-healthy"]["status"] == "SUCCEEDED"
    elif phase == "outage":
        assert previous and "release-healthy" in report["compared"]
    elif phase == "projection-failed":
        assert previous and projections["release-outage"]["status"] == "FAILED" and projections["release-outage"]["attempts"] >= 1
    elif phase in ("recovered", "post-revocation"):
        assert previous and set(report["compared"]) == {"release-healthy", "release-outage"}
        assert all(p["status"] == "SUCCEEDED" and p["external_id"] for p in projections.values())
        assert projections["release-outage"]["attempts"] >= 2
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps({"status": "PASS", "phase": phase, "publications": len(histories), "compared": report["compared"]}))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--runtime", type=Path, required=True)
    parser.add_argument("--phase", choices=("healthy", "outage", "projection-failed", "recovered", "post-revocation"), required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--previous", type=Path)
    args = parser.parse_args()
    capture(args.runtime, args.phase, args.output, args.previous)
