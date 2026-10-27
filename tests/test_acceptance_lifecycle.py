"""Shutdown regressions use synthetic ownership records, never existing resources."""
import copy
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import Mock, patch

sys.path.insert(0, str(Path(__file__).parent))
import acceptance_lifecycle as a


class Docker:
    def __init__(self):
        self.calls, self.items = [], {}
        self.daemon = 'isolated-daemon'
        self.lose_create_response = False
        self.reject_create = False
        self.ignore_stop = False
        self.stop_exit = 0

    def __call__(self, args):
        args = list(map(str, args))
        self.calls.append(args)
        if args == ['docker', 'context', 'show']:
            return 'isolated-context'
        if args[:3] != ['docker', '--context', 'isolated-context']:
            raise AssertionError('Unpinned or unexpected external command')
        command = args[3:]
        if command == ['info', '--format', '{{.ID}}']:
            return self.daemon
        if command[0] == 'create':
            if self.reject_create:
                raise a.CleanupError('CREATE_REJECTED')
            cid = f'{len(self.items) + 1:064x}'
            label = command[command.index('--label') + 1].split('=', 1)[1]
            Path(command[command.index('--cidfile') + 1]).write_text(cid)
            self.items[cid] = {'Id': cid, 'Name': '/owned-fixture', 'Image': 'immutable-image', 'Created': 'immutable-start',
                               'Config': {'Labels': {a.LABEL: label}}, 'HostConfig': {'AutoRemove': False, 'RestartPolicy': {'Name': 'no'}, 'OomKillDisable': False},
                               'Mounts': [{'Type': 'volume', 'Name': 'retained-data', 'Destination': '/data', 'RW': True},
                                          {'Type': 'bind', 'Source': 'retained-evidence', 'Destination': '/evidence', 'RW': False}],
                               'State': {'Running': False, 'Status': 'created', 'ExitCode': 0}}
            if self.lose_create_response:
                raise a.CleanupError('CREATE_RESPONSE_LOST')
            return cid
        if command[0] == 'inspect':
            return json.dumps([self.items[command[1]]])
        if command[0] == 'start':
            self.items[command[1]]['State'] = {'Running': True, 'Status': 'running', 'ExitCode': 0}
            return command[1]
        if command[0] == 'stop' and command[1:3] == ['--timeout', '30']:
            if not self.ignore_stop:
                self.items[command[3]]['State'] = {'Running': False, 'Status': 'exited', 'ExitCode': self.stop_exit}
            return command[3]
        raise AssertionError('Forbidden Docker operation: ' + command[0])


class Processes:
    def __init__(self):
        self.items, self.signals = {}, []

    def __call__(self, pid, *, expected=None, terminate=False):
        value = self.items.get(pid)
        if value is None:
            return None
        a.need(expected is None or value == expected, 'PROCESS_IDENTITY_UNVERIFIED')
        if terminate:
            self.signals.append(pid)
            del self.items[pid]
        return value


class LifecycleTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name)
        self.root, self.private = self.base / 'checkout', self.base / 'private'
        self.root.mkdir()
        def private_directory(path):
            path.mkdir(parents=True, mode=0o700)
            return path
        self.patches = [patch.object(a.AcceptanceRun, 'private_base', return_value=self.private),
                        patch.object(a.helpers, 'private_root', side_effect=private_directory),
                        patch.object(a.subprocess, 'run', side_effect=AssertionError('Unexpected real subprocess'))]
        for p in self.patches:
            p.start()
            self.addCleanup(p.stop)
        self.docker, self.processes = Docker(), Processes()
        self.scope = a.AcceptanceRun(self.root, self.root / '.data/owned', execute=self.docker, process=self.processes)
        self.addCleanup(self.scope.close)

    def prepare(self):
        self.scope.begin()
        self.cid = self.scope.create_container(['--name', 'owned-fixture', 'local-image'])
        for pid, name in [(100, 'identity'), (101, 'api'), (102, 'web')]:
            identity = {'pid': pid, 'created': pid * 100, 'exe': name}
            self.processes.items[pid] = identity
            self.scope.state['processes'].append({'name': name, 'identity': copy.deepcopy(identity)})
        self.scope.save()
        (self.scope.run / 'business-evidence.json').write_text('{"original":true}')
        return self.scope

    def reports(self):
        return [json.loads(p.read_text()) for p in self.scope.run.glob('cleanup-*.json')]

    def test_success_stops_only_owned_resources_and_preserves_evidence(self):
        scope = self.prepare()
        self.processes.items[999] = {'pid': 999, 'created': 1, 'exe': 'other-session'}
        before = copy.deepcopy(self.docker.items[self.cid])
        self.assertEqual(a.finish(scope, lambda: 42), 42)
        self.assertEqual(self.processes.signals, [102, 101, 100])
        self.assertIn(999, self.processes.items)
        after = self.docker.items[self.cid]
        self.assertEqual(after['State']['Status'], 'exited')
        self.assertEqual(after['Mounts'], before['Mounts'])
        self.assertEqual((scope.run / 'business-evidence.json').read_text(), '{"original":true}')
        self.assertEqual(self.reports()[0]['status'], 'PASS')
        self.assertEqual(self.reports()[0]['removals'], 0)

    def test_exception_and_keyboard_interrupt_still_stop_and_do_not_become_pass(self):
        self.prepare()
        def failure():
            raise KeyboardInterrupt
        with self.assertRaises(KeyboardInterrupt):
            a.finish(self.scope, failure)
        self.assertEqual(self.reports()[0]['status'], 'PASS')
        self.assertFalse(self.docker.items[self.cid]['State']['Running'])

    def test_partial_preparation_failure_stops_already_registered_siblings(self):
        self.prepare()
        def failure():
            raise RuntimeError('fixture stage failed')
        with self.assertRaisesRegex(RuntimeError, 'fixture stage failed'):
            a.finish(self.scope, failure)
        self.assertFalse(self.docker.items[self.cid]['State']['Running'])
        self.assertEqual(self.processes.items, {})

    def test_failure_before_begin_does_not_adopt_an_existing_run(self):
        self.scope.run.mkdir(parents=True)
        original = self.scope.run / 'original-evidence'
        original.write_text('keep')
        with self.assertRaisesRegex(a.CleanupError, 'EXISTING_RUN_RETAINED'):
            a.finish(self.scope, self.scope.begin)
        self.assertEqual(self.docker.calls, [])
        self.assertEqual(original.read_text(), 'keep')
        self.assertEqual(self.reports(), [])

    def test_second_session_cannot_reopen_a_live_owner(self):
        self.prepare()
        with self.assertRaisesRegex(a.CleanupError, 'RUN_OWNED_BY_ANOTHER_SESSION'):
            a.AcceptanceRun.reopen(self.root, self.scope.run)
        self.assertEqual(self.processes.signals, [])
        self.assertTrue(self.docker.items[self.cid]['State']['Running'])

    def test_recovery_after_owner_exit_is_idempotent_and_keeps_original_receipts(self):
        self.prepare()
        self.scope.close()
        recovered = a.AcceptanceRun.reopen(self.root, self.scope.run)
        recovered.execute, recovered.process = self.docker, self.processes
        try:
            recovered.stop()
            recovered.stop()
        finally:
            recovered.close()
        self.assertEqual(len(self.reports()), 2)
        self.assertTrue(all(r['status'] == 'PASS' for r in self.reports()))
        self.assertEqual(self.processes.signals, [102, 101, 100])

    def test_closed_scope_cannot_stop_after_another_session_takes_ownership(self):
        self.prepare()
        self.scope.close()
        other = a.AcceptanceRun.reopen(self.root, self.scope.run)
        try:
            before = len(self.docker.calls)
            with self.assertRaisesRegex(a.CleanupError, 'RUN_OWNER_LOCK_REQUIRED'):
                self.scope.stop()
            self.assertEqual(len(self.docker.calls), before)
            self.assertEqual(self.processes.signals, [])
        finally:
            other.close()

    def test_reused_pid_refuses_entire_stop_before_any_signal(self):
        self.prepare()
        self.processes.items[101]['created'] += 1
        with self.assertRaises(a.CleanupError):
            self.scope.stop()
        self.assertEqual(self.processes.signals, [])
        self.assertTrue(self.docker.items[self.cid]['State']['Running'])
        self.assertEqual(self.reports()[0]['status'], 'FAIL')

    def test_foreign_label_refuses_before_any_signal(self):
        self.prepare()
        self.docker.items[self.cid]['Config']['Labels'][a.LABEL] = 'other-run'
        with self.assertRaises(a.CleanupError):
            self.scope.stop()
        self.assertEqual(self.processes.signals, [])
        self.assertTrue(self.docker.items[self.cid]['State']['Running'])

    def test_container_configuration_drift_refuses_before_any_signal(self):
        self.prepare()
        self.docker.items[self.cid]['Mounts'][0]['Name'] = 'another-session-volume'
        with self.assertRaises(a.CleanupError):
            self.scope.stop()
        self.assertEqual(self.processes.signals, [])

    def test_mount_order_does_not_change_ownership(self):
        self.prepare()
        self.docker.items[self.cid]['Mounts'].reverse()
        self.assertEqual(self.scope.stop()['status'], 'PASS')

    def test_docker_start_disabled_oom_default_is_normalized_without_allowing_true(self):
        self.prepare()
        self.docker.items[self.cid]['HostConfig']['OomKillDisable'] = None
        self.assertEqual(self.scope.stop()['status'], 'PASS')
        self.docker.items[self.cid]['HostConfig']['OomKillDisable'] = True
        with self.assertRaises(a.CleanupError):
            self.scope.stop()

    def test_other_boolean_null_type_changes_still_refuse_shutdown(self):
        self.prepare()
        self.docker.items[self.cid]['HostConfig']['AutoRemove'] = None
        with self.assertRaises(a.CleanupError):
            self.scope.stop()

    def test_daemon_drift_refuses_without_falling_back_to_current_context(self):
        self.prepare()
        self.docker.daemon = 'different-daemon'
        with self.assertRaises(a.CleanupError):
            self.scope.stop()
        self.assertEqual(self.processes.signals, [])

    def test_lost_create_response_recovers_only_the_owned_cidfile(self):
        self.scope.begin()
        self.docker.lose_create_response = True
        with self.assertRaises(a.CleanupError):
            self.scope.create_container(['--name', 'owned-fixture', 'local-image'])
        self.assertEqual(self.scope.stop()['status'], 'PASS')
        self.assertFalse(next(iter(self.docker.items.values()))['State']['Running'])
        self.assertFalse(any('start' in command for command in self.docker.calls))

    def test_missing_create_identity_is_unknown_and_never_uses_a_name_scan(self):
        self.scope.begin()
        self.docker.reject_create = True
        with self.assertRaises(a.CleanupError):
            self.scope.create_container(['--name', 'owned-fixture', 'local-image'])
        with self.assertRaises(a.CleanupError):
            self.scope.stop()
        self.assertEqual(self.reports()[0]['status'], 'FAIL')
        self.assertFalse(any('ls' in command or 'rm' in command or 'prune' in command for command in self.docker.calls))

    def test_stop_timeout_is_not_reported_as_success(self):
        self.prepare()
        self.docker.ignore_stop = True
        with self.assertRaises(a.CleanupError):
            self.scope.stop()
        self.assertEqual(self.reports()[0]['status'], 'FAIL')

    def test_unclean_container_exit_is_not_reported_as_success(self):
        self.prepare()
        self.docker.stop_exit = 137
        with self.assertRaises(a.CleanupError):
            self.scope.stop()
        report = self.reports()[0]
        self.assertEqual(report['status'], 'FAIL')
        self.assertEqual(report['reasonCode'], 'CONTAINER_STOP_UNCLEAN')

    def test_missing_legacy_locator_cannot_be_adopted(self):
        self.scope.run.mkdir(parents=True)
        with self.assertRaises(FileNotFoundError):
            a.AcceptanceRun.reopen(self.root, self.scope.run)
        self.assertEqual(self.docker.calls, [])

    def test_forged_parent_escape_in_cidfile_is_rejected(self):
        self.prepare()
        self.scope.state['containers'][0]['cidfile'] = '../foreign.id'
        with self.assertRaises(a.CleanupError):
            self.scope.stop()
        self.assertEqual(self.processes.signals, [])

    def test_failed_new_process_identity_does_not_orphan_the_owned_child(self):
        self.scope.begin()
        child = Mock(pid=321)
        child.poll.return_value = 0
        with patch.object(a.subprocess, 'Popen', return_value=child):
            with self.assertRaisesRegex(a.CleanupError, 'NEW_PROCESS_EXITED'):
                self.scope.spawn('failed-child', ['owned-child'], cwd=self.root, env={})
        child.terminate.assert_called_once_with()
        child.wait.assert_called_once_with(timeout=10)
        self.assertEqual(self.scope.state['processes'], [])


