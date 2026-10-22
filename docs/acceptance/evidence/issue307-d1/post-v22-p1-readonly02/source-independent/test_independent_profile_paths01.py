import importlib.util,json,sys,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
source=Path(sys.argv.pop(1))
spec=importlib.util.spec_from_file_location('independent_paths',source);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
class ProfilePaths(unittest.TestCase):
 def test_only_enabled_change_works_with_actual_windows_manifest_key(self):
  with tempfile.TemporaryDirectory() as d:
   control=Path(d);host='dpp-lifecycle307-808a6960-postgres'
   def inspect(cid):
    return {'Name':'/'+host if cid=='Core' else '/Native','Image':m.PG if cid=='Core' else m.MI,'State':{'Running':True},'NetworkSettings':{'Networks':{'network':{'NetworkID':'network','Aliases':None,'DNSNames':[host] if cid=='Core' else ['Native']}}}}
   def sha(p):
    p=Path(p)
    if p.name=='recovery-disabled-original.json':return 'recovery-disabled'
    if p.name=='p1-readonly-permission-disabled-original.json':return 'root-disabled'
    return 'enabled-bytes'
   def load(p):return {'Enabled':False} if Path(p).name.endswith('-disabled-original.json') else {'Enabled':True}
   for separator in ('/',chr(92)):
    with self.subTest(separator=separator):
     manifest={'ExpiresAt':(m.now()+m.dt.timedelta(minutes=20)).isoformat(),'source_files':{},'inputs':{'worker-private'+separator+'recovery.json':'recovery-disabled','root-private'+separator+'p1-readonly-permission.json':'root-disabled'},'old_closed':{'owners':[],'old_control_sha256':{}},'owners':{},'source':{'Core':'Core','Native':'Native','Network':'network'},'redis':'Redis','core_connection_binding':{'host':host}}
     with patch.object(m,'CONTROL',control),patch.object(m,'inspect',side_effect=inspect),patch.object(m,'sha',side_effect=sha),patch.object(m,'load',side_effect=load),patch.object(m,'docker',return_value=json.dumps([{'Internal':True,'Containers':{'Core':{},'Native':{}}}]).encode()):
      try:m.active_topology(manifest,'enable-p1-readonly')
      except ValueError as e:self.fail('Only Enabled changed, but actual manifest path separator rejected the controlled transition: '+str(e))
if __name__=='__main__':unittest.main(verbosity=2)
