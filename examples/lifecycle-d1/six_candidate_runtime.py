"""Fixed non-delete preparation primitives for candidate19.

No send permission writer, authorization loader or S3 DELETE exists here.
Allocation, budget initialization, native preparation and freeze are separate
one-shot checkpoints. All original bytes and failed owners are retained.
"""
import copy
import datetime as dt
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import secrets
import shutil
import subprocess
import tarfile
import time
import uuid
from urllib.parse import urlsplit,urlunsplit

spec=importlib.util.spec_from_file_location('six_constructor',Path(__file__).with_name('prepare-six-candidate.py'))
p=importlib.util.module_from_spec(spec);spec.loader.exec_module(p)
ROOT,CONTROL,PUBLIC=p.ROOT,p.CONTROL,p.PUBLIC
import evidence_indexes
need,load,sha,save=p.need,p.load,p.sha,p.save
docker,inspect,closed=p.docker,p.inspect,p.closed
ZERO=p.ZERO


def now():return dt.datetime.now(dt.timezone.utc)


def replace(path,value):p.prior.replace_profile(path,value)


def dsn_for_restore(value,project):
    x=urlsplit(value)
    need(x.scheme=='postgres' and x.hostname==project+'-postgres' and x.port==5432 and x.path=='/lifecycle307' and x.query=='sslmode=disable','source DSN differs')
    return urlunsplit(x._replace(netloc=x.netloc.rsplit('@',1)[0]+'@'+project+'-'+p.GENERATION+'-rehearsal:5432'))


def write_raw(path,raw):
    with path.open('xb') as stream:stream.write(raw)


def project_owner(item):
    # Configuration/mount projection only. Do not publish Config.Env, which may
    # contain retained source database passwords or provider root credentials.
    return p.project_owner(item)


def unprivileged(item,entry,mode,netns):
    h=item['HostConfig']
    need(item['Image']==p.p1.PG and item['Config']['User']=='10001:10001'
         and item['Config']['Entrypoint']==[entry] and item['Config']['Cmd']==mode
         and h['NetworkMode']=='container:'+netns and h['ReadonlyRootfs'] and not h['Privileged']
         and not h.get('AutoRemove') and h['RestartPolicy']['Name']=='no' and h['CapDrop']==['ALL']
         and not h.get('CapAdd') and 'no-new-privileges' in h['SecurityOpt']
         and h.get('Tmpfs')=={'/tmp':'','/var/lib/postgresql/data':'size=1m'}
         and not h.get('PortBindings') and not any(item['NetworkSettings']['Ports'].values()),'worker image/command/security differs')
    need(not any('docker.sock' in m['Source'].lower() or 'docker_engine' in m['Source'].lower() for m in item['Mounts']),'socket mount exposed')
    need(not any(e.startswith(('MINIO_ROOT_','ROOT_ACCESS=','ROOT_SECRET=')) for e in item['Config']['Env']),'worker root secret environment')


def mounts_exact(item,expected):
    mounts={m['Destination']:m for m in item['Mounts']}
    need(len(mounts)==len(item['Mounts'])==len(expected) and set(mounts)==set(expected),'mount membership differs')
    for target,(kind,path,rw) in expected.items():
        m=mounts[target]
        need(m['Type']==kind and m['RW'] is rw,'mount type or mode differs')
        if kind=='volume':need(m['Name']==path,'mount volume differs')
        else:need(p.p1.hostpath(m['Source'])==Path(path).resolve(),'mount source differs')


def allocation_state():
    a=load(CONTROL/'allocation.json')
    need(a['execution']==p.EXECUTION and a['profile']==p.PROFILE and a==load(PUBLIC/'allocation.json'),'allocation differs')
    need(a['policy_checkpoint_sha256']==p.window.POLICY_CHECKPOINT_SHA256,'allocation current policy checkpoint differs')
    for name,digest in a['binaries'].items():need(sha(CONTROL/name)==digest,'runtime binary drift')
    need(sha(CONTROL/'six-map.json')==sha(PUBLIC/'six-map.json')==a['map_sha256'],'source/restore map drift')
    need(p.historical_gate()==a['retained'],'retained history drift')
    return a


BUILD_MEMBERS={'reader','operator','owner','readonly-check.exe'}|{name+'-build.'+channel for name in ('reader','operator','owner','checker') for channel in ('stdout','stderr')}


def build_contract(a):
    need(len(a['source'])==40 and all(c in '0123456789abcdef' for c in a['source']),'build source is not a commit')
    need(subprocess.check_output(['git','cat-file','-t',a['source']],cwd=ROOT).strip()==b'commit','initial build source is not an existing commit')
    need(subprocess.run(['git','diff','--quiet',a['source'],'--','apps/platform','examples/lifecycle-d1','migrations'],cwd=ROOT).returncode==0,'runtime tracked source drift')
    need(not subprocess.check_output(['git','ls-files','--others','--exclude-standard','apps/platform','examples/lifecycle-d1','migrations'],cwd=ROOT).strip(),'runtime untracked source drift')
    need(set(a['build_files'])==BUILD_MEMBERS,'initial build membership differs')
    for name,digest in a['build_files'].items():
        path=CONTROL/name
        need(path.is_file() and not path.is_symlink() and not path.is_junction() and path.stat().st_nlink==1 and sha(path)==digest,'initial build member bytes/type/link differs')
    need(all(a['build_files'][name]==digest for name,digest in a['binaries'].items()),'active binary differs from initial build')
    return {'source':a['source'],'files':a['build_files']}


def state():
    a=allocation_state();build_contract(a)
    return a