class NativeProcessTests(unittest.TestCase):
    def test_exact_native_handle_stop_keeps_other_process_alive(self):
        owned = subprocess.Popen([sys.executable, '-c', 'import time;time.sleep(60)'],
                                 creationflags=getattr(subprocess, 'CREATE_NO_WINDOW', 0))
        other = subprocess.Popen([sys.executable, '-c', 'import time;time.sleep(60)'],
                                 creationflags=getattr(subprocess, 'CREATE_NO_WINDOW', 0))
        try:
            expected = a.process_identity(owned.pid)
            wrong = {**expected, 'created': expected['created'] + 1}
            with self.assertRaises(a.CleanupError):
                a.process_identity(owned.pid, expected=wrong, terminate=True)
            self.assertIsNone(owned.poll())
            a.process_identity(owned.pid, expected=expected, terminate=True)
            owned.wait(timeout=10)
            self.assertIsNone(other.poll())
            self.assertIsNone(a.process_identity(owned.pid, expected=expected))
        finally:
            for p in (owned, other):
                if p.poll() is None:
                    p.terminate()
                p.wait(timeout=10)


class ManagedEntryTests(unittest.TestCase):
    setUp = LifecycleTests.setUp

    def test_each_stage_failure_enters_the_complete_entry_shutdown(self):
        import types
        entry = a.load_module('_acceptance_entry_test', Path(__file__).parent / 'platform-management/run.py')
        for index, failure in enumerate(('bootstrap', 'test-main-service.py', 'directory-product-contract.py',
                                         'web', 'user-product-browser.mjs', 'verify-main-service.py')):
            with self.subTest(stage=failure):
                scope = a.AcceptanceRun(self.root, self.root / '.data' / f'failure-{index}',
                                        execute=self.docker, process=self.processes)
                def prepare(lifecycle):
                    lifecycle.begin()
                    lifecycle.create_container(['--name', 'owned-fixture', 'local-image'])
                    if failure == 'bootstrap':
                        raise RuntimeError('original-stage-failure')
                def call(args, **kwargs):
                    if Path(args[-1]).name == failure:
                        raise RuntimeError('original-stage-failure')
                def web(build):
                    if failure == 'web':
                        raise RuntimeError('original-stage-failure')
                b = types.SimpleNamespace(main=prepare, call=call, RUN=scope.run)
                with patch.object(entry, 'load_module', return_value=types.SimpleNamespace(main=web)):
                    with self.assertRaisesRegex(RuntimeError, 'original-stage-failure'):
                        a.finish(scope, lambda: entry.execute('user-product', b, scope))
                self.assertEqual(json.loads((scope.run / 'acceptance-result.json').read_text())['status'], 'FAIL')
                reports = [json.loads(p.read_text()) for p in scope.run.glob('cleanup-*.json')]
                self.assertEqual(reports[0]['status'], 'PASS')
                self.assertFalse(any(c['State']['Running'] for c in self.docker.items.values()))


if __name__ == '__main__':
    unittest.main()
