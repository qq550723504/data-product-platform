"""Cross-entrypoint cleanup failures use mocked calls, never native Docker."""
import json
import contextlib
import io
from pathlib import Path
import subprocess
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

import storage_smoke
import b2_smoke
from cleanup_receipts import CleanupReceipts
from trial_images_test import DockerModel


class CleanupFailureContract(unittest.TestCase):
    def test_storage_partial_setup_and_cleanup_timeout_keep_receipt(self):
        with tempfile.TemporaryDirectory(prefix="dpp320-storage-receipt-") as directory:
            root = Path(directory); output = root / "output"; cache = root / "cache"; cache.mkdir()
            binary = cache / "minio"; binary.write_bytes(b"controlled synthetic binary")
            docker = DockerModel(); creates = 0
            def command(args, **kw):
                nonlocal creates
                if args[:2] == ["go", "version"]:
                    return subprocess.CompletedProcess(args, 0, b"controlled go version", b"")
                if args[:3] == ["docker", "network", "create"]:
                    creates += 1
                    if creates == 1: return subprocess.CompletedProcess(args, 0, b"", b"")
                    raise subprocess.TimeoutExpired(args, 1)
                if args[:3] == ["docker", "network", "rm"]:
                    raise subprocess.TimeoutExpired(args, 1)
                return docker.command(args, **kw)
            def prepare(cache, tag, *, owner):
                return binary, owner.build(tag, ["controlled-context"], 1)
            with patch.object(storage_smoke, "command", command), patch.object(storage_smoke, "prepare_binary", prepare), \
                 patch.object(storage_smoke, "source_sha", return_value="controlled-source-hash"), \
                 patch.object(storage_smoke.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, b"controlled-sdk", b"")):
                with self.assertRaises(subprocess.TimeoutExpired): storage_smoke.run(output, cache)
            report = json.loads((output / "storage-report.json").read_text())
            self.assertEqual(report["status"], "FAIL")
            self.assertEqual(report["cleanup"], "FAIL")
            group = report["cleanup_detail"]["networks"]
            self.assertEqual(group["status"], "UNKNOWN")
            self.assertEqual(group["attempts"][0]["failure_type"], "TimeoutExpired")
            self.assertNotEqual(report["cleanup_detail"]["image_refs"]["status"], "NOT_RUN")
            self.assertTrue(all(call == ["docker", "image", "rm", "--no-prune", call[-1]] for call in docker.mutations()))

    def test_b2_cleanup_oserror_does_not_erase_business_failure(self):
        with tempfile.TemporaryDirectory(prefix="dpp320-b2-receipt-") as directory:
            output = Path(directory) / "output"
            trial = SimpleNamespace(images=SimpleNamespace(tag=lambda _: "controlled-tag"), report={})
            def cleanup(): raise OSError("controlled secret diagnostic must not be exported")
            trial.cleanup = cleanup
            with patch.object(b2_smoke, "Trial", return_value=trial), patch.object(b2_smoke, "port", return_value=1), \
                 patch.object(b2_smoke, "command", return_value=subprocess.CompletedProcess([],0,b"controlled-head",b"")), \
                 patch.object(b2_smoke, "prepare_binary", side_effect=RuntimeError("controlled setup failure")):
                with contextlib.redirect_stdout(io.StringIO()), self.assertRaisesRegex(RuntimeError,"controlled setup failure"):
                    b2_smoke.run(output, Path(directory))
            text = (output / "b2-report.json").read_text(); report = json.loads(text)
            self.assertEqual(report["status"], "FAIL")
            self.assertEqual(report["cleanup"], "FAIL")
            attempt = report["cleanup_detail"]["trial"]["attempts"][0]
            self.assertEqual(attempt["status"], "UNKNOWN")
            self.assertEqual(attempt["failure_type"], "OSError")
            self.assertNotIn("secret diagnostic", text)

    def test_component_failure_does_not_skip_subsequent_attempt(self):
        report = {}; observer = CleanupReceipts(report, ("resources",))
        def failure(): raise OSError("controlled secret diagnostic")
        observer.attempt("resources", "first", failure)
        observer.attempt("resources", "second", lambda: None)
        observer.finish()
        self.assertEqual(report["cleanup"], "FAIL")
        self.assertEqual([a["status"] for a in report["cleanup_detail"]["resources"]["attempts"]], ["UNKNOWN", "PASS"])
        self.assertNotIn("secret diagnostic", json.dumps(report))


if __name__ == "__main__":
    unittest.main()
