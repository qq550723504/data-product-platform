import sys
sys.dont_write_bytecode=True
import datetime as dt,hashlib,importlib.util,json,pathlib,subprocess,traceback
R=pathlib.Path(__file__).resolve().parents[7]
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
result={'reviewed_at':dt.datetime.now(dt.timezone.utc).isoformat(),'status':'IN_PROGRESS','manifest_sha256':sha(P),'source_head':H,'remaining_blockers':[],'scope':'Independent actual stopped manifest binding; no start/stop/exec/provider/activation/signing'}
try:
    need(sha(P)==MS and sha(C/'manifest.json')==MS,'manifest raw bytes differ')
    m=load(P);srpath=D/'source-final-review01.json';s=load(srpath)
    need(sha(srpath)==SR and s['status']=='PASS_BOUNDED_INDEPENDENT_POST_V22_SEVENTEEN_SOURCE_REVIEW' and s['remaining_blockers']==[] and s['source_head']==H,'source review binding differs')
    need(m['source_files']==s['source_files'] and len(m['source_files'])==1914,'source file set differs')
    need(all(sha(R/n)==h for n,h in m['source_files'].items()),'actual source SHA drift')
    need(all(sha(C/n)==h for n,h in m['inputs'].items()) and len(m['inputs'])==17,'actual immutable input SHA drift')
    spec=importlib.util.spec_from_file_location('independent_actual17',R/'examples/lifecycle-d1/verify-post-v22-seventeen.py')
    host=importlib.util.module_from_spec(spec);spec.loader.exec_module(host)
    host.p1.CONTROL=D # Any failed read diagnostics belong only to this independent directory.
    host.fresh(m)
    result['fresh_gate']='PASS';result['source_files_verified']=1914;result['immutable_inputs_verified']=17
    need(list(m['owners'])==list(host.GROUPS) and m['activation_fields']==sorted(host.ACTIVATION) and m['native_delete']==0 and m['business_mutations']==0 and m['send_authority']=='NONE' and m['old_authorization_reused'] is False,'manifest finite authority differs')
    c,b=host.closure.packet();groups={g['Group']:g for g in c['Groups']}
    expected_fields={'Enabled','AuditID','Purpose','Group','CandidateSHA256','BinarySHA256','Core','Native','Network','Endpoint','RootAccess','RootSecret','FrozenInventorySHA256','PresentInventorySHA256','NotBefore','ExpiresAt','MaxProviderDeletes'}
    owners=[];fc=pc=absent=0
    for name,o in m['owners'].items():
        x=host.owner_contract(o);private=pathlib.Path(o['private']);profile=load(private/'post-v22-readonly.json');original=load(C/(name+'-disabled-original.json'))
        need(set(profile)==expected_fields and profile==original,'profile exact field set/original differs')
        need(profile['Enabled'] is False and type(profile['MaxProviderDeletes']) is int and profile['MaxProviderDeletes']==0 and profile['NotBefore']==profile['ExpiresAt']=='0001-01-01T00:00:00Z','profile not disabled/zero delete/zero time')
        need(profile['AuditID']==m['audit_id'] and profile['Purpose']=='POST_V22_SEVENTEEN_NATIVE_READONLY' and profile['Group']==name and profile['CandidateSHA256']==sha(host.p1.PACKET/'candidate.json') and profile['BinarySHA256']==o['binary_sha256'],'profile generation/binary binding differs')
        need(all(profile[k]==o[k] for k in ('Core','Native','Network')) and profile['Endpoint']=='127.0.0.1:9000' and profile['RootAccess'] and profile['RootSecret'],'profile fixed source/root missing')
        old=load(host.p1.PRIVATE/'remaining-executable03'/name/'source-verifier/remaining.json')['Base']
        need(profile['RootAccess']==old['RootAccess'] and profile['RootSecret']==old['RootSecret'],'profile source credential identity differs')
        frozen=load(private/'frozen-inventory.json');present=load(private/'present-inventory.json')
        need(profile['FrozenInventorySHA256']==sha(private/'frozen-inventory.json') and profile['PresentInventorySHA256']==sha(private/'present-inventory.json'),'inventory hashes differ')
        need([z['Copy'] for z in frozen]==groups[name]['Copies'],'frozen inventory/candidate differs')
        removed=[z for z in frozen if z['Copy']['ID']==host.P1]
        need(present==[z for z in frozen if z['Copy']['ID']!=host.P1] and len(present)==host.COUNTS[host.GROUPS.index(name)] and len(removed)==(1 if name=='business-races' else 0),'17/p1 exact difference differs')
        fc+=len(frozen);pc+=len(present);absent+=len(removed)
        need(x['State']['Status']=='created' and x['State']['StartedAt'].startswith('0001-') and x['State']['Pid']==0,'new owner was previously started')
        need(not x['Config'].get('Env') or all(not any(k in v.lower() for k in ('rootsecret=','token=','dsn=','wirekey=','delete_secret=')) for v in x['Config']['Env']),'credential environment expansion')
        need(list(pathlib.Path(o['output']).iterdir())==[],'new helper output not empty')
        owners.append({'group':name,'cid':o['cid'],'image':x['Image'],'user':x['Config']['User'],'entrypoint':x['Config']['Entrypoint'],'argv':x['Config']['Cmd'],'state':{k:x['State'][k] for k in ('Status','Running','Pid','StartedAt','FinishedAt','ExitCode')},'network_mode':x['HostConfig']['NetworkMode'],'rootfs_readonly':x['HostConfig']['ReadonlyRootfs'],'capdrop':x['HostConfig']['CapDrop'],'security_opt':x['HostConfig']['SecurityOpt'],'restart':x['HostConfig']['RestartPolicy'],'tmpfs':x['HostConfig']['Tmpfs'],'mounts':[{k:z[k] for k in ('Type','Source','Destination','RW')} for z in x['Mounts']],'exact_profile_fields':sorted(profile),'profile_sha256':sha(private/'post-v22-readonly.json'),'Enabled':False,'MaxProviderDeletes':0,'NotBefore':profile['NotBefore'],'ExpiresAt':profile['ExpiresAt'],'root_credential':'MATCHED_OLD_TECHNICAL_SOURCE_NOT_EMITTED','business_credentials':'ABSENT','frozen_versions':len(frozen),'present_versions':len(present),'absent_copy_ids':[z['Copy']['ID'] for z in removed]})
    need((fc,pc,absent)==(18,17,1),'global inventory counts differ')
    for name,path,system,main in [('reader',C/'reader','linux','lifecycle-fixture-reader'),('checker',C/'checker.exe','windows','lifecycle-fixture-readonly-check')]:
        q=safe_run(['go','version','-m',str(path)],capture_output=True,cwd=R);need(q.returncode==0,'build metadata unavailable')
        t=q.stdout.decode('utf-8','strict');need('github.com/yourorg/data-product-platform/apps/platform/cmd/'+main in t and 'GOOS='+system in t and 'GOARCH=amd64' in t and 'CGO_ENABLED=0' in t,'build metadata target differs')
        with (D/('manifest-'+name+'-buildinfo01.txt')).open('xb') as f:f.write(q.stdout)
        result[name+'_buildinfo']={'sha256':sha(path),'target_os':system,'arch':'amd64','CGO_ENABLED':0,'main':main,'raw_metadata_sha256':hashlib.sha256(q.stdout).hexdigest(),'vcs_revision_present':'vcs.revision=' in t}
    need(not (P.parent/'terminal.json').exists() and not (C/'terminal.json').exists(),'audit already terminal')
    result.update(status='PASS_ACTUAL_DISABLED_MANIFEST_BINDINGS',owners=owners,frozen_versions=fc,present_versions=pc,p1_absent_difference=absent,old_closed_owners=len(m['preservation']['all_87_closed_cids']),old_control_files=len(m['preservation']['old_retained']['old']['old_control_sha256']),old_readonly_profiles=len(m['preservation']['old_retained']['readonly_profile_sha256']),backups=len(m['preservation']['old_retained']['backup_sha256']),budget_volumes=len(m['preservation']['old_budget_mounts']),budget_content_anchor=m['preservation']['old_retained']['old_budgets'],preservation_sha256=hashlib.sha256(json.dumps(m['preservation'],sort_keys=True).encode()).hexdigest(),sealed_gateway='3 hashes and stopped CIDs verified by actual retained gate',Core_baseline={'business-races':{'path':'docs/acceptance/evidence/issue307-d1/post-v22-business-closure03/Core-after.json','sha256':sha(R/'docs/acceptance/evidence/issue307-d1/post-v22-business-closure03/Core-after.json')},'late-hold':{'sha256':sha(host.p1.PACKET/'late-hold/core-readonly.json')},'mixed':{'sha256':sha(host.p1.PACKET/'mixed/core-readonly.json')}},source_started=False,provider_calls=0,DELETE=0,actual17_native='NOT_RUN',Core_fresh_query='NOT_RUN_NO_DOCKER_EXEC',new_six_candidate='NOT_FROZEN')
except Exception as error:
    result['status']='BLOCKED_OR_UNCONFIRMED';result['remaining_blockers']=[type(error).__name__+': '+str(error)]
finally:
    result['completed_at']=dt.datetime.now(dt.timezone.utc).isoformat();result['read_only_commands']=commands;save('manifest-actual-binding-audit01.json',result)
    print(json.dumps({'status':result['status'],'remaining_blockers':result['remaining_blockers'],'read_only_commands':len(commands),'receipt_sha256':sha(D/'manifest-actual-binding-audit01.json')}))
