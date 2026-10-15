import importlib.util,json,uuid
from pathlib import Path
root=Path('C:/Users/Henry/.codex/worktrees/9400/data-product-platform')
spec=importlib.util.spec_from_file_location('retained',root/'examples/lifecycle-d1/complete-lc13-readonly.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
out=root/'docs/acceptance/evidence/issue307-d1/delete-candidate05-preparation01'
m.CONTROL=Path('C:/Users/Henry/.codex/fixture-private/dpp-lifecycle307-5d2274ee/reference-disposal-execution02')
native=m.load(out/'native/native-six-version.json');ids=[]
for value in native['versions']:ids.extend([*value['protection']['CallIDs'],value['read_call_id']])
ids.extend(x['CallID'] for x in native['inventories'].values());m.require(len(set(ids))==20,'Exact20 call identities required')
values=','.join("'"+str(uuid.UUID(x))+"'" for x in ids)
q="SELECT jsonb_build_object('calls',(SELECT jsonb_agg(to_jsonb(c) ORDER BY c.id) FROM lifecycle_storage_call c WHERE c.id IN("+values+")),'observations',(SELECT jsonb_agg(to_jsonb(o) ORDER BY o.call_id) FROM lifecycle_storage_observation o WHERE o.call_id IN("+values+")),'allocations',(SELECT jsonb_agg(to_jsonb(a) ORDER BY a.lifecycle_storage_call_id) FROM cost_allocation a WHERE a.lifecycle_storage_call_id IN("+values+")),'costs',(SELECT jsonb_agg(to_jsonb(e) ORDER BY e.activity_id) FROM cost_event e JOIN cost_allocation a ON a.cost_event_id=e.id WHERE a.lifecycle_storage_call_id IN("+values+")))::text;"
checkpoint=m.load(out/'preflight/checkpoint.json');ident=checkpoint['id'];item=m.inspect(ident)
m.require(item['Name']=='/'+checkpoint['container'] and item['Image']==m.PGIMAGE and item['State']['Status']=='exited' and any(x.get('Name')==checkpoint['volume'] for x in item['Mounts']),'Checkpoint identity drift')
m.validate_source()
try:
 m.docker('start',ident);m.wait_audit_core(ident);before=m.fingerprint(ident)
 facts=json.loads(m.source_read(ident,q))
 m.require(all(len(facts[k])==20 for k in ['calls','observations','allocations','costs']),'Exact read accounting incomplete')
 m.require({x['id'] for x in facts['calls']}=={x['call_id'] for x in facts['observations']}=={x['lifecycle_storage_call_id'] for x in facts['allocations']}=={x['activity_id'] for x in facts['costs']}==set(ids),'Read accounting identity mismatch')
 m.require(all(x['quantity']==1 and x['unit']=='invocation' for x in facts['costs']),'Read invocation quantity wrong')
 m.require(m.fingerprint(ident)==before,'Read-only checkpoint ledger audit mutated facts')
 m.write(out/'preflight/native-read-ledger.json',{'status':'PASS_EXACT_20_TYPED_READ_INVOCATIONS','checkpoint_id':ident,'facts':facts,'source_starts':0,'provider_io':0,'db_business_mutation':0,'delete':0})
 (out/'preflight/native-read-ledger.sql').write_text('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;\n'+q+'\nROLLBACK;\n',encoding='utf-8',newline='\n')
finally:
 if m.inspect(ident)['State']['Running']:m.docker('stop',ident)
 m.write(out/'preflight/native-read-ledger-sealed.json',{'checkpoint_id':ident,'status':m.inspect(ident)['State']['Status'],'pid':m.inspect(ident)['State']['Pid'],'source_pg':m.inspect(m.PG)['State']['Status'],'source_minio':m.inspect(m.MINIO)['State']['Status'],'source_gateway':m.inspect(m.GW)['State']['Status'],'cleanup':0})
print(json.dumps({'status':'PASS_EXACT20_READ_LEDGER','source_starts':0,'provider_io':0,'delete':0}))
