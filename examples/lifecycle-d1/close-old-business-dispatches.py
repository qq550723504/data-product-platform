"""Bounded old-v22 NOT_SENT closure. No enable, DELETE, budget refund or pin release."""
import argparse
import base64
import datetime as dt
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import time
import uuid

ROOT = Path(__file__).resolve().parents[2]
CONTROL = Path('C:/Users/Henry/.codex/fixture-private/post-v22-business-closure01')
PUBLIC = ROOT / 'docs/acceptance/evidence/issue307-d1/post-v22-business-closure01'
AUDIT = '23587686-0a95-4eaa-a010-a8c896c9735b'
ALLOWED = {'lifecycle_approval_revocation', 'lifecycle_provider_observation', 'lifecycle_dispatch_release',
           'command_idempotency', 'delivery_authorization_fence', 'audit_event', 'evidence', 'evidence_relation', 'outbox_event'}


def need(ok, message):
    if not ok:
        raise ValueError(message)


def module(name, filename):
    spec = importlib.util.spec_from_file_location(name, ROOT / 'examples/lifecycle-d1' / filename)
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


audit = module('closure_audit_tools', 'audit-old-business-dispatches.py')
p1 = module('closure_p1_tools', 'reconcile-p1-post-v22.py')
p1.CONTROL = CONTROL
load, sha, write, docker, inspect, closed, hostpath = p1.load, p1.sha, p1.write, p1.docker, p1.inspect, p1.closed, p1.hostpath


def now():
    return dt.datetime.now(dt.timezone.utc)


def original_targets():
    path = ROOT / 'docs/acceptance/evidence/issue307-d1/post-v22-dispatch-audit' / AUDIT / 'actual-Core-rows.json'
    result = []
    for row in load(path):
        need(audit.classify(row)['status'] == 'CORE_NO_CONFLICT_NOT_YET_TRUSTED_NOT_SENT', 'old read-only audit is not the expected source')
        a, d = row['attempts'][0], row['dispatches'][0]
        approval = next(x for x in row['approvals'] if x['id'] == d['approval_id'])
        result.append(dict(Case=row['case'], Plan=a['plan_id'], Copy=a['copy_id'], Dispatch=d['id'], Attempt=a['id'], Approval=approval['id'], PlanHash=approval['plan_hash']))
    need([x['Case'] for x in result] == ['P2', 'T1', 'T2'], 'old audit target membership differs')
    return result


def sources():
    names = p1.call(['git', 'ls-files', 'apps/platform']).decode('utf-8').splitlines()
    names = [n for n in names if n.endswith('.go') or n in ('apps/platform/go.mod', 'apps/platform/go.sum')]
    names += ['examples/lifecycle-d1/' + n for n in ('close-old-business-dispatches.py', 'audit-old-business-dispatches.py',
              'reconcile-p1-post-v22.py', 'prepare-remaining-native.py', 'complete-lc13-readonly.py', 'test_old_business_dispatch_closure.py')]
    tracked = set(p1.call(['git', 'ls-files']).decode('utf-8').splitlines())
    need(all(n in tracked for n in names), 'closure source must be committed')
    need(all(p.relative_to(ROOT).as_posix() in tracked for p in (ROOT / 'apps/platform').rglob('*.go')), 'untracked Go build input')
    return {n: sha(ROOT / n) for n in sorted(set(names))}


def packet():
    need(sha(p1.PACKET / 'candidate.json') == 'f624a5f50ccb4fa4aa8b465f5a60a090f6325e78f60544b4c18f9ba2604ef243'
         and sha(p1.PACKET / 'bindings.json') == '8e54b47e4a1f83c7ce66555ed6ce2f3ec66c962a394a8fa1feb84eeaccd82fc5', 'old packet drift')
    return load(p1.PACKET / 'candidate.json'), load(p1.PACKET / 'bindings.json')


