import hashlib,json,subprocess,sys,uuid,urllib.request
from pathlib import Path
root=Path(__file__).resolve().parent;repo=root.parents[1]
sys.path.insert(0,str(repo/'examples/enterprise-activity/postgres-ingestion'))
from c2_verify import C2Oracle
runtime=repo/'.data/independent299-runtime/runtime.json'
o=C2Oracle(runtime)
before={t:hashlib.sha256(json.dumps(o.rows(t),sort_keys=True).encode()).hexdigest() for t in ['source_schema_request','source_schema_attempt','source_schema_observation','source_schema_handling','cost_event','cost_allocation','evidence','audit_event']}
requests=o.rows('source_schema_request');handling=o.rows('source_schema_handling')[0]
old=next(r for r in requests if o.rows('source_schema_observation',f"request_id='{r['id']}' AND status='MATCH'"))
responses=[o.command(o.base,key='revoked-new-'+uuid.uuid4().hex)[0],o.command(o.base,key=old['request_key'])[0],
 o.command(f"/data-resources/{o.resource}/schema-observations/{handling['observation_id']}/handle?workspaceId={o.ws}",key=handling['request_key'],body={'reason':handling['reason']})[0]]
assert responses==[403,403,403],responses
after={t:hashlib.sha256(json.dumps(o.rows(t),sort_keys=True).encode()).hexdigest() for t in before}
assert before==after,'revoked replays added business facts/cost'
denied=o.rows('source_schema_observation',"request_id='8f8580ed-a177-4252-8983-662a6e2adc83'")
assert len(denied)==1 and denied[0]['status']=='DENIED' and denied[0]['attempt_id'] is None
assert not o.rows('source_schema_attempt',"request_id='8f8580ed-a177-4252-8983-662a6e2adc83'")
assert len(o.rows('source_schema_attempt'))==int((root/'scans-before-revoke.txt').read_text())==11
facts=json.loads((o.private/'facts.json').read_text())
assert o.rows('data_authorization',f"id='{facts['Authorization']}'")[0]['status']=='REVOKED'
cfg=json.loads((o.private/'test.json').read_text());env=json.loads((o.private/'environment.json').read_text())
secretValues={o.token,cfg['RootKey'],cfg['RootSecret']}
for name in ['source-password','hop-password','core-password']:
 secretValues.add((o.private/name).read_text().strip())
sourcePrivate=Path(o.state['sourcePrivate'])
secretValues.add((sourcePrivate/'admin-password').read_text().strip())
for group in env.values():
 for key,value in group.items():
  if isinstance(value,str) and (key.endswith('_TOKEN') or key.endswith('_SECRET_KEY') or key.endswith('_ACCESS_KEY')):secretValues.add(value)
for credential in ['core-identity.json','publisher-identity.json']:
 for key,value in json.loads((o.private/credential).read_text()).items():
  if isinstance(value,str) and key.lower() in ['accesskey','secretkey','sessiontoken']:secretValues.add(value)
secretValues={v for v in secretValues if len(v)>10}
dump=subprocess.check_output(['docker','exec',o.container,'pg_dump','-U','postgres','-d','dpp_ingestion_core','--data-only'])
assert all(secret.encode() not in dump for secret in secretValues),'credential in Core fact dump'
pages=[]
for path in [f'/resources/{o.resource}',f'/datasets/8c1eea3f-f0cd-4582-8f64-6d914c77b2b3/versions/7af0c9c5-95e6-4dcb-b9c3-2b3fcc44e829/remediation',f'/datasets/28c66686-f499-4bff-b1e6-80ca682316d5/versions/77598be5-83c5-43a6-b91f-081b829806ca?view=eligibility']:
 pages.append(urllib.request.urlopen(o.state['web']+path,timeout=20).read())
assert all(secret.encode() not in page for secret in secretValues for page in pages),'credential in production HTML'
initial=json.loads((root/'backup-initial.json').read_text())['databases']['dpp_ingestion_source']['before']
restored={}
for table in initial:
 raw=subprocess.check_output(['docker','exec',o.state['source'],'psql','-U','postgres','-d','dpp_ingestion_source','-At','-c',f'SELECT row_to_json(t)::text FROM {table} t ORDER BY row_to_json(t)::text'])
 rows=raw.splitlines();restored[table]={'rows':len(rows),'sha256':hashlib.sha256(b'\n'.join(rows)).hexdigest()}
assert initial==restored,'source drift restoration changed original rows'
result={'status':'PASS','revokedHttpStatuses':responses,'queuedScan':denied[0],'physicalScanCount':11,'newScanInvocationsAfterRevoke':0,'newBusinessFactsAfterRevokedReplay':0,'knownCredentialValuesChecked':len(secretValues),'coreFactsCredentialLeaks':0,'productionPagesCredentialLeaks':0,'sourceRestoreFingerprints':restored}
(root/'final-checks.json').write_text(json.dumps(result,indent=2),encoding='utf-8')
print(json.dumps({'status':'PASS','revokedHttpStatuses':responses,'physicalScanCount':11,'newScanInvocationsAfterRevoke':0,'credentialValuesChecked':len(secretValues),'sourceTablesUnchanged':3}))
