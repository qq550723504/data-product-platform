"""Append this execution's actual human authorization; no runtime or schedule API."""
from pathlib import Path
import datetime as dt
import hashlib
import json
import re
import subprocess

ROOT = Path.cwd()
OUT = Path(__file__).resolve().parent
BASE = ROOT/'docs/acceptance/evidence/issue307-d1'
PACKET = BASE/'remaining-candidate19'
PRIVATE = Path(r'C:\Users\Henry\.codex\fixture-private\remaining-candidate19')
CONTROL = PRIVATE/'authorized-execution01'
SESSION = Path(r'C:\Users\Henry\.codex\sessions\2026\10\13\rollout-2026-10-13T23-50-22-01a13579-573d-7a32-b1e1-3297fe78380a_01a135f5-2d3a-7953-8149-c86a26446d07.jsonl')
EXECUTION = 'a5d1e569-f80e-4b38-8f32-d8139a30a077'
HEAD = 'caab46da8761e71e1d80d0c0931707a410c13a4d'

def need(ok, message):
    if not ok: raise ValueError(message)
def digest(raw): return hashlib.sha256(raw).hexdigest()
def sha(path): return digest(path.read_bytes())
def load(path): return json.loads(path.read_bytes())
def write(path, raw):
    with path.open('xb') as stream: stream.write(raw)
def save(path, value): write(path,(json.dumps(value,ensure_ascii=False,indent=2)+'\n').encode())

candidate = load(PACKET/'candidate.json')
need(candidate['Profile']=='candidate19/v29' and candidate['Execution']==EXECUTION and candidate['NotBefore']=='2026-10-22T06:00:00Z' and candidate['NotAfter']=='2026-10-22T06:40:00Z','Unique candidate/window differs')
need(sha(PACKET/'candidate.json')=='af2499793a39b0515001fe11835305b73ef1055b76be2db929aada900e0acb98' and sha(PACKET/'bindings.json')=='581bd79ce4074f130cd4841d617960078421546c0287d6deaec91af9dc8e55a1','Frozen packet changed')
need(subprocess.check_output(['git','-c','core.longpaths=true','rev-parse','HEAD']).decode().strip()==HEAD,'Published HEAD differs')
need(not subprocess.check_output(['git','-c','core.longpaths=true','diff','--name-only','HEAD']).strip(),'Tracked files changed')
need(not CONTROL.exists(),'Actual authorization directory exists; never overwrite')
app=load(OUT/'actual-app-authority-source01.json')
matches={}; turn=None
ids={'msg_01a16187-4686-7943-8fa7-a51d8d27bd7d','msg_099c3841d45c7835016ad975581020819984a6a4cc19d0cc01',app['preceding_crosscheck_message_id']}
for raw in SESSION.open('rb'):
    event=json.loads(raw); payload=event.get('payload',{})
    if event.get('type')=='turn_context': turn=payload.get('turn_id')
    if event.get('type')=='response_item' and payload.get('type')=='message' and payload.get('id') in ids:
        text=''.join(x.get('text','') for x in payload.get('content',[]) if isinstance(x,dict))
        need(payload['id'] not in matches,'Duplicate message identity')
        matches[payload['id']]=(event,raw,text,turn)
need(set(matches)==ids,'Actual chat authorization/reference unavailable')
message=matches['msg_01a16187-4686-7943-8fa7-a51d8d27bd7d']
full=matches['msg_099c3841d45c7835016ad975581020819984a6a4cc19d0cc01']
preceding=matches[app['preceding_crosscheck_message_id']]
event,raw,user,turn=message
need(event['payload']['role']=='user' and user==app['user_message_verbatim'] and turn==app['turn_id'] and event['timestamp']=='2026-10-22T02:53:37.798Z','Actual direct human identity/bytes differ')
need(user.endswith('\n') and EXECUTION in user and '2026年10月22日新加坡14:00–14:40' in user and '每份最多一次、全局最多6次' in user and 'UNKNOWN不重发' in user and '旧授权不复用' in user and '#332保持Draft、不合并，#307保持OPEN' in user,'Actual direct scope differs')
need(full[0]['timestamp']<preceding[0]['timestamp']<event['timestamp'],'Reference order differs')
template=(PACKET/'confirmation-template.txt').read_bytes()
fence=chr(96)*3
need(re.findall(fence+r'(?:text)?\n(.*?)'+fence,full[2],re.S)[0].encode()==template,'Full session template differs')
cross=BASE/'candidate19-crosscheck02'
shown=(cross/'prior-display.md').read_bytes()
# read_thread omits memory-rendering metadata and appends LF; preserve both originals.
need(full[2].split('<oai-mem-citation>',1)[0].rstrip('\n')==shown.decode().rstrip('\n'),'App/session business confirmation differ')
result=load(cross/'crosscheck-report.json')
need(result['status']=='PASS_C19_V29_DISPLAY_AND_FROZEN_EVIDENCE_CROSSCHECK' and result['remaining_inconsistencies']==[] and result['head']==HEAD and result['execution']==EXECUTION,'Preceding crosscheck not passed')
frozen_path=BASE/'candidate19-planning01/approval-reference-manifest01.json'
frozen=load(frozen_path)
need(sha(frozen_path)=='b8a50270b47706c7fc250c7612925e5ad5dff575c022e657f8051f757ebb303a' and frozen['execution']==EXECUTION and frozen['count']==893,'Original manifest differs')
refs={row['path']:dict(row) for row in frozen['approved_reference_artifacts']}
extra=[frozen_path,ROOT/'docs/acceptance/issue307-d1-v29-crosscheck01.md',BASE/'candidate19-planning01/final-publication-checkpoint01.json']
extra+=sorted(x for x in cross.rglob('*') if x.is_file())
for path in extra:
    name=path.relative_to(ROOT).as_posix();row={'path':name,'sha256':sha(path),'bytes':path.stat().st_size}
    need(name not in refs or refs[name]==row,'Conflicting reference')
    refs[name]=row
