"""Full allocation refuses non-green exact-source CI before any resource I/O."""
import copy
import tempfile
from pathlib import Path
from unittest.mock import patch
import unittest
import importlib.util
from test_execution_freeze import snapshot, HEAD

loader=importlib.util.spec_from_file_location('preallocation_under_test',Path(__file__).with_name('prepare-six-candidate.py'))
p=importlib.util.module_from_spec(loader);loader.loader.exec_module(p)

class PreallocationTests(unittest.TestCase):
    def test_fresh_exact_source_success_records_provenance_without_lock_or_resources(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);(root/'docs/acceptance/evidence/issue307-d1').mkdir(parents=True)
            with patch.object(p,'ROOT',root),patch.object(p.subprocess,'check_output',return_value=(HEAD+'\n').encode()),\
                 patch.object(p.execution_freeze.GitHub,'snapshot',return_value=snapshot()),\
                 patch.object(p.Allocation,'__init__',side_effect=AssertionError('allocation before CI')):
                self.assertEqual(p.preallocation_ci()['head'],HEAD)
                records=list(root.rglob('passed.json'));self.assertEqual(len(records),1)
                self.assertEqual(p.load(records[0])['allocated_resources'],0)

    def test_complete_allocate_rejects_failed_pending_foreign_and_unknown_ci_before_history_or_build(self):
        for fault in ['failed','pending','head','provenance','unknown']:
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as tmp:
                root=Path(tmp);(root/'docs/acceptance/evidence/issue307-d1').mkdir(parents=True)
                state=copy.deepcopy(snapshot())
                if fault=='failed':state['ci']['conclusion']='failure'
                if fault=='pending':state['ci']['status']='in_progress'
                if fault=='head':state['head']='b'*40
                if fault=='provenance':state['checks'][0]['app_id']=9876
                with patch.object(p,'ROOT',root),patch.object(p,'CONTROL',root/'private'),patch.object(p,'PUBLIC',root/'public'),\
                     patch.object(p.window,'policy_checkpoint'),patch.object(p.subprocess,'check_output',return_value=(HEAD+'\n').encode()),\
                     patch.object(p.execution_freeze.GitHub,'snapshot',side_effect=OSError('unavailable') if fault=='unknown' else None,return_value=state),\
                     patch.object(p,'historical_gate',side_effect=AssertionError('Docker/history before CI')),\
                     patch.object(p,'build',side_effect=AssertionError('build before CI')),\
                     patch.object(p.Allocation,'__init__',side_effect=AssertionError('allocate before CI')):
                    with self.assertRaises((ValueError,OSError)):p.allocate()
                    self.assertFalse((root/'private').exists());self.assertFalse((root/'public').exists())

    def test_ci_A_cannot_allocate_build_B_or_drifted_A_after_long_history_and_build(self):
        for fault in ['build-head','host-bytes']:
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as tmp:
                root=Path(tmp)
                artifact={'source':'f'*40,'host':{'source':'b'*40 if fault=='build-head' else HEAD}}
                with patch.object(p,'ROOT',root),patch.object(p,'CONTROL',root/'private'),patch.object(p,'PUBLIC',root/'public'),\
                     patch.object(p.window,'policy_checkpoint'),patch.object(p,'preallocation_ci',return_value={'head':HEAD}),\
                     patch.object(p,'historical_gate',return_value={'recorded':True}),patch.object(p,'build',return_value=artifact),\
                     patch.object(p.fixture_artifacts,'verify_host',side_effect=ValueError('host bytes drift')),\
                     patch.object(p.Allocation,'network',side_effect=AssertionError('network before source binding')),\
                     patch.object(p.Allocation,'volume',side_effect=AssertionError('volume before source binding')),\
                     patch.object(p.Allocation,'create',side_effect=AssertionError('owner before source binding')):
                    with self.assertRaises(ValueError):p.allocate()
                    receipt=p.load(root/'public/allocation-closed.json')
                    self.assertEqual(receipt['owners'],[]);self.assertEqual(receipt['unconfirmed_names'],[])

if __name__=='__main__':unittest.main()
