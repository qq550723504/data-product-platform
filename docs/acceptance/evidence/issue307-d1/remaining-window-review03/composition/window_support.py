"""Host-only receipt preservation and containment for the frozen v19 owners.

No arm, enable, sender, provider DELETE, cleanup, or resource creation entrypoint.
Run check before the window. Mutating maintenance refuses before NotBefore.
Frozen binaries remain the sole implementation of IAM and business commands.
"""
import argparse
import base64
import copy
import datetime as dt
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import uuid

ROOT = Path(__file__).resolve().parents[6]
PACKET = ROOT / 'docs/acceptance/evidence/issue307-d1/remaining-candidate09'
PACKET_RELATIVE = 'docs/acceptance/evidence/issue307-d1/remaining-candidate09/'
AUTHORIZATION_PUBLIC = ROOT / 'docs/acceptance/evidence/issue307-d1/delete-confirmed-v19-execution01/human-authorization.json'
# Same approved v19 packet, preserved before this host-only preparation slice.
# It supplies exact containment identities only, never send authority.
PACKET_ANCHOR = '72e4ce098203f1c076bb3753c6b11931f734e911'
PRIVATE = Path(r'C:\Users\Henry\.codex\fixture-private\remaining-candidate09')
CONTROL = PRIVATE / 'authorized-execution01'
EXECUTION = '9780707d-55eb-4da8-873f-3a901d7aac3f'
CANDIDATE = '3a4a93c449a99f54925fa2924857b88e34929a4731b8e7bfdc3933d2eb29ef76'
BINDINGS = '985906a224f1e78899402b9d8af3daa93f37767bb73fb3e1be9edde61fec1ab8'
TEMPLATE = 'ffedae552010734f3142f11802c14557405d735a397dbcaeab3456546fd0f573'
ORIGINAL_REDIS = '25a4e218f72527e8f93a1df2eeb7a09833e5028c03d5ee178009bd9c71ee33e6'
START = dt.datetime(2026, 10, 18, 2, tzinfo=dt.timezone.utc)
END = START + dt.timedelta(minutes=40)


def need(ok, message):
    if not ok:
        raise ValueError(message)


def now():
    return dt.datetime.now(dt.timezone.utc)


def load(path):
    return json.loads(path.read_bytes())


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def sha(path):
    return digest(path.read_bytes())


def compact(value):
    return json.dumps(value, ensure_ascii=False, separators=(',', ':')).encode()


def exclusive(path, value):
    with path.open('xb') as stream:
        stream.write(compact(value) + b'\n')


def owned(path, root):
    need(path.resolve().is_relative_to(root.resolve()) and path != root,
         'maintenance path escaped its owned directory')
    need(not path.is_symlink(), 'symlink is not an owned receipt')
    return path


def maintenance_time(at, signing=False):
    need(at >= START, 'window not opened; zero maintenance side effects')
    if signing:
        need(at < END, 'H1 proof cannot extend the fixed window')


def closed(state):
    return (not state['State']['Running'] and state['State']['Pid'] == 0
            and state['HostConfig']['RestartPolicy']['Name'] == 'no')


def fresh_finished(item, at):
    stamp = dt.datetime.fromisoformat(item['State']['FinishedAt'].replace('Z', '+00:00'))
    need(closed(item) and item['State']['ExitCode'] == 0
         and dt.timedelta() <= at - stamp <= dt.timedelta(seconds=45),
         'fresh successful disabled-IAM check is required')


def rotate(output):
    """Preserve the two O_EXCL basenames; keep all journal files in place."""
    names = [n for n in ('result.json', 'status-changes.json') if (output / n).exists()]
    if not names:
        return {'rotated': []}
    history = owned(output / '.history' / str(uuid.uuid4()), output)
    history.mkdir(parents=True, exist_ok=False)
    records = {n: sha(owned(output / n, output)) for n in names}
    exclusive(history / 'intent.json', {'at': now().isoformat(), 'sha256': records})
    for name in names:
        target = owned(history / name, output)
        need(not target.exists(), 'receipt destination already exists')
        (output / name).rename(target)
        need(sha(target) == records[name], 'preserved receipt bytes differ')
    exclusive(history / 'complete.json', {'at': now().isoformat(), 'sha256': records})
    return {'rotated': names, 'archive': str(history), 'sha256': records}


