"""Finite continuation after run02 pg_restore startup race; never reruns setup.

Only the already-allocated empty clone receives the retained dump. Original Core
and objects are preserved. No native deletion credentials or routes are mounted.
"""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import time

ROOT=Path(__file__).resolve().parents[2]
spec=importlib.util.spec_from_file_location('runner',ROOT/'examples/lifecycle-d1/prepare-remaining-runner.py')
r=importlib.util.module_from_spec(spec);spec.loader.exec_module(r)
require,load,write,sha,docker,inspect=r.require,r.load,r.write,r.sha,r.docker,r.inspect

def main():
    require(not r.call(['git','status','--porcelain']).strip(),'Commit continuation source first')
    control=r.PRIVATE/'remaining-runner03';require(not control.exists(),'Continuation already allocated');control.mkdir()
    out=ROOT/'docs/acceptance/evidence/issue307-d1/remaining-runner03';out.mkdir()
    prior=ROOT/'docs/acceptance/evidence/issue307-d1/remaining-runner02';private=r.PRIVATE/'remaining-runner02'
    build=load(prior/'build.json');require(build['source_commit']=='892f17c7ada7586517baa27f53ebbdfba51ce52c' and build['binary_sha256']=='e97eb91064562232cbdae56840d60ae6c7c531741c508c5f07c69ac85d7296a2','Different run02 binary')
    batch=build['batch'];prefix=r.PROJECT+'-remaining-regression';pg=inspect(prefix+'-pg')['Id']
    require(pg=='365f9baafede4693f2f73caa36c6f6564a83b601a95be40ec9d91b7c46048b87' and not inspect(pg)['State']['Running'],'Different checkpoint Core')
    checkpoint=private/'checkpoint';before=load(prior/'candidate-core-before.json');owned=[]
    source=r.call(['git','rev-parse','HEAD']).decode().strip()
    env=os.environ.copy();env.update(GOOS='linux',GOARCH='amd64',CGO_ENABLED='0')
    result=subprocess.run(['go','build','-trimpath','-buildvcs=false','-o',str(control/'reader'),'./cmd/lifecycle-fixture-reader'],cwd=ROOT/'apps/platform',env=env,capture_output=True)
    require(result.returncode==0,'Build failed');write(out/'build.json',{'source_commit':source,'binary_sha256':sha(control/'reader'),'prior_build':build,'delete':0})
    network=prefix+'-private';ni=json.loads(docker('network','inspect',network))[0]
    require(ni['Internal'] and ni['IPAM']['Config'][0]['Subnet']=='10.234.169.0/24' and ni['Labels']['lifecycle307.remaining']==batch,'Clone network differs')
    worker=load(private/'owner/remaining.json');r.checkpoint_dsn(worker['Base']['PostgresDSN'],prefix+'-pg')
    reg=control/'regression';reg.mkdir();regout=out/'regression';regout.mkdir()
    (reg/'manifest.json').write_bytes((private/'owner/manifest.json').read_bytes());(regout/'remaining-state.json').write_bytes((prior/'business/remaining-state.json').read_bytes())
    worker['Base']['PostgresDSN']=r.checkpoint_dsn(worker['Base']['PostgresDSN'],prefix+'-pg');write(reg/'remaining.json',worker)
    def utility(name,command,mounts):
        ident=docker('create','--pull','never','--name',r.PROJECT+'-remaining-03-'+name,'--label','lifecycle307.remaining='+batch,'--network','none','--read-only','--tmpfs','/tmp',*mounts,'--entrypoint','/bin/sh',r.PG,'-ec',command).decode().strip();owned.append(ident);docker('start',ident);require(docker('wait',ident).strip()==b'0',name+' failed');return ident
    def helper(name,rooted,netns,output):
        args=['create','--pull','never','--name',r.PROJECT+'-remaining-03-'+name,'--label','lifecycle307.remaining='+batch,'--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp','--network','container:'+netns,
              '--mount',f'type=bind,source={private / "admin" if rooted else reg},target=/run/private,readonly','--mount',f'type=bind,source={control}/reader,target=/run/reader,readonly','--mount',f'type=bind,source={output},target=/run/output','--mount',f'type=bind,source={ROOT}/industry-packs,target=/run/repo/industry-packs,readonly','--mount',f'type=bind,source={private}/owner/rules,target=/run/rules,readonly','--entrypoint','/run/reader',r.PG,'-mode','remaining-'+('seal' if rooted else 'business')]
        ident=docker(*args).decode().strip();owned.append(ident);docker('start',ident);code=int(docker('wait',ident));(out/(name+'.stdout.txt')).write_bytes(docker('logs',ident));write(out/(name+'.json'),{'container':ident,'binary_sha256':sha(control/'reader'),'exit_code':code,'delete':0});require(code==0,name+' failed')
    try:
        owned.append(pg);docker('start',pg)
        r.native.wait_pg(pg,'checkpoint_admin')
        require(docker('exec',pg,'psql','-X','-U','checkpoint_admin','-d','postgres','-qAt','-c',"SELECT count(*) FROM pg_roles WHERE rolname='fixture';").strip()==b'0','Clone globals already applied')
        docker('exec',pg,'psql','-X','-U','checkpoint_admin','-d','postgres','-v','ON_ERROR_STOP=1','-f','/run/backup/globals.sql')
        docker('exec',pg,'pg_restore','--exit-on-error','--create','--role=fixture','-U','checkpoint_admin','-d','postgres','/run/backup/core.dump')
        restored=r.native.fingerprints(pg);write(out/'restored-core.json',restored);require(before==restored,'Restored Core rows/roles/ACL differ')
        utility('unpack','exec tar -C /restore -xpf /backup/native.tar .',['--mount','type=volume,source='+prefix+'-objects,target=/restore','--mount',f'type=bind,source={checkpoint},target=/backup,readonly'])
        compare=utility('compare',r'set -o pipefail; cd /source; find . -type f -exec sha256sum {} \; | sort > /tmp/a; cd /restore; find . -type f -exec sha256sum {} \; | sort > /tmp/b; cmp /tmp/a /tmp/b; cat /tmp/a',['--mount','type=volume,source='+r.PROJECT+'-objects,target=/source,readonly','--mount','type=volume,source='+prefix+'-objects,target=/restore,readonly'])
        (out/'native-file-sha256.txt').write_bytes(docker('logs',compare))
        mi=docker('create','--pull','never','--name',prefix+'-minio','--label','lifecycle307.remaining='+batch,'--network',network,'--network-alias','minio','--mount','type=volume,source='+prefix+'-objects,target=/data','--mount',f'type=bind,source={r.PRIVATE / r.PROJECT},target=/run/credentials,readonly','--entrypoint','/bin/sh',r.MI,'/run/credentials/start.sh').decode().strip();owned.append(mi);docker('start',mi)
        gateway=control/'Caddyfile';gateway.write_bytes((prior/'gateway-8.Caddyfile').read_bytes())
        gw=docker('create','--pull','never','--name',prefix+'-gateway','--label','lifecycle307.remaining='+batch,'--network',network,'--mount',f'type=bind,source={gateway},target=/etc/caddy/Caddyfile,readonly',r.GW,'caddy','run','--config','/etc/caddy/Caddyfile').decode().strip();owned.append(gw);docker('start',gw)
        redis=docker('create','--pull','never','--name',prefix+'-redis','--label','lifecycle307.remaining='+batch,'--network',network,'--network-alias',r.PROJECT+'-remaining-redis','--mount','type=volume,source='+prefix+'-redis,target=/data',r.REDIS,'redis-server','--appendonly','yes').decode().strip();owned.append(redis);docker('start',redis);time.sleep(3)
        ids=[inspect(x)['NetworkSettings']['Networks'][network]['NetworkID'] for x in (pg,mi,gw,redis)]
        require(len(set(ids))==1 and all(not any(inspect(x)['NetworkSettings']['Ports'].values()) for x in (pg,mi,gw,redis)),'Physical clone isolation differs')
        write(out/'topology.json',{'core':pg,'minio':mi,'gateway':gw,'redis':redis,'network':ni['Id'],'subnet':'10.234.169.0/24','source_core':inspect(r.PROJECT+'-postgres')['Id'],'source_minio':inspect(r.PROJECT+'-minio')['Id'],'core_full_restore':'PASS','native_file_restore':'PASS','gateway_sha256':sha(gateway),'delete_route':'DENY_ALL'})
        helper('business',False,gw,regout)
        helper('clone-seal',True,mi,regout)
        original=r.PROJECT+'-minio';owned.append(original);docker('start',original);time.sleep(3);helper('source-seal',True,original,out)
        write(out/'checkpoint.json',{'core_dump_sha256':sha(checkpoint/'core.dump'),'globals_sha256':sha(checkpoint/'globals.sql'),'native_archive_sha256':sha(checkpoint/'native.tar'),'delete':0,'full_D1':'NOT_RUN'})
    finally:
        seals=[]
        for ident in dict.fromkeys(reversed(owned)):
            state=inspect(ident)
            if state['State']['Running']:docker('stop','--time','20',ident)
            state=inspect(ident);require(state['State']['Pid']==0 and not any(state['NetworkSettings']['Ports'].values()),'Unsealed resource')
            seals.append({'id':state['Id'],'name':state['Name'],'status':state['State']['Status'],'pid':state['State']['Pid'],'ports':state['NetworkSettings']['Ports']})
        write(out/'seal.json',{'containers':seals,'retained':True,'delete':0,'cleanup':0})

if __name__=='__main__':main()
