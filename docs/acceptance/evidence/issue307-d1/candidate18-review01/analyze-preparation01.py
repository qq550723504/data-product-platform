"""Independent file-only analysis of completed c18 native preparation receipts."""
from collections import Counter
import datetime as dt
import hashlib
import json
from pathlib import Path
import subprocess
import sys

OUT = Path(__file__).resolve().parent
ROOT = OUT.parents[4]
REL = 'docs/acceptance/evidence/issue307-d1'
PREP = ROOT/REL/'remaining-candidate18-preparation01'
sys.path.insert(0, str(ROOT/'examples/lifecycle-d1'))
import evidence_indexes as audit
import retained_v27

def need(value, message):
    if not value:
        raise ValueError(message)

def load(path):
    return audit.decode(path.read_bytes())

def meta(path):
    raw = audit.unlinked_absolute(path).read_bytes()
    return {'sha256': hashlib.sha256(raw).hexdigest(), 'bytes': len(raw)}

def save(name, value):
    with (OUT/name).open('x', encoding='utf-8', newline='\n') as target:
        json.dump(value, target, indent=2)
        target.write('\n')

terminal = load(PREP/'non-delete-terminal.json')
need(terminal['status'] == 'PASS_NONDELETE_SOURCE_RESTORE_PREPARATION' and terminal['groups_completed'] == 3 and terminal['failure'] is None, 'native preparation not complete')
need(terminal['native_delete'] == 0 and terminal['old_authorization_reused'] is False, 'authority/delete constraint differs')
need(len(terminal['closure']) == 92 and all(x['closed'] and not x['initial_inspect_failed'] and not x['stop_failed'] for x in terminal['closure']), '92 owner closure incomplete')
need(len(terminal['profile_revocations']) == 19 and all(x['disabled'] for x in terminal['profile_revocations']), '19 profile revocation incomplete')
index = load(PREP/'non-delete-evidence-index.json')
verified = audit.audit_index(ROOT, {'id': 'c18-native-preparation', 'index': (PREP/'non-delete-evidence-index.json').relative_to(ROOT).as_posix(),
    'sha256': terminal['evidence_index_sha256'], 'format': 'files-map-v1', 'member_root': PREP.relative_to(ROOT).as_posix(),
    'expected_count': index['count'], 'expected_status': 'SIX_NONDELETE_PREPARATION_EVIDENCE_INDEX'})
