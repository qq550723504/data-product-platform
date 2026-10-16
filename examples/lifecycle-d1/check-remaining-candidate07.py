"""Strict offline candidate07 audit. No Docker, SQL, network or secret loading."""
import hashlib
import json
from pathlib import Path
import re
import uuid

ROOT=Path(__file__).resolve().parents[2]
PACKET=Path('docs/acceptance/evidence/issue307-d1/remaining-candidate07')
EXECUTION='effa0385-9308-42eb-bd66-d0c379b6a989'
START='2026-10-17T02:00:00Z';END='2026-10-17T02:40:00Z'
FIELDS=('ID','WorkspaceID','DatasetVersionID','ProviderInstanceID','IncarnationID','Namespace','Key','ProviderVersion','ContentSHA256','ByteSize')
KEYS={'fixture/p1.csv':'P1','fixture/p2.csv':'P2','fixture/t1.csv':'T1','fixture/t2.csv':'T2','fixture/h2-a.csv':'H2','fixture/m1-a.csv':'M1A','fixture/m1-b.csv':'M1B'}

def require(value,message):
    if not value:raise ValueError(message)
def sha(raw):return hashlib.sha256(raw).hexdigest()
def load(path):return json.loads(path.read_bytes())
def hash64(value):return isinstance(value,str) and re.fullmatch('[0-9a-f]{64}',value) is not None
def compact(v):return json.dumps(v,separators=(',',':'),ensure_ascii=False).encode()

def verify_records(root,records):
    for name,expected in records.items():
        path=root/name
        require(path.resolve().is_relative_to(root.resolve()) and '\\' not in name and '..' not in Path(name).parts,'unsafe manifest path')
        require(path.is_file() and hash64(expected) and sha(path.read_bytes())==expected,'raw evidence changed: '+name)

def validate(candidate,bindings,scope):
    c=candidate
    require(c['Profile']=='candidate07/v17' and c['Execution']==EXECUTION,'foreign candidate/execution')
    require(c['NotBefore']==START and c['NotAfter']==END,'absolute window drifted')
    require(re.fullmatch('[0-9a-f]{40}',c['Source']) is not None,'source identity invalid')
    for field in ('SourceFreezeSHA256','ReaderBinarySHA256','SenderBinarySHA256','OwnerBinarySHA256','TargetDigest','PublicKey','WirePublicKey'):require(hash64(c[field]),'invalid hash/key '+field)
    require(c['PublicKey']!=c['WirePublicKey'],'host signing key exposed to wire owner')
    require(len(c['Groups'])==len(bindings)==len(scope)==3,'group count differs')
    targets=[];survivors=[];helpers=set();case_owners=set()
    for group,fixed,binding in zip(c['Groups'],scope,bindings):
        g=group;require(g['Group']==fixed['Group']==binding['group'],'group order/scope differs')
        require(binding['workspace']==fixed['Workspace'] and binding['provider']==fixed['Provider'] and binding['incarnation']==fixed['Incarnation'],'environment identity differs')
        for field in ('Core','Native','Gateway','Network','CheckpointCore','CheckpointNative','CoreDumpSHA256','GlobalsSHA256','NativeArchiveSHA256','IAMSHA256','GatewaySHA256','SealedGatewaySHA256'):require(hash64(g[field]),'physical/checkpoint binding invalid')
        require(g['Core']!=g['CheckpointCore'] and g['Native']!=g['CheckpointNative'],'restore is source')
        originals={cp['ID']:cp for cp in fixed['Copies']};seen=set();seen_keys=set();expected_policies=set()
        for cp in g['Copies']:
            require(set(cp)==set(FIELDS) and cp['ID'] not in seen and cp['Namespace']+'/'+cp['Key'] not in seen_keys,'copy shape/duplicate differs')
            seen.add(cp['ID']);seen_keys.add(cp['Namespace']+'/'+cp['Key'])
            require(cp['WorkspaceID']==fixed['Workspace'] and cp['ProviderInstanceID']==fixed['Provider'] and cp['IncarnationID']==fixed['Incarnation'],'copy environment differs')
            if cp['ID'] in originals:require(cp==originals[cp['ID']],'frozen source locator/version/content differs')
            else:require(g['Group']=='business-races' and cp['Key'] in ('fixture/remaining-standardized.csv','fixture/remaining-processed.csv') and cp['Namespace']=='dpp-lifecycle307-online','unapproved additional copy')
            proof=next(o for o in binding['copy_protection'] if o['Copy']['ID']==cp['ID']);require(proof['Copy']==cp,'native evidence identity differs')
            if cp['Key'] in KEYS:
                require(not proof['LegalHold'] and proof['Retention']=='NONE' and proof['RetainUntil'] is None,'protected prospective target')
                targets.append({k:cp[k] for k in FIELDS})
            else:survivors.append(cp)
            if g['Group']=='business-races' and cp['Key'] not in ('fixture/control-hold.csv','fixture/control-retention.csv'):expected_policies.add(cp['DatasetVersionID'])
        require(set(originals).issubset(seen) and len(g['Copies'])==len(originals)+(2 if g['Group']=='business-races' else 0),'missing original/output')
        require(set(g['PolicyHeads'])==expected_policies and g['PolicyHeads']==binding['previous_policy_heads'],'policy heads differ')
        for head in g['PolicyHeads'].values():require(uuid.UUID(head).int!=0,'nil policy head')
        stages={'wire'}
        for cp in fixed['Copies']:
            if cp['Key'] in KEYS:stages.update([KEYS[cp['Key']],'recover-'+KEYS[cp['Key']]])
        require(set(g['Helpers'])==set(g['ConfigSHA256'])==stages,'operator stage set differs')
        cases={'business-races':{'renew','business'},'late-hold':{'late','close-H1'},'mixed':{'mixed','mixed-b','mixed-c'}}[g['Group']]
        require(set(g['CaseHelpers'])==set(g['CaseConfigSHA256'])==cases,'case owner set differs')
        for field in ('Helpers','ConfigSHA256','CaseHelpers','CaseConfigSHA256'):
            require(all(hash64(v) for v in g[field].values()),'helper/config binding invalid')
        helpers.update(g['Helpers'].values());case_owners.update(g['CaseHelpers'].values())
        require(g['CoreDumpSHA256']==binding['checkpoint']['core_dump_sha256'] and g['GlobalsSHA256']==binding['checkpoint']['globals_sha256'] and g['NativeArchiveSHA256']==binding['checkpoint']['native_archive_sha256'],'backup evidence binding differs')
    require(len(targets)==7 and len(survivors)==11 and len(helpers)==17 and len(case_owners)==7,'finite budget/inventory/owner count differs')
    # Go target digest enumerates fixed case order, independent of registration order.
    order=['P1','P2','T1','T2','H2','M1A','M1B'];targets.sort(key=lambda cp:order.index(KEYS[cp['Key']]))
    require(sha(compact(targets))==c['TargetDigest'],'target digest differs')
    return targets,survivors

