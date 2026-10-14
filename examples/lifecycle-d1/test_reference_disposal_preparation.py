import importlib.util, sys, unittest
from pathlib import Path
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('reference_disposal',Path(__file__).with_name('prepare-reference-disposal.py'))
module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
class PreparationSafety(unittest.TestCase):
    def test_destructive_and_expanded_arguments_fail_before_any_io(self):
        for args in [('execute',),('provision',),('enable',),('cleanup',),('native-before','--human-confirmed'),('native-before','--target','foreign'),('no-send','--release','foreign')]:
            with self.subTest(args=args),patch.object(sys,'argv',['prepare',*args]),patch.object(module.subprocess,'run') as run:
                with self.assertRaises(SystemExit):module.main()
                run.assert_not_called()
    def test_authorization_check_survives_optimized_python(self):
        with self.assertRaises(RuntimeError):module.require(False,'unsupported authority')
    def test_frozen_source_topology_input_cannot_be_redefined_by_working_files(self):
        with patch.object(module,'call',side_effect=RuntimeError('pinned input unavailable')) as call,patch.object(module,'docker') as docker:
            with self.assertRaises(RuntimeError):module.preflight()
            self.assertEqual(call.call_args.args[0][0:3],['git','show','917465d88a0f6e8dfd0a4745fb4d8d767a2c75a1:docs/acceptance/evidence/issue307-d1/delete-proposal-5d2274ee/topology-readback.json'])
            docker.assert_not_called()
if __name__=='__main__':unittest.main()
