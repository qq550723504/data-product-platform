"""Reject stale or expanded approval inputs without touching a retained runtime."""
import copy
import importlib.util
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

    def run_check(self, proposal):
        with patch.object(m, 'read', self.reader):
            return m.check(proposal)

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
                    self.run_check(changed)

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
            with self.assertRaisesRegex(ValueError, 'Runtime source drift'):
                m.check(self.proposal)


if __name__ == '__main__':
    unittest.main()
