import importlib.util
from pathlib import Path
import unittest
from unittest.mock import patch
spec=importlib.util.spec_from_file_location('independent_p1',Path(r'C:\Users\Henry\.codex\worktrees\9400\data-product-platform\examples\lifecycle-d1\reconcile-p1-post-v22.py'))
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
class Containment(unittest.TestCase):
 def test_unavailable_inspection_does_not_skip_known_owner_stop(self):
  manifest={'owners':{k:{'cid':k} for k in ('worker','enable-p1-readonly','disable-all')},'source':{'Core':'Core','Native':'Native'},'redis':'Redis'}
  stopped=[]
  def inspect(cid):
   if cid=='worker':raise RuntimeError('unavailable worker inspection')
   return {'State':{'Running':cid not in stopped,'Pid':0 if cid in stopped else 1},'HostConfig':{'PortBindings':{},'RestartPolicy':{'Name':'no'}},'NetworkSettings':{'Ports':{}}}
  def docker(*args):
   self.assertEqual(args[0],'stop');stopped.append(args[-1]);return b''
  with patch.object(m,'revoke_profiles',return_value=[{'profile':'test','disabled':True}]),patch.object(m,'inspect',side_effect=inspect),patch.object(m,'docker',side_effect=docker),patch.object(m,'execute_owner',side_effect=RuntimeError('disabled readback unavailable')):
   result=m.seal(manifest)
  self.assertFalse(result['all_closed'])
  self.assertFalse(result['fresh_disabled_readback'])
  self.assertIn('worker',stopped,'Known bound worker was not sent a physical stop when inspect was unavailable')
if __name__=='__main__':unittest.main(verbosity=2)