def execution_owners(candidate, bindings):
    facts = {}
    for group, binding in zip(candidate['Groups'], bindings):
        need(group['Group'] == binding['group'], 'historical groups reordered')
        base = p1.OLD / group['Group']
        for role, digest in group['ConfigSHA256'].items():
            need(sha(base / role / 'operator.json') == digest, 'old operator input drift')
        for role, digest in group['CaseConfigSHA256'].items():
            # Frozen cases were Enabled=true; terminal containment revoked just
            # that field. Compare the exact prepared serialization after undoing
            # that single, explicitly recorded revocation in memory only.
            case = load(base / ('case-' + role) / 'remaining.json')
            need(case['Base']['Enabled'] is False, 'old case re-enabled')
            case['Base']['Enabled'] = True
            need(hashlib.sha256((json.dumps(case, indent=2) + '\n').encode()).hexdigest() == digest, 'old case immutable input drift')
        for role, root in binding['root_owners'].items():
            need(sha(base / ('root-' + role) / 'root.json') == root['root_config_sha256'], 'old root input drift')
        roles = [(cid, '/run/operator', candidate['SenderBinarySHA256']) for cid in group['Helpers'].values()]
        roles += [(cid, '/run/reader', candidate['ReaderBinarySHA256']) for cid in group['CaseHelpers'].values()]
        roles += [(r['container'], '/run/owner', candidate['OwnerBinarySHA256']) for r in binding['root_owners'].values()]
        for cid, binary_mount, digest in roles:
            x = inspect(cid)
            h = x['HostConfig']
            need(x['Id'] == cid and x['Image'] == p1.PG and closed(x) and h['ReadonlyRootfs'] and not h['Privileged']
                 and 'ALL' in h['CapDrop'] and 'no-new-privileges' in h['SecurityOpt'], 'old execution owner is not physically closed')
            need(not any('docker.sock' in m['Source'].lower() or 'docker_engine' in m['Source'].lower() for m in x['Mounts']), 'old owner socket expansion')
            mount = next(m for m in x['Mounts'] if m['Destination'] == binary_mount)
            need(mount['Type'] == 'bind' and not mount['RW'] and sha(hostpath(mount['Source'])) == digest, 'old execution binary drift')
            # Config/command/mount definitions are immutable for a fixed Docker
            # CID. Record the actual retired UID; do not claim old UID was nonroot.
            facts[cid] = {'binary_sha256': digest, 'retired_user': x['Config']['User'], 'entrypoint': x['Config']['Entrypoint'],
                          'command': x['Config']['Cmd'], 'network_mode': h['NetworkMode']}
    need(len(facts) == 39, 'old execution owner membership differs')
    return facts


def own_contract(owner, manifest):
    x = inspect(owner['cid'])
    h, mounts = x['HostConfig'], {m['Destination']: m for m in x['Mounts']}
    need(x['Image'] == p1.PG and x['Config']['User'] == '10001:10001' and h['ReadonlyRootfs'] and not h['Privileged']
         and h['CapDrop'] == ['ALL'] and 'no-new-privileges' in h['SecurityOpt'] and h['RestartPolicy']['Name'] == 'no'
         and not h.get('PortBindings') and not any(x['NetworkSettings']['Ports'].values()), 'new closure owner containment differs')
    need(h['NetworkMode'] == 'container:' + owner['network_source'] and x['Config']['Entrypoint'] == ['/run/executor']
         and x['Config']['Cmd'] == owner['arguments'] and set(mounts) == {'/run/private', '/run/executor', '/run/output'}, 'new closure owner command/mount set differs')
    for destination, path, rw in [('/run/private', owner['private'], False), ('/run/executor', owner['binary'], False), ('/run/output', owner['output'], True)]:
        m = mounts[destination]
        need(m['Type'] == 'bind' and hostpath(m['Source']) == Path(path).resolve() and m['RW'] is rw, 'new closure mount identity differs')
    need(sha(Path(owner['binary'])) == owner['binary_sha256'], 'new closure binary drift')
    if owner['kind'] == 'observer':
        allowed = {'closure.json', 'proof.json'} if (Path(owner['private']) / 'proof.json').exists() else {'closure.json'}
        entries = list(Path(owner['private']).iterdir())
        need({p.name for p in entries} == allowed and all(p.is_file() and not p.is_symlink() and not p.is_junction() for p in entries), 'observer private directory expansion or alias')
    return x


def allocate(kind, binary, private, output, source, arguments, label):
    output.mkdir()
    cid = p1.text('create', '--name', 'dpp307-old-closure-' + label + '-' + kind, '--label', 'lifecycle307.closure=' + label,
                  '--network', 'container:' + source, '--user', '10001:10001', '--read-only', '--cap-drop', 'ALL',
                  '--security-opt', 'no-new-privileges', '--restart', 'no', '--pids-limit', '64', '--memory', '256m', '--cpus', '1',
                  '--mount', 'type=bind,source=' + str(private) + ',target=/run/private,readonly',
                  '--mount', 'type=bind,source=' + str(binary) + ',target=/run/executor,readonly',
                  '--mount', 'type=bind,source=' + str(output) + ',target=/run/output', '--entrypoint', '/run/executor', p1.PG, *arguments)
    return {'kind': kind, 'cid': cid, 'binary': str(binary), 'binary_sha256': sha(binary), 'private': str(private), 'output': str(output),
            'network_source': source, 'arguments': arguments}


