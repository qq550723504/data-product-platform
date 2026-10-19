"""One finite fresh P1 read-only reconciliation; no DELETE/resume/window shifting.

prepare allocates stopped owners only. run requires an independent report bound
to the actual source and preparation manifest; all close operations run finally.
"""
import argparse
import datetime as dt
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import tarfile
import time
import uuid
from collections import Counter
from urllib.parse import urlsplit

ROOT=Path(__file__).resolve().parents[2]
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private')
CONTROL=PRIVATE/'post-v22-p1-readonly01'
PUBLIC=ROOT/'docs/acceptance/evidence/issue307-d1/post-v22-p1-readonly01'
PACKET=ROOT/'docs/acceptance/evidence/issue307-d1/remaining-candidate12'
OLD=PRIVATE/'remaining-candidate12'
FAILED=ROOT/'docs/acceptance/evidence/issue307-d1/delete-confirmed-v22-execution01/window01'
PG='sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea'
MI='sha256:72476ee7f52a538e561627f7d8746e20c15d50c13e8bcdc58b22fa65cff94df6'
REDIS='sha256:858f009f9709ce576febc734aa78b8f6d624b82571f9ddb6bda4377c833b3499'
P1='5a4c9789-3e67-4e54-ab58-128d111cbd65'
ATTEMPT='009f4317-8855-4f99-80ec-cafc225f654e'

def need(ok,message):
    if not ok:raise ValueError(message)
def now():return dt.datetime.now(dt.timezone.utc)
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def load(path):return json.loads(path.read_bytes())
def write(path,value):
    path.parent.mkdir(parents=True,exist_ok=True)
    with path.open('x',encoding='utf-8',newline='\n') as f:json.dump(value,f,indent=2);f.write('\n')
def call(args,data=None):
    p=subprocess.run(list(map(str,args)),input=data,capture_output=True,cwd=ROOT)
    if p.returncode:
        if CONTROL.exists():
            with (CONTROL/('failure-'+str(time.time_ns())+'.private')).open('xb') as f:f.write(p.stdout+b'\n'+p.stderr)
        raise RuntimeError('P1 read-only command failed; original private diagnostics retained')
    need(isinstance(p.stdout,bytes),'raw command output must be bytes')
    return p.stdout
def docker(*args):return call(['docker',*args])
def text(*args):return docker(*args).decode('utf-8',errors='strict').strip()
def inspect(cid):return json.loads(docker('inspect',cid))[0]
def closed(x):
    h=x['HostConfig']
    return not x['State']['Running'] and x['State']['Pid']==0 and not h.get('PortBindings') and not any(x['NetworkSettings']['Ports'].values()) and h['RestartPolicy']['Name']=='no'
def hostpath(value):
    prefix='/run/desktop/mnt/host/'
    if value.startswith(prefix):value=value[len(prefix):];value=value[0].upper()+':/'+value[2:]
    return Path(value).resolve()

def core_connection_binding(base,group):
    u=urlsplit(base['PostgresDSN']);source=inspect(group['Core'])
    need(u.scheme=='postgres' and u.hostname==source['Name'].lstrip('/')=='dpp-lifecycle307-808a6960-postgres'
         and u.port==5432 and u.path=='/lifecycle307' and u.username=='fixture' and u.password and u.query=='sslmode=disable' and not u.fragment,
         'P1 DSN is not the fixed Core host/database/user')
    need(base['WorkspaceID']==group['Copies'][0]['WorkspaceID'] and base['ProviderInstanceID']==group['Copies'][0]['ProviderInstanceID']
         and base['IncarnationID']==group['Copies'][0]['IncarnationID'] and base['ActorID']==group['Actor'] and base['Endpoint']=='127.0.0.1:9000'
         and base['InspectAccess']=='inspect808a69606c36','P1 source profile identity differs')
    net=source['NetworkSettings']['Networks'];need(len(net)==1,'Core network expanded')
    endpoint=next(iter(net.values()))
    need(u.hostname in set((endpoint.get('Aliases') or [])+(endpoint.get('DNSNames') or [])),'Core DNS identity absent')
    return {'Core':group['Core'],'host':u.hostname,'port':u.port,'database':u.path[1:],'user':u.username,'transport':'sslmode=disable; isolated internal network','credential':'PRIVATE_NOT_EMITTED'}
