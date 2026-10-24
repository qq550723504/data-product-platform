"""One new, isolated production adapter contract; never a source resource."""
from pathlib import Path
import datetime as dt
import hashlib
import json
import os
import subprocess
import time
import uuid

ROOT=Path.cwd()
PUBLIC=Path(__file__).resolve().parent.parent
PRIVATE=Path(r'C:\Users\Henry\.codex\fixture-private\disabled-bootstrap-native04')
MI='sha256:72476ee7f52a538e561627f7d8746e20c15d50c13e8bcdc58b22fa65cff94df6'
PG='sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea'

def save(path,value):
    with path.open('xb') as stream:stream.write((json.dumps(value,ensure_ascii=False,indent=2)+'\n').encode())

def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()

def call(args):
    result=subprocess.run(args,cwd=ROOT,capture_output=True)
    if result.returncode:raise RuntimeError('scratch operation failed: '+str(args[:2])+'; original CLI stderr held privately')
    return result.stdout

def inspect(cid):return json.loads(call(['docker','inspect',cid]))[0]

def main():
    PRIVATE.mkdir();cfg=PRIVATE/'config';cfg.mkdir();output=PUBLIC/'outputs';output.mkdir()
    audit=str(uuid.uuid4());name='dpp307-disabled-native-'+audit[:8]
    access='contract'+uuid.uuid4().hex[:16];secret=uuid.uuid4().hex+uuid.uuid4().hex
    save(cfg/'contract.json',dict(RootAccess=access,RootSecret=secret,Output='/run/output'))
    (cfg/'root-access').write_text(access,encoding='utf8');(cfg/'root-secret').write_text(secret,encoding='utf8')
    (cfg/'start.sh').write_bytes(b'export MINIO_ROOT_USER="$(cat /run/private/root-access)"\nexport MINIO_ROOT_PASSWORD="$(cat /run/private/root-secret)"\nexec minio server /data --address :9000 --console-address :9001\n')
    env=os.environ.copy();env.update(GOOS='linux',GOARCH='amd64',CGO_ENABLED='0')
    build=subprocess.run(['go','test','-c','-trimpath','-buildvcs=false','-o',str(PRIVATE/'reader-contract.test'),'./cmd/lifecycle-fixture-reader'],cwd=ROOT/'apps/platform',env=env,capture_output=True)
    (PUBLIC/'build.stdout.txt').write_bytes(build.stdout);(PUBLIC/'build.stderr.txt').write_bytes(build.stderr)
    if build.returncode:raise RuntimeError('native contract build failed')
    names=subprocess.check_output(['git','ls-files','--cached','--others','--exclude-standard','apps/platform/cmd/lifecycle-fixture-reader','apps/platform/internal/lifecycle/pilot'],cwd=ROOT).decode().splitlines()
    names+=['apps/platform/cmd/lifecycle-fixture-reader/six_mapping.go','apps/platform/cmd/lifecycle-fixture-reader/six_mapping_test.go']
    save(PUBLIC/'intent.json',dict(audit=audit,image=MI,worker_image=PG,binary_sha256=sha(PRIVATE/'reader-contract.test'),source_files={p:sha(ROOT/p) for p in sorted(set(names))},scope='Fresh scratch production adapter only; no source/objects/DELETE'))
    cids=[];failure=None
    try:
        volume=call(['docker','volume','create','--label','lifecycle307.scratch='+audit,name+'-objects']).decode().strip()
        native=call(['docker','create','--pull','never','--name',name+'-native','--label','lifecycle307.scratch='+audit,'--network','none','--mount','type=volume,source='+volume+',target=/data','--mount',f'type=bind,source={cfg},target=/run/private,readonly','--entrypoint','/bin/sh',MI,'/run/private/start.sh']).decode().strip();cids.append(native)
        save(PUBLIC/'native-owner.json',dict(cid=native,new_volume=volume,network='none',image=MI))
        worker=call(['docker','create','--pull','never','--name',name+'-worker','--label','lifecycle307.scratch='+audit,'--user','10001:10001','--network','container:'+native,'--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data:size=1m','--workdir','/run/reader-tests','--env','DPP307_SIX_NATIVE_CONTRACT=/run/private/contract.json','--mount',f'type=bind,source={cfg},target=/run/private,readonly','--mount',f'type=bind,source={PRIVATE}/reader-contract.test,target=/run/contract.test,readonly','--mount',f'type=bind,source={ROOT}/apps/platform/cmd/lifecycle-fixture-reader/testdata,target=/run/reader-tests/testdata,readonly','--mount',f'type=bind,source={output},target=/run/output','--entrypoint','/run/contract.test',PG,'-test.run','^TestSixNativeDisabledBootstrapContract$','-test.v','-test.timeout','60s']).decode().strip();cids.append(worker)
        save(PUBLIC/'worker-owner.json',dict(cid=worker,network_namespace=native,image=PG,binary_sha256=sha(PRIVATE/'reader-contract.test')))
        call(['docker','start',native]);time.sleep(3);call(['docker','start',worker])
        wait=call(['docker','wait',worker]);(PUBLIC/'wait.stdout.txt').write_bytes(wait)
        logs=subprocess.run(['docker','logs',worker],capture_output=True);(PUBLIC/'worker.stdout.txt').write_bytes(logs.stdout);(PUBLIC/'worker.stderr.txt').write_bytes(logs.stderr)
        if wait.strip()!=b'0':raise RuntimeError('production native adapter contract failed; see original logs')
        if json.loads((output/'native-contract-result.json').read_text())['status']!='PASS_PRODUCTION_DISABLED_BOOTSTRAP_ADAPTER':raise RuntimeError('adapter result absent')
    except BaseException as error:
        failure=dict(type=type(error).__name__,message=str(error));raise
    finally:
        closure=[]
        for cid in reversed(cids):
            try:
                value=inspect(cid)
                if value['State']['Running']:call(['docker','stop','--time','5',cid])
                value=inspect(cid)
                closure.append(dict(cid=cid,closed=not value['State']['Running'] and value['State']['Pid']==0,pid=value['State']['Pid'],network=value['HostConfig']['NetworkMode'],noports=not value['HostConfig']['PortBindings'] and not any(value['NetworkSettings']['Ports'].values()),restart=value['HostConfig']['RestartPolicy']['Name']))
            except Exception as error:closure.append(dict(cid=cid,closed=False,error=type(error).__name__))
        save(PUBLIC/'terminal.json',dict(status='PASS_PRODUCTION_NATIVE_ADAPTER_CONTRACT' if failure is None and all(x['closed'] for x in closure) else 'FAILED_OR_UNCONFIRMED_RETAINED',at=dt.datetime.now(dt.timezone.utc).isoformat(),failure=failure,closure=closure,technical_starts=len(list(output.glob('prepare-start-*.json'))),observations=len(list(output.glob('prepare-observation-*.json'))),amount='NOT_ASSESSED',native_delete=0,object_put=0,source_calls=0,resources_retained=True))
    print(json.dumps(dict(terminal_sha256=sha(PUBLIC/'terminal.json'),result=json.loads((PUBLIC/'terminal.json').read_text()))))

if __name__=='__main__':main()
