"""Offline verification of frozen candidate05; never a send authorization."""
import argparse
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import zipfile

ROOT = Path(__file__).resolve().parents[2]
BASE = 'docs/acceptance/evidence/issue307-d1/'
DATA = BASE + 'delete-candidate05-preparation01/'
SOURCE = 'dbe3fbd666146db6a010a508fa99e6ed558b6fe9'
HISTORY = 'ce7057b296adde0ef9101c4233ff7af051f455d0'
EXECUTION = '9ea8f3e2-c805-44b4-b133-a36f50df9d15'
PINS = {'binary_sha256': '4614fe539f2a41a39fa3d07bf0adf10f832fcc7dff04abf6e9f538617a21b4cd',
        'host_binary_sha256': '46ea9e223f190e80be55ef21032eb94b8e64c992a70ddd9a3483460fdf06a6aa',
        'recovery_binary_sha256': 'dfb2b5e3c76164868bdb8950dbd153e5e8ebf0ddf4a446b36c92d7f1be1c10c3',
        'iam_readonly_binary_sha256': 'a3a5fc3417e6c23e9073a36371eae972f8882a727fa0d5c86d0fe975202b201e',
        'profile_sha256': '7f38680f8d75c3ae2332ad3bb70f64e9923abaecc504481d3930ca73194546ca',
        'iam_sha256': '77e082bcfec2a34745e24227758ce6a6af40ed1e55e834791fbd0313067de94a',
        'active_gateway_sha256': 'cf528277a361b62d19b478069391fafa596833e7cc71722a9f1632dd9b80b4d6',
        'sealed_gateway_sha256': '6a08b8647cc63cb389e2a511a5ad944b82d14e1ec9721f1a5ae45cb984bed4f6',
        'target_digest': '4cbb36e5aebbc9451603efd276aa309a2c4afdefcf252107571b0f8334360a38'}
DUMP = '4457301bc212eff3d5010b61a907948c48c330aae70d614c0530f685386f58d7'
SCOPE = {'project': 'dpp-lifecycle307-5d2274ee', 'environment': '5d2274ee-a671-4bdc-b406-22b8dad8fd61',
         'workspace': '89eba3fd-9763-4f66-8cec-1a779104f155', 'provider': '1980751b-c50e-4895-93a8-cb55960cd635',
         'incarnation': 'be784eda-96e4-49a9-ae47-eaa3203a2dc5', 'plan': '8f0e9340-18f0-4a73-bdd6-622196a9011a',
         'plan_hash': 'd7db735bf2ea7a8bc8eefca70ad4fc046be18e7e159e6a5231be3073e550bbfc',
         'inventory_hash': '2b1df1ccd9e7a138bf47050cad9f031c02bd04664b64eb12aee0337ad670c9f8'}


def require(ok, message):
    if not ok:
        raise ValueError(message)


def raw(path):
    return (ROOT / path).read_bytes()


def load(path):
    return json.loads(raw(DATA + path))


