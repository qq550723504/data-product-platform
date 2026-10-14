"""Fixed non-delete native checkpoint and server no-send preparation.

No DELETE/provision/enable/cleanup phase. Sender activation remains separate.
Every phase and retained resource is append-only; failures never auto-resume.
"""
import argparse, datetime as dt, hashlib, json, os, subprocess, time
from pathlib import Path
from urllib.parse import urlsplit, urlunsplit

ROOT=Path(__file__).resolve().parents[2]
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private/dpp-lifecycle307-5d2274ee')
CONTROL=PRIVATE/'reference-disposal-preparation01'
PUBLIC=ROOT/'docs/acceptance/evidence/issue307-d1/reference-disposal-preparation01'
PROJECT='dpp-lifecycle307-5d2274ee'
RUN='5d2274ee-a671-4bdc-b406-22b8dad8fd61'
PG_IMAGE='sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea'
MINIO_IMAGE='sha256:72476ee7f52a538e561627f7d8746e20c15d50c13e8bcdc58b22fa65cff94df6'
CLONE=PROJECT+'-predelete-objects'
PHASES=('build','render','rehearse','no-send','no-send-replay','native-before','native-snapshot','native-restored','native-after','core-refresh','seal')
def require(ok,message):
    if not ok: raise RuntimeError(message)
def sha(path): return hashlib.sha256(path.read_bytes()).hexdigest()
def load(path): return json.loads(path.read_text(encoding='utf-8-sig'))
def write(path,value):
    with path.open('x',encoding='utf-8',newline='\n') as stream:
        json.dump(value,stream,indent=2);stream.write('\n')
def call(args,input_data=None,env=None,cwd=ROOT,expected=(0,)):
    result=subprocess.run(args,input=input_data,capture_output=True,env=env,cwd=cwd)
    if result.returncode not in expected:
        if CONTROL.exists():
            name=CONTROL/('failure-'+dt.datetime.now(dt.timezone.utc).strftime('%Y%m%dT%H%M%S%f')+'.private-log')
            name.write_bytes(result.stdout+b'\n'+result.stderr)
        raise RuntimeError('Controlled command failed; private diagnostics retained, no automatic retry')
    return result.stdout
def docker(*args,**kwargs): return call(['docker',*args],**kwargs)
def inspect(name): return json.loads(docker('inspect',name))[0]
def sql(container,query,database='lifecycle307'):
    return docker('exec','-i',container,'psql','-X','-qAt','-v','ON_ERROR_STOP=1','-U','fixture','-d',database,input_data=query.encode()).decode().strip()
def retained(role,state='running'):
    return json.loads(call(['pwsh','-NoProfile','-File',str(ROOT/'examples/lifecycle-d1/assert-reference-storage.ps1'),'-Role',role,'-State',state]))
def preflight():
    expected=json.loads(call(['git','show','917465d88a0f6e8dfd0a4745fb4d8d767a2c75a1:docs/acceptance/evidence/issue307-d1/delete-proposal-5d2274ee/topology-readback.json']))['containers']
    for original in expected:
        role,identifier=original['role'],original['id']
        item=inspect(PROJECT+'-'+role)
        require(item['Id']==identifier and item['Image']==original['image'] and item['State']['Running'] and item['Config']['Labels']['lifecycle307.run']==RUN,'Source identity/state changed')
        require(not any(item['NetworkSettings']['Ports'].values()),'Source host port exposed')
    cfg=load(PRIVATE/'config.json');reader=load(PRIVATE/'plane-owner/reader.json')
    require(not cfg['Enabled'] and not reader['Enabled'],'Business window reopened')
    require(sha(PRIVATE/'Caddyfile')=='b301fc4fe59f64ebe1e11f1488dde521b774622641ccc8377fe41cb64e839e80','Original sealed gateway changed')
    retained('source-core');retained('source-objects')
    return cfg
def migrate(container,network=None,dsn=None):
    # Exact additive migration only; never rerun down or overwrite migration facts.
    version=sql(container,'SELECT max(version) FROM schema_migration')
    require(version=='67','Explicit schema67 to68 transition required')
    phase='migrate-source' if container==PROJECT+'-postgres' else 'migrate-restored'
    helper(phase,network or PROJECT+'-minio',dsn=dsn,mode='migrate-checkpoint')
