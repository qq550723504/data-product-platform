"""One-shot isolated Docker/PG host composition, with no deletion authority.

Uses real inspect/mount/volume/read-only SQL and the actual attester main. Test
identities and ephemeral keys are explicit fixture context, not a candidate or
human authorization. Source CIDs, volumes and credentials are never mounted.
All allocated resources and failures are retained, stopped, with no rm/prune.
"""
import base64
from contextlib import ExitStack, redirect_stdout
import copy
import datetime as dt
import importlib.util
import io
import ipaddress
import json
import os
from pathlib import Path
import subprocess
import sys
import time
import uuid
from unittest.mock import patch

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives.serialization import Encoding, PublicFormat, PrivateFormat, NoEncryption

ROOT = Path(__file__).resolve().parents[2]
CONTROL = Path('C:/Users/Henry/.codex/fixture-private/host-path-repair04')
OUTPUT = ROOT/'docs/acceptance/evidence/issue307-d1/host-path-repair01/isolated04'
spec = importlib.util.spec_from_file_location('actual_attester', Path(__file__).with_name('attest-remaining-executable.py'))
m = importlib.util.module_from_spec(spec); spec.loader.exec_module(m)
owners, networks, volumes = [], [], []

def call(args, data=None):
    r = subprocess.run(list(map(str,args)),input=data,capture_output=True,cwd=ROOT,timeout=120)
    if r.returncode:
        if CONTROL.exists():
            (CONTROL/('failure-'+str(time.time_ns())+'.private')).write_bytes(r.stdout+b'\n'+r.stderr)
        raise RuntimeError('isolated command failed; original diagnostics retained')
    if not isinstance(r.stdout,bytes): raise TypeError('process output is not bytes')
    return r.stdout

def docker(*args): return call(['docker',*args])
def text(*args): return docker(*args).decode('utf-8',errors='strict').strip()
def inspect(cid): return json.loads(docker('inspect',cid))[0]
def dump(path,value):
    path.parent.mkdir(parents=True,exist_ok=True)
    path.write_text(json.dumps(value,indent=2)+'\n',encoding='utf-8',newline='\n')
def create(*args):
    cid=text('create','--pull','never','--restart','no','--label','lifecycle307.host-path-rehearsal=isolated01',*args)
    owners.append(cid); dump(CONTROL/'allocations.json',dict(owners=owners,networks=networks,volumes=volumes)); return cid
def sql(cid,query):
    # The attester's real query runs read-only, never a host receipt projection.
    return docker('exec',cid,'psql','-h','127.0.0.1','-X','-U','fixture','-d','lifecycle307','-At','-v','ON_ERROR_STOP=1','-c','BEGIN READ ONLY; '+query+' COMMIT;').decode().removeprefix('BEGIN\n').removesuffix('COMMIT\n').strip()
def exec_sql(cid,query): return docker('exec',cid,'psql','-h','127.0.0.1','-X','-U','fixture','-d','lifecycle307','-v','ON_ERROR_STOP=1','-c',query)
def wait(cid):
    for _ in range(40):
        if subprocess.run(['docker','exec',cid,'pg_isready','-h','127.0.0.1','-U','fixture','-d','lifecycle307'],capture_output=True).returncode==0:return
        time.sleep(.25)
    raise RuntimeError('isolated PostgreSQL readiness failed')

def unused_subnets():
    used=[]
    ids=text('network','ls','-q').split()
    for network in json.loads(docker('network','inspect',*ids)):
        used.extend(ipaddress.ip_network(x['Subnet']) for x in network.get('IPAM',{}).get('Config') or [] if x.get('Subnet'))
    selected=[]
    for subnet in ipaddress.ip_network('10.245.240.0/20').subnets(new_prefix=28):
        if not any(subnet.version==x.version and subnet.overlaps(x) for x in used):selected.append(str(subnet))
        if len(selected)==3:return selected
    raise ValueError('no disjoint isolated subnet available; preserve every retained network')

