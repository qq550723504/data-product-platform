"""Review saved/displayed c19 bindings; never enable, sign, start or delete."""
import datetime as dt
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import subprocess
import sys
import zipfile

ROOT=Path(__file__).resolve().parents[5]
BASE=ROOT/'docs/acceptance/evidence/issue307-d1'
HERE=Path(__file__).parent
PACKET=BASE/'remaining-candidate19'
PLANNING=BASE/'candidate19-planning01'
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private/remaining-candidate19')
HEAD='caab46da8761e71e1d80d0c0931707a410c13a4d'
SOURCE='a260cfd25c164652ee0e908c6e8fb65d1be9ec38'
EXECUTION='a5d1e569-f80e-4b38-8f32-d8139a30a077'
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
import remaining_sequence as sequence

def need(ok,message):
    if not ok:raise ValueError(message)

def load(path):return json.loads(path.read_bytes())
def digest(raw):return hashlib.sha256(raw).hexdigest()
def sha(path):return digest(path.read_bytes())
def save(path,value):
    with path.open('xb') as f:f.write((json.dumps(value,ensure_ascii=False,indent=2)+'\n').encode('utf-8'))

def run(name,argv,*,input=None):
    result=subprocess.run(argv,cwd=ROOT,input=input,capture_output=True)
    for channel in ('stdout','stderr'):
        with (HERE/(name+'-'+channel+'.txt')).open('xb') as f:f.write(getattr(result,channel))
    need(result.returncode==0,name+' failed; original subprocess channels retained')
    return result.stdout

def git(*argv):return subprocess.check_output(['git','-c','core.longpaths=true',*argv],cwd=ROOT)

def module(name,path):
    spec=importlib.util.spec_from_file_location(name,path)
    value=importlib.util.module_from_spec(spec);sys.modules[name]=value;spec.loader.exec_module(value)
    return value