def native_module():
    spec=importlib.util.spec_from_file_location('p1_native_tools',ROOT/'examples/lifecycle-d1/prepare-remaining-native.py')
    mod=importlib.util.module_from_spec(spec);spec.loader.exec_module(mod);return mod

def old_closed():
    previous=load(FAILED/'post-failure-readonly-audit.json')
    ids=[v['id'] for v in previous['physical_owners']]
    need(len(ids)==61 and len(set(ids))==61,'old closed membership differs')
    for cid in ids:need(closed(inspect(cid)),'old retained owner not closed')
    files={}
    permissions=list(OLD.rglob('permission.json'))
    cases=list(OLD.glob('*/case-*/remaining.json'))
    recoveries=list(OLD.glob('*/recover-*/recovery.json'))
    need((len(permissions),len(cases),len(recoveries))==(32,7,7),'old control membership differs')
    for path in permissions+cases+recoveries:
        c=load(path);enabled=c['Base']['Enabled'] if path in cases else c['Enabled']
        need(enabled is False,'old permission/case/recovery re-enabled')
        files[path.relative_to(OLD).as_posix()]=sha(path)
    for g in load(PACKET/'candidate.json')['Groups']:
        x=inspect(g['Gateway']);m=next(v for v in x['Mounts'] if v['Destination']=='/etc/caddy/Caddyfile')
        need(sha(hostpath(m['Source']))==g['SealedGatewaySHA256'],'old gateway not sealed')
    return {'owners':ids,'old_control_sha256':files}

def backups():
    facts={}
    for g in load(PACKET/'candidate.json')['Groups']:
        p=PRIVATE/'remaining-executable03'/g['Group']/'checkpoint'
        for name,key in [('core.dump','CoreDumpSHA256'),('globals.sql','GlobalsSHA256'),('native.tar','NativeArchiveSHA256')]:
            path=p/name;need(sha(path)==g[key],'retained backup drift');facts[g['Group']+'/'+name]=sha(path)
    need(len(facts)==9,'backup membership differs');return facts

def budgets():
    facts={}
    for g in load(PACKET/'candidate.json')['Groups']:
        cid=g['Helpers']['wire']
        raw=docker('cp',cid+':/run/budget/.','-')
        entries={}
        with tarfile.open(fileobj=io.BytesIO(raw)) as archive:
            for m in archive.getmembers():
                need(m.isdir() or m.isfile(),'unexpected budget entry type')
                if m.isfile():entries[m.name]={'bytes':m.size,'sha256':hashlib.sha256(archive.extractfile(m).read()).hexdigest()}
        facts[g['Group']]=entries
    return facts

def owner_contract(x,kind,source,private,output,binary):
    h=x['HostConfig'];mounts={m['Destination']:m for m in x['Mounts']}
    need(closed(x) and x['Image']==PG and h['NetworkMode']=='container:'+source and h['ReadonlyRootfs'] and not h['Privileged']
         and x['Config']['User']=='10001:10001' and h['CapDrop']==['ALL'] and 'no-new-privileges' in h['SecurityOpt'], 'new owner isolation differs')
    need(set(mounts)=={'/run/private','/run/executor','/run/output'},'new owner mount expansion')
    for target,path,writable in [('/run/private',private,False),('/run/executor',binary,False),('/run/output',output,True)]:
        m=mounts[target];need(m['Type']=='bind' and hostpath(m['Source'])==path.resolve() and m['RW'] is writable,'new owner mount identity differs')
    need(x['Config']['Entrypoint']==['/run/executor'],'new owner command differs')
    need(x['Config']['Cmd']==(['-action',kind] if kind!='worker' else ['p1-post-v22-readonly']),'new owner arguments differ')
    if kind=='worker':
        need(set(p.name for p in private.iterdir())=={'audit.json','recovery.json','bindings.json'},'worker private whitelist differs')
        for p in private.iterdir():
            c=load(p) if p.suffix=='.json' else {}
            need(not any(k in c for k in ('RootAccess','RootSecret','DeleteSecret','WirePrivateKey')),'root/send credential mounted to worker')