def direct_network(item,network,network_name):
    need(item['HostConfig']['NetworkMode']==network and set(item['NetworkSettings']['Networks'])=={network_name},'direct network configured identity differs')
    endpoint=item['NetworkSettings']['Networks'][network_name]
    if endpoint['NetworkID']==network:return
    state=item['State']
    need(endpoint['NetworkID']=='' and state['Status']=='created' and state['Running'] is False and state['Pid']==0
         and state['StartedAt']==ZERO and state['FinishedAt']==ZERO
         and all(endpoint.get(key)=='' for key in ('EndpointID','Gateway','IPAddress','MacAddress','IPv6Gateway','GlobalIPv6Address'))
         and endpoint.get('IPPrefixLen')==0 and endpoint.get('GlobalIPv6PrefixLen')==0,'missing runtime network identity is not a never-started empty endpoint')


def initializer_security(x):
    h=x['HostConfig']
    need(x['Image']==p.p1.PG and x['Config']['User']=='0:0' and x['Config']['Entrypoint']==['/bin/sh'] and x['Config']['Cmd']==['-e','/run/budget-init.sh'] and h['NetworkMode']=='none'
         and h['ReadonlyRootfs'] and h['CapDrop']==['ALL'] and h['CapAdd']==['CAP_CHOWN'] and not h['Privileged'] and h['RestartPolicy']['Name']=='no' and not h.get('PortBindings'),'budget initializer expanded')


def cold_topology(a):
    need([g['group'] for g in a['groups']]==list(p.GROUPS),'finite preparation group membership differs')
    need(all(closed(inspect(cid)) for cid in a['owners']),'new owner is active')
    need(sha(PUBLIC/'owner-descriptors.json')==a['owner_descriptors_sha256'],'approved allocation descriptor drift')
    descriptors=load(PUBLIC/'owner-descriptors.json')
    need(set(descriptors)==set(a['owners']) and len(a['owners'])==len(set(a['owners'])),'allocation owner membership differs')
    for cid in a['owners']:
        x=inspect(cid)
        need(project_owner(x)==descriptors[cid] and not x['HostConfig']['Privileged'] and not x['HostConfig'].get('PortBindings')
             and x['HostConfig']['RestartPolicy']['Name']=='no' and not any(x['NetworkSettings']['Ports'].values()),'actual owner configuration differs from approved allocation')
        need(x['Config']['Labels'].get('lifecycle307.remaining')==p.EXECUTION and x['Config']['Labels'].get('lifecycle307.allocation')==a['allocation_nonce'],'allocated owner label differs')
    expected_volumes={v for g in a['groups'] for v in g['volumes'].values()}
    need(expected_volumes=={x['volume'] for x in a['resources'] if 'volume' in x},'complete new volume membership differs')
    source_volumes={g['source_volume'] for g in a['groups']}
    need(all(m['Name'] in expected_volumes or (m['Name'] in source_volumes and m['Destination']=='/source' and m['RW'] is False)
             for item in descriptors.values() for m in item['mounts'] if m['Type']=='volume'),'unbound anonymous volume or source write mount exposed')
    for volume_name in expected_volumes:
        volume=json.loads(docker('volume','inspect',volume_name))[0]
        expected={cid for cid,item in descriptors.items() for m in item['mounts'] if m['Type']=='volume' and m['Name']==volume_name}
        mounted=docker('ps','-aq','--no-trunc','--filter','volume='+volume_name).decode('utf-8','strict').split()
        need(volume['Name']==volume_name and volume['Driver']=='local' and volume['Scope']=='local' and not volume.get('Options')
             and volume['Labels'].get('lifecycle307.remaining')==p.EXECUTION and volume['Labels'].get('lifecycle307.allocation')==a['allocation_nonce']
             and len(mounted)==len(set(mounted)) and set(mounted)==expected,'new restore/queue/budget volume ownership or complete mount graph differs')
    old=load(p.p1.PACKET/'candidate.json')
    for g in a['groups']:
        group=g['group'];folder=Path(g['private']);scope=g['scope']
        expected_helpers={'wire',*[stage for name,stage,_ in p.sequence.ORDER if name==group],*['recover-'+stage for name,stage,_ in p.sequence.ORDER if name==group]}
        need(set(g['helpers'])==expected_helpers and set(g['cases'])==set(p.window.CASE_MODES[group])
             and set(g['root_owners'])=={'check-disabled','enable-read','enable-send','disable-send','disable-all'}
             and set(g['maintenance'])=={'SOURCE','RESTORE','PROVISION','restore-inspect-enable','restore-inspect-disable'},'finite owner role membership differs')
        source=next(x for x in old['Groups'] if x['Group']==group)
        need(g['source']=={k:source[k] for k in ('Core','Native','Network')},'original source identity changed')
        p.prior.closure.source_contract({'source':source})
        restore_name=None
        for network_id in (source['Network'],g['restore_network']):
            n=json.loads(docker('network','inspect',network_id))[0]
            need(n['Id']==network_id and n['Internal'] and not n.get('Containers'),'network active, external or changed')
            if network_id==g['restore_network']:
                need(n['Labels'].get('lifecycle307.remaining')==p.EXECUTION and n['Labels'].get('lifecycle307.allocation')==a['allocation_nonce'] and n['Driver']=='bridge' and n['Scope']=='local','restore network ownership differs')
                restore_name=n['Name']
        for stage,cid in g['helpers'].items():
            x=inspect(cid);unprivileged(x,'/run/operator',['-stage',stage],g['gateway'])
            mounts_exact(x,{'/run/private':('bind',folder/stage,False),'/run/operator':('bind',CONTROL/'operator',False),'/run/output':('bind',folder/'outputs'/stage,True),'/run/budget':('volume',g['volumes']['budget'],True)})
        for stage,cid in g['cases'].items():
            x=inspect(cid);unprivileged(x,'/run/reader',['-mode',p.window.CASE_MODES[group][stage]],g['gateway'])
            mounts_exact(x,{'/run/private':('bind',folder/('case-'+stage),False),'/run/reader':('bind',CONTROL/'reader',False),'/run/output':('bind',folder/'outputs'/('case-'+stage),True)})
        for action,cid in g['root_owners'].items():
            x=inspect(cid);unprivileged(x,'/run/owner',['-action',action],source['Native'])
            mounts_exact(x,{'/run/private':('bind',folder/('root-'+action),False),'/run/owner':('bind',CONTROL/'owner',False),'/run/output':('bind',folder/'outputs'/('root-'+action),True)})
        for role,cid in g['maintenance'].items():
            mode='six-verify-native' if role in ('SOURCE','RESTORE') else ('six-provision-disabled' if role=='PROVISION' else 'six-'+role)
            stage='maintenance-'+role.lower() if role in ('SOURCE','RESTORE','PROVISION') else role
            netns=source['Native'] if role in ('SOURCE','PROVISION') else g['restore_native']
            x=inspect(cid);unprivileged(x,'/run/reader',['-mode',mode],netns)
            mounts_exact(x,{'/run/private':('bind',folder/stage,False),'/run/reader':('bind',CONTROL/'reader',False),'/run/output':('bind',folder/'outputs'/stage,True)})
        for stage,cid in g['rehearsals'].items():
            mode={'business':'remaining-resume-business','late':'remaining-late','mixed':'remaining-mixed','protected':'remaining-mixed-protected'}[stage]
            x=inspect(cid);unprivileged(x,'/run/reader',['-mode',mode],g['restore_gateway'])
            mounts_exact(x,{'/run/private':('bind',folder/('rehearsal-'+stage),False),'/run/reader':('bind',CONTROL/'reader',False),'/run/output':('bind',folder/'outputs'/('rehearsal-'+stage),True)})
        for cid in (g['restore_core'],g['rehearsal_core'],g['restore_native']):
            x=inspect(cid)
            direct_network(x,g['restore_network'],restore_name)
            need(cid not in (source['Core'],source['Native']) and x['HostConfig']['RestartPolicy']['Name']=='no' and not x['HostConfig'].get('PortBindings') and not any(x['NetworkSettings']['Ports'].values()),'restore source alias or exposed')
        volume=json.loads(docker('volume','inspect',g['volumes']['budget']))[0]
        mounted=docker('ps','-aq','--no-trunc','--filter','volume='+volume['Name']).decode('utf-8','strict').split()
        need(volume['Labels'].get('lifecycle307.remaining')==p.EXECUTION and len(mounted)==len(set(mounted)) and set(mounted)==set(g['budget_expected_mounted_cids']),'new budget ownership/mount graph expanded')
        x=inspect(g['budget_initializer']);initializer_security(x)
        mounts_exact(x,{'/run/budget':('volume',g['volumes']['budget'],True),'/run/budget-init.sh':('bind',folder/'budget-init.sh',False)})
        need(sha(folder/'budget-init.sh')==g['budget_initializer_script_sha256'],'budget initializer code changed')
    return {'status':'PASS_COLD_SIX_TOPOLOGY','owners':len(a['owners']),'source_provider_io':0,'native_delete':0}