def disable_json(path, archive, case=False, expected=None):
    """Only true->false (or already false); preserve original raw bytes first."""
    raw = path.read_bytes()
    if expected is not None:
        need(digest(raw) == expected, 'frozen case profile differs before revocation')
    before = json.loads(raw)
    after = copy.deepcopy(before)
    target = after['Base'] if case else after
    need(isinstance(target.get('Enabled'), bool), 'Enabled is not explicit boolean')
    if target['Enabled'] is False:
        return {'already_disabled': True, 'sha256': digest(raw)}
    target['Enabled'] = False
    archive.mkdir(parents=True, exist_ok=False)
    with (archive / 'original.private').open('xb') as stream:
        stream.write(raw)
    changed = compact(after) + b'\n'
    path.write_bytes(changed)
    need(load(path) == after, 'disabled config readback differs')
    receipt = {'at': now().isoformat(), 'before_sha256': digest(raw),
               'disabled_sha256': digest(changed), 'field': 'Base.Enabled' if case else 'Enabled',
               'before': True, 'after': False, 'other_fields_equal': True}
    exclusive(archive / 'revocation.json', receipt)
    return receipt


def all_steps(steps):
    """A failure never skips later independent containment steps."""
    results = []
    for name, action in steps:
        try:
            results.append({'step': name, 'ok': True, 'result': action()})
        except Exception as error:
            # Command stdout/stderr and private configs never enter public results.
            results.append({'step': name, 'ok': False, 'error_type': type(error).__name__})
    return results


def approved_packet(name, expected):
    result = subprocess.run(['git', 'show', PACKET_ANCHOR + ':' + PACKET_RELATIVE + name],
                            cwd=ROOT, capture_output=True, timeout=10)
    need(result.returncode == 0 and digest(result.stdout) == expected,
         'approved containment identity snapshot unavailable or different')
    return json.loads(result.stdout)


