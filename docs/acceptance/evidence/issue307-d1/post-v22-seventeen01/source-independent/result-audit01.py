import sys
sys.dont_write_bytecode=True
import datetime as dt,hashlib,importlib.util,json,pathlib,subprocess,traceback
R=pathlib.Path(__file__).resolve().parents[6]
D=pathlib.Path(__file__).resolve().parent
P=R/'docs/acceptance/evidence/issue307-d1/post-v22-seventeen01/preparation.json'
C=pathlib.Path('C:/Users/Henry/.codex/fixture-private/post-v22-seventeen01')
H='9708a54d44346906e8cff89bfbd77ec8e6ed28d1'
MS='dc8aed61f6a6755cea5c43d6a35aa19adf2c544d6e8b9754de7d66bfdf0d6c9f'
SR='b864aecf9dd0f9067c4a4448415e2d79b2fc37bb9d3059940206103fa6367784'
sha=lambda p:hashlib.sha256(pathlib.Path(p).read_bytes()).hexdigest()
load=lambda p:json.loads(pathlib.Path(p).read_bytes())
commands=[]
real_run=subprocess.run
original_popen=subprocess.Popen

def safe_run(args,*a,**kw):
    v=list(map(str,args));ok=False
    if v[0]=='docker':
        ok=(v[1]=='inspect' or v[1:3] in (['network','inspect'],['volume','inspect']) or v[1]=='ps' or (v[1]=='cp' and len(v)==4 and v[2].endswith(':/run/budget/.') and v[3]=='-'))
    elif v[0]=='git':ok=v[1] in ('ls-files','diff','rev-parse')
    elif v[0]=='go':ok=v[1:3]==['version','-m']
    elif v[0]==str(C/'checker.exe'):ok=(v[1]=='-post-v22')
    if not ok:raise RuntimeError('independent read-only subprocess boundary refused '+v[0])
    subprocess.Popen=original_popen
    try:result=real_run(v,*a,**kw)
    finally:subprocess.Popen=deny_popen
    commands.append({'argv':v,'exit':result.returncode,'stdout_bytes':len(result.stdout or b''),'stdout_sha256':hashlib.sha256(result.stdout or b'').hexdigest(),'stderr_bytes':len(result.stderr or b''),'stderr_sha256':hashlib.sha256(result.stderr or b'').hexdigest()})
    return result
subprocess.run=safe_run
def deny_popen(*a,**k):raise RuntimeError('independent Popen boundary refused')
subprocess.Popen=deny_popen

def need(v,msg):
    if not v:raise RuntimeError(msg)
def save(name,value):
    with (D/name).open('x',encoding='utf-8',newline='\n') as f:json.dump(value,f,indent=2);f.write('\n')