def prepare():
    need(not call(['git','diff','--name-only']).strip() and not call(['git','diff','--cached','--name-only']).strip(),'commit reviewed P1 source before preparing binary bindings')
    tracked=set(call(['git','ls-files','apps/platform','examples/lifecycle-d1/reconcile-p1-post-v22.py']).decode().splitlines())
    need(all(n in tracked for n in SOURCE_FILES),'P1 source is untracked')
    need(all(p.relative_to(ROOT).as_posix() in tracked for p in (ROOT/'apps/platform').rglob('*.go')),'untracked Go build input')
    need(not CONTROL.exists(),'P1 one-shot control already allocated')
    CONTROL.mkdir();PUBLIC.mkdir(exist_ok=True)
    before=old_closed();backup=backups();budget=budgets()
    candidate=load(PACKET/'candidate.json');g=next(v for v in candidate['Groups'] if v['Group']=='business-races')
    need(sha(PACKET/'candidate.json')=='f624a5f50ccb4fa4aa8b465f5a60a090f6325e78f60544b4c18f9ba2604ef243'
         and sha(PACKET/'bindings.json')=='8e54b47e4a1f83c7ce66555ed6ce2f3ec66c962a394a8fa1feb84eeaccd82fc5','frozen metadata differs')
    for field,image in [('Core',PG),('Native',MI)]:
        x=inspect(g[field]);need(x['Image']==image and closed(x) and {n['NetworkID'] for n in x['NetworkSettings']['Networks'].values()}=={g['Network']},'source identity differs')
    net=json.loads(docker('network','inspect',g['Network']))[0];need(net['Internal'],'source network is not internal')
    env=os.environ.copy();env.update(GOOS='linux',GOARCH='amd64',CGO_ENABLED='0')
    for name,pkg in [('worker','lifecycle-fixture-recovery'),('bootstrap','lifecycle-fixture-remaining-iam')]:
        p=subprocess.run(['go','build','-o',str(CONTROL/name),'./cmd/'+pkg],cwd=ROOT/'apps/platform',env=env,capture_output=True)
        with (CONTROL/(name+'-build.private')).open('xb') as f:f.write(p.stdout+p.stderr)
        need(p.returncode==0,'P1 read-only binary build failed')
    audit=uuid.uuid4();start=now();end=start+dt.timedelta(minutes=40)
    worker=CONTROL/'worker-private';worker.mkdir()
    base=load(PRIVATE/'remaining-executable03/business-races/source-verifier/remaining.json')['Base']
    connection=core_connection_binding(base,g)
    recovery={k:base[k] for k in ('WorkspaceID','ProviderInstanceID','IncarnationID','ActorID','Endpoint','InspectAccess','InspectSecret','ManifestSHA256')}
    recovery.update(Enabled=False,SyntheticOnly=True,RunID=str(audit),Token=uuid.uuid4().hex,ValidUntil=end.isoformat())
    write(worker/'recovery.json',recovery)
    cfg={'AuditID':str(audit),'Purpose':'POST_V22_P1_READONLY_RECONCILIATION','PostgresDSN':base['PostgresDSN'],'RedisAddr':'dpp307-p1-readonly-'+str(audit)[:8]+'-redis:6379',
         'RecoveryConfig':'/run/private/recovery.json','BinarySHA256':sha(CONTROL/'worker'),'Core':g['Core'],'Native':g['Native'],
         'NotBefore':start.isoformat(),'ExpiresAt':end.isoformat(),'MaxProviderDeletes':0}
    write(worker/'audit.json',cfg);(worker/'bindings.json').write_bytes((PACKET/'bindings.json').read_bytes())
    root=CONTROL/'root-private';root.mkdir()
    for name in ('root.json','candidate.json','iam.json'):(root/name).write_bytes((OLD/'business-races/root-disable-all'/name).read_bytes())
    (root/'policies').mkdir()
    for p in (OLD/'business-races/root-disable-all/policies').iterdir():need(p.is_file(),'unexpected root policy member');(root/'policies'/p.name).write_bytes(p.read_bytes())
    permission={'Enabled':False,'AuditID':str(audit),'Group':'business-races','Purpose':cfg['Purpose'],'SourceCandidateSHA256':sha(PACKET/'candidate.json'),
                'BinarySHA256':sha(CONTROL/'bootstrap'),'Core':g['Core'],'Native':g['Native'],'CopyID':P1,'AttemptID':ATTEMPT,
                'NotBefore':start.isoformat(),'ExpiresAt':end.isoformat(),'MaxProviderDeletes':0}
    write(root/'p1-readonly-permission.json',permission)
    redis=cfg['RedisAddr'].split(':')[0];volume=redis+'-data'
    docker('volume','create','--label','lifecycle307.readonly='+str(audit),volume)
    redis_cid=text('create','--pull','never','--name',redis,'--label','lifecycle307.readonly='+str(audit),'--network',g['Network'],
        '--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp','--mount','type=volume,source='+volume+',target=/data',REDIS,'redis-server','--appendonly','yes')
    owners={}
    for kind in ('enable-p1-readonly','disable-all','worker'):
        private=worker if kind=='worker' else root;binary=CONTROL/('worker' if kind=='worker' else 'bootstrap')
        output=PUBLIC/('output-'+kind);output.mkdir()
        args=['create','--pull','never','--name','dpp307-p1-'+str(audit)[:8]+'-'+kind,'--label','lifecycle307.readonly='+str(audit),'--user','10001:10001','--network','container:'+g['Native'],
              '--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp',
              '--tmpfs','/var/lib/postgresql/data:size=1m',
              '--mount',f'type=bind,source={private},target=/run/private,readonly','--mount',f'type=bind,source={binary},target=/run/executor,readonly',
              '--mount',f'type=bind,source={output},target=/run/output','--entrypoint','/run/executor']
        if kind=='worker':args+=['--env','LIFECYCLE_P1_POST_V22_CONFIG=/run/private/audit.json']
        args+=[PG]
        if kind!='worker':args+=['-action',kind]
        else:args+=['p1-post-v22-readonly']
        cid=text(*args);owner_contract(inspect(cid),kind,g['Native'],private,output,binary)
        owners[kind]={'cid':cid,'private':str(private),'output':str(output),'binary':str(binary),'binary_sha256':sha(binary)}
    manifest={'status':'PREPARED_DISABLED_READONLY_ONLY','audit_id':str(audit),'NotBefore':start.isoformat(),'ExpiresAt':end.isoformat(),
              'source_head':call(['git','rev-parse','HEAD']).decode().strip(),'source_delta':call(['git','diff','--name-only']).decode().splitlines(),
              'source':g,'owners':owners,'redis':redis_cid,'redis_name':redis,'redis_volume':volume,'old_closed':before,'backup_sha256':backup,'budgets':budget,
              'core_connection_binding':connection,'source_profile_sha256':sha(PRIVATE/'remaining-executable03/business-races/source-verifier/remaining.json'),
              'inputs':{str(p.relative_to(CONTROL)):sha(p) for p in CONTROL.rglob('*') if p.is_file() and p.suffix!='.private'},
              'source_files':{n:sha(ROOT/n) for n in SOURCE_FILES},'native_delete':0,'send_authority':'NONE'}
    write(CONTROL/'manifest.json',manifest);write(PUBLIC/'preparation.json',manifest)
    print(json.dumps({'status':manifest['status'],'audit_id':str(audit),'manifest_sha256':sha(CONTROL/'manifest.json'),'source_started':False}))

