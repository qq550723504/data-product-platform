import importlib.util
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch
spec=importlib.util.spec_from_file_location('independent_logs',Path(r'C:\Users\Henry\.codex\worktrees\9400\data-product-platform\examples\lifecycle-d1\reconcile-p1-post-v22.py'))
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
class Logs(unittest.TestCase):
 def test_failed_container_original_stderr_is_retained(self):
  with tempfile.TemporaryDirectory() as d:
   root=Path(d);private=root/'private';public=root/'public';private.mkdir();public.mkdir()
   manifest={'owners':{'disable-all':{'cid':'fixed-disable-cid'}}}
   def docker(*args):
    if args[0]=='logs':return m.call(['docker',*args])
    if args[0]=='wait':return b'1\n'
    return b''
   diagnostic=b'ORIGINAL_IDENTITY_READBACK_FAILURE\n'
   fake=SimpleNamespace(returncode=0,stdout=b'',stderr=diagnostic)
   with patch.object(m,'CONTROL',private),patch.object(m,'PUBLIC',public),patch.object(m,'docker',side_effect=docker),patch.object(m,'inspect',return_value={'State':{'StartedAt':'s','FinishedAt':'f'}}),patch.object(m,'closed',return_value=True),patch.object(m.subprocess,'run',return_value=fake),self.assertRaises(ValueError):
    m.execute_owner(manifest,'disable-all')
   retained=b''.join(p.read_bytes() for p in private.rglob('*') if p.is_file())
   self.assertIn(diagnostic,retained,'Docker logs exited 0 but its container stderr was discarded')
if __name__=='__main__':unittest.main(verbosity=2)
