"""File/Git/read-only scheduler audit; append a blocked prewindow terminal."""
import hashlib
import json
import sqlite3
import subprocess
import tomllib
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[6]
PUBLIC = ROOT / 'docs/acceptance/evidence/issue307-d1/delete-confirmed-v25-execution01'
OUT = PUBLIC / 'review-preparation01'
REVIEW = ROOT / 'docs/acceptance/evidence/issue307-d1/delete-confirmed-v25-review01'
PRIVATE = Path('C:/Users/Henry/.codex/fixture-private/remaining-candidate15/authorized-execution01')
REPAIR = Path('C:/Users/Henry/.codex/worktrees/d1-cli-fix/data-product-platform')


def need(value, message):
    if not value:
        raise RuntimeError(message)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def encoded(value):
    return (json.dumps(value, ensure_ascii=False, indent=2) + '\n').encode('utf-8')


def create(path, raw):
    with path.open('xb') as stream:
        stream.write(raw)


auth_raw = (PUBLIC / 'human-authorization.json').read_bytes()
need(sha(auth_raw) == '55f0be454529612e1d81c47c68f10542929f96d661be675d50aea32fe4029b9e', 'original authorization changed')
auth = json.loads(auth_raw)
for member in auth['approved_reference_artifacts']:
    raw = (ROOT / member['path']).read_bytes()
    need(sha(raw) == member['sha256'] and len(raw) == member['bytes'], 'frozen member changed: ' + member['path'])
need(sha((PRIVATE / 'user-message-verbatim.txt').read_bytes()) == auth['user_message_sha256'], 'human original changed')
need(sha((PRIVATE / 'authorization-envelope.txt').read_bytes()) == auth['authorization_envelope_sha256'], 'authorization envelope changed')
need(subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT).decode().strip() == auth['final_approved_head'], 'frozen checkout changed HEAD')
need(not subprocess.check_output(['git', '-c', 'core.longpaths=true', 'diff', '--name-only', 'HEAD', '--'], cwd=ROOT).strip(), 'frozen tracked file changed')

blocked_path = REVIEW / 'independent-authorization-window-review01.json'
repair_path = REVIEW / 'independent-isolated-cli-repair-addendum02.json'
need(sha(blocked_path.read_bytes()) == 'b0a4ef12e4b8604c2b18228ef1ce242b53c618df3dff56d4771b305d02c74331', 'BLOCKED review drift')
need(sha(repair_path.read_bytes()) == '966dd18c814bf6890ced56a3b81b4d7c807436089d15ae026f8c3853dd27acad', 'repair review drift')
blocked, repaired = json.loads(blocked_path.read_bytes()), json.loads(repair_path.read_bytes())
need(blocked['status'] == 'BLOCKED_BOUNDED_INDEPENDENT_AUTHORIZATION_WINDOW_REVIEW' and len(blocked['remaining_blockers']) == 1, 'wrong frozen review result')
need(repaired['status'] == 'PASS_BOUNDED_INDEPENDENT_ISOLATED_HOST_CLI_REPAIR_REVIEW' and repaired['remaining_blockers'] == [], 'repair not independently passed')
for name in ('independent-evidence-index01.json', 'independent-repair-evidence-index02.json'):
    index = json.loads((REVIEW / name).read_bytes())
    for path, entry in index['members'].items():
        raw = (REVIEW / path).read_bytes()
        need(sha(raw) == entry['sha256'] and len(raw) == entry['bytes'], 'review index drift: ' + path)

repair_head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=REPAIR).decode().strip()
need(repair_head == '8a172cade7b5eab2d871fc75ac9d0963f9d27199', 'repair HEAD changed')
need(not subprocess.check_output(['git', '-c', 'core.longpaths=true', 'status', '--porcelain'], cwd=REPAIR).strip(), 'repair checkout is dirty')
publication = json.loads((OUT / 'repair-publication-checkpoint.json').read_bytes())
checks, run, pr = publication['checks'], publication['run'], publication['pull_request']
need(checks['headRevision'] == repair_head and len(checks['checks']) == 13 and not checks['hasMore'], 'wrong CI HEAD/count')
need(all(x['head_sha'] == repair_head and x['status'] == 'completed' and x['conclusion'] == 'success' for x in checks['checks']), 'CI incomplete or failed')
need(run['head_sha'] == repair_head and run['event'] == 'pull_request' and run['status'] == 'completed' and run['conclusion'] == 'success', 'run binding failed')
need(pr['head_sha'] == repair_head and pr['draft'] and not pr['merged'] and pr['state'] == 'open' and pr['body_exact'], 'PR boundary/body mismatch')
thread_issue = json.loads((OUT / 'repair-review-threads-and-issue.json').read_bytes())
need(thread_issue['issue']['status'] == 'open' and thread_issue['threads']['status'] == 'fulfilled' and thread_issue['threads']['value']['structuredContent']['review_threads'] == [], 'Issue/thread drift')