def main():
    if CONTROL.exists() or OUTPUT.exists(): raise ValueError('one-shot rehearsal already allocated')
    CONTROL.mkdir(); OUTPUT.mkdir(parents=True)
    private,public=CONTROL/'private',CONTROL/'packet';private.mkdir();public.mkdir()
    env=os.environ.copy();env.update(GOOS='linux',GOARCH='amd64',CGO_ENABLED='0')
    for name,pkg in [('operator','./cmd/lifecycle-fixture-remaining'),('pilot.test','./internal/lifecycle/pilot')]:
        args=['go','build','-trimpath','-buildvcs=false','-o',str(private/name),pkg] if name=='operator' else ['go','test','-c','-o',str(private/name),pkg]
        result=subprocess.run(args,cwd=ROOT/'apps/platform',env=env,capture_output=True)
        (CONTROL/(name+'.build.private')).write_bytes(result.stdout+result.stderr)
        if result.returncode: raise RuntimeError('isolated binary build failed')
    candidate=copy.deepcopy(m.r.load(m.f.PUBLIC/'candidate.json'))
    at=dt.datetime.now(dt.timezone.utc);start=(at-dt.timedelta(minutes=1)).isoformat();end=(at+dt.timedelta(minutes=30)).isoformat()
    execution=str(uuid.uuid4());generation='host-path-rehearsal04'
    candidate.update(Execution=execution,Profile='ISOLATED_NONDELETE_NO_AUTHORITY',NotBefore=start,NotAfter=end,SenderBinarySHA256=m.r.sha(private/'operator'))
    key=Ed25519PrivateKey.generate();candidate['PublicKey']=key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw).hex()
    (private/'topology-key.private').write_bytes(key.private_bytes(Encoding.Raw,PrivateFormat.Raw,NoEncryption()))
    bindings=[]; budget_initializers=[]; facts=[]; failure=None
    try:
        subnets=unused_subnets()
        for index,g in enumerate(candidate['Groups']):
            name=g['Group'];folder=private/name;folder.mkdir()
            net=text('network','create','--internal','--subnet',subnets[index],'--label','lifecycle307.host-path-rehearsal=isolated01','dpp307-path04-'+name)
            networks.append(net);g['Network']=net
            pgvol=text('volume','create','--label','lifecycle307.host-path-rehearsal=isolated01','dpp307-path04-'+name+'-pg');volumes.append(pgvol)
            core=create('--network',net,'-e','POSTGRES_USER=fixture','-e','POSTGRES_DB=lifecycle307','-e','POSTGRES_HOST_AUTH_METHOD=trust','--mount','type=volume,source='+pgvol+',target=/var/lib/postgresql/data',m.r.PG)
            docker('start',core);wait(core);g['Core']=core
            exec_sql(core,'CREATE TABLE lifecycle_copy_absence_receipt(workspace_id uuid,copy_id uuid);')
            p1=[c for c in g['Copies'] if c['Key']=='fixture/p1.csv']
            if p1:exec_sql(core,"INSERT INTO lifecycle_copy_absence_receipt VALUES('"+p1[0]['WorkspaceID']+"','"+p1[0]['ID']+"');")
            nativevol=text('volume','create','--label','lifecycle307.host-path-rehearsal=isolated01','dpp307-path04-'+name+'-native');volumes.append(nativevol)
            native=create('--network',net,'-e','MINIO_ROOT_USER=isolated-fixture','-e','MINIO_ROOT_PASSWORD=not-a-source-or-delete-credential','--mount','type=volume,source='+nativevol+',target=/data',m.r.MI,'server','/data')
            docker('start',native);g['Native']=native
            gateway=folder/'gateway.Caddyfile';gateway.write_text('{\n admin off\n auto_https off\n}\n:9000 {\n respond 403\n}\n',encoding='utf-8',newline='\n')
            gw=create('--network',net,'--mount','type=bind,source='+str(gateway)+',target=/etc/caddy/Caddyfile,readonly',m.r.GW)
            docker('start',gw);g['Gateway']=gw;g['GatewaySHA256']=m.r.sha(gateway)
            run=next(x['Run'][:8] for x in m.r.load(m.f.ROOT/'apps/platform/internal/lifecycle/pilot/remaining_scope.json') if x['Group']==name)
            volume=text('volume','create','--label','lifecycle307.remaining='+execution,'dpp-lifecycle307-'+run+'-'+generation+'-budget');volumes.append(volume)
            script=folder/'budget-init.sh';script.write_bytes(b'#!/bin/sh\nchown 10001:10001 /run/budget\nchmod 700 /run/budget\n')
            initializer=create('--network','none','--user','0:0','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data:size=1m','--mount','type=volume,source='+volume+',target=/run/budget','--mount','type=bind,source='+str(script)+',target=/run/budget-init.sh,readonly','--entrypoint','/bin/sh',m.r.PG,'-e','/run/budget-init.sh')
            docker('start',initializer)
            if text('wait',initializer)!='0':raise RuntimeError('budget initialization failed')
            bindings.append(dict(group=name,budget_initializer=initializer,budget_initializer_script_sha256=m.r.sha(script)))
            budget_initializers.append((g,volume,initializer))
            for stage in g['Helpers']:
                owner=folder/stage;owner.mkdir();output=folder/'outputs'/stage;output.mkdir(parents=True)
                dump(owner/'operator.json',dict(Group=name,DeleteSecret='',WirePrivateKey=''))
                g['ConfigSHA256'][stage]=m.r.sha(owner/'operator.json');dump(owner/'permission.json',dict(Enabled=False))
                if stage.startswith('recover-'):dump(owner/'recovery.json',dict(Enabled=False))
                g['Helpers'][stage]=create('--network','container:'+gw,'--user','10001:10001','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data:size=1m','--mount','type=bind,source='+str(owner)+',target=/run/private,readonly','--mount','type=bind,source='+str(private/'operator')+',target=/run/operator,readonly','--mount','type=bind,source='+str(output)+',target=/run/output','--mount','type=volume,source='+volume+',target=/run/budget','--entrypoint','/run/operator',m.r.PG,'-stage',stage)
        dump(public/'candidate.json',candidate);dump(public/'bindings.json',bindings);dump(public/'status.json',dict(candidate_sha256=m.r.sha(public/'candidate.json')))
        template=b'ISOLATED_REHEARSAL_NOT_HUMAN_AUTHORIZATION\n';(public/'confirmation-template.txt').write_bytes(template)
        for g in candidate['Groups']:
            for stage in g['Helpers']:
                owner=private/g['Group']/stage
                (owner/'candidate.json').write_bytes((public/'candidate.json').read_bytes());(owner/'confirmation-template.txt').write_bytes(template)
        # Real PostgreSQL adapter tests, no source database/credentials present.
        test=create('--network','container:'+candidate['Groups'][0]['Core'],'--mount','type=bind,source='+str(private/'pilot.test')+',target=/run/test,readonly','-e','TEST_POSTGRES_DSN=postgres://fixture@127.0.0.1:5432/lifecycle307?sslmode=disable','--entrypoint','/run/test',m.r.PG,'-test.v','-test.run','TestPostV(22|27).*Postgres')
        docker('start',test);code=text('wait',test);raw=docker('logs',test);(OUTPUT/'postgres-contract.log').write_bytes(raw)
        if code!='0':raise RuntimeError('real PostgreSQL contract failed')
        facts.append(dict(phase='actual_PostgreSQL_adapter',status='PASS',test_binary_sha256=m.r.sha(private/'pilot.test')))
        with ExitStack() as stack:
            for obj,name,value in [(m.f,'PRIVATE',private),(m.f,'PUBLIC',public),(m.f,'EXECUTION',execution),(m.f,'PROFILE',candidate['Profile']),(m.f,'START',start),(m.f,'END',end),(m.f,'GENERATION',generation),(m.r,'inspect',inspect),(m.r,'docker',docker),(m.r,'docker_text',text),(m.r.native,'sql',sql)]:stack.enter_context(patch.object(obj,name,value))
            def attest(stage,expected=None):
                owner=private/'business-races'/stage;proof=owner/('topology-'+stage+'.json')
                permission=dict(Enabled=not stage.startswith('recover-'),Execution=execution,Profile=candidate['Profile'],CandidateSHA256=m.r.sha(public/'candidate.json'),ConfirmationSHA256=m.r.sha(public/'confirmation-template.txt'),BinarySHA256=candidate['SenderBinarySHA256'],NotBefore=start,ExpiresAt=end,ConfirmedAt=at.isoformat(),MaxPerTarget=1,MaxTotal=6)
                dump(owner/'permission.json',permission);(owner/'human-confirmation.txt').write_bytes(template)
                if stage.startswith('recover-'):dump(owner/'recovery.json',dict(Enabled=True))
                with patch.object(sys,'argv',['attest','--group','business-races','--stage',stage]),redirect_stdout(io.StringIO()):
                    if expected:
                        try:m.main()
                        except ValueError as e:
                            if expected not in str(e):raise
                            facts.append(dict(phase='actual_Docker_reject',stage=stage,reason=str(e)));return
                        raise ValueError('counterexample signed')
                    m.main()
                signed=json.loads(proof.read_bytes());key.public_key().verify(base64.b64decode(signed['Signature']),base64.b64decode(signed['Body']))
                facts.append(dict(phase='actual_Docker_sign_verify',stage=stage,body=json.loads(base64.b64decode(signed['Body'])),proof_sha256=m.r.sha(proof)))
                permission['Enabled']=False;dump(owner/'permission.json',permission)
            for stage in ('wire','P2','recover-P2'):attest(stage)
            # Actual host directory expansion, preserving the rejected file.
            owner=private/'business-races'/'T1';(owner/'root.json').write_bytes(b'NO_REAL_SECRET')
            attest('T1','unexpected private file')
            # Slot 0 must remain the only send slot; no lookup/absence fiction.
            owner=private/'business-races'/'P2';dump(owner/'permission.json',dict(Enabled=False))
            # Run the real operator with no authority; its own contract rejects
            # before any Core/provider work. Gateway also denies every request.
            cid=candidate['Groups'][0]['Helpers']['P2'];docker('start',cid);code=text('wait',cid)
            logs=docker('logs',cid);(OUTPUT/'operator-rejected.log').write_bytes(logs)
            if code=='0':raise ValueError('unapproved isolated operator unexpectedly succeeded')
            facts.append(dict(phase='actual_operator_pre_provider_rejection',exit=code,source_access=False,provider_delete=0))
    except Exception as e:
        failure=type(e).__name__+': '+str(e)
    finally:
        closure=[]
        for cid in reversed(owners):
            item=dict(cid=cid)
            try:docker('stop','-t','5',cid)
            except Exception:item['stop_error']=True
            try:
                x=inspect(cid);item['stopped_PID0']=not x['State']['Running'] and x['State']['Pid']==0;item['noports']=not x['HostConfig'].get('PortBindings') and not any(x['NetworkSettings']['Ports'].values());item['norestart']=x['HostConfig']['RestartPolicy']['Name']=='no'
            except Exception:item['stopped_PID0']=False
            closure.append(item)
        status='PASS_ISOLATED_HOST_COMPOSITION_NO_DELETE' if not failure and all(x.get('stopped_PID0') and x.get('noports') and x.get('norestart') for x in closure) else 'FAILED_OR_UNCONFIRMED_RETAINED'
        result=dict(status=status,failure=failure,execution=execution,test_context_only=True,source_resources_started=0,real_delete=0,full_source_native_preflight='NOT_RUN',facts=facts,closure=closure,retained_volumes=volumes,retained_networks=networks)
        dump(OUTPUT/'terminal.json',result);dump(CONTROL/'terminal.json',result)
    print(json.dumps(dict(status=status,failure=failure,owners=len(owners),native_delete=0)))
    return status=='PASS_ISOLATED_HOST_COMPOSITION_NO_DELETE'

if __name__=='__main__':raise SystemExit(0 if main() else 1)
