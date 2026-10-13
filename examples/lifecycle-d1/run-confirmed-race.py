"""Finite composition of existing commands/adapter/worker for one approved run.

No cleanup, target arguments, root payload mount, automatic resend or migration.
The caller must first obtain the exact-list human confirmation described in #307.
"""
import argparse, datetime as dt, hashlib, json, os, secrets, subprocess
from pathlib import Path

RUN='9812f0fc-c05a-4bcb-87ea-a4cef9089f3f'
PROJECT='dpp-lifecycle307-9812f0fc'
HASH='fa2b19e1d7fc12132b3f4e4b4acee09e4f5bc0f59eae87820b83fb2fff220c86'
PLAN='756a4139-bfae-4b5e-9368-e65d28d4c877'
VERSIONS={'dpp-lifecycle307-online/fixture/a.csv':'0f6aa755-4b86-496b-9ee1-43d7c59a8bb3','dpp-lifecycle307-quarantine/fixture/b.csv':'e1611b0a-efc0-4939-abc2-30394ec2c8d2'}
parser=argparse.ArgumentParser()
parser.add_argument('phase',choices=['prepare','provision','provision-resume','a','a-resume','b','worker-a','worker-b','seal'])
parser.add_argument('--private-directory',required=True)
parser.add_argument('--confirmed-plan',required=True)
args=parser.parse_args()
assert args.confirmed_plan==HASH,'Exact new human-confirmed plan required'
root=Path(__file__).resolve().parents[2]
private=Path(args.private_directory).resolve()
assert private.name==PROJECT and not private.is_relative_to(root)
control=private/'race-control'
out=root/'examples/lifecycle-d1/results'/RUN
r=json.loads((private/'resources.json').read_text())
c=json.loads((private/'config.json').read_text())
m=json.loads((out/'manifest.json').read_text())
assert r['project']==PROJECT and r['run_id']==RUN and c['RunID']==RUN and not c['Enabled']
assert c['WorkspaceID']==m['workspace_id'] and m['environment']['migrations_through']==66
assert c['ProviderInstanceID']==m['provider_instance_id'] and c['IncarnationID']==m['incarnation_id']
sha=lambda path:hashlib.sha256(path.read_bytes()).hexdigest()
now=lambda:dt.datetime.now(dt.timezone.utc)
def write(path,value):
    path.write_text(json.dumps(value,indent=2)+'\n' if not isinstance(value,str) else value,encoding='utf-8',newline='\n')
def call(argv,expected=(0,),env=None,cwd=None):
    proc=subprocess.run(argv,capture_output=True,env=env,cwd=cwd)
    if proc.returncode not in expected:
        folder=control if control.exists() else private
        write(folder/'last-command-error.txt',proc.stderr.decode(errors='replace'))
        raise RuntimeError('Owned command failed; private diagnostics and all resources retained')
    return proc.stdout
def docker(*argv):return call(['docker',*argv])
def sql(query):return docker('exec',r['containers']['postgres'],'psql','-X','-qAt','-v','ON_ERROR_STOP=1','-U','fixture','-d','lifecycle307','-c',query).decode().strip()
def runtime_preflight():
    for item in json.loads(docker('inspect',*r['containers'].values())):
        expected=next(v for v in m['environment']['containers'] if v['name']==item['Name'].lstrip('/'))
        assert item['Id']==expected['id'] and item['Image']==expected['image']
        assert item['Config']['Labels']['lifecycle307.run']==RUN and item['State']['Running']
        assert not any(item['NetworkSettings']['Ports'].values())
        if item['Name'].endswith('-postgres') or item['Name'].endswith('-minio'):
            role='postgres' if item['Name'].endswith('-postgres') else 'objects'
            path='/var/lib/postgresql/data' if role=='postgres' else '/data'
            mounts=[v for v in item['Mounts'] if v['Destination']==path]
            assert len(mounts)==1 and mounts[0]['Type']=='volume' and mounts[0]['Name']==PROJECT+'-'+role and mounts[0]['RW']
            assert not item['HostConfig'].get('Tmpfs')
    net=json.loads(docker('network','inspect',r['network']))[0]
    assert net['Id']==m['environment']['network']['id'] and net['Internal'] and len(net['Containers'])==4
    assert {v['Name'] for v in net['Containers'].values()}==set(r['containers'].values())|{PROJECT+'-race-redis'}
    assert docker('exec',r['containers']['minio'],'sha256sum','/usr/local/bin/minio').decode().split()[0]==m['environment']['minio_binary_sha256']
    assert sql('SELECT max(version) FROM schema_migration')=='66'