refs=[refs[name] for name in sorted(refs)]
for row in refs:
    path=(ROOT/row['path']).resolve()
    need(path.is_relative_to(ROOT) and sha(path)==row['sha256'] and path.stat().st_size==row['bytes'],'Approved reference byte drift: '+row['path'])
CONTROL.mkdir()
write(CONTROL/'user-message-verbatim.txt',user.encode())
write(CONTROL/'original-user-message-event.jsonl',raw)
write(CONTROL/'approved-referenced-template.txt',template)
write(CONTROL/'approved-full-confirmation.txt',shown)
write(CONTROL/'full-confirmation-session-verbatim.txt',full[2].encode())
write(CONTROL/'approved-full-confirmation-event.jsonl',full[1])
write(CONTROL/'preceding-crosscheck-verbatim.txt',preceding[2].encode())
write(CONTROL/'preceding-crosscheck-event.jsonl',preceding[1])
reference={'status':'DIRECT_HUMAN_APPROVED_REFERENCES_C19_V29','profile':candidate['Profile'],'execution':EXECUTION,'original_frozen_manifest_sha256':sha(frozen_path),'actual_human_message_sha256':digest(user.encode()),'count':len(refs),'approved_reference_artifacts':refs,'authority_scope':'Only this exact execution/fixed window and frozen six targets; old grants not reused'}
save(CONTROL/'approved-reference-manifest.json',reference)
host={'status':'CURRENT_HUMAN_APPROVED_C19_V29_HOST_AND_PUBLICATION_BINDINGS','profile':candidate['Profile'],'execution':EXECUTION,'final_approved_head':HEAD,'source':candidate['Source'],'packet_containment_anchor':'749257f15e13ee7ab64b4b828857e2e88a054fbc','host_anchor_sha256':sha(PRIVATE/'host-anchor.json'),'frozen_host_bindings':{'path':'docs/acceptance/evidence/issue307-d1/candidate19-planning01/candidate19-host-bindings01.json','sha256':sha(BASE/'candidate19-planning01/candidate19-host-bindings01.json')},'original_frozen_reference_manifest_sha256':sha(frozen_path),'approved_reference_manifest_sha256':sha(CONTROL/'approved-reference-manifest.json'),'publication_checkpoint_sha256':sha(BASE/'candidate19-planning01/final-publication-checkpoint01.json'),'crosscheck_result_sha256':sha(cross/'crosscheck-report.json'),'crosscheck_index_sha256':sha(cross/'evidence-index.json')}
save(CONTROL/'approved-host-bindings.txt',host)
source={'thread_id':app['thread_id'],'session_message_id':event['payload']['id'],'app_message_id':app['app_message_id'],'human_turn_id':turn,'timestamp':event['timestamp'],'user_message_sha256':digest(user.encode()),'user_message_utf8_bytes':len(user.encode()),'original_session_event_sha256':digest(raw),'original_session_event_utf8_bytes':len(raw),'source_session':str(SESSION),'source':'ACTUAL_CURRENT_CHAT_DIRECT_USER_MESSAGE','preceding_crosscheck_message_id':preceding[0]['payload']['id'],'preceding_crosscheck_timestamp':preceding[0]['timestamp'],'preceding_crosscheck_sha256':digest(preceding[2].encode()),'full_confirmation_message_id':full[0]['payload']['id'],'full_confirmation_app_sha256':digest(shown),'full_confirmation_session_sha256':digest(full[2].encode()),'rendering_metadata_difference':'Session additionally retains memory citation; business text/template identical','approval_mode':'EXPLICIT_SCOPE_BY_REFERENCE_TO_CURRENT_UNIQUE_C19_V29_FULL_PLAN_AND_PASS_CROSSCHECK','user_retyped_template':False}
save(CONTROL/'human-message-source.json',source)
scope={'profile':candidate['Profile'],'execution':EXECUTION,'source':candidate['Source'],'final_approved_head':HEAD,'packet_containment_anchor':host['packet_containment_anchor'],'candidate_sha256':sha(PACKET/'candidate.json'),'bindings_sha256':sha(PACKET/'bindings.json'),'referenced_template_sha256':digest(template),'not_before':candidate['NotBefore'],'not_after':candidate['NotAfter'],'readonly_recovery_not_after':'2026-10-22T08:40:00Z','max_per_target':1,'max_total':6,'targets':6,'survivors':11,'order':['P2','T1','T2','H2A','M1A','M1B'],'p1':'PERMANENTLY_EXCLUDED','unknown_resend':False,'cleanup':0,'hold_release':0,'bypass':0,'keep_old_pins':True,'keep_old_and_new_budgets':True,'keep_all_backups':True,'keep_all_other_environments':True,'keep_failure_unknown_attempt_cost_evidence':True,'old_authorization_reused':False,'PR332':'DRAFT_UNMERGED','Issue307':'OPEN','failure_or_expiry':'STOP_NEW_ENABLE_SEND_AND_SEAL_ALL_GROUPS; NO_EXTEND_SHIFT_REOPEN_OR_RESEND'}
envelope={'status':'DIRECT_HUMAN_EXPLICIT_BY_REFERENCE_AUTHORIZATION','source':source,'scope':scope,'original_frozen_reference_manifest_sha256':sha(frozen_path),'approved_reference_manifest_sha256':sha(CONTROL/'approved-reference-manifest.json'),'approved_reference_count':len(refs),'approved_host_bindings_sha256':sha(CONTROL/'approved-host-bindings.txt'),'user_message_verbatim':user,'user_retyped_template':False,'schedule':'NOT_CREATED_REQUIRES_SEPARATE_HUMAN_REQUEST','execution_condition':'Only inside fixed window after all fresh frozen preconditions; this record does not enable permissions'}
save(CONTROL/'authorization-envelope.txt',envelope)
auth={'status':'RECEIVED_EXPLICIT_USER_AUTHORIZATION_BY_REFERENCE','recorded_at':dt.datetime.now(dt.timezone.utc).isoformat(),**scope,'thread_id':source['thread_id'],'human_message_id':source['session_message_id'],'human_app_message_id':source['app_message_id'],'human_turn_id':turn,'human_message_timestamp':event['timestamp'],'user_message_verbatim':user,'user_message_sha256':digest(user.encode()),'user_message_utf8_bytes':len(user.encode()),'user_retyped_template':False,'approved_reference':'Actual direct human message explicitly accepts candidate19/v29 full plan, all frozen bindings and immediately preceding PASS crosscheck','source_freeze_sha256':candidate['SourceFreezeSHA256'],'profile_windows_sha256':candidate['ProfileWindowsSHA256'],'reader_binary_sha256':candidate['ReaderBinarySHA256'],'sender_wire_recovery_binary_sha256':candidate['SenderBinarySHA256'],'owner_binary_sha256':candidate['OwnerBinarySHA256'],'target_digest':candidate['TargetDigest'],'final_head_PR_CI':{'run_id':39710200629,'event':'pull_request','head':HEAD,'jobs':13,'conclusion':'success','is_main_ci':False,'fresh_recheck':'REQUIRED_BEFORE_EXECUTION'},'authorization_envelope_sha256':sha(CONTROL/'authorization-envelope.txt'),'additional_host_bindings_sha256':sha(CONTROL/'approved-host-bindings.txt'),'approved_reference_manifest_sha256':sha(CONTROL/'approved-reference-manifest.json'),'original_frozen_reference_manifest_sha256':sha(frozen_path),'approved_full_assistant_confirmation_sha256':digest(shown),'approved_full_plan_sha256':sha(ROOT/'docs/acceptance/issue307-d1-candidate19-confirmation.md'),'confirmation_audit_sha256':sha(cross/'crosscheck-report.json'),'approved_reference_artifact_count':len(refs),'approved_reference_artifacts':refs,'schedule':'NOT_CREATED_REQUIRES_SEPARATE_HUMAN_REQUEST','send_permissions':'REMAIN_DISABLED','source_provider_io':0,'native_delete':0,'full_D1':'NOT_ACCEPTED'}
save(OUT/'human-authorization.json',auth)
save(OUT/'human-message-source.json',source)
write(OUT/'user-message-verbatim.txt',user.encode())
print(json.dumps({'status':auth['status'],'execution':EXECUTION,'user_message_bytes':len(user.encode()),'user_message_sha256':auth['user_message_sha256'],'approved_references':len(refs),'authorization_sha256':sha(OUT/'human-authorization.json'),'envelope_sha256':auth['authorization_envelope_sha256'],'schedule':'NOT_CREATED','native_delete':0}))

