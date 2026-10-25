"""Close actual full-entry, document, backup and containment evidence by files."""
from collections import Counter
import datetime as dt
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys
import tomllib

OUT = Path(__file__).resolve().parent
ROOT = OUT.parents[4]
BASE = ROOT/'docs/acceptance/evidence/issue307-d1'
PACKET = BASE/'remaining-candidate18'
FRESH = BASE/'remaining-candidate18-readonly01'
PLAN = BASE/'candidate18-planning01'
PRIVATE = Path('C:/Users/Henry/.codex/fixture-private')
CONTROL = PRIVATE/'remaining-candidate18'
sys.path.insert(0, str(ROOT/'examples/lifecycle-d1'))
import evidence_indexes as audit

def need(value, message):
    if not value: raise ValueError(message)
def load(path): return audit.decode(path.read_bytes())
def meta(path):
    raw=audit.unlinked_absolute(path).read_bytes()
    return {'sha256':hashlib.sha256(raw).hexdigest(),'bytes':len(raw)}
def save(name,value):
    with (OUT/name).open('x',encoding='utf-8',newline='\n') as target:
        json.dump(value,target,indent=2)
        target.write('\n')

c=load(PACKET/'candidate.json')
b=json.loads((PACKET/'bindings.json').read_bytes())
complete=load(FRESH/'complete.json')
need(complete['status']=='PASS_COMPLETE_SIX_NATIVE_ENTRY' and complete['native_delete']==complete['business_mutations']==0
     and complete['technical_requests']==137 and (complete['present_versions'],complete['targets'],complete['survivors'])==(17,6,11),'full entry receipt differs')
checker=CONTROL/'readonly-check.exe'
need(meta(checker)['sha256']=='b05d9f331cb2126176021db402b189678a7b0a0c1aaa8776612e5751de5b7081','checker changed')
journals=[]
for g in c['Groups']:
    group=g['Group'];fresh=FRESH/group
    need(load(fresh/'Core-before.json')==load(fresh/'Core-after.json')==load(PACKET/group/'core-readonly.json'),'fresh complete Core differs')
    closure=load(fresh/'closure.json')
    need(closure['all_closed'] and closure['failure'] is None and closure['unknown_temporary_owner'] is None and len(closure['owners'])==3
         and all(x['closed'] and not x['initial_inspect_failed'] and not x['stop_failed'] for x in closure['owners']),'full-entry physical closure incomplete')
    expected=load(fresh/'expectation.json')
    owners=[p/group for p in CONTROL.glob('complete-*') if (p/group/'six-readonly.json').exists()
            and load(p/group/'six-readonly.json')['AuditID']==expected['audit_id']]
    need(len(owners)==1,'actual reader private profile missing or ambiguous')
    private=owners[0]
    need(load(private/'six-readonly.json')['Enabled'] is False,'actual read profile was not revoked')
    command=[str(checker),'-six','-baseline',str(PACKET/group/'expectation'),'-fresh',str(fresh/'files'),'-group',group,
             '-expectation',str(fresh/'expectation.json'),'-inventory',str(private/'present-inventory.json'),'-mapping',str(private/'six-map.json')]
    run=subprocess.run(command,capture_output=True,timeout=45)
    for stream in ('stdout','stderr'):
        with (OUT/(group+'-complete-checker.'+stream+'.txt')).open('xb') as target:target.write(getattr(run,stream))
    need(run.returncode==0 and json.loads(run.stdout)['status']=='PASS_COMPLETE_SIX_NATIVE_AGGREGATION','fresh complete file-only aggregation failed')
    starts=[load(x) for x in (fresh/'files').glob('readonly-start-*.json')]
    observations=[load(x) for x in (fresh/'files').glob('readonly-observation-*.json')]
    need(len(starts)==len(observations) and len({x['id'] for x in starts})==len(starts) and {x['id'] for x in starts}=={x['id'] for x in observations},'fresh technical request pairing differs')
    observed={x['id']:x for x in observations}
    need(all(x['method'] in ('GET','HEAD') and not observed[x['id']]['transport_error'] and x['at']<=observed[x['id']]['at'] for x in starts),'fresh readonly attempt contract differs')
    journals.append({'group':group,'requests':len(starts),'methods':dict(Counter(x['method'] for x in starts)),
                     'http_statuses':dict(Counter(str(x['status']) for x in observations))})
