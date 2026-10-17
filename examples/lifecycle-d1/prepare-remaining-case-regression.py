"""Restore v16 late/mixed copies and run their native NON-DELETE case entry.

Original three Core/provider environments and their immutable backups are never
modified. No deletion identity is enabled; metadata inspector alone is enabled.
"""
import base64
import importlib.util
import json
import os
from pathlib import Path
import secrets
import subprocess
import time
import uuid
import sys
from urllib.parse import urlsplit,urlunsplit
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives.serialization import Encoding,PublicFormat,PrivateFormat,NoEncryption

ROOT=Path(__file__).resolve().parents[2]
spec=importlib.util.spec_from_file_location('runner',ROOT/'examples/lifecycle-d1/prepare-remaining-runner.py')
r=importlib.util.module_from_spec(spec);spec.loader.exec_module(r)
require,load,write,sha,docker,inspect=r.require,r.load,r.write,r.sha,r.docker,r.inspect

def main():
    only_mixed=sys.argv[1:]==['--mixed-after-retained-late-failure']
    require(not sys.argv[1:] or only_mixed,'Unsupported case invocation')
    require(not r.call(['git','status','--porcelain']).strip(),'Commit source before native case tests')
    generation='03' if only_mixed else '01'
    control=r.PRIVATE/('remaining-case'+generation);require(not control.exists(),'One-shot case regression already allocated');control.mkdir()
    out=ROOT/('docs/acceptance/evidence/issue307-d1/remaining-case'+generation);out.mkdir();source=r.call(['git','rev-parse','HEAD']).decode().strip()
    env=os.environ.copy();env.update(GOOS='linux',GOARCH='amd64',CGO_ENABLED='0')
    result=subprocess.run(['go','build','-trimpath','-buildvcs=false','-o',str(control/'reader'),'./cmd/lifecycle-fixture-reader'],cwd=ROOT/'apps/platform',env=env,capture_output=True)
    require(result.returncode==0,'Case runner build failed');write(out/'build.json',{'source':source,'binary_sha256':sha(control/'reader'),'delete':0})
    for group,run,subnet in [('late-hold','754ea8f0','10.234.170.0/24'),('mixed','dfda43b4','10.234.171.0/24')]:
        if only_mixed and group!='mixed':continue
        r.check_subnet(subnet);project='dpp-lifecycle307-'+run;prefix=project+'-remaining-regression';private=r.PRIVATE/project;original=load(private/'config.json');checkpoint=private/'checkpoint';batch=str(uuid.uuid4());owned=[]
        target=out/group;target.mkdir();business=target/'business';business.mkdir();cfgdir=control/group;cfgdir.mkdir();admin=cfgdir/'admin';owner=cfgdir/'owner';admin.mkdir();owner.mkdir()
        manifest=ROOT/'docs/acceptance/evidence/issue307-d1/remaining-native01'/group/'manifest.json'
        for path in (admin,owner):(path/'manifest.json').write_bytes(manifest.read_bytes())
        dsn=urlsplit(original['PostgresDSN']);require(dsn.hostname==project+'-postgres' and dsn.path=='/lifecycle307' and dsn.username=='fixture' and dsn.port==5432,'Original DSN identity differs')
        clone_dsn=urlunsplit(dsn._replace(netloc=dsn.netloc.rsplit('@',1)[0]+'@'+prefix+'-pg:5432'));require(urlsplit(clone_dsn).hostname!=dsn.hostname,'Clone DSN unchanged')
        now=r.utc();base={k:original[k] for k in ('RunID','WorkspaceID','ProviderInstanceID','IncarnationID','ActorID','Endpoint','InspectAccess','InspectSecret')}
        base.update(Enabled=True,Epoch=str(uuid.uuid4()),PostgresDSN=clone_dsn,Token=secrets.token_hex(32),Access=base['InspectAccess'],Secret=base['InspectSecret'],ManifestSHA256=sha(manifest),OutputDirectory='/run/output',StartedAt=now.isoformat(),ExpiresAt=(now+r.dt.timedelta(minutes=55)).isoformat())
        c={'Base':base,'Group':group,'Batch':batch,'NonDelete':True,'Rehearsal':True};write(owner/'remaining.json',c)
        ac=json.loads(json.dumps(c));ac['Base'].update(RootAccess=original['RootAccess'],RootSecret=original['RootSecret']);write(admin/'remaining.json',ac)
        gateway=cfgdir/'Caddyfile';gateway.write_text("{\n admin off\n auto_https off\n}\n:9000 {\n route {\n @metadata {\n method GET HEAD\n expression {http.request.header.Authorization}.contains('Credential="+base['InspectAccess']+"/')\n }\n reverse_proxy @metadata minio:9000\n respond 403\n }\n}\n")
        network=prefix+'-private';docker('network','create','--internal','--subnet',subnet,'--label','lifecycle307.remaining='+batch,network)
        for suffix in ('core','objects'):docker('volume','create','--label','lifecycle307.remaining='+batch,prefix+'-'+suffix)
        def helper(name,mode,rooted=False,output=None,private_path=None):
            ident=docker('create','--pull','never','--name',prefix+'-'+name,'--label','lifecycle307.remaining='+batch,'--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp','--network','container:'+(mi if rooted else gw),'--mount',f'type=bind,source={private_path or (admin if rooted else owner)},target=/run/private,readonly','--mount',f'type=bind,source={control}/reader,target=/run/reader,readonly','--mount',f'type=bind,source={output or business},target=/run/output','--entrypoint','/run/reader',r.PG,'-mode',mode).decode().strip();owned.append(ident);docker('start',ident);code=int(docker('wait',ident));(target/(name+'.stdout.txt')).write_bytes(docker('logs',ident));write(target/(name+'.json'),{'container':ident,'mode':mode,'exit_code':code,'binary_sha256':sha(control/'reader'),'delete':0});require(code==0,name+' failed');return ident
        try:
            pg=docker('create','--pull','never','--name',prefix+'-pg','--label','lifecycle307.remaining='+batch,'--network',network,'--mount','type=volume,source='+prefix+'-core,target=/var/lib/postgresql/data','--mount',f'type=bind,source={checkpoint},target=/run/backup,readonly','-e','POSTGRES_USER=checkpoint_admin','-e','POSTGRES_DB=postgres','-e','POSTGRES_PASSWORD_FILE=/run/backup/password',r.PG).decode().strip();owned.append(pg);docker('start',pg)
            # pg_isready can see initdb's temporary server. Wait for initdb to
            # finish before the first mutating restore command, then probe.
            ready=False
            for _ in range(60):
                logs=docker('logs',pg).decode(errors='replace')
                if 'PostgreSQL init process complete; ready for start up.' in logs:ready=True;break
                time.sleep(.5)
            require(ready,'PG init completion marker unavailable');time.sleep(1);r.native.wait_pg(pg,'checkpoint_admin')
            docker('exec',pg,'psql','-X','-U','checkpoint_admin','-d','postgres','-v','ON_ERROR_STOP=1','-f','/run/backup/globals.sql')
            docker('exec',pg,'pg_restore','--exit-on-error','--create','--role=fixture','-U','checkpoint_admin','-d','postgres','/run/backup/core.dump')
            fingerprints=r.native.fingerprints(pg);expected=load(ROOT/'docs/acceptance/evidence/issue307-d1/remaining-native01'/group/'source-before.json');require(fingerprints==expected,'Full clone Core rows/roles/ACL differ');write(target/'restored-core.json',fingerprints)
            unpack=docker('create','--pull','never','--name',prefix+'-unpack','--label','lifecycle307.remaining='+batch,'--network','none','--read-only','--mount','type=volume,source='+prefix+'-objects,target=/restore','--mount',f'type=bind,source={checkpoint},target=/backup,readonly','--entrypoint','/bin/sh',r.PG,'-ec','exec tar -C /restore -xpf /backup/native.tar .').decode().strip();owned.append(unpack);docker('start',unpack);require(docker('wait',unpack).strip()==b'0','Native unpack failed')
            mi=docker('create','--pull','never','--name',prefix+'-minio','--label','lifecycle307.remaining='+batch,'--network',network,'--network-alias','minio','--mount','type=volume,source='+prefix+'-objects,target=/data','--mount',f'type=bind,source={private},target=/run/credentials,readonly','--entrypoint','/bin/sh',r.MI,'/run/credentials/start.sh').decode().strip();owned.append(mi);docker('start',mi)
            gw=docker('create','--pull','never','--name',prefix+'-gateway','--label','lifecycle307.remaining='+batch,'--network',network,'--mount',f'type=bind,source={gateway},target=/etc/caddy/Caddyfile,readonly',r.GW,'caddy','run','--config','/etc/caddy/Caddyfile').decode().strip();owned.append(gw);docker('start',gw);time.sleep(3)
            helper('inspect-enable','remaining-inspection-enable',True)
            case=helper('cases','remaining-late' if group=='late-hold' else 'remaining-mixed')
            helper('inspect-disable','remaining-inspection-disable',True)
            # Closed H1 can be proven without guessing from a deadline. The
            # closed command has no deleter; its exact binary/config/native IAM
            # and deny-all gateway are attested by the trusted host owner.
            if group=='late-hold':
                c['Base']['Enabled']=False;(owner/'remaining.json').write_text(json.dumps(c)+'\n')
                require(inspect(case)['State']['Pid']==0 and inspect(case)['State']['ExitCode']==0 and inspect(case)['HostConfig']['RestartPolicy']['Name']=='no','Case owner not closed')
                key=Ed25519PrivateKey.generate();(cfgdir/'closure-key.private').write_bytes(key.private_bytes(Encoding.Raw,PrivateFormat.Raw,NoEncryption()))
                owners={case:sha(control/'reader')};binding=sha(business/'remaining-H1.json')
                body={'Execution':batch,'Nonce':str(uuid.uuid4()),'CandidateSHA256':binding,'Group':group,'At':r.utc().isoformat(),'Owners':owners,'StoppedPIDZero':True,'NoRestart':True,'ConfigDisabled':True,'DeleteIdentityDisabled':True,'GatewaySealed':True}
                encoded=json.dumps(body,separators=(',',':')).encode();proof={'Body':base64.b64encode(encoded).decode(),'Signature':base64.b64encode(key.sign(encoded)).decode()}
                close=cfgdir/'close';close.mkdir();cc=json.loads(json.dumps(c));cc['Base']['Enabled']=True;cc.update(ClosurePublicKey=key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw).hex(),ClosureCandidateSHA256=binding,ClosureOwners=owners)
                write(close/'remaining.json',cc);(close/'manifest.json').write_bytes(manifest.read_bytes());write(close/'closed-H1.json',proof)
                helper('close-H1','remaining-close-H1',private_path=close)
            write(target/'final-core.json',r.native.fingerprints(pg))
            write(target/'topology.json',{'batch':batch,'core':pg,'native':mi,'gateway':gw,'network':json.loads(docker('network','inspect',network))[0]['Id'],'subnet':subnet,'source_core':inspect(project+'-postgres')['Id'],'source_native':inspect(project+'-minio')['Id'],'native_delete':0,'source_core_mutations':0})
        finally:
            seals=[]
            for ident in dict.fromkeys(reversed(owned)):
                state=inspect(ident)
                if state['State']['Running']:docker('stop','--time','20',ident)
                state=inspect(ident);require(state['State']['Pid']==0 and not any(state['NetworkSettings']['Ports'].values()),'Resource not sealed');seals.append({'id':state['Id'],'name':state['Name'],'status':state['State']['Status'],'pid':state['State']['Pid'],'ports':state['NetworkSettings']['Ports']})
            write(target/'seal.json',{'containers':seals,'retained':True,'native_delete':0,'cleanup':0})

if __name__=='__main__':main()
