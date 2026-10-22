import copy
import importlib.util
import json
import unittest
from pathlib import Path
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location('v9', Path(__file__).with_name('check-reference-v10.py'))
m = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(m)
s = m.refresh
PUBLIC, NATIVE, _ = s.refresh_paths('09')
PREVIOUS = s.refresh_paths('08')[2]


class V9BasedRefreshContract(unittest.TestCase):
    def setUp(self):
        self.proposal = s.load(m.PROPOSAL)

    def test_fresh_v10_keeps_reviewed_v9_scope_and_closed_authority(self):
        result = m.check(self.proposal)
        self.assertEqual(result['baseline'], 'INDEPENDENTLY_REVIEWED_V9')
        self.assertEqual(result['native_storage_calls'], 20)
        self.assertEqual(result['management_gets_this_refresh'], 4)
        self.assertEqual(result['send_authorization'], 'NOT_RECEIVED')
        previous = s.load(PREVIOUS)
        for name in ('candidate', 'execution_budget', 'authority', 'excluded', 'serial_sequence'):
            self.assertEqual(self.proposal[name], previous[name])
        for name in ('targets', 'survivors'):
            clean = lambda values: [{k: v for k, v in value.items() if k != 'protection_observed_at'} for value in values]
            self.assertEqual(clean(self.proposal[name]), clean(previous[name]))

    def test_full_proposal_scope_and_activation_mutations_are_rejected(self):
        for mutate in (
            lambda p: p.update(network_id='0' * 64),
            lambda p: p['authority'].update(permission_enabled=True),
            lambda p: p['targets'].append(copy.deepcopy(p['survivors'][0])),
            lambda p: p.update(execution_authorized=True),
        ):
            changed = copy.deepcopy(self.proposal)
            mutate(changed)
            with self.assertRaisesRegex(ValueError, 'Input differs from fixed v10'):
                m.check(changed)

    def test_inner_facts_reject_stale_v9_time_and_reopened_closure(self):
        original = s.load
        prior = original(PREVIOUS)
        times = {v['copy_id']: v['protection_observed_at'] for v in prior['targets'] + prior['survivors']}
        def stale(v):
            for value in v['versions']:
                value['protection']['ObservedAt'] = times[value['copy']['ID']]
        for name, mutate in (
            ('native-six-version.json', stale),
            ('readonly-checkpoint.json', lambda v: v.update(source_pg='0' * 64)),
            ('current-core-readback.json', lambda v: v['counts'].update(active_pins=1)),
            ('retained-final-state.json', lambda v: v['states'].update(source_pg='running')),
        ):
            def changed(path, root=s.ROOT):
                value = original(path, root)
                if path == NATIVE + name:
                    mutate(value)
                return value
            with self.subTest(name=name), patch.object(s, 'load', changed):
                with self.assertRaises(ValueError):
                    s.expected_proposal(self.proposal['created_at'], generation='09')

    def test_fixed_v10_index_rejects_bytes_and_duplicate_membership(self):
        read = s.v6.read
        with patch.object(s.v6, 'read', lambda root, path: read(root, path) + (b' ' if path == NATIVE + 'native-helper.json' else b'')):
            with self.assertRaisesRegex(ValueError, 'Refresh artifact bytes drift'):
                m.check(self.proposal)
        index = s.load(PUBLIC + 'evidence-index.json')
        index['files'][-1] = copy.deepcopy(index['files'][0])
        with patch.object(s.v6, 'read', lambda root, path: json.dumps(index).encode() if path == PUBLIC + 'evidence-index.json' else read(root, path)):
            with self.assertRaisesRegex(ValueError, 'Refresh index membership drift'):
                m.check(self.proposal)


if __name__ == '__main__':
    unittest.main()
