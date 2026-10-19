import importlib.util,json,sys,unittest
from pathlib import Path
from unittest.mock import patch
source=Path(sys.argv.pop(1))
spec=importlib.util.spec_from_file_location('independent_shadow',source);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
class CoreBinding(unittest.TestCase):
 def test_running_foreign_owner_cannot_shadow_bound_core_dns(self):
  host='dpp-lifecycle307-808a6960-postgres'
  def info(cid):
   return {'Image':m.PG if cid=='Core' else m.MI,'Name':'/'+host if cid=='Core' else '/'+cid,'State':{'Running':True},'NetworkSettings':{'Networks':{'network':{'NetworkID':'network','Aliases':None,'DNSNames':[host] if cid in ('Core','Redis') else [cid]}}}}
  manifest={'ExpiresAt':(m.now()+m.dt.timedelta(minutes=20)).isoformat(),'source_files':{},'inputs':{},'old_closed':{'owners':[],'old_control_sha256':{}},'owners':{},'source':{'Core':'Core','Native':'Native','Network':'network'},'redis':'Redis','core_connection_binding':{'host':host}}
  net=[{'Internal':True,'Containers':{'Core':{},'Native':{},'Redis':{}}}]
  with patch.object(m,'inspect',side_effect=info),patch.object(m,'docker',return_value=json.dumps(net).encode()),self.assertRaises(ValueError):
   m.active_topology(manifest,'worker')
if __name__=='__main__':unittest.main(verbosity=2)
