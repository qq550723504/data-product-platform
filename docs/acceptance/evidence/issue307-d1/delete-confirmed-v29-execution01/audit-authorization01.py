"""Audit append-only actual grant references using the frozen complete entry."""
from pathlib import Path
import datetime as dt
import hashlib
import json
import os
import subprocess
import sys

ROOT=Path.cwd(); OUT=Path(__file__).resolve().parent
CONTROL=Path(r'C:\Users\Henry\.codex\fixture-private\remaining-candidate19\authorized-execution01')
def need(ok,message):
    if not ok: raise ValueError(message)
def sha(path): return hashlib.sha256(path.read_bytes()).hexdigest()
def load(path): return json.loads(path.read_bytes())
def write(path,raw):
    with path.open('xb') as f:f.write(raw)
def save(path,value):write(path,(json.dumps(value,ensure_ascii=False,indent=2)+'\n').encode())
def item(path):return {'sha256':sha(path),'bytes':path.stat().st_size}

a=load(OUT/'human-authorization.json')
e=load(CONTROL/'authorization-envelope.txt')
host=load(CONTROL/'approved-host-bindings.txt')
manifest=load(CONTROL/'approved-reference-manifest.json')
need(a['user_retyped_template'] is False and a['old_authorization_reused'] is False,'Attribution differs')
need(sha(CONTROL/'authorization-envelope.txt')==a['authorization_envelope_sha256'],'Envelope differs')
need(sha(CONTROL/'approved-host-bindings.txt')==a['additional_host_bindings_sha256']==e['approved_host_bindings_sha256'],'Host bindings differ')
need(sha(CONTROL/'approved-reference-manifest.json')==a['approved_reference_manifest_sha256']==e['approved_reference_manifest_sha256']==host['approved_reference_manifest_sha256'],'Manifest differs')
need(manifest['approved_reference_artifacts']==a['approved_reference_artifacts'] and manifest['count']==a['approved_reference_artifact_count']==len(a['approved_reference_artifacts']),'References differ')
need((CONTROL/'user-message-verbatim.txt').read_bytes()==(OUT/'user-message-verbatim.txt').read_bytes()==a['user_message_verbatim'].encode() and sha(CONTROL/'user-message-verbatim.txt')==a['user_message_sha256'],'User original bytes differ')
need((CONTROL/'approved-referenced-template.txt').read_bytes()==(ROOT/'docs/acceptance/evidence/issue307-d1/remaining-candidate19/confirmation-template.txt').read_bytes(),'Approved template differs')
need(sha(CONTROL/'approved-full-confirmation.txt')==a['approved_full_assistant_confirmation_sha256'],'Approved full display differs')
need(sha(CONTROL/'preceding-crosscheck-verbatim.txt')==e['source']['preceding_crosscheck_sha256'],'Preceding chat crosscheck differs')
save(OUT/'actual-authorization-readback01.json',{'status':'PASS_ACTUAL_C19_AUTHORIZATION_ORIGINALS_AND_PRIVATE_ENVELOPE','execution':a['execution'],'at':dt.datetime.now(dt.timezone.utc).isoformat(),'private_files':{p.name:item(p) for p in sorted(CONTROL.iterdir()) if p.is_file()},'user_retyped_template':False,'old_authorization_reused':False,'approved_references':len(a['approved_reference_artifacts']),'source_start':0,'permission_enable':0,'native_delete':0})
save(OUT/'approved-artifacts-index01.json',{'status':'C19_V29_ACTUAL_AUTHORIZATION_APPROVED_ARTIFACTS','execution':a['execution'],'count':len(a['approved_reference_artifacts']),'members':a['approved_reference_artifacts']})
names=['record-human-authorization01.py','actual-app-authority-source01.json','human-authorization.json','human-message-source.json','user-message-verbatim.txt','actual-authorization-readback01.json','audit-authorization01.py']
save(OUT/'authorization-evidence-index01.json',{'status':'C19_V29_ACTUAL_AUTHORIZATION_EVIDENCE_INDEX','execution':a['execution'],'count':len(names),'files':{n:item(OUT/n) for n in names}})
plan=load(ROOT/'docs/acceptance/evidence/issue307-d1/candidate19-planning01/final-static-evidence-audit-plan01.json')
for name,fmt,ident in [('approved-artifacts-index01.json','members-list-v1','actual-authorization-approved-artifacts'),('authorization-evidence-index01.json','files-map-v1','actual-direct-human-authorization')]:
    index=load(OUT/name)
    plan['indexes'].append({'id':ident,'index':(OUT/name).relative_to(ROOT).as_posix(),'sha256':sha(OUT/name),'format':fmt,'member_root':'.' if fmt=='members-list-v1' else OUT.relative_to(ROOT).as_posix(),'expected_count':index['count'],'expected_status':index['status']})
path=OUT/'authorized-evidence-audit-plan01.json';save(path,plan)
command=[sys.executable,'-X','utf8','-B','examples/lifecycle-d1/verify-six-native.py','--candidate-sha256',a['candidate_sha256'],'--bindings-sha256',a['bindings_sha256'],'--checker-sha256','b4f6f240101df39c7dddf68fd1734a3bbbb32049736e94032d0dfdfbac26176e','--evidence-audit-plan',str(path),'--evidence-audit-plan-sha256',sha(path),'--audit-only','--output',str(OUT/'authorized-static-audit01')]
env=os.environ.copy();env.update(GIT_CONFIG_COUNT='1',GIT_CONFIG_KEY_0='core.longpaths',GIT_CONFIG_VALUE_0='true',PYTHONUTF8='1')
r=subprocess.run(command,capture_output=True,env=env)
write(OUT/'authorized-static-audit01.stdout.json',r.stdout);write(OUT/'authorized-static-audit01.stderr.txt',r.stderr)
need(r.returncode==0,'Frozen complete audit-only failed; original channels retained')
result=load(OUT/'authorized-static-audit01/evidence-audit.json')
need(result['status']=='PASS_EXPLICIT_EVIDENCE_INDEX_AUDIT','No audit PASS')
print(json.dumps({'status':result['status'],'indexes':len(result['indexes']),'members_verified':result['members_verified'],'authorization_sha256':sha(OUT/'human-authorization.json'),'authorized_plan_sha256':sha(path),'native_delete':0}))