class Host:
    def __init__(self, containment=False):
        if containment:
            # A failed current-packet preflight must not gate known-owner stops.
            self.candidate = approved_packet('candidate.json', CANDIDATE)
            self.bindings = approved_packet('bindings.json', BINDINGS)
        else:
            need(sha(PACKET / 'candidate.json') == CANDIDATE, 'candidate changed')
            need(sha(PACKET / 'bindings.json') == BINDINGS, 'bindings changed')
            need(sha(PACKET / 'confirmation-template.txt') == TEMPLATE, 'template changed')
            self.candidate = load(PACKET / 'candidate.json')
            self.bindings = load(PACKET / 'bindings.json')

    def group(self, name):
        return next(g for g in self.candidate['Groups'] if g['Group'] == name)

    def binding(self, group):
        return next(b for b in self.bindings if b['group'] == group['Group'])

    def docker(self, *args, timeout=45):
        result = subprocess.run(['docker', *args], capture_output=True, timeout=timeout)
        if result.returncode:
            logs = CONTROL / 'diagnostics'
            logs.mkdir(exist_ok=True)
            (logs / (str(uuid.uuid4()) + '.private')).write_bytes(result.stdout + b'\n' + result.stderr)
            raise RuntimeError('Docker command failed; private diagnostic retained')
        return result.stdout

    def inspect(self, ident):
        value = json.loads(self.docker('inspect', ident))[0]
        need(value['Id'] == ident, 'full CID differs')
        return value

    def root_owner(self, group, action):
        binding = self.binding(group)['root_owners'][action]
        folder = PRIVATE / group['Group'] / ('root-' + action)
        need(sha(folder / 'root.json') == binding['root_config_sha256'], 'root config drift')
        need(sha(PRIVATE / 'owner') == self.candidate['OwnerBinarySHA256'], 'owner binary drift')
        item = self.inspect(binding['container'])
        host = item['HostConfig']
        need(host['ReadonlyRootfs'] and not host['Privileged'] and not host.get('AutoRemove')
             and host['RestartPolicy']['Name'] == 'no' and 'ALL' in host['CapDrop']
             and any('no-new-privileges' in x for x in host['SecurityOpt'])
             and not host.get('PortBindings') and not any(item['NetworkSettings']['Ports'].values()),
             'root owner host contract drift')
        need(host['NetworkMode'] == 'container:' + group['Native'], 'root namespace drift')
        need(item['Config']['Entrypoint'] == ['/run/owner']
             and item['Config']['Cmd'] == ['-action', action], 'root command drift')
        mounts = {m['Destination']: m for m in item['Mounts']}
        output = PRIVATE / group['Group'] / 'outputs' / ('root-' + action)
        expected = {'/run/private': (folder, False), '/run/owner': (PRIVATE / 'owner', False),
                    '/run/output': (output, True)}
        need(set(mounts) == set(expected), 'root mount membership drift')
        for key, (path, writable) in expected.items():
            need(Path(mounts[key]['Source']).resolve() == path.resolve()
                 and mounts[key]['RW'] is writable, 'root mount path/mode drift')
        return item, output

    def check(self):
        need(approved_packet('candidate.json', CANDIDATE) == self.candidate
             and approved_packet('bindings.json', BINDINGS) == self.bindings,
             'approved containment snapshot differs from the active candidate')
        auth = load(AUTHORIZATION_PUBLIC)
        need(auth['execution'] == EXECUTION and auth['candidate_sha256'] == CANDIDATE
             and auth['bindings_sha256'] == BINDINGS and auth['referenced_template_sha256'] == TEMPLATE
             and auth['old_authorization_reused'] is False and auth['user_retyped_template'] is False,
             'new by-reference authorization binding differs')
        need(auth['not_before'] == self.candidate['NotBefore'] and auth['not_after'] == self.candidate['NotAfter']
             and auth['max_per_target'] == 1 and auth['max_total'] == 7 and auth['survivors'] == 11
             and auth['unknown_resend'] is False
             and all(auth[k] == 0 for k in ('cleanup', 'hold_release', 'bypass'))
             and auth['source'] == self.candidate['Source']
             and auth['source_freeze_sha256'] == self.candidate['SourceFreezeSHA256']
             and auth['reader_binary_sha256'] == self.candidate['ReaderBinarySHA256']
             and auth['sender_wire_recovery_binary_sha256'] == self.candidate['SenderBinarySHA256']
             and auth['owner_binary_sha256'] == self.candidate['OwnerBinarySHA256']
             and auth['target_digest'] == self.candidate['TargetDigest'], 'authorization scope/window changed')
        need(sha(CONTROL / 'user-message-verbatim.txt') == auth['user_message_sha256']
             and (CONTROL / 'user-message-verbatim.txt').read_text(encoding='utf-8') == auth['user_message_verbatim']
             and sha(CONTROL / 'authorization-envelope.txt') == auth['authorization_envelope_sha256']
             and (CONTROL / 'approved-referenced-template.txt').read_bytes()
             == (PACKET / 'confirmation-template.txt').read_bytes(), 'authorization private receipt differs')
        permissions, inspected = [], []
        for group in self.candidate['Groups']:
            folder = PRIVATE / group['Group']
            need(sha(PRIVATE / 'reader') == self.candidate['ReaderBinarySHA256']
                 and sha(PRIVATE / 'operator') == self.candidate['SenderBinarySHA256'], 'binary changed')
            for stage, expected in group['ConfigSHA256'].items():
                need(sha(folder / stage / 'operator.json') == expected, 'operator config changed')
                permissions.append(folder / stage / 'permission.json')
            for stage, expected in group['CaseConfigSHA256'].items():
                need(sha(folder / ('case-' + stage) / 'remaining.json') == expected, 'case config changed')
            for action in self.binding(group)['root_owners']:
                item, _ = self.root_owner(group, action)
                need(closed(item), 'root helper is active')
                inspected.append(item['Id'])
                permissions.append(folder / ('root-' + action) / 'permission.json')
            for ident in ([group[x] for x in ('Core', 'Native', 'Gateway', 'CheckpointCore', 'CheckpointNative')]
                          + list(group['Helpers'].values()) + list(group['CaseHelpers'].values())
                          + [self.binding(group)['recovery_redis']]):
                need(closed(self.inspect(ident)), 'owner/resource is active')
                inspected.append(ident)
            gateway = self.inspect(group['Gateway'])
            mounted = next(m for m in gateway['Mounts'] if m['Destination'] == '/etc/caddy/Caddyfile')
            need(not mounted['RW'] and sha(Path(mounted['Source'])) == group['SealedGatewaySHA256'],
                 'gateway is not sealed')
        need(closed(self.inspect(ORIGINAL_REDIS)), 'business Redis is active')
        for path in permissions:
            need(load(path)['Enabled'] is False, 'permission enabled before window')
            need(not (path.parent / 'human-confirmation.txt').exists(), 'worker authority prematurely materialized')
        return {'status': 'PASS_PREWINDOW_CLOSED', 'at': now().isoformat(), 'execution': EXECUTION,
                'permissions_disabled': len(permissions), 'containers_stopped': len(set(inspected)) + 1,
                'containment_anchor': PACKET_ANCHOR, 'containment_snapshot_available': True,
                'native_delete': 0, 'provider_io': 0, 'container_starts': 0}

    def archive_root(self, group, action):
        maintenance_time(now())
        item, output = self.root_owner(group, action)
        need(closed(item), 'cannot rotate an active root helper output')
        return rotate(output)

    def revoke_case(self, group, stage):
        maintenance_time(now())
        item = self.inspect(group['CaseHelpers'][stage])
        need(closed(item) and item['State']['ExitCode'] == 0, 'case has not successfully closed')
        path = PRIVATE / group['Group'] / ('case-' + stage) / 'remaining.json'
        return disable_json(path, CONTROL / 'revocations' / group['Group'] / stage,
                            case=True, expected=group['CaseConfigSHA256'][stage])

    def h1_proof(self):
        at = now()
        maintenance_time(at, signing=True)
        group = self.group('late-hold')
        folder = PRIVATE / 'late-hold'
        config = load(folder / 'case-close-H1/remaining.json')
        need(sha(folder / 'case-close-H1/remaining.json') == group['CaseConfigSHA256']['close-H1'],
             'frozen close-H1 profile drift')
        need(config['ClosureOwners'] == {group['CaseHelpers']['late']: self.candidate['ReaderBinarySHA256']}
             and config['ClosurePublicKey'] == self.candidate['PublicKey'], 'H1 closure owner/key drift')
        owner = self.inspect(group['CaseHelpers']['late'])
        need(closed(owner) and owner['State']['ExitCode'] == 0, 'H1 owner did not close successfully')
        need(owner['Config']['Entrypoint'] == ['/run/reader']
             and owner['Config']['Cmd'] == ['-mode', 'remaining-late']
             and owner['HostConfig']['NetworkMode'] == 'container:' + group['Gateway']
             and owner['HostConfig']['ReadonlyRootfs'] and not owner['HostConfig']['Privileged'],
             'H1 owner contract drift')
        mounts = {m['Destination']: m for m in owner['Mounts']}
        need(set(mounts) == {'/run/private', '/run/reader', '/run/output', '/run/repo/industry-packs'},
             'H1 owner mount membership drift')
        for target, path, writable in [('/run/private', folder / 'case-late', False),
            ('/run/reader', PRIVATE / 'reader', False), ('/run/output', folder / 'outputs', True),
            ('/run/repo/industry-packs', ROOT / 'industry-packs', False)]:
            need(Path(mounts[target]['Source']).resolve() == path.resolve()
                 and mounts[target]['RW'] is writable, 'H1 owner mount drift')
        revocation = CONTROL / 'revocations/late-hold/late'
        proof = load(revocation / 'revocation.json')
        raw = (revocation / 'original.private').read_bytes()
        need(digest(raw) == proof['before_sha256'] == group['CaseConfigSHA256']['late'],
             'H1 owner original bytes not preserved')
        disabled = json.loads(raw)
        disabled['Base']['Enabled'] = False
        need(load(folder / 'case-late/remaining.json') == disabled
             and sha(folder / 'case-late/remaining.json') == proof['disabled_sha256'],
             'H1 owner was not disabled by the one-field revocation')
        need(sha(PRIVATE / 'reader') == self.candidate['ReaderBinarySHA256'], 'H1 owner binary drift')
        for stage in group['Helpers']:
            need(load(folder / stage / 'permission.json')['Enabled'] is False
                 and closed(self.inspect(group['Helpers'][stage])), 'late operator is not closed')
        for action in self.binding(group)['root_owners']:
            item, _ = self.root_owner(group, action)
            need(closed(item) and load(folder / ('root-' + action) / 'permission.json')['Enabled'] is False,
                 'root owner is active or enabled during H1 proof')
        root, output = self.root_owner(group, 'check-disabled')
        fresh_finished(root, at)
        result = load(output / 'result.json')
        need(result['action'] == 'check-disabled' and result['group'] == 'late-hold'
             and result['exact_policy_set_verified'] is True
             and result['before'] == {'delete754ea8f0': 'disabled', 'inspect754ea8f06ffc': 'disabled'},
             'H1 disabled-IAM receipt differs')
        gateway = self.inspect(group['Gateway'])
        need(closed(gateway), 'H1 gateway must be stopped during host proof')
        mount = next(m for m in gateway['Mounts'] if m['Destination'] == '/etc/caddy/Caddyfile')
        need(not mount['RW'] and sha(Path(mount['Source'])) == group['SealedGatewaySHA256'],
             'H1 actual gateway seal differs')
        # Maintained OSS signing; host key is never mounted in a worker.
        from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
        from cryptography.hazmat.primitives.serialization import Encoding, PublicFormat
        key = Ed25519PrivateKey.from_private_bytes((PRIVATE / 'topology-key.private').read_bytes())
        need(key.public_key().public_bytes(Encoding.Raw, PublicFormat.Raw).hex() == self.candidate['PublicKey'],
             'H1 host key differs')
        body = {'Execution': EXECUTION, 'Nonce': str(uuid.uuid4()), 'Group': 'late-hold',
                'CandidateSHA256': config['ClosureCandidateSHA256'], 'Owners': config['ClosureOwners'],
                'At': at.isoformat(), 'StoppedPIDZero': True, 'NoRestart': True,
                'ConfigDisabled': True, 'DeleteIdentityDisabled': True, 'GatewaySealed': True}
        encoded = compact(body)
        signed = {'Body': base64.b64encode(encoded).decode(),
                  'Signature': base64.b64encode(key.sign(encoded)).decode()}
        exclusive(folder / 'case-close-H1/closed-H1.json', signed)
        receipt = {'at': at.isoformat(), 'execution': EXECUTION,
                   'proof_sha256': sha(folder / 'case-close-H1/closed-H1.json'),
                   'root_check_finished_at': root['State']['FinishedAt'],
                   'root_result_sha256': sha(output / 'result.json'),
                   'revocation_sha256': sha(revocation / 'revocation.json'), 'native_delete': 0}
        exclusive(CONTROL / 'H1-proof.json', receipt)
        return receipt

    def stop(self, ident):
        item = self.inspect(ident)
        if item['State']['Running']:
            self.docker('stop', '-t', '2', ident, timeout=15)
        need(closed(self.inspect(ident)), 'PID0/no-restart closure unconfirmed')
        return {'id': ident, 'stopped_pid_zero': True}

    def disable_all(self, group):
        native = self.inspect(group['Native'])
        need(native['State']['Running'], 'native stopped: IAM status NOT freshly confirmed')
        item, output = self.root_owner(group, 'disable-all')
        need(closed(item), 'disable helper already running')
        archive = {'complete': True}
        try:
            archive['receipt'] = rotate(output)
        except Exception as error:
            # The frozen helper disables before its O_EXCL summary write.
            # Still attempt containment; never adopt a pre-existing summary.
            archive = {'complete': False, 'error_type': type(error).__name__}
        try:
            stale_summary = (output / 'status-changes.json').exists()
        except OSError:
            stale_summary = True  # Unreadable evidence is never fresh evidence.
        self.docker('start', item['Id'])  # the fixed disable-all command only
        code = int(self.docker('wait', item['Id'], timeout=40).strip())
        prefix = self.binding(group)['project'].removeprefix('dpp-lifecycle307-')
        known = {'delete' + prefix, 'inspect' + {'business-races': '808a69606c36', 'late-hold': '754ea8f06ffc', 'mixed': 'dfda43b44419'}[group['Group']]}
        if group['Group'] == 'business-races':
            known |= {'remainread808a6960', 'remainput808a6960'}
        readback = False
        if not stale_summary:
            try:
                changes = load(output / 'status-changes.json')
                readback = (changes['action'] == 'disable-all' and not changes['unconfirmed']
                    and set(changes['disabled_readback']) == known
                    and set(changes['disabled_readback'].values()) == {'disabled'}
                    and closed(self.inspect(item['Id'])))
            except Exception:
                pass  # Missing/invalid fresh evidence remains unconfirmed.
        enablers = all_steps([('enabler:' + action,
            lambda a=action: closed(self.inspect(self.binding(group)['root_owners'][a]['container'])))
            for action in ('enable-read', 'enable-send')])
        enablers_closed = all(x['ok'] and x['result'] is True for x in enablers)
        return {'identity_disable_confirmed': readback and enablers_closed,
                'last_readback_all_disabled': readback, 'enablers_closed': enablers_closed,
                'enablers': enablers, 'archive': archive, 'stale_summary_rejected': stale_summary,
                'policy_validation_passed': code == 0, 'helper_exit': code,
                'status_changes_sha256': sha(output / 'status-changes.json') if readback else None}

    def seal_gateway(self, group):
        need(closed(self.inspect(group['Gateway'])), 'stop gateway before seal bytes')
        gateway = self.inspect(group['Gateway'])
        mount = next(m for m in gateway['Mounts'] if m['Destination'] == '/etc/caddy/Caddyfile')
        path = Path(mount['Source'])
        # Exact original fixture config directory; do not follow an arbitrary mount.
        fixture = PRIVATE.parent / self.binding(group)['project']
        owned(path, fixture)
        golden = PRIVATE / group['Group'] / 'gateway-sealed.Caddyfile'
        need(sha(golden) == group['SealedGatewaySHA256'], 'sealed golden drift')
        raw = path.read_bytes()
        archive = CONTROL / 'gateway-history'
        archive.mkdir(exist_ok=True)
        with (archive / (group['Group'] + '-' + str(uuid.uuid4()) + '.private')).open('xb') as stream:
            stream.write(raw)
        path.write_bytes(golden.read_bytes())
        need(sha(path) == group['SealedGatewaySHA256'], 'gateway seal readback differs')
        return {'gateway_stopped_and_sealed': True}

    def seal(self):
        maintenance_time(now())
        steps = []
        event = str(uuid.uuid4())
        for group in self.candidate['Groups']:
            folder = PRIVATE / group['Group']
            stages = set(group['Helpers']) | {'root-' + a for a in self.binding(group)['root_owners']}
            for stage in sorted(stages):
                path = folder / stage / 'permission.json'
                steps.append(('permission:' + group['Group'] + ':' + path.parent.name,
                              lambda p=path: disable_json(p, CONTROL / 'seal-history' / event / p.parent.parent.name / p.parent.name)))
            for stage in group['Helpers']:
                if stage.startswith('recover-'):
                    path = folder / stage / 'recovery.json'
                    steps.append(('recovery-revoke:' + group['Group'] + ':' + stage,
                                  lambda p=path: disable_json(p, CONTROL / 'seal-history' / event / p.parent.parent.name / ('recovery-' + p.parent.name))))
            for stage in group['CaseHelpers']:
                path = folder / ('case-' + stage) / 'remaining.json'
                steps.append(('case-revoke:' + group['Group'] + ':' + stage,
                              lambda p=path: disable_json(p, CONTROL / 'seal-history' / event / p.parent.parent.name / p.parent.name, case=True)))
        for group in self.candidate['Groups']:
            for stage, ident in {**group['Helpers'], **{'case-' + k: v for k, v in group['CaseHelpers'].items()}}.items():
                steps.append(('stop-operator:' + group['Group'] + ':' + stage, lambda i=ident: self.stop(i)))
            # Stop enabling root owners before the disabling bootstrap (no race).
            for action in ('enable-read', 'enable-send'):
                ident = self.binding(group)['root_owners'][action]['container']
                steps.append(('stop-enable:' + group['Group'] + ':' + action, lambda i=ident: self.stop(i)))
        # Close every group's producers before any IAM or filesystem validation.
        for group in self.candidate['Groups']:
            steps.append(('disable-all:' + group['Group'], lambda g=group: self.disable_all(g)))
        for group in self.candidate['Groups']:
            steps.append(('stop-gateway:' + group['Group'], lambda g=group: self.stop(g['Gateway'])))
            steps.append(('seal-gateway:' + group['Group'], lambda g=group: self.seal_gateway(g)))
            for ident in ([group['Native'], group['Core'], self.binding(group)['recovery_redis']]
                          + [o['container'] for o in self.binding(group)['root_owners'].values()]):
                steps.append(('stop-final:' + ident, lambda i=ident: self.stop(i)))
        steps.append(('stop-business-redis', lambda: self.stop(ORIGINAL_REDIS)))
        result = {'at': now().isoformat(), 'execution': EXECUTION, 'steps': all_steps(steps),
                  'mode': 'CONTAINMENT_ONLY', 'new_delete': 0, 'cleanup': 0, 'hold_release': 0, 'bypass': 0}
        result['all_steps_confirmed'] = all(x['ok'] and not (isinstance(x.get('result'), dict)
            and (x['result'].get('policy_validation_passed') is False
                 or x['result'].get('identity_disable_confirmed') is False
                 or x['result'].get('archive', {}).get('complete') is False)) for x in result['steps'])
        exclusive(CONTROL / ('seal-' + event + '.json'), result)
        return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('mode', choices=['check', 'archive-root', 'revoke-case', 'h1-proof', 'seal'])
    parser.add_argument('--group', choices=['business-races', 'late-hold', 'mixed'])
    parser.add_argument('--action', choices=['check-disabled', 'enable-read', 'enable-send', 'disable-send', 'disable-all'])
    parser.add_argument('--stage')
    args = parser.parse_args()
    host = Host(containment=args.mode == 'seal')
    if args.mode == 'check':
        result = host.check()
    elif args.mode == 'seal':
        result = host.seal()
    elif args.mode == 'h1-proof':
        result = host.h1_proof()
    else:
        need(args.group, 'bound group is required')
        group = host.group(args.group)
        if args.mode == 'archive-root':
            need(args.action, 'bound root action required; this mode never starts it')
            result = host.archive_root(group, args.action)
        else:
            need(args.stage in group['CaseHelpers'], 'unbound case stage')
            result = host.revoke_case(group, args.stage)
    print(json.dumps(result, ensure_ascii=False))
    if args.mode == 'seal' and not result['all_steps_confirmed']:
        sys.exit(1)


if __name__ == '__main__':
    main()