def validate(read=load):
    binding, proposal, prepared = (read(p) for p in ('source-binding.json', 'proposal.json', 'prepared-execution.json'))
    profile = read('render/bounded-profile.json')
    require(all(proposal[k] == v for k, v in SCOPE.items())
            and all(profile[k] == v for k, v in SCOPE.items() if k != 'project')
            and profile['execution'] == EXECUTION, 'Exact proposal/render scope changed')
    require(binding['source_commit'] == SOURCE and binding['execution'] == proposal['execution'] == prepared['execution'] == EXECUTION, 'Frozen identity changed')
    require(all(binding[k] == v for k, v in PINS.items()) and proposal['frozen_candidate'] == {k: v for k, v in binding.items() if k != 'runtime_source_lf_sha256'}, 'Frozen candidate hashes changed')
    require(proposal['status'] == 'PENDING_NEW_USER_CONFIRMATION' and prepared['status'] == 'PREPARED_NOT_AUTHORIZED'
            and prepared['human_confirmation'] == 'NOT_RECEIVED' and not binding['permission'] and binding['approval'] == 'NOT_RECEIVED', 'Prepared snapshot became authority')
    for key in ('basis', 'approval', 'dispatch', 'send_claim', 'permission_enabled', 'recovery_enabled', 'active_gateway_started', 'delete', 'cleanup', 'hold_release', 'bypass'):
        require(prepared[key] == 0, 'Prepared authority/side effect expanded: ' + key)
    require(not proposal['authority']['reuse_previous_authorization'] and proposal['authority']['human_confirmation'] == 'NOT_RECEIVED'
            and all(proposal['authority'][k] == 0 for k in ('basis', 'approval', 'dispatch', 'send_claim', 'permission_enabled', 'recovery_enabled', 'active_gateway_started')), 'Proposal grants authority')
    require(proposal['future_limits'] == {'max_total': 2, 'max_per_target': 1, 'max_minutes': 40,
            'target_order': ['8b1ff30c-3695-4847-b4cf-cc92cd6a399c', '0a239dc6-9e1d-49eb-8604-e22a93e7b6a0'],
            'b_requires_verified_a_absence': True, 'unknown_outcome': 'READ_ONLY_RECOVERY_NO_RESEND', 'cleanup': 0, 'hold_release': 0, 'bypass': 0}, 'Future scope/budget expanded')
    native = read('native/native-six-version.json')
    previous = json.loads(raw(BASE + 'delete-confirmed-v14-execution01/failure-native/native-six-version.json'))
    def six(proof):
        return sorted((json.dumps(v['copy'], sort_keys=True), v['bytes'], v['sha256'], v['protection']['RetentionMode'], v['protection']['RetainUntil'] or '', v['protection']['LegalHold']) for v in proof['versions'])
    require(len(native['versions']) == 6 and native['provider_delete'] == 0 and six(native) == six(previous), 'Native exact inventory/protection drift')
    require({k: v['Versions'] for k, v in native['inventories'].items()} == {k: v['Versions'] for k, v in previous['inventories'].items()}, 'Native inventory marker/membership drift')
    inventory = {v['copy']['ID']: v for v in proposal['inventory']}
    require(len(inventory) == 6 and {k for k, v in inventory.items() if v['target']} == set(proposal['future_limits']['target_order']), 'Proposal target membership changed')
    for v in native['versions']:
        row = inventory[v['copy']['ID']]
        require(row['copy'] == v['copy'] and row['bytes'] == v['bytes'] and row['sha256'] == v['sha256']
                and row['legal_hold'] == v['protection']['LegalHold'] and row['retention_mode'] == v['protection']['RetentionMode']
                and row['retain_until'] == v['protection']['RetainUntil'] and row['observed_at'] == v['protection']['ObservedAt'], 'Proposal protection observation changed')
    before, after, checkpoint = (read(p) for p in ('preflight/source-before.json', 'preflight/source-after.json', 'preflight/checkpoint.json'))
    require(before['relations'] == after['relations'] == json.loads(raw(BASE + 'delete-confirmed-v14-execution01/source-after.json'))['relations']
            and before['roles_acl'] == after['roles_acl'] == checkpoint['roles_acl'], 'Source facts or roles changed')
    require(checkpoint == proposal['core_checkpoint'] and checkpoint['execution'] == EXECUTION and checkpoint['tables'] == 194
            and checkpoint['rows'] == 2361 and checkpoint['dump_sha256'] == DUMP
            and checkpoint['id'] == '85567273b268e66511ab58d6c03a430788b3f93ceb7346a52cf44eaa82939ab6'
            and checkpoint['new_basis'] == checkpoint['new_approval'] == 0, 'Checkpoint identity/content changed')
    require(after['source_mutation'] == after['iam_mutation'] == after['delete'] == 0
            and after['native_reads_in_checkpoint'] == 20 and after['checkpoint_calls_observations_costs'] == '145|145|145', 'Read accounting/isolation changed')
    spec = importlib.util.spec_from_file_location('prep', ROOT / 'examples/lifecycle-d1/prepare-reference-candidate05.py')
    prep = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(prep)
    prep.validate_closure(read('preflight/prior-execution-closure.json'))
    require(read('preflight/sender-history-readonly.json')['result'] == 't|t|f|f', 'Current sender history blocked')
    ledger = read('preflight/native-read-ledger.json')['facts']
    ids = {v['read_call_id'] for v in native['versions']} | {x for v in native['versions'] for x in v['protection']['CallIDs']} | {v['CallID'] for v in native['inventories'].values()}
    require(len(ids) == 20 and all(len(ledger[k]) == 20 for k in ('calls', 'observations', 'allocations', 'costs'))
            and {x['id'] for x in ledger['calls']} == {x['call_id'] for x in ledger['observations']} == {x['lifecycle_storage_call_id'] for x in ledger['allocations']} == {x['activity_id'] for x in ledger['costs']} == ids
            and all(x['quantity'] == 1 and x['unit'] == 'invocation' for x in ledger['costs']), 'Exact20 typed read ledger changed')
    iam = read('iam/native-iam-readonly.json')
    require(iam['status'] == 'PASS' and iam['rpc_count'] == 5 and iam['mutations'] == iam['delete'] == 0
            and {v['role']: v['status'] for v in iam['facts']} == {'reader': 'disabled', 'producer': 'disabled', 'inspector': 'enabled', 'executor': 'NOT_PROVISIONED', 'previous_executor': 'disabled'}, 'Native IAM no longer closed')
    for mode, expected in [('render', 0), ('dry-run-send', 0), ('send-a', 1), ('send-b', 1), ('old-profile', 1)]:
        helper = read(mode + '/helper.json')
        require(helper['exit'] == expected and helper['network'] == 'none' and helper['pid'] == 0 and not helper['root_mounted'], 'Disabled/old entrypoint did not fail closed')
    sealed = read('sealed-state.json')
    require(not sealed['failures'] and sealed['old_private_seals_and_backups_unchanged'] and len(sealed['all_project_containers']) == 94
            and all(x['status'] in ('created', 'exited') and x['pid'] == 0 for x in sealed['all_project_containers']), 'Project not sealed')
    ledger_seal = read('preflight/native-read-ledger-sealed.json')
    require(ledger_seal['status'] == 'exited' and ledger_seal['pid'] == 0
            and all(ledger_seal[k] == 'exited' for k in ('source_pg', 'source_minio', 'source_gateway')), 'Ledger audit did not return sealed')


