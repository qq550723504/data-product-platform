"""Read back the created heartbeat; append receipts without runtime operations."""
import hashlib
import json
import sqlite3
import subprocess
import tomllib
from datetime import datetime, timedelta, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[6]
PUBLIC = ROOT / 'docs/acceptance/evidence/issue307-d1/delete-confirmed-v25-execution01'
PRIVATE = Path('C:/Users/Henry/.codex/fixture-private/remaining-candidate15/authorized-execution01')
AUTO = '307-d1-candidate15-v25'
NOW = datetime.now(timezone.utc).isoformat()


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
need(sha(auth_raw) == '55f0be454529612e1d81c47c68f10542929f96d661be675d50aea32fe4029b9e', 'authorization drift')
auth = json.loads(auth_raw)
consent_raw = (PUBLIC / 'schedule-human-authority.json').read_bytes()
need(sha(consent_raw) == '65b68dd6c3f7f44dfec9d99e201deeb91e0d953c70bb55ec7ff0a7e13bdc4774', 'scheduling consent drift')
for member in auth['approved_reference_artifacts']:
    raw = (ROOT / member['path']).read_bytes()
    need(sha(raw) == member['sha256'] and len(raw) == member['bytes'], 'approved member drift: ' + member['path'])
head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT).decode('ascii').strip()
need(head == auth['final_approved_head'], 'HEAD drift')
need(not subprocess.check_output(['git', 'diff', '--name-only', 'HEAD', '--'], cwd=ROOT).strip(), 'tracked change')
need(datetime.now(timezone.utc) < datetime(2026, 10, 20, 6, 0, tzinfo=timezone.utc), 'prewindow no longer true')

config_path = Path('C:/Users/Henry/.codex/automations') / AUTO / 'automation.toml'
config_raw = config_path.read_bytes()
config = tomllib.loads(config_raw.decode('utf-8'))
proposed = (PUBLIC / 'scheduled-prompt.txt').read_bytes()
actual_prompt = config['prompt'].encode('utf-8')
need(proposed == actual_prompt + b'\n', 'tool prompt transformation beyond one final LF')
need(sha(actual_prompt) == '9200bc1c93375483b012948a1fb7c5db21fd00ecadfef2a2b4f33f427bdfb4eb', 'actual prompt drift')
need(config['kind'] == 'heartbeat' and config['status'] == 'ACTIVE' and config['target_thread_id'] == auth['thread_id'], 'schedule scope/status differs')

db = Path('C:/Users/Henry/.codex/sqlite/codex-dev.db')
connection = sqlite3.connect(db.as_uri() + '?mode=ro', uri=True)
connection.row_factory = sqlite3.Row
try:
    rows = connection.execute('SELECT id,name,status,next_run_at,next_run_nominal_at,last_run_at,rrule,kind,target_thread_id,created_at,updated_at,model,reasoning_effort,notification_policy,prompt FROM automations WHERE id=?', (AUTO,)).fetchall()
    need(len(rows) == 1, 'automation identity not unique')
    row = dict(rows[0])
    need(row.pop('prompt').encode('utf-8') == actual_prompt, 'DB and TOML prompt differ')
    for key in ('id', 'name', 'status', 'rrule', 'kind', 'target_thread_id', 'created_at', 'updated_at'):
        need(row[key] == config[key], 'DB and TOML differ: ' + key)
    need(row['next_run_at'] == row['next_run_nominal_at'] == 1792476000000, 'wrong fixed-date time')
    need(row['model'] is None and row['reasoning_effort'] is None, 'unrequested model override')
    need(row['last_run_at'] is None, 'unexpected prior run')
    runs = connection.execute('SELECT count(*) FROM automation_runs WHERE automation_id=?', (AUTO,)).fetchone()[0]
    need(runs == 0, 'unexpected prior scheduler run')
    duplicates = connection.execute('SELECT id FROM automations WHERE prompt LIKE ?', ('%9f9c67d7-8a09-45a8-897f-9e3077b60980%',)).fetchall()
    need([r['id'] for r in duplicates] == [AUTO], 'duplicate execution schedule')
    old = {}
    for name in ('307-d1-candidate07-v17', '307-d1-candidate10-v20', '307-d1-candidate11-v21', '307-d1-candidate12-v22'):
        old[name] = connection.execute('SELECT status FROM automations WHERE id=?', (name,)).fetchone()[0]
        need(old[name] == 'PAUSED', 'old automation active')
finally:
    connection.close()

