"""Independent file-only audit. Does not invoke Docker, WSL, or providers."""
import hashlib
import io
import json
from pathlib import Path
import subprocess
import sys
import zipfile

OUT = Path(__file__).resolve().parent
ROOT = OUT.parents[4]
sys.path.insert(0, str(ROOT / 'examples/lifecycle-d1'))
import fixture_artifacts as artifacts

def need(value, message):
    if not value:
        raise ValueError(message)

def sha(raw):
    return hashlib.sha256(raw).hexdigest()

inputs = {}
def raw(path):
    path = Path(path)
    data = path.read_bytes()
    label = str(path.relative_to(ROOT)) if path.is_relative_to(ROOT) else str(path)
    inputs[label] = {'sha256': sha(data), 'bytes': len(data)}
    return data

def load(path):
    return json.loads(raw(path))

head = artifacts.git(ROOT, 'rev-parse', 'HEAD').decode().strip()
need(head == 'd8b782183d49f201c105e90e4c5215c1b8d7d3f4', 'review HEAD drift')
parents = artifacts.git(ROOT, 'show', '-s', '--format=%P', 'HEAD').decode().split()
need(parents == ['8249b55dec8fcf20de2d378887b9aff66047961f', '53f6f8a6e69c17fa6896a40293d323f6ee9d0958'], 'review parents differ')
need(not artifacts.git(ROOT, 'diff', '--name-only', 'HEAD', '--', 'apps/platform', 'examples/lifecycle-d1', 'migrations', '.github/workflows/ci.yml').strip(), 'tracked working source drift')
documentation_diff = artifacts.git(ROOT, 'diff', '--name-only', 'HEAD', '--', 'docs').decode().splitlines()
for name in documentation_diff:
    raw(ROOT/name)
spec = load(ROOT / artifacts.SPEC)
archive = artifacts.runtime_archive(ROOT, spec['source'])
need(sha(archive) == spec['archive_sha256'] == '1f9462c13204dddd6f8fc475b2cf21830d094aba14e1677d03848c8d70906557', 'R3 archive hash')
need(len(archive) == spec['archive_bytes'] == 6504490, 'R3 archive length')
files = artifacts.archive_members(archive)
need(len(files) == 780 and {n: sha(v) for n, v in files.items()} == spec['files'], 'R3 exact file membership')
for name in spec['shared_contract_files']:
    need(sha(raw(ROOT/name).replace(b'\r\n', b'\n')) == spec['files'][name], 'shared contract differs: '+name)
changed = artifacts.git(ROOT, 'diff', '--name-only', spec['base_source'], spec['source']).decode().splitlines()
need(sorted(changed) == sorted(spec['runtime_delta_files']), 'R3 delta outside declared two files')

evidence = ROOT/'docs/acceptance/evidence/issue307-d1'
probe = evidence/'retention-runtime-fix03'
private = Path(r'C:\Users\Henry\.codex\fixture-private\retention-runtime-fix03')
need(sha(raw(private/'approved-runtime-source.zip')) == sha(archive), 'actual Linux probe archive differs')
for name, content in files.items():
    need(sha((private/'linux-probe-source'/name).read_bytes()) == sha(content), 'actual Linux build source differs: '+name)
binary_sha = sha(raw(private/'retention-test-linux'))
summary = load(probe/'linux-probe01.json')
need(summary['status'] == 'PASS_ISOLATED_LINUX_CUSTODY_FSYNC_WAIT_TESTS' and summary['failure'] is None, 'Linux result not PASS')
need(all(summary[k] == 0 for k in ('retained_source_start','retained_provider_io','retention_write','delete','cleanup')), 'Linux result side effects')
need(summary['source_states_unchanged'] is True, 'source state drift flag')

custody = []
def inspect(prefix):
    doc = load(probe/(prefix+'-inspect.redacted.json'))
    original = raw(Path(doc['raw_private_path']))
    need(sha(original) == doc['original_sha256'], 'private inspect original SHA differs')
    actual = json.loads(original)
    need(len(actual) == len(doc['inspect']) == 1, 'inspect count differs')
    for env in doc['inspect'][0].get('Config',{}).get('Env',[]):
        need(env.endswith('=<REDACTED_PRIVATE_VALUE>'), 'public Env value not redacted')
    need(actual[0]['State'] == doc['inspect'][0]['State'], 'redacted State differs')
    custody.append({'evidence': prefix, 'sha256': sha(original), 'verified': True})
    return actual[0]

before = [inspect('probe-01'), inspect('probe-02')]
after = [inspect('probe-32'), inspect('probe-33')]
for first, last in zip(before, after):
    need(first['Id'] == last['Id'] and first['State'] == last['State'], 'exact source State drift')
    need(last['State']['Running'] is False and last['State']['Pid'] == 0 and last['HostConfig']['RestartPolicy']['Name'] == 'no', 'source not stopped PID0/no-restart')
