import copy,datetime as dt,hashlib,importlib.util,json,subprocess,sys,tempfile
from contextlib import ExitStack
from pathlib import Path
from unittest.mock import patch
sys.dont_write_bytecode=True
R=Path(__file__).resolve().parents[5];D=Path(__file__).parent;sys.path.insert(0,str(R/'examples/lifecycle-d1'))
def refuse(*a,**k):raise RuntimeError('unmodeled subprocess refused')
class RefusedPopen(subprocess.Popen):
    def __init__(self,*a,**k):refuse(*a,**k)
subprocess.run=refuse;subprocess.Popen=RefusedPopen
def module(name,file):
 spec=importlib.util.spec_from_file_location(name,R/'examples/lifecycle-d1'/file);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
w=module('independent_complete_support','window_six_support.py');v=w.v;r=w.r;fixture=module('independent_window_fixture','test_remaining_six_window.py')
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest();rejected=[]
for kind in ('candidate-end','actual-use-until','actual-group','actual-head','base-end','recovery-end'):
 with tempfile.TemporaryDirectory() as temp,ExitStack() as stack:
  packet,owner,c,rows=fixture.SixWindow().fixture(Path(temp));fixture.save(packet/'bindings.json',[])
  row=rows['cases'][0];path=owner/row['group']/('case-'+row['stage'])/'remaining.json';cfg=json.loads(path.read_bytes())
  if kind=='candidate-end':c['NotAfter']='2026-10-20T06:41:00Z'
  elif kind=='recovery-end':
   rec=rows['recoveries'][0];rec['ValidUntil']='2026-10-20T08:41:00Z';q=owner/rec['group']/rec['stage']/'recovery.json';fixture.save(q,{'ValidUntil':rec['ValidUntil']});rec['config_sha256']=sha(q)
  else:
   if kind=='actual-use-until':cfg['CandidateUseUntil']='2026-10-20T06:41:00Z'
   elif kind=='actual-group':cfg['Group']='mixed'
   elif kind=='actual-head':cfg['ExpectedPolicyHeads']['0']='foreign-head'
   elif kind=='base-end':cfg['Base']['ExpiresAt']='2026-10-20T06:41:00Z'
   fixture.save(path,cfg);row['config_sha256']=sha(path);c['Groups'][0]['CaseConfigSHA256'][row['stage']]=sha(path)
   if kind=='base-end':row['Base']['ExpiresAt']=cfg['Base']['ExpiresAt']
  fixture.save(packet/'profile-windows.json',rows);c['ProfileWindowsSHA256']=sha(packet/'profile-windows.json');fixture.save(packet/'candidate.json',c)
  stack.enter_context(patch.object(r,'CONTROL',owner));stack.enter_context(patch.object(r,'state',lambda: (_ for _ in ()).throw(AssertionError('runtime/permission/Docker gate reached'))))
  try:v.sealed_gate(packet)
  except ValueError:rejected.append(kind)
  else:raise AssertionError(kind)
# Positive cross-field packet reaches the subsequent state gate. Permission
# and Docker remain blocked by the sentinel.
with tempfile.TemporaryDirectory() as temp,ExitStack() as stack:
 packet,owner,c,rows=fixture.SixWindow().fixture(Path(temp));fixture.save(packet/'bindings.json',[])
 stack.enter_context(patch.object(r,'CONTROL',owner));calls=[]
 def state():calls.append('runtime-state-after-semantics');raise RuntimeError('intentional subsequent gate sentinel')
 stack.enter_context(patch.object(r,'state',state))
 try:v.sealed_gate(packet)
 except RuntimeError:pass
 assert calls==['runtime-state-after-semantics']