SOURCE_FILES=['examples/lifecycle-d1/reconcile-p1-post-v22.py','apps/platform/cmd/lifecycle-fixture-recovery/p1_post_v22.go',
              'apps/platform/cmd/lifecycle-fixture-recovery/main.go','apps/platform/cmd/lifecycle-fixture-remaining-iam/main.go',
              'apps/platform/cmd/lifecycle-fixture-remaining-iam/p1_readonly.go','apps/platform/internal/platform/outbox/publisher.go']

def fresh(manifest,allow_enabled=False):
    need(call(['git','rev-parse','HEAD']).decode().strip()==manifest['source_head'] and not call(['git','diff','--name-only']).strip() and not call(['git','diff','--cached','--name-only']).strip(),'P1 source commit/tracked worktree drift')
    need(now()<dt.datetime.fromisoformat(manifest['ExpiresAt']) and now()>=dt.datetime.fromisoformat(manifest['NotBefore']),'P1 new read-only window unavailable')
    need(old_closed()==manifest['old_closed'] and backups()==manifest['backup_sha256'] and budgets()==manifest['budgets'],'retained facts drift')
    for name,h in manifest['source_files'].items():need(sha(ROOT/name)==h,'P1 read-only source changed')
    for name,h in manifest['inputs'].items():
        if allow_enabled and name in ('worker-private/recovery.json','root-private/p1-readonly-permission.json'):continue
        need(sha(CONTROL/name)==h,'P1 read-only input changed')
    g=manifest['source']
    for kind,x in manifest['owners'].items():
        owner_contract(inspect(x['cid']),kind,g['Native'],Path(x['private']),Path(x['output']),Path(x['binary']))
        need(sha(Path(x['binary']))==x['binary_sha256'],'P1 binary changed')
    need(closed(inspect(manifest['redis'])),'new Redis not closed')
    g=manifest['source']
    for field,image in [('Core',PG),('Native',MI)]:
        x=inspect(g[field]);need(x['Image']==image and {n['NetworkID'] for n in x['NetworkSettings']['Networks'].values()}=={g['Network']},'source image/network drift')
    network=json.loads(docker('network','inspect',g['Network']))[0]
    need(network['Internal'] and not network['Containers'],'foreign running owner on the bound network')

