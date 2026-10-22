"""Read-only preparation for the fixed 5d2274ee reference-demand scope.

This command cannot provision credentials, release reservations, or send DELETE.
The resulting NOT_SENT candidate must be revalidated by an authorized server
verifier inside the existing observation/release transaction before it is used.
"""
import argparse
import datetime as dt
import hashlib
import json
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
RUN = '5d2274ee-a671-4bdc-b406-22b8dad8fd61'
PROJECT = 'dpp-lifecycle307-5d2274ee'
WORKSPACE = '89eba3fd-9763-4f66-8cec-1a779104f155'
PROVIDER = '1980751b-c50e-4895-93a8-cb55960cd635'
INCARNATION = 'be784eda-96e4-49a9-ae47-eaa3203a2dc5'
PLAN = '8f0e9340-18f0-4a73-bdd6-622196a9011a'
HASH = 'd7db735bf2ea7a8bc8eefca70ad4fc046be18e7e159e6a5231be3073e550bbfc'
EXECUTION = '614cd9ad-3f9d-40be-b110-3f7cf95f0790'
SOURCE = '4650c435f8ba89fdb0b2dc0591152d25ded182d7'
BINARY = '5e04e88bbd65880a5e65da5795dcdc116021512e04d27a200ec0d03f83e972c8'
HELPER = PROJECT + '-reader-reference-races-d85398'
HELPER_ID = 'fb92bdcd150127c50163448de9d918f3a2d9a18e2384274b30febfaf647bdb87'
RECEIPTS = ROOT / 'examples/lifecycle-d1/results' / RUN
PROPOSAL = ROOT / 'docs/acceptance/evidence/issue307-d1/delete-proposal-5d2274ee/proposal.json'
EXPECTED = {
    '8b1ff30c-3695-4847-b4cf-cc92cd6a399c': ('dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152', 'dpp-lifecycle307-online', 'fixture/a.csv', '312365a9-f984-43de-b9d5-7e26009ccd82', '722b7c8834f35892e3a3404b43ff8118b326b5c0c992207290dcc674f6b7ade0', 232),
    '3feb660b-21aa-4494-ada8-110cf3cf0e22': ('35b26496-f832-4496-bb91-1f50fb56e7fa', 'dpp-lifecycle307-online', 'fixture/b.csv', '159e90c3-adb6-4ea3-b2e8-2804e578e86f', 'b5b9321c647c6dc75e27c25beb46ecf7f4cf10f9bdd0d288887e41c343ef3524', 129328),
    'dbe34c34-cecb-4183-a121-5eac1035d176': ('dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152', 'dpp-lifecycle307-quarantine', 'fixture/a.csv', '5a7ce790-9093-4569-a8fd-0726009a5eb4', '722b7c8834f35892e3a3404b43ff8118b326b5c0c992207290dcc674f6b7ade0', 232),
    '0a239dc6-9e1d-49eb-8604-e22a93e7b6a0': ('35b26496-f832-4496-bb91-1f50fb56e7fa', 'dpp-lifecycle307-quarantine', 'fixture/b.csv', '846d0c7c-289d-4667-bc7c-aa04e44357d4', 'b5b9321c647c6dc75e27c25beb46ecf7f4cf10f9bdd0d288887e41c343ef3524', 129328),
    '686f6ecc-40a2-4a8f-a10b-ceaf026c6ada': ('03437e8c-9bbd-4693-87e0-fff6e651d0a5', 'dpp-lifecycle307-online', 'fixture/standardized.csv', '2139f293-87f4-4d49-a9f0-941b22275093', '044a854f6cf61dec3d77c6a907b0b30363ec5e96d5dca12649865772d7ee9163', 494),
    '7d85a166-59ae-4a72-a562-138e9a1617c7': ('47451712-ee38-45d1-a94e-786f59aa1647', 'dpp-lifecycle307-online', 'fixture/processed.csv', '7c8ad93b-31ee-445c-9402-816dda0410ec', '99cefa99326fb2e5daa5f23ae8802b057e5f467d5ed9813c73cdecd71f4368ad', 232),
}
TARGETS = ('8b1ff30c-3695-4847-b4cf-cc92cd6a399c', '0a239dc6-9e1d-49eb-8604-e22a93e7b6a0')
TRIALS = {
    '0a86461d-d5c2-4c00-99ae-0d44ac6dde2a': (TARGETS[0], '8b9b4d0c-df37-41b5-b3d1-0630deae8675'),
    'd8a52b7b-9cad-4ab4-a8d5-286d37ceafb9': (TARGETS[1], 'aa0dc82a-06b4-4b0d-a066-33d19ccc8f45'),
}


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def call(arguments, cwd=None):
    result = subprocess.run(arguments, cwd=cwd, capture_output=True)
    require(result.returncode == 0, 'Read-only subprocess failed; preserve diagnostics privately')
    return result.stdout


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def digest(data):
    return hashlib.sha256(data).hexdigest()


