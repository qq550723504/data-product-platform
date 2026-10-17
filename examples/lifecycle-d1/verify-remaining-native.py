"""Complete bounded GET/HEAD-only native preflight; never enables or sends.

The file checker owns semantic aggregation; this host entry owns the frozen
baseline, actual helper, evidence paths, time, Core and physical-close bindings.
There is no permission writer, human authorization loader or DELETE entry.
"""
import argparse
import datetime as dt
import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import time
import uuid

ROOT = Path(__file__).resolve().parents[2]
PRIVATE = Path('C:/Users/Henry/.codex/fixture-private')

def module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value

runner = module('native_readonly_runner', ROOT / 'examples/lifecycle-d1/prepare-remaining-runner.py')

def need(ok, message):
    if not ok:
        raise ValueError(message)

def load(path): return json.loads(path.read_bytes())
def sha(path): return hashlib.sha256(path.read_bytes()).hexdigest()
def now(): return dt.datetime.now(dt.timezone.utc)
def save(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open('x', encoding='utf-8', newline='\n') as stream:
        json.dump(value, stream, ensure_ascii=False, indent=2)
        stream.write('\n')

def closed(item):
    h = item['HostConfig']
    return (not item['State']['Running'] and item['State']['Pid'] == 0
            and h['RestartPolicy']['Name'] == 'no' and not h.get('PortBindings')
            and not any(item['NetworkSettings']['Ports'].values()))

def baseline(packet, group):
    candidate = load(packet / 'candidate.json')
    binding = next(b for b in load(packet / 'bindings.json') if b['group'] == group['Group'])
    fixed = next(g for g in candidate['Groups'] if g['Group'] == group['Group'])
    need(fixed == group, 'group not from frozen candidate')
    folder = packet / group['Group']
    need(sha(folder / 'iam.json') == group['IAMSHA256'], 'baseline IAM anchor differs')
    need(sha(folder / 'core-readonly.json') == binding['source_core_sha256'], 'Core checkpoint anchor differs')
    need(sha(folder / 'readonly/native-verification.json') == binding['native_readonly_sha256'], 'native baseline receipt anchor differs')
    iam = load(folder / 'iam.json')
    receipt = load(folder / 'readonly/native-verification.json')
    need(receipt['disabled_identities'] == iam['disabled_identities']
         and receipt['policy_sha256'] == iam['policy_sha256']
         and [p['Copy'] for p in receipt['copies']] == group['Copies'], 'baseline receipt/candidate differs')
    need(set(iam['policy_file_sha256']) == set(iam['policy_sha256']), 'baseline policy membership differs')
    for name, expected in iam['policy_file_sha256'].items():
        path = folder / 'readonly' / ('native-policy-' + name + '.json')
        need(path.resolve().is_relative_to((folder / 'readonly').resolve()) and sha(path) == expected,
             'baseline policy file anchor differs')
    return folder

def aggregate(packet, group, output, checker):
    frozen = baseline(packet, group)
    completed = subprocess.run([str(checker), '-baseline', str(frozen / 'readonly'),
                                '-fresh', str(output), '-group', group['Group']],
                               capture_output=True, timeout=45)
    need(completed.returncode == 0, 'complete native semantic aggregation denied')
    result = json.loads(completed.stdout)
    need(result['status'] == 'PASS_COMPLETE_NATIVE_AGGREGATION'
         and result['group'] == group['Group'], 'offline checker output differs')
    for policy in result['policies']:
        need(policy['official_equals'] is True
             and sha(output / ('native-policy-' + policy['policy'] + '.json')) == policy['fresh_file_sha256'],
             'fresh policy changed during aggregation')
    # The SDK response raw SHA is an audit fact; Encoder may compact that JSON
    # and append LF. Preserve it separately from the exact file hashes above.
    result['provider_raw_sha256'] = load(output / 'native-verification.json')['policy_sha256']
    result['native_receipt_file_sha256'] = sha(output / 'native-verification.json')
    return result

def journals(output):
    starts = [load(p) for p in output.glob('readonly-start-*.json')]
    observations = [load(p) for p in output.glob('readonly-observation-*.json')]
    need(starts and len(starts) == len(observations), 'readonly request journal incomplete')
    need(len({s['id'] for s in starts}) == len(starts)
         and {s['id'] for s in starts} == {o['id'] for o in observations}, 'readonly attempt membership differs')
    # The fixed native verifier owns response semantics (including the expected
    # 400 NoSuchObjectLockConfiguration for NONE). Do not invent a second,
    # incompatible HTTP-success gate; keep every original status observation.
    need(all(s['method'] in ('GET', 'HEAD') for s in starts)
         and all(not o['transport_error'] and 100 <= o['status'] <= 599 for o in observations),
         'readonly journal has mutation or incomplete transport observation')
    return len(starts)

def sealed_gate(packet, owner_root):
    """Shared by candidate preparation and the future complete fresh entry.

    Verify the actual mounted files of the candidate's owners, including old
    disabled owners when making a new generation; technical reader is separate.
    """
    c, bindings = load(packet/'candidate.json'), load(packet/'bindings.json')
    need(len(c['Groups']) == len(bindings) == 3, 'finite group count differs')
    attester = module('readonly_existing_contracts', ROOT/'examples/lifecycle-d1/attest-remaining-executable.py')
    need(sha(owner_root/'operator') == c['SenderBinarySHA256']
         and sha(owner_root/'owner') == c['OwnerBinarySHA256'], 'owner binaries differ')
    count = 0
    for g,b in zip(c['Groups'], bindings):
        need(g['Group'] == b['group'], 'finite group order differs')
        folder = owner_root/g['Group']
        baseline(packet, g)
        backup = PRIVATE/'remaining-executable03'/g['Group']/'checkpoint'
        for file,key in [('core.dump','CoreDumpSHA256'),('globals.sql','GlobalsSHA256'),('native.tar','NativeArchiveSHA256')]:
            need(sha(backup/file) == g[key], 'retained backup differs')
        for stage,ident in g['Helpers'].items():
            item = runner.inspect(ident)
            attester.helper_contract(item,g,stage,folder/stage,owner_root/'operator',b['budget_volume'])
            need(closed(item) and load(folder/stage/'permission.json')['Enabled'] is False,
                 'actual operator is active or permission enabled')
            need(not (folder/stage/'human-confirmation.txt').exists(), 'old authority materialized')
            count += 1
        for action,owner in b['root_owners'].items():
            item = runner.inspect(owner['container']); h = item['HostConfig']
            mounts = {m['Destination']:m for m in item['Mounts']}
            root_folder = folder/('root-'+action)
            need(item['Image'] == runner.PG and closed(item) and h['ReadonlyRootfs'] and not h['Privileged']
                 and h['NetworkMode'] == 'container:'+g['Native'] and 'ALL' in h['CapDrop']
                 and any('no-new-privileges' in x for x in h['SecurityOpt'])
                 and set(mounts) == {'/run/private','/run/owner','/run/output'}, 'bootstrap actual contract differs')
            for target,path,rw in [('/run/private',root_folder,False),('/run/owner',owner_root/'owner',False),('/run/output',folder/'outputs'/('root-'+action),True)]:
                need(Path(mounts[target]['Source']).resolve() == path.resolve() and mounts[target]['RW'] is rw,
                     'bootstrap actual private mount differs')
            need(sha(root_folder/'root.json') == owner['root_config_sha256']
                 and load(root_folder/'permission.json')['Enabled'] is False, 'bootstrap config/permission differs')
            count += 1
        for ident in [*g['CaseHelpers'].values(),g['Core'],g['Native'],g['Gateway'],g['CheckpointCore'],g['CheckpointNative'],b['recovery_redis']]:
            need(closed(runner.inspect(ident)), 'business/source/restore owner active')
        gateway = runner.inspect(g['Gateway'])
        mount = next(m for m in gateway['Mounts'] if m['Destination'] == '/etc/caddy/Caddyfile')
        need(not mount['RW'] and sha(Path(mount['Source'])) == g['SealedGatewaySHA256'], 'gateway is not sealed')
    need(closed(runner.inspect('25a4e218f72527e8f93a1df2eeb7a09833e5028c03d5ee178009bd9c71ee33e6')), 'business Redis active')
    return {'status':'PASS_ACTUAL_SEALED_OWNERS_BACKUPS','operators_and_bootstrap':count,
            'case_owners':7,'backup_hashes':9,'permission_enabled':False,'native_delete':0,'provider_io':0}

def collect_group(packet, group, private, public, reader, checker):
    """Always close the two exact sources and the newly allocated helper.

    This function is also used during candidate freezing. A one-shot output
    directory prevents old exit0/receipts from being reused as fresh evidence.
    """
    packet, private, public, reader, checker = (p.resolve() for p in (packet, private, public, reader, checker))
    frozen = baseline(packet, group)
    need(not private.exists() and not public.exists(), 'one-shot preflight already allocated')
    private.mkdir(parents=True); public.mkdir(parents=True)
    expected_reader = sha(reader)
    old = PRIVATE / 'remaining-executable03' / group['Group'] / 'source-verifier'
    cfg = load(old / 'remaining.json')
    start = now(); cfg['Base'].update(StartedAt=start.isoformat(), ExpiresAt=(start + dt.timedelta(minutes=55)).isoformat())
    save(private / 'remaining.json', cfg)
    for name in ('inventory.json', 'manifest.json'):
        (private / name).write_bytes((old / name).read_bytes())
    owned = []; helper = None; failed = None
    try:
        for field, image in [('Core', runner.PG), ('Native', runner.MI)]:
            item = runner.inspect(group[field])
            need(closed(item) and item['Id'] == group[field] and item['Image'] == image
                 and {n['NetworkID'] for n in item['NetworkSettings']['Networks'].values()} == {group['Network']},
                 'bound source contract differs')
        network = json.loads(runner.docker('network', 'inspect', group['Network']))[0]
        need(network['Internal'], 'source network is not internal')
        for field in ('Core', 'Native'):
            owned.append(group[field]); runner.docker('start', group[field])
        runner.native.wait_pg(group['Core'], 'fixture'); time.sleep(3)
        before = runner.native.fingerprints(group['Core'])
        need(before == load(frozen / 'core-readonly.json'), 'source Core checkpoint differs')
        save(public / 'core-before.json', before)
        output = public / 'readonly'; output.mkdir()
        helper = runner.docker('create', '--pull', 'never', '--name', 'dpp307-readonly-' + uuid.uuid4().hex,
            '--label', 'lifecycle307.technical=readonly-preflight', '--network', 'container:' + group['Native'],
            '--read-only', '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges',
            '--tmpfs', '/tmp', '--tmpfs', '/var/lib/postgresql/data:size=1m',
            '--mount', f'type=bind,source={private},target=/run/private,readonly',
            '--mount', f'type=bind,source={reader},target=/run/reader,readonly',
            '--mount', f'type=bind,source={output},target=/run/output',
            '--entrypoint', '/run/reader', runner.PG, '-mode', 'remaining-verify-native').decode().strip()
        owned.append(helper); save(private / 'helper.json', {'id': helper})
        item = runner.inspect(helper); h = item['HostConfig']; mounts = {m['Destination']: m for m in item['Mounts']}
        need(item['Image'] == runner.PG and closed(item) and h['ReadonlyRootfs'] and not h['Privileged']
             and 'ALL' in h['CapDrop'] and any('no-new-privileges' in x for x in h['SecurityOpt'])
             and h['NetworkMode'] == 'container:' + group['Native']
             and set(mounts) == {'/run/private', '/run/reader', '/run/output'}, 'technical helper contract differs')
        for target, path, writable in [('/run/private', private, False), ('/run/reader', reader, False), ('/run/output', output, True)]:
            need(Path(mounts[target]['Source']).resolve() == path.resolve() and mounts[target]['RW'] is writable,
                 'technical helper mount differs')
        need(sha(reader) == expected_reader, 'reader changed before start')
        runner.docker('start', helper)
        code = int(runner.docker('wait', helper).strip()); item = runner.inspect(helper)
        save(public / 'owner.json', {'id': helper, 'exit': code, 'started_at': item['State']['StartedAt'],
             'finished_at': item['State']['FinishedAt'], 'stopped_pid_zero': closed(item),
             'reader_sha256': expected_reader, 'Core': group['Core'], 'Native': group['Native']})
        need(code == 0 and closed(item), 'native verifier failed')
        started = dt.datetime.fromisoformat(item['State']['StartedAt'].replace('Z', '+00:00'))
        finished = dt.datetime.fromisoformat(item['State']['FinishedAt'].replace('Z', '+00:00'))
        need(start <= started <= finished <= now() and now() - finished <= dt.timedelta(minutes=1),
             'technical helper is not a fresh execution')
        result = aggregate(packet, group, output, checker)
        after = runner.native.fingerprints(group['Core'])
        need(after == before, 'readonly verification changed Core')
        save(public / 'core-after.json', after)
        result.update(technical_requests=journals(output), at=now().isoformat(),
                      native_delete=0, native_mutations=0, source_Core_unchanged=True)
        save(public / 'aggregation.json', result)
        return result
    except Exception as error:
        failed = type(error).__name__ + ': ' + str(error)
        save(public / 'failure.json', {'at': now().isoformat(), 'error': failed, 'native_delete': 0})
        raise
    finally:
        closure = []
        for ident in reversed(owned):
            try:
                state = runner.inspect(ident)
                if state['State']['Running']: runner.docker('stop', '--time', '15', ident)
                state = runner.inspect(ident)
                closure.append({'id': ident, 'stopped_pid_zero': closed(state)})
            except Exception:
                closure.append({'id': ident, 'stopped_pid_zero': False})
        save(public / 'physical-close.json', {'at': now().isoformat(), 'containers': closure,
             'all_closed': all(c['stopped_pid_zero'] for c in closure), 'resources_retained': True,
             'cleanup': 0, 'native_delete': 0, 'IAM_fresh_disable_readback': 'NOT_PERFORMED_READONLY_ENTRY'})
        if not all(c['stopped_pid_zero'] for c in closure) and failed is None:
            raise RuntimeError('readonly physical closure unconfirmed')

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--packet', type=Path, required=True)
    parser.add_argument('--candidate-sha256', required=True)
    parser.add_argument('--bindings-sha256', required=True)
    parser.add_argument('--private', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--reader', type=Path, required=True)
    parser.add_argument('--checker', type=Path, required=True)
    parser.add_argument('--checker-sha256', required=True)
    args = parser.parse_args()
    # Docker --mount requires absolute host paths; apply the same contract to
    # CLI and direct library use before any source is started.
    for name in ('packet', 'private', 'output', 'reader', 'checker'):
        setattr(args, name, getattr(args, name).resolve())
    need(sha(args.packet / 'candidate.json') == args.candidate_sha256, 'frozen candidate differs')
    need(sha(args.packet / 'bindings.json') == args.bindings_sha256, 'frozen bindings differ')
    c = load(args.packet / 'candidate.json')
    need(sha(args.reader) == c['ReaderBinarySHA256'] and sha(args.checker) == args.checker_sha256,
         'frozen verification binary differs')
    need(not args.private.exists() and not args.output.exists(), 'preflight run already allocated')
    # The same precondition is used before preparation; no bypass by calling
    # only a narrower collecting segment from the freeze composition.
    sealed = sealed_gate(args.packet, args.reader.parent)
    args.output.mkdir(parents=True)
    save(args.output/'sealed-preconditions.json', sealed)
    results = []
    for group in c['Groups']:
        results.append(collect_group(args.packet, group, args.private / group['Group'],
                                    args.output / group['Group'], args.reader, args.checker))
    save(args.output / 'complete.json', {'status': 'PASS_COMPLETE_FRESH_NATIVE_PREFLIGHT',
         'candidate_sha256': args.candidate_sha256, 'execution': c['Execution'],
         'at': now().isoformat(), 'groups': results, 'versions': 18, 'native_delete': 0,
         'technical_requests': sum(r['technical_requests'] for r in results), 'send_authority': 'NONE'})

if __name__ == '__main__': main()