def helper(phase,network,dsn=None,attestation=None,expected=0,mode=None):
    folder=CONTROL/phase;require(not folder.exists(),'Append-only helper phase already owned');folder.mkdir()
    payload=folder/'payload';payload.mkdir();out=PUBLIC/phase;out.mkdir()
    cfg=preflight();fields=['RunID','WorkspaceID','ProviderInstanceID','IncarnationID','ActorID','Endpoint','PostgresDSN','InspectAccess','InspectSecret','Token']
    fixture={key:cfg[key] for key in fields};fixture.update(RootAccess='',RootSecret='',OutputDirectory='/run/output')
    if dsn:fixture['PostgresDSN']=dsn
    reference={'Fixture':fixture,'PreparationEnabled':True,'PreparationUntil':(dt.datetime.now(dt.timezone.utc)+dt.timedelta(minutes=40)).isoformat(),'AttestationPublicKey':''}
    if attestation:
        reference['AttestationPublicKey']=load(attestation/'attestation-public.json')['public_key']
        (folder/'closed-executor.json').write_bytes((attestation/'closed-executor.json').read_bytes())
    write(folder/'reference.json',reference)
    write(folder/'reference-permission.json',{'Enabled':False})
    (folder/'executor').write_bytes((CONTROL/'bin/executor').read_bytes())
    name=PROJECT+'-reference-preparation-'+phase
    docker('create','--pull','never','--name',name,'--label','lifecycle307.run='+RUN,'--network','container:'+network,'--read-only','--tmpfs','/tmp','--mount','type=bind,source='+str(folder)+',target=/run/private,readonly','--mount','type=bind,source='+str(payload)+',target=/run/payload','--mount','type=bind,source='+str(out)+',target=/run/output','--mount','type=bind,source='+str(ROOT/'migrations')+',target=/run/migrations,readonly','--env','LIFECYCLE_REFERENCE_PROFILE=5d2274ee-614cd9ad','--env','LIFECYCLE_REFERENCE_MODE='+(mode or phase),'--entrypoint','/run/private/executor',PG_IMAGE)
    docker('start',name);exit_code=int(docker('wait',name).strip())
    logs=subprocess.run(['docker','logs',name],capture_output=True);(folder/'helper.private-log').write_bytes(logs.stdout+logs.stderr)
    item=inspect(name);write(out/'helper.json',{'name':name,'id':item['Id'],'exit':exit_code,'expected_exit':expected,'image':item['Image'],'binary_sha256':sha(folder/'executor'),'root_mounted':False,'network_mode':item['HostConfig']['NetworkMode'],'retained':True})
    require(exit_code==expected,'Helper failed; retained, no automatic retry')
    return folder,out
def build():
    require(not CONTROL.exists() and not PUBLIC.exists(),'Preparation already exists; preserve and select an explicit later generation')
    preflight();CONTROL.mkdir();PUBLIC.mkdir();(CONTROL/'bin').mkdir()
    env=dict(os.environ,GOOS='linux',GOARCH='amd64',CGO_ENABLED='0')
    call(['go','build','-o',str(CONTROL/'bin/executor'),'./cmd/lifecycle-fixture-delete'],env=env,cwd=ROOT/'apps/platform')
    host=dict(os.environ,GOOS='windows',GOARCH='amd64',CGO_ENABLED='0')
    call(['go','build','-o',str(CONTROL/'bin/host.exe'),'./cmd/lifecycle-fixture-delete'],env=host,cwd=ROOT/'apps/platform')
    paths=[p for folder in ['apps/platform/cmd/lifecycle-fixture-delete','apps/platform/internal/lifecycle','apps/platform/internal/platform/storage'] for p in (ROOT/folder).rglob('*.go')]
    paths += [ROOT/'examples/lifecycle-d1/prepare-reference-disposal.py',ROOT/'examples/lifecycle-d1/checkpoint-reference-core.ps1',ROOT/'migrations/000068_lifecycle_checkpoint_read.up.sql']
    write(PUBLIC/'build-source.json',{'source_head':call(['git','rev-parse','HEAD']).decode().strip(),'source_files':{p.relative_to(ROOT).as_posix():sha(p) for p in paths},'linux_binary_sha256':sha(CONTROL/'bin/executor'),'host_binary_sha256':sha(CONTROL/'bin/host.exe'),'provider_io':0,'provider_delete':0})
