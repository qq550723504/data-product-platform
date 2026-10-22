from pathlib import Path
import hashlib,importlib.util,json,tempfile,unittest
from unittest.mock import patch
ROOT=Path(r'C:\Users\Henry\.codex\worktrees\9400\data-product-platform')
PUBLIC=ROOT/'docs/acceptance/evidence/issue307-d1'
CONTROL=Path(r'C:\Users\Henry\.codex\fixture-private\remaining-candidate10\authorized-execution01')
spec=importlib.util.spec_from_file_location('reviewed_window_support',PUBLIC/'remaining-window-review04/composition/window_support.py');w=importlib.util.module_from_spec(spec);spec.loader.exec_module(w)
AUTH=json.loads((PUBLIC/'delete-confirmed-v20-execution01/human-authorization.json').read_bytes())
C=json.loads((PUBLIC/'remaining-candidate10/candidate.json').read_bytes());B=json.loads((PUBLIC/'remaining-candidate10/bindings.json').read_bytes())
ALLOWED={name:(CONTROL/name).read_bytes() for name in ['user-message-verbatim.txt','approved-referenced-template.txt','authorization-envelope.txt']}
class AuthorizationChecks(unittest.TestCase):
 def run_check(self,mutate_auth=None,mutate_file=None,permission=False):
  with tempfile.TemporaryDirectory(prefix='auth-review-',dir=Path(__file__).parent) as name:
   temp=Path(name); owners=temp/'owners'; control=temp/'control';control.mkdir()
   for file,raw in ALLOWED.items():(control/file).write_bytes(raw)
   auth=json.loads(json.dumps(AUTH))
   if mutate_auth:mutate_auth(auth)
   if mutate_file:
    file,raw=mutate_file;(control/file).write_bytes(raw)
   authpath=temp/'auth.json';authpath.write_text(json.dumps(auth),encoding='utf-8')
   hashes={owners/'reader':C['ReaderBinarySHA256'],owners/'operator':C['SenderBinarySHA256']}
   for g in C['Groups']:
    for stage,expected in g['ConfigSHA256'].items():hashes[owners/g['Group']/stage/'operator.json']=expected
    for stage,expected in g['CaseConfigSHA256'].items():hashes[owners/g['Group']/('case-'+stage)/'remaining.json']=expected
    hashes[temp/('sealed-'+g['Group'])]=g['SealedGatewaySHA256']
   actual_sha=w.sha;actual_load=w.load;inspections=[]
   def sha(path):return hashes[path] if path in hashes else actual_sha(path)
   def load(path):
    if path.name=='permission.json':return {'Enabled':permission}
    return actual_load(path)
   def inspect(ident):
    inspections.append(ident)
    result={'Id':ident,'State':{'Running':False,'Pid':0},'HostConfig':{'RestartPolicy':{'Name':'no'}}}
    group=next((g for g in C['Groups'] if g['Gateway']==ident),None)
    if group:result['Mounts']=[{'Destination':'/etc/caddy/Caddyfile','RW':False,'Source':str(temp/('sealed-'+group['Group']))}]
    return result
   def root_owner(host,group,action):return inspect(host.binding(group)['root_owners'][action]['container']),temp/'unused-root-output'
   with patch.object(w,'PRIVATE',owners),patch.object(w,'CONTROL',control),patch.object(w,'AUTHORIZATION_PUBLIC',authpath),patch.object(w,'sha',sha),patch.object(w,'load',load),patch.object(w.Host,'inspect',lambda host,ident:inspect(ident)),patch.object(w.Host,'root_owner',root_owner),patch.object(w.Host,'docker',side_effect=AssertionError('No Docker allowed')):
    try:return w.Host().check(),inspections
    except Exception as error:return error,inspections
 def test_exact_by_reference_original_with_final_LF_is_accepted(self):
  result,seen=self.run_check();self.assertIsInstance(result,dict);self.assertEqual(result['status'],'PASS_PREWINDOW_CLOSED');self.assertEqual(result['permissions_disabled'],32);self.assertEqual(result['containers_stopped'],58);self.assertTrue(seen)
 def test_foreign_scope_window_budget_and_resend_fail_before_inspection(self):
  cases={'execution':'previous-execution','not_before':'2026-10-18T09:00:00Z','not_after':'2026-10-18T11:00:00Z','max_per_target':2,'max_total':8,'unknown_resend':True,'old_authorization_reused':True,'user_retyped_template':True,'source':'previous-source','sender_wire_recovery_binary_sha256':'0'*64,'target_digest':'0'*64}
  for field,value in cases.items():
   with self.subTest(field=field):
    result,seen=self.run_check(mutate_auth=lambda auth,k=field,v=value:auth.update({k:v}));self.assertIsInstance(result,ValueError);self.assertEqual(seen,[])
 def test_raw_user_final_LF_envelope_and_machine_payload_tamper_fail_before_inspection(self):
  cases=[('user-message-verbatim.txt',ALLOWED['user-message-verbatim.txt'].rstrip(b'\n')),('authorization-envelope.txt',ALLOWED['authorization-envelope.txt']+b'changed'),('approved-referenced-template.txt',ALLOWED['approved-referenced-template.txt']+b'changed')]
  for file,raw in cases:
   with self.subTest(file=file):
    result,seen=self.run_check(mutate_file=(file,raw));self.assertIsInstance(result,ValueError);self.assertEqual(seen,[])
 def test_premature_enabled_permission_is_rejected(self):
  result,seen=self.run_check(permission=True);self.assertIsInstance(result,ValueError);self.assertIn('permission enabled before window',str(result));self.assertTrue(seen)
if __name__=='__main__':unittest.main(verbosity=2)