def profile(g,role,purpose,audit):
    folder=Path(g['private']);mapping=load(CONTROL/'six-map.json')
    row=next(x for x in mapping['Groups'] if x['Group']==g['group']);source=row['Source'] if role=='SOURCE' else row['Restore']
    original=load(p.p1.PRIVATE/('dpp-lifecycle307-'+g['scope']['Run'][:8])/'config.json')
    return dict(Enabled=False,AuditID=audit,Purpose=purpose,Group=g['group'],CandidateSHA256=sha(CONTROL/'six-map.json'),BinarySHA256=sha(CONTROL/'reader'),
                Core=source[0],Native=source[1],Network=source[2],Endpoint='127.0.0.1:9000',RootAccess=original['RootAccess'],RootSecret=original['RootSecret'],
                FrozenInventorySHA256=sha(folder/'frozen-inventory.json'),PresentInventorySHA256=sha(folder/'present-inventory.json'),NotBefore=ZERO,ExpiresAt=ZERO,MaxProviderDeletes=0,
                Execution=p.EXECUTION,Profile=p.PROFILE,Role=role,MapSHA256=sha(CONTROL/'six-map.json'))


def materialize():
    a=state();cold_topology(a)
    need(not (PUBLIC/'profiles.json').exists(),'one-shot profiles already exist')
    rows=[];old=load(p.p1.PACKET/'candidate.json');audit=str(uuid.uuid4())
    for g in a['groups']:
        group=g['group'];folder=Path(g['private']);project='dpp-lifecycle307-'+g['scope']['Run'][:8]
        original=p.p1.PRIVATE/'remaining-executable03'/group/'source-verifier'
        frozen=(original/'inventory.json').read_bytes();write_raw(folder/'frozen-inventory.json',frozen)
        save(folder/'present-inventory.json',[r for r in json.loads(frozen) if r['Copy']['ID']!=p.prior.P1])
        baseline=PUBLIC/group/'expectation'
        baseline.parent.mkdir()
        result=subprocess.run([str(CONTROL/'readonly-check.exe'),'-six-expectation','-baseline',str(p.p1.PACKET/group/'readonly'),'-fresh',str(baseline),'-group',group,'-inventory',str(folder/'present-inventory.json'),'-frozen',str(folder/'frozen-inventory.json')],capture_output=True)
        write_raw(baseline.parent/'expectation-stdout.txt',result.stdout);write_raw(baseline.parent/'expectation-stderr.txt',result.stderr)
        need(result.returncode==0,'file-only expectation generation failed')
        for role in g['maintenance']:
            stage='maintenance-'+role.lower() if role in ('SOURCE','RESTORE','PROVISION') else role
            target=folder/stage
            purpose='SIX_PREPARATION_PROVISION_DISABLED_ONLY' if role=='PROVISION' else ('SIX_RESTORE_INSPECTOR_NONDELETE_REHEARSAL' if role.startswith('restore-inspect-') else 'SIX_TARGET_PREPARATION_NATIVE_READONLY')
            filename='six-provision.json' if role=='PROVISION' else ('restore-inspector.json' if role.startswith('restore-inspect-') else 'six-readonly.json')
            item=profile(g,'SOURCE' if role in ('SOURCE','PROVISION') else 'RESTORE',purpose,audit)
            save(target/filename,item)
            for name in ('frozen-inventory.json','present-inventory.json'):write_raw(target/name,(folder/name).read_bytes())
            write_raw(target/'six-map.json',(CONTROL/'six-map.json').read_bytes())
            if role=='PROVISION':write_raw(target/'new-secrets.json',(folder/'new-secrets.private.json').read_bytes())
            rows.append({'group':group,'role':role,'filename':filename,'sha256':sha(target/filename),'enabled':False})
        old_group=next(x for x in old['Groups'] if x['Group']==group)
        heads=p.window.policy_heads(group)
        template=load(p.p1.PRIVATE/'remaining-executable03'/group/'source-owner/remaining.json')
        base=p.window.case_profile(template,heads,secrets.token_hex(32),template['Base']['Epoch'])
        base['Rehearsal']=False;base['Checkpoint']=''
        access=p.identity(g['scope'],'inspect');secret=load(folder/'new-secrets.private.json')[access]
        base['Base'].update(Access=access,Secret=secret,InspectAccess=access,InspectSecret=secret,OutputDirectory='/run/output')
        for name in ('ProducerAccess','ProducerSecret','RootAccess','RootSecret'):base['Base'].pop(name,None)
        for stage in g['cases']:
            target=folder/('case-'+stage);save(target/'remaining.json',base)
            write_raw(target/'manifest.json',(original/'manifest.json').read_bytes())
        for stage in g['rehearsals']:
            target=folder/('rehearsal-'+stage);trial=copy.deepcopy(base)
            trial.update(Rehearsal=True,Checkpoint=p.GENERATION)
            trial['Base'].update(Enabled=False,StartedAt=ZERO,ExpiresAt=ZERO,PostgresDSN=dsn_for_restore(base['Base']['PostgresDSN'],project))
            save(target/'remaining.json',trial);write_raw(target/'manifest.json',(original/'manifest.json').read_bytes())
        # All sending/root controls remain false even though the independently
        # time-bound non-delete case profile describes the proposed window.
        for stage in g['helpers']:
            save(folder/stage/'permission.json',{'Enabled':False})
        for action in g['root_owners']:save(folder/('root-'+action)/'permission.json',{'Enabled':False})
    save(PUBLIC/'profiles.json',{'audit_id':audit,'profiles':rows,'send_permissions':False,'native_delete':0,'authorization':'NOT_RECEIVED'})


