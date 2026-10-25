"""Finite final readback of known CIDs, unchanged old budgets and new empty ones."""
import datetime as dt
import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tomllib

ROOT=Path(__file__).resolve().parents[5]
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
import remaining_sequence as sequence
import retained_v27

BASE=ROOT/'docs/acceptance/evidence/issue307-d1'
HERE=Path(__file__).parent
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private')
CONTROL=PRIVATE/'remaining-candidate18'
load=lambda p:json.loads(p.read_bytes())
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()


def need(ok,message):
    if not ok:raise ValueError(message)


def docker(*args):return subprocess.check_output(['docker',*args])
def save(path,value):
    with path.open('xb') as f:f.write((json.dumps(value,indent=2)+'\n').encode())


def main():
    a=load(CONTROL/'allocation.json');c=load(BASE/'remaining-candidate18/candidate.json')
    complete=load(BASE/'remaining-candidate18-readonly01/complete.json')
    need(complete['status']=='PASS_COMPLETE_SIX_NATIVE_ENTRY','current full read incomplete')
    bindings=load(BASE/'remaining-candidate18/bindings.json');permissions=[];cases=[];recoveries=[]
    for group,b in zip(c['Groups'],bindings):
        folder=CONTROL/group['Group']
        for stage in [*group['Helpers'],*['root-'+x for x in b['root_owners']]]:
            path=folder/stage/'permission.json';cfg=load(path)
            need(cfg['Enabled'] is False and cfg['Execution']==c['Execution'] and cfg['ConfirmedAt']=='0001-01-01T00:00:00Z','permission changed or enabled')
            need(not (path.parent/'human-confirmation.txt').exists(),'unexpected worker authority')
            permissions.append({'group':group['Group'],'stage':stage,'sha256':sha(path),'enabled':False})
        for stage,digest in group['CaseConfigSHA256'].items():
            path=folder/('case-'+stage)/'remaining.json';need(sha(path)==digest,'frozen case changed')
            cases.append({'group':group['Group'],'stage':stage,'sha256':digest})
        for stage in group['Helpers']:
            if stage.startswith('recover-'):
                path=folder/stage/'recovery.json';cfg=load(path)
                need(cfg['ValidUntil']=='2026-10-21T16:40:00Z','recovery deadline changed')
                recoveries.append({'group':group['Group'],'stage':stage,'sha256':sha(path),'valid_until':cfg['ValidUntil']})
    need(len(permissions)==30 and len(cases)==len(recoveries)==6,'permission/case/recovery membership differs')
    old=a['retained']['closed_cids'];ids=set(old)|set(a['owners'])
    readers=[]
    for group in ('business-races','late-hold','mixed'):
        folder=BASE/'remaining-candidate18-readonly01'/group
        owner=load(folder/'allocated-owner.json');readers.append(owner['cid'])
        need(load(folder/'closure.json')['all_closed'],'technical read closure failed')
    ids.update(readers);need(len(old)==623 and len(a['owners'])==86 and len(set(readers))==3 and len(ids)==712,'known CID union differs')
    rows=[]
    for n in range(0,len(ids),64):
        for x in json.loads(docker('inspect',*sorted(ids)[n:n+64])):
            h=x['HostConfig'];closed=not x['State']['Running'] and x['State']['Pid']==0 and h['RestartPolicy']['Name']=='no' and not h.get('PortBindings') and not any(x['NetworkSettings']['Ports'].values())
            need(closed,'known owner not closed');rows.append({'cid':x['Id'],'PID':0,'running':False,'noports':True,'restart':'no'})
    need(len(rows)==len({x['cid'] for x in rows})==712,'CID observation set differs')
    old_budgets=a['retained']['c17']['old_budget_volumes'];budgets=[]
    for name,expected in old_budgets.items():
        users=docker('ps','-aq','--no-trunc','--filter','volume='+name).decode('utf-8','strict').split()
        retained_v27.budget_users(expected['mounted_cids'],users)
        raw=docker('cp',users[0]+':/run/budget','-');need(hashlib.sha256(raw).hexdigest()==expected['sha256'],'old budget bytes changed')
        budgets.append({'volume':name,'sha256':expected['sha256'],'mounted_cids':users,'preserved':True})
    need(len(budgets)==18,'old budget membership differs')
    new=[]
    for g in a['groups']:
        name=g['volumes']['budget'];volume=json.loads(docker('volume','inspect',name))[0]
        need(volume['Labels']['lifecycle307.remaining']==c['Execution'] and volume['Labels']['lifecycle307.allocation']==a['allocation_nonce'] and volume['Driver']=='local' and volume['Scope']=='local' and not volume.get('Options'),'new budget ownership differs')
        users=docker('ps','-aq','--no-trunc','--filter','volume='+name).decode('utf-8','strict').split()
        retained_v27.budget_users(g['budget_expected_mounted_cids'],users)
        raw=docker('cp',g['helpers']['wire']+':/run/budget','-');verified=sequence.budget_archive(raw,c,g['group'],initial=True)
        new.append({'group':g['group'],'volume':name,'sha256':hashlib.sha256(raw).hexdigest(),'mounted_cids':users,'empty':True,'verification':verified})
    old_expected=load(BASE/'post-v27-seventeen01/plan.json')['preservation']['retained']['files']
    retained_v27.fixed_old_files(PRIVATE,old_expected)
    save(HERE/'final-cold-inventory01.json',{'status':'PASS_712_KNOWN_CIDS_CLOSED_OLD_BUDGETS_PRESERVED_NEW_EMPTY',
        'observed_at':dt.datetime.now(dt.timezone.utc).isoformat(),'execution':c['Execution'],'owners':rows,'count':712,
        'old_cids':623,'new_owners':86,'new_readers':readers,'old_budgets':budgets,'new_budgets':new,
        'fixed_old_control_backup_files':len(old_expected),'permissions':permissions,'cases':cases,'recoveries':recoveries,
        'native_delete':0,'provider_io':0,'source_start':0})
    authority=[]
    for n,v in ((7,17),(10,20),(11,21),(12,22),(15,25),(16,26),(17,27)):
        path=Path('C:/Users/Henry/.codex/automations')/f'307-d1-candidate{n:02}-v{v}'/'automation.toml'
        if not path.exists():path=Path('C:/Users/Henry/.codex/automations')/f'307-d1-candidate{n}-v{v}'/'automation.toml'
        data=tomllib.loads(path.read_text(encoding='utf-8'));need(data['status']=='PAUSED','old heartbeat active')
        authority.append({'id':path.parent.name,'status':'PAUSED'})
    need(not (CONTROL/'authorized-execution01').exists() and not (BASE/'delete-confirmed-v28-execution01').exists()
         and not Path('C:/Users/Henry/.codex/automations/307-d1-candidate18-v28').exists(),'unexpected current authority or schedule')
    save(HERE/'authority-schedule-absence01.json',{'status':'OBSERVED_NEW_AUTHORIZATION_AND_SCHEDULE_ABSENT',
        'observed_at':dt.datetime.now(dt.timezone.utc).isoformat(),'execution':c['Execution'],'authorization':'NOT_RECEIVED',
        'schedule':'NOT_CREATED','old_schedules':authority,'permission_enabled':False,'execution_authority':False,'native_delete':0})
    print(json.dumps({'status':'PASS_FINAL_COLD_AND_AUTHORITY_ABSENCE','owners':712,'old_budgets':18,'new_budgets':3,'native_delete':0}))


if __name__=='__main__':main()
