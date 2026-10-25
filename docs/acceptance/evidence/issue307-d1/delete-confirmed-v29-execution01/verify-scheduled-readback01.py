"""Read actual scheduler/TOML/closed owners; never write the scheduler database."""
from pathlib import Path
import concurrent.futures
import datetime as dt
import hashlib
import json
import sqlite3
import subprocess
import tomllib
ROOT=Path.cwd();OUT=Path(__file__).resolve().parent
PRIVATE=Path(r'C:\Users\Henry\.codex\fixture-private\remaining-candidate19')
def need(ok,message):
    if not ok:raise ValueError(message)
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def load(path):return json.loads(path.read_bytes())
def write(path,raw):
    with path.open('xb') as f:f.write(raw)
def save(path,value):write(path,(json.dumps(value,ensure_ascii=False,indent=2)+'\n').encode())
a=load(OUT/'human-authorization.json')
args=load(OUT/'schedule-create-arguments01.json')
config=Path(r'C:\Users\Henry\.codex\automations\307-d1-candidate19-v29\automation.toml')
raw=config.read_bytes();value=tomllib.loads(raw.decode())
need(value['id']=='307-d1-candidate19-v29' and value['status']=='ACTIVE' and value['kind']=='heartbeat' and value['target_thread_id']==a['thread_id'] and value['prompt']==args['prompt'] and value['prompt'].encode()==(OUT/'scheduled-prompt01.txt').read_bytes() and value['rrule']==args['rrule'],'Actual TOML drift')
connection=sqlite3.connect(Path(r'C:\Users\Henry\.codex\sqlite\codex-dev.db').as_uri()+'?mode=ro',uri=True)
connection.execute('PRAGMA query_only=ON');connection.row_factory=sqlite3.Row
row=dict(connection.execute('SELECT id,name,status,next_run_at,next_run_nominal_at,last_run_at,rrule,kind,target_thread_id,created_at,updated_at,model,reasoning_effort,notification_policy,prompt FROM automations WHERE id=?',(value['id'],)).fetchone())
matches=[dict(x) for x in connection.execute('SELECT id,status FROM automations WHERE instr(prompt,?)>0',(a['execution'],)).fetchall()]
runs=connection.execute('SELECT COUNT(*) FROM automation_runs WHERE automation_id=?',(value['id'],)).fetchone()[0]
prior_ids=['307-d1-candidate07-v17','307-d1-candidate10-v20','307-d1-candidate11-v21','307-d1-candidate12-v22','307-d1-candidate15-v25','307-d1-candidate16-v26','307-d1-candidate17-v27']
prior=[]
for ident in prior_ids:
    prior.append(dict(connection.execute('SELECT id,status,next_run_at,next_run_nominal_at FROM automations WHERE id=?',(ident,)).fetchone()))
old18=connection.execute('SELECT COUNT(*) FROM automations WHERE id=?',('307-d1-candidate18-v28',)).fetchone()[0]
connection.close()
start=dt.datetime(2026,10,22,6,0,tzinfo=dt.timezone.utc);end=start+dt.timedelta(minutes=40)
need(row['status']=='ACTIVE' and row['prompt']==args['prompt'] and row['target_thread_id']==a['thread_id'] and row['kind']=='heartbeat','Actual scheduler contract drift')
need(row['next_run_at']==row['next_run_nominal_at']==int(start.timestamp()*1000) and row['last_run_at'] is None and runs==0 and len(matches)==1 and matches[0]['id']==value['id'],'Actual next run/duplicate/run count differs')
need(all(x['status']=='PAUSED' and x['next_run_at'] is None and x['next_run_nominal_at'] is None for x in prior) and old18==0,'Old schedule unexpectedly active or restored')
need(dt.datetime.now(dt.timezone.utc)<start,'Not prewindow')
write(OUT/'automation-toml-snapshot01.txt',raw)
candidate=load(ROOT/'docs/acceptance/evidence/issue307-d1/remaining-candidate19/candidate.json')
permissions=[]
for group in candidate['Groups']:
    folder=PRIVATE/group['Group']
    for path in sorted(folder.glob('*/permission.json')):
        p=load(path)
        need(p['Enabled'] is False and not (path.parent/'human-confirmation.txt').exists(),'Permission or worker confirmation prematurely enabled')
        permissions.append({'path':str(path),'sha256':sha(path),'Enabled':False})
