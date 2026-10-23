from pathlib import Path
import sys,tempfile,unittest
from unittest.mock import patch
ROOT=Path(__file__).resolve().parents[5]
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
from test_preallocation_ci import p,runner,PreallocationTests,HEAD
class IndependentUnknownNetworkTests(unittest.TestCase):
    def test_each_network_read_failure_stops_full_allocate_before_history_build_and_control(self):
        for slot in range(3):
            with self.subTest(slot=slot),tempfile.TemporaryDirectory() as tmp:
                root=Path(tmp);call_count=0
                good=PreallocationTests().network_reply()
                def docker(*args):
                    nonlocal call_count
                    if args==('network','ls','-q'):
                        call_count+=1
                        if call_count==slot+1:raise OSError('independent network read unavailable')
                    return good(*args)
                with patch.object(p,'ROOT',root),patch.object(p,'CONTROL',root/'private'),patch.object(p,'PUBLIC',root/'public'),patch.object(p.window,'policy_checkpoint'),patch.object(p,'preallocation_ci',return_value={'head':HEAD}),patch.object(p.prior,'module',return_value=runner),patch.object(runner,'docker',side_effect=docker),patch.object(p,'historical_gate',side_effect=AssertionError('history reached')) as history,patch.object(p,'build',side_effect=AssertionError('build reached')) as build,patch.object(p.Allocation,'__init__',side_effect=AssertionError('allocation reached')) as allocation:
                    with self.assertRaisesRegex(OSError,'independent network read unavailable'):p.allocate()
                    self.assertEqual(call_count,slot+1)
                    history.assert_not_called();build.assert_not_called();allocation.assert_not_called()
                    self.assertFalse((root/'private').exists());self.assertFalse((root/'public').exists())
if __name__=='__main__':
    modules=['test_preallocation_ci','test_receipt_bytes','test_retained_v31','test_fixture_artifacts','test_retained_v26','test_freeze_six_candidate','test_six_candidate_runtime']
    suite=unittest.defaultTestLoader.loadTestsFromNames(modules)
    suite.addTests(unittest.defaultTestLoader.loadTestsFromTestCase(IndependentUnknownNetworkTests))
    result=unittest.TextTestRunner(verbosity=2).run(suite)
    raise SystemExit(0 if result.wasSuccessful() else 1)