def pinned_file(path, commit):
    relative = path.relative_to(ROOT).as_posix()
    canonical = call(['git', 'show', commit + ':' + relative], ROOT)
    require(path.read_bytes().replace(b'\r\n', b'\n') == canonical, 'Pinned preparation input modified')
    return canonical


def validate_scope(copies, proposal):
    require(proposal['status'] == 'PENDING_USER_CONFIRMATION' and proposal['authority']['provider_delete_authorized'] is False and proposal['authority']['permission_enabled'] is False, 'Proposal is not a grant')
    require(proposal['project'] == PROJECT and proposal['environment_run_id'] == RUN, 'Foreign environment')
    require((proposal['workspace_id'], proposal['provider_instance_id'], proposal['incarnation_id'], proposal['plan_id'], proposal['plan_hash']) == (WORKSPACE, PROVIDER, INCARNATION, PLAN, HASH), 'Foreign frozen scope')
    require(proposal['authority']['future_execution_run_id'] == EXECUTION, 'Foreign execution stage')
    require(len(copies) == len(EXPECTED), 'Incomplete inventory')
    remaining = dict(EXPECTED)
    for copy in copies:
        identity = (copy['dataset_version_id'], copy['namespace'], copy['object_key'], copy['provider_version'], copy['content_sha256'], copy['byte_size'])
        require(remaining.pop(copy['id'], None) == identity, 'Foreign, duplicate, or changed exact copy')
        require((copy['workspace_id'], copy['provider_instance_id'], copy['incarnation_id']) == (WORKSPACE, PROVIDER, INCARNATION), 'Physical identity mismatch')
    require(not remaining and {x['copy_id'] for x in proposal['targets']} == set(TARGETS) and len(proposal['targets']) == 2, 'Expanded deletion targets')
    for target in proposal['targets']:
        expected = EXPECTED[target['copy_id']]
        require((target['dataset_version_id'], target['namespace'], target['key'], target['provider_version'], target['content_sha256'], target['byte_size']) == expected, 'Proposal target differs from registry')
        require(target['legal_hold'] is False and target['retention_mode'] == 'NONE' and target['retain_until'] is None, 'Protected proposal target')


