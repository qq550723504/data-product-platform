"""Pure host-seal regression: no Docker, SQL or provider operations."""
import importlib.util,json,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
spec=importlib.util.spec_from_file_location('continuation',Path(__file__).with_name('complete-lc13-continuation.py'))
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
class HostSeal(unittest.TestCase):
 def test_processing_seal_closes_unused_root_and_payload_aliases(self):
  with tempfile.TemporaryDirectory() as d:
   root=Path(d);private=root/'private';control=root/'control';public=root/'public'
   for p in [private,control,public]:p.mkdir()
   roles=['plane-owner','plane-admin','resume-owner','resume-admin','credential-owner','credential-admin']
   for role in roles:(private/role).mkdir();m.save(private/role/'reader.json',{'Enabled':True,'RunID':'closed-synthetic'})
   calls=[]
   with patch.object(m,'fresh',return_value=(private,{'run_id':'closed-synthetic'})),patch.object(m,'CONTROL',control),patch.object(m,'PUBLIC',public),patch.object(m,'step',side_effect=lambda *a:calls.append(a)):
    m.processing_seal()
   self.assertEqual(calls,[('seal','resume-admin')])
   self.assertTrue(all(m.load(private/role/'reader.json')['Enabled'] is False for role in roles))
   self.assertTrue(all(m.load(control/(role+'-before-final-seal.private.json'))['Enabled'] for role in roles))
if __name__=='__main__':unittest.main()
