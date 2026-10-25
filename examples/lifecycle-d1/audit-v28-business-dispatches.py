"""Fresh fixed Core-only audit of the sealed v28 P2/T1/T2; zero mutations."""
import importlib.util,json,pathlib,subprocess,time
ROOT=pathlib.Path(__file__).resolve().parents[2]
OUT=ROOT/'docs/acceptance/evidence/issue307-d1/post-v28-core-audit01'
def module(name,file):
 s=importlib.util.spec_from_file_location(name,ROOT/'examples/lifecycle-d1'/file);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
p=module('v28_audit_native','prepare-remaining-native.py');old=module('v28_audit_query','audit-old-business-dispatches.py')
EXECUTION='4fd5a7e7-779e-41d7-a35a-3bab7dd615b5'
IDS=[('eacd910a-3ecd-448e-9c87-13bdf14d8817','4d930776-fd98-4a86-8ed2-0a1eaad7dd34'),('002be431-180b-44c3-9f0b-747aaeaaa17a','4d5b1b64-fd97-4626-877f-09195725f11b'),('caade58d-9243-4bd3-b4c8-147d104858ca','fc9aca0e-dd87-40cc-8cbc-b629a8c9e2c4')]
old.CASES={name:(*v[:2],*ids) for (name,v),ids in zip(old.CASES.items(),IDS)}
def save(name,obj):
 with (OUT/name).open('x',encoding='utf-8',newline='\n') as f:json.dump(obj,f,indent=2);f.write('\n')
def main():
 OUT.mkdir(parents=True,exist_ok=False)
 packet=json.loads((ROOT/'docs/acceptance/evidence/issue307-d1/remaining-candidate18/candidate.json').read_bytes());g=packet['Groups'][0];cid=g['Core']
 x=p.inspect(cid);assert x['Id']==cid and not x['State']['Running'] and x['State']['Pid']==0
 save('intent.json',{'execution':EXECUTION,'Core':cid,'scope':'REPEATABLE READ READ ONLY fixed three copies and full fingerprint','native_started':False,'provider_calls':0,'deletion_authority':'NONE'})
 failure=None
 try:
  p.docker('start',cid);p.wait_pg(cid,'fixture')
  before=p.fingerprints(cid);save('Core-before.json',before)
  rows=[json.loads(s) for s in p.sql(cid,old.query()).splitlines()];save('actual-Core-rows.json',rows)
  targets=[]
  for row in rows:
   name=row['case'];_,copy,dispatch,attempt=old.CASES[name]
   a=next(v for v in row['attempts'] if v['id']==attempt);d=next(v for v in row['dispatches'] if v['id']==dispatch);approval=next(v for v in row['approvals'] if v['id']==d['approval_id'])
   assert a['dispatch_id']==dispatch and a['copy_id']==copy and approval['basis_ref']=='REMAINING_NON_DELETE_REHEARSAL:'+EXECUTION and approval['principal_ref']=='remaining-fixture:'+EXECUTION
   targets.append({'Case':name,'Plan':d['plan_id'],'Copy':copy,'Dispatch':dispatch,'Attempt':attempt,'Approval':approval['id'],'PlanHash':approval['plan_hash']})
  save('actual-v28-targets.json',targets)
  after=p.fingerprints(cid);save('Core-after.json',after);assert before==after
 except BaseException as ex:failure=type(ex).__name__+': '+str(ex)
 finally:
  p.docker('stop','-t','15',cid);x=p.inspect(cid)
  save('terminal.json',{'status':'PASS_READONLY_V28_CORE_AUDIT_SEALED' if failure is None else 'FAILED_READONLY_AUDIT_RETAINED','failure':failure,'Core_stopped':not x['State']['Running'] and x['State']['Pid']==0,'provider_calls':0,'native_delete':0,'business_mutations':0})
 if failure:raise RuntimeError(failure)
 print(json.dumps({'status':'PASS_READONLY_V28_CORE_AUDIT_SEALED','targets':targets}))
if __name__=='__main__':main()
