"""Offline contract tests; Docker and Core are explicitly modeled, never started."""
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch
import uuid

spec = importlib.util.spec_from_file_location('old_dispatch_audit', Path(__file__).with_name('audit-old-business-dispatches.py'))
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)


def row(name):
    plan, copy_id, dispatch, attempt = m.CASES[name]
    return {'case': name, 'attempts': [{'id': attempt, 'workspace_id': m.WORKSPACE, 'plan_id': plan, 'copy_id': copy_id, 'dispatch_id': dispatch}],
            'dispatches': [{'id': dispatch, 'workspace_id': m.WORKSPACE, 'plan_id': plan, 'copy_id': copy_id, 'approval_id': 'approval'}],
            'approvals': [{'id': 'approval', 'basis_ref': 'REMAINING_NON_DELETE_REHEARSAL:' + m.EXECUTION, 'principal_ref': 'remaining-fixture:' + m.EXECUTION}],
            'claims': [], 'observations': [], 'releases': [], 'absences': [], 'pins': [{'preserved': True}], 'revocations': [], 'unresolved': 1}


class OldDispatchAuditTests(unittest.TestCase):
    def test_three_exact_historical_identities_exclude_P1(self):
        text = m.query()
        for ids in m.CASES.values():
            for value in ids:
                self.assertIn(value, text)
        self.assertNotIn('5a4c9789-3e67-4e54-ab58-128d111cbd65', text)
        self.assertEqual(set(m.CASES), {'P2', 'T1', 'T2'})

    def test_core_no_conflict_is_never_NOT_SENT_authority(self):
        for name in m.CASES:
            result = m.classify(row(name))
            self.assertEqual(result['status'], 'CORE_NO_CONFLICT_NOT_YET_TRUSTED_NOT_SENT')
            self.assertIs(result['proof_signed'], False)
            self.assertEqual(result['business_mutations'], 0)

    def test_each_actual_conflict_is_retained(self):
        for relation in ('claims', 'observations', 'releases', 'absences'):
            with self.subTest(relation=relation):
                value = row('P2')
                value[relation] = [{'original': 'UNKNOWN' if relation == 'observations' else 'retained'}]
                before = copy.deepcopy(value)
                result = m.classify(value)
                self.assertEqual(result['status'], 'CONFLICT_REQUIRES_REVIEW')
                self.assertEqual(result['conflicts'], {relation: 1})
                self.assertEqual(value, before)

    def test_extra_attempt_dispatch_or_foreign_binding_fails_closed(self):
        mutations = [lambda r: r['attempts'].append(copy.deepcopy(r['attempts'][0])),
                     lambda r: r['dispatches'].append(copy.deepcopy(r['dispatches'][0])),
                     lambda r: r['attempts'][0].update(workspace_id=str(uuid.uuid4())),
                     lambda r: r['dispatches'][0].update(copy_id=str(uuid.uuid4())),
                     lambda r: r['approvals'][0].update(principal_ref='remaining-fixture:other'),
                     lambda r: r['approvals'][0].update(basis_ref='old approval reused')]
        for change in mutations:
            value = row('T1')
            change(value)
            with self.subTest(change=change), self.assertRaises(ValueError):
                m.classify(value)

    def test_real_shared_sql_helper_wraps_in_repeatable_read_read_only(self):
        native = m.module('audit_test_native', 'prepare-remaining-native.py')
        calls = []
        with patch.object(native, 'docker', side_effect=lambda *args, **kw: calls.append((args, kw)) or b'{}\n'):
            native.sql('fixed-Core', m.query())
        args, kw = calls[0]
        self.assertEqual(args[:3], ('exec', '-i', 'fixed-Core'))
        self.assertTrue(kw['data'].startswith(b'BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;'))
        self.assertTrue(kw['data'].endswith(b'ROLLBACK;'))
        self.assertNotIn(b'INSERT ', kw['data'])
        self.assertNotIn(b'UPDATE ', kw['data'])

    def harness(self, directory, fail_query=False, fail_stop=False, fail_readback=False, changed_core=False):
        root = Path(directory)
        commands = []
        fingerprint = iter([{'same': True}, {'same': not changed_core}])
        def sql(*args):
            if fail_query:
                raise RuntimeError('original query failure')
            return '\n'.join(json.dumps(row(n)) for n in m.CASES)
        native = SimpleNamespace(wait_pg=lambda *a: None, fingerprints=lambda *a: next(fingerprint), sql=sql)
        def docker(*args):
            commands.append(args)
            if args[0] == 'network':
                return b'[{"Internal":true,"Containers":{}}]'
            if args[0] == 'stop' and fail_stop:
                raise RuntimeError('stop failed')
            return b'fixed-Core\n'
        def inspect(*args):
            if commands and commands[-1][0] == 'stop' and fail_readback:
                raise RuntimeError('unreadable state')
            return {'Image': 'PG', 'HostConfig': {'Privileged': False}, 'NetworkSettings': {'Networks': {'fixed': {'NetworkID': 'net'}}}}
        def write(path, value):
            path.parent.mkdir(parents=True, exist_ok=True)
            with path.open('x', encoding='utf-8') as f:
                json.dump(value, f)
        p1 = SimpleNamespace(PRIVATE=root / 'private', PACKET=root / 'packet', PG='PG', docker=docker, inspect=inspect,
                             closed=lambda x: True, native_module=lambda: native, write=write,
                             sha=lambda p: 'f624a5f50ccb4fa4aa8b465f5a60a090f6325e78f60544b4c18f9ba2604ef243' if p.name == 'candidate.json' else '8e54b47e4a1f83c7ce66555ed6ce2f3ec66c962a394a8fa1feb84eeaccd82fc5',
                             load=lambda p: {'Groups': [{'Group': 'business-races', 'Core': 'fixed-Core', 'Network': 'net'}]})
        return p1, commands

    def run_modeled(self, **options):
        with tempfile.TemporaryDirectory() as directory:
            p1, commands = self.harness(directory, **options)
            with patch.object(m, 'ROOT', Path(directory)), patch.object(m, 'module', return_value=p1), patch.object(m, 'retained', return_value={'preserved': True}):
                audit_id = str(uuid.uuid4())
                result = m.run(audit_id)
                terminal = json.loads((Path(directory) / 'docs/acceptance/evidence/issue307-d1/post-v22-dispatch-audit' / audit_id / 'terminal.json').read_text())
                with self.assertRaises(FileExistsError):
                    m.run(audit_id)
            self.assertEqual([c for c in commands if c[0] in ('start', 'stop')], [('start', 'fixed-Core'), ('stop', '-t', '15', 'fixed-Core')])
            return result, terminal

    def test_success_composition_starts_only_core_and_stops_it(self):
        ok, terminal = self.run_modeled()
        self.assertTrue(ok)
        self.assertEqual(terminal['status'], 'PASS_CORE_READONLY_AUDIT_ONLY')
        self.assertFalse(terminal['fresh_IAM_or_native_verified'])

    def test_query_failure_still_stops_core_and_keeps_original_error(self):
        ok, terminal = self.run_modeled(fail_query=True)
        self.assertFalse(ok)
        self.assertEqual(terminal['failure'], 'RuntimeError: original query failure')
        self.assertTrue(terminal['source_stopped_PID0'])

    def test_failed_stop_and_unreadable_readback_are_unconfirmed(self):
        ok, terminal = self.run_modeled(fail_stop=True, fail_readback=True)
        self.assertFalse(ok)
        self.assertFalse(terminal['source_stopped_PID0'])

    def test_core_mutation_cannot_be_reported_as_pass(self):
        ok, terminal = self.run_modeled(changed_core=True)
        self.assertFalse(ok)
        self.assertIn('changed relations/roles/ACL', terminal['failure'])

    def test_preservation_gate_failure_prevents_all_core_start(self):
        for reason in ('budget slot missing', 'budget bytes drift', 'disabled profile drift'):
            with self.subTest(reason=reason), tempfile.TemporaryDirectory() as directory:
                p1, commands = self.harness(directory)
                with patch.object(m, 'ROOT', Path(directory)), patch.object(m, 'module', return_value=p1), patch.object(m, 'retained', side_effect=ValueError(reason)):
                    self.assertFalse(m.run(str(uuid.uuid4())))
                self.assertEqual(commands, [])

    def test_actual_retained_budget_missing_or_modified_rejects_frozen_anchor(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            public = base / 'public' / 'post-v22-p1-readonly04'
            old = {'owners': [str(n) for n in range(61)]}
            budget = {'business-races': {'spent-P2': {'bytes': 123, 'sha256': 'original'}, 'wire-P1': {'bytes': 456, 'sha256': 'original-wire'}}}
            facts = {str(public / 'preparation.json'): {'old_closed': old, 'backup_sha256': {'nine': 'unchanged'}, 'budgets': budget},
                     str(public / 'post-close-physical-readback.json'): {'audits': [{'owners': [str(n) for n in range(61, 77)]}]}}
            def digest(path):
                if path == public / 'preparation.json':
                    return '08575df192136760be928c199ab06df921bb3ef1bd13c81115d25886ba3eea4c'
                return hashlib.sha256(path.read_bytes()).hexdigest()
            for audit in ('01', '02', '03', '04'):
                private = base / ('post-v22-p1-readonly' + audit)
                inputs = {}
                for name in ('worker-private/recovery.json', 'root-private/p1-readonly-permission.json'):
                    p = private / name
                    p.parent.mkdir(parents=True, exist_ok=True)
                    p.write_text('{"Enabled":false,"exact":"binding"}', encoding='utf-8')
                    inputs[name] = digest(p)
                (private / 'manifest.json').write_text(json.dumps({'inputs': inputs}), encoding='utf-8')
                pub = public.parent / private.name / 'preparation.json'
                if pub != public / 'preparation.json':
                    pub.parent.mkdir(parents=True, exist_ok=True)
                    pub.write_bytes((private / 'manifest.json').read_bytes())
                else:
                    # The test models the separately pinned full audit04 manifest.
                    (private / 'manifest.json').write_bytes(b'{}')
                facts[str(pub)] = {**facts.get(str(pub), {}), 'inputs': inputs}
            real_digest = digest
            def anchored_digest(path):
                if path in (public / 'preparation.json', base / 'post-v22-p1-readonly04/manifest.json'):
                    return '08575df192136760be928c199ab06df921bb3ef1bd13c81115d25886ba3eea4c'
                return real_digest(path)
            load = lambda p: facts[str(p)] if str(p) in facts else json.loads(p.read_bytes())
            p1 = SimpleNamespace(PUBLIC=public, PRIVATE=base, old_closed=lambda: old, sha=anchored_digest, load=load,
                                 inspect=lambda cid: {}, closed=lambda x: True, input_entries=lambda v: v['inputs'].items(),
                                 backups=lambda: {'nine': 'unchanged'}, budgets=lambda: budget)
            self.assertEqual(m.retained(p1)['old_budgets'], budget)
            for changed in ({}, {'business-races': {}}, {'business-races': {'spent-P2': {'bytes': 123, 'sha256': 'changed'}}}):
                p1.budgets = lambda: changed
                with self.subTest(changed=changed), self.assertRaisesRegex(ValueError, 'backup/budget anchor drift'):
                    m.retained(p1)


if __name__ == '__main__':
    unittest.main()
