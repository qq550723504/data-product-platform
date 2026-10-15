import copy
import importlib.util
import hashlib
import json
from pathlib import Path
import sys
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location('candidate05_check', Path(__file__).with_name('check-reference-candidate05.py'))
m = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(m)


class Candidate05OfflineEvidence(unittest.TestCase):
    def reject(self, path, mutate):
        value = copy.deepcopy(m.load(path))
        mutate(value)
        # Exercise inner semantic checks, independently of the outer raw index.
        with self.assertRaises(ValueError):
            m.validate(lambda p: value if p == path else m.load(p))

    def test_complete_current_proposal_is_prepared_without_authority(self):
        m.validate()

    def test_authority_and_old_execution_cannot_be_substituted(self):
        self.reject('prepared-execution.json', lambda v: v.update(permission_enabled=True))
        self.reject('proposal.json', lambda v: v['authority'].update(reuse_previous_authorization=True))
        self.reject('source-binding.json', lambda v: v.update(execution='614cd9ad-3f9d-40be-b110-3f7cf95f0790'))
        self.reject('send-a/helper.json', lambda v: v.update(exit=0))

    def test_all_exact_scope_fields_are_bound_to_frozen_render(self):
        for field in m.SCOPE:
            with self.subTest(field=field):
                self.reject('proposal.json', lambda v: v.update({field: 'foreign-review-counterexample'}))

    def test_changed_render_artifact_and_matching_new_index_still_fail(self):
        original = m.raw
        path = m.DATA + 'render/delete-executor-policy.json'
        changed = original(path) + b'garbage'
        index = json.loads(original(m.DATA + 'evidence-index.json'))
        for entry in index['files']:
            if entry['path'] == path:
                entry.update(bytes=len(changed), sha256=hashlib.sha256(changed).hexdigest())
        def read(path_read):
            if path_read == path:
                return changed
            if path_read == m.DATA + 'evidence-index.json':
                return json.dumps(index).encode()
            return original(path_read)
        with patch.object(m, 'raw', read), self.assertRaisesRegex(ValueError, 'Frozen rendered artifact changed'):
            m.check()

    def test_native_scope_protection_and_iam_are_not_relaxed(self):
        self.reject('native/native-six-version.json', lambda v: v['versions'][0]['protection'].update(LegalHold=True))
        self.reject('native/native-six-version.json', lambda v: v['inventories']['dpp-lifecycle307-online']['Versions'][0].update(DeleteMarker=True))
        self.reject('proposal.json', lambda v: v['future_limits'].update(max_per_target=2))
        self.reject('iam/native-iam-readonly.json', lambda v: v['facts'][-1].update(status='enabled'))

    def test_checkpoint_history_accounting_and_seal_are_required(self):
        self.reject('preflight/checkpoint.json', lambda v: v.update(dump_sha256='0' * 64))
        self.reject('preflight/source-after.json', lambda v: v['relations'][0].update(rows=999))
        self.reject('preflight/prior-execution-closure.json', lambda v: v['results'].pop())
        self.reject('preflight/native-read-ledger.json', lambda v: v['facts']['costs'][0].update(quantity=0))
        self.reject('sealed-state.json', lambda v: v.update(old_private_seals_and_backups_unchanged=False))

    def test_cli_has_no_authorization_or_native_options(self):
        for args in (['delete'], ['--confirm'], ['--source', 'HEAD'], ['--execution', 'old']):
            with self.subTest(args=args), patch.object(sys, 'argv', ['check', *args]), patch.object(m, 'check') as check:
                with self.assertRaises(SystemExit):
                    m.main()
                check.assert_not_called()


if __name__ == '__main__':
    unittest.main()
