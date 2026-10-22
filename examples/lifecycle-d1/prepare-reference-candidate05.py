"""Freeze candidate05 and prepare one readonly checkpoint for execution02.

No confirmation, provision, enable, dispatch, send, resume or cleanup options.
Source SQL is READ ONLY; native inspection journals only in the new Core clone.
"""
import argparse
import datetime as dt
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import subprocess
from urllib.parse import urlsplit, urlunsplit

ROOT = Path(__file__).resolve().parents[2]
PRIVATE = Path('C:/Users/Henry/.codex/fixture-private/dpp-lifecycle307-5d2274ee')
CONTROL = PRIVATE / 'reference-disposal-execution02'
PUBLIC = ROOT / 'docs/acceptance/evidence/issue307-d1/delete-candidate05-preparation01'
EXECUTION = '9ea8f3e2-c805-44b4-b133-a36f50df9d15'
PROFILE = '5d2274ee-9ea8f3e2'
PROJECT = 'dpp-lifecycle307-5d2274ee'
PRIOR = ROOT / 'docs/acceptance/evidence/issue307-d1/delete-confirmed-v14-execution01'
AUDIT = PROJECT + '-delete-checkpoint-core-02'


def require(ok, message):
    if not ok:
        raise ValueError(message)


def module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def write(path, value):
    with path.open('x', encoding='utf-8', newline='\n') as stream:
        stream.write(json.dumps(value, indent=2) + '\n')


def run(argv, cwd=ROOT, env=None):
    result = subprocess.run(list(map(str, argv)), cwd=cwd, env=env, capture_output=True)
    if result.returncode:
        if CONTROL.exists():
            (CONTROL / ('failure-' + dt.datetime.now(dt.timezone.utc).strftime('%H%M%S%f') + '.private-log')).write_bytes(result.stdout + result.stderr)
        raise RuntimeError('Fixed preparation failed; retained, no automatic retry')
    return result.stdout


def validate_closure(value):
    previous = json.loads((PRIOR / 'final-core-readback.json').read_text(encoding='utf-8'))
    for key in ('attempts', 'observations', 'releases', 'claims', 'results', 'provider_costs', 'basis'):
        canonical = lambda rows: sorted(json.dumps(x, sort_keys=True) for x in rows)
        require(canonical(value[key]) == canonical(previous[key]), 'Prior immutable facts changed: ' + key)
    require(value['absence'] == value['completion'] == value['unaccounted_storage'] == 0
            and value['storage_calls'] == value['storage_observations'] == value['storage_costs'] == 125
            and value['new_basis'] == value['new_command_keys'] == 0,
            'Prepared execution acquired business authority or accounting drifted')


def checkpoint_dsn(source):
    value = urlsplit(source)
    require(value.scheme == 'postgres' and value.hostname == PROJECT + '-postgres'
            and value.port == 5432 and value.path == '/lifecycle307'
            and value.username == 'fixture' and value.password
            and value.query == 'sslmode=disable' and not value.fragment,
            'Unexpected source DSN; checkpoint isolation refused')
    target = urlunsplit((value.scheme, value.netloc.rsplit('@', 1)[0] + '@' + AUDIT + ':5432',
                        value.path, value.query, ''))
    require(urlsplit(target).hostname == AUDIT and target != source,
            'Checkpoint DSN still addresses source')
    return target


def stop_all(m, identifiers):
    states, failures = [], []
    for identifier in dict.fromkeys(identifiers):
        try:
            item = m.inspect(identifier)
            if item['State']['Running']:
                m.docker('stop', identifier)
            item = m.inspect(identifier)
            require(item['State']['Status'] in ('created', 'exited') and item['State']['Pid'] == 0
                    and not any(item['NetworkSettings']['Ports'].values()), 'Resource remains active')
            states.append({'id': item['Id'], 'name': item['Name'], 'status': item['State']['Status'], 'pid': 0})
        except Exception:
            failures.append(identifier)
    return {'states': states, 'failures': failures, 'cleanup': 0}