next_run = datetime.fromtimestamp(row['next_run_at'] / 1000, timezone.utc)
receipt = {
    'schema_version': 1, 'recorded_at': NOW,
    'status': 'PASS_FIXED_WINDOW_HEARTBEAT_SCHEDULED',
    'automation_id': AUTO, 'execution': auth['execution'],
    'expected_head': head, 'automation': row,
    'next_run_utc': next_run.isoformat(),
    'next_run_singapore': next_run.astimezone(timezone(timedelta(hours=8))).isoformat(),
    'window_end_exclusive_utc': auth['not_after'],
    'readonly_recovery_end_utc': auth['readonly_recovery_not_after'],
    'timezone': 'Asia/Singapore', 'host_timezone': 'Singapore Standard Time',
    'saved_config_sha256': sha(config_raw),
    'actual_prompt_sha256': sha(actual_prompt), 'actual_prompt_utf8_bytes': len(actual_prompt),
    'submitted_prompt_sha256': sha(proposed), 'submitted_prompt_utf8_bytes': len(proposed),
    'prompt_change_only_final_LF_omission': True,
    'human_original_and_approved_machine_template_unchanged': True,
    'authorization_sha256': sha(auth_raw), 'scheduling_consent_sha256': sha(consent_raw),
    'approved_reference_artifacts_verified': len(auth['approved_reference_artifacts']),
    'old_heartbeats': old, 'automation_count_for_execution': len(duplicates), 'scheduler_runs': runs,
    'deletion_window_currently_open': False,
    'source_starts_this_authorization_schedule_turn': 0,
    'provider_IO_this_authorization_schedule_turn': 0,
    'permission_enable_proof_sign_DELETE_this_authorization_schedule_turn': 0,
    'worker_human_confirmation_materialized': False,
    'remaining_in_window_fresh_checks': 'NOT_RUN', 'full_D1': 'NOT_ACCEPTED',
    'terminal_policy': 'Pause this same heartbeat on completion, failure or expiry; never shift or reopen the fixed date window.',
    'PR332': 'DRAFT_OPEN', 'Issue307': 'OPEN',
    'verification_sources': ['purpose-built automation create and view', 'actual saved automation TOML', 'read-only SQLite schedule row and runs count'],
    'local_requirements': 'Computer and desktop app must be running; Docker Desktop Linux engine and retained resources must remain available. This is a scheduled attempt, not proof that future preconditions will pass.',
}
for base in (PUBLIC, PRIVATE):
    create(base / 'scheduled-automation-snapshot.toml', config_raw)
    create(base / 'scheduled-prompt-actual.txt', actual_prompt)
    create(base / 'scheduled-resume.json', encoded(receipt))

check_raw = (PUBLIC / 'composition/authorized-prewindow-check.json').read_bytes()
check = json.loads(check_raw)
need(check['status'] == 'PASS_PREWINDOW_CLOSED', 'prewindow check not passed')
github_raw = (PUBLIC / 'github-checkpoint.json').read_bytes()
github = json.loads(github_raw)
need(github['status'] == 'PASS_AUTHORIZATION_HEAD_CI_REVIEW_READBACK', 'GitHub checkpoint not passed')

