"""Create private native Connector configuration for this isolated source only."""
import argparse, hashlib, json
from pathlib import Path
from urllib.parse import urlparse
import yaml
from native_schema_scan import connector_hash

def prepare(runtime, metadata):
    state=json.loads(runtime.read_text()); private=Path(state["private"])
    cfg=json.loads((private/"test.json").read_text()); profile=cfg["Profile"]
    om=json.loads((metadata/"state.json").read_text())
    assert state["source"].startswith("dpp-pgi-") and profile["Database"]=="dpp_ingestion_source"
    source=urlparse(profile["SourceDSN"])
    fqn="dpp_source.dpp_ingestion_source.dpp_source.energy"
    connector={"dppTableFqn":fqn,"source":{"type":"postgres","serviceName":"dpp_source",
      "serviceConnection":{"config":{"type":"Postgres","username":"dpp_reader","authType":{"password":(private/"source-password").read_text()},"hostPort":f"host.docker.internal:{source.port}","database":"dpp_ingestion_source"}},
      "sourceConfig":{"config":{"type":"DatabaseMetadata","databaseFilterPattern":{"includes":["^dpp_ingestion_source$"]},"schemaFilterPattern":{"includes":["^dpp_source$"]},"tableFilterPattern":{"includes":["^energy$"]},"includeViews":False,"markDeletedTables":False,"includeTags":False,"includeStoredProcedures":False}}},
      "sink":{"type":"metadata-rest","config":{}},"workflowConfig":{"loggerLevel":"ERROR","openMetadataServerConfig":{"hostPort":f"http://host.docker.internal:{urlparse(om['url']).port}/api","authProvider":"openmetadata","securityConfig":{"jwtToken":(metadata/"admin-token").read_text()}}}}
    path=private/"native-schema.yaml";path.write_text(yaml.safe_dump(connector))
    command=["docker","run","-i","--label","dpp.trial.run="+state["run_id"],"--network",om["project"]+"_app_net","--mount",f"type=bind,src={path},dst=/run/credentials/native-schema.yaml,readonly","--mount",f"type=bind,src={Path(__file__).parent.resolve()},dst=/adapter,readonly","dpp-c2-metadata-ingestion:2.0.5","/run/credentials/native-schema.yaml"]
    (private/"native-schema-command.json").write_text(json.dumps(command))
    facts=json.loads((private/"facts.json").read_text())
    trusted=json.loads((private/"profile.json").read_text())
    target=next(x for x in trusted["Targets"] if x["code"]=="energy_raw")
    rule_path=Path(__file__).resolve().parents[3]/"industry-packs/park/source-observation/energy-schema-v1.json"
    rule=json.loads(rule_path.read_text())
    assert [c["name"] for c in rule["columns"]]==next(t["Columns"] for t in trusted["Tables"] if t["Code"]=="energy_raw")
    plan={"enabled":True,"workspaceId":state["workspaceId"],"resourceId":target["resourceId"],"actorId":state["actorId"],"authorizationId":facts["Authorization"],"consumerRef":"LICENSED_BANK","purpose":"ENTERPRISE_CREDIT_RISK_SUPPORT","externalFqn":fqn,"ruleRef":rule["ruleRef"],"ruleVersion":rule["version"],"ruleHash":hashlib.sha256(json.dumps(rule,sort_keys=True,separators=(",",":")).encode()).hexdigest(),"connectorHash":connector_hash(connector),"columns":rule["columns"]}
    command=command[:2]+["--name","dpp-c2-schema-{attemptId}"]+command[2:]
    profile_path=private/"schema-profile.json"
    profile_path.write_text(json.dumps({"plan":plan,"command":command,"stopCommand":["docker","stop","--timeout","20","dpp-c2-schema-{attemptId}"]}))
    envs=json.loads((private/"environment.json").read_text());envs["api"]["OPENMETADATA_SCHEMA_PROFILE"]=str(profile_path)
    (private/"environment.json").write_text(json.dumps(envs))
    print(json.dumps({"configuration":"private","fqn":fqn,"reader":"dpp_reader","nativeVersion":"2.0.5"}))

if __name__=="__main__":
    p=argparse.ArgumentParser();p.add_argument("--runtime",type=Path,required=True);p.add_argument("--metadata",type=Path,required=True);a=p.parse_args();prepare(a.runtime,a.metadata)
