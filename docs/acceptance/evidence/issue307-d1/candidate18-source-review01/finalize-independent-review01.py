"""Close the bounded independent source review and its explicit metadata index."""
import datetime as dt
import hashlib
import json
from pathlib import Path
import subprocess
import sys

REVIEW = Path(__file__).resolve().parent
ROOT = REVIEW.parents[4]
HEAD = '618f484061c3624ff2195c0b591e7d8e3d743ce1'

def raw_git(*args):
    return subprocess.check_output(['git', '-c', 'core.longpaths=true', *args], cwd=ROOT)

def meta(path):
    raw = path.read_bytes()
    return {'sha256': hashlib.sha256(raw).hexdigest(), 'bytes': len(raw)}

def put(name, value):
    with (REVIEW/name).open('x', encoding='utf-8', newline='\n') as target:
        json.dump(value, target, indent=2, ensure_ascii=False)
        target.write('\n')

if raw_git('rev-parse', 'HEAD').decode().strip() != HEAD or raw_git('status', '--porcelain', '--untracked-files=no').strip():
    raise ValueError('exact source HEAD or tracked state differs')
subprocess.run(['git', '-c', 'core.longpaths=true', 'diff', '--check', 'HEAD^', 'HEAD'], cwd=ROOT, check=True)
regression = json.loads((REVIEW/'independent-regression01/result.json').read_bytes())
if regression['status'] != 'PASS_INDEPENDENT_SOURCE_ONLY_REGRESSION' or regression['source_head'] != HEAD:
    raise ValueError('independent regression did not pass this source')
author = json.loads((REVIEW/'author-regression01/result.json').read_bytes())
if author['status'] != 'FAIL' or [row['index'] for row in author['failed']] != [56, 57]:
    raise ValueError('author failures differ from the preserved known missing-packet tests')
source = {}
for name in raw_git('diff-tree', '--no-commit-id', '--name-only', '-r', 'HEAD').decode().splitlines():
    raw = (ROOT/name).read_bytes()
    committed = raw_git('show', HEAD + ':' + name)
    if raw.replace(b'\r\n', b'\n') != committed.replace(b'\r\n', b'\n'):
        raise ValueError('reviewed source differs from commit: ' + name)
    source[name] = {**meta(ROOT/name), 'git_blob_sha256': hashlib.sha256(committed).hexdigest(),
                    'lf_sha256': hashlib.sha256(raw.replace(b'\r\n', b'\n')).hexdigest()}

planning = ROOT/'docs/acceptance/evidence/issue307-d1/candidate18-planning01/preallocation02.json'
planning_value = json.loads(planning.read_bytes())
if planning_value['status'] != 'PASS_RETAINED_623_AND_NEW_SUBNETS_NO_ALLOCATION' or planning_value['retained_CIDs'] != 623:
    raise ValueError('author preallocation receipt differs')
inputs = {
    'status': 'RECORDED_C18_INDEPENDENT_SOURCE_REVIEW_INPUTS',
    'source_head': HEAD,
    'source_files': source,
    'author_preallocation_receipt': {'path': planning.relative_to(ROOT).as_posix(), **meta(planning), 'reported': planning_value},
    'author_regression': {**meta(REVIEW/'author-regression01/result.json'), 'status': author['status'], 'commands': author['commands'],
                          'passed_commands': 59, 'failed_commands': author['failed']},
    'author_go_stdout': meta(REVIEW/'go-packages.stdout.txt'),
    'author_go_stderr': meta(REVIEW/'go-packages.stderr.txt'),
}
put('independent-review-inputs01.json', inputs)