def prepare():
    need(not p1.call(['git', 'diff', '--name-only']).strip() and not p1.call(['git', 'diff', '--cached', '--name-only']).strip(), 'commit reviewed closure sources first')
    need(not CONTROL.exists(), 'one-shot closure already allocated')
    CONTROL.mkdir()
    PUBLIC.mkdir(exist_ok=False)
    before = audit.retained(p1)
    candidate, bindings = packet()
    group, binding = candidate['Groups'][0], bindings[0]
    owners = execution_owners(candidate, bindings)
    env = os.environ.copy()
    env.update(GOOS='linux', GOARCH='amd64', CGO_ENABLED='0')
    p1.call(['go', 'version'])
    built = subprocess.run(['go', 'build', '-o', str(CONTROL / 'observer'), './cmd/lifecycle-fixture-old-dispatch'], cwd=ROOT / 'apps/platform', env=env, capture_output=True)
    with (CONTROL / 'build.private').open('xb') as f:
        f.write(built.stdout + built.stderr)
    need(built.returncode == 0, 'bounded observer build failed')
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
    from cryptography.hazmat.primitives import serialization
    key = Ed25519PrivateKey.generate()
    with (CONTROL / 'host-key.private').open('xb') as f:
        f.write(key.private_bytes(serialization.Encoding.Raw, serialization.PrivateFormat.Raw, serialization.NoEncryption()))
    public_key = key.public_key().public_bytes(serialization.Encoding.Raw, serialization.PublicFormat.Raw).hex()
    audit_id = str(uuid.uuid4())
    worker = CONTROL / 'worker-private'
    root = CONTROL / 'root-private'
    worker.mkdir()
    root.mkdir()
    old_root = p1.OLD / 'business-races/root-check-disabled'
    for name in ('root.json', 'candidate.json', 'iam.json'):
        with (root / name).open('xb') as f:
            f.write((old_root / name).read_bytes())
    shutil.copytree(old_root / 'policies', root / 'policies')
    need(sha(root / 'root.json') == binding['root_owners']['check-disabled']['root_config_sha256'] and sha(root / 'iam.json') == group['IAMSHA256'], 'new read-only bootstrap inputs drift')
    cfg = load(p1.PRIVATE / 'remaining-executable03/business-races/source-verifier/remaining.json')['Base']
    p1.core_connection_binding(cfg, group)
    profile = dict(Enabled=False, AuditID=audit_id, WorkspaceID=cfg['WorkspaceID'], ActorID=cfg['ActorID'], Purpose='POST_V22_BUSINESS_NOT_SENT_CLOSURE',
                   PostgresDSN=cfg['PostgresDSN'], Core=group['Core'], BinarySHA256=sha(CONTROL / 'observer'), PublicKey=public_key,
                   EvidenceSHA256='0' * 64, Token=secrets.token_hex(32), Owners={cid: value['binary_sha256'] for cid, value in owners.items()},
                   NotBefore='0001-01-01T00:00:00Z', ExpiresAt='0001-01-01T00:00:00Z', MaxProviderDeletes=0)
    write(worker / 'closure.json', profile)
    write(CONTROL / 'closure-disabled-original.json', profile)
    root_binary = p1.OLD / 'owner'
    need(sha(root_binary) == candidate['OwnerBinarySHA256'], 'fixed read-only bootstrap binary drift')
    allocated = {'observer': allocate('observer', CONTROL / 'observer', worker, CONTROL / 'output-observer', group['Core'], [], audit_id[:8])}
    for action in ('check-disabled', 'disable-all'):
        allocated[action] = allocate(action, root_binary, root, CONTROL / ('output-' + action), group['Native'], ['-action', action], audit_id[:8])
    manifest = {'status': 'PREPARED_DISABLED_NON_DELETE_ONLY', 'audit_id': audit_id, 'source_head': p1.call(['git', 'rev-parse', 'HEAD']).decode().strip(),
                'source_files': sources(), 'source': group, 'old_retained': before, 'old_execution_owners': owners, 'targets': original_targets(),
                'owners': allocated, 'public_key': public_key, 'activation_fields': ['Enabled', 'NotBefore', 'ExpiresAt', 'EvidenceSHA256'],
                'inputs': {p.relative_to(CONTROL).as_posix(): sha(p) for p in CONTROL.rglob('*') if p.is_file() and p.suffix != '.private'},
                'root_read_actions': ['check-disabled', 'disable-all'], 'enable': 0, 'native_delete': 0, 'pins_released': 0, 'budgets_refunded': 0}
    write(CONTROL / 'manifest.json', manifest)
    write(PUBLIC / 'preparation.json', manifest)
    for owner in allocated.values():
        need(closed(own_contract(owner, manifest)), 'allocated closure owner not stopped')
    print(json.dumps({'status': manifest['status'], 'audit_id': audit_id, 'manifest_sha256': sha(CONTROL / 'manifest.json'), 'source_started': False}))