matrix = f'''# #307 D1 candidate15/v25 execution01

本代新的人类删除授权和单独固定窗口调度授权均已记录；execution `{auth['execution']}`。原准备快照、独立评审和冻结手册的 NOT_RECEIVED/FROZEN_NOT_AUTHORIZED 是当时事实，原字节保留。本文件只追加后来的授权和调度事实，不改写历史，不放宽执行门禁。

| 项目 | 当前证据与状态 |
| --- | --- |
| 精确删除授权 | [实际人类按引用授权](evidence/issue307-d1/delete-confirmed-v25-execution01/human-authorization.json)，原文含末尾 LF，共432 UTF8字节；user_retyped_template=false；56项批准引用实际字节匹配 |
| 独立调度授权 | [单独人类回答](evidence/issue307-d1/delete-confirmed-v25-execution01/schedule-human-authority.json)，只绑定同一 execution、日期和窗口 |
| 实际调度 | [创建与读回](evidence/issue307-d1/delete-confirmed-v25-execution01/scheduled-resume.json)，同一聊天 heartbeat `{AUTO}`，ACTIVE；SQLite实际 next_run_at={row['next_run_at']}，UTC2026-10-20 06:00 / 新加坡2026-10-20 14:00；仅一次，scheduler runs=0 |
| 窗口和预算 | UTC06:00≤now<06:40；六目标各最多一次、全局最多六次、UNKNOWN不重发；失败/过窗封存，不延长/平移/重开 |
| 固定候选 | candidate15/v25；运行 source f6acb072db9881e7b1cfd72003d4ef616955947b；原 candidate/template/bindings/source/binary/全部配置不变 |
| 最终 HEAD / PR CI | `{head}`；PR CI39320908916，13/13 SUCCESS，event=pull_request；[授权后实时读回](evidence/issue307-d1/delete-confirmed-v25-execution01/github-checkpoint.json)；窗口前须再次实时复核，不能当 main CI |
| 窗口前关闭核验 | [固定Host.check](evidence/issue307-d1/delete-confirmed-v25-execution01/composition/authorized-prewindow-check.json) PASS_PREWINDOW_CLOSED；86本代owner停止、30 permission false、六case/六recovery及九新/九旧备份、三空预算正确；896次只读进程，无启动/exec/启用/签名/Source/provider/DELETE |
| 最终独立评审 | [独立07](evidence/issue307-d1/post-v22-policy-fix01/independent-frozen-confirmation-review07.json) PASS_BOUNDED_INDEPENDENT_FROZEN_CONFIRMATION_REVIEW，remaining_blockers=[]；不替代人类授权或窗口内 fresh 前置 |
| 新鲜完整入口 | NOT_RUN；开窗后固定 verify-six-native.py 新O_EXCL输出；检查17现存版本、完整IAM/保护/拓扑/预算/三源Core及备份，自动关闭后先 Host.check 再启动 |
| 历史 P1 | 原v22已记录一次DELETE/204/absence；永久排除本代六目标和预算，不重跑 |
| H0/H1 / H2 / M1 | 本代live结果 NOT_RUN；H0/H1零发送；H2真实204之后才晚hold/revoke/响应丢失；M1 checkpoint复制并核SHA后才B只读lookup |
| 只读恢复 | NOT_RUN；逐份send→禁用send→选定Outbox→既有Asynq→提交Core immutable absence→下一份；独立只读期限UTC08:40（新加坡16:40），不得重开身份或延长发送 |
| 本代实际DELETE | 0；六个slot全部NOT_RUN，不预先接受完整D1 |
| 保留边界 | 十一survivor、其他环境、全部新旧备份、旧pins、新旧预算、所有失败/UNKNOWN/attempt/成本保留；cleanup=0、hold-release=0、bypass=0 |
| PR / Issue | #332 OPEN/Draft，不Ready、不合并；#307 OPEN；#323已按独立历史授权合并，不回写 |

六个一次性slot的唯一顺序为 P2→T1→T2→H2A→M1A→M1B；当前均为NOT_RUN。exact环境、key/version、copy/DV、plan/hash和十一保留版本以[完整精确确认文本](issue307-d1-candidate15-confirmation.md)及[固定手册](issue307-d1-v25-window-runbook.md)为准。

调度工具只省略了提交prompt末尾一个LF。提交原字节与实际保存prompt分别留档并记录不同SHA；实际人类原文和批准机器模板未变。创建工具、TOML和SQLite三方读回一致，旧07/10/11/12调度均PAUSED，没有重复本execution调度。本地电脑、桌面app和Docker Linux引擎须可用；调度成功不保证未来新鲜前置通过。

任何非预期失败立即停止新增发送，使用固定window_six_support.py seal逐组关闭；一项失败继续其他关闭。无fresh IAM读回、enabler未闭合或归档不完整时保留UNCONFIRMED，不报告整轮PASS。完成、失败或过期后暂停同一heartbeat，已有terminal只读报告，不重复发起。
'''.encode('utf-8')
matrix_path = ROOT / 'docs/acceptance/issue307-d1-v25-execution01.md'
create(matrix_path, matrix)

members = {}
for path in sorted(PUBLIC.rglob('*')):
    if path.is_file():
        raw = path.read_bytes()
        members[path.relative_to(ROOT).as_posix()] = {'sha256': sha(raw), 'bytes': len(raw)}
index = {'status': 'PASS_AUTHORIZATION_AND_SCHEDULE_EVIDENCE_INDEX', 'recorded_at': NOW, 'execution': auth['execution'], 'members': members, 'excludes': ['this index itself', 'final checkpoint', 'append-only current execution matrix']}
index_raw = encoded(index)
create(PUBLIC / 'schedule-evidence-index.json', index_raw)
final = {
    'status': 'PASS_AUTHORIZED_SCHEDULED_PREWINDOW_CLOSED', 'recorded_at': NOW,
    'execution': auth['execution'], 'automation_id': AUTO,
    'head': head, 'schedule_evidence_index_sha256': sha(index_raw),
    'execution_matrix_sha256': sha(matrix), 'scheduled_resume_sha256': sha(encoded(receipt)),
    'authorization_sha256': sha(auth_raw), 'separate_schedule_authority_sha256': sha(consent_raw),
    'prewindow_check_sha256': sha(check_raw), 'github_checkpoint_sha256': sha(github_raw),
    'current_frozen_approved_artifacts_checked': len(auth['approved_reference_artifacts']),
    'current_in_window_preflight': 'NOT_RUN', 'actual_six_target_delete': 'NOT_RUN',
    'remaining_six': {key: 'NOT_RUN' for key in ('P2', 'T1', 'T2', 'H2A', 'M1A', 'M1B')},
    'current_source_provider_delete_enable_sign_actions': 0,
    'full_D1': 'NOT_ACCEPTED', 'PR332': 'DRAFT_OPEN', 'Issue307': 'OPEN',
}
for base in (PUBLIC, PRIVATE):
    create(base / 'prewindow-final-checkpoint.json', encoded(final))
print(json.dumps({'status': final['status'], 'next_run_UTC': receipt['next_run_utc'], 'next_run_SGT': receipt['next_run_singapore'], 'actual_prompt_sha256': sha(actual_prompt), 'schedule_receipt_sha256': sha(encoded(receipt)), 'index_members': len(members), 'matrix_sha256': sha(matrix)}, ensure_ascii=False))