def active_topology(manifest,kind):
    g=manifest['source']
    need(now()<dt.datetime.fromisoformat(manifest['ExpiresAt']),'new read-only audit expired')
    for n,h in manifest['source_files'].items():need(sha(ROOT/n)==h,'P1 source changed during audit')
    for field,image in [('Core',PG),('Native',MI)]:
        x=inspect(g[field]);need(x['State']['Running'] and x['Image']==image and {n['NetworkID'] for n in x['NetworkSettings']['Networks'].values()}=={g['Network']},'active P1 source topology differs')
    for name,h in manifest['inputs'].items():
        p=CONTROL/name
        if name in ('worker-private/recovery.json','root-private/p1-readonly-permission.json'):
            original=CONTROL/(p.stem+'-disabled-original.json')
            need(sha(original)==h,'original disabled profile drift')
            want=load(original);want['Enabled']=kind!='disable-all'
            need(load(p)==want,'read-only profile fields drift')
        else:need(sha(p)==h,'P1 input drift during audit')
    for cid in manifest['old_closed']['owners']:
        if cid not in (g['Core'],g['Native']):need(closed(inspect(cid)),'old unrelated owner running')
    for name,h in manifest['old_closed']['old_control_sha256'].items():need(sha(OLD/name)==h,'old control changed during audit')
    for name,x in manifest['owners'].items():
        owner_contract(inspect(x['cid']),name,g['Native'],Path(x['private']),Path(x['output']),Path(x['binary']))
        need(sha(Path(x['binary']))==x['binary_sha256'],'P1 binary drift during audit')
    network=json.loads(docker('network','inspect',g['Network']))[0]
    allowed={g['Core'],g['Native'],manifest['redis'],*[x['cid'] for x in manifest['owners'].values()]}
    need(network['Internal'] and set(network['Containers'])<=allowed,'P1 running network expanded')
    host=manifest['core_connection_binding']['host']
    core=inspect(g['Core'])
    names=[v for n in core['NetworkSettings']['Networks'].values() for v in (n.get('Aliases') or [])+(n.get('DNSNames') or [])]
    need(core['Name'].lstrip('/')==host and host in names,'fixed Core DNS name drift')
    for cid in set(network['Containers'])- {g['Core']}:
        aliases=[v for n in inspect(cid)['NetworkSettings']['Networks'].values() for v in (n.get('Aliases') or [])+(n.get('DNSNames') or [])]
        need(host not in aliases,'another owner shadows the fixed Core DNS alias')

def owner_logs(cid,kind):
    p=subprocess.run(['docker','logs',cid],capture_output=True,cwd=ROOT)
    facts={'exit_code':p.returncode}
    for name,raw in [('stdout',p.stdout),('stderr',p.stderr)]:
        need(isinstance(raw,bytes),'raw owner log channel is not bytes')
        with (CONTROL/(kind+'-logs-'+name+'.private')).open('xb') as f:f.write(raw)
        facts[name]={'bytes':len(raw),'sha256':hashlib.sha256(raw).hexdigest()}
    return facts

def execute_owner(manifest,kind):
    # Containment may never be prevented by an expired/corrupt enable packet.
    # This still uses the fixed reviewed disable-all CID and separate root data.
    if kind!='disable-all':active_topology(manifest,kind)
    print(json.dumps({'at':now().isoformat(),'phase':kind}),flush=True)
    x=manifest['owners'][kind];cid=x['cid'];docker('start',cid)
    code=int(text('wait',cid));state=inspect(cid)
    logs=owner_logs(cid,kind)
    write(PUBLIC/(kind+'-owner.json'),{'cid':cid,'exit':code,'started_at':state['State']['StartedAt'],'finished_at':state['State']['FinishedAt'],'closed':closed(state),'logs':logs})
    need(code==0 and logs['exit_code']==0 and closed(state),'P1 read-only owner failed')