def run_owner(cid,destination):
    need(closed(inspect(cid)),'one-shot owner not initially stopped')
    destination.mkdir(parents=True,exist_ok=False)
    failure=None
    try:
        docker('start',cid)
        waited=subprocess.run(['docker','wait',cid],capture_output=True,timeout=790)
        write_raw(destination/'wait-stdout.txt',waited.stdout);write_raw(destination/'wait-stderr.txt',waited.stderr)
        need(waited.returncode==0 and waited.stdout.strip()==b'0','non-delete helper failed')
    except subprocess.TimeoutExpired as e:
        failure='TimeoutExpired: owner wait unconfirmed'
        write_raw(destination/'wait-stdout.txt',e.stdout or b'');write_raw(destination/'wait-stderr.txt',e.stderr or b'')
        raise
    except BaseException as e:
        failure=type(e).__name__+': '+str(e)
        raise
    finally:
        stdout=stderr=b''
        try:
            logs=subprocess.run(['docker','logs',cid],capture_output=True,timeout=30)
            stdout,stderr=logs.stdout,logs.stderr
            if logs.returncode:failure=failure or 'owner logs unavailable'
        except subprocess.TimeoutExpired as error:
            stdout,stderr=error.stdout or b'',error.stderr or b''
            failure=failure or 'owner logs timeout'
        except Exception:failure=failure or 'owner logs unconfirmed'
        result=p.p1.close_known(cid)
        write_raw(destination/'stdout.txt',stdout);write_raw(destination/'stderr.txt',stderr)
        save(destination/'owner.json',{'cid':cid,'closure':result,'failure':failure,'native_delete':0})
        need(result['closed'] and failure is None,'non-delete owner failure or physical closure unconfirmed')