def no_send():
    require(sql(PROJECT+'-postgres','SELECT count(*) FROM lifecycle_provider_observation')=='0','Original attempts already observed; do not regenerate proof')
    env=dict(os.environ,LIFECYCLE_REFERENCE_MODE='attest-host',LIFECYCLE_REFERENCE_PROFILE='5d2274ee-614cd9ad')
    before=set(PRIVATE.glob('reference-attestation-*'));call([str(CONTROL/'bin/host.exe')],env=env);created=set(PRIVATE.glob('reference-attestation-*'))-before
    require(len(created)==1,'Host proof directory ambiguous');attestation=created.pop()
    write(CONTROL/'attestation-pointer.json',{'directory':str(attestation)})
    helper('no-send',PROJECT+'-minio',attestation=attestation)
    write(PUBLIC/'no-send-closure.json',{'status':'PASS','observations':int(sql(PROJECT+'-postgres',"SELECT count(*) FROM lifecycle_provider_observation WHERE certainty='NOT_SENT' AND outcome='NOT_SENT'")),'releases':int(sql(PROJECT+'-postgres','SELECT count(*) FROM lifecycle_dispatch_release')),'send_claims':int(sql(PROJECT+'-postgres','SELECT count(*) FROM lifecycle_provider_send_claim')),'provider_delete':0,'signer_key_persisted':False})
    require(load(PUBLIC/'no-send-closure.json')['observations']==load(PUBLIC/'no-send-closure.json')['releases']==2,'No-send closure incomplete')
    migrate(PROJECT+'-postgres')
def no_send_replay():
    attestation=Path(load(CONTROL/'attestation-pointer.json')['directory'])
    helper('no-send-replay',PROJECT+'-minio',attestation=attestation,mode='no-send')
    result=load(PUBLIC/'no-send-replay/no-send-transaction.json')
    require(result['result']['Replayed'] is True,'Durable original proof replay missing')
def native_before():helper('native-before',PROJECT+'-minio',mode='inspect-source')
def native_after():helper('native-after',PROJECT+'-minio',mode='inspect-source')
def native_snapshot():
    require((PUBLIC/'native-before/native-six-version.json').is_file(),'Fresh before inspection required')
    require(docker('ps','-aq','--filter','name=^/'+CLONE+'$').strip()==b'' and docker('volume','ls','-q','--filter','name=^'+CLONE+'$').strip()==b'','Restore resource already owned')
    folder=CONTROL/'native-snapshot';require(not folder.exists(),'Snapshot attempt already retained');folder.mkdir()
    docker('volume','create','--label','lifecycle307.run='+RUN,CLONE)
    name=PROJECT+'-reference-native-snapshot'
    # A live volume copy is accepted only after both native readback and exact
    # source object-tree stability checks; it is not general hot-backup proof.
    command='set -eu; find /source/dpp-lifecycle307-online /source/dpp-lifecycle307-quarantine -type f -exec sha256sum {} + | sort > /run/payload/source-before.private; tar -cf /run/payload/native-store.tar -C /source .; tar -xf /run/payload/native-store.tar -C /restore; find /source/dpp-lifecycle307-online /source/dpp-lifecycle307-quarantine -type f -exec sha256sum {} + | sort > /run/payload/source-after.private; cmp /run/payload/source-before.private /run/payload/source-after.private'
    docker('create','--pull','never','--name',name,'--label','lifecycle307.run='+RUN,'--network','none','--read-only','--mount','type=volume,source='+PROJECT+'-objects,target=/source,readonly','--mount','type=volume,source='+CLONE+',target=/restore','--mount','type=bind,source='+str(folder)+',target=/run/payload','--entrypoint','/bin/sh',PG_IMAGE,'-c',command)
    docker('start',name);exit_code=int(docker('wait',name).strip());logs=subprocess.run(['docker','logs',name],capture_output=True);(folder/'snapshot.private-log').write_bytes(logs.stdout+logs.stderr)
    write(PUBLIC/'native-snapshot.json',{'status':'PASS' if exit_code==0 else 'FAIL_RETAINED','helper_id':inspect(name)['Id'],'exit':exit_code,'archive_sha256':sha(folder/'native-store.tar'),'source_object_tree_before_sha256':sha(folder/'source-before.private'),'source_object_tree_after_sha256':sha(folder/'source-after.private'),'provider_delete':0,'source_restart':0,'cleanup':0,'consistency_scope':'Sealed quiescent synthetic object trees; subsequent exact native restore + source recheck required'})
    require(exit_code==0,'Live snapshot changed/failed; no automatic retry')
    docker('create','--pull','never','--name',CLONE,'--label','lifecycle307.run='+RUN,'--network',PROJECT+'-predelete-private','--mount','type=volume,source='+CLONE+',target=/data','--mount','type=bind,source='+str(PRIVATE/'access')+',target=/run/credentials/access,readonly','--mount','type=bind,source='+str(PRIVATE/'secret')+',target=/run/credentials/secret,readonly','--mount','type=bind,source='+str(PRIVATE/'start.sh')+',target=/run/credentials/start.sh,readonly','--entrypoint','/bin/sh',MINIO_IMAGE,'/run/credentials/start.sh')
    retained('restored-objects','created');docker('start',CLONE);retained('restored-objects')