def revoke_profiles():
    results=[]
    for relative in ('worker-private/recovery.json','root-private/p1-readonly-permission.json'):
        try:
            p=CONTROL/relative;c=load(p);c['Enabled']=False;p.write_text(json.dumps(c,indent=2)+'\n',encoding='utf-8',newline='\n')
            results.append({'profile':relative,'disabled':load(p)['Enabled'] is False})
        except Exception:results.append({'profile':relative,'disabled':False})
    return results

def close_known(cid):
    # An unreadable inspect is uncertainty, never a reason to omit stopping a
    # fixed owner. Stop failure and the final physical readback are separate.
    inspect_failed=False;stop_failed=False
    try:running=inspect(cid)['State']['Running']
    except Exception:running=True;inspect_failed=True
    if running:
        try:docker('stop','--time','15',cid)
        except Exception:stop_failed=True
    try:confirmed=closed(inspect(cid))
    except Exception:confirmed=False
    return {'cid':cid,'closed':confirmed,'initial_inspect_failed':inspect_failed,'stop_failed':stop_failed}

def seal(manifest):
    profiles=revoke_profiles();checks=[]
    for kind in ('worker','enable-p1-readonly'):
        cid=manifest['owners'][kind]['cid']
        checks.append(close_known(cid))
    disabled=False
    try:native_running=inspect(manifest['source']['Native'])['State']['Running']
    except Exception:native_running=True
    if native_running:
        try:
            execute_owner(manifest,'disable-all')
            receipt=load(Path(manifest['owners']['disable-all']['output'])/'status-changes.json')
            disabled=(len(receipt['disabled_readback'])==4 and not receipt['unconfirmed'] and all(v=='disabled' for v in receipt['disabled_readback'].values()))
        except Exception:disabled=False
    for cid in [manifest['owners']['disable-all']['cid'],manifest['redis'],manifest['source']['Native'],manifest['source']['Core']]:
        checks.append(close_known(cid))
    return {'at':now().isoformat(),'closed':checks,'profiles':profiles,'all_closed':all(v['closed'] for v in checks) and all(v['disabled'] for v in profiles),'fresh_disabled_readback':disabled,'cleanup':0,'native_delete':0}