def gateway(text):
    write(private/'Caddyfile',text)
    docker('exec',r['containers']['gateway'],'caddy','validate','--config','/etc/caddy/Caddyfile','--adapter','caddyfile')
    docker('restart',r['containers']['gateway'])
    docker('exec',r['containers']['gateway'],'wget','-T','10','-q','-O','-','http://127.0.0.1:9000/minio/health/ready')
def helper(folder,binary,name,env,expected=0):
    private_mount='type=bind,source='+str(folder)+',target=/run/private'+('' if folder.name=='admin' else ',readonly')
    docker('create','--pull','never','--name',name,'--label','lifecycle307.run='+RUN,'--network','container:'+r['containers']['gateway'],'--read-only','--tmpfs','/tmp','--mount',private_mount,'--mount','type=bind,source='+str(root/'examples/lifecycle-d1/results')+',target=/run/output',*[v for key,value in env.items() for v in ['--env',key+'='+value]],'--entrypoint','/run/private/'+binary,m['environment']['containers'][0]['image'])
    docker('start',name)
    code=int(docker('wait',name).strip())
    log=subprocess.run(['docker','logs',name],capture_output=True,check=True)
    write(control/(name+'.log'),(log.stdout+log.stderr).decode(errors='replace'))
    write(out/(name+'-exit.json'),{'run_id':RUN,'helper':name,'id':json.loads(docker('inspect',name))[0]['Id'],'exit':code,'expected_exit':expected,'binary_sha256':sha(folder/binary),'driver_sha256':sha(Path(__file__)),'root_mounted':folder.name=='admin','cleanup':0})
    assert code==expected,'Helper failed; no automatic resend; private log retained'