need(len(permissions)==30,'Permission membership differs')
# Batch only actual inspect readbacks for the previously fixed, explicitly known set.
cold=load(ROOT/'docs/acceptance/evidence/issue307-d1/candidate19-crosscheck02/final-cold-inventory01.json')
ids=sorted({x['cid'] for x in cold['owners']})
need(len(ids)==820,'Fixed prior known CID set differs')
def inspect(chunk):
    result=subprocess.run(['docker','inspect',*chunk],capture_output=True,timeout=45)
    need(result.returncode==0,'Read-only owner inspect failed')
    rows=json.loads(result.stdout);need({x['Id'] for x in rows}==set(chunk),'Actual inspect membership differs')
    answer=[]
    for x in rows:
        need(x['State']['Running'] is False and x['State']['Pid']==0 and x['HostConfig']['RestartPolicy']['Name']=='no' and not x['HostConfig']['PortBindings'] and not x['NetworkSettings']['Ports'],'Retained owner closure differs')
        answer.append({'cid':x['Id'],'running':False,'PID':0,'restart':'no','noports':True})
    return answer
chunks=[ids[n:n+48] for n in range(0,len(ids),48)]
with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:closed=[x for batch in pool.map(inspect,chunks) for x in batch]
save(OUT/'schedule-cold-owners01.json',{'status':'PASS_CURRENT_820_FIXED_KNOWN_OWNERS_CLOSED_AND_30_FALSE','at':dt.datetime.now(dt.timezone.utc).isoformat(),'owners':sorted(closed,key=lambda x:x['cid']),'permissions':permissions,'container_starts':0,'source_start':0,'provider_io':0,'native_delete':0})
head=subprocess.check_output(['git','-c','core.longpaths=true','rev-parse','HEAD']).decode().strip()
need(head==a['final_approved_head'] and not subprocess.check_output(['git','-c','core.longpaths=true','diff','--name-only','HEAD']).strip(),'Tracked source/HEAD drift')
row.pop('prompt')
receipt={'status':'PASS_ACTUAL_SINGLE_HEARTBEAT_FIXED_NEXT_RUN_AND_PREWINDOW_CLOSED','checked_at':dt.datetime.now(dt.timezone.utc).isoformat(),'automation':row,'actual_prompt_sha256':sha(OUT/'scheduled-prompt01.txt'),'actual_prompt_utf8_bytes':len(args['prompt'].encode()),'actual_prompt_exact':True,'saved_toml_sha256':sha(OUT/'automation-toml-snapshot01.txt'),'next_run_utc':start.isoformat(),'next_run_singapore':start.astimezone(dt.timezone(dt.timedelta(hours=8))).isoformat(),'window_end_exclusive_utc':end.isoformat(),'recovery_end_utc':'2026-10-22T08:40:00Z','automation_count_for_execution':len(matches),'scheduler_runs':runs,'old_heartbeats':prior,'old18_absent':True,'head':head,'tracked_modifications':0,'permissions_false':30,'known_cids_closed':len(closed),'new_budget_empty_check':'PASS_03:01Z_FIXED_HOST_CHECK; NOT_REPEATED_FOR_SCHEDULING','new_native_preflight':'NOT_RUN','source_start':0,'provider_io':0,'native_delete':0,'verification_sources':['automation create/view','actual TOML exact prompt','SQLite read-only next_run_at/nominal/run/duplicate counts','current 820 fixed Docker owner inspect and 30 false permission readbacks']}
save(OUT/'scheduled-local-readback01.json',receipt)
print(json.dumps({'status':receipt['status'],'automation_id':row['id'],'next_run_utc':receipt['next_run_utc'],'next_run_singapore':receipt['next_run_singapore'],'matching_automations':len(matches),'runs':runs,'permissions_false':30,'known_cids_closed':len(closed),'source_start':0,'native_delete':0}))

