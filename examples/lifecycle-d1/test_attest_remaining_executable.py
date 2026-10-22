"""Run the real attester main and real signatures; Docker/SQL are explicit doubles."""
import base64
import copy
import datetime as dt
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
from contextlib import ExitStack, redirect_stdout
from types import SimpleNamespace
import unittest
from unittest.mock import patch

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives.serialization import Encoding, PublicFormat, PrivateFormat, NoEncryption
from docker_host_paths import hostpath, normalize_source

spec = importlib.util.spec_from_file_location('attester_under_test', Path(__file__).with_name('attest-remaining-executable.py'))
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)


def dump(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value), encoding='utf-8')


def desktop(path):
    text = path.resolve().as_posix()
    return '/run/desktop/mnt/host/' + text[0].lower() + text[2:] if os.name == 'nt' else text


class AttesterTests(unittest.TestCase):
    def composition(self, stage='P2', fault=None, desktop_sources=True):
        with tempfile.TemporaryDirectory() as tmp, ExitStack() as stack:
            root = Path(tmp)
            private, public = root / 'private', root / 'packet'
            private.mkdir(); public.mkdir()
            # Fixed recorded bytes for a synthetic contract; never the current runtime packet.
            candidate = copy.deepcopy(m.r.load(m.r.ROOT / 'docs/acceptance/evidence/issue307-d1/remaining-candidate18/candidate.json'))
            execution = 'dd6321d9-d838-42d8-ac32-c921ac1c02c4'
            candidate.update(Execution=execution, Profile='test-attester/nondelete', NotBefore='2026-01-01T00:00:00Z', NotAfter='2026-01-01T00:40:00Z')
            at = dt.datetime(2026, 1, 1, 0, 10, tzinfo=dt.timezone.utc)
            key = Ed25519PrivateKey.generate()
            candidate['PublicKey'] = key.public_key().public_bytes(Encoding.Raw, PublicFormat.Raw).hex()
            (private / 'topology-key.private').write_bytes(key.private_bytes(Encoding.Raw, PrivateFormat.Raw, NoEncryption()))
            binary = private / 'operator'; binary.write_bytes(b'NON_EXECUTABLE_TEST_BYTES_NO_CREDENTIALS')
            candidate['SenderBinarySHA256'] = m.r.sha(binary)
            sources = desktop if desktop_sources else lambda path: str(path.resolve())
            states, budgets, bindings = {}, {}, []
            for group in candidate['Groups']:
                name = group['Group']; folder = private / name
                volume = 'dpp-lifecycle307-' + next(x['Run'][:8] for x in m.r.load(m.f.ROOT / 'apps/platform/internal/lifecycle/pilot/remaining_scope.json') if x['Group'] == name) + '-test-attester-budget'
                budgets[name] = volume
                gateway = folder / 'gateway.Caddyfile'; gateway.parent.mkdir(parents=True); gateway.write_bytes(b'sealed test gateway')
                group['GatewaySHA256'] = m.r.sha(gateway)
                initializer = hashlib.sha256((name + 'initializer').encode()).hexdigest()
                script = folder / 'budget-init.sh'; script.write_bytes(b'# not executed\n')
                bindings.append(dict(group=name, budget_initializer=initializer, budget_initializer_script_sha256=m.r.sha(script)))
                for role, image in [('Core', m.r.PG), ('Native', m.r.MI), ('Gateway', m.r.GW)]:
                    states[group[role]] = dict(Id=group[role], Image=image, State=dict(Running=True, Pid=1),
                        HostConfig=dict(RestartPolicy=dict(Name='no'), PortBindings={}),
                        NetworkSettings=dict(Ports={}, Networks={'isolated': dict(NetworkID=group['Network'])}),
                        Mounts=[dict(Type='bind', Destination='/etc/caddy/Caddyfile', Source=sources(gateway), RW=False)] if role == 'Gateway' else [])
                for helper, cid in group['Helpers'].items():
                    owner = folder / helper; owner.mkdir()
                    cfg = dict(Group=name, DeleteSecret='', WirePrivateKey='')
                    dump(owner / 'operator.json', cfg)
                    group['ConfigSHA256'][helper] = m.r.sha(owner / 'operator.json')
                    dump(owner / 'permission.json', dict(Enabled=False))
                    if helper.startswith('recover-'):
                        dump(owner / 'recovery.json', dict(Enabled=True))
                    states[cid] = dict(Id=cid, Image=m.r.PG, Config=dict(User='10001:10001', Entrypoint=['/run/operator'], Cmd=['-stage', helper], Env=[]),
                        State=dict(Running=False, Pid=0),
                        HostConfig=dict(ReadonlyRootfs=True, Privileged=False, RestartPolicy=dict(Name='no'), AutoRemove=False,
                            CapDrop=['ALL'], SecurityOpt=['no-new-privileges'], PortBindings={}, NetworkMode='container:' + group['Gateway'], Tmpfs={'/tmp': '', '/var/lib/postgresql/data': 'size=1m'}),
                        NetworkSettings=dict(Ports={}), Mounts=[
                            dict(Type='bind', Destination='/run/private', Source=sources(owner), RW=False),
                            dict(Type='bind', Destination='/run/operator', Source=sources(binary), RW=False),
                            dict(Type='bind', Destination='/run/output', Source=sources(folder / 'outputs' / helper), RW=True),
                            dict(Type='volume', Destination='/run/budget', Name=volume, Source='/var/lib/docker/volumes/test', RW=True)])
                states[initializer] = dict(Id=initializer, State=dict(Running=False, Pid=0),
                    Config=dict(User='0:0', Entrypoint=['/bin/sh'], Cmd=['-e', '/run/budget-init.sh']),
                    HostConfig=dict(NetworkMode='none', RestartPolicy=dict(Name='no'), PortBindings={}), NetworkSettings=dict(Ports={}),
                    Mounts=[dict(Type='volume', Destination='/run/budget', Name=volume, RW=True),
                            dict(Type='bind', Destination='/run/budget-init.sh', Source=sources(script), RW=False)])
                states[volume] = dict(Labels={'lifecycle307.remaining': execution})
            group = candidate['Groups'][0]
            selected = private / 'business-races' / stage
            dump(public / 'candidate.json', candidate)
            dump(public / 'bindings.json', bindings)
            dump(public / 'status.json', dict(candidate_sha256=m.r.sha(public / 'candidate.json')))
            template = b'ISOLATED_TEST_CONFIRMATION_NOT_HUMAN_AUTHORITY\n'
            (public / 'confirmation-template.txt').write_bytes(template)
            for g in candidate['Groups']:
                for helper in g['Helpers']:
                    owner = private / g['Group'] / helper
                    (owner / 'candidate.json').write_bytes((public / 'candidate.json').read_bytes())
                    (owner / 'confirmation-template.txt').write_bytes(template)
            permission = dict(Enabled=not stage.startswith('recover-'), Execution=execution, Profile=candidate['Profile'],
                CandidateSHA256=m.r.sha(public / 'candidate.json'), ConfirmationSHA256=m.r.sha(public / 'confirmation-template.txt'),
                BinarySHA256=candidate['SenderBinarySHA256'], NotBefore=candidate['NotBefore'], ExpiresAt=candidate['NotAfter'],
                ConfirmedAt='2026-01-01T00:00:01Z', MaxPerTarget=1, MaxTotal=6)
            dump(selected / 'permission.json', permission)
            (selected / 'human-confirmation.txt').write_bytes(template)
            target = states[group['Helpers'][stage]]
            init = states[bindings[0]['budget_initializer']]
            extras = []
            if fault == 'sealed':
                permission['Enabled'] = False; dump(selected / 'permission.json', permission)
            elif fault == 'foreign-confirmation':
                (selected / 'human-confirmation.txt').write_bytes(b'OTHER')
            elif fault == 'private-root':
                dump(selected / 'root.json', dict(RootSecret='not-a-real-secret'))
            elif fault == 'private-directory':
                (selected / 'other').mkdir()
            elif fault == 'junction':
                body = selected.with_name(stage + '-body'); selected.rename(body)
                if os.name == 'nt':
                    subprocess.run(['cmd', '/c', 'mklink', '/J', str(selected), str(body)], check=True, capture_output=True)
                else:
                    selected.symlink_to(body, target_is_directory=True)
            elif fault == 'mount-type':
                target['Mounts'][0]['Type'] = 'volume'
            elif fault == 'mount-type-bool':
                target['Mounts'][0]['RW'] = 0
            elif fault == 'mount-expanded':
                target['Mounts'].append(dict(Type='bind', Destination='/root', Source=sources(root), RW=False))
            elif fault == 'mount-duplicate':
                target['Mounts'].append(copy.deepcopy(target['Mounts'][0]))
            elif fault == 'mount-foreign':
                target['Mounts'][0]['Source'] = sources(root)
            elif fault == 'mount-bytes':
                target['Mounts'][0]['Source'] = sources(root).encode()
            elif fault == 'initializer-bytes':
                init['Mounts'][1]['Source'] = sources(root).encode()
            elif fault == 'initializer-drift':
                (private / 'business-races/budget-init.sh').write_bytes(b'drift')
            elif fault == 'initializer-expanded':
                init['Mounts'].append(dict(Type='bind', Destination='/root', Source=sources(root), RW=False))
            elif fault == 'initializer-running':
                init['State'].update(Running=True, Pid=10)
            elif fault == 'foreign-budget':
                states[budgets['business-races']]['Labels']['lifecycle307.remaining'] = 'OTHER'
            elif fault == 'budget-expanded':
                extras = ['f' * 64]
            elif fault == 'root-user':
                target['Config']['User'] = '0:0'
            elif fault == 'root-env':
                target['Config']['Env'] = ['MINIO_ROOT_PASSWORD=not-real']
            elif fault == 'ports':
                target['HostConfig']['PortBindings'] = {'9000/tcp': [{}]}
            elif fault == 'source-network':
                states[group['Native']]['NetworkSettings']['Networks']['foreign'] = dict(NetworkID='other')
            seen = []
            def docker(*args):
                seen.append(args)
                if args[:2] == ('network', 'inspect'):
                    return json.dumps([dict(Id=group['Network'], Internal=True)]).encode()
                self.assertEqual(args[0], 'cp')
                blob = io.BytesIO()
                with tarfile.open(fileobj=blob, mode='w') as tar:
                    info = tarfile.TarInfo('budget'); info.type = tarfile.DIRTYPE; info.uid = info.gid = 10001; info.mode = 0o700; tar.addfile(info)
                    if fault == 'budget-nonempty':
                        info = tarfile.TarInfo('budget/foreign.json'); info.size = 2; info.uid = info.gid = 10001; tar.addfile(info, io.BytesIO(b'{}'))
                return blob.getvalue()
            def sql(core, query):
                self.assertIn('lifecycle_copy_absence_receipt', query)
                self.assertNotIn('UPDATE', query.upper())
                return json.dumps([g['ID'] for g in group['Copies'] if g['Key'] == 'fixture/p1.csv']) if core == group['Core'] else '[]'
            for obj, name, val in [(m.f, 'PRIVATE', private), (m.f, 'PUBLIC', public), (m.f, 'EXECUTION', execution),
                    (m.f, 'PROFILE', candidate['Profile']), (m.f, 'GENERATION', 'test-attester'), (m.f, 'START', candidate['NotBefore']),
                    (m.f, 'END', candidate['NotAfter']), (m.r, 'utc', lambda: at), (m.r, 'inspect', lambda cid: copy.deepcopy(states[cid])),
                    (m.r, 'docker', docker), (m.r, 'docker_text', lambda *args: '\n'.join([*group['Helpers'].values(), bindings[0]['budget_initializer'], *extras])),
                    (m.r.native, 'sql', sql)]:
                stack.enter_context(patch.object(obj, name, val))
            stack.enter_context(patch.object(sys, 'argv', ['attest', '--group', 'business-races', '--stage', stage]))
            path = selected / ('topology-' + stage + '.json')
            with redirect_stdout(io.StringIO()):
                if fault:
                    with self.assertRaises((ValueError, TypeError)):
                        m.main()
                    self.assertFalse(path.exists(), 'rejected contract must never emit a signature')
                    if fault in ('sealed', 'foreign-confirmation'):
                        self.assertEqual(seen, [])
                    return
                m.main()
            signed = json.loads(path.read_bytes()); body = base64.b64decode(signed['Body'])
            key.public_key().verify(base64.b64decode(signed['Signature']), body)
            fact = json.loads(body)
            self.assertEqual(fact['Execution'], execution)
            self.assertEqual(fact['Stage'], stage)
            self.assertEqual(fact['Sequence']['status'], 'PASS_SIX_GLOBAL_SEQUENCE')
            self.assertEqual(fact['Sequence']['slot'], 0)
            self.assertTrue(all(args[0] in ('network', 'cp') for args in seen))

    def test_complete_main_signs_native_and_docker_desktop_sources(self):
        for stage in ('wire', 'P2', 'recover-P2'):
            for desktop_sources in (False, True):
                with self.subTest(stage=stage, desktop=desktop_sources):
                    self.composition(stage, desktop_sources=desktop_sources)

    def test_all_counterexamples_reject_before_signature(self):
        for fault in ('sealed', 'foreign-confirmation', 'private-root', 'private-directory', 'junction',
                'mount-type', 'mount-type-bool', 'mount-expanded', 'mount-duplicate', 'mount-foreign', 'mount-bytes',
                'initializer-bytes', 'initializer-drift', 'initializer-expanded', 'initializer-running',
                'foreign-budget', 'budget-expanded', 'budget-nonempty', 'root-user', 'root-env', 'ports', 'source-network'):
            with self.subTest(fault=fault):
                self.composition(fault=fault)

    def test_path_boundary_is_strict_and_reuses_existing_contract(self):
        self.assertEqual(normalize_source('/run/desktop/mnt/host/c/one two/file'), 'C:/one two/file')
        self.assertEqual(normalize_source('/var/lib/docker/test'), '/var/lib/docker/test')
        for path in (None, b'/tmp/file', Path('/tmp/file'), '', 'x\x00y', '/run/desktop/mnt/host/cc/file'):
            with self.subTest(path=path), self.assertRaises(ValueError):
                normalize_source(path)
        with self.assertRaises(ValueError):
            hostpath('relative/file')


if __name__ == '__main__':
    unittest.main()