def profile(manifest, active):
    original = load(CONTROL / 'closure-disabled-original.json')
    current = load(CONTROL / 'worker-private/closure.json')
    for key in manifest['activation_fields']:
        current[key] = original[key]
    need(current == original and current['MaxProviderDeletes'] == 0, 'observer configuration expanded')
    actual = load(CONTROL / 'worker-private/closure.json')
    need(actual['Enabled'] is active, 'observer activation state differs')
    if active:
        start, end = map(dt.datetime.fromisoformat, (actual['NotBefore'], actual['ExpiresAt']))
        need(start <= now() < end and dt.timedelta() < end - start <= dt.timedelta(minutes=40), 'non-delete observer deadline invalid')
    return actual


def frozen(manifest):
    packet()
    need(p1.call(['git', 'rev-parse', 'HEAD']).decode().strip() == manifest['source_head']
         and not p1.call(['git', 'diff', '--name-only']).strip() and not p1.call(['git', 'diff', '--cached', '--name-only']).strip(), 'closure source HEAD/tracked drift')
    for name, digest in manifest['source_files'].items():
        need(sha(ROOT / name) == digest, 'closure source bytes changed')
    for name, digest in manifest['inputs'].items():
        if name != 'worker-private/closure.json':
            need(sha(CONTROL / name) == digest, 'closure immutable input changed')


def active_old_state(manifest):
    group = manifest['source']
    for cid in manifest['old_retained']['owner_ids']:
        if cid != group['Core']:
            need(closed(inspect(cid)), 'old owner/source is running during proof capture')
    for name, digest in manifest['old_retained']['old']['old_control_sha256'].items():
        need(sha(p1.OLD / name) == digest, 'old control drift during closure')
    for name, digest in manifest['old_retained']['readonly_profile_sha256'].items():
        need(sha(p1.PRIVATE / name) == digest, 'old read-only profile drift during closure')
    need(p1.backups() == manifest['old_retained']['backup_sha256'] and p1.budgets() == manifest['old_retained']['old_budgets'], 'old budget or backup drift during closure')
    candidate, bindings = packet()
    for source in candidate['Groups']:
        gateway = inspect(source['Gateway'])
        mounted = next(m for m in gateway['Mounts'] if m['Destination'] == '/etc/caddy/Caddyfile')
        need(closed(gateway) and mounted['Type'] == 'bind' and not mounted['RW']
             and sha(hostpath(mounted['Source'])) == source['SealedGatewaySHA256'], 'old gateway is not physically sealed')
    need(execution_owners(candidate, bindings) == manifest['old_execution_owners'], 'old execution ownership drift')
    core = inspect(group['Core'])
    need(core['State']['Running'] and core['Image'] == p1.PG and core['HostConfig']['RestartPolicy']['Name'] == 'no' and not core['HostConfig'].get('PortBindings'), 'active Core identity/state differs')
    network = json.loads(docker('network', 'inspect', group['Network']))[0]
    need(network['Internal'] and set(network['Containers']) == {group['Core']}, 'active closure network has an unexpected owner')
    return {'at': now().isoformat(), 'old_owners_stopped': len(manifest['old_retained']['owner_ids']) - 1,
            'old_execution_owner_sha256': {cid: x['binary_sha256'] for cid, x in manifest['old_execution_owners'].items()}, 'budget_backup_controls': 'EXACT_PREPARATION_ANCHOR'}