def prepare():
    require(not CONTROL.exists() and not PUBLIC.exists(), 'Append-only preparation already owned')
    require(not run(['git', 'status', '--porcelain']).strip(), 'Freeze requires committed clean checkout')
    source = run(['git', 'rev-parse', 'HEAD']).decode().strip()
    m = module('retained', ROOT / 'examples/lifecycle-d1/complete-lc13-readonly.py')
    m.validate_source()
    previous_ids = m.docker('ps', '-aq', '--filter', 'label=lifecycle307.run=5d2274ee-a671-4bdc-b406-22b8dad8fd61').decode().split()
    require(len(previous_ids) == 86, 'Reviewed retained topology membership changed')
    for identifier in previous_ids:
        item = m.inspect(identifier)
        require(not item['State']['Running'] and item['State']['Pid'] == 0 and not any(item['NetworkSettings']['Ports'].values()), 'Retained topology active/exposed')
    seals = [PRIVATE / 'config.json', *PRIVATE.rglob('reader.json'), *PRIVATE.rglob('reference-permission.json'), PRIVATE / 'reference-disposal-execution01/recovery/recovery.json']
    require(all(m.load(p).get('Enabled') is False for p in seals), 'Prior authority reopened')
    preservation = {p: sha(p) for p in [*seals, PRIVATE / 'Caddyfile', PRIVATE / 'reference-disposal-execution01/gateway/Caddyfile', *PRIVATE.rglob('*.dump'), *PRIVATE.rglob('*.tar')]}
    m.CONTROL = CONTROL
    CONTROL.mkdir()
    candidate = CONTROL / 'candidate05'
    candidate.mkdir()
    build = dict(os.environ, CGO_ENABLED='0', GOOS='linux', GOARCH='amd64')
    for binary, package in [('executor', './cmd/lifecycle-fixture-delete'), ('recovery', './cmd/lifecycle-fixture-recovery'), ('iam-readonly', ROOT / 'examples/lifecycle-d1/candidate05/iam-readonly.go')]:
        run(['go', 'build', '-trimpath', '-o', candidate / binary, package], ROOT / 'apps/platform', build)
    build['GOOS'] = 'windows'
    run(['go', 'build', '-trimpath', '-o', candidate / 'host.exe', './cmd/lifecycle-fixture-delete'], ROOT / 'apps/platform', build)
    PUBLIC.mkdir()
    write(PUBLIC / 'initial-host.json', {'retained_containers': previous_ids, 'all_stopped_pid0_no_ports': True,
          'private_seals_and_backups_sha256': {p.relative_to(PRIVATE).as_posix(): h for p, h in preservation.items()}, 'delete': 0})
    config = m.load(PRIVATE / 'reference-disposal-preparation01/native-after/reference.json')
    config['PreparationEnabled'] = False
    config['PreparationUntil'] = '2026-10-14T00:00:00Z'
    require(not config['Fixture']['RootAccess'] and not config['Fixture']['RootSecret'], 'Root-free candidate required')
    config['Fixture']['Enabled'] = False
    write(candidate / 'reference.json', config)
    write(candidate / 'reference-permission.json', {'Enabled': False})
    owned = []

    def helper(role, folder, out, binary, network, mode=None, expected=0, payload=None, profile=PROFILE):
        out.mkdir()
        argv = ['create', '--pull', 'never', '--name', PROJECT + '-candidate05-' + role,
                '--label', 'lifecycle307.run=' + m.load(PRIVATE / 'resources.json')['run_id'],
                '--label', 'lifecycle307.execution=' + EXECUTION, '--network', network, '--read-only',
                '--tmpfs', '/tmp', '--tmpfs', '/var/lib/postgresql/data', '--mount',
                f'type=bind,source={folder},target=/run/private,readonly', '--mount',
                f'type=bind,source={out},target=/run/output']
        if payload:
            argv += ['--mount', f'type=bind,source={payload},target=/run/payload']
        if mode:
            argv += ['--env', 'LIFECYCLE_REFERENCE_PROFILE=' + profile, '--env', 'LIFECYCLE_REFERENCE_MODE=' + mode]
        identifier = m.docker(*argv, '--entrypoint', '/run/private/' + binary, m.PGIMAGE).decode().strip()
        owned.append(identifier)
        m.docker('start', identifier)
        code = int(m.docker('wait', identifier).decode().strip())
        item = m.inspect(identifier)
        write(out / 'helper.json', {'id': identifier, 'name': item['Name'], 'image': item['Image'], 'mode': mode,
              'profile': profile, 'exit': code, 'expected_exit': expected, 'pid': item['State']['Pid'],
              'network': network, 'binary_sha256': sha(folder / binary), 'read_only': True,
              'mounts': [{'destination': x['Destination'], 'rw': x['RW'], 'type': x['Type']} for x in item['Mounts']],
              'root_mounted': role == 'iam', 'provider_delete': 0})
        require(code == expected and item['State']['Status'] == 'exited', 'Helper failed; no retry')

    try:
        for mode in ('render', 'dry-run-send', 'send-a', 'send-b'):
            helper(mode, candidate, PUBLIC / mode, 'executor', 'none', mode, 1 if mode.startswith('send-') else 0)
        helper('old-profile', candidate, PUBLIC / 'old-profile', 'executor', 'none', 'render', 1, profile='5d2274ee-614cd9ad')
        profile = m.load(PUBLIC / 'render/bounded-profile.json')
        require(profile['execution'] == EXECUTION and not profile['permission_enabled'] and profile['human_confirmation'] == 'NOT_RECEIVED', 'Foreign/enabled render')
        runtime = {}
        for path in sorted((ROOT / 'apps/platform').rglob('*.go')):
            if not path.name.endswith('_test.go'):
                runtime[path.relative_to(ROOT).as_posix()] = path.read_bytes().replace(b'\r\n', b'\n')
        for name in ('go.mod', 'go.sum'):
            runtime['apps/platform/' + name] = (ROOT / 'apps/platform' / name).read_bytes().replace(b'\r\n', b'\n')
        for path, raw in runtime.items():
            require(run(['git', 'show', source + ':' + path]) == raw, 'Runtime differs from source commit')
        write(PUBLIC / 'source-binding.json', {'status': 'FROZEN_DISABLED_CANDIDATE05', 'source_commit': source,
              'execution': EXECUTION, 'binary_sha256': sha(candidate / 'executor'), 'host_binary_sha256': sha(candidate / 'host.exe'),
              'recovery_binary_sha256': sha(candidate / 'recovery'), 'iam_readonly_binary_sha256': sha(candidate / 'iam-readonly'),
              'runtime_source_lf_sha256': {p: hashlib.sha256(raw).hexdigest() for p, raw in runtime.items()},
              'profile_sha256': sha(PUBLIC / 'render/bounded-profile.json'), 'target_digest': profile['target_digest'],
              'iam_sha256': profile['iam_policy_sha256'], 'active_gateway_sha256': profile['active_gateway_sha256'],
              'sealed_gateway_sha256': profile['sealed_gateway_sha256'], 'build': 'go build -trimpath; CGO_ENABLED=0; amd64',
              'permission': False, 'approval': 'NOT_RECEIVED', 'provider_delete': 0})
        (PUBLIC / 'binary-modules.txt').write_bytes(run(['go', 'version', '-m', candidate / 'executor']))
        stage = CONTROL / 'checkpoint'
        stage.mkdir()
        out = PUBLIC / 'preflight'
        out.mkdir()
        for identifier in (m.PG, m.MINIO):
            m.docker('start', identifier)
        for _ in range(60):
            try:
                before = m.fingerprint(m.PG)
                break
            except RuntimeError:
                import time
                time.sleep(1)
        else:
            raise RuntimeError('Source Core unavailable')
        require(before == m.load(PRIOR / 'source-after.json')['relations'], 'Sealed source Core changed')
        roles = m.role_acl(m.PG)
        require(roles == m.load(PRIOR / 'preflight/source-before.json')['roles_acl'], 'Source roles/ACL changed')
        write(out / 'source-before.json', {'relations': before, 'roles_acl': roles, 'at': dt.datetime.now(dt.timezone.utc).isoformat()})
        collector = module('current_collector', ROOT / 'examples/lifecycle-d1/candidate05/core-readonly-collector.py')
        collector.collect(m, out)
        closure_query = (ROOT / 'examples/lifecycle-d1/candidate05/closure-readback.sql').read_text(encoding='utf-8')
        closure = json.loads(m.source_read(m.PG, closure_query))
        validate_closure(closure)
        write(out / 'prior-execution-closure.json', closure)
        history = re.search(r'const referenceSendHistoryQuery = `(.*?)`', (ROOT / 'apps/platform/cmd/lifecycle-fixture-delete/reference_send.go').read_text(encoding='utf-8'), re.S).group(1)
        # Execute exactly the sender's SQL, as READ ONLY, with fixed typed values.
        q = "PREPARE candidate05_history(uuid,uuid,uuid) AS " + history + "; EXECUTE candidate05_history('8b1ff30c-3695-4847-b4cf-cc92cd6a399c','8f0e9340-18f0-4a73-bdd6-622196a9011a','89eba3fd-9763-4f66-8cec-1a779104f155'); DEALLOCATE candidate05_history;"
        gate = m.source_read(m.PG, q)
        require(gate == 't|t|f|f', 'Current source blocks new A history precondition')
        write(out / 'sender-history-readonly.json', {'result': gate, 'old_trials_closed': True, 'old_denied_closed': True, 'new_target_attempt': False, 'first_absent': False, 'query_sha256': hashlib.sha256(history.encode()).hexdigest(), 'delete': 0})
        m.binary_dump(['exec', m.PG, 'pg_dump', '-U', 'fixture', '-d', 'lifecycle307', '--format=custom', '--create'], stage / 'core.dump')
        m.binary_dump(['exec', m.PG, 'pg_dumpall', '-U', 'fixture', '--globals-only'], stage / 'globals.sql')
        (stage / 'password').write_text(secrets.token_hex(32), encoding='ascii')
        volume = AUDIT + '-data'
        m.docker('volume', 'create', '--label', 'lifecycle307.execution=' + EXECUTION, volume)
        audit_id = m.docker('create', '--pull', 'never', '--name', AUDIT, '--label', 'lifecycle307.run=' + profile['environment'],
                           '--label', 'lifecycle307.execution=' + EXECUTION, '--network', PROJECT + '-private', '--mount',
                           f'type=volume,source={volume},target=/var/lib/postgresql/data', '--mount', f'type=bind,source={stage},target=/run/backup,readonly',
                           '-e', 'POSTGRES_USER=checkpoint_admin', '-e', 'POSTGRES_DB=postgres', '-e', 'POSTGRES_PASSWORD_FILE=/run/backup/password',
                           '-e', 'POSTGRES_HOST_AUTH_METHOD=scram-sha-256', m.PGIMAGE).decode().strip()
        owned.append(audit_id)
        m.docker('start', audit_id)
        m.wait_audit_core(AUDIT)
        m.docker('exec', AUDIT, 'psql', '-X', '-U', 'checkpoint_admin', '-d', 'postgres', '-v', 'ON_ERROR_STOP=1', '-f', '/run/backup/globals.sql')
        m.docker('exec', AUDIT, 'pg_restore', '--exit-on-error', '--create', '-U', 'checkpoint_admin', '-d', 'postgres', '/run/backup/core.dump')
        require(m.fingerprint(AUDIT) == before and m.role_acl(AUDIT) == roles, 'New Core checkpoint restore differs')
        clone_closure = json.loads(m.source_read(AUDIT, closure_query))
        validate_closure(clone_closure)
        write(out / 'checkpoint.json', {'status': 'PASS_FULL_CURRENT_CORE_RESTORE', 'execution': EXECUTION, 'container': AUDIT, 'id': audit_id, 'volume': volume,
              'tables': len(before), 'rows': sum(x['rows'] for x in before), 'roles_acl': roles, 'dump_sha256': sha(stage / 'core.dump'), 'globals_sha256': sha(stage / 'globals.sql'), 'prior_facts_preserved': True, 'new_basis': 0, 'new_approval': 0, 'delete': 0})
        native = stage / 'native'
        native.mkdir()
        payload = stage / 'payload'
        payload.mkdir()
        shutil.copyfile(candidate / 'executor', native / 'executor')
        config['Fixture']['PostgresDSN'] = checkpoint_dsn(config['Fixture']['PostgresDSN'])
        config['PreparationEnabled'] = True
        config['PreparationUntil'] = (dt.datetime.now(dt.timezone.utc) + dt.timedelta(minutes=40)).isoformat()
        write(native / 'reference.json', config)
        write(native / 'reference-permission.json', {'Enabled': False})
        helper('native', native, PUBLIC / 'native', 'executor', 'container:' + m.MINIO, 'inspect-source', payload=payload)
        m.six_equal(m.load(PUBLIC / 'native/native-six-version.json'), m.load(PRIOR / 'failure-native/native-six-version.json'))
        config['PreparationEnabled'] = False
        (native / 'reference.json').write_text(json.dumps(config, indent=2) + '\n', encoding='utf-8')
        calls = m.source_read(AUDIT, 'SELECT (SELECT count(*) FROM lifecycle_storage_call),(SELECT count(*) FROM lifecycle_storage_observation),(SELECT count(*) FROM cost_allocation WHERE lifecycle_storage_call_id IS NOT NULL);')
        require(calls == '145|145|145', 'Native checkpoint reads not fully journaled')
        admin = CONTROL / 'iam'
        admin.mkdir()
        cfg, owner = m.load(PRIVATE / 'config.json'), m.load(PRIVATE / 'plane-owner/reader.json')
        write(admin / 'iam.json', {'RootAccess': cfg['RootAccess'], 'RootSecret': cfg['RootSecret'], 'RunID': profile['environment'], 'Enabled': False,
              'Users': {'reader': owner['Access'], 'producer': owner['ProducerAccess'], 'inspector': cfg['InspectAccess'], 'executor': 'refdelete9ea8f3e2', 'previous_executor': 'refdelete614cd9ad'}})
        shutil.copyfile(candidate / 'iam-readonly', admin / 'iam-readonly')
        helper('iam', admin, PUBLIC / 'iam', 'iam-readonly', 'container:' + m.MINIO)
        require(m.load(PUBLIC / 'iam/native-iam-readonly.json')['status'] == 'PASS', 'IAM closed state unavailable')
        require(m.fingerprint(m.PG) == before and m.role_acl(m.PG) == roles, 'Source facts changed during readonly preflight')
        validate_closure(json.loads(m.source_read(m.PG, closure_query)))
        write(out / 'source-after.json', {'relations': before, 'roles_acl': roles, 'source_mutation': 0, 'native_reads_in_checkpoint': 20, 'checkpoint_calls_observations_costs': calls, 'admin_rpc': 5, 'iam_mutation': 0, 'delete': 0})
    finally:
        # Independent stop attempts even after helper creation/readback failure.
        sealed = stop_all(m, [*reversed(owned), m.MINIO, m.PG])
        for path in CONTROL.rglob('reference.json'):
            try:
                cfg = m.load(path)
                cfg['PreparationEnabled'] = False
                path.write_text(json.dumps(cfg, indent=2) + '\n', encoding='utf-8')
            except Exception:
                sealed['failures'].append('close-preparation:' + path.relative_to(CONTROL).as_posix())
        sealed['old_private_seals_and_backups_unchanged'] = all(sha(p) == h for p, h in preservation.items())
        if not sealed['old_private_seals_and_backups_unchanged']:
            sealed['failures'].append('prior-private-preservation')
        try:
            identifiers = m.docker('ps', '-aq', '--filter', 'label=lifecycle307.run=5d2274ee-a671-4bdc-b406-22b8dad8fd61').decode().split()
            project = []
            for identifier in identifiers:
                item = m.inspect(identifier)
                require(not item['State']['Running'] and item['State']['Pid'] == 0 and not any(item['NetworkSettings']['Ports'].values()), 'Project remains active/exposed')
                project.append({'id': item['Id'], 'image': item['Image'], 'status': item['State']['Status'], 'pid': 0})
            sealed['all_project_containers'] = project
        except Exception:
            sealed['failures'].append('project-closed-metadata')
        write(PUBLIC / 'sealed-state.json', sealed)
        require(not sealed['failures'], 'Seal failed; retained diagnostics require attention')
    write(PUBLIC / 'prepared-execution.json', {'status': 'PREPARED_NOT_AUTHORIZED', 'execution': EXECUTION,
          'source_commit': source, 'checkpoint': 'preflight/checkpoint.json', 'candidate': 'source-binding.json',
          'human_confirmation': 'NOT_RECEIVED', 'permission_enabled': False, 'basis': 0, 'approval': 0,
          'dispatch': 0, 'send_claim': 0, 'native_delete_iam': 'NOT_PROVISIONED', 'active_gateway_started': False,
          'recovery_enabled': False, 'delete': 0, 'cleanup': 0, 'hold_release': 0, 'bypass': 0})
    print(json.dumps({'status': 'PASS_FROZEN_CANDIDATE05_AND_READONLY_CHECKPOINT', 'execution': EXECUTION, 'source': source, 'provider_delete': 0, 'source_mutation': 0}))


def main():
    argparse.ArgumentParser(description=__doc__).parse_args()
    prepare()


if __name__ == '__main__':
    main()