def prepare():
    assert not control.exists(),'Already prepared; never overwrite authority'
    backup=json.loads((private/'backup-after-confirmation/result.json').read_text())
    assert backup['status']=='PASS' and backup['objectVersions']==4 and backup['fullFactsEqualIndependentRestore']
    assert sql('SELECT count(*) FROM lifecycle_provider_attempt')=='0'
    topology=[]
    for item in json.loads(docker('inspect',*r['containers'].values())):
        assert item['Config']['Labels']['lifecycle307.run']==RUN and item['State']['Running']
        assert not any(item['NetworkSettings']['Ports'].values())
        expected=next(v for v in m['environment']['containers'] if v['name']==item['Name'].lstrip('/'))
        assert item['Id']==expected['id'] and item['Image']==expected['image']
        topology.append({'name':item['Name'],'id':item['Id'],'image':item['Image'],'mounts':[{key:v.get(key) for key in ['Type','Name','Destination','RW']} for v in item['Mounts']]})
    net=json.loads(docker('network','inspect',r['network']))[0]
    assert net['Id']==m['environment']['network']['id'] and net['Internal'] and len(net['Containers'])==3
    control.mkdir();(control/'admin').mkdir();(control/'executor').mkdir();(control/'worker').mkdir()
    write(out/'confirmed-race-backup.json',backup)
    write(out/'confirmed-race-topology.json',{'run_id':RUN,'network_id':net['Id'],'internal':True,'host_ports':[],'containers':topology,'cleanup':0})
    env=dict(os.environ,GOOS='linux',GOARCH='amd64',CGO_ENABLED='0')
    for binary,command in [('delete','lifecycle-fixture-delete'),('recovery','lifecycle-fixture-recovery')]:
        target=control/('admin' if binary=='delete' else 'worker')/binary
        call(['go','build','-o',str(target),'./cmd/'+command],env=env,cwd=root/'apps/platform')
    (control/'executor/delete').write_bytes((control/'admin/delete').read_bytes())
    start=now();end=start+dt.timedelta(minutes=40)
    permission={'Enabled':True,'RunID':RUN,'WorkspaceID':c['WorkspaceID'],'PlanHash':HASH,'DeleteAccess':'delete9812f0fc','DeleteSecret':secrets.token_hex(32),'BasisRef':'human-confirmed:call_0275d4f95cd44342bf0c36afa69053e3:exact-two-source-versions','TopologySHA256':sha(out/'confirmed-race-topology.json'),'ConfirmedAt':start.isoformat(),'ExpiresAt':end.isoformat()}
    reader_access='reader9812f0fc';reader_secret=secrets.token_hex(32)
    minimal={key:c[key] for key in ['RunID','WorkspaceID','ProviderInstanceID','IncarnationID','ActorID','Endpoint','PostgresDSN','InspectAccess','InspectSecret','Token','OutputDirectory']}
    minimal.update(ReaderAccess=reader_access,ReaderSecret=reader_secret)
    admin=dict(minimal,RootAccess=c['RootAccess'],RootSecret=c['RootSecret'])
    for folder,cfg in [('admin',admin),('executor',minimal)]:
        write(control/folder/'config.json',cfg);write(control/folder/'delete-permission.json',permission)
    write(out/'confirmed-race-human-approval.json',{'run_id':RUN,'project':PROJECT,'plan_id':PLAN,'scope_hash':HASH,'source':'Explicit user reply to call_0275d4f95cd44342bf0c36afa69053e3 in current Codex chat','targets':VERSIONS,'confirmed_at':start.isoformat(),'window_expires_at':end.isoformat(),'cleanup':0,'bypass':0,'hold_release':0,'backups_excluded':True})
    template=(root/'examples/lifecycle-d1/reader.Caddyfile').read_text()
    template=template.replace('  @delete method DELETE\n  respond @delete 403\n','')
    template=template.replace('__ROOT_ACCESS__',c['RootAccess']).replace('__READER_ACCESS__',reader_access).replace('__INSPECT_ACCESS__',c['InspectAccess']).replace('__A_VERSION__',VERSIONS['dpp-lifecycle307-online/fixture/a.csv'])
    beta='  @beta {\n   method GET\n   path /dpp-lifecycle307-quarantine/fixture/b.csv\n   expression {http.request.header.Authorization}.contains(\'Credential='+reader_access+'/\') && {http.request.uri} == \'/dpp-lifecycle307-quarantine/fixture/b.csv?versionId='+VERSIONS['dpp-lifecycle307-quarantine/fixture/b.csv']+'\'\n  }\n  reverse_proxy @beta minio:9000\n'
    template=template.replace('  @inspection {',beta+'  @inspection {')
    template=template.replace('  @copy header','  @bypass header X-Amz-Bypass-Governance-Retention *\n  respond @bypass 403\n  @copy header')
    match=' || '.join("{http.request.uri} == '/"+key+'?versionId='+version+"'" for key,version in VERSIONS.items())
    grant="  @approvedDelete {\n   method DELETE\n   expression {http.request.header.Authorization}.contains('Credential="+permission['DeleteAccess']+"/') && ("+match+")\n  }\n"
    fault=grant+'  reverse_proxy @approvedDelete minio:9000 {\n   @accepted status 204\n   handle_response @accepted {\n    abort\n   }\n  }\n'
    sealed=template.replace('  @write method','  @delete method DELETE\n  respond @delete 403\n  @write method')
    active=template.replace('  @write method',fault+'  @write method')
    write(control/'gateway-sealed',sealed);write(control/'gateway-fault-active',active)
    redis_image=json.loads(docker('image','inspect','redis:7-alpine'))[0]['Id']
    redis=PROJECT+'-race-redis';volume=redis+'-data'
    docker('volume','create','--label','lifecycle307.run='+RUN,volume)
    docker('create','--pull','never','--name',redis,'--label','lifecycle307.run='+RUN,'--network',r['network'],'--mount','type=volume,source='+volume+',target=/data',redis_image,'redis-server','--appendonly','yes')
    docker('start',redis);assert docker('exec',redis,'redis-cli','PING').strip()==b'PONG'
    scope={'Enabled':True,'SyntheticOnly':True,'RunID':RUN,'WorkspaceID':c['WorkspaceID'],'ProviderInstanceID':c['ProviderInstanceID'],'IncarnationID':c['IncarnationID'],'ActorID':c['ActorID'],'Endpoint':c['Endpoint'],'InspectAccess':c['InspectAccess'],'InspectSecret':c['InspectSecret'],'Token':c['Token'],'ManifestSHA256':sha(out/'manifest.json'),'ValidUntil':end.isoformat()}
    write(control/'worker/recovery.json',scope)
    write(control/'worker/base.json',{'Mode':'','Scenario':'','PostgresDSN':c['PostgresDSN'],'RedisAddr':redis+':6379','RecoveryConfig':'/run/private/recovery.json','Manifest':'/run/output/'+RUN+'/manifest.json','OutputDirectory':'/run/output/'+RUN,'WorkspaceID':c['WorkspaceID'],'ActorID':c['ActorID'],'ExpiresAt':end.isoformat()})
    code_hashes={str(path.relative_to(root)).replace('\\','/'):sha(path) for folder in ['apps/platform/cmd/lifecycle-fixture-delete','apps/platform/cmd/lifecycle-fixture-recovery','apps/platform/internal/lifecycle','apps/platform/internal/platform/storage'] for path in (root/folder).rglob('*.go')}
    write(out/'confirmed-race-sources.json',{'source_head':call(['git','rev-parse','HEAD'],cwd=root).decode().strip(),'source_hashes':code_hashes,'delete_binary_sha256':sha(control/'executor/delete'),'recovery_binary_sha256':sha(control/'worker/recovery'),'redis_image':redis_image,'redis_volume':volume,'gateway_fault_sha256':sha(control/'gateway-fault-active'),'response_loss':'Caddy intercepts successful upstream204 and aborts downstream connection; no proxy/SDK retry','cleanup':0})
    gateway(sealed)
    print('Prepared exact confirmed scope; native DELETE=0; no cleanup.')

