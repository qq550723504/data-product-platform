import sys,pathlib,json,hashlib,tempfile,copy,unittest.mock,subprocess,importlib.util
root=pathlib.Path(__file__).resolve().parents[6];sys.path.insert(0,str(root/'examples/lifecycle-d1'))
from test_remaining_six_window import SixWindow,save,sha
spec=importlib.util.spec_from_file_location('independent_complete05',root/'examples/lifecycle-d1/verify-six-native.py');v=importlib.util.module_from_spec(spec);spec.loader.exec_module(v)
class ReachedState(RuntimeError):pass
blocked=[]
class DeniedPopen(subprocess.Popen):
 def __init__(self,*a,**kw):blocked.append(str(a));raise AssertionError('No unmodeled process permitted')
subprocess.Popen=DeniedPopen
rows=[]
for fault in ['positive','predecessor','foreign','missing','extra','checkpoint']:
 with tempfile.TemporaryDirectory() as t:
  temp=pathlib.Path(t);packet,owner,c,windows=SixWindow().fixture(temp);head=c['Groups'][0]['PolicyHeads'];key=next(iter(head))
  if fault=='predecessor':head[key]=next(x['supersedes_policy_id'] for x in v.p.window.policy_checkpoint()['policies'] if x['dataset_version_id']==key)
  elif fault=='foreign':head[key]='11111111-1111-4111-8111-111111111111'
  elif fault=='missing':del head[key]
  elif fault=='extra':head['76f7f44e-4a2a-46c6-a6dc-a529f9ade834']='310853bb-939b-4ba1-900c-33a4ae0e9a58'
  elif fault=='checkpoint':c['PolicyCheckpointSHA256']='f'*64
  row=windows['cases'][0];file=owner/row['group']/('case-'+row['stage'])/'remaining.json';cfg=json.loads(file.read_bytes());cfg['ExpectedPolicyHeads']=copy.deepcopy(head);save(file,cfg);row['config_sha256']=sha(file);c['Groups'][0]['CaseConfigSHA256'][row['stage']]=sha(file)
  save(packet/'profile-windows.json',windows);c['ProfileWindowsSHA256']=sha(packet/'profile-windows.json');save(packet/'candidate.json',c);save(packet/'bindings.json',[]);(owner/'readonly-check.exe').write_bytes(b'NON_EXECUTABLE_TEST_ONLY')
  output=temp/'docs/acceptance/evidence/issue307-d1/output';calls=[]
  def state():calls.append('state');raise ReachedState('actual CLI completed real semantic gate before state')
  argv=['verify-six-native.py','--candidate-sha256',sha(packet/'candidate.json'),'--bindings-sha256',sha(packet/'bindings.json'),'--checker-sha256',sha(owner/'readonly-check.exe'),'--output',str(output)]
  with unittest.mock.patch.object(v.sealed_gate,'__defaults__',(packet,)),unittest.mock.patch.object(v,'PACKET',packet),unittest.mock.patch.object(v.r,'CONTROL',owner),unittest.mock.patch.object(v.r,'ROOT',temp),unittest.mock.patch.object(v.r,'state',state),unittest.mock.patch.object(sys,'argv',argv):
   try:v.main();raise AssertionError('entry returned unexpectedly')
   except ReachedState:
    if fault!='positive':raise
   except ValueError:
    if fault=='positive':raise
  assert calls==(['state'] if fault=='positive' else []),(fault,calls)
  assert not output.exists(),fault
  rows.append({'fault':fault,'state_calls':len(calls),'output_created':False,'recomputed_candidate_sha256':sha(packet/'candidate.json'),'result':'PASS'})
assert not blocked
print(json.dumps({'status':'PASS_ACTUAL_COMPLETE_MAIN_POLICY_GATE','cases':rows,'unmodeled_processes':blocked,'native':False}))
