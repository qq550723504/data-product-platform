"""Run the unmodified native PostgreSQL workflow, then read its exact table.

This adapter emits structural metadata only. Native diagnostics remain on stderr;
credentials and source records must never be included in its result.
"""
import contextlib, hashlib, json, signal, sys, traceback, urllib.request
from datetime import datetime, timezone
from pathlib import Path

def connector_hash(cfg):
    # Freeze the actual non-secret source coordinates, filters, reader and native
    # version. Credential rotation does not change the observation contract.
    source = cfg["source"]
    connection = source["serviceConnection"]["config"]
    identity = {"nativeVersion":"2.0.5", "serviceName":source["serviceName"],
        "connection":{k:connection[k] for k in ("type","username","hostPort","database")},
        "sourceConfig":source["sourceConfig"], "tableFqn":cfg["dppTableFqn"]}
    return hashlib.sha256(json.dumps(identity,sort_keys=True,separators=(",",":")).encode()).hexdigest()

def observed_tables(source, target, observations):
    """Observe native typed records while preserving its iterator unchanged."""
    from metadata.generated.schema.api.data.createTable import CreateTableRequest
    from metadata.generated.schema.entity.data.table import Table
    from metadata.utils import fqn
    for record in source():
        if isinstance(record, CreateTableRequest):
            parts = fqn.split(record.databaseSchema.root)
            name = fqn.build(None, Table, service_name=parts[0], database_name=parts[1],
                schema_name=parts[2], table_name=record.name.root, skip_es_search=True)
            if name == target:
                observations.append([{"name":c.name.root,"type":c.dataType.value,
                    "constraint":c.constraint.value if c.constraint else "UNKNOWN"} for c in record.columns])
        yield record

def run(path):
    import yaml
    from importlib.metadata import version
    from packaging.version import Version
    if Version(version("openmetadata-ingestion")) != Version("2.0.5"):
        raise RuntimeError("native connector version does not match the admitted contract")
    from metadata.workflow.metadata import MetadataWorkflow
    from metadata.workflow.workflow_output_handler import WorkflowResultStatus
    cfg = yaml.safe_load(Path(path).read_text())
    contract_hash = connector_hash(cfg)
    fqn = cfg.pop("dppTableFqn")
    started = datetime.now(timezone.utc).isoformat()
    # Core durably records this physical invocation before allowing source IO.
    # An EOF/crash before RUN cannot execute the connector.
    print(json.dumps({"invocationStarted":True,"connectorHash":contract_hash,"externalFqn":fqn}), flush=True)
    if sys.stdin.readline().strip() != "RUN":
        raise RuntimeError("source scan was not authorized to start")
    def timeout(*_):
        raise TimeoutError("bounded native scan timeout")
    signal.signal(signal.SIGALRM, timeout)
    signal.alarm(85)
    with contextlib.redirect_stdout(sys.stderr):
        workflow = MetadataWorkflow.create(cfg)
        observations = []
        native_source = workflow.source.run
        workflow.source.run = lambda: observed_tables(native_source, fqn, observations)
        try:
            workflow.execute()
            workflow.raise_from_status()
            # Failed stages must not silently fall back to old catalog data.
            if workflow.result_status() != WorkflowResultStatus.SUCCESS:
                raise RuntimeError("native workflow did not complete successfully")
            if any(step.get_status().failures for step in workflow.workflow_steps()):
                raise RuntimeError("native workflow contains a failed step")
            if len(observations) != 1:
                raise RuntimeError("native workflow did not emit the exact source table once")
        finally:
            workflow.stop()
    connection = cfg["workflowConfig"]["openMetadataServerConfig"]
    headers = {"Authorization": "Bearer " + connection["securityConfig"]["jwtToken"]}
    from urllib.parse import quote
    req = urllib.request.Request(connection["hostPort"] + "/v1/tables/name/" + quote(fqn, safe=""), headers=headers)
    with urllib.request.urlopen(req, timeout=15) as response:
        table = json.load(response)
    columns = [{"name":c["name"],"type":c["dataType"],"constraint":c.get("constraint") or "UNKNOWN"} for c in table["columns"]]
    if table["fullyQualifiedName"] != fqn or columns != observations[0]:
        raise RuntimeError("catalog table differs from this native source observation")
    print(json.dumps({"externalId":table["id"],"externalFqn":table["fullyQualifiedName"],"connectorHash":contract_hash,
        "startedAt":started,"completedAt":datetime.now(timezone.utc).isoformat(),
        "columns":observations[0]}), flush=True)

if __name__ == "__main__":
    try:
        run(sys.argv[1])
    except Exception:
        # Native stderr may contain private connection diagnostics; callers keep
        # it private and publish only the platform's bounded error code.
        print("native schema scan failed", file=sys.stderr)
        traceback.print_exc(file=sys.stderr)
        sys.exit(1)
