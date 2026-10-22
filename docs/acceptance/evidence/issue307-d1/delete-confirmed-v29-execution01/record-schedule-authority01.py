"""Record only this human's separate one-window schedule instruction."""
from pathlib import Path
import datetime as dt
import hashlib
import json
import os
import subprocess
import sys
ROOT=Path.cwd();OUT=Path(__file__).resolve().parent
CONTROL=Path(r'C:\Users\Henry\.codex\fixture-private\remaining-candidate19\authorized-execution01')
SESSION=Path(r'C:\Users\Henry\.codex\sessions\2026\10\13\rollout-2026-10-13T23-50-22-01a13579-573d-7a32-b1e1-3297fe78380a_01a135f5-2d3a-7953-8149-c86a26446d07.jsonl')
def need(ok,message):
    if not ok:raise ValueError(message)
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def digest(raw):return hashlib.sha256(raw).hexdigest()
def load(path):return json.loads(path.read_bytes())
def write(path,raw):
    with path.open('xb') as f:f.write(raw)
def save(path,value):write(path,(json.dumps(value,ensure_ascii=False,indent=2)+'\n').encode())
app=load(OUT/'actual-app-schedule-source01.json');a=load(OUT/'human-authorization.json')
message=None;turn=None
for raw in SESSION.open('rb'):
    event=json.loads(raw);p=event.get('payload',{})
    if event.get('type')=='turn_context':turn=p.get('turn_id')
    if event.get('type')=='response_item' and p.get('type')=='message' and p.get('id')=='msg_01a1618f-3c40-7513-879e-4807194274b2':
        need(message is None,'Duplicate human message')
        wrapper=''.join(x.get('text','') for x in p.get('content',[]) if isinstance(x,dict))
        message=(event,raw,wrapper,turn)