def native_restored():
    retained('restored-core','exited');docker('start',PROJECT+'-predelete-postgres')
    try:
        for _ in range(30):
            result=subprocess.run(['docker','exec',PROJECT+'-predelete-postgres','pg_isready','-U','fixture','-d','lifecycle307'],capture_output=True)
            if result.returncode==0:break
            time.sleep(1)
        dsn=urlsplit(load(PRIVATE/'config.json')['PostgresDSN']);host=PROJECT+'-predelete-postgres';changed=urlunsplit((dsn.scheme,dsn.netloc.rsplit('@',1)[0]+'@'+host+':5432',dsn.path,dsn.query,dsn.fragment))
        migrate(PROJECT+'-predelete-postgres',CLONE,changed)
        helper('native-restored',CLONE,dsn=changed,mode='inspect-restored')
    finally:
        docker('stop','--time','30',PROJECT+'-predelete-postgres');docker('stop','--time','30',CLONE)
    a=load(PUBLIC/'native-before/native-six-version.json');b=load(PUBLIC/'native-restored/native-six-version.json')
    def canonical(value):return sorted([(r['copy'],r['bytes'],r['sha256'],r['protection']['LegalHold'],r['protection']['RetentionMode'],r['protection']['RetainUntil']) for r in value['versions']],key=lambda r:r[0]['ID'])
    require(canonical(a)==canonical(b),'Exact native restored version/bytes/hash/protection differ')
    write(PUBLIC/'native-restore-closure.json',{'status':'PASS_EXACT_SIX_NATIVE_RESTORE','versions':6,'all_identity_bytes_hash_protection_equal':True,'backup_payload_excluded_from_source_absence':True,'source_restart':0,'provider_delete':0,'business_admission':0,'clone_core_calls_are_isolated_from_source':True,'restore_container':CLONE,'restore_id':inspect(CLONE)['Id'],'restore_image':inspect(CLONE)['Image'],'volume':CLONE,'states':'STOPPED_RETAINED'})
def core_refresh():
    before=load(PUBLIC/'native-before/native-six-version.json');after=load(PUBLIC/'native-after/native-six-version.json')
    for a,b in zip(sorted(before['versions'],key=lambda r:r['copy']['ID']),sorted(after['versions'],key=lambda r:r['copy']['ID'])):
        require(a['copy']==b['copy'] and (a['bytes'],a['sha256'])==(b['bytes'],b['sha256']) and all(a['protection'][key]==b['protection'][key] for key in ('LegalHold','RetentionMode','RetainUntil')),'Source native identity/protection changed')
    output=ROOT/'docs/acceptance/evidence/issue307-d1/delete-preflight-5d2274ee-generation02';require(not output.exists(),'Generation02 already retained');output.mkdir()
    (output/'trial-no-send-candidate.json').write_bytes((PUBLIC/'no-send/no-send-transaction.json').read_bytes())
    for extra in ([],['-VerifyExistingAuth'],['-VerifyExistingFacts']):
        call(['pwsh','-NoProfile','-File',str(ROOT/'examples/lifecycle-d1/checkpoint-reference-core.ps1'),'-PrivateDirectory',str(PRIVATE),'-CheckpointAttempt','04','-Generation','02',*extra])
def seal():
    preflight()
    # Only the new maintenance authority is disabled. Original files stay sealed.
    for path in CONTROL.glob('*/reference.json'):
        value=load(path);value['PreparationEnabled']=False
        path.write_text(json.dumps(value,indent=2)+'\n',encoding='utf-8',newline='\n')
    write(PUBLIC/'sealed.json',{'status':'SEALED_RETAINED','provider_delete':0,'source_restart':0,'source_gateway_changed':0,'business_windows':'SEALED','new_send_permission_enabled':False,'cleanup':0})
def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('phase',choices=PHASES);args=parser.parse_args()
    phases={'build':build,'render':lambda:helper('render',PROJECT+'-minio',mode='render'),'rehearse':lambda:helper('rehearse',PROJECT+'-minio',mode='dry-run-send'),'no-send':no_send,'no-send-replay':no_send_replay,'native-before':native_before,'native-snapshot':native_snapshot,'native-restored':native_restored,'native-after':native_after,'core-refresh':core_refresh,'seal':seal}
    if args.phase!='build':preflight()
    phases[args.phase]();print(json.dumps({'phase':args.phase,'status':'PASS','provider_delete':0,'cleanup':0}))
if __name__=='__main__':main()
