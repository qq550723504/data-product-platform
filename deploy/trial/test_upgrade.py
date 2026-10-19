"""Upgrade boundaries; no retained runtime or provider mutation."""
import copy
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import runtime as r
import resume as c
import upgrade as u


class BusinessUpgradeContracts(unittest.TestCase):
    def facts(self):
        return {"workspaceId": "original", "objects": [{"sha256": "original-raw"}],
                "facts": {"operations": [{"id": "original-batch"}], "receipts": [{"id": "original-receipt"}],
                          "bindings": [{"version_id": "raw"}], "versions": [{"id": "raw", "hash": "original"}]}}

    def test_append_new_versions_preserves_original_raw_but_rewrite_refused(self):
        before = self.facts(); after = copy.deepcopy(before)
        after["facts"]["versions"].append({"id": "new-standardized"})
        u.raw_equal(before, after)
        for change in (lambda v: v["objects"][0].update(sha256="changed"),
                       lambda v: v["facts"]["versions"][0].update(hash="changed"),
                       lambda v: v["facts"]["receipts"].clear()):
            broken = copy.deepcopy(after); change(broken)
            with self.assertRaises(r.Refused): u.raw_equal(before, broken)

    def test_old_cost_evidence_and_source_rows_cannot_disappear_or_change(self):
        before = {"core": {"cost_event": [{"id": "old", "amount": 2}]}, "source": {"energy": [{"value": -1}]}}
        after = copy.deepcopy(before); after["core"]["cost_event"].append({"id": "new", "amount": 3})
        u.retained_equal(before, after)
        after["core"]["cost_event"][0]["amount"] = 0
        with self.assertRaisesRegex(r.Refused, "cost_event"): u.retained_equal(before, after)
        after = copy.deepcopy(before); after["source"]["energy"][0]["value"] = 1
        with self.assertRaisesRegex(r.Refused, "source rows"): u.retained_equal(before, after)

    def test_upgrade_does_not_admit_shared_core_migration_or_industry_changes(self):
        for path in ("apps/platform/internal/workflow/application/execution.go", "migrations/new.sql", "industry-packs/park/policy.yaml"):
            self.assertFalse(u.allowed(path))
        for path in ("deploy/trial/upgrade.py", "apps/web/next.config.mjs"):
            self.assertTrue(u.allowed(path))

    def test_manifest_must_match_immutable_hash_and_original_identity(self):
        with tempfile.TemporaryDirectory() as d:
            private = Path(d); folder = private / "upgrades" / "attempt"; folder.mkdir(parents=True)
            path = folder / "manifest.json"
            s = {"private": str(private), "runId": "old", "workspaceId": "old", "actorId": "old", "root": "old"}
            manifest = {k: s[k] for k in ("runId", "workspaceId", "actorId", "root")}
            manifest.update(head="a" * 40, sources={}, artifacts={"binaries": {}, "web": {"root": str(folder / "web"), "files": {}}})
            c.write_once(path, manifest)
            s["businessBinding"] = {"path": str(path), "sha256": r.digest(path.read_bytes())}
            with patch.object(c, "committed_sources", return_value={}):
                u.verify_business(s)
                s["actorId"] = "new"
                with self.assertRaisesRegex(r.Refused, "identity drift"): u.verify_business(s)
                s["actorId"] = "old"; path.write_text("changed")
                with self.assertRaisesRegex(r.Refused, "manifest drift"): u.verify_business(s)

    def test_plain_resume_still_refuses_unapproved_business_source(self):
        with patch.object(u, "verify_business", return_value={"sources": {"apps/web/app/batches/actions.ts": "approved"}}):
            s = {"businessBinding": {}, "sources": {}}
            self.assertEqual(c.check_sources(s, {"apps/web/app/batches/actions.ts": "approved"}), [])
            with self.assertRaisesRegex(r.Refused, "Business source drift"):
                c.check_sources(s, {"apps/web/app/batches/actions.ts": "unapproved"})

    def test_duplicate_upgrade_is_read_only(self):
        s = {"businessBinding": {}}
        with patch.object(r, "status", return_value={"phase": "RUNNING"}), patch.object(u, "target") as target, patch.object(u, "prerequisites") as commands:
            self.assertEqual(u.upgrade(s), {"phase": "RUNNING"})
        target.assert_not_called(); commands.assert_not_called()

    def test_build_failure_retains_old_artifacts_and_never_stops_or_seeds(self):
        with tempfile.TemporaryDirectory() as d, patch.object(r, "STATE", Path(d) / "state"):
            r.STATE.mkdir(); private = Path(d) / "private"; private.mkdir()
            s = {"private": str(private), "phase": "RUNNING", "runId": "kept", "binaries": {"api": {"path": "original"}}}
            r.save(s); original = (r.STATE / "state.json").read_bytes()
            with patch.object(u, "target", return_value=({"head": "a" * 40, "deltaPaths": []}, b"workflow")), patch.object(r, "inspect_facts", return_value={}), patch.object(u, "retained", return_value={}), patch.object(u, "build", side_effect=r.Refused("build failed")), patch.object(u, "prerequisites") as business, patch.object(r, "stop") as stop, patch.object(r, "seed_source") as seed:
                with self.assertRaisesRegex(r.Refused, "build failed"): u.upgrade(s)
            self.assertEqual(s["binaries"]["api"]["path"], "original")
            self.assertEqual(s["phase"], "UNKNOWN")
            receipts = Path(s["upgrade"]["receipts"])
            self.assertEqual((receipts / "before-state.bytes").read_bytes(), original)
            self.assertEqual(r.read(receipts / "failed.json")["status"], "UNKNOWN")
            business.assert_not_called(); stop.assert_not_called(); seed.assert_not_called()


if __name__ == "__main__": unittest.main()