need(message is not None,'Actual human schedule instruction unavailable')
event,raw,wrapper,turn=message
need(event['payload']['role']=='user' and wrapper==app['wrapper_verbatim'] and turn==app['turn_id'] and event['timestamp']=='2026-10-22T03:02:19.456Z','Schedule source identity/bytes differ')
need(wrapper.startswith('<send_user_message_question_reply>\n') and wrapper.endswith('</send_user_message_question_reply>\n'),'Reply wrapper differs')
entries=json.loads(wrapper.split('>',1)[1].rsplit('</',1)[0]);need(len(entries)==1,'Schedule reply count differs')
entry=entries[0];question=entry['question'];answer=entry['answer']
need(entry['questionItemId']=='["request_user_input_async","call_5ecfd3aa49524fddad58f5bb0965dd04",0]' and answer=='安排固定窗口续跑（推荐）','Actual schedule response differs')
need(a['execution'] in question and 'candidate19/v29' in question and '2026年10月22日新加坡14:00' in question and '14:00–14:40' in question and '完成、失败或过期即停止' in question,'Question scope differs')
need(sha(OUT/'human-authorization.json')=='c4e37256edd4138f5d443287373efb6b0e1810c559fb0899caf014762840d858','Deletion authority changed')
s={'status':'RECEIVED_SEPARATE_DIRECT_HUMAN_FIXED_WINDOW_SCHEDULE_AUTHORITY','recorded_at':dt.datetime.now(dt.timezone.utc).isoformat(),'profile':'candidate19/v29','execution':a['execution'],'thread_id':a['thread_id'],'human_message_id':event['payload']['id'],'human_app_message_id':app['app_message_id'],'human_turn_id':turn,'human_message_timestamp':event['timestamp'],'question_item_id':entry['questionItemId'],'question_verbatim':question,'answer_verbatim':answer,'actual_wrapper_sha256':digest(wrapper.encode()),'actual_wrapper_utf8_bytes':len(wrapper.encode()),'actual_session_event_sha256':digest(raw),'delete_authorization_sha256':sha(OUT/'human-authorization.json'),'delete_authorization_message_id':a['human_message_id'],'delete_authorization_envelope_sha256':a['authorization_envelope_sha256'],'authorized_send_window':{'Singapore':'2026-10-22 14:00-14:40','not_before':'2026-10-22T06:00:00Z','not_after_exclusive':'2026-10-22T06:40:00Z'},'one_run_only':True,'on_completion_failure_or_expiry':'PAUSE_THIS_SAME_HEARTBEAT; NEVER_RESCHEDULE_OR_EXTEND','permits_early_send':False,'expands_delete_scope':False,'native_delete':0,'source_start':0,'provider_io':0}
write(CONTROL/'schedule-original-user-event.jsonl',raw)
write(CONTROL/'schedule-human-reply-verbatim.txt',wrapper.encode())
write(CONTROL/'schedule-human-answer-verbatim.txt',answer.encode())
save(CONTROL/'schedule-human-authority.json',s)
save(OUT/'schedule-human-authority.json',s)
write(OUT/'schedule-human-reply-verbatim.txt',wrapper.encode())
write(OUT/'schedule-human-answer-verbatim.txt',answer.encode())
names=['schedule-human-authority.json','schedule-human-reply-verbatim.txt','schedule-human-answer-verbatim.txt']
save(OUT/'schedule-authority-index01.json',{'status':'C19_V29_SEPARATE_SCHEDULE_AUTHORITY_INDEX','execution':a['execution'],'count':len(names),'files':{name:{'sha256':sha(OUT/name),'bytes':(OUT/name).stat().st_size} for name in names}})
plan=load(OUT/'authorized-evidence-audit-plan01.json')
plan['indexes'].append({'id':'separate-direct-human-schedule-authority','index':(OUT/'schedule-authority-index01.json').relative_to(ROOT).as_posix(),'sha256':sha(OUT/'schedule-authority-index01.json'),'format':'files-map-v1','member_root':OUT.relative_to(ROOT).as_posix(),'expected_count':3,'expected_status':'C19_V29_SEPARATE_SCHEDULE_AUTHORITY_INDEX'})
save(OUT/'scheduled-evidence-audit-plan01.json',plan)
env=os.environ.copy();env.update(GIT_CONFIG_COUNT='1',GIT_CONFIG_KEY_0='core.longpaths',GIT_CONFIG_VALUE_0='true',PYTHONUTF8='1')
command=[sys.executable,'-X','utf8','-B','examples/lifecycle-d1/verify-six-native.py','--candidate-sha256',a['candidate_sha256'],'--bindings-sha256',a['bindings_sha256'],'--checker-sha256','b4f6f240101df39c7dddf68fd1734a3bbbb32049736e94032d0dfdfbac26176e','--evidence-audit-plan',str(OUT/'scheduled-evidence-audit-plan01.json'),'--evidence-audit-plan-sha256',sha(OUT/'scheduled-evidence-audit-plan01.json'),'--audit-only','--output',str(OUT/'scheduled-static-audit01')]
r=subprocess.run(command,capture_output=True,env=env)
write(OUT/'scheduled-static-audit01.stdout.json',r.stdout);write(OUT/'scheduled-static-audit01.stderr.txt',r.stderr)
need(r.returncode==0,'Frozen schedule-phase audit failed; original channels retained')
result=load(OUT/'scheduled-static-audit01/evidence-audit.json')
need(result['status']=='PASS_EXPLICIT_EVIDENCE_INDEX_AUDIT' and len(result['indexes'])==10 and result['members_verified']==5473,'Schedule-phase audit count differs')
print(json.dumps({'status':s['status'],'execution':a['execution'],'authority_sha256':sha(OUT/'schedule-human-authority.json'),'wrapper_bytes':s['actual_wrapper_utf8_bytes'],'wrapper_sha256':s['actual_wrapper_sha256'],'audit_indexes':10,'references':5473,'plan_sha256':sha(OUT/'scheduled-evidence-audit-plan01.json'),'native_delete':0}))