config_raw = Path('C:/Users/Henry/.codex/automations/307-d1-candidate15-v25/automation.toml').read_bytes()
config = tomllib.loads(config_raw.decode('utf-8'))
need(config['status'] == 'PAUSED' and config['prompt'].encode() == (PUBLIC / 'scheduled-prompt-actual.txt').read_bytes(), 'paused schedule changed scope/prompt')
db = Path('C:/Users/Henry/.codex/sqlite/codex-dev.db')
connection = sqlite3.connect(db.as_uri() + '?mode=ro', uri=True)
try:
    row = connection.execute('SELECT status,next_run_at,next_run_nominal_at FROM automations WHERE id=?', (config['id'],)).fetchone()
    runs = connection.execute('SELECT count(*) FROM automation_runs WHERE automation_id=?', (config['id'],)).fetchone()[0]
finally:
    connection.close()
need(row == ('PAUSED', None, None) and runs == 0, 'scheduler pause not confirmed')

at = datetime.now(timezone.utc).isoformat()
terminal = {
    'status': 'BLOCKED_PREWINDOW_REVIEW', 'at': at, 'execution': auth['execution'],
    'kind': 'operator_execution_control_terminal_not_Core_business_fact',
    'profile': auth['profile'], 'window_lifecycle_started': False,
    'original_authorization_sha256': sha(auth_raw),
    'original_frozen_HEAD': auth['final_approved_head'],
    'frozen_host_sha256': '18eb0e8eb022a328d37908d0423a5289b843e5b608ed3437bb06b9c178c46542',
    'original_frozen_review': blocked['status'], 'original_frozen_blockers': ['B1'],
    'development_repair_HEAD': repair_head,
    'development_repair_review': repaired['status'], 'development_repair_blockers': [],
    'repair_activated_for_original_execution': False, 'old_authority_reused': False,
    'automation_id': config['id'], 'automation_status': row[0],
    'next_run_at': row[1], 'next_run_nominal_at': row[2], 'scheduler_runs': runs,
    'physical_preexecution_containment': 'independent inspect93 stopped; root contract15; permission30 false',
    'fresh_native_identity_readback_this_review': 'NOT_RUN', 'real_seal_this_review': 'NOT_RUN',
    'scope': 'No Source/Core/helper/enable/sender/wire started during this review; existing retained state preserved.',
    'six_slots': {key: 'NOT_RUN' for key in ('P2', 'T1', 'T2', 'H2A', 'M1A', 'M1B')},
    'actual_DELETE_this_review': 0, 'source_provider_enable_proof_actions_this_review': 0,
    'full_D1': 'NOT_ACCEPTED', 'cleanup': 0, 'hold_release': 0, 'bypass': 0,
    'backups_budgets_pins_failure_unknown_attempt_cost_facts': 'RETAINED',
    'next_execution_requirement': 'new exact frozen bindings/proposal and renewed human authorization; no old execution resume, window shift or reopen',
    'PR332': 'OPEN_DRAFT_UNMERGED', 'Issue307': 'OPEN',
}
terminal_raw = encoded(terminal)
create(PUBLIC / 'review-blocked-terminal.json', terminal_raw)
create(PRIVATE / 'terminal.json', terminal_raw)

members = {}
for directory in (OUT, REVIEW):
    for path in sorted(directory.rglob('*')):
        if path.is_file():
            raw = path.read_bytes()
            members[path.relative_to(ROOT).as_posix()] = {'sha256': sha(raw), 'bytes': len(raw)}
members[(PUBLIC / 'review-blocked-terminal.json').relative_to(ROOT).as_posix()] = {'sha256': sha(terminal_raw), 'bytes': len(terminal_raw)}
index_raw = encoded({'status': 'REVIEW_CLOSEOUT_EVIDENCE_INDEX', 'at': at, 'execution': auth['execution'], 'members': members, 'excludes': ['this index', 'final checkpoint', 'editable current matrix']})
create(PUBLIC / 'review-closeout-evidence-index.json', index_raw)
matrix_raw = (ROOT / 'docs/acceptance/issue307-d1-v25-execution01.md').read_bytes()
final = {
    'status': 'DEVELOPMENT_REPAIR_PASS_FROZEN_V25_BLOCKED_PAUSED', 'at': at,
    'execution': auth['execution'], 'frozen_HEAD': auth['final_approved_head'], 'development_HEAD': repair_head,
    'frozen_approved_members_unchanged': len(auth['approved_reference_artifacts']),
    'independent_BLOCKED_report_sha256': sha(blocked_path.read_bytes()),
    'independent_repair_PASS_report_sha256': sha(repair_path.read_bytes()),
    'new_HEAD_PR_CI': {'run_id': run['id'], 'checks_success': 13, 'event': run['event'], 'not_main_CI': True},
    'automation': {'id': config['id'], 'status': row[0], 'next_run_at': row[1], 'runs': runs, 'config_sha256': sha(config_raw)},
    'operator_terminal_sha256': sha(terminal_raw), 'review_closeout_index_sha256': sha(index_raw),
    'matrix_sha256': sha(matrix_raw), 'evidence_members': len(members),
    'new_candidate_execution_window_created': False, 'old_authorization_reused': False,
    'actual_delete_source_provider_enable_sign_this_review': 0,
    'full_D1': 'NOT_ACCEPTED', 'PR332': 'OPEN_DRAFT_UNMERGED', 'Issue307': 'OPEN',
}
for base in (PUBLIC, PRIVATE):
    create(base / 'review-final-checkpoint.json', encoded(final))
print(json.dumps(final, ensure_ascii=False))
