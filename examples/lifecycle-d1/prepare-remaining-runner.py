"""Fixed v16 business runner preparation and a separate non-delete restore run.

No DELETE route, sender credential, hold release, cleanup or merge operation is
available. All allocations, including failed helpers, are retained. Never retry
this one-shot entry against an already allocated directory.
"""
import datetime as dt
import importlib.util
import json
import os
from pathlib import Path
import secrets
import subprocess
import time
import uuid
import ipaddress
import sys
from urllib.parse import urlsplit,urlunsplit

ROOT=Path(__file__).resolve().parents[2]
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private')
PUBLIC=ROOT/'docs/acceptance/evidence/issue307-d1/remaining-runner01'
PG='sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea'
MI='sha256:72476ee7f52a538e561627f7d8746e20c15d50c13e8bcdc58b22fa65cff94df6'
GW='sha256:d8542f48d34a9cf4e4c11a478865229840e87e4c96ea3f439101f31a5d35f75f'
REDIS='sha256:858f009f9709ce576febc734aa78b8f6d624b82571f9ddb6bda4377c833b3499'
PROJECT='dpp-lifecycle307-808a6960'

spec=importlib.util.spec_from_file_location('native_prepare',ROOT/'examples/lifecycle-d1/prepare-remaining-native.py')
native=importlib.util.module_from_spec(spec);spec.loader.exec_module(native)
require,sha,load,write=native.require,native.sha,native.load,native.write

def output_bytes(value):
    """Raw process output has one contract; text consumers explicitly decode."""
    if not isinstance(value, bytes):
        raise TypeError('command output must be bytes')
    return value

def call(args,data=None,output=None):
    if output:
        with output.open('xb') as stream:r=subprocess.run(list(map(str,args)),cwd=ROOT,input=data,stdout=stream,stderr=subprocess.PIPE)
    else:r=subprocess.run(list(map(str,args)),cwd=ROOT,input=data,capture_output=True)
    stdout=output_bytes(r.stdout if r.stdout is not None else b'')
    stderr=output_bytes(r.stderr if r.stderr is not None else b'')
    if r.returncode:
        log=PRIVATE/'remaining-runner01'/('failure-'+dt.datetime.now(dt.timezone.utc).strftime('%H%M%S%f')+'.private-log')
        log.write_bytes(stdout+b'\n'+stderr)
        raise RuntimeError('Command failed; diagnostics/resources retained: '+log.name)
    return stdout
def docker(*args,**kwargs):return output_bytes(call(['docker',*args],**kwargs))

def docker_text(*args,**kwargs):
    """The only text boundary, with strict UTF-8 and no str/bytes fallback."""
    return output_bytes(docker(*args,**kwargs)).decode('utf-8')
def inspect(name):return json.loads(docker('inspect',name))[0]
def utc():return dt.datetime.now(dt.timezone.utc)

def checkpoint_dsn(dsn,host):
    parsed=urlsplit(dsn)
    require(parsed.scheme=='postgres' and parsed.hostname==PROJECT+'-postgres' and parsed.port==5432 and parsed.path=='/lifecycle307' and parsed.username=='fixture' and parsed.password and parsed.query=='sslmode=disable','Unexpected source Core DSN')
    require(host==PROJECT+'-remaining-regression-pg' and host!=parsed.hostname,'Unexpected clone Core host')
    result=urlunsplit(parsed._replace(netloc=parsed.netloc.rsplit('@',1)[0]+'@'+host+':5432'))
    require(urlsplit(result).hostname==host,'Clone DSN substitution failed')
    return result

def check_subnet(value):
    subnet=ipaddress.ip_network(value)
    ids=docker('network','ls','-q').decode().split()
    for ident in ids:
        network=json.loads(docker('network','inspect',ident))[0]
        for item in network.get('IPAM',{}).get('Config') or []:
            if item.get('Subnet'):
                existing=ipaddress.ip_network(item['Subnet'])
                require(subnet.version!=existing.version or not subnet.overlaps(existing),'Regression subnet overlaps '+network['Name'])
    return str(subnet)