def budget_candidate(a):
    """Use the same complete aggregate at initialization and the source gate."""
    old=load(p.p1.PACKET/'candidate.json')
    need([g['group'] for g in a['groups']]==list(p.GROUPS),'budget groups differ')
    groups=[]
    for g in a['groups']:
        stages={stage for group,stage,_ in p.sequence.ORDER if group==g['group']}
        need(set(g['helpers'])=={'wire'}|stages|{'recover-'+stage for stage in stages},'budget helper membership differs')
        groups.append({'Group':g['group'],'Copies':next(x for x in old['Groups'] if x['Group']==g['group'])['Copies'],'Helpers':g['helpers']})
    candidate={'Execution':p.EXECUTION,'Groups':groups}
    need(len(p.sequence.targets(candidate))==6,'budget aggregate does not bind six targets')
    return candidate


def uninitialized_budget(raw):
    need(isinstance(raw,bytes),'budget readback must be bytes')
    with tarfile.open(fileobj=io.BytesIO(raw),mode='r:*') as archive:
        members=archive.getmembers()
        need(len(members)==1 and members[0].isdir() and members[0].name.rstrip('/')=='budget'
             and members[0].uid==members[0].gid==0 and members[0].mode&0o200,'uninitialized new budget is not one empty root-owned directory')
    return {'uid':0,'gid':0,'files':0}


def initialize_budgets():
    a=state();cold_topology(a)
    need(not (PUBLIC/'budget-initialization.json').exists() and not (PUBLIC/'budget-initialization-start.json').exists(),'budget initialization already attempted')
    save(PUBLIC/'budget-initialization-start.json',{'execution':p.EXECUTION,'source':a['source'],'allocation_sha256':sha(PUBLIC/'allocation.json'),'initializers':[g['budget_initializer'] for g in a['groups']]})
    # All three must be genuinely new, empty and never-started before any start.
    for g in a['groups']:
        item=inspect(g['budget_initializer'])
        need(closed(item) and item['State']['Status']=='created' and item['State']['StartedAt']==item['State']['FinishedAt']==ZERO,'initializer already started or unknown')
        uninitialized_budget(docker('cp',g['helpers']['wire']+':/run/budget','-'))
    results=[];failure=None
    try:
        for g in a['groups']:
            run_owner(g['budget_initializer'],PUBLIC/g['group']/'budget-initialization')
            raw=docker('cp',g['helpers']['wire']+':/run/budget','-')
            write_raw(CONTROL/g['group']/'initial-budget.tar',raw)
            result=p.sequence.budget_archive(raw,budget_candidate(a),g['group'],initial=True)
            result.update(initializer=g['budget_initializer'],archive_sha256=sha(CONTROL/g['group']/'initial-budget.tar'),owner_receipt_sha256=sha(PUBLIC/g['group']/'budget-initialization/owner.json'))
            results.append(result)
    except BaseException as error:
        failure={'type':type(error).__name__,'message':str(error)};raise
    finally:
        closure=[p.p1.close_known(g['budget_initializer']) for g in a['groups']]
        complete=failure is None and len(results)==3 and all(x['closed'] for x in closure)
        save(PUBLIC/'budget-initialization.json',{'status':'PASS_NEW_BUDGET_INITIALIZATION' if complete else 'FAILED_OR_UNCONFIRMED_RETAINED','execution':p.EXECUTION,'source':a['source'],'allocation_sha256':sha(PUBLIC/'allocation.json'),'groups':results,'all_complete':complete,'failure':failure,'closure':closure,'native_delete':0,'old_budget_changed':False})
    need(complete,'new budget initialization closure unconfirmed')


def budget_completion(a):
    receipt=load(PUBLIC/'budget-initialization.json')
    need(receipt['status']=='PASS_NEW_BUDGET_INITIALIZATION' and receipt['all_complete'] is True and receipt['failure'] is None and receipt['execution']==p.EXECUTION and receipt['source']==a['source'] and receipt['allocation_sha256']==sha(PUBLIC/'allocation.json') and receipt['native_delete']==0 and receipt['old_budget_changed'] is False,'new budget initialization incomplete or cross-bound')
    need([row['group'] for row in receipt['groups']]==list(p.GROUPS) and len(receipt['closure'])==3 and {row['cid'] for row in receipt['closure']}=={g['budget_initializer'] for g in a['groups']} and all(row['closed'] is True for row in receipt['closure']),'new budget completion membership/closure differs')
    for g,row in zip(a['groups'],receipt['groups']):
        path=Path(g['private'])/'initial-budget.tar'
        need(row['initializer']==g['budget_initializer'] and row['archive_sha256']==sha(path),'initial budget bytes/owner differ')
        p.sequence.budget_archive(path.read_bytes(),budget_candidate(a),g['group'],initial=True)
        owner_folder=PUBLIC/g['group']/'budget-initialization';owner=load(owner_folder/'owner.json')
        need(row['owner_receipt_sha256']==sha(owner_folder/'owner.json') and owner['cid']==g['budget_initializer'] and owner['failure'] is None and owner['closure']['closed'] is True and (owner_folder/'wait-stdout.txt').read_bytes().strip()==b'0','initial budget owner receipt/exit differs')
    return receipt


def preparation_gate(a):
    # This semantic gate precedes permission reads and any service/provider I/O.
    for g in a['groups']:
        folder=Path(g['private'])
        for stage in g['cases']:
            cfg=load(folder/('case-'+stage)/'remaining.json');p.window.validate_case(cfg,p.W)
            need(cfg['Group']==g['group'] and cfg['NonDelete'] is True and cfg.get('Rehearsal',False) is False
                 and not any(cfg['Base'].get(k) for k in ('RootAccess','RootSecret','ProducerAccess','ProducerSecret')),'current case is not source-scoped and root-free')
            need({x.name for x in (folder/('case-'+stage)).iterdir()}=={'remaining.json','manifest.json'},'unbound case private file')
    cold_topology(a)
    for g in a['groups']:
        folder=Path(g['private'])
        for stage in g['helpers']:need(load(folder/stage/'permission.json')=={'Enabled':False},'sending permission enabled')
        for action in g['root_owners']:need(load(folder/('root-'+action)/'permission.json')=={'Enabled':False},'root permission enabled')
        packet=budget_candidate(a)
        p.sequence.budget_archive(docker('cp',next(iter(g['helpers'].values()))+':/run/budget','-'),packet,g['group'],initial=True)
    return {'status':'PASS_COMPLETE_PREPARATION_BEFORE_SOURCE_IO','cases':6,'source_provider_io':0,'native_delete':0}


