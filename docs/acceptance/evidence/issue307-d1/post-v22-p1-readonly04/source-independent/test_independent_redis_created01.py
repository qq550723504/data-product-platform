import copy,importlib.util,json,sys,unittest
from pathlib import Path
source=Path(sys.argv.pop(1));root=Path(r'C:\Users\Henry\.codex\worktrees\9400\data-product-platform')
spec=importlib.util.spec_from_file_location('redis_created',source);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
class CreatedRedis(unittest.TestCase):
 def test_intended_nonroot_fix_accepts_real_created_network_snapshot(self):
  p=root/'docs/acceptance/evidence/issue307-d1/post-v22-p1-readonly03'
  manifest=json.loads((p/'preparation.json').read_bytes());receipt=json.loads((p/'actual-disabled-bindings03.json').read_bytes())
  x=copy.deepcopy(next(v for v in receipt['owners'] if v['id']==manifest['redis']))
  self.assertEqual(x['state']['Status'],'created')
  state={'Image':x['image'],'Config':{'User':'999:1000','Cmd':x['cmd']},'State':x['state'],'HostConfig':x['host'],'NetworkSettings':x['network'],'Mounts':x['mounts']}
  self.assertEqual(state['HostConfig']['NetworkMode'],manifest['source']['Network'])
  self.assertEqual(next(iter(state['NetworkSettings']['Networks'].values()))['NetworkID'],'')
  try:m.redis_contract(state,manifest,False)
  except ValueError as e:self.fail('Real stopped-created Redis with explicit intended UID was rejected before any start: '+str(e))
if __name__=='__main__':unittest.main(verbosity=2)