need(sum(x['requests'] for x in journals)==137,'fresh request count differs')

cold=load(PLAN/'final-cold-inventory01.json')
allocation=load(BASE/'remaining-candidate18-preparation01/allocation.json')
readers=[load(FRESH/g['Group']/'allocated-owner.json')['cid'] for g in c['Groups']]
expected_cids=set(allocation['retained']['closed_cids'])|set(allocation['owners'])|set(readers)
need(len(expected_cids)==cold['count']==len(cold['owners'])==712 and {x['cid'] for x in cold['owners']}==expected_cids,'cold CID union incomplete')
need(all(x['PID']==0 and x['running'] is False and x['noports'] and x['restart']=='no' for x in cold['owners']),'cold physical closure differs')
need(len(cold['permissions'])==30 and len(cold['cases'])==len(cold['recoveries'])==6,'cold profile set differs')
for row in cold['permissions']:
    path=CONTROL/row['group']/row['stage']/'permission.json'
    need(not row['enabled'] and meta(path)['sha256']==row['sha256'] and load(path)['Enabled'] is False,'cold permission differs')
for row in cold['cases']:need(meta(CONTROL/row['group']/('case-'+row['stage'])/'remaining.json')['sha256']==row['sha256'],'cold case bytes differ')
for row in cold['recoveries']:need(meta(CONTROL/row['group']/row['stage']/'recovery.json')['sha256']==row['sha256'] and row['valid_until']=='2026-10-21T16:40:00Z','cold recovery differs')
old_budgets=allocation['retained']['c17']['old_budget_volumes']
need(len(cold['old_budgets'])==len(old_budgets)==18,'old budget membership differs')
for row in cold['old_budgets']:
    old=old_budgets[row['volume']]
    need(row['preserved'] and row['sha256']==old['sha256'] and len(row['mounted_cids'])==len(set(row['mounted_cids']))
         and set(row['mounted_cids'])==set(old['mounted_cids']),'old budget preservation differs')
need(len(cold['new_budgets'])==3,'new budget set differs')
for row,g in zip(cold['new_budgets'],allocation['groups']):
    need(row['group']==g['group'] and row['volume']==g['volumes']['budget'] and row['empty']
         and len(row['mounted_cids'])==len(set(row['mounted_cids'])) and set(row['mounted_cids'])==set(g['budget_expected_mounted_cids'])
         and row['verification']['uid']==row['verification']['gid']==10001 and row['verification']['files']==[],'new budget closure differs')

backup=load(PLAN/'retained-backups48-readback01.json')
need((backup['old_complete_checkpoint_files'],backup['old_failed_partial_files'],backup['new_total'],backup['total'])==(36,3,9,48),'full backup membership differs')
old_files=load(BASE/'post-v27-seventeen01/plan.json')['preservation']['retained']['files']
old_archives={name:sha for name,sha in old_files.items() if name.rsplit('/',1)[-1] in ('core.dump','globals.sql','native.tar')}
need(len(old_archives)==len(backup['old'])==39 and {x['private_relative_path'] for x in backup['old']}==set(old_archives),'old backup list incomplete')
for row in backup['old']:
    path=audit.bound_path(PRIVATE,row['private_relative_path'])
    need(meta(path)=={'sha256':row['sha256'],'bytes':row['bytes']} and row['sha256']==old_archives[row['private_relative_path']],'old actual backup differs')
for row in backup['new']:need(meta(audit.bound_path(PRIVATE,row['private_relative_path']))=={'sha256':row['sha256'],'bytes':row['bytes']},'new actual backup differs')

