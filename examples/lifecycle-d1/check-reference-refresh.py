"""Offline check of one native refresh of immutable, independently reviewed v6."""
import copy
import datetime as dt
import importlib.util
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
BASE = 'docs/acceptance/evidence/issue307-d1/'
PUBLIC = BASE + 'delete-readonly-v6-refresh02/'
NATIVE = PUBLIC + '5d-06/'
PROPOSAL = PUBLIC + 'proposal-v7.json'
NAMES = ('audit-costs.json', 'audit-restored.json', 'candidate-binding.json',
         'current-core-readback.json', 'current-preconditions-readonly.sql',
         'native-helper.json', 'native-iam-helper.json', 'native-iam-readonly.json',
         'native-six-version.json', 'owner-closure-readback.json', 'readonly-checkpoint.json',
         'reference-qualification-readback.json', 'retained-final-state.json',
         'source-after.json', 'source-before.json')
PATHS = {NATIVE + name for name in NAMES} | {
    PROPOSAL, PUBLIC + 'execution-preconditions.json', PUBLIC + 'verification.json',
    PUBLIC + 'retained-topology.json'}
SPEC = importlib.util.spec_from_file_location('reviewed_v6', Path(__file__).with_name('check-reference-proposal.py'))
v6 = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(v6)


def load(path, root=ROOT):
    return v6.decode(v6.read(root, path))


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'), allow_nan=False)


def at(text):
    value = dt.datetime.fromisoformat(text.replace('Z', '+00:00'))
    v6.require(value.tzinfo is not None, 'Timestamp lacks timezone')
    return value


def refresh_paths(generation):
    v6.require(generation in ('06', '07', '08', '09', '10', '11', '12', '13'), 'Refresh outside finite generation')
    public, version = {'06': (PUBLIC, '7'), '07': (BASE + 'delete-readonly-v7-refresh01/', '8'),
                       '08': (BASE + 'delete-readonly-v8-refresh01/', '9'),
                       '09': (BASE + 'delete-readonly-v9-refresh01/', '10'),
                       '10': (BASE + 'delete-readonly-v10-refresh01/', '11'),
                       '11': (BASE + 'delete-readonly-v11-refresh01/', '12'),
                       '12': (BASE + 'delete-readonly-v12-refresh01/', '13'),
                       '13': (BASE + 'delete-readonly-v13-refresh01/', '14')}[generation]
    return public, public + '5d-' + generation + '/', public + 'proposal-v' + version + '.json'


