import copy
import importlib.util
import json
import unittest
from pathlib import Path
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location('refresh', Path(__file__).with_name('check-reference-refresh.py'))
m = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(m)


class NativeRefreshContract(unittest.TestCase):
    def setUp(self):
        self.proposal = m.load(m.PROPOSAL)

    def test_fresh_snapshot_preserves_reviewed_scope_and_blocked_authority(self):
        result = m.check(self.proposal)
        self.assertEqual(result['send_authorization'], 'NOT_RECEIVED')
        self.assertEqual(result['native_storage_calls'], 20)
        self.assertEqual(len(self.proposal['targets']), 2)
        self.assertEqual(len(self.proposal['survivors']), 4)

    def test_expired_survivor_retention_never_expands_targets(self):
        survivors = {v['copy_id']: v for v in self.proposal['survivors']}
        expired = survivors['dbe34c34-cecb-4183-a121-5eac1035d176']
        self.assertEqual(expired['retention_mode'], 'GOVERNANCE')
        self.assertIs(expired['retention_active_at_observation'], False)
        self.assertNotIn(expired['copy_id'], {v['copy_id'] for v in self.proposal['targets']})

    def test_any_proposal_scope_or_authority_change_is_rejected(self):
        for mutate in (
            lambda p: p.update(network_id='0' * 64),
            lambda p: p.update(cleanup=['delete backups']),
            lambda p: p['authority'].update(permission_enabled=True),
            lambda p: p['targets'][0].update(protection_observed_at='future'),
            lambda p: p['targets'].append(copy.deepcopy(p['survivors'][0])),
            lambda p: p.update(execution_authorized=True),
        ):
            changed = copy.deepcopy(self.proposal)
            mutate(changed)
            with self.assertRaisesRegex(ValueError, 'Input differs from fixed v7'):
                m.check(changed)

    def test_facts_reject_stale_or_changed_native_identity_and_protection(self):
        original = m.load
        cases = (
            ('native-six-version.json', lambda v: v['versions'][0]['protection'].update(LegalHold=True)),
            ('native-six-version.json', lambda v: v['versions'][0]['protection'].update(ObservedAt='2026-10-14T16:22:28Z')),
            ('native-six-version.json', lambda v: v['inventories']['dpp-lifecycle307-online']['Versions'].pop()),
            ('readonly-checkpoint.json', lambda v: v.update(source_pg='0' * 64)),
            ('native-helper.json', lambda v: v.update(binary_sha256='0' * 64)),
            ('native-iam-readonly.json', lambda v: v['facts'][0].update(status='enabled')),
            ('current-core-readback.json', lambda v: v['counts'].update(active_pins=1)),
            ('retained-final-state.json', lambda v: v['states'].update(source_pg='running')),
        )
        for name, mutate in cases:
            with self.subTest(name=name):
                def changed(path, root=m.ROOT):
                    value = original(path, root)
                    if path == m.NATIVE + name:
                        mutate(value)
                    return value
                with patch.object(m, 'load', changed):
                    with self.assertRaises(ValueError):
                        m.expected_proposal(self.proposal['created_at'])

    def test_fixed_index_rejects_bytes_or_duplicate_membership(self):
        original_read = m.v6.read
        with patch.object(m.v6, 'read', lambda root, path: original_read(root, path) + (b' ' if path == m.NATIVE + 'native-helper.json' else b'')):
            with self.assertRaisesRegex(ValueError, 'Refresh artifact bytes drift'):
                m.check(self.proposal)
        index = m.load(m.PUBLIC + 'evidence-index.json')
        index['files'][-1] = copy.deepcopy(index['files'][0])
        with patch.object(m.v6, 'read', lambda root, path: json.dumps(index).encode() if path == m.PUBLIC + 'evidence-index.json' else original_read(root, path)):
            with self.assertRaisesRegex(ValueError, 'Refresh index membership drift'):
                m.check(self.proposal)


if __name__ == '__main__':
    unittest.main()
