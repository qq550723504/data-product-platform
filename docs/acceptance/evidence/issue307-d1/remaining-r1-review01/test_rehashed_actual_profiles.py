from pathlib import Path
import hashlib,importlib.util,json,sys,tempfile,unittest
from unittest.mock import patch
ROOT=Path(r'C:\Users\Henry\.codex\worktrees\9400\data-product-platform')
spec=importlib.util.spec_from_file_location('r1_repository_tests',ROOT/'examples/lifecycle-d1/test_remaining_window.py');t=importlib.util.module_from_spec(spec);spec.loader.exec_module(t)
class RehashedActualProfileTests(unittest.TestCase):
 def test_rehashed_actual_semantic_mismatch_fails_before_permission_source_or_output(self):
  for kind in ['actual_use_until','actual_base_end','actual_batch','actual_group','actual_same_count_heads','actual_recovery_until']:
   with self.subTest(kind=kind),tempfile.TemporaryDirectory(dir=Path(__file__).parent) as temp:
    root=Path(temp);packet,owner,c,receipt=t.WindowContractTests().fixture(root)
    row=receipt['recoveries'][0] if kind=='actual_recovery_until' else receipt['cases'][0]
    path=owner/row['group']/(row['stage'] if kind=='actual_recovery_until' else 'case-'+row['stage'])/('recovery.json' if kind=='actual_recovery_until' else 'remaining.json')
    profile=json.loads(path.read_bytes())
    if kind=='actual_use_until':profile['CandidateUseUntil']='2026-10-17T02:40:00Z'
    elif kind=='actual_base_end':profile['Base']['ExpiresAt']='2026-10-18T14:40:01Z'
    elif kind=='actual_batch':profile['Batch']='67de3acd-3a9d-4ece-bebb-cd53d0ab3ac5'
    elif kind=='actual_group':profile['Group']='mixed'
    elif kind=='actual_same_count_heads':profile['ExpectedPolicyHeads']['0']='foreign-but-same-count'
    elif kind=='actual_recovery_until':profile['ValidUntil']='2026-10-18T12:40:00Z'
    t.save(path,profile);row['config_sha256']=t.sha(path)
    if kind!='actual_recovery_until':c['Groups'][0]['CaseConfigSHA256'][row['stage']]=t.sha(path)
    # Keep the public semantic projection valid while updating every raw anchor.
    # Public-only checks cannot attest the actual private contents by themselves.
    t.save(packet/'profile-windows.json',receipt);c['ProfileWindowsSHA256']=t.sha(packet/'profile-windows.json');t.save(packet/'candidate.json',c)
    self.assertEqual(t.w.validate_packet(packet)['status'],'PASS_CURRENT_CROSS_FIELD_WINDOWS')
    argv=['verify-remaining-native.py','--packet',str(packet),'--candidate-sha256',t.sha(packet/'candidate.json'),'--bindings-sha256',t.sha(packet/'bindings.json'),'--private',str(root/'collection'),'--output',str(root/'output'),'--reader',str(owner/'reader'),'--checker',str(owner/'checker'),'--checker-sha256',t.sha(owner/'checker')]
    original_read=Path.read_bytes
    def read_bytes(p):
     if p.name=='permission.json':raise AssertionError('permission I/O before semantic denial')
     return original_read(p)
    with patch.object(sys,'argv',argv),patch.object(t.v,'sealed_gate',side_effect=AssertionError('sealed/permission gate reached')) as sealed,patch.object(t.v,'collect_group',side_effect=AssertionError('source collect reached')) as collect,patch.object(t.v.runner,'docker',side_effect=AssertionError('Docker reached')) as docker,patch.object(t.v.runner,'inspect',side_effect=AssertionError('source inspection reached')) as inspect,patch.object(Path,'read_bytes',read_bytes):
     with self.assertRaises(ValueError):t.v.main()
    sealed.assert_not_called();collect.assert_not_called();docker.assert_not_called();inspect.assert_not_called();self.assertFalse((root/'collection').exists());self.assertFalse((root/'output').exists())
if __name__=='__main__':unittest.main(verbosity=2)