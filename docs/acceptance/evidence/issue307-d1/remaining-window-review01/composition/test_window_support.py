"""Offline host-maintenance fault tests. Never contact Docker or a provider."""
import datetime as dt
import base64
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('window_support', Path(__file__).with_name('window_support.py'))
w = importlib.util.module_from_spec(spec)
spec.loader.exec_module(w)


class WindowSupportTests(unittest.TestCase):
    def test_containment_uses_approved_identity_when_current_packet_is_damaged(self):
        with tempfile.TemporaryDirectory() as name:
            packet = Path(name)
            for item in ('candidate.json', 'bindings.json', 'confirmation-template.txt'):
                (packet / item).write_bytes(b'damaged current preflight file')
            with patch.object(w, 'PACKET', packet):
                with self.assertRaises(ValueError):
                    w.Host()
                host = w.Host(containment=True)
            self.assertEqual(host.candidate['Execution'], w.EXECUTION)
            self.assertEqual(host.candidate['Groups'][0]['Core'],
                             '9bc6e9ec894f9fecb1566940d9b4680e0a9ddee52f31f9aec1f45afdb35c7ab0')
            self.assertEqual(len(host.bindings), 3)

    def test_approved_identity_snapshot_still_requires_exact_hash(self):
        with self.assertRaises(ValueError):
            w.approved_packet('candidate.json', '0' * 64)

    def test_cli_seal_selects_containment_context(self):
        with patch.object(w.sys, 'argv', ['window_support.py', 'seal']):
            with patch.object(w, 'Host') as host, patch('builtins.print'):
                host.return_value.seal.return_value = {'all_steps_confirmed': True}
                w.main()
                host.assert_called_once_with(containment=True)
                host.return_value.seal.assert_called_once()

    def test_disable_attempts_after_archive_failure_without_adopting_old_summary(self):
        for stale, enabler_live, policy_exit, archive_fails in [(True, False, 1, True), (False, False, 0, True),
                (False, True, 0, True), (False, False, 1, True), (True, False, 0, False)]:
            with self.subTest(stale=stale, enabler_live=enabler_live, policy_exit=policy_exit, archive_fails=archive_fails), tempfile.TemporaryDirectory() as name:
                output = Path(name)
                host = w.Host(containment=True)
                group = host.group('late-hold')
                root_id = host.binding(group)['root_owners']['disable-all']['container']
                enablers = {host.binding(group)['root_owners'][a]['container'] for a in ('enable-read', 'enable-send')}
                changes = {'action': 'disable-all', 'unconfirmed': [],
                           'disabled_readback': {'delete754ea8f0': 'disabled', 'inspect754ea8f06ffc': 'disabled'}}
                if stale:
                    w.exclusive(output / 'status-changes.json', changes)
                root = {'Id': root_id, 'State': {'Running': False, 'Pid': 0},
                        'HostConfig': {'RestartPolicy': {'Name': 'no'}}}
                host.root_owner = lambda g, action: (root, output)
                def inspect(ident):
                    return {'State': {'Running': ident == group['Native'] or (enabler_live and ident in enablers),
                                      'Pid': 10 if ident == group['Native'] or (enabler_live and ident in enablers) else 0},
                            'HostConfig': {'RestartPolicy': {'Name': 'no'}}}
                host.inspect = inspect
                commands = []
                def docker(*args, **kwargs):
                    commands.append(args)
                    if args[0] == 'start' and not (output / 'status-changes.json').exists():
                        w.exclusive(output / 'status-changes.json', changes)
                    return str(policy_exit).encode() if args[0] == 'wait' else b''
                host.docker = docker
                rotate = w.rotate
                with patch.object(w, 'rotate', side_effect=PermissionError('archive inaccessible') if archive_fails else rotate):
                    result = host.disable_all(group)
                self.assertEqual(commands, [('start', root_id), ('wait', root_id)])
                self.assertEqual(result['archive']['complete'], not archive_fails)
                rejected = stale and archive_fails
                self.assertEqual(result['stale_summary_rejected'], rejected)
                self.assertEqual(result['identity_disable_confirmed'], not rejected and not enabler_live)
                self.assertEqual(result['last_readback_all_disabled'], not rejected)
                self.assertEqual(result['policy_validation_passed'], policy_exit == 0)

    def test_time_limits(self):
        for at in (w.START - dt.timedelta(microseconds=1), w.END):
            with self.subTest(at=at), self.assertRaises(ValueError):
                w.maintenance_time(at, signing=True)
        w.maintenance_time(w.START, signing=True)
        w.maintenance_time(w.END)  # containment can continue; authority cannot

    def test_rotate_preserves_raw_and_journals(self):
        with tempfile.TemporaryDirectory() as name:
            root = Path(name)
            original = {'result.json': b'{"old":1}\r\n', 'status-changes.json': b'old-disable\n'}
            for n, raw in original.items():
                (root / n).write_bytes(raw)
            (root / 'start-kept.json').write_bytes(b'journal')
            result = w.rotate(root)
            archive = Path(result['archive'])
            for n, raw in original.items():
                self.assertFalse((root / n).exists())
                self.assertEqual((archive / n).read_bytes(), raw)
                self.assertEqual(result['sha256'][n], w.digest(raw))
            self.assertEqual((root / 'start-kept.json').read_bytes(), b'journal')
            self.assertEqual(w.rotate(root), {'rotated': []})

    def test_one_field_revocation_retains_original(self):
        with tempfile.TemporaryDirectory() as name:
            root = Path(name)
            path = root / 'remaining.json'
            original = b'{"Base":{"Enabled":true,"Token":"fixture"},"Group":"late-hold"}\r\n'
            path.write_bytes(original)
            receipt = w.disable_json(path, root / 'archive', case=True, expected=w.digest(original))
            self.assertEqual((root / 'archive/original.private').read_bytes(), original)
            self.assertEqual(w.load(path), {'Base': {'Enabled': False, 'Token': 'fixture'}, 'Group': 'late-hold'})
            self.assertEqual(receipt['disabled_sha256'], w.sha(path))
            self.assertTrue(w.disable_json(path, root / 'unused', case=True)['already_disabled'])
            self.assertFalse((root / 'unused').exists())

    def test_drift_refuses_before_revocation(self):
        with tempfile.TemporaryDirectory() as name:
            root = Path(name)
            path = root / 'remaining.json'
            raw = b'{"Base":{"Enabled":true}}'
            path.write_bytes(raw)
            with self.assertRaises(ValueError):
                w.disable_json(path, root / 'archive', case=True, expected='0' * 64)
            self.assertEqual(path.read_bytes(), raw)
            self.assertFalse((root / 'archive').exists())

    def test_path_escape(self):
        with tempfile.TemporaryDirectory() as name:
            root = Path(name)
            for path in [root, root / '../foreign']:
                with self.assertRaises(ValueError):
                    w.owned(path, root)

    def test_fresh_iam_requires_exit_pid_restart_and_time(self):
        at = w.START + dt.timedelta(minutes=5)
        item = {'State': {'Running': False, 'Pid': 0, 'ExitCode': 0, 'FinishedAt': (at - dt.timedelta(seconds=5)).isoformat()},
                'HostConfig': {'RestartPolicy': {'Name': 'no'}}}
        w.fresh_finished(item, at)
        for key, value in [('Running', True), ('Pid', 1), ('ExitCode', 1),
                           ('FinishedAt', (at - dt.timedelta(seconds=46)).isoformat()),
                           ('FinishedAt', (at + dt.timedelta(seconds=1)).isoformat())]:
            changed = json.loads(json.dumps(item))
            changed['State'][key] = value
            with self.subTest(key=key), self.assertRaises(ValueError):
                w.fresh_finished(changed, at)
        item['HostConfig']['RestartPolicy']['Name'] = 'always'
        with self.assertRaises(ValueError):
            w.fresh_finished(item, at)

    def test_each_fault_still_runs_every_containment_step(self):
        # Local close, every IAM group, sealing and final PID0 checks all run.
        labels = ['local-permission', 'stop-worker', 'business-IAM', 'late-IAM', 'mixed-IAM', 'seal-gateway', 'stop-native', 'stop-Core']
        for failure in labels:
            calls = []
            def action(label):
                calls.append(label)
                if label == failure:
                    raise RuntimeError('PRIVATE SECRET MUST NOT ENTER RECEIPT')
                return {'closed': True}
            result = w.all_steps([(x, lambda label=x: action(label)) for x in labels])
            self.assertEqual(calls, labels)
            self.assertEqual([x['step'] for x in result if not x['ok']], [failure])
            self.assertNotIn('PRIVATE SECRET', json.dumps(result))

    def test_prewindow_mutation_is_zero_actions(self):
        host = object.__new__(w.Host)
        with patch.object(w, 'now', return_value=w.START - dt.timedelta(seconds=1)):
            with patch.object(host, 'root_owner') as root_owner:
                with self.assertRaises(ValueError):
                    host.archive_root({}, 'check-disabled')
                root_owner.assert_not_called()
            with patch.object(host, 'inspect') as inspect:
                for action in [lambda: host.revoke_case({}, 'late'), host.h1_proof, host.seal]:
                    with self.assertRaises(ValueError):
                        action()
                inspect.assert_not_called()

    def test_composed_seal_continues_across_groups_and_local_failure(self):
        for failure in ['permission', 'business-races', 'late-hold', 'mixed', 'gateway',
                        'archive-incomplete', 'identity-unconfirmed']:
            with self.subTest(failure=failure), tempfile.TemporaryDirectory() as name:
                private = Path(name)
                control = private / 'authorized-execution01'
                control.mkdir()
                host = object.__new__(w.Host)
                host.candidate = w.load(w.PACKET / 'candidate.json')
                host.bindings = w.load(w.PACKET / 'bindings.json')
                for group in host.candidate['Groups']:
                    stages = set(group['Helpers']) | {'root-' + a for a in host.binding(group)['root_owners']}
                    for stage in stages:
                        path = private / group['Group'] / stage
                        path.mkdir(parents=True)
                        (path / 'permission.json').write_text('{"Enabled":true}')
                        if stage.startswith('recover-'):
                            (path / 'recovery.json').write_text('{"Enabled":true}')
                    for stage in group['CaseHelpers']:
                        path = private / group['Group'] / ('case-' + stage)
                        path.mkdir(parents=True)
                        (path / 'remaining.json').write_text('{"Base":{"Enabled":true}}')
                stopped, disabled, gateways = [], [], []
                def stop(ident):
                    stopped.append(ident)
                    return {'stopped': True}
                def disable(group):
                    disabled.append(group['Group'])
                    if group['Group'] == failure:
                        raise RuntimeError('IAM unavailable')
                    if failure == 'archive-incomplete':
                        return {'identity_disable_confirmed': True, 'policy_validation_passed': True,
                                'archive': {'complete': False}}
                    if failure == 'identity-unconfirmed':
                        return {'identity_disable_confirmed': False, 'policy_validation_passed': True}
                    return {'identity_disable_confirmed': True, 'policy_validation_passed': True}
                def gateway(group):
                    gateways.append(group['Group'])
                    if failure == 'gateway':
                        raise RuntimeError('mount unavailable')
                original = w.disable_json
                def close(path, *args, **kwargs):
                    if failure == 'permission' and path.parent.name == 'P1':
                        raise OSError('read-only host path')
                    return original(path, *args, **kwargs)
                host.stop, host.disable_all, host.seal_gateway = stop, disable, gateway
                with patch.object(w, 'PRIVATE', private), patch.object(w, 'CONTROL', control), \
                     patch.object(w, 'now', return_value=w.START), patch.object(w, 'disable_json', side_effect=close):
                    result = host.seal()
                self.assertFalse(result['all_steps_confirmed'])
                groups = [g['Group'] for g in host.candidate['Groups']]
                self.assertEqual(disabled, groups)
                self.assertEqual(gateways, groups)
                for group in host.candidate['Groups']:
                    for role in ['Native', 'Core', 'Gateway']:
                        self.assertIn(group[role], stopped)
                    for ident in group['Helpers'].values():
                        self.assertIn(ident, stopped)
                self.assertIn(w.ORIGINAL_REDIS, stopped)
                for path in private.glob('*/*/permission.json'):
                    if failure != 'permission' or path.parent.name != 'P1':
                        self.assertIs(w.load(path)['Enabled'], False)

    def test_h1_signing_requires_actual_closed_config_and_fresh_iam(self):
        from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
        from cryptography.hazmat.primitives.serialization import Encoding, PrivateFormat, PublicFormat, NoEncryption
        for fault in [None, 'owner-running', 'enabled-config', 'stale-IAM', 'IAM-enabled', 'gateway-active']:
            with self.subTest(fault=fault), tempfile.TemporaryDirectory() as name:
                private = Path(name)
                control = private / 'authorized-execution01'
                control.mkdir()
                at = w.START + dt.timedelta(minutes=5)
                host = object.__new__(w.Host)
                host.candidate = json.loads(json.dumps(w.load(w.PACKET / 'candidate.json')))
                host.bindings = w.load(w.PACKET / 'bindings.json')
                group = host.group('late-hold')
                folder = private / 'late-hold'
                states = {}
                def state(ident):
                    return {'Id': ident, 'State': {'Running': False, 'Pid': 0, 'ExitCode': 0,
                            'FinishedAt': (at - dt.timedelta(seconds=5)).isoformat()},
                            'HostConfig': {'RestartPolicy': {'Name': 'no'}}}
                for stage, ident in group['Helpers'].items():
                    path = folder / stage
                    path.mkdir(parents=True)
                    (path / 'permission.json').write_text('{"Enabled":false}')
                    states[ident] = state(ident)
                for action, binding in host.binding(group)['root_owners'].items():
                    path = folder / ('root-' + action)
                    path.mkdir(parents=True)
                    (path / 'permission.json').write_text('{"Enabled":false}')
                    output = folder / 'outputs' / ('root-' + action)
                    output.mkdir(parents=True)
                    item = state(binding['container'])
                    states[item['Id']] = item
                check_id = host.binding(group)['root_owners']['check-disabled']['container']
                check_out = folder / 'outputs/root-check-disabled'
                result = {'group': 'late-hold', 'action': 'check-disabled', 'exact_policy_set_verified': True,
                          'before': {'delete754ea8f0': 'disabled', 'inspect754ea8f06ffc': 'disabled'}}
                if fault == 'IAM-enabled':
                    result['before']['delete754ea8f0'] = 'enabled'
                w.exclusive(check_out / 'result.json', result)
                if fault == 'stale-IAM':
                    states[check_id]['State']['FinishedAt'] = (at - dt.timedelta(seconds=46)).isoformat()
                reader = b'non-executable-offline-fixture'
                (private / 'reader').write_bytes(reader)
                host.candidate['ReaderBinarySHA256'] = w.digest(reader)
                key = Ed25519PrivateKey.generate()
                (private / 'topology-key.private').write_bytes(key.private_bytes(Encoding.Raw, PrivateFormat.Raw, NoEncryption()))
                host.candidate['PublicKey'] = key.public_key().public_bytes(Encoding.Raw, PublicFormat.Raw).hex()
                close = folder / 'case-close-H1'
                close.mkdir()
                w.exclusive(close / 'remaining.json', {'ClosureOwners': {group['CaseHelpers']['late']: w.digest(reader)},
                    'ClosurePublicKey': host.candidate['PublicKey'], 'ClosureCandidateSHA256': 'a' * 64})
                group['CaseConfigSHA256']['close-H1'] = w.sha(close / 'remaining.json')
                owner_path = folder / 'case-late'
                owner_path.mkdir()
                profile = owner_path / 'remaining.json'
                profile.write_text('{"Base":{"Enabled":true,"Token":"fixture"}}')
                group['CaseConfigSHA256']['late'] = w.sha(profile)
                w.disable_json(profile, control / 'revocations/late-hold/late', case=True)
                if fault == 'enabled-config':
                    profile.write_text('{"Base":{"Enabled":true,"Token":"fixture"}}')
                owner = state(group['CaseHelpers']['late'])
                owner['Config'] = {'Entrypoint': ['/run/reader'], 'Cmd': ['-mode', 'remaining-late']}
                owner['HostConfig'].update(NetworkMode='container:' + group['Gateway'], ReadonlyRootfs=True, Privileged=False)
                owner['Mounts'] = [{'Destination': dest, 'Source': str(path), 'RW': rw} for dest, path, rw in
                    [('/run/private', owner_path, False), ('/run/reader', private / 'reader', False),
                     ('/run/output', folder / 'outputs', True), ('/run/repo/industry-packs', w.ROOT / 'industry-packs', False)]]
                if fault == 'owner-running':
                    owner['State'].update(Running=True, Pid=123)
                states[owner['Id']] = owner
                gateway = state(group['Gateway'])
                sealed = private / 'gateway.Caddyfile'
                sealed.write_bytes(b'deny all')
                group['SealedGatewaySHA256'] = w.sha(sealed)
                gateway['Mounts'] = [{'Destination': '/etc/caddy/Caddyfile', 'Source': str(sealed), 'RW': False}]
                if fault == 'gateway-active':
                    sealed.write_bytes(b'active')
                states[gateway['Id']] = gateway
                host.inspect = lambda ident: states[ident]
                host.root_owner = lambda g, action: (states[host.binding(g)['root_owners'][action]['container']], folder / 'outputs' / ('root-' + action))
                with patch.object(w, 'PRIVATE', private), patch.object(w, 'CONTROL', control), patch.object(w, 'now', return_value=at):
                    if fault:
                        with self.assertRaises(ValueError):
                            host.h1_proof()
                        self.assertFalse((close / 'closed-H1.json').exists())
                    else:
                        host.h1_proof()
                        signed = w.load(close / 'closed-H1.json')
                        body = base64.b64decode(signed['Body'])
                        key.public_key().verify(base64.b64decode(signed['Signature']), body)
                        proof = json.loads(body)
                        self.assertEqual(proof['Owners'], {owner['Id']: w.digest(reader)})
                        self.assertEqual(proof['CandidateSHA256'], 'a' * 64)
                        with self.assertRaises(FileExistsError):
                            host.h1_proof()  # one proof, no silent renewal


if __name__ == '__main__':
    unittest.main(verbosity=2)
