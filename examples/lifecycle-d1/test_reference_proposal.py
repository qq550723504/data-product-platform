"""Reject stale or expanded approval inputs without touching a retained runtime."""
import copy
import hashlib
import importlib.util
import json
import unittest
from functools import lru_cache
from pathlib import Path
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location('proposal', Path(__file__).with_name('check-reference-proposal.py'))
m = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(m)


class ProposalConsistencyTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.proposal = m.decode(m.read(m.ROOT, m.PROPOSAL))
        cls.reader = staticmethod(lru_cache(maxsize=None)(m.read))

    def run_check(self, proposal, facts_only=False):
        with patch.object(m, 'read', self.reader):
            return m.check_facts(proposal) if facts_only else m.check(proposal)

    def test_current_public_receipts_are_consistent_but_do_not_grant_send(self):
        result = self.run_check(self.proposal)
        self.assertEqual(result['audit_core'], 'dpp-lifecycle307-5d2274ee-readonly-audit-core-04')
        self.assertEqual(result['runtime_source_files'], 317)
        self.assertIs(result['snapshot_is_send_authorization'], False)
        self.assertEqual(result['provider_io'], 0)

    def test_stale_generation_or_scope_expansion_is_rejected(self):
        cases = {
            'stale audit': lambda p: p.update(audit_core=p['audit_core'][:-2] + '03'),
            'parallel current alias': lambda p: p.update(this_preparation={'journal_database': 'audit-core-03'}),
            'old current receipt': lambda p: p['current_readonly']['checkpoint'].update(path=m.BASE + 'lc13-readonly01/5d-03/readonly-checkpoint.json'),
            'new source': lambda p: p['candidate'].update(source_commit='0' * 40),
            'new binary': lambda p: p['candidate'].update(binary_sha256='0' * 64),
            'formatted IAM bytes': lambda p: p['candidate'].update(iam_policy_sha256='0' * 64),
            'other workspace': lambda p: p.update(workspace_id='00000000-0000-0000-0000-000000000001'),
            'other provider': lambda p: p.update(provider_instance_id='00000000-0000-0000-0000-000000000001'),
            'changed version': lambda p: p['targets'][0].update(provider_version='other-version'),
            'survivor as target': lambda p: p['targets'].__setitem__(0, copy.deepcopy(p['survivors'][0])),
            'duplicate target': lambda p: p['targets'].__setitem__(0, copy.deepcopy(p['targets'][1])),
            'invented protection': lambda p: p['targets'][0].update(legal_hold=True),
            'invented fresh time': lambda p: p['targets'][0].update(protection_observed_at='2026-10-15T00:00:00Z'),
            'omit fresh gate': lambda p: p['targets'][0].update(requires_fresh_protection_before_send=False),
            'wrong ledger': lambda p: p['source_sql_counts'].update(storage_calls=112),
            'three sends': lambda p: p['execution_budget'].update(max_provider_delete_requests=3),
            'allow bypass': lambda p: p['execution_budget'].update(retention_bypass=1),
            'enable permission': lambda p: p['authority'].update(permission_enabled=True),
            'invent confirmation': lambda p: p['authority'].update(human_confirmation='RECEIVED'),
            'open business': lambda p: p['authority'].update(business_windows='OPEN'),
            'claim D1': lambda p: p.update(full_D1='ACCEPTED'),
            'claim authorization': lambda p: p.update(snapshot_is_send_authorization=True),
        }
        for name, mutate in cases.items():
            with self.subTest(name=name):
                changed = copy.deepcopy(self.proposal)
                mutate(changed)
                with self.assertRaises(ValueError):
                    self.run_check(changed, facts_only=True)

    def test_complete_fixed_proposal_rejects_unconsumed_and_added_fields(self):
        cases = {
            'network': lambda p: p.update(network_id='0' * 64),
            'inventory plan': lambda p: p.update(inventory_plan_id='00000000-0000-0000-0000-000000000000'),
            'source container': lambda p: p['source_services'][0].update(id='0' * 64),
            'cleanup': lambda p: p.update(cleanup=['delete retained backups']),
            'unknown resend': lambda p: p['execution_budget'].update(unknown_delete_resend='ALLOW'),
            'serial order': lambda p: p.update(serial_sequence=['send B before A without recovery']),
            'new approval omitted': lambda p: p['authority'].update(new_basis_and_approval_required=False),
            'old trial identity': lambda p: p['old_trial_attempts_resolved_not_sent'][0].update(id='0' * 36),
            'backup digest': lambda p: p['backup_restore_evidence']['historical_checkpoint'].update(core_dump_sha256='0' * 64),
            'backup receipt': lambda p: p['backup_restore_evidence']['receipt'].update(sha256='0' * 64),
            'new shadow authority': lambda p: p.update(execution_authorized=True),
            'boolean number alias': lambda p: p['authority'].update(new_basis_and_approval_required=1),
        }
        for name, mutate in cases.items():
            with self.subTest(name=name):
                changed = copy.deepcopy(self.proposal)
                mutate(changed)
                with self.assertRaisesRegex(ValueError, 'Input differs from indexed'):
                    self.run_check(changed)

    def test_fixed_index_rejects_artifact_bytes_and_membership_drift(self):
        for path in m.INDEXED_PATHS:
            with self.subTest(path=path):
                def changed(root, name):
                    raw = self.reader(root, name)
                    return raw + b' ' if name == path else raw
                with patch.object(m, 'read', changed):
                    with self.assertRaisesRegex(ValueError, 'Indexed artifact bytes drift'):
                        m.check(self.proposal)
        index = m.decode(self.reader(m.ROOT, m.INDEX))
        index['files'][-1] = copy.deepcopy(index['files'][0])
        with patch.object(m, 'read', lambda root, path: json_bytes(index) if path == m.INDEX else self.reader(root, path)):
            with self.assertRaisesRegex(ValueError, 'Fixed v6 index membership drift'):
                m.check(self.proposal)

    def test_json_duplicate_keys_and_path_escape_fail_closed(self):
        with self.assertRaisesRegex(ValueError, 'Duplicate JSON key'):
            m.decode(b'{"permission_enabled":false,"permission_enabled":true}')
        with self.assertRaisesRegex(ValueError, 'Repository path escaped'):
            m.read(m.ROOT, '../outside.json')

    def test_runtime_text_accepts_lf_checkout_but_rejects_source_change(self):
        runtime = m.decode(m.read(m.ROOT, self.proposal['runtime_text_binding']['path']))['files']

        def lf_checkout(root, path):
            raw = self.reader(root, path)
            return raw.replace(b'\r\n', b'\n') if path in runtime else raw

        with patch.object(m, 'read', lf_checkout):
            self.assertEqual(m.check(self.proposal)['runtime_source_files'], 317)

        def changed_checkout(root, path):
            raw = lf_checkout(root, path)
            return raw + b'// changed\n' if path == next(iter(runtime)) else raw

        with patch.object(m, 'read', changed_checkout):
            with self.assertRaisesRegex(ValueError, 'Runtime text differs from frozen raw source'):
                m.check(self.proposal)

    def test_source_and_lf_manifest_cannot_self_attest_to_frozen_source(self):
        path = 'apps/platform/cmd/lifecycle-fixture-delete/reference_send.go'
        changed_source = self.reader(m.ROOT, path).replace(b'\r\n', b'\n') + b'// changed\n'
        manifest_path = self.proposal['runtime_text_binding']['path']
        manifest = m.decode(self.reader(m.ROOT, manifest_path))
        manifest['files'][path] = hashlib.sha256(changed_source).hexdigest()
        changed_manifest = json_bytes(manifest)
        proposal = copy.deepcopy(self.proposal)
        proposal['runtime_text_binding']['sha256'] = hashlib.sha256(changed_manifest).hexdigest()

        def changed(root, name):
            if name == path:
                return changed_source
            return changed_manifest if name == manifest_path else self.reader(root, name)

        # Bypass the outer fixed-proposal comparison deliberately: even a newly
        # re-indexed manifest must not attest modified text to the old raw binding.
        with patch.object(m, 'read', changed):
            with self.assertRaisesRegex(ValueError, 'Runtime text differs from frozen raw source'):
                m.check_facts(proposal)


def json_bytes(value):
    return json.dumps(value).encode('utf-8')


if __name__ == '__main__':
    unittest.main()
