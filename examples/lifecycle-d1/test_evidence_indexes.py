"""Real file/schema/hash checks; no Docker, database or provider calls."""
import copy
import hashlib
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import evidence_indexes as audit


def raw(value):
    return (json.dumps(value, ensure_ascii=False) + '\n').encode()


class EvidenceIndexTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.original_temp = Path(self.tmp.name)
        self.root = self.original_temp.resolve()
        self.review = self.root / 'review'
        self.review.mkdir()
        self.member = self.review / 'original.txt'
        self.member.write_bytes(b'original raw bytes\r\n')
        self.attrs = {'sha256': audit.digest(self.member.read_bytes()),
                      'bytes': self.member.stat().st_size}
        self.index = self.review / 'index.json'
        self.candidate = self.root / 'candidate.json'
        self.candidate.write_bytes(raw({'Execution': 'new-execution'}))
        self.plan = self.root / 'plan.json'

    def document(self, kind):
        if kind == audit.FILES_MAP:
            return {'status': 'PASS_REVIEW', 'count': 1,
                    'files': {'original.txt': dict(self.attrs)}}
        return {'status': 'PASS_REVIEW', 'members': [
            {'path': 'review/original.txt', **self.attrs}]}

    def specification(self, kind, document=None):
        self.index.write_bytes(raw(self.document(kind) if document is None else document))
        return {'id': 'review', 'index': 'review/index.json',
                'sha256': audit.digest(self.index.read_bytes()), 'format': kind,
                'member_root': 'review' if kind == audit.FILES_MAP else '.',
                'expected_count': 1, 'expected_status': 'PASS_REVIEW'}

    def make_plan(self, specs):
        document = {'version': 1, 'execution': 'new-execution',
                    'candidate_sha256': audit.digest(self.candidate.read_bytes()),
                    'indexes': specs}
        self.plan.write_bytes(raw(document))
        return audit.digest(self.plan.read_bytes())

    def test_both_explicit_formats_verify_original_bytes(self):
        for kind in (audit.FILES_MAP, audit.MEMBERS_LIST):
            with self.subTest(kind=kind):
                spec = self.specification(kind)
                before = self.member.read_bytes()
                result = audit.audit_plan(self.root, self.plan,
                                         self.make_plan([spec]), self.candidate)
                self.assertEqual(result['members_verified'], 1)
                self.assertFalse(result['execution_authority'])
                self.assertEqual(self.member.read_bytes(), before)

    def test_original_temp_spelling_and_canonical_root_both_pass(self):
        # Windows hosted TEMP can start with RUNNER~1 while resolve() returns
        # runneradmin. Exercise both spellings through the real plan reader.
        spec = self.specification(audit.FILES_MAP)
        pinned = self.make_plan([spec])
        for root, plan in [(self.root, self.original_temp / 'plan.json'),
                           (self.original_temp, self.plan)]:
            with self.subTest(root=root, plan=plan):
                self.assertEqual(audit.audit_plan(root, plan, pinned,
                                                 self.candidate)['members_verified'], 1)

    def test_plan_parent_link_is_rejected_before_normalization(self):
        spec = self.specification(audit.FILES_MAP)
        pinned = self.make_plan([spec])
        real = Path.is_junction
        with patch.object(Path, 'is_junction', lambda path: path == self.root or real(path)):
            with self.assertRaisesRegex(ValueError, 'linked evidence path'):
                audit.audit_plan(self.root, self.plan, pinned, self.candidate)

    def test_order_changes_are_valid_only_when_new_index_bytes_are_pinned(self):
        other = self.review / 'second.txt'
        other.write_bytes(b'second')
        second = {'sha256': audit.digest(b'second'), 'bytes': 6}
        for kind in (audit.FILES_MAP, audit.MEMBERS_LIST):
            document = self.document(kind)
            if kind == audit.FILES_MAP:
                document.update(count=2, files={'second.txt': second,
                                               'original.txt': self.attrs})
            else:
                document['members'].insert(0, {'path': 'review/second.txt', **second})
            spec = self.specification(kind, document)
            spec['expected_count'] = 2
            self.assertEqual(audit.audit_index(self.root, spec)['members_verified'], 2)
            self.index.write_bytes(json.dumps(document, sort_keys=True).encode())
            with self.assertRaises(ValueError):
                audit.audit_index(self.root, spec)

    def test_mixed_swapped_unknown_or_legacy_formats_are_rejected(self):
        for kind in (audit.FILES_MAP, audit.MEMBERS_LIST):
            opposite = audit.MEMBERS_LIST if kind == audit.FILES_MAP else audit.FILES_MAP
            documents = [self.document(opposite), {'status': 'PASS_REVIEW'},
                         dict(self.document(audit.FILES_MAP),
                              members=self.document(audit.MEMBERS_LIST)['members'])]
            for document in documents:
                with self.subTest(kind=kind, document=document), self.assertRaises(ValueError):
                    audit.audit_index(self.root, self.specification(kind, document))
        spec = self.specification(audit.FILES_MAP)
        for kind in ('auto', 'members-map-v1', '', None, []):
            with self.subTest(kind=kind), self.assertRaises(ValueError):
                audit.audit_index(self.root, dict(spec, format=kind))

    def test_member_count_status_metadata_and_type_failures(self):
        for kind in (audit.FILES_MAP, audit.MEMBERS_LIST):
            document = self.document(kind)
            mutations = [dict(document, status='FAILED'),
                         dict(document, **({'files': []} if kind == audit.FILES_MAP else {'members': {}})),
                         dict(document, **({'files': {}} if kind == audit.FILES_MAP else {'members': []}))]
            if kind == audit.FILES_MAP:
                mutations += [dict(document, count=x) for x in (True, 0, '1', 2)]
            for item in mutations:
                with self.subTest(kind=kind, item=item), self.assertRaises(ValueError):
                    audit.audit_index(self.root, self.specification(kind, item))
            for key, value in [('bytes', True), ('bytes', -1), ('bytes', '20'),
                               ('bytes', 999), ('sha256', 'F' * 64),
                               ('sha256', 0), ('sha256', '0' * 64)]:
                item = copy.deepcopy(document)
                entry = item['files']['original.txt'] if kind == audit.FILES_MAP else item['members'][0]
                entry[key] = value
                with self.subTest(kind=kind, key=key, value=value), self.assertRaises(ValueError):
                    audit.audit_index(self.root, self.specification(kind, item))
        for count in (True, 0, -1, '1', 2):
            spec = self.specification(audit.FILES_MAP)
            with self.subTest(expected_count=count), self.assertRaises(ValueError):
                audit.audit_index(self.root, dict(spec, expected_count=count))

    def test_missing_changed_truncated_and_extra_member_fields(self):
        spec = self.specification(audit.FILES_MAP)
        self.member.write_bytes(b'changed')
        with self.assertRaises(ValueError):
            audit.audit_index(self.root, spec)
        self.member.unlink()
        with self.assertRaises(FileNotFoundError):
            audit.audit_index(self.root, spec)
        for kind in (audit.FILES_MAP, audit.MEMBERS_LIST):
            document = self.document(kind)
            entry = document['files']['original.txt'] if kind == audit.FILES_MAP else document['members'][0]
            entry['unexpected'] = True
            with self.subTest(kind=kind), self.assertRaises(ValueError):
                audit.audit_index(self.root, self.specification(kind, document))

    def test_escapes_ambiguous_paths_and_duplicate_members_are_rejected(self):
        for name in ('../outside', '/absolute', 'C:/outside', 'review\\original.txt',
                     './review/original.txt', 'review//original.txt',
                     'review/original.txt ', 'review/original.txt.', ''):
            document = self.document(audit.MEMBERS_LIST)
            document['members'][0]['path'] = name
            with self.subTest(path=name), self.assertRaises(ValueError):
                audit.audit_index(self.root, self.specification(audit.MEMBERS_LIST, document))
        document = self.document(audit.MEMBERS_LIST)
        document['members'] *= 2
        spec = self.specification(audit.MEMBERS_LIST, document)
        with self.assertRaises(ValueError):
            audit.audit_index(self.root, dict(spec, expected_count=2))
        document = {'status': 'PASS_REVIEW', 'members': [
            {'path': 'review/index.json', **self.attrs}]}
        with self.assertRaises(ValueError):
            audit.audit_index(self.root, self.specification(audit.MEMBERS_LIST, document))

    def test_duplicate_json_keys_nonobjects_nonfinite_and_bad_utf8(self):
        for value in (b'{"files":{},"files":{}}', b'[]', b'{"x":NaN}',
                      b'{"x":Infinity}', b'\xff', b'{"nested":{"x":1,"x":2}}'):
            with self.subTest(value=value), self.assertRaises((ValueError, UnicodeError)):
                audit.decode(value)

    def test_symlink_member_and_parent_are_rejected(self):
        alias = self.review / 'alias'
        try:
            alias.symlink_to(self.root, target_is_directory=True)
        except OSError:
            self.skipTest('OS does not permit symlinks for this account')
        document = self.document(audit.MEMBERS_LIST)
        document['members'][0]['path'] = 'review/alias/review/original.txt'
        with self.assertRaises(ValueError):
            audit.audit_index(self.root, self.specification(audit.MEMBERS_LIST, document))

    def test_junction_flag_rejects_before_reading_linked_member(self):
        spec = self.specification(audit.FILES_MAP)
        real = Path.is_junction
        with patch.object(Path, 'is_junction', lambda path: path == self.member or real(path)):
            with self.assertRaisesRegex(ValueError, 'linked evidence path'):
                audit.audit_index(self.root, spec)

    def test_plan_binding_shape_duplicates_and_pinned_digest(self):
        spec = self.specification(audit.FILES_MAP)
        good = self.make_plan([spec])
        with self.assertRaises(ValueError):
            audit.audit_plan(self.root, self.plan, '0' * 64, self.candidate)
        original = json.loads(self.plan.read_bytes())
        for key, value in [('version', True), ('execution', 'old-execution'),
                           ('candidate_sha256', '0' * 64), ('indexes', []),
                           ('indexes', {}), ('indexes', [spec, spec])]:
            changed = dict(original, **{key: value})
            self.plan.write_bytes(raw(changed))
            with self.subTest(field=key, value=value), self.assertRaises(ValueError):
                audit.audit_plan(self.root, self.plan, audit.digest(self.plan.read_bytes()), self.candidate)
        self.plan.write_bytes(raw(original))
        with self.assertRaises(ValueError):
            audit.audit_plan(self.review, self.plan, good, self.candidate)


if __name__ == '__main__':
    unittest.main()