def source_contract(manifest):
    group = manifest['source']
    for field, image in (('Core', p1.PG), ('Native', p1.MI)):
        x = inspect(group[field])
        need(x['Id'] == group[field] and x['Image'] == image and closed(x)
             and {n['NetworkID'] for n in x['NetworkSettings']['Networks'].values()} == {group['Network']}, 'closed source image/network identity differs')
    network = json.loads(docker('network', 'inspect', group['Network']))[0]
    need(network['Internal'] and not network.get('Containers'), 'source network is external or has an active owner')


def run_owner(manifest, kind):
    owner = manifest['owners'][kind]
    need(closed(own_contract(owner, manifest)), 'new owner not closed before start')
    print(json.dumps({'phase': kind, 'at': now().isoformat()}), flush=True)
    receipt = {'cid': owner['cid'], 'closed': False, 'failure_channels': [], 'log_sha256': {}}
    channels = {}
    waited = logs = x = None
    try:
        docker('start', owner['cid'])
        waited = subprocess.run(['docker', 'wait', owner['cid']], capture_output=True, timeout=120)
        channels.update({'wait-stdout': waited.stdout, 'wait-stderr': waited.stderr})
    except subprocess.TimeoutExpired as error:
        channels.update({'wait-stdout': error.stdout or b'', 'wait-stderr': error.stderr or b''})
        receipt['failure_channels'].append('wait-timeout')
    except Exception:
        receipt['failure_channels'].append('start-or-wait-unconfirmed')
    try:
        logs = subprocess.run(['docker', 'logs', owner['cid']], capture_output=True, timeout=30)
        channels.update({'stdout': logs.stdout, 'stderr': logs.stderr})
    except subprocess.TimeoutExpired as error:
        channels.update({'stdout': error.stdout or b'', 'stderr': error.stderr or b''})
        receipt['failure_channels'].append('logs-timeout')
    except Exception:
        receipt['failure_channels'].append('logs-unconfirmed')
    try:
        x = inspect(owner['cid'])
        receipt.update(exit=x['State']['ExitCode'], closed=closed(x), started_at=x['State']['StartedAt'], finished_at=x['State']['FinishedAt'])
    except Exception:
        receipt['failure_channels'].append('state-unconfirmed')
    for name, raw in channels.items():
        try:
            need(isinstance(raw, bytes), 'raw Docker channel is not bytes')
            with (CONTROL / (kind + '-' + name + '.private')).open('xb') as f:
                f.write(raw)
            receipt['log_sha256'][name] = hashlib.sha256(raw).hexdigest()
        except Exception:
            receipt['failure_channels'].append(name + '-persistence-unconfirmed')
    write(PUBLIC / (kind + '-owner.json'), receipt)
    need(not receipt['failure_channels'] and waited is not None and waited.returncode == 0 and waited.stdout.strip() == b'0'
         and logs is not None and logs.returncode == 0 and receipt['exit'] == 0 and receipt['closed'], 'bounded closure owner failed; see independent raw channel receipts')
    return receipt


def iam_receipt(owner, action='check-disabled'):
    folder = Path(owner['output'])
    result = load(folder / 'result.json')
    starts = sorted(folder.glob('start-*.json'))
    expected_requests = 8 if action == 'check-disabled' else 16
    need(action in ('check-disabled', 'disable-all') and result['action'] == action and result['exact_policy_set_verified'] is True
         and (not result['changed'] if action == 'check-disabled' else len(result['changed']) == 4 and set(result['changed'].values()) == {'disabled'})
         and len(result['before']) == 4 and set(result['before'].values()) == {'disabled'} and len(starts) == expected_requests, 'fresh complete IAM disabled proof absent')
    x = inspect(owner['cid'])
    beginning, end = (dt.datetime.fromisoformat(x['State'][k]) for k in ('StartedAt', 'FinishedAt'))
    need(closed(x) and x['State']['ExitCode'] == 0 and beginning <= end <= now()
         and now() - end <= dt.timedelta(seconds=45), 'IAM owner time/closure is stale')
    records = []
    for path in starts:
        request = load(path)
        observation = load(folder / path.name.replace('start-', 'observation-'))
        allowed = request['method'] == 'GET' and request['path'] in ('/minio/admin/v3/user-info', '/minio/admin/v3/info-canned-policy')
        allowed = allowed or action == 'disable-all' and request['method'] == 'PUT' and request['path'] == '/minio/admin/v3/set-user-status'
        need(allowed
             and observation['http_status'] == 200 and not observation['transport_error'], 'IAM actual request/observation differs')
        need(beginning <= dt.datetime.fromisoformat(request['at']) <= dt.datetime.fromisoformat(observation['at']) <= end, 'IAM request/observation timing differs')
        records.append({'request_sha256': sha(path), 'observation_sha256': sha(folder / path.name.replace('start-', 'observation-'))})
    return {'result': result, 'technical_requests': records, 'quantity': len(records), 'amount': 'NOT_ASSESSED'}