allocation = load(PREP/'allocation.json')
need(allocation['source'] == '618f484061c3624ff2195c0b591e7d8e3d743ce1', 'allocation source differs')
private = Path(allocation['groups'][0]['private']).parent
checker = private/'readonly-check.exe'
need(meta(checker)['sha256'] == allocation['binaries']['readonly-check.exe'], 'checker binary differs')
budget = load(PREP/'budget-initialization.json')
need(budget['status'] == 'PASS_NEW_BUDGET_INITIALIZATION' and budget['failure'] is None and budget['old_budget_changed'] is False, 'budget init not complete')
need(len(budget['groups']) == 3 and all(x['initial_empty'] and x['files'] == [] and x['uid'] == x['gid'] == 10001 for x in budget['groups']), 'new budget not empty or owned')
groups = []
all_journals = []
for g in allocation['groups']:
    group = g['group']
    folder = PREP/group
    checkpoint = load(folder/'checkpoint.json')
    raw_base, expected_sha = retained_v27.CORE_BASELINES[group]
    base_file = ROOT/REL/raw_base
    need(meta(base_file)['sha256'] == expected_sha, 'accepted Core baseline differs')
    core = load(base_file)
    equality = []
    for name in ('source-Core-before.json', 'source-Core-after.json', 'clean-restored-Core.json', 'trial-restored-Core.json', 'clean-Core-after-trial.json'):
        need(load(folder/name) == core, 'complete Core fingerprint differs: ' + group + '/' + name)
        equality.append({'path': (folder/name).relative_to(ROOT).as_posix(), **meta(folder/name)})
    need(len(set(checkpoint[x] for x in ('source_core', 'restored_core', 'rehearsal_core'))) == 3, 'Core role aliases')
    need(checkpoint['source_native'] != checkpoint['restored_native'], 'native source/restore alias')
    backups = {}
    for name, digest in checkpoint['backups'].items():
        path = Path(g['private'])/'checkpoint'/name
        need(meta(path)['sha256'] == digest, 'new backup bytes differ')
        backups[name] = meta(path)
    before = load(folder/'rehearsal/immutable-before.json')
    after = load(folder/'rehearsal/immutable-after.json')
    need(set(before) == set(after) and len(before) == 20, 'old fact table membership differs')
    need(all(all(after[table].get(digest, 0) >= count for digest, count in rows.items()) for table, rows in before.items()), 'old immutable facts lost or rewritten')
    trial = load(folder/'rehearsal/result.json')
    closure = load(folder/'rehearsal/inspector-closure.json')
    need(trial['status'] == 'PASS_ISOLATED_NONDELETE_COMMANDS' and trial['native_delete'] == trial['source_business_mutations'] == trial['native_object_put'] == 0 and trial['full_mixed_outcomes'] == 'NOT_RUN', 'trial boundary differs')
    need(closure['disabled_readback'] == 'FRESH_CONFIRMED_DISABLED' and closure['failure'] is None and closure['source_identity_enabled'] is False, 'restore inspector not contained')
    replays = []
    native = []
    for role in ('source', 'restore'):
        data = folder/('maintenance-' + role)
        command = [str(checker), '-six', '-baseline', str(folder/'expectation'), '-fresh', str(data/'files'),
                   '-group', group, '-expectation', str(data/'expectation.json'),
                   '-inventory', str(Path(g['private'])/'present-inventory.json'), '-mapping', str(private/'six-map.json')]
        run = subprocess.run(command, capture_output=True, timeout=45)
        for stream in ('stdout', 'stderr'):
            with (OUT/(group + '-' + role + '-checker.' + stream + '.txt')).open('xb') as target:
                target.write(getattr(run, stream))
        need(run.returncode == 0, 'file-only checker replay failed: ' + group + '/' + role)
        output = json.loads(run.stdout)
        need(output['status'] == 'PASS_COMPLETE_SIX_NATIVE_AGGREGATION', 'file-only checker not PASS')
        verify = load(data/'files/six-native-verification.json')
        facts = verify['verification']
        need(facts['delete'] == 0 and facts['mutations'] == 0, 'native verify mutation differs')
        absent = load(data/'files/six-uncreated-users.json')['users']
        need(all(v['status'] == 404 and v['code'] == 'XMinioAdminNoSuchUser' for v in absent.values()), 'uncreated user not absent')
        native.append({'role': role, 'copies': len(facts['copies']), 'known_disabled_users': len(facts['disabled_identities']),
                       'absent_users': len(absent), 'p1_absent': verify['p1_absence'], 'checker_status': output['status']})
    for role in ('maintenance-source', 'maintenance-restore', 'maintenance-provision', 'restore-inspect-enable', 'restore-inspect-disable'):
        data = folder/role/'files'
        prefix = 'readonly' if role in ('maintenance-source', 'maintenance-restore') else 'prepare'
        starts = [load(x) for x in data.glob(prefix + '-start-*.json')]
        observed = [load(x) for x in data.glob(prefix + '-observation-*.json')]
        need(starts and len(starts) == len(observed) and len({x['id'] for x in starts}) == len(starts) and {x['id'] for x in starts} == {x['id'] for x in observed}, 'technical attempt membership differs')
        by_id = {x['id']: x for x in observed}
        need(all(not by_id[x['id']]['transport_error'] and 100 <= by_id[x['id']]['status'] <= 599 and x['at'] <= by_id[x['id']]['at'] for x in starts), 'technical observations missing/invalid')
        methods = dict(Counter(x['method'] for x in starts))
        need('DELETE' not in methods, 'DELETE seen in preparation journals')
        if prefix == 'readonly':
            need(set(methods) <= {'GET', 'HEAD'}, 'readonly transport mutation')
        else:
            need(all(x['path'].startswith('/minio/admin/') for x in starts), 'preparation object mutation')
        all_journals.append({'group': group, 'role': role, 'starts': len(starts), 'observations': len(observed),
                             'methods': methods, 'http_statuses': dict(Counter(str(x['status']) for x in observed)), 'amount': 'NOT_ASSESSED'})
    groups.append({'group': group, 'Core_fingerprints_equal': equality, 'three_Core_roles_distinct': True,
                   'new_backups': backups, 'old_immutable_tables': len(before),
                   'old_rows_multiset_preserved': True, 'trial_facts': load(folder/'rehearsal/facts.json'), 'native': native})

need(sum(g['native'][0]['copies'] for g in groups) == 17, '17 version count differs')
need(sum(g['native'][0]['known_disabled_users'] for g in groups) == 35, '35 IAM count differs')
need(sum(g['native'][0]['absent_users'] for g in groups) == 9, '9 absent IAM count differs')
save('preparation-file-analysis01.json', {'status': 'PASS_INDEPENDENT_C18_PREPARATION_FILE_ANALYSIS', 'at': dt.datetime.now(dt.timezone.utc).isoformat(),
     'execution': allocation['execution'], 'source_head': allocation['source'], 'source_preparation_index': verified,
     'groups': groups, 'technical_journals': all_journals, 'technical_requests': sum(x['starts'] for x in all_journals),
     'source_restore_read_requests': sum(x['starts'] for x in all_journals if x['role'] in ('maintenance-source', 'maintenance-restore')),
     'terminal': meta(PREP/'non-delete-terminal.json'), 'budget_initialization': meta(PREP/'budget-initialization.json'),
     'actual_native_calls_by_reviewer': 0, 'reviewer_file_only_checker_replays': 6, 'reviewer_DELETE': 0,
     'cost_note': 'Technical request quantities and starts/observations are retained. Amount remains NOT_ASSESSED; provider_io=0 in the aggregate is not a claim that technical HTTP calls or their costs did not occur.',
     'remaining': ['Frozen packet, full-entry fresh receipt, confirmation/runbook and final physical closure are pending separate review.'],
     'native_mixed_outcomes': 'NOT_RUN'})
print(json.dumps({'status': 'PASS_INDEPENDENT_C18_PREPARATION_FILE_ANALYSIS', 'groups': 3, 'checker_replays': 6,
                  'technical_requests': sum(x['starts'] for x in all_journals), 'source_restore_read_requests': sum(x['starts'] for x in all_journals if x['role'] in ('maintenance-source', 'maintenance-restore')),
                  'report': meta(OUT/'preparation-file-analysis01.json')}))
