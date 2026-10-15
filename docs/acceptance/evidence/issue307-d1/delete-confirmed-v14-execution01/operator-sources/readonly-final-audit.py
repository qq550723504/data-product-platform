import datetime as dt,hashlib,importlib.util,json
from pathlib import Path
p=Path('C:/Users/Henry/.codex/fixture-private/d1-confirmed-v14-execution01.py');s=importlib.util.spec_from_file_location('op',p);o=importlib.util.module_from_spec(s);s.loader.exec_module(o)
q="""SELECT jsonb_build_object(
'as_of',now(),'basis',(SELECT jsonb_agg(to_jsonb(x)) FROM lifecycle_fixture_disposal_basis x WHERE run_id='614cd9ad-3f9d-40be-b110-3f7cf95f0790'),
'observations',(SELECT jsonb_agg(to_jsonb(x)) FROM lifecycle_provider_observation x),
'attempts',(SELECT jsonb_agg(to_jsonb(x)) FROM lifecycle_provider_attempt x),
'claims',(SELECT jsonb_agg(to_jsonb(x)) FROM lifecycle_provider_send_claim x),
'releases',(SELECT jsonb_agg(to_jsonb(x)) FROM lifecycle_dispatch_release x),
'results',(SELECT jsonb_agg(to_jsonb(x)) FROM lifecycle_reconciliation_result x),
'absence',(SELECT count(*) FROM lifecycle_copy_absence_receipt),'completion',(SELECT count(*) FROM lifecycle_plan_completion),
'storage_calls',(SELECT count(*) FROM lifecycle_storage_call),'storage_observations',(SELECT count(*) FROM lifecycle_storage_observation),
'storage_costs',(SELECT count(*) FROM cost_allocation WHERE lifecycle_storage_call_id IS NOT NULL),
'provider_costs',(SELECT jsonb_agg(jsonb_build_object('allocation',to_jsonb(a),'cost',to_jsonb(e))) FROM cost_allocation a JOIN cost_event e ON e.id=a.cost_event_id WHERE a.lifecycle_provider_attempt_id IS NOT NULL),
'unaccounted_storage',(SELECT count(*) FROM lifecycle_storage_observation x WHERE x.invoked_at IS NOT NULL AND (SELECT count(*) FROM cost_allocation a JOIN cost_event e ON e.id=a.cost_event_id WHERE a.lifecycle_storage_call_id=x.call_id AND e.activity_id=x.call_id AND e.quantity=1 AND e.unit='invocation')<>1),
'old_outbox_states',(SELECT jsonb_agg(to_jsonb(x)) FROM (SELECT event_type,status,count(*) FROM outbox_event WHERE created_at<'2026-10-15T08:37:14Z' GROUP BY event_type,status ORDER BY event_type,status)x));"""
v=json.loads(o.m.source_read(o.m.PG,q));o.need(v['absence']==v['completion']==0 and len(v['claims'])==1 and len(v['attempts'])==3,'Unexpected send/absence/completion facts')
observed=[x for x in v['observations'] if x['attempt_id']=='c163dca9-4650-4efe-bdef-e3380bdc9da0'];o.need(len(observed)==1 and observed[0]['certainty']=='CONFIRMED' and observed[0]['outcome']=='DENIED','Original immutable denial lost')
o.need(v['storage_calls']==v['storage_observations']==v['storage_costs'] and v['unaccounted_storage']==0 and len(v['provider_costs'])==1,'Physical call/cost accounting mismatch')
o.need(v['provider_costs'][0]['cost']['quantity']==1 and v['provider_costs'][0]['cost']['unit']=='invocation','Physical denial cost missing')
o.need(v['old_outbox_states']==o.read(o.PUBLIC/'outbox-before.json'),'Old business queue event state changed')
o.need(len(v['releases'])==3 and any(x['terminal'] and x['outcome']=='ALREADY_RESOLVED' for x in v['results']),'Denied positive-presence reservation resolution incomplete')
ob=observed[0];stamp=dt.datetime.fromisoformat(ob['observed_at']).astimezone(dt.timezone.utc).isoformat().replace('+00:00','Z');http=[]
for code in (401,403):
 b=json.dumps({'Attempt':ob['attempt_id'],'Status':code,'Outcome':ob['outcome'],'Observed':stamp},separators=(',',':')).encode()
 if hashlib.sha256(b).hexdigest()==ob['proof_sha256']:http.append(code)
v['physical_denial_http_status_from_immutable_proof']=http
if not (o.PUBLIC/'final-core-readback.json').exists():o.write(o.PUBLIC/'final-core-readback.json',v)
before=o.read(o.PUBLIC/'preflight/source-before.json');after=o.m.fingerprint(o.m.PG);changed=[x['relation'] for x in after if x!=next(b for b in before['relations'] if b['relation']==x['relation'])]
allowed={'audit_event','evidence','evidence_relation','command_idempotency','outbox_event','outbox_event_consumption','cost_event','cost_allocation','delivery_authorization_fence'}
o.need(all(x in allowed or x.startswith('lifecycle_') for x in changed),'Unrelated business relation changed: '+repr(changed))
o.need(o.m.role_acl(o.m.PG)==before['roles_acl'],'Source roles/ACL changed')
o.write(o.PUBLIC/'source-after.json',{'at':dt.datetime.now(dt.timezone.utc).isoformat(),'relations':after,'changed_relations':changed,'old_business_relations_unchanged':True,'source_roles_acl_unchanged':True,'old_outbox_states_unchanged':True})
print(json.dumps({'status':'PASS_DENIED_PRESENT_READONLY_RECONCILIATION','attempted_exact_delete':1,'denial_http_status':http,'successful_delete':0,'b_started':False,'storage_calls':v['storage_calls'],'provider_invocation_costs':len(v['provider_costs']),'absence':v['absence'],'completion':v['completion'],'unaccounted':0,'changed_relations':changed}))