def archive(revision, path):
    return zipfile.ZipFile(io.BytesIO(subprocess.run(['git', 'archive', '--format=zip', revision, path], cwd=ROOT, capture_output=True, check=True).stdout))


def check():
    index = json.loads(raw(DATA + 'evidence-index.json'))
    require(len(index['files']) == 35 and {v['path'] for v in index['files']} == {p.relative_to(ROOT).as_posix() for p in (ROOT / DATA).rglob('*') if p.is_file() and p.name != 'evidence-index.json'}, 'Evidence membership drift')
    for entry in index['files']:
        require(entry['path'].startswith(DATA) and '..' not in Path(entry['path']).parts, 'Foreign evidence path')
        value = raw(entry['path'])
        require(len(value) == entry['bytes'] and hashlib.sha256(value).hexdigest() == entry['sha256'], 'Raw evidence/index mismatch')
    for name, pin in [('bounded-profile.json', 'profile_sha256'), ('delete-executor-policy.json', 'iam_sha256'), ('gateway-active.Caddyfile', 'active_gateway_sha256'), ('gateway-sealed.Caddyfile', 'sealed_gateway_sha256')]:
        require(hashlib.sha256(raw(DATA + 'render/' + name)).hexdigest() == PINS[pin], 'Frozen rendered artifact changed')
    with archive(SOURCE, 'apps/platform') as source:
        files = load('source-binding.json')['runtime_source_lf_sha256']
        require(len(files) == 317, 'Runtime membership changed')
        for path, digest in files.items():
            value = source.read(path).replace(b'\r\n', b'\n')
            require(hashlib.sha256(value).hexdigest() == digest and raw(path).replace(b'\r\n', b'\n') == value, 'Frozen runtime/source changed')
    with archive(HISTORY, BASE.rstrip('/')) as history:
        native = BASE + 'delete-readonly-v13-refresh01/'
        groups = [v['directory'] + '/' for v in json.loads(history.read(native + 'preflight-host.json'))['historical_preservation']]
        groups += [native, *[BASE + name + '/' for name in ('delete-proposal-v14-review01', 'delete-confirmed-v14-execution01', 'delete-confirmed-v14-review01', 'delete-confirmed-v14-ci01')]]
        old = set()
        for group in groups:
            idx = group + 'evidence-index.json'
            for path in [idx, *[v['path'] for v in json.loads(history.read(idx))['files']]]:
                require(raw(path) == history.read(path), 'Prior raw evidence changed')
                old.add(path)
        require(len(old) == 344, 'Historical raw membership changed')
    validate()
    print(json.dumps({'status': 'PASS_CANDIDATE05_PREPARED_NOT_AUTHORIZED', 'execution': EXECUTION, 'old_raw': 344, 'new_payloads': len(index['files']), 'runtime': 317, 'provider_io': 0, 'db_io': 0, 'container_starts': 0, 'delete': 0}))


def main():
    argparse.ArgumentParser(description=__doc__).parse_args()
    check()


if __name__ == '__main__':
    main()