# Exercise inherited full seal with current stop override. Failure of one
# group's IAM and gateway seal cannot skip other groups/final known CIDs.
with tempfile.TemporaryDirectory() as temp,ExitStack() as stack:
 private=Path(temp);control=private/'authorized';control.mkdir()
 for obj,name,value in ((w,'PRIVATE',private),(w,'CONTROL',control),(w.base,'PRIVATE',private),(w.base,'CONTROL',control)):
  stack.enter_context(patch.object(obj,name,value))
 at=w.p.window.instant(w.p.W['NotBefore'])+dt.timedelta(minutes=1)
 stack.enter_context(patch.object(w.base,'now',lambda:at));stack.enter_context(patch.object(w.base,'START',w.p.window.instant(w.p.W['NotBefore'])))
 original=r.load(w.p.p1.PACKET/'candidate.json');groups=copy.deepcopy(original['Groups'])
 bindings=[]
 for group in groups:
  group['Helpers']={k:v for k,v in group['Helpers'].items() if k not in ('P1','recover-P1')}
  group['CaseHelpers']={k:v for k,v in group['CaseHelpers'].items() if k in w.p.window.CASE_MODES[group['Group']]}
  roots={action:{'container':hashlib.sha256((group['Group']+action).encode()).hexdigest()} for action in ('check-disabled','enable-read','enable-send','disable-send','disable-all')}
  bindings.append({'group':group['Group'],'root_owners':roots,'recovery_redis':hashlib.sha256((group['Group']+'queue').encode()).hexdigest()})
 events=[];records=[]
 stack.enter_context(patch.object(w.base,'disable_json',lambda path,archive,**kw:events.append(('revoke',str(path))) or {'already_disabled':True}))
 stack.enter_context(patch.object(w.base,'exclusive',lambda path,value:records.append(copy.deepcopy(value))))
 class ModelHost(w.Host):
  def __init__(self):self.candidate={'Groups':groups};self.bindings=bindings;self.inspect_count={}
  def inspect(self,cid):
   self.inspect_count[cid]=self.inspect_count.get(cid,0)+1
   if cid==groups[0]['Helpers']['wire'] and self.inspect_count[cid]==1:raise OSError('recorded initial readback unavailable')
   return {'State':{'Running':False,'Pid':0},'HostConfig':{'RestartPolicy':{'Name':'no'},'PortBindings':{}},'NetworkSettings':{'Ports':{}}}
  def docker(self,*args,**kw):events.append(tuple(args));return b''
  def stop(self,cid):events.append(('stop-step',cid));return super().stop(cid)
  def disable_all(self,g):
   events.append(('disable-all',g['Group']))
   if g['Group']=='business-races':raise OSError('recorded IAM failure')
   return {'identity_disable_confirmed':True,'policy_validation_passed':True,'archive':{'complete':True}}
  def seal_gateway(self,g):
   events.append(('seal-gateway',g['Group']))
   if g['Group']=='business-races':raise OSError('recorded archive failure')
   return {'gateway_stopped_and_sealed':True}
 out=ModelHost().seal();assert out['all_steps_confirmed'] is False
 assert [x[1] for x in events if x[0]=='disable-all']==list(w.p.GROUPS)
 assert [x[1] for x in events if x[0]=='seal-gateway']==list(w.p.GROUPS)
 expected={cid for g,b in zip(groups,bindings) for cid in [*g['Helpers'].values(),*g['CaseHelpers'].values(),g['Gateway'],g['Native'],g['Core'],b['recovery_redis'],*[x['container'] for x in b['root_owners'].values()]]}
 stopped={x[1] for x in events if x[0]=='stop-step'};assert expected<=stopped
 assert ('stop','--time','15',groups[0]['Helpers']['wire']) in events
 assert [x['step'] for x in out['steps'] if not x['ok']]==['disable-all:business-races','seal-gateway:business-races']
 seal={'all_steps':len(out['steps']),'failed_steps':[x['step'] for x in out['steps'] if not x['ok']],'all_groups_attempted':True,'all_expected_CIDs_reached':len(expected),'first_inspect_failure_still_fixed_stop':True,'all_steps_confirmed':out['all_steps_confirmed']}
result={'at':dt.datetime.now(dt.timezone.utc).isoformat(),'source_files':{n:sha(R/'examples/lifecycle-d1'/n) for n in ('verify-six-native.py','window_six_support.py','remaining_window.py','remaining_six_window.py')},'rehash_semantic_counterexamples_rejected_before_runtime_state':rejected,'positive_reached_only_subsequent_state':True,'seal_model':seal,'actual_Docker_source_provider':0,'scope':'Synthetic profiles/candidates and full containment methods; no original auth or proof signing'}
p=D/'independent-entry-semantics-seal04.json'
with p.open('x',encoding='utf-8',newline='\n') as f:json.dump(result,f,indent=2);f.write('\n')
print(json.dumps({'receipt':str(p),'sha256':sha(p),'semantic_rejections':len(rejected),'seal':seal}))
