import copy, importlib.util, json, subprocess, sys, unittest
from pathlib import Path
SPEC=importlib.util.spec_from_file_location('lc13',Path(__file__).with_name('complete-lc13-readonly.py'))
m=importlib.util.module_from_spec(SPEC);SPEC.loader.exec_module(m)
class ReadonlyContract(unittest.TestCase):
 def config(self):
  return {'Fixture':{'RunID':'5d2274ee-a671-4bdc-b406-22b8dad8fd61','WorkspaceID':'89eba3fd-9763-4f66-8cec-1a779104f155','ProviderInstanceID':'1980751b-c50e-4895-93a8-cb55960cd635','IncarnationID':'be784eda-96e4-49a9-ae47-eaa3203a2dc5','RootAccess':'','RootSecret':'','Endpoint':'127.0.0.1:9000','PostgresDSN':'postgres://fixture:synthetic@dpp-lifecycle307-5d2274ee-postgres:5432/lifecycle307?sslmode=disable'}}
 def test_inspector_entrypoint_is_existing_env_and_lf(self):
  self.assertNotIn(b'\r',m.INSPECT_SCRIPT);self.assertIn(b'LIFECYCLE_REFERENCE_MODE=inspect-source\n',m.INSPECT_SCRIPT);self.assertIn(b'LIFECYCLE_REFERENCE_PROFILE=5d2274ee-614cd9ad\n',m.INSPECT_SCRIPT);self.assertNotIn(b' -mode',m.INSPECT_SCRIPT)
 def test_audit_config_preserves_source_and_rejects_wrong_target(self):
  c=self.config();before=copy.deepcopy(c);project='dpp-lifecycle307-5d2274ee'
  r=m.shadow_config(c,project+'-readonly-audit-core-03',project);self.assertEqual(c,before);self.assertIn('audit-core-03:5432',r['Fixture']['PostgresDSN'])
  for bad in (project+'-postgres',project+'-predelete-postgres-02','foreign'):
   with self.assertRaises(RuntimeError):m.shadow_config(c,bad,project)
 def test_source_dsn_identity_and_root_fail_closed(self):
  base=self.config();project='dpp-lifecycle307-5d2274ee';audit=project+'-readonly-audit-core-03'
  for key,value in [('RootAccess','root'),('WorkspaceID','foreign'),('IncarnationID','foreign'),('PostgresDSN','postgres://fixture:test@foreign:5432/lifecycle307?sslmode=disable'),('PostgresDSN','postgres://fixture:test@'+project+'-postgres:5432/other?sslmode=disable')]:
   c=copy.deepcopy(base);c['Fixture'][key]=value
   with self.assertRaises(RuntimeError):m.shadow_config(c,audit,project)
 def proof(self):
  return {'provider_delete':0,'versions':[{'copy':{'ID':str(i),'ProviderVersion':str(i)},'bytes':232,'sha256':'hash','protection':{'RetentionMode':'NONE','RetainUntil':None,'LegalHold':False}} for i in range(6)]}
 def test_six_versions_reject_hash_version_and_protection_drift(self):
  before=self.proof();m.six_equal(before,copy.deepcopy(before))
  for key,value in [('LegalHold',True),('RetentionMode','GOVERNANCE'),('RetainUntil','future')]:
   after=copy.deepcopy(before);after['versions'][0]['protection'][key]=value
   with self.assertRaises(RuntimeError):m.six_equal(after,before)
  for target,key,value in [('copy','ProviderVersion','new')]:
   after=copy.deepcopy(before);after['versions'][0][target][key]=value
   with self.assertRaises(RuntimeError):m.six_equal(after,before)
  after=copy.deepcopy(before);after['versions'][0]['sha256']='changed'
  with self.assertRaises(RuntimeError):m.six_equal(after,before)
 def test_cli_does_not_admit_delete_or_arbitrary_target(self):
  for args in (['delete'],['5d-readonly','--confirm'],['5d-readonly','--source','foreign']):
   p=subprocess.run([sys.executable,str(SPEC.origin),*args],capture_output=True);self.assertEqual(p.returncode,2)
if __name__=='__main__':unittest.main()
