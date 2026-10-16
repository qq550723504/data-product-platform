import importlib.util,sys,unittest,json,copy
from pathlib import Path
from unittest.mock import patch
spec=importlib.util.spec_from_file_location('native_completion',Path(__file__).with_name('complete-native-d1.py'))
module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
class CompletionSafety(unittest.TestCase):
 def test_destructive_expanded_or_confirmation_cli_rejected_before_io(self):
  for args in [('delete',),('enable',),('cleanup',),('archive-run','--target','foreign'),('pre-issued','--human-confirmed'),('prepare-plane','--private-root','foreign')]:
   with self.subTest(args=args),patch.object(sys,'argv',['completion',*args]),patch.object(module.subprocess,'run') as run:
    with self.assertRaises(SystemExit):module.main()
    run.assert_not_called()
 def test_checks_survive_python_optimization(self):
  with self.assertRaises(RuntimeError):module.require(False,'unsupported scope')
 def test_changed_service_identity_image_owner_state_or_port_rejected(self):
  run='11111111-1111-1111-1111-111111111111';project='dpp-lifecycle307-11111111'
  resource={'run_id':run,'project':project,'containers':{'postgres':project+'-postgres'}}
  item={'Id':'exact','Image':'pinned','Config':{'Labels':{'lifecycle307.run':run}},'State':{'Running':True},'NetworkSettings':{'Ports':{'5432/tcp':None}}}
  changes=[lambda x:x.update(Id='other'),lambda x:x.update(Image='other'),lambda x:x['Config']['Labels'].update({'lifecycle307.run':'other'}),lambda x:x['State'].update(Running=False),lambda x:x['NetworkSettings']['Ports'].update({'5432/tcp':[{'HostPort':'5432'}]})]
  for mutate in changes:
   changed=copy.deepcopy(item);mutate(changed)
   with patch.object(module,'load',side_effect=[{'private':project},resource,{'RunID':run},{'services':[{'name':project+'-postgres','id':'exact','image':'pinned'}]}]),patch.object(module,'docker',return_value=json.dumps([changed]).encode()):
    with self.assertRaises(RuntimeError):module.fixture('plane')
if __name__=='__main__':unittest.main()