def activate(path):
    original=load(path);need(original['Enabled'] is False,'maintenance already activated')
    start=now().replace(microsecond=0);end=start+dt.timedelta(minutes=50)
    active=dict(original,Enabled=True,NotBefore=start.isoformat().replace('+00:00','Z'),ExpiresAt=end.isoformat().replace('+00:00','Z'))
    replace(path,active)
    return original,active


def maintenance(g,role):
    stage='maintenance-'+role.lower() if role in ('SOURCE','RESTORE','PROVISION') else role
    filename='six-provision.json' if role=='PROVISION' else ('restore-inspector.json' if role.startswith('restore-inspect-') else 'six-readonly.json')
    folder=Path(g['private'])/stage;path=folder/filename
    index=next(x for x in load(PUBLIC/'profiles.json')['profiles'] if x['group']==g['group'] and x['role']==role)
    need(sha(path)==index['sha256'] and load(path)['Enabled'] is False,'disabled preparation profile drift')
    # Every activation gets a new exact public binding while the actual root
    # secret stays solely in its private owner directory.
    original,active=activate(path)
    target=PUBLIC/g['group']/stage
    try:
        run_owner(g['maintenance'][role],target/'operation')
        shutil.copytree(Path(g['private'])/'outputs'/stage,target/'files')
        if role in ('SOURCE','RESTORE'):
            expectation=dict(audit_id=active['AuditID'],profile_sha256=sha(path),binary_sha256=active['BinarySHA256'],candidate_sha256=active['CandidateSHA256'],
                             frozen_inventory_sha256=active['FrozenInventorySHA256'],present_inventory_sha256=active['PresentInventorySHA256'],core=active['Core'],native=active['Native'],network=active['Network'],
                             not_before=active['NotBefore'],expires_at=active['ExpiresAt'],Execution=p.EXECUTION,Profile=p.PROFILE,Role=role)
            save(target/'expectation.json',expectation)
            result=subprocess.run([str(CONTROL/'readonly-check.exe'),'-six','-baseline',str(PUBLIC/g['group']/'expectation'),'-fresh',str(target/'files'),'-group',g['group'],
                                   '-expectation',str(target/'expectation.json'),'-inventory',str(folder/'present-inventory.json'),'-mapping',str(CONTROL/'six-map.json')],capture_output=True,timeout=45)
            write_raw(target/'checker-stdout.txt',result.stdout);write_raw(target/'checker-stderr.txt',result.stderr)
            need(result.returncode==0,'six complete native file-only aggregation failed')
            out=json.loads(result.stdout);need(out['status']=='PASS_COMPLETE_SIX_NATIVE_AGGREGATION','six native aggregation did not pass')
            out.update(technical_requests=p.prior.old_verifier.journals(target/'files'),amount='NOT_ASSESSED',role=role,native_delete=0)
            save(target/'result.json',out)
        else:
            starts=[load(x) for x in (target/'files').glob('prepare-start-*.json')]
            observed=[load(x) for x in (target/'files').glob('prepare-observation-*.json')]
            need(starts and {x['id'] for x in starts}=={x['id'] for x in observed} and len(starts)==len(observed)
                 and all(not x['transport_error'] and 100<=x['status']<=599 for x in observed),'preparation technical journals incomplete')
            save(target/'result.json',{'technical_requests':len(starts),'actual_methods':{method:sum(x['method']==method for x in starts) for method in {x['method'] for x in starts}},'amount':'NOT_ASSESSED','native_delete':0})
        return load(target/'result.json')
    finally:
        # Revoke the private projection even if wait, evidence copy or checker
        # failed; original raw bytes are bound by the immutable profiles index.
        replace(path,original)


def baseline_core(group):
    import retained_v28
    path,digest=retained_v28.CORE_BASELINES[group]
    baseline=ROOT/'docs/acceptance/evidence/issue307-d1'/path
    need(sha(baseline)==digest,'current post-closure Core checkpoint bytes drift')
    return load(baseline)


def start_pg(cid):
    docker('start',cid)
    # pg_isready alone can observe the temporary init server. Wait for the final
    # image initialization marker before executing the one-shot restore.
    for _ in range(55):
        logs=subprocess.run(['docker','logs',cid],capture_output=True)
        if b'PostgreSQL init process complete; ready for start up.' in logs.stdout+logs.stderr:break
        time.sleep(1)
    else:raise ValueError('new PostgreSQL init marker absent')
    p.native.wait_pg(cid,'checkpoint_admin')


