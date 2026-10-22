"""Real CLI/typed audit/three collect/finally composition; external I/O doubled.

This is NOT native acceptance: Docker, PostgreSQL, SDK reader and the Windows
checker are recorded doubles. The immutable saved indexes use actual bytes.
"""
from contextlib import ExitStack
import copy
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('six_entry_integration',
                                            Path(__file__).with_name('verify-six-native.py'))
v = importlib.util.module_from_spec(spec)
spec.loader.exec_module(v)
r, p = v.r, v.p


class SixNativeEntryTests(unittest.TestCase):
    def composition(self, fault=None, audit_only=False, authorized=False):
        # A recorded fixture must exist before the new generation is allocated.
        # This is a double, never a production candidate fallback.
        fixture = r.ROOT / 'docs/acceptance/evidence/issue307-d1/remaining-candidate16/candidate.json'
        groups = copy.deepcopy(v.load(fixture)['Groups'])
        with tempfile.TemporaryDirectory() as tmp, ExitStack() as stack:
            root = Path(tmp)
            packet, control = root / 'packet', root / 'control'
            packet.mkdir(); control.mkdir()
            evidence = root / 'docs/acceptance/evidence/issue307-d1'
            evidence.mkdir(parents=True)
            output = evidence / 'fresh-output'
            r.save(packet / 'candidate.json', {'Execution': p.EXECUTION, 'Groups': groups})
            r.save(packet / 'bindings.json', [{'group': g['Group']} for g in groups])
            (control / 'readonly-check.exe').write_bytes(b'recorded external checker')
            (control / 'six-map.json').write_bytes(b'NON_AUTHORITATIVE_TEST_MAP')
            allocation, indexes = [], []
            for n, g in enumerate(groups):
                group = g['Group']
                folder = control / group
                folder.mkdir(); (packet / group).mkdir()
                allocation.append({'group': group, 'private': str(folder)})
                for name in ('frozen-inventory.json', 'present-inventory.json'):
                    (folder / name).write_bytes(b'[]')
                r.save(packet / group / 'core-readonly.json', {'recorded_source': group})
                member = root / (group + '.txt')
                member.write_bytes(('ORIGINAL_' + group + '\r\n').encode())
                attrs = {'sha256': v.evidence_indexes.digest(member.read_bytes()),
                         'bytes': member.stat().st_size}
                name = group + '-index.json'
                if n == 0:
                    document = {'status': 'PASS_REVIEW', 'count': 1,
                                'files': {member.name: attrs}}
                    kind = v.evidence_indexes.FILES_MAP
                else:
                    document = {'status': 'PASS_REVIEW',
                                'members': [{'path': member.name, **attrs}]}
                    kind = v.evidence_indexes.MEMBERS_LIST
                r.save(root / name, document)
                indexes.append({'id': group, 'index': name, 'sha256': r.sha(root / name),
                                'format': kind, 'member_root': '.', 'expected_count': 1,
                                'expected_status': 'PASS_REVIEW'})
            r.save(control / 'allocation.json', {'groups': allocation})
            plan = root / 'audit-plan.json'
            r.save(plan, {'version': 1, 'execution': p.EXECUTION,
                          'candidate_sha256': r.sha(packet / 'candidate.json'),
                          'indexes': indexes})
            if fault == 'index-format':
                bad = r.load(root / indexes[0]['index'])
                bad['members'] = []
                r.replace(root / indexes[0]['index'], bad)
                indexes[0]['sha256'] = r.sha(root / indexes[0]['index'])
                data = r.load(plan); data['indexes'] = indexes; r.replace(plan, data)
            elif fault == 'index-bytes':
                (root / (groups[0]['Group'] + '.txt')).write_bytes(b'drift')
            elif fault == 'terminal':
                (control / 'authorized-execution01').mkdir()
                r.save(control / 'authorized-execution01/terminal.json', {'status': 'FAILED'})
            elif fault == 'terminal-parent-alias':
                history = control / 'authorized-history'
                history.mkdir()
                r.save(history / 'terminal.json', {'status': 'ORIGINAL_FAILED_RETAINED'})
                empty = control / 'empty-target'
                empty.mkdir()
                link = control / 'authorized-execution01'
                if os.name == 'nt':
                    import _winapi
                    _winapi.CreateJunction(str(empty), str(link))
                else:
                    link.symlink_to(empty, target_is_directory=True)
            argv = ['verify-six-native.py', '--candidate-sha256', r.sha(packet / 'candidate.json'),
                    '--bindings-sha256', r.sha(packet / 'bindings.json'),
                    '--checker-sha256', r.sha(control / 'readonly-check.exe'),
                    '--output', str(output), '--evidence-audit-plan', str(plan),
                    '--evidence-audit-plan-sha256', r.sha(plan)]
            if audit_only:
                argv.append('--audit-only')
            if authorized:
                (control/'authorized-execution01').mkdir(exist_ok=True)
                (control/'authorized-execution01/user-message-verbatim.txt').write_bytes(b'ACTUAL_RECORDED_AUTHORITY_DOUBLE')
                if fault != 'missing-freeze-flag':
                    argv += ['--execution-binding-sha256','a'*64]
            if fault == 'missing-plan':
                argv = argv[:-4]
            if fault == 'plan-binding':
                argv[-1] = '0' * 64
            if fault == 'existing-output':
                output.mkdir(); (output / 'original.txt').write_bytes(b'preserved')
            calls, states, owners, current = [], {}, {}, {}
            def branch_freeze(*args,**kwargs):
                calls.append(('branch-freeze',))
                if fault=='branch-freeze':raise ValueError('recorded unlocked or drifted branch')
                return {'status':'RECORDED_NATIVE_BRANCH_FREEZE_DOUBLE'}
            stack.enter_context(patch.object(v.execution_freeze,'authorized_gate',branch_freeze))

            def owner_state(cid, g):
                return {'Id': cid, 'Image': p.p1.PG, 'Name': '/' + cid,
                        'Config': {'User': '10001:10001', 'Entrypoint': ['/run/reader'],
                                   'Cmd': ['-mode', 'six-verify-native'], 'Env': [],
                                   'Labels': {'lifecycle307.six-readonly': p.EXECUTION}},
                        'State': {'Running': False, 'Pid': 0},
                        'HostConfig': {'NetworkMode': 'container:' + g['Native'],
                            'ReadonlyRootfs': True, 'Privileged': False, 'AutoRemove': False,
                            'RestartPolicy': {'Name': 'no'}, 'CapDrop': ['ALL'], 'CapAdd': None,
                            'SecurityOpt': ['no-new-privileges'], 'Tmpfs': {'/tmp': '',
                            '/var/lib/postgresql/data': 'size=1m'}, 'PortBindings': {}},
                        'Mounts': [], 'NetworkSettings': {'Ports': {}}}

            for g in groups:
                for cid in (g['Core'], g['Native']):
                    states[cid] = owner_state(cid, g)

            def sealed():
                calls.append(('sealed-gate',))
                if fault == 'sealed-gate':
                    raise ValueError('recorded real host gate rejection')
                return {'status': 'RECORDED_HOST_GATE_NOT_NATIVE_PROOF'}

            def docker(*args, **kw):
                calls.append(args)
                if args[0] == 'network':
                    g = next(x for x in groups if x['Network'] == args[2])
                    return json.dumps([{'Internal': True,
                        'Containers': {g['Core']: {}, g['Native']: {}}}]).encode()
                if args[0] == 'create':
                    native = args[args.index('--network') + 1].split(':', 1)[1]
                    g = next(x for x in groups if x['Native'] == native)
                    name = args[args.index('--name') + 1]
                    cid = hashlib.sha256(name.encode()).hexdigest()
                    states[cid] = owner_state(cid, g); states[cid]['Name'] = '/' + name
                    owners[cid] = g
                    for n, value in enumerate(args):
                        if value == '--mount':
                            fields = dict(x.split('=', 1) for x in args[n + 1].split(',') if '=' in x)
                            states[cid]['Mounts'].append({'Destination': fields['target'],
                                'Source': fields['source'], 'Type': 'bind',
                                'RW': 'readonly' not in args[n + 1].split(',')})
                    return cid.encode()
                if args[0] == 'start':
                    states[args[1]]['State'].update(Running=True, Pid=55)
                    if args[1] in owners:
                        g = owners[args[1]]; current['group'] = g['Group']
                        folder = output / g['Group'] / 'files'
                        r.save(folder / 'readonly-start-request.json', {'id': 'request', 'method': 'GET'})
                        r.save(folder / 'readonly-observation-request.json', {'id': 'request',
                            'status': 200, 'transport_error': False})
                return b''

            def process(args, **kw):
                if args[0] == 'docker' and args[1] == 'wait':
                    states[args[2]]['State'].update(Running=False, Pid=0)
                    return SimpleNamespace(returncode=0, stdout=b'0\n', stderr=b'')
                if args[0] == 'docker' and args[1] == 'logs':
                    return SimpleNamespace(returncode=0, stdout=b'RECORDED_READER', stderr=b'')
                if '-six' not in args:
                    raise AssertionError('unexpected process or legacy checker')
                group = args[args.index('-group') + 1]
                if fault == 'second-group-checker' and group == 'late-hold':
                    return SimpleNamespace(returncode=1, stdout=b'original-rejection', stderr=b'checker-failure')
                result = {'status': 'PASS_COMPLETE_SIX_NATIVE_AGGREGATION', 'group': group,
                          'copies': {'business-races': 7, 'late-hold': 6, 'mixed': 4}[group]}
                return SimpleNamespace(returncode=0, stdout=json.dumps(result).encode(), stderr=b'')

            def close(cid):
                calls.append(('close', cid)); states[cid]['State'].update(Running=False, Pid=0)
                return {'cid': cid, 'closed': True}

            def fingerprint(cid):
                group = next(x for x in groups if x['Core'] == cid)
                return {'recorded_source': group['Group']}

            cfg = {'Enabled': False, 'AuditID': '00000000-0000-4000-8000-000000000011',
                   'BinarySHA256': 'b' * 64, 'CandidateSHA256': 'c' * 64,
                   'FrozenInventorySHA256': 'f' * 64, 'PresentInventorySHA256': 'e' * 64}
            for obj, name, value in [(v, 'PACKET', packet), (r, 'ROOT', root),
                (r, 'CONTROL', control), (v, 'sealed_gate', sealed), (r, 'docker', docker),
                (r, 'inspect', lambda cid: copy.deepcopy(states[cid])),
                (r, 'profile', lambda *a: dict(copy.deepcopy(cfg), AuditID=str(v.uuid.uuid4()))),
                (p.p1, 'close_known', close), (p.native, 'fingerprints', fingerprint),
                (p.native, 'wait_pg', lambda *a: None), (v.time, 'sleep', lambda *a: None),
                (p.prior.closure, 'source_contract', lambda *a: None),
                (v.subprocess, 'run', process), (sys, 'argv', argv)]:
                stack.enter_context(patch.object(obj, name, value))
            stack.enter_context(patch('subprocess.Popen', side_effect=AssertionError('external process forbidden')))
            stack.enter_context(patch('sys.stderr', io.StringIO()))
            printer = stack.enter_context(patch('builtins.print'))
            expect_failure = fault is not None and not (audit_only and fault == 'terminal')
            if expect_failure:
                with self.assertRaises((ValueError, SystemExit)):
                    v.main()
            else:
                v.main()
            self.assertFalse(any(x['State']['Running'] for x in states.values()))
            if expect_failure and fault != 'second-group-checker':
                self.assertFalse(any(x[0] in ('start', 'create') for x in calls))
                if fault != 'sealed-gate':
                    self.assertNotIn(('sealed-gate',), calls)
                self.assertFalse((output / 'complete.json').exists())
                if fault == 'existing-output':
                    self.assertEqual((output / 'original.txt').read_bytes(), b'preserved')
                if fault == 'terminal-parent-alias':
                    self.assertEqual(r.load(control / 'authorized-history/terminal.json'),
                                     {'status': 'ORIGINAL_FAILED_RETAINED'})
            elif fault == 'second-group-checker':
                self.assertFalse((output / 'complete.json').exists())
                self.assertFalse((output / 'mixed').exists())
                self.assertEqual((output / 'late-hold/checker-stderr.txt').read_bytes(), b'checker-failure')
                self.assertTrue(r.load(output / 'business-races/closure.json')['all_closed'])
                self.assertIsNotNone(r.load(output / 'late-hold/closure.json')['failure'])
            elif audit_only:
                self.assertEqual(calls, [])
                self.assertEqual(json.loads(printer.call_args.args[0])['native_preflight'], 'NOT_RUN')
                self.assertFalse((output / 'complete.json').exists())
            else:
                result = r.load(output / 'complete.json')
                if authorized:
                    self.assertLess(calls.index(('branch-freeze',)),calls.index(('sealed-gate',)))
                self.assertEqual([x['copies'] for x in result['groups']], [7, 6, 4])
                self.assertEqual(result['evidence_audit']['members_verified'], 3)
                self.assertEqual(len(result['evidence_audit']['indexes']), 3)
                self.assertEqual(result['technical_requests'], 3)
                self.assertEqual(len(owners), 3)
                self.assertEqual(len([x for x in calls if x[0] == 'start']), 9)
                for group in groups:
                    self.assertTrue(r.load(output / group['Group'] / 'closure.json')['all_closed'])
            self.assertEqual([x for x in calls if x[0] in ('delete', 'enable', 'send')], [])

    def test_main_real_audit_three_real_collects_and_finally_success(self):
        self.composition()

    def test_authorized_complete_entry_checks_branch_before_cold_gate_and_three_collects(self):
        self.composition(authorized=True)

    def test_authorized_complete_entry_rejects_missing_or_rejected_freeze_without_source_io(self):
        for fault in ('missing-freeze-flag','branch-freeze'):
            with self.subTest(fault=fault):self.composition(fault=fault,authorized=True)

    def test_main_audit_only_never_enters_host_or_native(self):
        self.composition(audit_only=True)

    def test_main_can_only_audit_saved_evidence_of_a_terminal_execution(self):
        self.composition(fault='terminal', audit_only=True)

    def test_unreadable_terminal_is_not_absence(self):
        with tempfile.TemporaryDirectory() as tmp:
            control = Path(tmp)
            target = control / 'authorized-execution01/terminal.json'
            original = Path.stat
            def stat(path, *args, **kwargs):
                if path == target:
                    raise PermissionError('recorded unreadable terminal')
                return original(path, *args, **kwargs)
            with patch.object(r, 'CONTROL', control), patch.object(Path, 'stat', stat):
                with self.assertRaises(PermissionError):
                    v.require_unfinished_execution()

    def test_main_index_binding_format_bytes_cli_terminal_and_host_rejections(self):
        for fault in ('index-format', 'index-bytes', 'plan-binding', 'missing-plan',
                      'terminal', 'terminal-parent-alias', 'sealed-gate', 'existing-output'):
            with self.subTest(fault=fault):
                self.composition(fault=fault)

    def test_main_second_group_failure_closes_and_does_not_collect_third(self):
        self.composition(fault='second-group-checker')


if __name__ == '__main__':
    unittest.main()
