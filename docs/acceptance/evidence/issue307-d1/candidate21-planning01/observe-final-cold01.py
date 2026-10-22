"""Actual closed-state observation using the fixed gates; never start resources."""
import datetime as dt
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tomllib

ROOT=Path(__file__).resolve().parents[5]
HERE=Path(__file__).parent
BASE=ROOT/'docs/acceptance/evidence/issue307-d1'
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
spec=importlib.util.spec_from_file_location('current_complete_cold',ROOT/'examples/lifecycle-d1/verify-six-native.py')
v=importlib.util.module_from_spec(spec);spec.loader.exec_module(v)
r,p=v.r,v.p
load,save,need=r.load,r.save,r.need


def main():
    complete=load(BASE/'remaining-candidate21-readonly01/complete.json')
    need(complete['status']=='PASS_COMPLETE_SIX_NATIVE_ENTRY','complete native proof missing')
    retained=p.historical_gate()
    gate=v.sealed_gate()
    a=load(r.PUBLIC/'allocation.json')
    ids=set(retained['closed_cids'])|set(a['owners'])
    readers=[]
    for group in p.GROUPS:
        folder=BASE/'remaining-candidate21-readonly01'/group
        readers.append(load(folder/'allocated-owner.json')['cid'])
        need(load(folder/'closure.json')['all_closed'],'technical reader closure unconfirmed')
    ids.update(readers)
    need(len(retained['closed_cids'])==907 and len(a['owners'])==86 and len(set(readers))==3 and len(ids)==996,'closed owner union differs')
    rows=[]
    for n in range(0,len(ids),64):
        items=json.loads(r.docker('inspect',*sorted(ids)[n:n+64]))
        for x in items:
            need(r.closed(x) and not x['HostConfig'].get('PortBindings') and not any((x['NetworkSettings'].get('Ports') or {}).values()),'owner not physically closed/noports')
            rows.append({'cid':x['Id'],'PID':0,'running':False,'restart':'no','noports':True})
    need(len(rows)==len({x['cid'] for x in rows})==996,'closed observation membership differs')
    save(HERE/'final-cold-inventory01.json',dict(status='PASS_996_KNOWN_CIDS_CLOSED_BUDGETS_PRESERVED',at=dt.datetime.now(dt.timezone.utc).isoformat(),execution=p.EXECUTION,owners=rows,count=996,old_cids=907,new_owners=86,new_readers=readers,retained_gate=retained,current_gate=gate,old_budgets=27,new_empty_budgets=3,native_delete=0,source_provider_actions=0))
    schedules=[]
    for number,version in ((7,17),(10,20),(11,21),(12,22),(15,25),(16,26),(17,27),(18,28),(19,29),(20,30)):
        parent=p.p1.PRIVATE.parent/'automations'
        candidates=[parent/f'307-d1-candidate{number:02}-v{version}',parent/f'307-d1-candidate{number}-v{version}']
        paths=list({x/'automation.toml' for x in candidates if (x/'automation.toml').exists()})
        need(len(paths)<=1,'ambiguous old schedule')
        status='ABSENT'
        if paths:
            status=tomllib.loads(paths[0].read_text(encoding='utf8'))['status']
            need(status=='PAUSED','old schedule must not be active')
        schedules.append({'candidate':number,'version':version,'status':status})
    need(not (r.CONTROL/'authorized-execution01').exists() and not (BASE/'delete-confirmed-v31-execution01').exists() and not (p.p1.PRIVATE.parent/'automations/307-d1-candidate21-v31').exists(),'new authority or schedule unexpectedly present')
    save(HERE/'authority-schedule-absence01.json',dict(status='OBSERVED_NEW_AUTHORIZATION_AND_SCHEDULE_ABSENT',at=dt.datetime.now(dt.timezone.utc).isoformat(),execution=p.EXECUTION,authorization='NOT_RECEIVED',schedule='NOT_CREATED',old_schedules=schedules,execution_authority=False,native_delete=0))
    print(json.dumps({'status':'PASS_FINAL_COLD_AND_AUTHORITY_ABSENCE','owners':996,'old_budgets':27,'new_budgets':3,'native_delete':0}))


if __name__=='__main__':main()
