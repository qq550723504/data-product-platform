"""Append exact human schedule authority; file/Git reads and O_EXCL evidence only."""
import hashlib
import json
import subprocess
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[6]
PUBLIC = ROOT / 'docs/acceptance/evidence/issue307-d1/delete-confirmed-v25-execution01'
PRIVATE = Path('C:/Users/Henry/.codex/fixture-private/remaining-candidate15/authorized-execution01')
LOG = Path('C:/Users/Henry/.codex/sessions/2026/10/13/rollout-2026-10-13T23-50-22-01a13579-573d-7a32-b1e1-3297fe78380a_01a135f5-2d3a-7953-8149-c86a26446d07.jsonl')
CALL = 'call_ed04bf5c942342fab116743d6d0eb7db'


def need(value, message):
    if not value:
        raise RuntimeError(message)


def digest(data):
    return hashlib.sha256(data).hexdigest()


def create(path, data):
    with path.open('xb') as stream:
        stream.write(data)


def encoded(value):
    return (json.dumps(value, ensure_ascii=False, indent=2) + '\n').encode('utf-8')


auth_bytes = (PUBLIC / 'human-authorization.json').read_bytes()
need(digest(auth_bytes) == '55f0be454529612e1d81c47c68f10542929f96d661be675d50aea32fe4029b9e', 'authorization bytes drift')
auth = json.loads(auth_bytes)
need(auth['execution'] == '9f9c67d7-8a09-45a8-897f-9e3077b60980', 'execution differs')
for member in auth['approved_reference_artifacts']:
    raw = (ROOT / member['path']).read_bytes()
    need(digest(raw) == member['sha256'] and len(raw) == member['bytes'], 'approved reference drift: ' + member['path'])
for name, expected in [
    ('user-message-verbatim.txt', auth['user_message_sha256']),
    ('approved-reference-manifest.json', auth['approved_reference_manifest_sha256']),
    ('authorization-envelope.txt', auth['authorization_envelope_sha256']),
    ('approved-referenced-template.txt', auth['referenced_template_sha256']),
    ('approved-host-bindings.txt', auth['additional_host_bindings_sha256']),
]:
    need(digest((PRIVATE / name).read_bytes()) == expected, 'private authorization drift: ' + name)
head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT).decode('ascii').strip()
need(head == auth['final_approved_head'], 'approved HEAD drift')
need(not subprocess.check_output(['git', 'diff', '--name-only', 'HEAD', '--'], cwd=ROOT).strip(), 'tracked changes')

matches = []
with LOG.open('rb') as stream:
    stream.seek(max(0, LOG.stat().st_size - 4000000))
    stream.readline()
    for raw in stream:
        try:
            record = json.loads(raw)
        except ValueError:
            continue
        payload = record.get('payload', {})
        if record.get('type') != 'response_item' or payload.get('role') != 'user':
            continue
        text = ''.join(c.get('text', '') for c in payload.get('content', []) if c.get('type') in ('input_text', 'text'))
        if CALL in text:
            matches.append((raw, record, payload, text))
need(len(matches) == 1, 'schedule authority message not unique')
raw_record, record, payload, text = matches[0]
inside = text.split('<send_user_message_question_reply>\n', 1)[1].rsplit('\n</send_user_message_question_reply>', 1)[0]
items = json.loads(inside)
need(len(items) == 1 and CALL in items[0]['questionItemId'], 'question binding differs')
need(items[0]['answer'] == '安排固定窗口续跑（推荐）', 'schedule not authorized')
need(auth['execution'] in items[0]['question'] and '2026年10月20日' in items[0]['question'] and '14:00–14:40' in items[0]['question'], 'schedule context differs')
need(payload['id'] == 'msg_01a156e1-1904-7b73-9ac5-90595b195274', 'schedule message identity differs')
message = text.encode('utf-8')
receipt = {
    'status': 'RECEIVED_SEPARATE_EXPLICIT_HUMAN_FIXED_WINDOW_SCHEDULE_AUTHORITY',
    'recorded_at': datetime.now(timezone.utc).isoformat(),
    'execution': auth['execution'], 'profile': auth['profile'],
    'thread_id': auth['thread_id'], 'human_message_id': payload['id'],
    'human_turn_id': payload['internal_chat_message_metadata_passthrough']['turn_id'],
    'human_message_timestamp': record['timestamp'],
    'question_call_id': CALL, 'question': items[0]['question'], 'answer': items[0]['answer'],
    'actual_UI_reply_message_sha256': digest(message), 'actual_UI_reply_message_utf8_bytes': len(message),
    'source_record_sha256': digest(raw_record),
    'deletion_authorization_sha256': digest(auth_bytes),
    'not_before': auth['not_before'], 'not_after_exclusive': auth['not_after'],
    'timezone': 'Asia/Singapore', 'single_fixed_date': '2026-10-20',
    'repeats_after_completion_failure_or_expiry': False,
    'scope_expansion': False, 'old_authorization_reused': False,
    'approved_reference_members_checked': len(auth['approved_reference_artifacts']),
    'current_approved_HEAD': head,
    'runtime_source_provider_enable_sign_delete_actions': 0,
    'historical_authorization_scheduler_NOT_RECEIVED_snapshot_overwritten': False,
    'schedule_creation': 'PENDING_TOOL_CREATE_AND_READBACK',
}
create(PRIVATE / 'schedule-user-message-verbatim.txt', message)
create(PRIVATE / 'schedule-user-source-record.jsonl', raw_record)
create(PRIVATE / 'schedule-human-authority.json', encoded(receipt))
create(PUBLIC / 'schedule-human-authority.json', encoded(receipt))
print(json.dumps({'status': receipt['status'], 'human_message_id': payload['id'], 'receipt_sha256': digest(encoded(receipt)), 'raw_reply_sha256': digest(message), 'approved_references': len(auth['approved_reference_artifacts'])}))