import uuid,zipfile,collections,urllib.parse
B=P.parent
result={'status':'IN_PROGRESS','source_head':H,'manifest_sha256':sha(P),'audit_id':load(P)['audit_id'],'reviewed_at':dt.datetime.now(dt.timezone.utc).isoformat(),'remaining_blockers':[]}
try:
    m=load(P);t=load(B/'terminal.json');idx=load(B/'execution-evidence-index01.json')
    need(sha(P)==MS and sha(B/'terminal.json')=='c6e12ffdb14b64c139d7352f67b6c54b1c3ff859fe0400f03c3343dcd4c92a33' and sha(B/'execution-evidence-index01.json')=='13601fe7a46de3dc2752f325a0e3d94362ea58a7d6d88e24158d6ca775fea236','terminal/index binding mismatch')
    for name,h in idx.items():
        parts=pathlib.PurePosixPath(name)
        need(not parts.is_absolute() and '..' not in parts.parts and '__pycache__' not in parts.parts and not name.endswith('.pyc') and sha(B/name)==h,'evidence index missing/unsafe/raw SHA mismatch')
    need(len(idx)==297,'index member count changed')
    need(t['status']=='PASS_COMPLETE_POST_V22_SEVENTEEN_READONLY' and t['manifest_sha256']==MS and t['independent_review_sha256']==sha(D/'manifest-final-review01.json') and t['audit_id']==m['audit_id'] and t['versions']==17 and t['remaining_targets']==6 and t['survivors']==11,'terminal scope differs')
    need(t['attempted_groups']==['business-races','late-hold','mixed'] and t['technical_requests']==74 and t['native_delete']==0 and t['business_mutations']==0 and t['amount']=='NOT_ASSESSED' and t['D1']=='NOT_ACCEPTED' and t['six_candidate']=='NOT_FROZEN' and t['old_authorization_reused'] is False,'terminal extent/limits differ')
    window=load(B/'shared-readonly-window.json');begin=dt.datetime.fromisoformat(window['not_before']);end=dt.datetime.fromisoformat(window['expires_at']);need(end-begin==dt.timedelta(minutes=55) and window['audit_id']==m['audit_id'] and window['max_provider_deletes']==0 and window['send_authority']=='NONE','shared window scope differs')
    spec=importlib.util.spec_from_file_location('independent_result17',R/'examples/lifecycle-d1/verify-post-v22-seventeen.py');host=importlib.util.module_from_spec(spec);spec.loader.exec_module(host);host.p1.CONTROL=D
    # This composition is read-only: source/input fingerprints, stopped inspect,
    # exact volume ownership, budget cp and sealed gateway/backup hashes only.
    host.fresh(m)
    need(sha(C/'checker.exe')==m['checker_sha256'],'checker binary drift')
    groups=[];allids=set();statuscounts=collections.Counter();methods=collections.Counter();allcopies=[]
    for group,count,requests in zip(host.GROUPS,host.COUNTS,(32,24,18)):
        f=B/group;ro=f/'readonly';o=m['owners'][group];receipt=load(ro/'post-v22-native-verification.json');e=load(f/'expectation.json');a=load(f/'aggregation.json');owner=load(f/'owner.json');seal=load(f/'physical-close.json')
        need(e['not_before']==window['not_before'] and e['expires_at']==window['expires_at'] and e['audit_id']==m['audit_id'],'per-group shared window differs')
        original=load(C/(group+'-disabled-original.json'));active=dict(original,Enabled=True,NotBefore=window['not_before'],ExpiresAt=window['expires_at']);active_sha=hashlib.sha256((json.dumps(active,indent=2)+'\n').encode()).hexdigest()
        need(active_sha==e['profile_sha256']==receipt['profile_sha256'],'active profile original+three allowed fields differs')
        need(load(pathlib.Path(o['private'])/'post-v22-readonly.json')==original and original['Enabled'] is False,'disabled original not restored')
        st=dt.datetime.fromisoformat(receipt['started_at'].replace('Z','+00:00'));fi=dt.datetime.fromisoformat(receipt['finished_at'].replace('Z','+00:00'));os=dt.datetime.fromisoformat(owner['started_at'].replace('Z','+00:00'));of=dt.datetime.fromisoformat(owner['finished_at'].replace('Z','+00:00'))
        need(begin<=os<=st<=fi<=of<end and owner['exit']==0 and owner['closed'] and owner['cid']==o['cid'] and owner['binary_sha256']==o['binary_sha256'],'actual owner execution times/status differ')
        starts={q.stem.removeprefix('readonly-start-'):load(q) for q in ro.glob('readonly-start-*.json')};obs={q.stem.removeprefix('readonly-observation-'):load(q) for q in ro.glob('readonly-observation-*.json')}
        need(len(starts)==len(obs)==requests and set(starts)==set(obs),'actual request membership incomplete')
        p1fact=None
        for id,s in starts.items():
            z=obs[id];need(str(uuid.UUID(id))==id and id not in allids,'request UUID collision');allids.add(id)
            need(s['id']==z['id']==id and s['audit_id']==z['audit_id']==m['audit_id'] and s['method'] in ('GET','HEAD') and not z['transport_error'],'actual request identity/method/transport differs')
            a1=dt.datetime.fromisoformat(s['at'].replace('Z','+00:00'));a2=dt.datetime.fromisoformat(z['at'].replace('Z','+00:00'));need(st<=a1<=a2<=fi,'request outside actual helper interval')
            need(z['status'] in (200,400,404),'unexpected actual response status')
            if z['status']==404:
                need(group=='business-races' and receipt['p1_absence']['attempt_id']==id and s['method']=='HEAD' and z['provider_error_code']=='NoSuchVersion' and z['provider_request_id'],'untyped or unexpected404')
                p1fact={'start':s,'observation':z,'receipt':receipt['p1_absence']}
            methods[s['method']]+=1;statuscounts[z['status']]+=1
        q=safe_run([str(C/'checker.exe'),'-post-v22','-baseline',str(host.p1.PACKET/group/'readonly'),'-fresh',str(ro),'-group',group,'-expectation',str(f/'expectation.json'),'-inventory',str(pathlib.Path(o['private'])/'present-inventory.json')],capture_output=True,timeout=45)
        for name,raw in (('stdout',q.stdout),('stderr',q.stderr)):
            with (D/(group+'-result-checker01-'+name+'.txt')).open('xb') as out:out.write(raw)
        need(q.returncode==0,'independent actual file-only aggregation failed');check=json.loads(q.stdout)
        need(check=={k:v for k,v in a.items() if k not in ('technical_requests','amount','native_delete','business_mutations')} and a in t['groups'],'terminal actual aggregation mismatch')
        need(check['copies']==count and check['provider_io']==0 and check['delete']==0 and all(z['official_equals'] for z in check['policies']),'independent policy/copy verification denied')
        before=load(f/'Core-before.json');after=load(f/'Core-after.json');baseline=R/'docs/acceptance/evidence/issue307-d1/post-v22-business-closure03/Core-after.json' if group=='business-races' else host.p1.PACKET/group/'core-readonly.json'
        need(before==after==load(baseline),'Core relations/roles/ACL baseline before after differ')
        need(set(before)=={'relations','roles_acl','all_roles_sha256','schema_function_default_acl_sha256'},'Core fingerprint fields missing')
        need(seal['all_closed'] and seal['profiles_restored'] and seal['resources_retained'] and seal['cleanup']==0 and len(seal['owners'])==3 and all(z['closed'] for z in seal['owners']),'per-group final closure incomplete')
        need((f/'wait-stdout.txt').read_bytes().strip()==b'0' and (f/'wait-stderr.txt').read_bytes()==b'' and (f/'checker-stderr.txt').read_bytes()==b'' and (f/'stderr.txt').read_bytes()==b'','actual raw channels indicate failure')
        need(json.loads((f/'checker-stdout.txt').read_bytes())==check,'original checker raw stdout differs')
        copies=receipt['verification']['copies'];need(copies==load(pathlib.Path(o['private'])/'present-inventory.json'),'17 actual receipt content/protection mismatch');allcopies+=copies
        groups.append({'group':group,'copies':count,'technical_requests':requests,'method_counts':dict(collections.Counter(s['method'] for s in starts.values())),'status_counts':dict(collections.Counter(z['status'] for z in obs.values())),'actual_helper':owner,'receipt_sha256':sha(ro/'post-v22-native-verification.json'),'Core_before_sha256':sha(f/'Core-before.json'),'Core_after_sha256':sha(f/'Core-after.json'),'Core_baseline_sha256':sha(baseline),'Core_relation_count':len(before['relations']),'Core_roles_acl_fields_verified':True,'file_only_aggregation':check,'file_only_provider_IO':0,'actual_P1_absence':p1fact,'raw_channel_files':{n:sha(f/n) for n in ('stdout.txt','stderr.txt','wait-stdout.txt','wait-stderr.txt','checker-stdout.txt','checker-stderr.txt')}})
    need(len(allids)==74 and len(allcopies)==17 and len({z['Copy']['ID'] for z in allcopies})==17,'global request/copy membership differs')
    need(len(t['physical_close'])==9 and all(z['closed'] for z in t['physical_close']) and len(t['disabled_profiles'])==3 and all(z['disabled_readback'] for z in t['disabled_profiles']),'terminal closure incomplete')
    zip_path=C/'approved-runtime-source.zip';arc=load(B/'source-archive01.json');need(sha(zip_path)==arc['archive_sha256']=='708917b381b24047ca28444d70fb5c2f0e22da6c2722eb02d6f3d4b5533dc76b' and arc['source_files']==1914 and arc['source_head']==H,'archive binding differs')
    with zipfile.ZipFile(zip_path) as z:
        names=z.namelist();need(len(names)==len(set(names))==1914 and set(names)==set(m['source_files']),'archive source membership differs')
        need(all(hashlib.sha256(z.read(n)).hexdigest()==m['source_files'][n] for n in names),'archive source bytes differ')
    result.update(status='PASS_ACTUAL_POST_V22_SEVENTEEN_EVIDENCE_AUDIT',index_members_verified=len(idx),source_archive_members_verified=1914,source_archive_sha256=sha(zip_path),source_archive_receipt_sha256=sha(B/'source-archive01.json'),groups=groups,actual_technical_requests=len(allids),actual_method_counts=dict(methods),actual_status_counts=dict(statuscounts),amount='NOT_ASSESSED',copies=allcopies,Core_all_groups_unchanged=True,old_preservation_actual_equals_manifest=True,old_closed_owner_count=87,new_closed_helpers=3,unique_closed_total=90,rawchannels='Original indexed bytes verified, each checker/wait successful, no redaction/rewrite',DELETE=0,business_mutations=0,six_candidate='NOT_FROZEN',D1='NOT_ACCEPTED',runtime_native_evidence='Actual parent run, independently audited and file-only reaggregated; reviewer did not call provider or rerun source')
except Exception as error:
    result['status']='BLOCKED_OR_UNCONFIRMED';result['remaining_blockers']=[type(error).__name__+': '+str(error)]
finally:
    result['completed_at']=dt.datetime.now(dt.timezone.utc).isoformat();result['independent_readonly_commands']=commands;save('result-actual-evidence-audit01.json',result)
    print(json.dumps({'status':result['status'],'remaining_blockers':result['remaining_blockers'],'receipt_sha256':sha(D/'result-actual-evidence-audit01.json'),'readonly_commands':len(commands)}))