def check(root=ROOT):
    folder=root/PACKET;index=load(folder/'evidence-index.json')
    verify_records(root,index['files']);verify_records(root,index['references'])
    require(set(index['files'])=={p.relative_to(root).as_posix() for p in folder.rglob('*') if p.is_file() and p.name!='evidence-index.json'},'unindexed frozen payload')
    c=load(folder/'candidate.json');bindings=load(folder/'bindings.json');source=load(folder/'source-freeze.json')
    require(sha((folder/'source-freeze.json').read_bytes())==c['SourceFreezeSHA256'] and source['source_commit']==c['Source'],'source manifest binding differs')
    require('apps/platform/internal/lifecycle/pilot/remaining_scope.json' in source['files'],'embedded fixed scope omitted')
    for name,hashes in source['files'].items():
        path=root/name;require(path.resolve().is_relative_to(root.resolve()) and path.is_file(),'source path missing/unsafe')
        require(sha(path.read_bytes().replace(b'\r\n',b'\n'))==hashes['lf_sha256'],'current source drift: '+name)
    scope=load(root/'apps/platform/internal/lifecycle/pilot/remaining_scope.json');targets,survivors=validate(c,bindings,scope)
    status=load(folder/'status.json');require(status['candidate_sha256']==sha((folder/'candidate.json').read_bytes()) and status['execution']==EXECUTION and status['human_confirmation']=='NOT_RECEIVED' and status['permission_enabled'] is False,'authorization boundary changed')
    require(all(status[k]==0 for k in ('current_delete_budget','native_delete','hold_release','bypass','cleanup')) and status['PR323']=='DRAFT' and status['Issue307']=='OPEN' and status['full_D1']=='NOT_ACCEPTED','scope boundary changed')
    for filename,count in [('sealed-owner-regression.json',17),('sealed-case-regression.json',7)]:
        denials=load(folder/filename);require(len(denials)==count and all(x['exit_code']==1 and x['source_core_native_stopped'] and x['native_delete']==0 for x in denials),'sealed actual-owner regression incomplete')
    require('PASS_FROZEN_CANDIDATE_OFFLINE' in (folder/'offline-validator.txt').read_text(),'Go offline check missing')
    text=(folder/'confirmation-template.txt').read_text(encoding='utf-8')
    for value in [EXECUTION,c['Source'],c['TargetDigest'],status['candidate_sha256'],START,END,*[cp['ProviderVersion'] for g in c['Groups'] for cp in g['Copies']]]:require(value in text,'exact confirmation binding omitted')
    require('旧授权全部不复用' in text and text.count('仅删除 ')==7 and text.count('必须保留 ')==11,'exact confirmation scope incomplete')
    return {'candidate':c['Profile'],'execution':EXECUTION,'targets':len(targets),'survivors':len(survivors),'source_files':len(source['files']),'raw_evidence':len(index['files'])+len(index['references']),'native_delete':0,'status':'PASS_OFFLINE'}

if __name__=='__main__':print(json.dumps(check()))
