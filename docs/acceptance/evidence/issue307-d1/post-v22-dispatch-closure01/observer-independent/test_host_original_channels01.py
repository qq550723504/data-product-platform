"""Independent original-channel counterexample, all Docker calls mocked."""
from pathlib import Path
import types,unittest,tempfile,subprocess
from unittest.mock import patch
R=Path(r'C:/Users/Henry/.codex/worktrees/d1-six/data-product-platform');E=R/'docs/acceptance/evidence/issue307-d1/post-v22-dispatch-closure01/observer-independent'
m=types.ModuleType('draft_closure');m.__file__=str(R/'examples/lifecycle-d1/close-old-business-dispatches.py');exec(compile((E/'host-draft-snapshot01.py').read_bytes(),m.__file__,'exec'),m.__dict__)
class OriginalChannels(unittest.TestCase):
 def test_wait_timeout_and_inspect_failure_keep_original_channels(self):
  for failure in ('timeout','inspect'):
   with self.subTest(failure=failure),tempfile.TemporaryDirectory(prefix='mock-host-channels-',dir=E) as td:
    root=Path(td);self.assertTrue(root.resolve().is_relative_to(E.resolve()));calls=[]
    def run(args,**kw):
     calls.append(args)
     if args[1]=='wait':
      if failure=='timeout':raise subprocess.TimeoutExpired(args,120,output=b'partial wait stdout',stderr=b'partial wait stderr')
      return subprocess.CompletedProcess(args,0,b'0\n',b'')
     return subprocess.CompletedProcess(args,0,b'original owner stdout',b'original owner stderr')
    def inspect(cid):raise RuntimeError('fresh inspect unavailable')
    manifest={'owners':{'observer':{'cid':'fixed-observer'}}}
    with patch.object(m,'CONTROL',root),patch.object(m,'PUBLIC',root),patch.object(m,'docker',return_value=b''),patch.object(m,'own_contract',return_value={}),patch.object(m,'closed',return_value=True),patch.object(m,'inspect',side_effect=inspect),patch.object(m.subprocess,'run',side_effect=run):
     with self.assertRaises(Exception):m.run_owner(manifest,'observer')
    self.assertTrue(any(args[1]=='logs' for args in calls),'logs skipped after wait/inspect failure')
    self.assertEqual((root/'observer-stdout.private').read_bytes(),b'original owner stdout')
    self.assertEqual((root/'observer-stderr.private').read_bytes(),b'original owner stderr')
if __name__=='__main__':unittest.main(verbosity=2)
