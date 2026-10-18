"""Independent no-provider counterexamples to the complete no-root gate."""
import importlib.util
from pathlib import Path
import unittest

source = Path(r'C:\Users\Henry\.codex\worktrees\9400\data-product-platform\examples\lifecycle-d1\test_complete_remaining_preflight.py')
spec=importlib.util.spec_from_file_location('complete_recording_reuse',source)
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)

class PrivateExposure(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        m.CompleteCLI.setUpClass()
        cls.checker=m.CompleteCLI.checker
    @classmethod
    def tearDownClass(cls):m.CompleteCLI.tearDownClass()
    run_entry=m.CompleteCLI.run_entry
    def test_extra_case_private_root_profile_refused(self):
        def mutate(d):
            m.write(d.owner/'business-races/case-renew/root.json',
                    {'RootAccess':'FAKE_ONLY_NOT_A_CREDENTIAL','RootSecret':'FAKE_ONLY'})
        self.run_entry(mutate,denied=True)
    def test_extra_recovery_private_host_key_refused(self):
        def mutate(d):
            (d.owner/'business-races/recover-P1/topology-key.private').write_bytes(b'FAKE_ONLY_NOT_A_KEY')
        self.run_entry(mutate,denied=True)

if __name__=='__main__':unittest.main(verbosity=2)