def contain(manifest, started, before_source_stop=None):
    """Continue every close even when an earlier close or fresh IAM check fails."""
    result = {'profile_disabled': False, 'identity': 'UNCHANGED_NO_ENABLE_PATH', 'owners': []}
    try:
        c = load(CONTROL / 'worker-private/closure.json')
        c['Enabled'] = False
        (CONTROL / 'worker-private/closure.json').write_text(json.dumps(c, indent=2) + '\n', encoding='utf-8', newline='\n')
        result['profile_disabled'] = profile(manifest, False)['Enabled'] is False
    except Exception:
        pass
    # A timed-out observer must stop before the final Core inspection. Stopping
    # it does not undo any transaction that already committed.
    try:
        cid = manifest['owners']['observer']['cid']
        docker('stop', '-t', '15', cid)
        result['observer_stopped'] = closed(inspect(cid))
    except Exception:
        result['observer_stopped'] = False
    if before_source_stop is not None:
        before_source_stop(result['observer_stopped'])
    native = manifest['source']['Native']
    try:
        running = False
        if native in started:
            try:
                running = inspect(native)['State']['Running']
            except Exception:
                # Unknown source state cannot veto a fixed disable attempt.
                running = True
        if running:
            # Never start a source in containment. This fixed owner can only
            # disable the four exact identities; all readback/partial errors stay.
            result['identity'] = 'UNCONFIRMED'
            run_owner(manifest, 'disable-all')
            result['disable_all'] = iam_receipt(manifest['owners']['disable-all'], 'disable-all')
            result['identity'] = 'FRESH_CONFIRMED_DISABLED'
    except Exception:
        result['identity'] = 'UNCONFIRMED'
    for cid in [x['cid'] for x in manifest['owners'].values()] + list(reversed(started)):
        item = {'cid': cid}
        try:
            docker('stop', '-t', '15', cid)
        except Exception:
            item['stop_error'] = True
        try:
            item['stopped_PID0'] = closed(inspect(cid))
        except Exception:
            item['stopped_PID0'] = False
        result['owners'].append(item)
    return result


def wire_receipt():
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
    folder = p1.OLD / 'business-races/outputs/wire'
    files = list(folder.iterdir())
    need([p.name for p in files] == ['wire-5a4c9789-3e67-4e54-ab58-128d111cbd65.json'], 'original wire output membership changed')
    path = files[0]
    need(sha(path) == 'd1b291a02ff6d1401891b85d4da0160fc956716c412df6198b67859f69576a32', 'original P1 wire witness changed')
    proof = load(path)
    body = base64.b64decode(proof['Body'], validate=True)
    Ed25519PublicKey.from_public_bytes(bytes.fromhex(load(p1.PACKET / 'candidate.json')['WirePublicKey'])).verify(base64.b64decode(proof['Signature'], validate=True), body)
    value = json.loads(body)
    need(value['Execution'] == audit.EXECUTION and value['UpstreamStatus'] == 204 and value['Attempt']['ID'] == '009f4317-8855-4f99-80ec-cafc225f654e', 'original wire witness identity differs')
    return {'retained_P1_witness_sha256': sha(path), 'other_wire_witnesses': [], 'NOT_SENT_basis': 'frozen claim-before-wire and budget-before-provider contracts, current no-conflict Core plus trusted physically stopped owners and fresh disabled IAM/sealed gateway; not absence alone'}


def delta(before, after, anchors_before, anchors_after, complete=True):
    need({k: v for k, v in before.items() if k != 'relations'} == {k: v for k, v in after.items() if k != 'relations'}, 'Core roles or ACL changed')
    old, new = ({x['relation']: x for x in v['relations']} for v in (before, after))
    changed = {name for name in old if old[name] != new[name]}
    need(set(old) == set(new) and changed <= ALLOWED, 'unexpected Core relation changed')
    for name, rows in anchors_before.items():
        need(all(anchors_after[name][digest] >= count for digest, count in rows.items()), 'old Core fact overwritten or lost')
    expected = {'lifecycle_approval_revocation': 3, 'lifecycle_provider_observation': 3, 'lifecycle_dispatch_release': 3, 'command_idempotency': 6,
                'audit_event': 9, 'evidence': 9, 'evidence_relation': 9, 'outbox_event': 9}
    actual = {name: new[name]['rows'] - old[name]['rows'] for name in expected}
    if complete:
        need(actual == expected, 'NOT_SENT closure fact counts differ')
    return {'changed_relations': sorted(changed), 'new_row_counts': actual, 'old_rows_preserved': True, 'new_provider_Cost': 0, 'pins_released': 0}


