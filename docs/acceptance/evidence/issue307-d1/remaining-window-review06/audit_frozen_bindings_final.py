"""Independent exact public binding audit; no Docker/provider/private profiles."""
import datetime as dt
import hashlib
import json
import re
import subprocess
import tomllib
from pathlib import Path

ROOT=Path(r'C:\Users\Henry\.codex\worktrees\9400\data-product-platform')
OUT=Path(__file__).parent
BASE=ROOT/'docs/acceptance/evidence/issue307-d1'
PACKET=BASE/'remaining-candidate12'
REVIEW=BASE/'remaining-window-review06'
ANCHOR='30e954027bc1d5a74d683f0dcd8bd141aa7a678b'
EXECUTION='8e41cfba-a41e-44dd-9ce8-d6365b3bb90e'

def need(ok,message):
    if not ok:raise ValueError(message)
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def load(path):return json.loads(path.read_bytes())

def main():
    c=load(PACKET/'candidate.json');bindings=load(PACKET/'bindings.json')
    idx=load(PACKET/'evidence-index.json');entries={**idx['files'],**idx['references']}
    need(len(entries)==1305,'raw membership count')
    names=sorted(entries)
    data=subprocess.run(['git','cat-file','--batch'],cwd=ROOT,
        input=''.join(ANCHOR+':'+name+'\n' for name in names).encode(),stdout=subprocess.PIPE,check=True).stdout
    offset=0
    for name in names:
        end=data.index(b'\n',offset);header=data[offset:end].decode('ascii').split()
        need(len(header)==3 and header[1]=='blob','missing Git raw '+name)
        count=int(header[2]);offset=end+1;raw=data[offset:offset+count];offset+=count+1
        need(hashlib.sha256(raw).hexdigest()==entries[name] and raw==(ROOT/name).read_bytes(),'working/Git raw differs '+name)
    need(offset==len(data),'unparsed Git output')
    inputs=load(REVIEW/'review-inputs.json')
    need(len(inputs['files'])==25 and all(sha(ROOT/name)==value for name,value in inputs['files'].items()),'review input25 SHA')
    need(inputs['status']=='PREPARED_NOT_AUTHORIZED' and inputs['window_proposer']=='agent'
         and inputs['execution']==EXECUTION and inputs['packet_anchor']==ANCHOR,'review input authority state')

    doc_path=ROOT/'docs/acceptance/issue307-d1-candidate12-confirmation.md'
    run_path=ROOT/'docs/acceptance/issue307-d1-v22-window-runbook.md'
    doc_raw=doc_path.read_bytes();doc=doc_raw.decode('utf-8');run=run_path.read_text(encoding='utf-8')
    template=(PACKET/'confirmation-template.txt').read_bytes();text=template.decode('utf-8')
    need(re.findall(rb'```text\n(.*?)```',doc_raw,re.S)==[template],'machine template not exact embedded bytes')
    need(c['Source']=='5c7e964dac83ebda0cf44743d49a546123735119' and c['Execution']==EXECUTION
         and c['Profile']=='candidate12/v22' and c['NotBefore']=='2026-10-19T06:00:00Z'
         and c['NotAfter']=='2026-10-19T06:40:00Z','generation/window')
    need(all(value in doc for key,value in c.items() if key!='Groups' and isinstance(value,str)), 'top level binding omitted')
    need(sha(run_path) in doc and sha(PACKET/'candidate.json') in doc and sha(PACKET/'bindings.json') in doc
         and sha(PACKET/'confirmation-template.txt') in doc,'document digest binding')
    need('由代理提议' in doc and '尚非人类选定' in doc and '由代理' in run and '尚非人类选定' in run,'proposal source misstated')
    targets={'fixture/p1.csv':'P1','fixture/p2.csv':'P2','fixture/t1.csv':'T1','fixture/t2.csv':'T2',
             'fixture/h2-a.csv':'H2','fixture/m1-a.csv':'M1A','fixture/m1-b.csv':'M1B'}
    nt=ns=nh=0
    for g,b in zip(c['Groups'],bindings):
        need(g['Group']==b['group'],'group order')
        need(g['Copies']==[row['Copy'] for row in b['copy_protection']],'copies/protection order')
        for key,value in g.items():
            if isinstance(value,str):need(value in doc,'group scalar omitted '+key)
        for key in ('project','workspace','provider','incarnation','manifest_sha256','source_core_sha256','native_readonly_sha256'):
            need(b[key] in text,'binding omitted '+key)
        need(b['recovery_redis'] in doc and b['budget_volume'] in doc,'recovery/budget environment binding')
        for row in b['copy_protection']:
            cp=row['Copy'];case=targets.get(cp['Key'])
            line=next((x for x in text.splitlines() if '@'+cp['ProviderVersion']+'；copy '+cp['ID']+'；' in x),'')
            tail=(f"{cp['Namespace']}/{cp['Key']}@{cp['ProviderVersion']}；copy {cp['ID']}；DV {cp['DatasetVersionID']}；"
                  f"SHA256 {cp['ContentSHA256']}；bytes {cp['ByteSize']}；hold={row['LegalHold']}；retention={row['Retention']}；retain-until={row['RetainUntil']}")
            need(tail in line,'exact version/content/protection differs '+cp['ID'])
            if case:
                nt+=1;plan=next(p for p in b['plans'] if p['case']==('M1' if case.startswith('M1') else case))
                need(line.startswith('仅删除 '+case+' ') and ';bad' not in line
                     and '；plan '+plan['id']+'；hash '+plan['scope_hash'] in line,'target plan/case')
            else:ns+=1;need(line.startswith('必须保留 '),'survivor not retained')
        for stage,cid in g['Helpers'].items():need(cid in text and g['ConfigSHA256'][stage] in text,'operator/config omitted')
        for stage,cid in g['CaseHelpers'].items():need(cid in text and g['CaseConfigSHA256'][stage] in text,'case/config omitted')
        for action,row in b['root_owners'].items():need(row['container'] in text and row['root_config_sha256'] in text,'root bootstrap omitted')
        for dv,head in g['PolicyHeads'].items():nh+=1;need('DV '+dv+' -> '+head in text,'expected-head omitted')
    need((nt,ns,nh)==(7,11,6),'finite copy/head inventory')

    complete=load(PACKET/'complete-entry/complete.json');host=load(PACKET/'complete-entry/sealed-preconditions.json')
    need(complete['host_gate']==host and complete['host_gate_file_sha256']==sha(PACKET/'complete-entry/sealed-preconditions.json'),'embedded host receipt')
    need(host['execution']==EXECUTION and host['permissions_false']==32 and host['retained']['operators_and_bootstrap']==32
         and host['retained']['case_owners']==7 and host['retained']['backup_hashes']==9,'host gate finite counts')
    expected_cases={(g['Group'],stage,cid) for g in c['Groups'] for stage,cid in g['CaseHelpers'].items()}
    need({(r['group'],r['stage'],r['cid']) for r in host['cases']}==expected_cases
         and all(r['actual_contract']=='PASS' for r in host['cases']),'actual case receipt membership')
    need({(r['group'],r['stage']) for r in host['recoveries']}=={(g['Group'],s) for g in c['Groups'] for s in g['Helpers'] if s.startswith('recover-')}
         and all(r['read_only'] for r in host['recoveries']),'restricted recovery membership')
    need({(r['group'],r['id']) for r in host['networks']}=={(g['Group'],g['Network']) for g in c['Groups']}
         and all(r['internal'] for r in host['networks']),'internal networks')
    for budget in host['budgets']:
        g=next(g for g in c['Groups'] if g['Group']==budget['group']);b=next(b for b in bindings if b['group']==g['Group'])
        need(budget['volume']==b['budget_volume'] and budget['owner_execution']==EXECUTION and budget['empty']
             and set(budget['mounted_cids'])==set(g['Helpers'].values()),'exact budget execution/mount/empty receipt')
    for g in c['Groups']:
        f=PACKET/'complete-entry'/g['Group']
        need(load(f/'core-before.json')==load(f/'core-after.json')==load(PACKET/g['Group']/'core-readonly.json'),'Core drift')
        need(load(f/'physical-close.json')['all_closed'] is True,'native complete physical close')
        a=load(f/'aggregation.json');need(all(p['official_equals'] for p in a['policies']),'native official policy mismatch')
        starts=[load(p) for p in (f/'readonly').glob('readonly-start-*.json')]
        obs=[load(p) for p in (f/'readonly').glob('readonly-observation-*.json')]
        need(len(starts)==len(obs)==a['technical_requests'] and {r['id'] for r in starts}=={r['id'] for r in obs}
             and all(r['method'] in ('GET','HEAD') for r in starts),'real GET/HEAD journal')
    win=load(PACKET/'profile-windows.json')
    need(len(win['cases'])==len(win['recoveries'])==7,'window projection members')
    need(all(r['ValidUntil']=='2026-10-19T08:40:00Z' and r['config_sha256'] in text for r in win['recoveries']),'recovery deadlines/template')
    support=REVIEW/'composition/window_support.py';st=support.read_text(encoding='utf-8')
    for value in (ANCHOR,EXECUTION,sha(PACKET/'candidate.json'),sha(PACKET/'bindings.json'),sha(PACKET/'confirmation-template.txt')):
        need(value in st,'support frozen binding')
    need('START = dt.datetime(2026, 10, 19, 6,' in st and sha(support) in doc and sha(support) in run,'support window/hash')
    need(not (BASE/'delete-confirmed-v22-execution01/human-authorization.json').exists(),'unexpected authorization')
    automations=[]
    for path in Path(r'C:\Users\Henry\.codex\automations').glob('307-d1-*/automation.toml'):
        config=tomllib.loads(path.read_text(encoding='utf-8'))
        automations.append({'id':path.parent.name,'status':config['status'],'sha256':sha(path)})
        need(config['status']=='PAUSED','old or unauthorized schedule active '+path.parent.name)
    result={'status':'PASS_INDEPENDENT_FROZEN_BINDING_AUDIT','at':dt.datetime.now(dt.timezone.utc).isoformat(),
        'execution':EXECUTION,'packet_anchor':ANCHOR,'working_Git_raw_blobs':1305,'review_inputs':25,
        'targets':7,'survivors':11,'expected_heads':6,'case_receipts':7,'recovery_receipts':7,'empty_budget_receipts':3,
        'actual_main_receipts_reviewed_not_reexecuted':True,'reviewer_Docker_provider_io':0,'old_schedules':automations,
        'proposal_authority':'agent proposal; no human authorization or new schedule'}
    with (OUT/'exact-binding-final-audit.json').open('x',encoding='utf-8') as stream:json.dump(result,stream,indent=2);stream.write('\n')
    print(json.dumps(result))

if __name__=='__main__':main()
