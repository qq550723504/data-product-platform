"""Offline whole-entry composition. All Docker/Core/provider calls are modeled."""
import base64
from collections import Counter
from contextlib import ExitStack
import copy
import datetime as dt
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('old_closure', Path(__file__).with_name('close-old-business-dispatches.py'))
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)


def save(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value), encoding='utf-8')


class ClosureTests(unittest.TestCase):
    def composition(self, fault=None):
        with tempfile.TemporaryDirectory() as tmp, ExitStack() as stack:
            private, public = Path(tmp) / 'private', Path(tmp) / 'public'
            private.mkdir(); public.mkdir()
            stack.enter_context(patch.object(m, 'CONTROL', private))
            stack.enter_context(patch.object(m, 'PUBLIC', public))
            from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
            from cryptography.hazmat.primitives import serialization
            key = Ed25519PrivateKey.generate()
            (private / 'host-key.private').write_bytes(key.private_bytes(serialization.Encoding.Raw, serialization.PrivateFormat.Raw, serialization.NoEncryption()))
            initial = json.loads((m.ROOT / 'docs/acceptance/evidence/issue307-d1/post-v22-dispatch-audit' / m.AUDIT / 'actual-Core-rows.json').read_bytes())
            final = copy.deepcopy(initial)
            for row in final:
                row.update(observations=[{'certainty': 'NOT_SENT', 'outcome': 'NOT_SENT', 'invoked_at': None}], releases=[{}], revocations=[{}], unresolved=0)
            targets = m.original_targets()
            old = {'owner_ids': ['old', 'Core']}
            owners = {name: {'cid': name, 'output': str(private / ('output-' + name))} for name in ('observer', 'check-disabled', 'disable-all')}
            manifest = {'audit_id': 'new-audit', 'source': {'Core': 'Core', 'Native': 'Native', 'Network': 'net'}, 'old_retained': old,'prior_failed_preparation':{},
                        'owners': owners, 'targets': targets, 'activation_fields': ['Enabled', 'NotBefore', 'ExpiresAt', 'EvidenceSHA256']}
            save(private / 'manifest.json', manifest)
            cfg = {'Enabled': False, 'MaxProviderDeletes': 0, 'NotBefore': '0001-01-01T00:00:00Z', 'ExpiresAt': '0001-01-01T00:00:00Z', 'EvidenceSHA256': '0' * 64, 'Owners': {'old': 'd' * 64}}
            save(private / 'closure-disabled-original.json', cfg); save(private / 'worker-private/closure.json', cfg)
            review = public / 'review.json'
            save(review, {'status': 'PASS_BOUNDED_INDEPENDENT_OLD_DISPATCH_CLOSURE_MANIFEST_REVIEW', 'remaining_blockers': [], 'manifest_sha256': m.sha(private / 'manifest.json')})
            increments = {'lifecycle_approval_revocation': 3, 'lifecycle_provider_observation': 3, 'lifecycle_dispatch_release': 3,
                          'command_idempotency': 6, 'audit_event': 9, 'evidence': 9, 'evidence_relation': 9, 'outbox_event': 9}
            before = {'roles': 'same', 'relations': [{'relation': n, 'rows': 10, 'sha': 'old'} for n in sorted(m.ALLOWED)]}
            after = copy.deepcopy(before)
            for item in after['relations']:
                item['rows'] += increments.get(item['relation'], 0); item['sha'] = 'new'
            running, commands, phases = set(), [], []
            observed = [False]

            def docker(*args):
                commands.append(args)
                if args[0] == 'network': return json.dumps([{'Internal': fault != 'source-network', 'Containers': {}}]).encode()
                if args[0] == 'start': running.add(args[1])
                if args[0] == 'stop':
                    if fault == 'one-stop-error' and args[-1] == 'check-disabled': raise ValueError('modeled stop failure')
                    running.discard(args[-1])
                return b''

            def inspect(cid):
                if fault == 'Native-inspect-error' and cid == 'Native' and cid in running: raise ValueError('modeled Native inspect error')
                return {'Id':cid, 'Image':m.p1.MI if cid == 'Native' else m.p1.PG, 'State': {'Running': cid in running}, 'NetworkSettings':{'Networks':{'net':{'NetworkID':'net'}}}}

            def stage(manifest, kind):
                phases.append(kind)
                if kind == 'check-disabled':
                    save(public / 'check-disabled-owner.json', {'finished_at': m.now().isoformat()})
                if kind == 'observer':
                    observed[0] = True
                    result = {'completed': fault != 'partial-observer', 'cases': [{'NOT_SENT': {}} for _ in range(3 if fault != 'partial-observer' else 1)]}
                    save(private / 'output-observer/result.json', result)
                    if fault == 'partial-observer': raise ValueError('modeled partial transaction failure')
                if kind == 'disable-all' and fault == 'disable-all-error': raise ValueError('modeled disable error')
                return {}

            def iam(owner, action='check-disabled'):
                if action == 'check-disabled' and fault in ('IAM-error', 'disable-all-error', 'Native-inspect-error'):
                    raise ValueError('modeled fresh IAM failure')
                return {'status': 'MODELED_ONLY'}

            native = SimpleNamespace(wait_pg=lambda *a: None,
                                     fingerprints=lambda *a: copy.deepcopy(after if observed[0] else before),
                                     sql=lambda *a: '\n'.join(json.dumps(x) for x in (final if observed[0] else initial)))
            bindings = [(m, 'frozen', lambda *a: m.need(fault != 'pre-gate', 'modeled freeze drift')),
                        (m,'prior_preparation_closed',lambda: m.need(fault != 'prior-drift','modeled prior drift') or {}),
                        (m.audit, 'retained', lambda *a: old), (m, 'own_contract', lambda *a: inspect(a[0]['cid'])),
                        (m, 'closed', lambda x: not x['State']['Running']), (m, 'inspect', inspect), (m, 'docker', docker),
                        (m.p1, 'native_module', lambda: native), (m.p1, 'append_anchors', lambda *a: {'old': Counter({'hash': 1})}),
                        (m, 'run_owner', stage), (m, 'iam_receipt', iam), (m, 'active_old_state', lambda *a: {'MODELED_ONLY': True}),
                        (m, 'wire_receipt', lambda: {'MODELED_ONLY': True})]
            for target, name, value in bindings: stack.enter_context(patch.object(target, name, value))
            if fault in ('pre-gate', 'source-network','prior-drift'):
                with self.assertRaises(ValueError): m.run(review, m.sha(review))
                self.assertFalse(any(x[0] == 'start' for x in commands)); return
            passed = m.run(review, m.sha(review))
            terminal = m.load(public / 'terminal.json')
            self.assertEqual(terminal, m.load(private / 'terminal.json'))
            self.assertFalse(m.load(private / 'worker-private/closure.json')['Enabled'])
            self.assertEqual(terminal['native_delete'], 0)
            self.assertEqual(terminal['budgets_refunded'], 0)
            self.assertEqual(terminal['pins_released'], 0)
            self.assertFalse(running)
            self.assertEqual({c[-1] for c in commands if c[0] == 'stop'}, set(owners) | {'Core', 'Native'})
            self.assertNotIn('enable', phases)
            if fault is None:
                self.assertTrue(passed); self.assertEqual(phases, ['check-disabled', 'observer'])
                proof = m.load(private / 'worker-private/proof.json')
                body = base64.b64decode(proof['Body']); key.public_key().verify(base64.b64decode(proof['Signature']), body)
                self.assertEqual(json.loads(body)['Targets'], targets)
                self.assertEqual(json.loads(body)['Owners'], cfg['Owners'])
                self.assertTrue(terminal['final_Core_delta']['old_rows_preserved'])
            elif fault != 'one-stop-error':
                self.assertFalse(passed)
            if fault == 'one-stop-error':
                self.assertTrue(passed)  # failed stop command, but fresh PID0 is authoritative
                self.assertTrue(next(x for x in terminal['closure']['owners'] if x['cid'] == 'check-disabled')['stop_error'])
            if fault == 'partial-observer':
                self.assertFalse(m.load(public / 'final-observer_result.json')['completed'])
                self.assertEqual(len(m.load(public / 'final-observer_result.json')['cases']), 1)
                self.assertEqual(terminal['final_Core_rows']['status'], 'OBSERVED')
                self.assertEqual(terminal['final_Core_fingerprint']['status'], 'OBSERVED')
            if fault in ('IAM-error', 'disable-all-error', 'Native-inspect-error'):
                self.assertIn('disable-all', phases)
                self.assertNotIn('observer', phases)
                self.assertEqual(terminal['closure']['identity'], 'UNCONFIRMED' if fault == 'disable-all-error' else 'FRESH_CONFIRMED_DISABLED')

    def test_whole_entry_success(self): self.composition()
    def test_whole_entry_pre_gate_has_no_I_O(self): self.composition('pre-gate')
    def test_whole_entry_source_network_expansion_precedes_start(self): self.composition('source-network')
    def test_whole_entry_prior_failed_preparation_drift_precedes_start(self): self.composition('prior-drift')
    def test_whole_entry_partial_observer_preserves_committed_rows(self): self.composition('partial-observer')
    def test_whole_entry_IAM_failure_disables_then_stops_all(self): self.composition('IAM-error')
    def test_whole_entry_disable_failure_still_stops_all(self): self.composition('disable-all-error')
    def test_whole_entry_Native_inspect_error_still_attempts_disable(self): self.composition('Native-inspect-error')
    def test_whole_entry_individual_stop_error_does_not_skip_others(self): self.composition('one-stop-error')

    def test_raw_owner_channels_survive_wait_timeout_and_inspect_failure(self):
        with tempfile.TemporaryDirectory() as tmp, ExitStack() as stack:
            root = Path(tmp)
            stack.enter_context(patch.object(m, 'CONTROL', root)); stack.enter_context(patch.object(m, 'PUBLIC', root))
            stack.enter_context(patch.object(m, 'own_contract', return_value={}))
            stack.enter_context(patch.object(m, 'closed', return_value=True))
            stack.enter_context(patch.object(m, 'docker', return_value=b''))
            stack.enter_context(patch.object(m, 'inspect', side_effect=ValueError('modeled state failure')))
            def run(args, **kw):
                if args[1] == 'wait': raise subprocess.TimeoutExpired(args, 120, output=b'partial wait', stderr=b'wait timeout')
                return SimpleNamespace(stdout=b'original stdout', stderr=b'original stderr', returncode=0)
            stack.enter_context(patch.object(m.subprocess, 'run', side_effect=run))
            with self.assertRaises(ValueError): m.run_owner({'owners': {'observer': {'cid': 'fixed'}}}, 'observer')
            self.assertEqual((root / 'observer-wait-stdout.private').read_bytes(), b'partial wait')
            self.assertEqual((root / 'observer-stderr.private').read_bytes(), b'original stderr')
            self.assertEqual(set(m.load(root / 'observer-owner.json')['failure_channels']), {'wait-timeout', 'state-unconfirmed'})

    def test_packet_checks_actual_bytes_before_any_Docker(self):
        with patch.object(m, 'sha', return_value='foreign'), patch.object(m, 'docker', side_effect=AssertionError('no I/O')):
            with self.assertRaises(ValueError): m.packet()

    def test_allocate_rejects_inherited_image_command_before_Docker(self):
        with tempfile.TemporaryDirectory() as tmp, patch.object(m.p1,'text',side_effect=AssertionError('no Docker before fixed command')):
            with self.assertRaises(ValueError): m.allocate('observer', Path(tmp)/'binary', Path(tmp)/'private', Path(tmp)/'output', 'Core', [], 'label')

    def test_allocate_passes_explicit_observer_command_to_Docker(self):
        with tempfile.TemporaryDirectory() as tmp, patch.object(m.p1,'text',return_value='fixed') as docker:
            root=Path(tmp);binary=root/'binary';binary.write_bytes(b'approved')
            owner=m.allocate('observer',binary,root/'private',root/'output','Core',['post-v22-close'],'label')
            self.assertEqual(docker.call_args.args[-2:],(m.p1.PG,'post-v22-close'))
            self.assertIn('/var/lib/postgresql/data:size=1m',docker.call_args.args)
            self.assertEqual(docker.call_args.args.count('--tmpfs'),2)
            self.assertEqual(owner['arguments'],['post-v22-close'])

    def test_source_wrong_image_network_and_active_endpoint_refused(self):
        state = {'Id':'Core', 'Image':m.p1.PG,'NetworkSettings':{'Networks':{'bound':{'NetworkID':'net'}}}}
        other = copy.deepcopy(state); other.update(Id='Native',Image=m.p1.MI)
        manifest = {'source':{'Core':'Core','Native':'Native','Network':'net'}}
        with patch.object(m,'inspect',side_effect=lambda cid:state if cid=='Core' else other), patch.object(m,'closed',return_value=True), patch.object(m,'docker',return_value=b'[{"Internal":true,"Containers":{}}]'):
            m.source_contract(manifest)
            state['Image']='foreign'
            with self.assertRaises(ValueError):m.source_contract(manifest)
            state['Image']=m.p1.PG;state['NetworkSettings']['Networks']['extra']={'NetworkID':'external'}
            with self.assertRaises(ValueError):m.source_contract(manifest)
            del state['NetworkSettings']['Networks']['extra']
            with patch.object(m,'docker',return_value=b'[{"Internal":true,"Containers":{"foreign":{}}}]'), self.assertRaises(ValueError):m.source_contract(manifest)

    def test_legacy_configuration_drift_precedes_any_owner_inspection(self):
        candidate = {'Groups': [{'Group': 'g', 'ConfigSHA256': {'P2': 'approved'}, 'CaseConfigSHA256': {}}]}
        with patch.object(m, 'sha', return_value='foreign'), patch.object(m, 'inspect', side_effect=AssertionError('no owner I/O')):
            with self.assertRaises(ValueError): m.execution_owners(candidate, [{'group': 'g'}])

    def test_new_owner_private_whitelist_rejects_directory_and_alias(self):
        with tempfile.TemporaryDirectory() as tmp, ExitStack() as stack:
            root = Path(tmp); private = root / 'private'; private.mkdir(); output = root / 'output'; output.mkdir()
            binary = root / 'binary'; binary.write_bytes(b'approved')
            (private / 'closure.json').write_bytes(b'{}')
            owner = {'cid': 'fixed', 'kind': 'observer', 'binary': str(binary), 'binary_sha256': m.sha(binary), 'private': str(private), 'output': str(output), 'network_source': 'Core', 'arguments': []}
            state = {'Image': m.p1.PG, 'Config': {'User': '10001:10001', 'Entrypoint': ['/run/executor'], 'Cmd': []},
                     'HostConfig': {'ReadonlyRootfs': True, 'Privileged': False, 'CapDrop': ['ALL'], 'SecurityOpt': ['no-new-privileges'], 'RestartPolicy': {'Name': 'no'}, 'PortBindings': {}, 'NetworkMode': 'container:Core','Tmpfs':{'/tmp':'','/var/lib/postgresql/data':'size=1m'}},
                     'NetworkSettings': {'Ports': {}}, 'Mounts': [{'Destination': n, 'Source': str(p), 'RW': rw, 'Type': 'bind'} for n,p,rw in [('/run/private', private, False),('/run/executor',binary,False),('/run/output',output,True)]]}
            stack.enter_context(patch.object(m, 'inspect', return_value=state))
            self.assertEqual(m.own_contract(owner, {}), state)
            state['HostConfig']['Tmpfs']={}
            with self.assertRaises(ValueError):m.own_contract(owner,{})
            state['HostConfig']['Tmpfs']={'/tmp':'','/var/lib/postgresql/data':'size=1m'}
            state['Mounts'].append({'Destination':'/var/lib/postgresql/data','Source':'anonymous','RW':True,'Type':'volume'})
            with self.assertRaises(ValueError):m.own_contract(owner,{})
            state['Mounts'].pop()
            (private / 'proof.json').mkdir()
            with self.assertRaises(ValueError): m.own_contract(owner, {})
            (private / 'proof.json').rmdir()
            with patch.object(Path, 'is_symlink', return_value=True), self.assertRaises(ValueError): m.own_contract(owner, {})
            with patch.object(Path, 'is_junction', return_value=True), self.assertRaises(ValueError): m.own_contract(owner, {})

    def test_active_proof_requires_sealed_gateway_budget_and_closed_owners(self):
        candidate = {'Groups': [{'Gateway': 'gateway', 'SealedGatewaySHA256': 'approved'}]}
        old = {'owner_ids': ['Core', 'old'], 'old': {'old_control_sha256': {}}, 'readonly_profile_sha256': {}, 'backup_sha256': 'backup', 'old_budgets': 'budget'}
        manifest = {'source': {'Core': 'Core', 'Network': 'net'}, 'old_retained': old, 'old_execution_owners': {},'prior_failed_preparation':{}}
        def inspect(cid):
            if cid == 'Core': return {'State': {'Running': True}, 'Image': m.p1.PG, 'HostConfig': {'RestartPolicy': {'Name': 'no'}, 'PortBindings': {}}}
            return {'State': {'Running': False}, 'Mounts': [{'Destination': '/etc/caddy/Caddyfile', 'Source': 'sealed', 'Type': 'bind', 'RW': False}]}
        with ExitStack() as stack:
            for target,name,value in [(m,'packet',lambda:(candidate,[])),(m,'inspect',inspect),(m,'closed',lambda x:not x['State']['Running']),
                    (m,'prior_preparation_closed',lambda:{}),
                    (m,'sha',lambda p:'approved'),(m.p1,'backups',lambda:'backup'),(m.p1,'budgets',lambda:'budget'),
                    (m,'execution_owners',lambda *a:{}),(m,'docker',lambda *a:json.dumps([{'Internal':True,'Containers':{'Core':{}}}]).encode())]:
                stack.enter_context(patch.object(target,name,value))
            self.assertEqual(m.active_old_state(manifest)['old_owners_stopped'],1)
            with patch.object(m,'sha',return_value='active-gateway'), self.assertRaises(ValueError): m.active_old_state(manifest)
            with patch.object(m.p1,'budgets',return_value='new-or-refunded-budget'), self.assertRaises(ValueError): m.active_old_state(manifest)
            with patch.object(m,'closed',return_value=False), self.assertRaises(ValueError): m.active_old_state(manifest)

    def test_IAM_complete_entry_requires_actual_fresh_channels(self):
        with tempfile.TemporaryDirectory() as tmp, ExitStack() as stack:
            root = Path(tmp); at = m.now()
            save(root / 'result.json', {'action': 'check-disabled', 'exact_policy_set_verified': True, 'changed': {}, 'before': {n: 'disabled' for n in ('a','b','c','d')}})
            for n in range(8):
                save(root / f'start-{n}.json', {'method': 'GET', 'path': '/minio/admin/v3/user-info' if n % 2 else '/minio/admin/v3/info-canned-policy', 'at': (at - dt.timedelta(seconds=2)).isoformat()})
                save(root / f'observation-{n}.json', {'http_status': 200, 'transport_error': '', 'at': (at - dt.timedelta(seconds=1)).isoformat()})
            state = {'State': {'StartedAt': (at - dt.timedelta(seconds=3)).isoformat(), 'FinishedAt': at.isoformat(), 'ExitCode': 0}}
            stack.enter_context(patch.object(m, 'inspect', return_value=state)); stack.enter_context(patch.object(m, 'closed', return_value=True))
            owner = {'output': str(root), 'cid': 'fixed'}
            self.assertEqual(m.iam_receipt(owner)['quantity'], 8)
            state['State']['FinishedAt'] = (at - dt.timedelta(minutes=1)).isoformat()
            with self.assertRaises(ValueError): m.iam_receipt(owner)
            state['State']['FinishedAt'] = at.isoformat()
            save(root / 'observation-0.json', {'http_status': 200, 'transport_error': '', 'at': (at + dt.timedelta(minutes=1)).isoformat()})
            with self.assertRaises(ValueError): m.iam_receipt(owner)
            (root / 'observation-0.json').unlink()
            with self.assertRaises(FileNotFoundError): m.iam_receipt(owner)

    def test_delta_rejects_extra_relation_and_old_row_loss(self):
        names = m.ALLOWED
        before = {'relations': [{'relation': n, 'rows': 0} for n in names]}
        after = copy.deepcopy(before); after['relations'].append({'relation': 'reader_pin', 'rows': 1})
        with self.assertRaises(ValueError): m.delta(before, after, {}, {}, complete=False)
        with self.assertRaises(ValueError): m.delta(before, before, {'old': Counter({'hash': 1})}, {'old': Counter()}, complete=False)

    def test_profile_rejects_expanded_non_activation_field(self):
        with tempfile.TemporaryDirectory() as tmp, patch.object(m, 'CONTROL', Path(tmp)):
            original = {'Enabled': False, 'MaxProviderDeletes': 0, 'EvidenceSHA256': 'a', 'NotBefore': 'a', 'ExpiresAt': 'a', 'Core': 'fixed'}
            save(Path(tmp) / 'closure-disabled-original.json', original)
            changed = dict(original, Core='foreign')
            save(Path(tmp) / 'worker-private/closure.json', changed)
            with self.assertRaises(ValueError): m.profile({'activation_fields': ['Enabled', 'EvidenceSHA256', 'NotBefore', 'ExpiresAt']}, False)


if __name__ == '__main__': unittest.main()