def validate_trial(core, helper, config, launch_script, private):
    require(core['schema'] == 67, 'Unexpected schema')
    counts = core['counts']
    for name in ('send_claims', 'provider_observations', 'absence_receipts', 'completion', 'active_pins'):
        require(counts[name] == 0, 'Observed/possibly invoked or occupied trial cannot become NOT_SENT')
    require(counts['pins'] == counts['terminations'] == 7, 'Owner termination closure unavailable')
    require(len(core['attempts']) == 2 and len(core['dispatches']) == 2, 'Trial membership changed')
    remaining = dict(TRIALS)
    for attempt in core['attempts']:
        require(remaining.pop(attempt['id'], None) == (attempt['copy_id'], attempt['dispatch_id']), 'Foreign/duplicate trial attempt')
        require(attempt['workspace_id'] == WORKSPACE and attempt['plan_id'] == PLAN, 'Trial scope mismatch')
    require(not remaining, 'Missing original trial')
    expected_dispatches = {dispatch: copy for copy, dispatch in TRIALS.values()}
    for dispatch in core['dispatches']:
        require(expected_dispatches.pop(dispatch['id'], None) == dispatch['copy_id'] and dispatch['workspace_id'] == WORKSPACE and dispatch['plan_id'] == PLAN and dispatch['approval_id'] == '34274662-4973-4621-ab4d-b89c0493f1dc', 'Trial dispatch/approval mismatch')
    require(not expected_dispatches, 'Missing trial dispatch')
    approvals = [a for a in core['approvals'] if a['id'] == '34274662-4973-4621-ab4d-b89c0493f1dc']
    require(len(approvals) == 1 and approvals[0]['basis_ref'] == 'CONTROLLED_NON_DELETE_ADMISSION_EXERCISE' and approvals[0]['plan_id'] == PLAN and approvals[0]['plan_hash'] == HASH, 'Trial approval was relabeled')
    plans = [p for p in core['plans'] if p['id'] == PLAN]
    require(len(plans) == 1 and plans[0]['status'] == 'FINALIZED' and plans[0]['content_hash'] == plans[0]['computed_hash'] == HASH and set(plans[0]['targets']) == set(TARGETS), 'Frozen plan changed')
    require(helper['Id'] == HELPER_ID and helper['State']['Status'] == 'exited' and not helper['State']['Running'] and helper['State']['ExitCode'] == 0 and helper['RestartCount'] == 0, 'Executor not positively stopped')
    require(helper['State']['StartedAt'] == '2026-10-14T00:59:49.71037659Z' and helper['State']['FinishedAt'] == '2026-10-14T01:09:52.775625295Z', 'Original helper was restarted/replaced')
    require(helper['Config']['Entrypoint'] == ['/bin/sh'] and helper['Config']['Cmd'] == ['/run/private/' + HELPER + '.sh'], 'Foreign entry point')
    require(launch_script.replace('\r\n', '\n') == '#!/bin/sh\nset -eu\nexec /run/private/reader -mode plane-reference-races\n', 'Execution script changed')
    require(config['RootAccess'] == config['RootSecret'] == '' and config['Enabled'] is False and config['RunID'] == RUN and config['WorkspaceID'] == WORKSPACE, 'Root-bearing or reopened executor')
    require(helper['Config']['Labels']['lifecycle307.run'] == RUN and helper['HostConfig']['NetworkMode'] == 'container:ba90451138af2c980f2ee77b58bfe2908ef67e06b1e03c96fd1a4e0836400af4', 'Foreign helper ownership/network')
    require(helper['Image'] == 'sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea', 'Foreign executor image')
    expected_mounts = {'/run/private', '/run/output', '/run/repo', '/run/rules'}
    require({m['Destination'] for m in helper['Mounts']} == expected_mounts and len(helper['Mounts']) == 4, 'Extra executor credential mount')
    for mount in helper['Mounts']:
        require(mount['RW'] == (mount['Destination'] == '/run/output'), 'Writable executor inputs')
        source = {'/run/private': private / 'plane-owner', '/run/output': ROOT / 'examples/lifecycle-d1/results', '/run/repo': ROOT, '/run/rules': private / 'plane-owner/rules'}[mount['Destination']]
        require(mount['Type'] == 'bind' and Path(mount['Source']).resolve() == source.resolve(), 'Foreign executor input mount')
    allowed_env = {'PATH', 'GOSU_VERSION', 'LANG', 'PG_MAJOR', 'PG_VERSION', 'PG_SHA256', 'DOCKER_PG_LLVM_DEPS', 'PGDATA'}
    require({e.split('=', 1)[0] for e in helper['Config']['Env']} <= allowed_env, 'Ambient executor credentials')
    require(counts['copy_operations'] == 0 and counts['processing_output_intents'] == counts['processing_output_ready'] == 2, 'Pending-copy closure changed')
    for intent in core['processing_output_intents']:
        require(sum(r['intent_id'] == intent['id'] for r in core['processing_output_ready']) == 1, 'Pending output remains')


