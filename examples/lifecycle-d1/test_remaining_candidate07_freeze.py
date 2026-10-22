import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('candidate07_freeze', Path(__file__).with_name('check-remaining-candidate07-freeze.py'))
a = importlib.util.module_from_spec(spec)
spec.loader.exec_module(a)


class FrozenCandidate07Guards(unittest.TestCase):
    def test_only_exact_reviewed_gofmt_test_may_differ(self):
        original = subprocess.check_output(['git', 'show', a.SOURCE + ':' + a.FORMATTED_TEST], cwd=a.ROOT)
        formatted = subprocess.check_output(['gofmt'], input=original)
        self.assertEqual(a.sha(formatted), a.FORMATTED_TEST_SHA)
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            test = root / a.FORMATTED_TEST
            test.parent.mkdir(parents=True)
            test.write_bytes(formatted)
            runtime_name = 'apps/platform/cmd/lifecycle-fixture-remaining/main.go'
            runtime = root / runtime_name
            runtime.parent.mkdir(parents=True)
            runtime.write_bytes(b'frozen runtime\n')
            files = {a.FORMATTED_TEST: {'lf_sha256': a.sha(original.replace(b'\r\n', b'\n'))},
                     runtime_name: {'lf_sha256': a.sha(runtime.read_bytes())}}
            a.verify_current_source(root, files)
            runtime.write_bytes(b'changed runtime\n')
            with self.assertRaisesRegex(ValueError, 'current source differs'):
                a.verify_current_source(root, files)
            runtime.write_bytes(b'frozen runtime\n')
            test.write_bytes(formatted.replace(b'e == nil', b'e != nil'))
            self.assertNotEqual(a.sha(test.read_bytes()), a.FORMATTED_TEST_SHA)
            with self.assertRaisesRegex(ValueError, 'current source differs'):
                a.verify_current_source(root, files)

    def test_raw_working_bytes_and_git_bytes_both_required(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            receipt = b'exact raw\r\n'
            (root / 'receipt').write_bytes(receipt)
            records = {'receipt': a.sha(receipt)}
            a.verify_raw(root, records, {'receipt': receipt})
            with self.assertRaisesRegex(ValueError, 'Git raw bytes changed'):
                a.verify_raw(root, records, {'receipt': receipt.replace(b'\r\n', b'\n')})
            (root / 'receipt').write_bytes(b'changed raw\r\n')
            with self.assertRaisesRegex(ValueError, 'raw evidence changed'):
                a.verify_raw(root, records, {'receipt': receipt})

    def test_rewritten_index_or_candidate_anchor_rejected(self):
        for name in ['evidence-index.json', 'candidate.json']:
            with self.subTest(name=name):
                a.anchored_bytes(b'fixed', b'fixed', name)
                with self.assertRaisesRegex(ValueError, 'frozen anchor changed'):
                    a.anchored_bytes(b'rewritten', b'fixed', name)


if __name__ == '__main__':
    unittest.main()