def expected_proposal(created_at, root=ROOT, generation='06'):
    public, native_path, _ = refresh_paths(generation)
    baseline = load(v6.PROPOSAL, root)
    v6.check(baseline, root)
    review_path = BASE + 'delete-proposal-v6-review01/independent-re-review.json'
    review = load(review_path, root)
    v6.require(review['status'] == 'PASS_INDEPENDENT_OFFLINE_RE_REVIEW'
               and review['current_slice_blockers'] == 0, 'v6 review not closed')
    for path in ('examples/lifecycle-d1/check-reference-proposal.py',
                 'examples/lifecycle-d1/test_reference_proposal.py'):
        raw = v6.read(root, path).replace(b'\r\n', b'\n')
        v6.require(v6.hashlib.sha256(raw).hexdigest() == review['reviewed_fix_sha256'][path],
                   'Reviewed checker changed')
    facts = {name: load(native_path + name, root) for name in NAMES if name.endswith('.json')}
    old_native = load(v6.RECEIPTS['native'], root)
    native = facts['native-six-version.json']
    v6.require(native['provider_delete'] == native['business_admission'] == 0
               and len(native['versions']) == 6, 'Native refresh not closed six-version read')
    old = {v['copy']['ID']: v for v in old_native['versions']}
    fresh = {v['copy']['ID']: v for v in native['versions']}
    v6.require(set(fresh) == set(old), 'Native identity set changed')
    for ident, value in fresh.items():
        prior = old[ident]
        for key in ('copy', 'bytes', 'sha256'):
            v6.require(value[key] == prior[key], 'Native content changed')
        for key in ('Copy', 'LegalHold', 'RetentionMode', 'RetainUntil'):
            v6.require(value['protection'][key] == prior['protection'][key], 'Protection changed; new disposition required')
        v6.require(at(value['protection']['ObservedAt']) > at(prior['protection']['ObservedAt']), 'Observation not refreshed')
    v6.require(set(native['inventories']) == set(old_native['inventories']), 'Namespace drift')
    for namespace, inventory in native['inventories'].items():
        for key in ('WorkspaceID', 'ProviderInstanceID', 'IncarnationID', 'Namespace', 'Versions'):
            v6.require(inventory[key] == old_native['inventories'][namespace][key], 'Inventory or context drift')
    core = facts['current-core-readback.json']
    for name, ignored, old_path in (
        ('current-core-readback.json', {'as_of'}, v6.RECEIPTS['core']),
        ('reference-qualification-readback.json', {'at'}, v6.RECEIPTS['references']),
        ('owner-closure-readback.json', set(), v6.READONLY + 'retry02/5d-04/owner-closure-readback.json')):
        previous = load(old_path, root)
        v6.require({k: v for k, v in facts[name].items() if k not in ignored}
                   == {k: v for k, v in previous.items() if k not in ignored}, 'Current Core/reference closure changed')
    checkpoint = facts['readonly-checkpoint.json']
    v6.require(checkpoint['audit_core'] == baseline['project'] + '-readonly-audit-core-' + generation
               and checkpoint['source_business_mutations'] == checkpoint['native_delete'] == 0
               and checkpoint['approval'] == 'NOT_RECEIVED' and checkpoint['status'] == 'PASS', 'Checkpoint not closed')
    previous_checkpoint = load(v6.RECEIPTS['checkpoint'], root)
    v6.require({k: v for k, v in checkpoint.items() if k != 'audit_core'}
               == {k: v for k, v in previous_checkpoint.items() if k != 'audit_core'}, 'Source/checkpoint scope changed')
    binding = facts['candidate-binding.json']
    v6.require(binding['candidate'] == load(v6.CANDIDATE + 'source-binding.json', root)
               and binding['mode'] == 'inspect-source' and binding['root_mounted'] is False
               and binding['delete_permission_enabled'] is False, 'Inspection candidate/authority drift')
    helper = facts['native-helper.json']
    v6.require(helper['binary_sha256'] == baseline['candidate']['binary_sha256']
               and helper['exit'] == 0 and helper['root_mounted'] is False
               and helper['source_gateway_started'] is False
               and helper['journal_database'] == checkpoint['audit_core'], 'Native helper identity drift')
    iam_helper = facts['native-iam-helper.json']
    prior_helper = load(v6.READONLY + 'retry02/5d-04/native-iam-helper.json', root)
    v6.require({k: v for k, v in iam_helper.items() if k != 'id'}
               == {k: v for k, v in prior_helper.items() if k != 'id'}, 'IAM helper scope/binary drift')
    before, after, restored = (facts[name] for name in ('source-before.json', 'source-after.json', 'audit-restored.json'))
    v6.require(before['relations'] == after['relations'] == restored['relations']
               and before['roles_acl'] == after['roles_acl'] == restored['roles_acl']
               and before['rows'] == after['rows'] == restored['rows'] == 1879, 'Source or restored facts differ')
    v6.require(facts['audit-costs.json'] == load(v6.RECEIPTS['costs'], root), 'Audit invocation/cost accounting changed')
    iam = facts['native-iam-readonly.json']
    old_iam = load(v6.RECEIPTS['iam'], root)
    v6.require({k: v for k, v in iam.items() if k != 'checked_at'}
               == {k: v for k, v in old_iam.items() if k != 'checked_at'}, 'Native IAM closure changed')
    final = facts['retained-final-state.json']
    v6.require(not final['failures'] and final['cleanup'] == 0
               and set(final['states'].values()) == {'exited'}, 'Source/audit/helper not restored stopped')
    v6.require(at(created_at) >= at(after['at']), 'Proposal predates readback')
    result = copy.deepcopy(baseline)
    result.update(schema='lifecycle307-exact-delete-proposal-v7', created_at=created_at,
                  status='PENDING_USER_CONFIRMATION; V6_NATIVE_READONLY_REFRESH_PASS; SEND_BLOCKED',
                  audit_core=checkpoint['audit_core'], source_sql_counts=core['counts'], source_sql_as_of=core['as_of'],
                  previous_proposal=v6.reference(root, v6.PROPOSAL),
                  reviewed_baseline=v6.reference(root, review_path))
    paths = {key: native_path + Path(value).name for key, value in v6.RECEIPTS.items()}
    paths['conditions'] = public + 'execution-preconditions.json'
    result['current_readonly'] = {key: v6.reference(root, value) for key, value in paths.items()}
    for value in result['targets'] + result['survivors']:
        protection = fresh[value['copy_id']]['protection']
        value['protection_observed_at'] = protection['ObservedAt']
        value['retention_active_at_observation'] = (protection['RetentionMode'] != 'NONE'
            and protection['RetainUntil'] is not None
            and at(protection['ObservedAt']) < at(protection['RetainUntil']))
    if generation in ('07', '08', '09', '10', '11', '12', '13'):
        previous_generation = {'07': '06', '08': '07', '09': '08', '10': '09', '11': '10', '12': '11', '13': '12'}[generation]
        previous_path = refresh_paths(previous_generation)[2]
        previous = load(previous_path, root)
        check(previous, root, previous_generation)
        review_path = BASE + 'delete-proposal-v' + str(int(generation)) + '-review01/independent-review.json'
        review = load(review_path, root)
        v6.require(review['status'] == 'PASS_INDEPENDENT_OFFLINE_REVIEW'
                   and review['current_slice_blockers'] == 0, 'Predecessor review not closed')
        prior = {value['copy_id']: value for value in previous['targets'] + previous['survivors']}
        for value in result['targets'] + result['survivors']:
            v6.require(at(value['protection_observed_at']) > at(prior[value['copy_id']]['protection_observed_at']), 'Observation not newer than reviewed predecessor')
        v6.require(at(core['as_of']) > at(previous['source_sql_as_of']), 'Core observation not newer than predecessor')
        result.update(schema='lifecycle307-exact-delete-proposal-v' + str(int(generation) + 1),
                      status='PENDING_USER_CONFIRMATION; V' + str(int(generation)) + '_NATIVE_READONLY_REFRESH_PASS; SEND_BLOCKED',
                      previous_proposal=v6.reference(root, previous_path),
                      reviewed_baseline=v6.reference(root, review_path))
    return result


