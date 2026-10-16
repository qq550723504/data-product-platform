import importlib.util
import io
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch
import zipfile

spec=importlib.util.spec_from_file_location('candidate05_history',Path(__file__).with_name('check-reference-candidate05-history.py'))
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)

class Candidate05History(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        pairs=((m.ANCHOR,m.DATA.rstrip('/')),(m.SOURCE,'apps/platform'),(m.HISTORY,m.BASE.rstrip('/')))
        cls.archives={pair:m.archive(*pair) for pair in pairs}

    def test_full_original_checker_with_frozen_runtime_leaves_current_code(self):
        path='apps/platform/cmd/lifecycle-fixture-prepare/main.go';before=m.current(path)
        with patch.object(m,'archive',side_effect=lambda *pair:self.archives[pair]):m.check()
        self.assertEqual(m.current(path),before)

    def test_changed_old_payload_or_index_cannot_redefine_archive(self):
        original=m.current
        for path in [m.DATA+'source-binding.json',m.DATA+'evidence-index.json']:
            with self.subTest(path=path),tempfile.TemporaryDirectory() as directory,patch.object(m,'current',side_effect=lambda p:original(p)+(b' ' if p==path else b'')):
                with self.assertRaisesRegex(ValueError,'Current historical candidate05 evidence changed'):
                    m.build_snapshot(Path(directory)/'snapshot',self.archives)

    def test_corrupt_archived_runtime_is_rejected(self):
        altered=io.BytesIO();pair=(m.SOURCE,'apps/platform');path='apps/platform/cmd/lifecycle-fixture-prepare/main.go'
        with zipfile.ZipFile(io.BytesIO(self.archives[pair])) as source,zipfile.ZipFile(altered,'w') as target:
            for entry in source.infolist():target.writestr(entry,source.read(entry.filename)+(b'\n// changed' if entry.filename==path else b''))
        archives=dict(self.archives);archives[pair]=altered.getvalue()
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaisesRegex(ValueError,'Archived candidate05 runtime differs'):
                m.build_snapshot(Path(directory)/'snapshot',archives)

    def test_foreign_archives_and_cli_authority_fail_before_io(self):
        with patch.object(m.subprocess,'run') as run:
            with self.assertRaisesRegex(ValueError,'Unsupported'):m.archive('HEAD','apps/platform')
            run.assert_not_called()
        for args in [['delete'],['--confirm'],['--source','HEAD'],['--execution','old']]:
            with self.subTest(args=args),patch.object(sys,'argv',['history',*args]),patch.object(m,'check') as check:
                with self.assertRaises(SystemExit):m.main()
                check.assert_not_called()

if __name__=='__main__':unittest.main()
