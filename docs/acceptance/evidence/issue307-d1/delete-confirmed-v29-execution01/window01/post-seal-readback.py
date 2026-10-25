"""Preserve actual failed-gate/physical containment/retained budget evidence."""
from pathlib import Path
import concurrent.futures
import datetime as dt
import hashlib
import json
import sqlite3
import subprocess
import sys
ROOT=Path.cwd();OUT=Path(__file__).resolve().parent;BASE=ROOT/'docs/acceptance/evidence/issue307-d1'
PRIVATE=Path(r'C:\Users\Henry\.codex\fixture-private');CONTROL=PRIVATE/'remaining-candidate19'
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
import remaining_sequence
def need(ok,message):
    if not ok:raise ValueError(message)
def load(p):return json.loads(p.read_bytes())
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def digest(v):return hashlib.sha256(v).hexdigest()
def save(p,v):
    with p.open('xb') as f:f.write((json.dumps(v,ensure_ascii=False,indent=2)+'\n').encode())
def docker(*args):
    result=subprocess.run(['docker',*args],capture_output=True,timeout=40)
    need(result.returncode==0,'Read-only Docker failed: '+args[0])
    return result.stdout
cold=load(BASE/'candidate19-crosscheck02/final-cold-inventory01.json')
candidate=load(BASE/'remaining-candidate19/candidate.json')
seal=load(OUT/'failure-seal01.stdout.json')
ids=sorted({x['cid'] for x in cold['owners']})
def inspect(chunk):
    rows=json.loads(docker('inspect',*chunk))
    need({x['Id'] for x in rows}==set(chunk),'Inspect membership differs')
    result=[]
    for row in rows:
        need(row['State']['Running'] is False and row['State']['Pid']==0 and row['HostConfig']['RestartPolicy']['Name']=='no' and not row['HostConfig']['PortBindings'] and not row['NetworkSettings']['Ports'],'Physical owner not closed')
        result.append({'cid':row['Id'],'PID':0,'running':False,'noports':True,'restart':'no'})
    return result
chunks=[ids[n:n+48] for n in range(0,len(ids),48)]
with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:closed=[x for part in pool.map(inspect,chunks) for x in part]
budget_rows=[]
for original in cold['old_budgets']+cold['new_budgets']:
    current=docker('cp',original['mounted_cids'][0]+':/run/budget','-')
    need(digest(current)==original['sha256'],'Retained/current budget archive SHA changed: '+original['volume'])
    mounted=docker('ps','-aq','--no-trunc','--filter','volume='+original['volume']).decode('utf-8','strict').split()
    need(len(mounted)==len(set(mounted)) and set(mounted)==set(original['mounted_cids']),'Budget mount set changed')
    row={'volume':original['volume'],'sha256':digest(current),'mounted_cids':sorted(mounted),'preserved':True}
    if original in cold['new_budgets']:
        row['verification']=remaining_sequence.budget_archive(current,candidate,original['group'],initial=True)
    budget_rows.append(row)
backups=load(BASE/'candidate19-planning01/retained-backups57-readback01.json')
need(len(backups['old'])==48 and len(backups['new'])==9,'Backup scope changed')
backup_rows=[]
for item in backups['old']+backups['new']:
    path=PRIVATE/item['private_relative_path']
    need(sha(path)==item['sha256'] and path.stat().st_size==item['bytes'],'Backup bytes changed')
    backup_rows.append(item)
permissions=[];cases=[];recoveries=[];gateways=[]
for group in candidate['Groups']:
    folder=CONTROL/group['Group']
    for path in sorted(folder.glob('*/permission.json')):
        need(load(path)['Enabled'] is False,'Permission not revoked')
        need(not (path.parent/'human-confirmation.txt').exists(),'Worker approval unexpectedly materialized')
        permissions.append({'path':str(path),'sha256':sha(path),'Enabled':False})
    for stage in group['CaseHelpers']:
        path=folder/('case-'+stage)/'remaining.json'
        need(load(path)['Base']['Enabled'] is False,'Case not revoked')
        cases.append({'group':group['Group'],'stage':stage,'Enabled':False,'sha256':sha(path)})
    for stage in group['Helpers']:
        if stage.startswith('recover-'):
            path=folder/stage/'recovery.json';need(load(path)['Enabled'] is False,'Recovery not revoked')
            recoveries.append({'group':group['Group'],'stage':stage,'Enabled':False,'sha256':sha(path)})
    need(sha(folder/'gateway.Caddyfile')==group['SealedGatewaySHA256'],'Gateway not sealed')
    gateways.append({'group':group['Group'],'sha256':sha(folder/'gateway.Caddyfile'),'sealed':True})
need(len(closed)==820 and len(permissions)==30 and len(cases)==len(recoveries)==6,'Containment membership differs')
failed=[x for x in seal['steps'] if not x['ok']]
need({x['step'] for x in failed}=={'disable-all:'+g['Group'] for g in candidate['Groups']},'Unexpected containment failure')
db=sqlite3.connect(Path(r'C:\Users\Henry\.codex\sqlite\codex-dev.db').as_uri()+'?mode=ro',uri=True)
count=db.execute('SELECT COUNT(*) FROM automations WHERE id=?',('307-d1-candidate19-v29',)).fetchone()[0];db.close()
need(count==0 and not Path(r'C:\Users\Henry\.codex\automations\307-d1-candidate19-v29\automation.toml').exists(),'Heartbeat deletion unconfirmed')
save(OUT/'post-seal-readback.json',{'status':'PASS_PHYSICAL_CONTAINMENT_AND_RETENTION_IAM_UNCONFIRMED_NATIVE_STOPPED','at':dt.datetime.now(dt.timezone.utc).isoformat(),'execution':candidate['Execution'],'owners':closed,'owners_closed':len(closed),'old_budgets_preserved':21,'new_budgets_empty':3,'budget_readbacks':budget_rows,'backups_verified':57,'backup_readbacks':backup_rows,'permissions_false':permissions,'cases_revoked':cases,'recoveries_revoked':recoveries,'gateways':gateways,'all_fixed_seal_steps':103,'confirmed_steps':100,'unconfirmed_steps':failed,'fresh_IAM_disable':'UNCONFIRMED_NATIVE_STOPPED_NO_SOURCE_START','native_started_for_IAM_readback':False,'heartbeat_deleted':True,'source_start':0,'permission_enable':0,'provider_io':0,'native_delete':0,'cleanup':0,'hold_release':0,'bypass':0,'old_pins':'NO_CORE_COMMAND_OR_SOURCE_START; FRESH_CORE_PIN_QUERY_NOT_RUN','all_steps_confirmed':False})
print(json.dumps({'status':'PASS_PHYSICAL_CONTAINMENT_AND_RETENTION_IAM_UNCONFIRMED_NATIVE_STOPPED','known_cids_closed':len(closed),'old_budgets_preserved':21,'new_budgets_empty':3,'backups':57,'permissions_false':30,'cases_revoked':6,'recoveries_revoked':6,'heartbeat_deleted':True,'native_delete':0}))