def check(proposal, root=ROOT, generation='06'):
    public, native_path, proposal_path = refresh_paths(generation)
    paths = PATHS if generation == '06' else {native_path + name for name in NAMES} | {
        proposal_path, public + 'execution-preconditions.json', public + 'verification.json',
        public + 'retained-topology.json', public + 'preflight-host.json'}
    entries = load(public + 'evidence-index.json', root)['files']
    v6.require(len(entries) == len(paths) and {x['path'] for x in entries} == paths, 'Refresh index membership drift')
    for entry in entries:
        raw = v6.read(root, entry['path'])
        v6.require(len(raw) == entry['bytes'] and v6.hashlib.sha256(raw).hexdigest() == entry['sha256'], 'Refresh artifact bytes drift')
    v6.require(canonical(proposal) == canonical(load(proposal_path, root)), 'Input differs from fixed v' + str(int(generation) + 1))
    v6.require(canonical(proposal) == canonical(expected_proposal(proposal['created_at'], root, generation)), 'Refresh differs from reviewed scope/fresh receipts')
    return {'status': 'PASS_OFFLINE_V' + str(int(generation) + 1) + '_REFRESH_CONSISTENCY_ONLY',
            'baseline': 'INDEPENDENTLY_REVIEWED_V' + str(int(generation)),
            'native_storage_calls': 20,
            ('management_gets_including_failed05' if generation == '06' else 'management_gets_this_refresh'): 8 if generation == '06' else 4,
            'source_core_mutation': 0, 'send_authorization': 'NOT_RECEIVED', 'delete': 0}


if __name__ == '__main__':
    print(json.dumps(check(load(PROPOSAL)), indent=2))