def worker(stage):
    sent=json.loads((out/('race-delete-send-'+stage+'.json')).read_text())
    base=json.loads((control/'worker/base.json').read_text());base.update(WorkID=sent['work_id'],AttemptID=sent['attempt']['ID'],CopyID=sent['copy']['ID'])
    for mode,scenario in [('outbox','race-'+stage+'-outbox'),('worker','race-'+stage+'-worker'),('once','race-'+stage+'-terminal-replay')]:
        value=dict(base,Mode=mode,Scenario=scenario)
        write(control/'worker/step.json',value)
        helper(control/'worker','recovery',PROJECT+'-'+scenario,{'LIFECYCLE_RECOVERY_ACCEPTANCE_CONFIG':'/run/private/step.json'})
    print('Read-only worker terminal recovery/replay completed; no deletion resend.')

try:
    if args.phase=='prepare':prepare()
    elif args.phase in ['provision','provision-resume']:
        runtime_preflight()
        gateway((control/'gateway-sealed').read_text())
        if args.phase=='provision-resume':
            # The previous failure was before opening the required admin START
            # journal on a read-only mount, hence provably zero native IAM calls.
            assert not (control/'admin/executor-admin-receipts.jsonl').exists()
        helper(control/'admin','delete',PROJECT+'-race-'+args.phase,{'LIFECYCLE_DELETE_PROFILE':'race-delete-9812f0fc','LIFECYCLE_DELETE_MODE':'provision'})
    elif args.phase in ['a','a-resume','b']:
        runtime_preflight()
        if args.phase=='a-resume':
            assert sql('SELECT count(*) FROM lifecycle_provider_attempt')=='0'
            assert sql('SELECT count(*) FROM lifecycle_reader_pin')=='0'
        gateway((control/'gateway-fault-active').read_text())
        helper(control/'executor','delete',PROJECT+'-race-delete-'+args.phase,{'LIFECYCLE_DELETE_PROFILE':'race-delete-9812f0fc','LIFECYCLE_DELETE_MODE':'execute','LIFECYCLE_RACE_STAGE':args.phase[0]},93)
    elif args.phase.startswith('worker-'):worker(args.phase[-1])
    elif args.phase=='seal':
        gateway((control/'gateway-sealed').read_text())
        helper(control/'admin','delete',PROJECT+'-race-disable',{'LIFECYCLE_DELETE_PROFILE':'race-delete-9812f0fc','LIFECYCLE_DELETE_MODE':'disable'})
        # Shut the admin/payload paths after independently disabling both native users.
        gateway('{\n admin off\n auto_https off\n}\n:9000 {\n @health path /minio/health/ready\n reverse_proxy @health minio:9000\n respond 403\n}\n')
        for folder in ['admin','executor']:
            path=control/folder/'delete-permission.json';p=json.loads(path.read_text());p['Enabled']=False;write(path,p)
        path=control/'worker/recovery.json';p=json.loads(path.read_text());p['Enabled']=False;write(path,p)
        write(out/'confirmed-race-sealed.json',{'run_id':RUN,'sealed_at':now().isoformat(),'gateway':'DENY_ALL_OBJECT_ADMIN_IO','native_delete_reader_users':'DISABLED','recovery_authority':'DISABLED','cleanup':0,'remaining_native_versions':2,'backup_payloads':'RETAINED / excluded from source receipt'})
except Exception:
    if (control/'gateway-sealed').exists():gateway((control/'gateway-sealed').read_text())
    raise