host=load(PLAN/'candidate18-host-bindings01.json')
need(len(host['files'])==host['count']==35,'host binding membership differs')
for name,value in host['files'].items():need(meta(audit.bound_path(ROOT,name))==value,'host binding bytes differ')
anchor=load(CONTROL/'host-anchor.json')
need(anchor==load(PLAN/'host-anchor-readback01.json')['anchor'] and meta(CONTROL/'host-anchor.json')['sha256']==load(PLAN/'host-anchor-readback01.json')['private_anchor_sha256'],'host anchor differs')
services=load(PLAN/'engine-service-bindings01.json')['services']
need(len(services)==24 and all(x['cid'] in expected_cids and x['PID']==0 and x['restart']=='no' and x['image_id']==x['configured_image'] for x in services),'service image/CID closure differs')
authority=load(PLAN/'authority-schedule-absence01.json')
need(authority['authorization']=='NOT_RECEIVED' and authority['schedule']=='NOT_CREATED' and not authority['execution_authority'],'authority scope changed')
need(not (BASE/'delete-confirmed-v28-execution01/human-authorization.json').exists() and not (CONTROL/'authorized-execution01').exists(),'unexpected new human authority')
automation=Path('C:/Users/Henry/.codex/automations')
need(not (automation/'307-d1-candidate18-v28/automation.toml').exists(),'unexpected c18 scheduler')
for row in authority['old_schedules']:
    need(row['status']=='PAUSED' and tomllib.loads((automation/row['id']/'automation.toml').read_text(encoding='utf-8'))['status']=='PAUSED','old scheduler active')

docs={}
for name in ('issue307-d1-candidate18-confirmation.md','issue307-d1-v28-window-runbook.md'):
    path=ROOT/'docs/acceptance'/name
    docs[name]=meta(path)
text=(ROOT/'docs/acceptance/issue307-d1-candidate18-confirmation.md').read_bytes()
machine=text.split(b'```text\n',1)[1].split(b'```',1)[0]
need(machine==(PACKET/'confirmation-template.txt').read_bytes() and len(machine.splitlines())==121,'full confirmation machine text changed')
need('9新及39旧'.encode() in (ROOT/'docs/acceptance/issue307-d1-v28-window-runbook.md').read_bytes(),'backup accounting explanation missing')
attester=[]
for optimized in (False,True):
    command=[sys.executable,*(['-O'] if optimized else []),'-B','examples/lifecycle-d1/test_attest_remaining_executable.py']
    run=subprocess.run(command,cwd=ROOT,capture_output=True,timeout=90)
    name='optimized' if optimized else 'normal'
    for stream in ('stdout','stderr'):
        with (OUT/('postfreeze-attester-'+name+'.'+stream+'.txt')).open('xb') as target:target.write(getattr(run,stream))
    need(run.returncode==0,'independent frozen attester models failed')
    methods=re.search(rb'Ran (\d+) tests?',run.stderr)
    attester.append({'optimized':optimized,'exit_code':run.returncode,'methods':int(methods.group(1)) if methods else None})
result={'status':'PASS_INDEPENDENT_C18_COMPLETE_ENTRY_AND_FINAL_CONTAINMENT_FILES','execution':c['Execution'],
        'at':dt.datetime.now(dt.timezone.utc).isoformat(),'fresh_complete_entry':meta(FRESH/'complete.json'),'fresh_technical_journals':journals,
        'file_only_checker_replays':3,'postfreeze_attester_models':attester,'closed_cids':712,'permissions_false':30,
        'old_budgets_preserved':18,'new_budgets_empty':3,'actual_backups':{'new':9,'old_complete':36,'old_partial':3,'total':48},
        'host_bindings_verified':35,'service_image_cid_bindings_verified':24,'documents':docs,'machine_template_lines':121,
        'authorization':'NOT_RECEIVED','schedule':'NOT_CREATED','old_schedules_paused':7,'reviewer_native_calls':0,'reviewer_DELETE':0,
        'limits':'Cold state is independently checked against the author recorded physical readback; reviewer does not inspect Docker. Final publishing HEAD PR CI and future human/schedule authorization remain separate gates.'}
save('complete-entry-file-analysis01.json',result)
print(json.dumps({'status':result['status'],'report':meta(OUT/'complete-entry-file-analysis01.json'),'docs':docs}))
