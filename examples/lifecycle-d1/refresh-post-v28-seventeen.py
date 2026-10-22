"""New read authority after v28 terminal and trusted dispatch closure.

Reuse frozen c18 reader/checker and the complete collector. This is neither a
resume of that terminal execution nor a use of its expired DELETE grant.
"""
import argparse,datetime as dt,hashlib,importlib.util,json,subprocess,uuid
from pathlib import Path
def module(name,file):
 s=importlib.util.spec_from_file_location(name,Path(__file__).with_name(file));m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
v=module('post28_complete_read','verify-six-native.py');c=module('post28_maintenance_history','close-v28-business-dispatches.py');r,p=v.r,v.p
ROOT=r.ROOT;EVIDENCE=ROOT/'docs/acceptance/evidence/issue307-d1';PUBLIC=EVIDENCE/'post-v28-seventeen01';PRIVATE=c.PRIVATE/'post-v28-seventeen01'
need,load,sha,save=r.need,r.load,r.sha,r.save

def gate():
 candidate,bindings=c.packet();terminal=load(c.PUBLIC/'terminal.json')
 need(terminal['status']=='PASS_V28_MIXED_CLOSURE_APPEND_ONLY_SEALED' and terminal['retained_unchanged'] and terminal['native_delete']==terminal['pins_released']==terminal['budgets_refunded']==0,'trusted closure not complete')
 need(p.EXECUTION==c.EXECUTION and p.PROFILE=='candidate18/v28' and r.CONTROL==c.OLD,'new audit must explicitly reuse unchanged frozen read runtime')
 for name,digest in [('reader',candidate['ReaderBinarySHA256']),('readonly-check.exe','b05d9f331cb2126176021db402b189678a7b0a0c1aaa8776612e5751de5b7081')]:need(sha(r.CONTROL/name)==digest,'read binary changed')
 need(load(c.CONTROL/'worker-private/closure.json')['Enabled'] is False,'maintenance still active')
 return candidate,bindings

def baselines():
 return {g:dict(path=str(path),sha256=sha(path)) for g,path in [('business-races',c.PUBLIC/'final-Core-fingerprint.json'),('late-hold',c.PACKET/'late-hold/core-readonly.json'),('mixed',c.PACKET/'mixed/core-readonly.json')]}

def snapshot():
 state=c.snapshot();owners=[x['cid'] for x in load(c.CONTROL/'manifest.json')['owners'].values()]
 need(all(c.closed(c.inspect(cid)) for cid in owners),'maintenance owner active')
 state['maintenance_files']={str(x):sha(x) for x in c.CONTROL.rglob('*') if x.is_file() and x.name!='host-key.private'}
 state['maintenance_owners']=owners;return state

def code_inputs():return {x.name:sha(x) for x in Path(__file__).parent.glob('*.py')}

def prepare():
 need(not PRIVATE.exists() and not PUBLIC.exists(),'one shot read audit already prepared');gate();preservation=snapshot();PRIVATE.mkdir();PUBLIC.mkdir()
 plan=dict(status='PREPARED_POST_V28_GET_HEAD_ONLY',source_head=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT).decode().strip(),code_inputs=code_inputs(),historical_execution=c.EXECUTION,deletion_authority='NONE',old_authorization_reused=False,max_provider_deletes=0,baselines=baselines(),preservation=preservation)
 save(PUBLIC/'plan.json',plan);save(PRIVATE/'plan.json',plan);print(json.dumps(dict(status=plan['status'],plan_sha256=sha(PUBLIC/'plan.json'))),flush=True)

def run(plan_sha):
 need(not (PRIVATE/'terminal.json').exists() and not (PUBLIC/'terminal.json').exists(),'already terminal')
 need(sha(PUBLIC/'plan.json')==plan_sha==sha(PRIVATE/'plan.json'),'plan drift');plan=load(PUBLIC/'plan.json');candidate,bindings=gate()
 need(plan['baselines']==baselines() and plan['code_inputs']==code_inputs(),'audit code/baseline drift')
 results=[];failure=None;deadline=r.now()+dt.timedelta(minutes=50)
 try:
  need(snapshot()==plan['preservation'],'retained before read drift')
  for g,b in zip(candidate['Groups'],bindings):results.append(v.collect(g,b,c.PACKET,PRIVATE/g['Group'],PUBLIC/g['Group'],deadline,core_baseline=Path(plan['baselines'][g['Group']]['path'])))
  need([x['copies'] for x in results]==[7,6,4],'17 versions incomplete')
 except BaseException as e:failure=type(e).__name__+': '+str(e)
 finally:
  closure=[p.p1.close_known(cid) for g in candidate['Groups'] for cid in (g['Core'],g['Native'])]
  try:
   state=snapshot();save(PUBLIC/'preservation-after.json',state);need(state==plan['preservation'] and all(x['closed'] for x in closure),'retained/containment drift')
  except BaseException as e:failure=failure or type(e).__name__+': '+str(e)
  journals={}
  for g in candidate['Groups']:
   files=PUBLIC/g['Group']/'files';starts={x.stem[len('readonly-start-'):] for x in files.glob('readonly-start-*.json')};observations={x.stem[len('readonly-observation-'):] for x in files.glob('readonly-observation-*.json')};journals[g['Group']]=dict(starts=len(starts),observations=len(observations),unobserved_attempts=sorted(starts-observations))
  terminal=dict(status='PASS_POST_V28_SEVENTEEN_NATIVE_READONLY_SEALED' if failure is None else 'FAILED_POST_V28_READ_AUDIT_RETAINED',at=r.now().isoformat(),error=failure,plan_sha256=plan_sha,baselines=plan['baselines'],historical_execution=c.EXECUTION,deletion_authority='NONE',old_authorization_reused=False,groups=results,present_versions=sum(x['copies'] for x in results),technical_requests=sum(x['starts'] for x in journals.values()),request_journals=journals,amount='NOT_ASSESSED',closure=closure,native_delete=0,business_mutations=0,pins_released=0,budgets_refunded=0,D1='NOT_ACCEPTED')
  save(PUBLIC/'terminal.json',terminal);save(PRIVATE/'terminal.json',terminal)
 print(json.dumps({k:terminal[k] for k in ('status','error','present_versions','technical_requests','native_delete')}),flush=True);need(failure is None,failure or 'read audit failed')

if __name__=='__main__':
 parser=argparse.ArgumentParser();parser.add_argument('action',choices=['prepare','run']);parser.add_argument('--plan-sha256');a=parser.parse_args()
 if a.action=='prepare':prepare()
 else:need(a.plan_sha256,'plan hash required');run(a.plan_sha256)