def run(review_path,review_sha):
    need(not (CONTROL/'terminal.json').exists(),'P1 audit already terminal; no repeat')
    manifest=load(CONTROL/'manifest.json')
    need(sha(review_path)==review_sha,'independent P1 review bytes differ')
    review=load(review_path)
    need(review['status']=='PASS_BOUNDED_INDEPENDENT_P1_READONLY_REVIEW' and review['remaining_blockers']==[]
         and review['manifest_sha256']==sha(CONTROL/'manifest.json'),'independent P1 review not closed/bound')
    try:fresh(manifest)
    except Exception as e:
        revoke_profiles()
        terminal={'at':now().isoformat(),'audit_id':manifest['audit_id'],'status':'BLOCKED_PRESTART_NO_SOURCE_IO','failure':type(e).__name__+': '+str(e),
                  'source_started':False,'identity_enabled':False,'native_delete':0,'old_execution_resumed':False,'full_D1':'NOT_ACCEPTED'}
        write(CONTROL/'terminal.json',terminal);write(PUBLIC/'terminal.json',terminal);print(json.dumps(terminal));return
    failure=None;result=None;before=None
    try:
        for relative in ('worker-private/recovery.json','root-private/p1-readonly-permission.json'):
            p=CONTROL/relative;raw=p.read_bytes()
            with (CONTROL/(p.stem+'-disabled-original.json')).open('xb') as f:f.write(raw)
            c=load(p);need(c['Enabled'] is False,'new profile already enabled');c['Enabled']=True;p.write_text(json.dumps(c,indent=2)+'\n',encoding='utf-8',newline='\n')
        for field in ('Core','Native'):docker('start',manifest['source'][field])
        native=native_module();native.wait_pg(manifest['source']['Core'],'fixture');time.sleep(2)
        before=native.fingerprints(manifest['source']['Core']);write(PUBLIC/'Core-before.json',before)
        anchors=append_anchors(native,manifest['source']['Core'])
        docker('start',manifest['redis'])
        execute_owner(manifest,'enable-p1-readonly')
        enabled=load(Path(manifest['owners']['enable-p1-readonly']['output'])/'result.json')
        need(enabled['changed']=={'inspect808a69606c36':'enabled'} and len(enabled['before'])==4 and all(v=='disabled' for v in enabled['before'].values()),'bootstrap enabled another identity')
        execute_owner(manifest,'worker')
        result=load(Path(manifest['owners']['worker']['output'])/'P1-reconciled.json')
        after=native.fingerprints(manifest['source']['Core']);write(PUBLIC/'Core-after.json',after)
        for key in ('roles_acl','all_roles_sha256','schema_function_default_acl_sha256'):need(before[key]==after[key],'P1 audit changed Core role/ACL')
        a={r['relation']:r for r in before['relations']};b={r['relation']:r for r in after['relations']}
        changed={n for n in a if a[n]!=b[n]}
        need(set(a)==set(b) and changed<=ALLOWED_RELATIONS,'P1 audit changed an unrelated Core relation')
        write(PUBLIC/'Core-delta.json',{'changed_relations':sorted(changed),'immutable_provider_attempt_observation_budget_and_pin_facts_unchanged':True})
        after_anchors=append_anchors(native,manifest['source']['Core'])
        need(all(not (anchors[n]-after_anchors[n]) for n in anchors),'P1 audit changed an old fact/obligation/cost row')
        write(PUBLIC/'Core-append-preservation.json',{'old_rows_preserved':True,'original_counts':{n:sum(v.values()) for n,v in anchors.items()},'after_counts':{n:sum(v.values()) for n,v in after_anchors.items()}})
    except Exception as e:
        failure=type(e).__name__+': '+str(e)
        write(PUBLIC/'failure.json',{'at':now().isoformat(),'failure':failure,'native_delete':0})
    finally:
        closure=seal(manifest);write(PUBLIC/'seal.json',closure)
        retained=False
        try:retained=old_closed()==manifest['old_closed'] and backups()==manifest['backup_sha256'] and budgets()==manifest['budgets']
        except Exception as e:
            failure=failure or 'post-close retained-fact check failed: '+type(e).__name__
        terminal={'at':now().isoformat(),'audit_id':manifest['audit_id'],'status':'PASS_P1_READONLY_RECONCILIATION_SEALED' if failure is None and closure['all_closed'] and closure['fresh_disabled_readback'] and retained else 'FAILED_OR_UNCONFIRMED_SEALED',
                  'failure':failure,'reconciliation':result,'closure':closure,'retained_old_facts_unchanged':retained,'native_delete':0,'old_execution_resumed':False,'PR323':'DRAFT','Issue307':'OPEN','full_D1':'NOT_ACCEPTED'}
        write(CONTROL/'terminal.json',terminal);write(PUBLIC/'terminal.json',terminal)
    print(json.dumps(terminal))

ALLOWED_RELATIONS={'delivery_authorization_fence','lifecycle_storage_call','lifecycle_storage_observation','cost_event','cost_allocation',
    'lifecycle_reconciliation_work','lifecycle_reconciliation_run','lifecycle_reconciliation_result','lifecycle_copy_absence_receipt','lifecycle_plan_completion',
    'command_idempotency','outbox_event','outbox_event_consumption','audit_event','evidence','evidence_relation'}

def append_anchors(native,core):
    names=sorted(ALLOWED_RELATIONS-{'delivery_authorization_fence'})
    queries=["SELECT json_build_object('relation','"+n+"','row',to_jsonb(t))::text FROM public.\""+n+'\" t' for n in names]
    raw=native.sql(core,' UNION ALL '.join(queries)+';')
    result={n:Counter() for n in names}
    for line in raw.splitlines():
        item=json.loads(line);digest=hashlib.sha256(json.dumps(item['row'],sort_keys=True,separators=(',',':')).encode()).hexdigest()
        result[item['relation']][digest]+=1
    return result

if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=['prepare','run']);parser.add_argument('--review',type=Path);parser.add_argument('--review-sha256')
    args=parser.parse_args()
    if args.action=='prepare':prepare()
    else:
        need(args.review is not None and args.review_sha256 is not None,'bound P1 independent review required');run(args.review,args.review_sha256)
