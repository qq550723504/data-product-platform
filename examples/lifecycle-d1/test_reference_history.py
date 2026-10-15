import importlib.util
import io
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch
import zipfile

SPEC = importlib.util.spec_from_file_location('history', Path(__file__).with_name('check-reference-history.py'))
m = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(m)


class HistoricalRuntimeIsolation(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.history = m.archive(m.ROOT, m.HISTORY, m.BASE.rstrip('/'))
        cls.source = m.archive(m.ROOT, m.SOURCE, 'apps/platform')

    def archives(self, root, revision, path):
        return self.history if revision == m.HISTORY else self.source

    def test_snapshot_uses_exact_git_runtime_and_leaves_current_fix_in_place(self):
        path = 'apps/platform/cmd/lifecycle-fixture-delete/reference_gateway.go'
        before = m.current(m.ROOT, path)
        with tempfile.TemporaryDirectory() as temporary, patch.object(m, 'archive', self.archives):
            root = Path(temporary) / 'snapshot'
            result = m.build_snapshot(m.ROOT, root)
            with zipfile.ZipFile(io.BytesIO(self.source)) as archive:
                self.assertEqual((root / path).read_bytes(), archive.read(path))
            self.assertEqual(result['historical_raw_checked'], 255)
            self.assertEqual(result['runtime_source_files'], 317)
            self.assertFalse(result['snapshot_is_send_authorization'])
            self.assertFalse(result['current_source_replaced'])
        self.assertEqual(m.current(m.ROOT, path), before)

    def test_edited_current_receipt_cannot_redefine_frozen_history(self):
        original = m.current
        def changed(root, path):
            raw = original(root, path)
            return raw + b' ' if path == m.BINDING else raw
        with tempfile.TemporaryDirectory() as temporary, patch.object(m, 'archive', self.archives), patch.object(m, 'current', changed):
            with self.assertRaisesRegex(ValueError, 'Historical checkout artifact changed'):
                m.build_snapshot(m.ROOT, Path(temporary) / 'snapshot')

    def test_tampered_frozen_runtime_is_rejected(self):
        output = io.BytesIO()
        path = 'apps/platform/cmd/lifecycle-fixture-delete/reference_send.go'
        with zipfile.ZipFile(io.BytesIO(self.source)) as source, zipfile.ZipFile(output, 'w') as changed:
            for entry in source.infolist():
                raw = source.read(entry.filename)
                changed.writestr(entry, raw + b'// changed\n' if entry.filename == path else raw)
        def archives(root, revision, name):
            return self.history if revision == m.HISTORY else output.getvalue()
        with tempfile.TemporaryDirectory() as temporary, patch.object(m, 'archive', archives):
            with self.assertRaisesRegex(ValueError, 'Frozen Git runtime differs from binding'):
                m.build_snapshot(m.ROOT, Path(temporary) / 'snapshot')

    def test_cli_cannot_select_revision_target_or_confirmation_before_io(self):
        for args in (['delete'], ['--confirm'], ['--source', 'foreign'], ['--revision', 'HEAD'], ['--root', 'foreign']):
            with self.subTest(args=args), patch.object(sys, 'argv', ['history', *args]), patch.object(m, 'archive') as archive:
                with self.assertRaises(SystemExit):
                    m.main()
                archive.assert_not_called()
        with patch.object(m.subprocess, 'run') as run:
            with self.assertRaises(ValueError):
                m.archive(m.ROOT, 'HEAD', 'apps/platform')
            run.assert_not_called()

    def test_only_fixed_offline_commands_and_failure_is_not_retried(self):
        with patch.object(m.subprocess, 'run') as run:
            m.run_checks(Path('isolated-snapshot'))
        self.assertEqual(run.call_count, 27)
        for call in run.call_args_list:
            argv = call.args[0]
            self.assertEqual(argv[0], sys.executable)
            self.assertTrue(call.kwargs['check'])
            self.assertFalse(any('complete-' in arg or 'refresh-reference-' in arg for arg in argv))
        with patch.object(m.subprocess, 'run', side_effect=subprocess.CalledProcessError(1, ['checker'])) as run:
            with self.assertRaises(subprocess.CalledProcessError):
                m.run_checks(Path('isolated-snapshot'))
            self.assertEqual(run.call_count, 1)


if __name__ == '__main__':
    unittest.main()