def immutable_rows(core):
    # Persist a digest multiset, without credentials or raw business values, for
    # every old append-only fact touched by this bounded disposal admission.
    tables=('lifecycle_fixture_copy','lifecycle_copy_policy','lifecycle_provider_attempt','lifecycle_provider_observation','lifecycle_provider_send_claim',
            'lifecycle_reader_pin','lifecycle_reader_termination','lifecycle_fixture_reference_release','lifecycle_disposition_approval','lifecycle_approval_revocation',
            'lifecycle_disposition_dispatch','lifecycle_dispatch_release','lifecycle_copy_absence_receipt','dataset_version','cost_event','cost_allocation','audit_event','evidence','evidence_relation','outbox_event')
    result={}
    for table in tables:
        # table names are fixed source constants, never interpolated user data.
        raw=p.native.sql(core,'SELECT coalesce(jsonb_agg(to_jsonb(t)),\'[]\')::text FROM public."'+table+'" t;')
        rows={}
        for value in json.loads(raw):
            digest=hashlib.sha256(json.dumps(value,sort_keys=True,separators=(',',':')).encode()).hexdigest()
            rows[digest]=rows.get(digest,0)+1
        result[table]=rows
    return result


def trial(g):
    folder=Path(g['private']);group=g['group'];target=PUBLIC/group/'rehearsal'
    target.mkdir()
    before=immutable_rows(g['rehearsal_core']);save(target/'immutable-before.json',before)
    # On the restored native only, enable the separately generated sole-policy
    # inspector. There is no DELETE credential in any rehearsal worker mount.
    enabled=False;disable_failure=None;original_gateway=(folder/'restore-gateway.Caddyfile').read_bytes()
    try:
        maintenance(g,'restore-inspect-enable');enabled=True
        access=p.identity(g['scope'],'inspect')
        gateway=("{\n admin off\n auto_https off\n}\n:9000 {\n route {\n @metadata {\n method GET HEAD\n expression {http.request.header.Authorization}.contains('Credential="+access+"/')\n }\n reverse_proxy @metadata minio:9000\n respond 403\n }\n}\n").encode()
        (folder/'restore-gateway.Caddyfile').write_bytes(gateway)
        docker('start',g['restore_gateway'])
        for stage,cid in g['rehearsals'].items():
            path=folder/('rehearsal-'+stage)/'remaining.json';original=load(path)
            need(original['Base']['Enabled'] is False and original['Rehearsal'] is True,'trial profile drift')
            active=copy.deepcopy(original);start=now().replace(microsecond=0)
            active['Base'].update(Enabled=True,StartedAt=start.isoformat().replace('+00:00','Z'),ExpiresAt=(start+dt.timedelta(minutes=45)).isoformat().replace('+00:00','Z'))
            active['CandidateUseUntil']=active['Base']['ExpiresAt']
            replace(path,active)
            try:run_owner(cid,target/stage/'operation')
            finally:replace(path,original)
            shutil.copytree(folder/'outputs'/('rehearsal-'+stage),target/stage/'files')
    finally:
        # Close all potential callers before a stable disabled readback; a
        # partial enable failure still attempts this fixed restore-only disable.
        for cid in g['rehearsals'].values():p.p1.close_known(cid)
        p.p1.close_known(g['restore_gateway'])
        (folder/'restore-gateway.Caddyfile').write_bytes(original_gateway)
        try:maintenance(g,'restore-inspect-disable')
        except Exception as error:disable_failure=type(error).__name__+': '+str(error)
        save(target/'inspector-closure.json',{'enable_returned':enabled,'disabled_readback':'UNCONFIRMED' if disable_failure else 'FRESH_CONFIRMED_DISABLED','failure':disable_failure,'source_identity_enabled':False,'native_delete':0})
    need(disable_failure is None,'restore inspector containment unconfirmed')
    after=immutable_rows(g['rehearsal_core']);save(target/'immutable-after.json',after)
    need(all(all(after[table].get(digest,0)>=count for digest,count in rows.items()) for table,rows in before.items()),'old restored facts overwritten or lost')
    facts=json.loads(p.native.sql(g['rehearsal_core'],"SELECT json_build_object('send_claims',(SELECT count(*) FROM lifecycle_provider_send_claim),'copy_count',(SELECT count(*) FROM lifecycle_fixture_copy),'dataset_versions',(SELECT count(*) FROM dataset_version),'policies',(SELECT count(*) FROM lifecycle_copy_policy));"))
    save(target/'facts.json',facts)
    save(target/'result.json',{'status':'PASS_ISOLATED_NONDELETE_COMMANDS','old_fact_multisets_preserved':True,'native_delete':0,'native_object_put':0,'source_business_mutations':0,'full_mixed_outcomes':'NOT_RUN'})