def run(review_path, review_sha):
    manifest = load(CONTROL / 'manifest.json')
    report = load(review_path)
    need(sha(review_path) == review_sha and report['status'] == 'PASS_BOUNDED_INDEPENDENT_OLD_DISPATCH_CLOSURE_MANIFEST_REVIEW'
         and report['remaining_blockers'] == [] and report['manifest_sha256'] == sha(CONTROL / 'manifest.json'), 'closure manifest independent gate failed')
    need(not (PUBLIC / 'terminal.json').exists(), 'one-shot closure already terminal')
    frozen(manifest)
    need(audit.retained(p1) == manifest['old_retained'], 'retained pre-activation state drift')
    profile(manifest, False)
    source_contract(manifest)
    for owner in manifest['owners'].values():
        need(closed(own_contract(owner, manifest)), 'new closure owner not stopped')
    g = manifest['source']
    started = []
    result = {'audit_id': manifest['audit_id'], 'native_delete': 0, 'budgets_refunded': 0, 'pins_released': 0, 'old_execution_resumed': False}
    failure = None
    native = before = anchors_before = None
    try:
        for cid in (g['Native'], g['Core']):
            started.append(cid)
            docker('start', cid)
        native = p1.native_module()
        native.wait_pg(g['Core'], 'fixture')
        before = native.fingerprints(g['Core'])
        p1.ALLOWED_RELATIONS = ALLOWED
        anchors_before = p1.append_anchors(native, g['Core'])
        write(PUBLIC / 'Core-before.json', before)
        current = [json.loads(line) for line in native.sql(g['Core'], audit.query()).splitlines()]
        need(len(current) == 3 and all(audit.classify(x)['status'] == 'CORE_NO_CONFLICT_NOT_YET_TRUSTED_NOT_SENT' for x in current), 'current Core has an observation/claim/conflict; do not close')
        run_owner(manifest, 'check-disabled')
        iam = iam_receipt(manifest['owners']['check-disabled'])
        docker('stop', '-t', '15', g['Native'])
        need(closed(inspect(g['Native'])), 'Native did not stop after read-only IAM proof')
        checked_at = now()
        state = active_old_state(manifest)
        wire = wire_receipt()
        need(now() - dt.datetime.fromisoformat(load(PUBLIC / 'check-disabled-owner.json')['finished_at']) <= dt.timedelta(seconds=45), 'IAM/owner readback is stale')
        bundle = {'audit_id': manifest['audit_id'], 'checked_at': checked_at.isoformat(), 'manifest_sha256': sha(CONTROL / 'manifest.json'),
                  'old_current_state': state, 'wire': wire, 'IAM': iam, 'Core_before_sha256': sha(PUBLIC / 'Core-before.json')}
        write(PUBLIC / 'trusted-evidence.json', bundle)
        c = load(CONTROL / 'closure-disabled-original.json')
        c.update(Enabled=True, NotBefore=checked_at.isoformat(), ExpiresAt=(checked_at + dt.timedelta(minutes=40)).isoformat(), EvidenceSHA256=sha(PUBLIC / 'trusted-evidence.json'))
        (CONTROL / 'worker-private/closure.json').write_text(json.dumps(c, indent=2) + '\n', encoding='utf-8', newline='\n')
        from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
        key = Ed25519PrivateKey.from_private_bytes((CONTROL / 'host-key.private').read_bytes())
        body = {'AuditID': manifest['audit_id'], 'Nonce': str(uuid.uuid4()), 'At': checked_at.isoformat(), 'Execution': audit.EXECUTION,
                'CandidateSHA256': 'f624a5f50ccb4fa4aa8b465f5a60a090f6325e78f60544b4c18f9ba2604ef243',
                'BindingsSHA256': '8e54b47e4a1f83c7ce66555ed6ce2f3ec66c962a394a8fa1feb84eeaccd82fc5', 'Core': g['Core'],
                'EvidenceSHA256': c['EvidenceSHA256'], 'Targets': manifest['targets'], 'Owners': c['Owners'],
                'StoppedPIDZero': True, 'NoRestart': True, 'ConfigDisabled': True, 'DeleteIdentityDisabled': True, 'GatewaySealed': True}
        encoded = json.dumps(body, separators=(',', ':')).encode()
        write(CONTROL / 'worker-private/proof.json', {'Body': base64.b64encode(encoded).decode(), 'Signature': base64.b64encode(key.sign(encoded)).decode()})
        frozen(manifest)
        profile(manifest, True)
        run_owner(manifest, 'observer')
        result['observer'] = load(CONTROL / 'output-observer/result.json')
        need(result['observer']['completed'] and len(result['observer']['cases']) == 3 and all('NOT_SENT' in x for x in result['observer']['cases']), 'observer completion missing')
        after = native.fingerprints(g['Core'])
        write(PUBLIC / 'Core-after.json', after)
        result['Core_delta'] = delta(before, after, anchors_before, p1.append_anchors(native, g['Core']))
        rows = [json.loads(line) for line in native.sql(g['Core'], audit.query()).splitlines()]
        write(PUBLIC / 'actual-closed-Core-rows.json', rows)
        need(len(rows) == 3 and all(len(x['observations']) == len(x['releases']) == len(x['revocations']) == 1 and not x['claims'] and not x['absences'] and x['unresolved'] == 0
             and x['observations'][0]['certainty'] == x['observations'][0]['outcome'] == 'NOT_SENT' and x['observations'][0]['invoked_at'] is None for x in rows), 'actual Core closure does not match')
    except Exception as error:
        failure = type(error).__name__ + ': ' + str(error)
    finally:
        def final_core(observer_stopped):
            # Independent best effort after partial commit, timeout or result
            # decoding failure. Preserve original rows/results, never reissue.
            for name, work in [
                ('observer_result', lambda: load(CONTROL / 'output-observer/result.json')),
                ('Core_rows', lambda: [json.loads(line) for line in native.sql(g['Core'], audit.query()).splitlines()]),
                ('Core_fingerprint', lambda: native.fingerprints(g['Core'])),
                ('Core_anchors', lambda: p1.append_anchors(native, g['Core']))]:
                try:
                    value = work()
                    write(PUBLIC / ('final-' + name + '.json'), value)
                    result['final_' + name] = {'status': 'OBSERVED', 'sha256': sha(PUBLIC / ('final-' + name + '.json'))}
                except Exception:
                    result['final_' + name] = {'status': 'UNKNOWN_NOT_ASSESSED'}
            result['final_observer_stopped_before_Core'] = observer_stopped
            try:
                result['final_Core_delta'] = delta(before, load(PUBLIC / 'final-Core_fingerprint.json'), anchors_before,
                                                   load(PUBLIC / 'final-Core_anchors.json'), complete=False)
            except Exception:
                result['final_Core_delta'] = {'status': 'UNKNOWN_NOT_ASSESSED'}
        closure = contain(manifest, started, final_core)
        result['closure'] = closure
        try:
            result['retained_unchanged'] = audit.retained(p1) == manifest['old_retained']
        except Exception:
            result['retained_unchanged'] = False
        result['failure'] = failure
        complete = (not failure and closure['profile_disabled'] and closure['observer_stopped'] and closure['identity'] != 'UNCONFIRMED'
                    and all(x['stopped_PID0'] for x in closure['owners']) and result['retained_unchanged']
                    and result['final_Core_delta'].get('old_rows_preserved') is True
                    and all(result['final_' + name]['status'] == 'OBSERVED' for name in ('observer_result', 'Core_rows', 'Core_fingerprint', 'Core_anchors')))
        result['status'] = 'PASS_TRUSTED_OLD_DISPATCH_NOT_SENT_SEALED' if complete else 'FAILED_OR_UNCONFIRMED_KEEP_ALL_FACTS'
        write(PUBLIC / 'terminal.json', result)
        write(CONTROL / 'terminal.json', result)
    print(json.dumps({'status': result['status'], 'native_delete': 0}), flush=True)
    return result['status'] == 'PASS_TRUSTED_OLD_DISPATCH_NOT_SENT_SEALED'


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=('prepare', 'run'))
    parser.add_argument('--review', type=Path)
    parser.add_argument('--review-sha256')
    args = parser.parse_args()
    if args.action == 'prepare':
        prepare()
    else:
        need(args.review is not None and args.review_sha256 is not None, 'bound independent manifest review required')
        raise SystemExit(0 if run(args.review, args.review_sha256) else 1)