def gateway(c,copies):
    lines=['{\n admin off\n auto_https off\n}\n:9000 {\n route {',
           '  @delete method DELETE\n  respond @delete 403',
           '  @bypass header X-Amz-Bypass-Governance-Retention *\n  respond @bypass 403',
           '  @copy header X-Amz-Copy-Source *\n  respond @copy 403']
    for i,o in enumerate(copies):
        if o['Key'] in {'fixture/control-hold.csv','fixture/control-retention.csv'}:continue
        uri='/'+o['Namespace']+'/'+o['Key']+'?versionId='+o['ProviderVersion']
        lines.append(f"  @read{i} {{\n   method GET\n   expression {{http.request.header.Authorization}}.contains('Credential={c['Access']}/') && {{http.request.uri}} == '{uri}'\n  }}\n  reverse_proxy @read{i} minio:9000")
    lines.append(f"  @inspect {{\n   method GET HEAD\n   expression {{http.request.header.Authorization}}.contains('Credential={c['InspectAccess']}/')\n  }}\n  reverse_proxy @inspect minio:9000")
    for i,key in enumerate(('remaining-standardized.csv','remaining-processed.csv')):
        lines.append(f"  @put{i} {{\n   method PUT\n   expression {{http.request.header.If-None-Match}} == '*' && {{http.request.header.Authorization}}.contains('Credential={c['ProducerAccess']}/') && {{http.request.uri}} == '/dpp-lifecycle307-online/fixture/{key}'\n  }}\n  reverse_proxy @put{i} minio:9000")
    return '\n'.join(lines+['  respond 403\n }\n}\n']).encode()