def main():
    save(HERE/'intent.json',{'at':dt.datetime.now(dt.timezone.utc).isoformat(),'execution':EXECUTION,
         'scope':'Displayed text and saved evidence; Docker inspect/ps/cp only for cold state',
         'source_start':0,'source_provider_io':0,'native_delete':0,'authorization':False,'schedule':False})
    c=load(PACKET/'candidate.json');bindings=load(PACKET/'bindings.json');status=load(PACKET/'status.json')
    display=(HERE/'prior-display.md').read_text(encoding='utf-8')
    actual=display.split('```text\n',1)[1].split('```',1)[0].encode('utf-8')
    template=(PACKET/'confirmation-template.txt').read_bytes()
    need(actual==template and len(actual.splitlines())==127,'Actual prior message differs from frozen127-line template')
    need(c['Execution']==EXECUTION and c['Profile']=='candidate19/v29' and c['Source']==SOURCE,'Execution/profile/source differs')
    need(c['NotBefore']=='2026-10-22T06:00:00Z' and c['NotAfter']=='2026-10-22T06:40:00Z','Fixed window differs')
    for filename,key in [('candidate.json','candidate_sha256'),('bindings.json','bindings_sha256'),('confirmation-template.txt','confirmation_sha256')]:
        need(sha(PACKET/filename)==status[key],'Frozen status digest differs: '+filename)
    canonical=(ROOT/'docs/acceptance/issue307-d1-candidate19-confirmation.md').read_text(encoding='utf-8')
    need(canonical.split('```text\n',1)[1].split('```',1)[0].encode('utf-8')==template,'Document block differs')
    host=load(PLANNING/'candidate19-host-bindings01.json');displayed={}
    for name,value in re.findall(r'^\| `([^`]+)` \| `([0-9a-f]{64})` \|$',display,re.M):
        name=name.replace('E/','docs/acceptance/evidence/issue307-d1/',1) if name.startswith('E/') else name
        need(name not in displayed,'Duplicated display binding');displayed[name]=value
    need(set(displayed)==set(host['files']) and len(displayed)==host['count']==36,'Displayed host membership differs')
    for name,value in host['files'].items():
        path=ROOT/name
        need(displayed[name]==value['sha256']==sha(path) and path.stat().st_size==value['bytes'],'Host binding differs: '+name)
    extras={
        '完整确认文档':ROOT/'docs/acceptance/issue307-d1-candidate19-confirmation.md',
        'v29 有限窗口手册':ROOT/'docs/acceptance/issue307-d1-v29-window-runbook.md',
        '893 成员审批引用清单':PLANNING/'approval-reference-manifest01.json',
        '最终静态 audit plan：7 索引／4539 项引用':PLANNING/'final-static-evidence-audit-plan01.json',
        '36 成员宿主绑定索引':PLANNING/'candidate19-host-bindings01.json',
        '最终冻结独立评审，PASS／0 阻断':BASE/'candidate19-review01/independent-final-review01.json',
        '最终独立评审证据索引':BASE/'candidate19-review01/evidence-index.json',
        '独立核验的 2245 成员引用索引':BASE/'candidate19-review01/reviewed-artifacts01.json'}
    for label,path in extras.items():need('| '+label+' | `'+sha(path)+'` |' in display,'Displayed appendix differs: '+label)
    sf=load(PACKET/'source-freeze.json')
    need(sf['source']==SOURCE and sha(PACKET/'source-freeze.json')==c['SourceFreezeSHA256'],'Source freeze differs')
    archive=PRIVATE/'approved-runtime-source.zip'
    need(sha(archive)==sf['archive_sha256'] and archive.stat().st_size==sf['archive_bytes'],'Runtime ZIP differs')
    requests=''.join(SOURCE+':'+name+'\n' for name in sf['files']).encode('utf-8')
    objects=subprocess.run(['git','-c','core.longpaths=true','cat-file','--batch'],input=requests,cwd=ROOT,capture_output=True,check=True).stdout
    pointer=0;archive_raw_differences=0
    with zipfile.ZipFile(archive) as zipped:
        need(zipped.comment.decode('ascii')==SOURCE,'Source ZIP commit comment differs')
        for name,meta in sf['files'].items():
            path=ROOT/name;raw=path.read_bytes();lf=raw.replace(b'\r\n',b'\n')
            need(sha(path)==meta['raw_sha256'] and digest(lf)==meta['lf_sha256'],'Frozen source changed: '+name)
            end=objects.index(b'\n',pointer);header=objects[pointer:end].split();need(len(header)==3 and header[1]==b'blob','Git source object missing')
            length=int(header[2]);blob=objects[end+1:end+1+length];pointer=end+length+2
            zipped_raw=zipped.read(name)
            need(digest(blob.replace(b'\r\n',b'\n'))==meta['lf_sha256'] and digest(zipped_raw.replace(b'\r\n',b'\n'))==meta['lf_sha256'],'Git/ZIP normalized source differs')
            archive_raw_differences+=blob!=zipped_raw
    need(pointer==len(objects) and len(sf['files'])==2143,'Source object membership differs')
    for name,value in sf['initial_build_files'].items():need(sha(PRIVATE/name)==value,'Original build member differs: '+name)
    need(len(sf['initial_build_files'])==12,'Original build membership differs')
    manifest=load(PLANNING/'approval-reference-manifest01.json')
    need(sha(PLANNING/'approval-reference-manifest01.json')=='b8a50270b47706c7fc250c7612925e5ad5dff575c022e657f8051f757ebb303a','Approval manifest differs')
    need(len(manifest['approved_reference_artifacts'])==manifest['count']==893,'Approval membership differs')
    for item in manifest['approved_reference_artifacts']:
        path=ROOT/item['path']
        need(path.is_file() and not path.is_symlink() and not path.is_junction() and sha(path)==item['sha256'] and path.stat().st_size==item['bytes'],'Approval artifact differs: '+item['path'])
    targets=sequence.targets(c);target_ids={cp['ID'] for _,_,cp in targets};inventory=[];target_rows=[]
    key_cases={'fixture/p2.csv':'P2','fixture/t1.csv':'T1','fixture/t2.csv':'T2','fixture/h0.csv':'H0',
        'fixture/h1-a.csv':'H1','fixture/h1-b.csv':'H1','fixture/h2-a.csv':'H2','fixture/h2-b.csv':'H2',
        'fixture/m1-a.csv':'M1','fixture/m1-b.csv':'M1','fixture/m1-c.csv':'M1'}
    for group,binding in zip(c['Groups'],bindings):
        need(group['Group']==binding['group'],'Group order differs')
        native=load(BASE/'remaining-candidate19-readonly01'/group['Group']/'files/six-native-verification.json')
        seen={row['Copy']['ID']:row for row in native['verification']['copies']}
        present=[cp for cp in group['Copies'] if cp['Key']!='fixture/p1.csv']
        historical_p1=[cp for cp in group['Copies'] if cp['Key']=='fixture/p1.csv']
        if historical_p1:
            need(group['Group']=='business-races' and len(historical_p1)==1 and native['p1_absence']['copy']==historical_p1[0]
                 and native['p1_absence']['status']==404 and native['p1_absence']['code']=='NoSuchVersion','Historical P1 absence differs')
        else:need(native['p1_absence'] is None,'Unexpected P1 absence scope')
        need(set(seen)=={cp['ID'] for cp in present},'Stored17-present native membership differs')
        for cp in present:
            protection=next(x for x in binding['copy_protection'] if x['Copy']['ID']==cp['ID'])
            need(protection['Copy']==cp and seen[cp['ID']]==protection,'Exact native copy/protection differs')
            plan=next((x for x in binding['plans'] if x['case']==key_cases.get(cp['Key'])),None)
            is_target=cp['ID'] in target_ids
            line=('删除目标' if is_target else '保留版本')+f' {binding["project"]} {cp["Namespace"]}/{cp["Key"]}@{cp["ProviderVersion"]}；copy {cp["ID"]}；DV {cp["DatasetVersionID"]}；SHA256 {cp["ContentSHA256"]}；bytes {cp["ByteSize"]}；hold={protection["LegalHold"]}；retention={protection["Retention"]}；retain-until={protection["RetainUntil"]}；'
            line+=(f'plan {plan["id"]}；hash {plan["scope_hash"]}。' if plan else '非发送目标。')
            need(line in actual.decode('utf-8').splitlines(),'Displayed exact version/plan/protection differs')
            row={'group':group['Group'],'project':binding['project'],'target':is_target,'copy':cp,'protection':{k:v for k,v in protection.items() if k!='Copy'},'plan':plan}
            inventory.append(row)
            if is_target:
                need(protection['LegalHold'] is False and protection['Retention']=='NONE' and protection['RetainUntil'] is None,'Target protection blocks deletion')
                target_rows.append(row)
        need(load(BASE/'remaining-candidate19-readonly01'/group['Group']/'Core-before.json')==load(BASE/'remaining-candidate19-readonly01'/group['Group']/'Core-after.json'),'Stored read changed Core')
    need(len(inventory)==17 and len(target_rows)==6 and len({x['copy']['ID'] for x in inventory})==17,'Exact target/survivor membership differs')
    need(not any(x['copy']['Key']=='fixture/p1.csv' for x in inventory),'P1 reintroduced')
    save(HERE/'exact-version-matrix.json',{'status':'PASS_6_TARGETS_11_SURVIVORS_EXACT_FIELDS','order':[s for _,s,_ in targets],'versions':inventory,'native_observations':'SAVED_PREPARATION_NOT_FRESH_WINDOW_PROOF','native_delete':0})
    spec=module('c19_review_complete_entry',ROOT/'examples/lifecycle-d1/verify-six-native.py')
    window=spec.p.window.validate_packet(PACKET,PRIVATE)
    audit_command=['python','-X','utf8','-B','examples/lifecycle-d1/verify-six-native.py','--candidate-sha256',sha(PACKET/'candidate.json'),
        '--bindings-sha256',sha(PACKET/'bindings.json'),'--checker-sha256',sha(PRIVATE/'readonly-check.exe'),
        '--evidence-audit-plan',str(PLANNING/'final-static-evidence-audit-plan01.json'),'--evidence-audit-plan-sha256',sha(PLANNING/'final-static-evidence-audit-plan01.json'),
        '--output',str(HERE/'fixed-saved-audit01'),'--audit-only']
    run('fixed-saved-audit01',audit_command)
    replays=[]
    for group in c['Groups']:
        folder=BASE/'remaining-candidate19-readonly01'/group['Group']
        expected_audit=load(folder/'expectation.json')['audit_id']
        readonly_private=next(p for p in PRIVATE.glob('complete-*/'+group['Group']) if load(p/'six-readonly.json')['AuditID']==expected_audit)
        need(load(readonly_private/'six-readonly.json')['Enabled'] is False,'Completed audit profile not revoked')
        command=[str(PRIVATE/'readonly-check.exe'),'-six','-baseline',str(PACKET/group['Group']/'expectation'),'-fresh',str(folder/'files'),'-group',group['Group'],
            '-expectation',str(folder/'expectation.json'),'-inventory',str(readonly_private/'present-inventory.json'),'-mapping',str(readonly_private/'six-map.json')]
        replay=json.loads(run('checker-'+group['Group'],command));need(replay['status']=='PASS_COMPLETE_SIX_NATIVE_AGGREGATION','Saved checker replay denied')
        replays.append({'group':group['Group'],'copies':replay['copies'],'status':replay['status']})
    cold=spec.sealed_gate();save(HERE/'sealed-host-gate.json',cold)
    cold_module=module('c19_review_cold_composer',PLANNING/'observe-final-cold01.py');cold_module.HERE=HERE;cold_module.main()
    backup=load(PLANNING/'retained-backups57-readback01.json')
    for row in backup['old']+backup['new']:
        path=PRIVATE.parent/row['private_relative_path'];need(sha(path)==row['sha256'] and path.stat().st_size==row['bytes'],'Retained backup bytes differ')
    need(git('rev-parse','HEAD').decode().strip()==HEAD and git('status','--porcelain=v1','-uno')==b'' and git('diff',SOURCE,'--','apps/platform','examples/lifecycle-d1','migrations')==b'','Published/source/working tracked state drift')
    query='''query { repository(owner:"qq550723504",name:"data-product-platform") { pullRequest(number:332) { headRefOid isDraft state mergedAt reviewThreads(first:100) { pageInfo { hasNextPage } nodes { id isResolved } } } issue(number:307) { state } } }'''
    github=json.loads(run('github-pr-issue',['gh','api','graphql','-f','query='+query]))['data']['repository'];pr=github['pullRequest']
    need(pr['headRefOid']==HEAD and pr['isDraft'] and pr['state']=='OPEN' and pr['mergedAt'] is None and github['issue']['state']=='OPEN','Live PR/Issue scope differs')
    need(not pr['reviewThreads']['pageInfo']['hasNextPage'] and not any(not x['isResolved'] for x in pr['reviewThreads']['nodes']),'Unresolved/paginated review threads')
    checks=json.loads(run('github-checks',['gh','api','repos/qq550723504/data-product-platform/commits/'+HEAD+'/check-runs','-X','GET','-f','per_page=100']))
    need(checks['total_count']==len(checks['check_runs'])==13 and all(x['head_sha']==HEAD and x['status']=='completed' and x['conclusion']=='success' for x in checks['check_runs']),'Exact HEAD CI checks incomplete')
    ci=json.loads(run('github-ci-run',['gh','api','repos/qq550723504/data-product-platform/actions/runs/39710200629']))
    need(ci['head_sha']==HEAD and ci['event']=='pull_request' and ci['status']=='completed' and ci['conclusion']=='success','Bound PR run differs')
    anchor=load(PRIVATE/'host-anchor.json')
    need(anchor['execution']==EXECUTION and anchor['candidate_sha256']==sha(PACKET/'candidate.json') and anchor['bindings_sha256']==sha(PACKET/'bindings.json') and anchor['template_sha256']==sha(PACKET/'confirmation-template.txt'),'Containment anchor differs')
    for name in ('candidate.json','bindings.json','confirmation-template.txt'):
        need(git('show',anchor['packet_commit']+':'+(PACKET/name).relative_to(ROOT).as_posix())==(PACKET/name).read_bytes(),'Containment Git snapshot differs')
    final={'status':'PASS_C19_V29_DISPLAY_AND_FROZEN_EVIDENCE_CROSSCHECK','at':dt.datetime.now(dt.timezone.utc).isoformat(),
        'execution':EXECUTION,'profile':c['Profile'],'head':HEAD,'runtime_source':SOURCE,'prior_display_message':load(HERE/'prior-display-source.json'),
        'prior_display_sha256':sha(HERE/'prior-display.md'),'template_sha256':sha(PACKET/'confirmation-template.txt'),'template_lines':127,'display_template_bytes_equal':True,
        'host_members':36,'additional_displayed_digest_bindings':len(extras),'approved_reference_members':893,
        'explicit_audit':load(HERE/'fixed-saved-audit01/evidence-audit.json'),'source_members':2143,'build_members':12,
        'raw_Git_ZIP_differences_with_equal_LF':archive_raw_differences,'source_ZIP_raw_identity_not_claimed':True,
        'historical_copies':18,'targets':6,'survivors':11,'target_stage_order':[s for _,s,_ in targets],'P1':'EXCLUDED_HISTORICAL_ABSENCE',
        'window':window,'saved_native_checker_replays':replays,'cold_gate':cold,'known_owners_closed':820,'old_budgets_retained':21,
        'new_budgets_empty':3,'backup_hashes':57,'PR332':'OPEN_DRAFT_UNMERGED','Issue307':'OPEN','unresolved_review_threads':0,
        'exact_head_PR_CI':{'run':ci['id'],'event':ci['event'],'checks':13,'status':'SUCCESS','main_ci_claimed':False},
        'new_human_authorization':'NOT_RECEIVED','schedule':'NOT_CREATED','Host_check':'NOT_RUN_REQUIRES_NEW_HUMAN_AUTHORIZATION',
        'fresh_window_Core_native_IAM_inventory':'NOT_RUN_REQUIRED_IN_AUTHORIZED_WINDOW','source_start':0,'source_provider_io':0,
        'proof_signatures':0,'permission_enable':0,'native_delete':0,'remaining_inconsistencies':[],'full_D1':'NOT_ACCEPTED'}
    save(HERE/'crosscheck-report.json',final)
    print(json.dumps({k:final[k] for k in ('status','execution','targets','survivors','known_owners_closed','new_budgets_empty','backup_hashes','remaining_inconsistencies','native_delete')}))

if __name__=='__main__':
    try:main()
    except BaseException as error:
        save(HERE/'failure.json',{'status':'CROSSCHECK_FAILED','type':type(error).__name__,'message':str(error),'source_start':0,'native_delete':0})
        raise