report = {
    'status': 'PASS_BOUNDED_C18_SOURCE_PREPARATION_REVIEW',
    'at': dt.datetime.now(dt.timezone.utc).isoformat(),
    'reviewer': '/root/c18_independent_review',
    'source_head': HEAD,
    'profile': 'candidate18/v28',
    'execution': '4fd5a7e7-779e-41d7-a35a-3bab7dd615b5',
    'scope': 'Committed source preparation only; independent file/model review without Docker, database or provider I/O.',
    'remaining_blockers': [],
    'proposed_window': {'not_before': '2026-10-21T14:00:00Z', 'not_after': '2026-10-21T14:40:00Z',
                        'readonly_until': '2026-10-21T16:40:00Z', 'human_authorization': 'NOT_RECEIVED', 'schedule': 'NOT_CREATED'},
    'checks': [
        {'status': 'PASS', 'subject': 'Generation and authority separation',
         'finding': 'Go execution/profile/window constants select only c18. Current root IAM selected() rejects retained c17 users; maintenance authorization path is v28. Validating the original v27 authorization preserves historical facts and does not grant new authority.'},
        {'status': 'PASS', 'subject': 'Historical membership and failure preservation',
         'finding': 'retained_v27 pins original 826 references, 771 failed-window members, 348 prior committed read members, actual revoked 30/6/6 profiles, 531 fixed old control/backup files and 623 CIDs. Old folders are derived from pinned membership; adding c18 does not expand the historical set, while old-folder changes or expansion reject. Original failure remains FAILED_TOPOLOGY_ATTESTATION_SEALED_NO_DELETE.'},
        {'status': 'PASS', 'subject': 'Old budget identity and mount closure',
         'finding': 'Original c17 volumes require exact Name/local Driver/local Scope/no Options, old execution and allocation nonce. Complete old mounted-CID sets reject missing, duplicate or extra owners. All 18 old budget tar digests remain compared rather than reset.'},
        {'status': 'PASS', 'subject': 'Current Core versus immutable policy history',
         'finding': 'Source fingerprint equality to the pinned post-closure business Core precedes PROVISION. Late/mixed source baselines remain pinned. The aab0 policy checkpoint and five existing heads/expired deadlines remain historical bytes; no Renew or source Processing/DirectData rerun is introduced.',
         'business_Core_sha256': 'ffdf75013a393da6a8444b28284fdfcc47b5a01a346f7fdf25a8c03138cd8a7f',
         'policy_checkpoint_sha256': 'aab0e61530bee63429a15ce635880a6d1db286cb744767f7d9fdb50082313df8'},
        {'status': 'PASS', 'subject': 'Non-delete native preparation and trial boundary',
         'finding': 'Source native maintenance creates only the six current disabled identities. Trial commands use the new rehearsal Core, separate restore native/gateway, GET/HEAD-only access and old-row multiset checks; source and clean-restore Core fingerprints are unchanged. Existing Asynq/Outbox and official policy ParseConfig/Equals remain reused.'},
        {'status': 'PASS', 'subject': 'Window, profiles, types and complete entry',
         'finding': 'Current cases bind the new execution and unified END for ExpiresAt/CandidateUseUntil; recovery uses END+2h. Success plus cross-field rehash/extension, bytes/str, topology, budget and explicit files-map/members-list counterexamples pass in normal and optimized Python.'},
        {'status': 'PASS', 'subject': 'New allocation collision avoidance',
         'finding': 'Source uses fresh subnets 10.246.212/213/214.0/28 and existing overlap rejection. The author preallocation receipt reports 623 closed old owners and no subnet overlap; this review reads that receipt and does not independently inspect Docker.'},
    ],
    'independent_regression': {'result': 'independent-regression01/result.json', **meta(REVIEW/'independent-regression01/result.json'),
                               'commands': 25, 'python_methods_normal': 66, 'python_methods_optimized': 66,
                               'go_selected_packages': 3, 'status': regression['status']},
    'author_regression': {'status': 'FAIL', 'commands': 61, 'passed_commands': 59,
                          'preserved_failures': [56, 57], 'cause': 'test_attest_remaining_executable normal/-O requires candidate18/candidate.json, which is not frozen yet.'},
    'author_go': {'reported_packages_count_1_passed': 5, 'independent_selected_packages_count_1_passed': 3},
    'post_freeze_required': [
        'Run test_attest_remaining_executable.py normal and -O against the actual frozen packet; preserve this earlier FAIL receipt.',
        'Review the actual frozen candidate/bindings/template/source/build/profile/budget/Core/IAM/gateway/native evidence before claiming frozen non-delete acceptance.',
        'Fresh human delete authorization and separate future schedule authority remain absent; preparation and this review do not grant either.'
    ],
    'limits': {'frozen_packet_accepted': False, 'native_acceptance_by_this_reviewer': 'NOT_RUN', 'real_DELETE': 0,
               'docker_calls': 0, 'database_calls': 0, 'provider_calls': 0, 'authorization_reused': False,
               'full_suite_claimed_pass': False, 'D1_complete': False, 'PR332': 'REQUIRED_DRAFT_UNMERGED', 'Issue307': 'REQUIRED_OPEN'},
    'inputs': {'path': 'independent-review-inputs01.json', **meta(REVIEW/'independent-review-inputs01.json')},
}
put('independent-final-review01.json', report)

# All selected inputs/results have reached a terminal state. Do not index future
# packet/matrix/current paths, this index itself, or the executing finalizer.
members = [REVIEW/'merge-contract.md', REVIEW/'independent-review-inputs01.json', REVIEW/'independent-final-review01.json',
           REVIEW/'go-packages.stdout.txt', REVIEW/'go-packages.stderr.txt']
members += sorted((REVIEW/'author-regression01').glob('*'))
members += sorted((REVIEW/'independent-regression01').glob('*'))
files = {path.relative_to(REVIEW).as_posix(): meta(path) for path in members if path.is_file()}
put('evidence-index.json', {'status': 'C18_SOURCE_REVIEW_EVIDENCE_INDEX', 'count': len(files), 'files': files})

sys.path.insert(0, str(ROOT/'examples/lifecycle-d1'))
import evidence_indexes as audit
index_relative = (REVIEW/'evidence-index.json').relative_to(ROOT).as_posix()
verified = audit.audit_index(ROOT, {'id': 'c18-independent-source-review', 'index': index_relative,
    'sha256': meta(REVIEW/'evidence-index.json')['sha256'], 'format': 'files-map-v1',
    'member_root': REVIEW.relative_to(ROOT).as_posix(), 'expected_count': len(files),
    'expected_status': 'C18_SOURCE_REVIEW_EVIDENCE_INDEX'})
print(json.dumps({'report': meta(REVIEW/'independent-final-review01.json'), 'index': meta(REVIEW/'evidence-index.json'), 'audit': verified}))