def prepare(private, output):
    script_hash = digest(Path(__file__).read_bytes())
    script_head = call(['git', 'rev-parse', 'HEAD'], ROOT).decode().strip()
    require(private.name == PROJECT and private.is_absolute() and not private.is_relative_to(ROOT), 'Foreign private root')
    require(not output.exists(), 'Append-only preflight output exists')
    proposal = json.loads(pinned_file(PROPOSAL, '917465d88a0f6e8dfd0a4745fb4d8d767a2c75a1'))
    require(private == Path(proposal['private_root']).resolve(), 'Private root differs from pinned proposal')
    # Bind historical receipts to canonical committed bytes, not supplied flags.
    frozen_path = ROOT / 'docs/acceptance/evidence/issue307-d1/reference-native-regression-manifest.json'
    frozen = json.loads(pinned_file(frozen_path, 'fc009d88ce2e145d10bcf086addca80eb8686fee'))
    require(len(frozen['entries']) == 48 and len({entry['path'] for entry in frozen['entries']}) == 48, 'Incomplete native manifest')
    for entry in frozen['entries']:
        path = entry['path']
        blob = call(['git', 'show', 'fc009d88ce2e145d10bcf086addca80eb8686fee:' + path], ROOT)
        require(digest(blob) == entry['sha256'], 'Frozen native receipt mismatch')
        require((ROOT / path).read_bytes().replace(b'\r\n', b'\n') == blob, 'Native receipt modified')
    binding = read(RECEIPTS / 'reference-source-binding.json')
    require(binding['code_commit'] == SOURCE and binding['reader_binary_sha256'] == BINARY and len(binding['source_files']) == 685, 'Native code binding changed')
    for entry in binding['source_files']:
        require(call(['git', 'rev-parse', SOURCE + ':' + entry['path']], ROOT).decode().strip() == entry['git_blob'], 'Historical source blob changed')
    owner = private / 'plane-owner'
    require(digest((owner / 'reader').read_bytes()) == BINARY, 'Native reader binary changed')
    helper = json.loads(call(['docker', 'inspect', HELPER]))[0]
    config = read(owner / 'reader.json')
    query = pinned_file(PROPOSAL.parent / 'core-readback.sql', '917465d88a0f6e8dfd0a4745fb4d8d767a2c75a1').decode()
    core = json.loads(call(['docker', 'exec', PROJECT + '-postgres', 'psql', '-X', '-U', 'fixture', '-d', 'lifecycle307', '-v', 'ON_ERROR_STOP=1', '-Atqc', query]))
    validate_scope(core['copies'], proposal)
    validate_trial(core, helper, config, (owner / (HELPER + '.sh')).read_text(encoding='utf-8'), private)
    # Policies are canonical frozen inputs; neither actor owns native DELETE.
    for name, allowed in [('reader-iam-policy.json', {'s3:GetObjectVersion'}), ('plane-producer-policy.json', {'s3:PutObject'})]:
        policy = read(RECEIPTS / name)
        for statement in policy['Statement']:
            require(statement['Effect'] == 'Allow' and set(statement['Action']) <= allowed and 'NotAction' not in statement, 'Executor has unproven native authority')
    output.mkdir(parents=True)
    profile = {'schema': 'lifecycle307-reference-preflight-v1', 'project': PROJECT, 'environment_run_id': RUN, 'future_execution_run_id': EXECUTION, 'workspace_id': WORKSPACE, 'provider_instance_id': PROVIDER, 'incarnation_id': INCARNATION, 'plan_id': PLAN, 'plan_hash': HASH, 'inventory': core['copies'], 'target_copy_ids': list(TARGETS), 'permission_enabled': False, 'human_confirmation': 'NOT_RECEIVED', 'max_delete_requests_after_separate_authorization': 2, 'max_window_minutes_after_separate_authorization': 40, 'delete_execution_mode': 'UNSUPPORTED_IN_THIS_READ_ONLY_TOOL', 'cleanup': []}
    proof = {'schema': 'lifecycle307-trial-no-send-candidate-v1', 'status': 'VERIFIED_CANDIDATE_NOT_APPLIED', 'as_of': core['as_of'], 'helper_id': HELPER_ID, 'helper_finished_at': helper['State']['FinishedAt'], 'native_source': SOURCE, 'native_binary_sha256': BINARY, 'frozen_native_receipts_checked': len(frozen['entries']), 'source_files_checked': 685, 'positive_proof': ['original immutable helper exited successfully, never restarted', 'fixed bound plane-reference-races entry point with no deleter composition', 'root-free immutable executor mount and no ambient credentials', 'frozen native reader/producer policies do not grant DELETE', 'original exact plan/attempts, verified terminations and current no-send/read-only SQL snapshot'], 'trial_attempts': core['attempts'], 'proof_inputs_sha256': {'core_readback': digest(json.dumps(core, sort_keys=True, separators=(',', ':')).encode()), 'helper_inspect': digest(json.dumps(helper, sort_keys=True, separators=(',', ':')).encode()), 'native_source_binding': digest((RECEIPTS / 'reference-source-binding.json').read_bytes())}, 'apply_requirement': 'Authorized server verifier must recheck attempt scope, no send claim/observation/release and original executor proof while holding existing fence/attempt locks; only RecordProviderObservation may append NOT_SENT/release', 'observation_appended': 0, 'reservation_released': 0, 'provider_io': 0, 'core_mutation': 0, 'provider_delete': 0}
    require(digest(Path(__file__).read_bytes()) == script_hash, 'Preflight code changed during execution')
    proof.update(preflight_parent_head=script_head, preflight_script_checkout_sha256=script_hash, preflight_script_unchanged_during_run=True, trust_seed='canonical frozen Git blobs at fc009d88 and 917465d8; supplied manifest/SQL cannot redefine proof')
    for name, value in [('preflight-profile.json', profile), ('trial-no-send-candidate.json', proof), ('current-core-readback.json', core)]:
        with (output / name).open('x', encoding='utf-8', newline='\n') as stream:
            json.dump(value, stream, indent=2)
            stream.write('\n')
    print(json.dumps({'status': 'PASS_READ_ONLY_PREFLIGHT', 'output': str(output), 'source_files': 685, 'proof_applied': False, 'provider_io': 0, 'delete': 0}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--private-directory', type=Path, required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    args = parser.parse_args()
    prepare(args.private_directory.resolve(), args.output_directory.resolve())


if __name__ == '__main__':
    main()