def main():
    global PUBLIC
    continuation=sys.argv[1:]==['--continue-e311423a-empty-datasets']
    require(not sys.argv[1:] or continuation,'Unsupported finite continuation')
    generation='02' if continuation else '01'
    control=PRIVATE/('remaining-runner'+generation);require(not control.exists(),'One-shot runner directory already allocated')
    require(not call(['git','status','--porcelain']).strip(),'Commit all source before native runner preparation')
    subnet=check_subnet('10.234.169.0/24')
    if continuation:
        old=ROOT/'docs/acceptance/evidence/issue307-d1/remaining-runner01'
        failure=load(old/'setup.json');build=load(old/'build.json')
        require(failure['container']=='f040101a6f7e2a0654958dec3842714356e5762f009bc4d02610c1d7ce124e12' and failure['exit_code']==1 and build['source_commit']=='e311423a81dd9fd6848aad90acbb41e270537511' and build['binary_sha256']=='edfd2d3aa96df8391d525c4b37963d723278b5f2e15398344b44454352e7f1a5','Different retained failure')
        require(all(not inspect(x['id'])['State']['Running'] for x in load(old/'seal.json')['containers']),'Prior helper active')
        PUBLIC=ROOT/'docs/acceptance/evidence/issue307-d1/remaining-runner02'
    control.mkdir();PUBLIC.mkdir()
    private=PRIVATE/PROJECT;cfg=load(private/'config.json')
    source=call(['git','rev-parse','HEAD']).decode().strip()
    for image in (PG,MI,GW,REDIS):require(inspect(image)['Id']==image,'Cached pinned image differs')
    base_manifest=ROOT/'docs/acceptance/evidence/issue307-d1/remaining-native01/business-races/manifest.json'
    scope=json.loads((ROOT/'apps/platform/internal/lifecycle/pilot/remaining_scope.json').read_text())[0]
    batch=str(uuid.uuid4());now=utc();owned=[];active=[];sealed=(private/'Caddyfile').read_bytes()
    output=PUBLIC/'business';output.mkdir();admin=control/'admin';owner=control/'owner';admin.mkdir();owner.mkdir()
    for path in (admin,owner):(path/'manifest.json').write_bytes(base_manifest.read_bytes())
    (owner/'rules/fixture').mkdir(parents=True)
    for name,column in (('T1','source_company_id'),('T2','id')):
        (owner/f'rules/fixture/remaining-{name}.yaml').write_text(f'''apiVersion: dataprod.platform/v1alpha1
kind: QualityRuleSet
metadata:
  name: remaining-{name}
  version: 1.0.0
spec:
  rules:
    - id: LC-SYNTHETIC-ID
      stage: RAW
      dimension: COMPLETENESS
      type: completeness_ratio
      target: {column}
      threshold: 1
      required: true
      severity: CRITICAL
''')
    c={key:cfg[key] for key in ('RunID','WorkspaceID','ProviderInstanceID','IncarnationID','ActorID','Endpoint','PostgresDSN','InspectAccess','InspectSecret')}
    c.update(Enabled=True,Epoch=str(uuid.uuid4()),Token=secrets.token_hex(32),Access='remainread808a6960',Secret=secrets.token_hex(32),ProducerAccess='remainput808a6960',ProducerSecret=secrets.token_hex(32),OutputDirectory='/run/output',ManifestSHA256=sha(base_manifest),StartedAt=now.isoformat(),ExpiresAt=(now+dt.timedelta(minutes=55)).isoformat(),RedisAddr=PROJECT+'-remaining-redis:6379')
    if continuation:
        prior=load(PRIVATE/'remaining-runner01/owner/remaining.json')['Base']
        for key in ('Access','Secret','ProducerAccess','ProducerSecret'):c[key]=prior[key]
        write(owner/'setup-resume.json',{'StandardizedDatasetID':'667e9747-242e-418b-aa37-e1ea46d92aca','OutputDatasetID':'7b5f045a-170d-4f10-b7c2-2d5d609940d5','StandardizedVersionID':str(uuid.UUID(int=0)),'WorkflowVersionID':str(uuid.UUID(int=0)),'ProfileID':str(uuid.UUID(int=0))})
    worker={'Base':c,'Group':'business-races','Batch':batch,'NonDelete':True}
    root=json.loads(json.dumps(worker));root['Base'].update(RootAccess=cfg['RootAccess'],RootSecret=cfg['RootSecret'])
    write(owner/'remaining.json',worker);write(admin/'remaining.json',root)
    env=os.environ.copy();env.update(GOOS='linux',GOARCH='amd64',CGO_ENABLED='0')
    result=subprocess.run(['go','build','-trimpath','-buildvcs=false','-o',str(control/'reader'),'./cmd/lifecycle-fixture-reader'],cwd=ROOT/'apps/platform',env=env,capture_output=True)
    require(result.returncode==0,'Linux reader build failed')
    write(PUBLIC/'build.json',{'source_commit':source,'binary_sha256':sha(control/'reader'),'batch':batch,'go':call(['go','version']).decode().strip(),'delete':0})
    def helper(step,rooted=False,gw=None,pgconfig=None,out=None):
        name=PROJECT+'-remaining-'+generation+'-'+step
        mode='bind-outputs' if step.endswith('bind-outputs') else step.split('-')[-1]
        args=['create','--pull','never','--name',name,'--label','lifecycle307.remaining='+batch,'--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp','--network','container:'+(gw or PROJECT+('-minio' if rooted else '-gateway')),
              '--mount',f'type=bind,source={admin if rooted else (pgconfig or owner)},target=/run/private,readonly',
              '--mount',f'type=bind,source={control}/reader,target=/run/reader,readonly',
              '--mount',f'type=bind,source={out or output},target=/run/output',
              '--mount',f'type=bind,source={ROOT}/industry-packs,target=/run/repo/industry-packs,readonly',
              '--mount',f'type=bind,source={owner}/rules,target=/run/rules,readonly','--entrypoint','/run/reader',PG,'-mode','remaining-'+mode]
        ident=docker(*args).decode().strip();owned.append(ident);docker('start',ident)
        code=int(docker('wait',ident));logs=docker('logs',ident)
        (PUBLIC/(step+'.stdout.txt')).write_bytes(logs)
        write(PUBLIC/(step+'.json'),{'container':ident,'source':source,'binary_sha256':sha(control/'reader'),'exit_code':code,'root_mount':rooted,'delete':0})
        require(code==0,'Native '+step+' failed; no retry')
    def install(copies):
        raw=gateway(c,copies);(private/'Caddyfile').write_bytes(raw)
        name=PROJECT+'-gateway';state=inspect(name)
        if state['State']['Running']:docker('stop','--time','15',name)
        docker('start',name)
        time.sleep(1)
        (PUBLIC/('gateway-'+str(len(copies))+'.Caddyfile')).write_bytes(raw)
    try:
        for service in ('postgres','minio'):
            name=PROJECT+'-'+service;require(not inspect(name)['State']['Running'],'Source service unexpectedly active');docker('start',name);active.append(name)
        native.wait_pg(PROJECT+'-postgres','fixture');time.sleep(3)
        if not continuation:helper('provision',True)
        install(scope['Copies']);active.append(PROJECT+'-gateway')
        redis=PROJECT+'-remaining-redis';volume=redis+'-data'
        if continuation:redisid=inspect(redis)['Id']
        else:
            docker('volume','create','--label','lifecycle307.remaining='+batch,volume)
            redisid=docker('create','--pull','never','--name',redis,'--label','lifecycle307.remaining='+batch,'--network',PROJECT+'-private','--mount','type=volume,source='+volume+',target=/data',REDIS,'redis-server','--appendonly','yes').decode().strip()
        owned.append(redisid);docker('start',redisid)
        helper('setup')
        helper('bind-outputs',True)
        copies=json.loads(native.sql(PROJECT+'-postgres',"SELECT json_agg(json_build_object('ID',c.id,'Namespace',n.namespace,'Key',c.object_key,'ProviderVersion',c.provider_version)) FROM lifecycle_fixture_copy c JOIN lifecycle_fixture_namespace n ON n.id=c.namespace_id;"))
        install(copies)
        helper('baseline')
        helper('final-bind-outputs',True)
        copies=json.loads(native.sql(PROJECT+'-postgres',"SELECT json_agg(json_build_object('ID',c.id,'Namespace',n.namespace,'Key',c.object_key,'ProviderVersion',c.provider_version,'DatasetVersionID',c.dataset_version_id,'ContentSHA256',c.content_sha256,'ByteSize',c.byte_size)) FROM lifecycle_fixture_copy c JOIN lifecycle_fixture_namespace n ON n.id=c.namespace_id;"))
        install(copies)
        write(PUBLIC/'candidate-business-inventory.json',copies)
        before=native.fingerprints(PROJECT+'-postgres');write(PUBLIC/'candidate-core-before.json',before)
        checkpoint=control/'checkpoint';checkpoint.mkdir();(checkpoint/'password').write_text(secrets.token_hex(32))
        docker('exec',PROJECT+'-postgres','pg_dump','-U','fixture','-d','lifecycle307','--format=custom','--create',output=checkpoint/'core.dump')
        docker('exec',PROJECT+'-postgres','pg_dumpall','-U','fixture','--globals-only',output=checkpoint/'globals.sql')
        docker('stop','--time','20',PROJECT+'-minio')
        def utility(suffix,command,mounts):
            ident=docker('create','--pull','never','--name',PROJECT+'-remaining-'+suffix,'--label','lifecycle307.remaining='+batch,'--network','none','--read-only','--tmpfs','/tmp',*mounts,'--entrypoint','/bin/sh',PG,'-ec',command).decode().strip();owned.append(ident);docker('start',ident);require(docker('wait',ident).strip()==b'0',suffix+' failed');return ident
        utility('archive','exec tar -C /source -cpf /backup/native.tar .',['--mount','type=volume,source='+PROJECT+'-objects,target=/source,readonly','--mount',f'type=bind,source={checkpoint},target=/backup'])
        # Restore a business regression copy. No source database is replaced.
        prefix=PROJECT+'-remaining-regression';network=prefix+'-private'
        docker('network','create','--internal','--subnet',subnet,'--label','lifecycle307.remaining='+batch,network)
        for kind in ('core','objects','redis'):docker('volume','create','--label','lifecycle307.remaining='+batch,prefix+'-'+kind)
        pg=docker('create','--pull','never','--name',prefix+'-pg','--label','lifecycle307.remaining='+batch,'--network',network,'--mount','type=volume,source='+prefix+'-core,target=/var/lib/postgresql/data','--mount',f'type=bind,source={checkpoint},target=/run/backup,readonly','-e','POSTGRES_USER=checkpoint_admin','-e','POSTGRES_DB=postgres','-e','POSTGRES_PASSWORD_FILE=/run/backup/password',PG).decode().strip();owned.append(pg);docker('start',pg);native.wait_pg(pg,'checkpoint_admin')
        docker('exec',pg,'psql','-X','-U','checkpoint_admin','-d','postgres','-v','ON_ERROR_STOP=1','-f','/run/backup/globals.sql')
        docker('exec',pg,'pg_restore','--exit-on-error','--create','--role=fixture','-U','checkpoint_admin','-d','postgres','/run/backup/core.dump')
        restored=native.fingerprints(pg);write(PUBLIC/'regression-core-restored.json',restored);require(before==restored,'Restored Core rows/roles/ACL differ')
        utility('unpack','exec tar -C /restore -xpf /backup/native.tar .',['--mount','type=volume,source='+prefix+'-objects,target=/restore','--mount',f'type=bind,source={checkpoint},target=/backup,readonly'])
        cmp=utility('compare','set -o pipefail; cd /source; find . -type f -exec sha256sum {} \\; | sort > /tmp/a; cd /restore; find . -type f -exec sha256sum {} \\; | sort > /tmp/b; cmp /tmp/a /tmp/b; cat /tmp/a',['--mount','type=volume,source='+PROJECT+'-objects,target=/source,readonly','--mount','type=volume,source='+prefix+'-objects,target=/restore,readonly'])
        (PUBLIC/'native-restored-file-sha256.txt').write_bytes(docker('logs',cmp))
        mi=docker('create','--pull','never','--name',prefix+'-minio','--label','lifecycle307.remaining='+batch,'--network',network,'--network-alias','minio','--mount','type=volume,source='+prefix+'-objects,target=/data','--mount',f'type=bind,source={private},target=/run/credentials,readonly','--entrypoint','/bin/sh',MI,'/run/credentials/start.sh').decode().strip();owned.append(mi);docker('start',mi)
        gw=docker('create','--pull','never','--name',prefix+'-gateway','--label','lifecycle307.remaining='+batch,'--network',network,'--mount',f'type=bind,source={private}/Caddyfile,target=/etc/caddy/Caddyfile,readonly',GW,'caddy','run','--config','/etc/caddy/Caddyfile').decode().strip();owned.append(gw);docker('start',gw)
        rd=docker('create','--pull','never','--name',prefix+'-redis','--label','lifecycle307.remaining='+batch,'--network',network,'--network-alias',redis,'--mount','type=volume,source='+prefix+'-redis,target=/data',REDIS,'redis-server','--appendonly','yes').decode().strip();owned.append(rd);docker('start',rd);time.sleep(3)
        regression=control/'regression';regression.mkdir();regout=PUBLIC/'regression';regout.mkdir()
        (regression/'manifest.json').write_bytes(base_manifest.read_bytes());(regout/'remaining-state.json').write_bytes((output/'remaining-state.json').read_bytes())
        reg=json.loads(json.dumps(worker));reg['Base']['PostgresDSN']=checkpoint_dsn(reg['Base']['PostgresDSN'],prefix+'-pg');write(regression/'remaining.json',reg)
        require(inspect(pg)['NetworkSettings']['Networks'][network]['NetworkID']==inspect(mi)['NetworkSettings']['Networks'][network]['NetworkID']==inspect(gw)['NetworkSettings']['Networks'][network]['NetworkID'],'Clone physical network differs')
        write(PUBLIC/'regression-topology.json',{'core':pg,'minio':mi,'gateway':gw,'network':network,'subnet':subnet,'source_core':inspect(PROJECT+'-postgres')['Id'],'source_minio':inspect(PROJECT+'-minio')['Id'],'core_fingerprint_match':before==restored,'no_host_ports':True})
        helper('regression-business',gw=gw,pgconfig=regression,out=regout)
        helper('regression-seal',True,gw=mi,out=regout)
        docker('start',PROJECT+'-minio');time.sleep(3)
        helper('seal',True)
        write(PUBLIC/'checkpoint.json',{'source':source,'batch':batch,'core_dump_sha256':sha(checkpoint/'core.dump'),'globals_sha256':sha(checkpoint/'globals.sql'),'native_archive_sha256':sha(checkpoint/'native.tar'),'core_full_restore':'PASS','native_file_restore':'PASS','candidate_source_copies':len(copies),'new_survivors':len(copies)-6,'regression_core_container':pg,'regression_minio_container':mi,'actual_delete':0})
    finally:
        # Restore the candidate's sealed gateway. All newly allocated resources
        # are stopped with their data preserved; failures are never discarded.
        for name in reversed(owned+active):
            state=inspect(name)
            if state['State']['Running']:docker('stop','--time','20',state['Id'])
        (private/'Caddyfile').write_bytes(sealed)
        seals=[]
        for name in dict.fromkeys(owned+active):
            state=inspect(name);require(not state['State']['Running'] and state['State']['Pid']==0 and not any(state['NetworkSettings']['Ports'].values()),'Resource not sealed')
            seals.append({'id':state['Id'],'name':state['Name'],'image':state['Image'],'status':state['State']['Status'],'pid':state['State']['Pid'],'ports':state['NetworkSettings']['Ports']})
        write(PUBLIC/'seal.json',{'containers':seals,'delete':0,'cleanup':0,'retained':True})
    print('Native business non-delete runner complete; resources retained',flush=True)

if __name__=='__main__':main()