test = inspect('probe-31')
need(test['Id'] == summary['cid'] == 'a2c0aeedd29f76a5b9cfde7f0a562c25926688f7ea4654736b18e5dec4466b0c', 'test CID differs')
need(test['State']['Running'] is False and test['State']['Pid'] == test['State']['ExitCode'] == 0, 'test not stopped PASS')
need(test['HostConfig']['NetworkMode'] == 'none' and test['Config']['User'] == '10001:10001' and test['HostConfig']['ReadonlyRootfs'] is True, 'test sandbox')
need(not test['NetworkSettings']['Ports'] and test['HostConfig']['CapDrop'] == ['ALL'] and test['HostConfig']['SecurityOpt'] == ['no-new-privileges:true'], 'test capabilities')
mounts = {m['Destination']: m for m in test['Mounts']}
need(len(mounts) == len(test['Mounts']) == 5 and set(mounts) == {'/run/private','/run/output','/run/budget','/runtime/test','/testdata'}, 'test mount expansion')
for role, view in summary['views'].items():
    need(view['metadata']['Mountpoint'] == '/var/lib/docker/volumes/'+view['metadata']['Name']+'/_data', 'volume metadata identity')
    mount = mounts['/run/'+role]
    need(mount['Type'] == 'volume' and mount['Name'] == view['metadata']['Name'] and mount['Source'] == view['metadata']['Mountpoint'] and mount['RW'] == (role != 'private'), 'volume mount differs')
    for phase in ('before','after'):
        tree = view[phase]
        need(tree['']['uid'] == tree['']['gid'] == 10001 and tree['']['mode'] == 0o700, 'Linux root custody')
        for name, row in tree.items():
            if name:
                need(row['uid'] == row['gid'] == 10001 and row['mode'] == 0o600 and row['type'] == '0', 'Linux file custody')
need(mounts['/runtime/test']['RW'] is False and mounts['/testdata']['RW'] is False, 'test bind not readonly')
compile_record = load(probe/'probe-03-compile.json')
start_record = load(probe/'probe-21-start-test.json')
need(compile_record['returncode'] == start_record['returncode'] == 0, 'compile/run failed')
text = raw(probe/'probe-21-start-test.stdout').decode()
for name in ('TestRetentionWaitFalseToTrueThenFreshAuthorization','TestRetentionWaitRejectsPreEnableDriftExpiryAndCancellation/missing-false','TestRetentionWaitRejectsPreEnableDriftExpiryAndCancellation/null-false','TestRetentionLinuxRealDirectorySyncAndTwoOwnerReadback','TestRetentionLinuxMountedVolumeCustody'):
    need('--- PASS: '+name+' ' in text, 'missing actual Linux test PASS: '+name)
need(text.rstrip().endswith('PASS') and '--- FAIL:' not in text, 'Linux test output failure')
stats = list(probe.glob('probe-*-linux-snapshot.json'))
stat_count = 0
for path in stats:
    record = load(path)
    if 'stat' in record['command']:
        need(raw(path.with_suffix('.stdout')).decode().strip() == '10001:10001:600:1:regular file', 'actual Linux nlink/stat mismatch')
        stat_count += 1
need(stat_count >= 4, 'insufficient actual Linux link checks')

old_custody = load(evidence/'retention-runtime-fix01/inspect-private-custody01.json')
need(len(old_custody['members']) == 9, 'R2 private custody membership')
for row in old_custody['members']:
    need(sha(raw(Path(row['raw_private_path']))) == row['raw_sha256'], 'R2 private raw changed')
    need(sha(raw(ROOT/row['redacted_public_path'])) == row['redacted_sha256'], 'R2 redacted bytes changed')
    need(not (ROOT/row['original_public_path']).exists(), 'R2 public raw inspect remains')
failed = load(evidence/'retention-runtime-fix02/probe-01-inspect.json')
need(failed['returncode'] != 0 and b'dockerDesktopLinuxEngine' in raw(evidence/'retention-runtime-fix02/probe-01-inspect.stderr'), 'missing preserved engine failure')

report = dict(status='PASS_FILE_ONLY_R3_ARCHIVE_AND_LINUX_EVIDENCE_AUDIT', head=head, parents=parents,
              documentation_working_differences=documentation_diff,
              archive_sha256=sha(archive), archive_bytes=len(archive), file_members=len(files),
              actual_probe_archive_matches=True, all_actual_probe_source_members_match=True,
              test_binary_sha256=binary_sha, actual_linux_stat_nlink_checks=stat_count,
              test_cid=summary['cid'], exact_source_states_unchanged=True, custody_verified=custody,
              R2_raw_custody_verified=9, Docker_WSL_provider_commands_issued=0,
              scope='File-only audit of existing isolated tests; not retained native acceptance, full orchestration, publication, or execution authority')
with (OUT/'03-input-audit.json').open('x', encoding='utf-8') as stream:
    json.dump(report, stream, indent=2)
with (OUT/'04-input-hashes.json').open('x', encoding='utf-8') as stream:
    json.dump(inputs, stream, indent=2)
print(json.dumps(report, indent=2))