def run_non_delete():
    a=state();budget_completion(a);gate=preparation_gate(a)
    need(not (PUBLIC/'non-delete-start.json').exists(),'one-shot native preparation already started; retain failure, do not replay')
    save(PUBLIC/'non-delete-start.json',{'at':now().isoformat(),'execution':p.EXECUTION,'gate':gate,'authorization':'NONE_NONDELETE_PREPARATION_ONLY'})
    failure=None;results=[];known=[*a['owners'],*[g['source'][role] for g in a['groups'] for role in ('Core','Native')]]
    try:
        for g in a['groups']:
            group=g['group'];folder=Path(g['private']);target=PUBLIC/group
            docker('start',g['source']['Core']);docker('start',g['source']['Native'])
            p.native.wait_pg(g['source']['Core'],'fixture');time.sleep(3)
            before=p.native.fingerprints(g['source']['Core']);save(target/'source-Core-before.json',before)
            need(before==baseline_core(group),'source Core drifted from independently accepted checkpoint')
            # The policy checkpoint is the immutable v22 head/expiry fact.
            # Later trusted NOT_SENT commands append Core audit facts without
            # changing those heads. Their current full Core baseline is above.
            maintenance(g,'PROVISION')
            source_result=maintenance(g,'SOURCE')
            backup=folder/'checkpoint'
            docker('exec',g['source']['Core'],'pg_dump','-U','fixture','-d','lifecycle307','--format=custom','--create',output=backup/'core.dump')
            docker('exec',g['source']['Core'],'pg_dumpall','-U','fixture','--globals-only',output=backup/'globals.sql')
            after=p.native.fingerprints(g['source']['Core']);save(target/'source-Core-after.json',after)
            need(before==after,'source Core changed during preparation')
            p.p1.close_known(g['source']['Native']);need(closed(inspect(g['source']['Native'])),'source native not stopped for backup')
            run_owner(g['utilities']['archive'],target/'archive')
            for cid in (g['restore_core'],g['rehearsal_core']):
                start_pg(cid)
                docker('exec',cid,'psql','-X','-U','checkpoint_admin','-d','postgres','-v','ON_ERROR_STOP=1','-f','/run/backup/globals.sql')
                docker('exec',cid,'pg_restore','--exit-on-error','--create','--role=fixture','-U','checkpoint_admin','-d','postgres','/run/backup/core.dump')
                restored=p.native.fingerprints(cid)
                need(restored==before,'full restored Core relations/roles/ACL differ')
                save(target/('clean-restored-Core.json' if cid==g['restore_core'] else 'trial-restored-Core.json'),restored)
            run_owner(g['utilities']['unpack'],target/'unpack')
            run_owner(g['utilities']['compare'],target/'native-file-compare')
            need((target/'native-file-compare/stdout.txt').stat().st_size>0,'native archive full-file manifest empty')
            p.p1.close_known(g['source']['Core'])
            docker('start',g['restore_native']);time.sleep(3)
            trial(g)
            restore_result=maintenance(g,'RESTORE')
            clean=p.native.fingerprints(g['restore_core']);need(clean==before,'clean checkpoint changed during separate business trial')
            save(target/'clean-Core-after-trial.json',clean)
            checkpoint={'source_core':g['source']['Core'],'source_native':g['source']['Native'],'restored_core':g['restore_core'],'restored_native':g['restore_native'],'rehearsal_core':g['rehearsal_core'],
                        'backups':{name:sha(backup/name) for name in ('core.dump','globals.sql','native.tar')},'core_full_restore':'PASS','native_file_restore':'PASS','native_exact_versions_restore':'PASS','source':source_result,'restore':restore_result,'native_delete':0,'cleanup':0}
            save(target/'checkpoint.json',checkpoint);results.append(checkpoint)
            for cid in [g['restore_native'],g['restore_core'],g['rehearsal_core']]:need(p.p1.close_known(cid)['closed'],'restore closure unconfirmed')
    except BaseException as error:
        failure={'type':type(error).__name__,'message':str(error)}
        raise
    finally:
        # No sending identity was ever enabled. Every known owner still gets a
        # physical close attempt; no failed inspection may skip the other groups.
        revocations=[]
        for g in a['groups']:
            folder=Path(g['private'])
            paths=[(folder/('maintenance-'+role.lower()) if role in ('SOURCE','RESTORE','PROVISION') else folder/role)/('six-provision.json' if role=='PROVISION' else ('restore-inspector.json' if role.startswith('restore-inspect-') else 'six-readonly.json')) for role in g['maintenance']]
            paths.extend(folder/('rehearsal-'+stage)/'remaining.json' for stage in g['rehearsals'])
            for path in paths:
                try:
                    value=load(path)
                    if 'Base' in value:value['Base']['Enabled']=False
                    else:value['Enabled']=False
                    replace(path,value);revocations.append({'file':str(path.relative_to(CONTROL)),'disabled':True})
                except Exception:revocations.append({'file':str(path.relative_to(CONTROL)),'disabled':False})
        closure=[p.p1.close_known(cid) for cid in reversed(list(dict.fromkeys(known)))]
        terminal={'status':'PASS_NONDELETE_SOURCE_RESTORE_PREPARATION' if failure is None and len(results)==3 and all(x['closed'] for x in closure) and all(x['disabled'] for x in revocations) else 'FAILED_OR_UNCONFIRMED_RETAINED',
                  'execution':p.EXECUTION,'groups_completed':len(results),'failure':failure,'closure':closure,'native_delete':0,'old_authorization_reused':False,'all_resources_retained':True,'amount':'NOT_ASSESSED'}
        terminal['profile_revocations']=revocations
        index={path.relative_to(PUBLIC).as_posix():{'sha256':sha(path),'bytes':path.stat().st_size} for path in sorted(PUBLIC.rglob('*')) if path.is_file() and path.name not in ('non-delete-terminal.json','non-delete-evidence-index.json')}
        save(PUBLIC/'non-delete-evidence-index.json',{'status':'SIX_NONDELETE_PREPARATION_EVIDENCE_INDEX','count':len(index),'files':index,'native_delete':0,'scope':'original public preparation raw channels and observations; not a signature'})
        terminal['evidence_index_sha256']=sha(PUBLIC/'non-delete-evidence-index.json')
        save(PUBLIC/'non-delete-terminal.json',terminal);save(CONTROL/'non-delete-terminal.json',terminal)
    need(terminal['status']=='PASS_NONDELETE_SOURCE_RESTORE_PREPARATION','non-delete preparation unconfirmed')


def main():
    import argparse
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=('materialize','initialize-budgets','check','run-non-delete'));args=parser.parse_args()
    if args.action=='materialize':materialize()
    elif args.action=='initialize-budgets':initialize_budgets()
    elif args.action=='run-non-delete':run_non_delete()
    else:print(json.dumps(preparation_gate(state())))


if __name__=='__main__':main()
