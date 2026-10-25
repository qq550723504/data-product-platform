"""Actual-schema isolated observer/Command/replay regression, no provider.

Only the original Core is briefly opened for a read-only consistent dump and
fingerprints, then stopped. Restore uses a separate internal network/volume.
The ephemeral proof is a test oracle, never a production host attestation or
human deletion authority. It is only mounted to isolated maintenance observers.
"""
import base64
import datetime as dt
import importlib.util
import json
import os
from pathlib import Path
import secrets
import subprocess
import time
import uuid

ROOT=Path(__file__).resolve().parents[2]
spec=importlib.util.spec_from_file_location('v27_maintenance',Path(__file__).with_name('close-v27-business-dispatches.py'))
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
CONTROL=m.PRIVATE/'v27-restored-core-regression02'
PUBLIC=ROOT/'docs/acceptance/evidence/issue307-d1/host-path-repair01/restored-core02'
docker,load,sha,need,write,inspect,closed=m.docker,m.load,m.sha,m.need,m.write,m.inspect,m.closed

def main():
    need(not CONTROL.exists() and not PUBLIC.exists(),'one-shot restored rehearsal already allocated')
    CONTROL.mkdir();PUBLIC.mkdir(parents=True);owners=[];failure=None;result=dict(native_delete=0,source_Native_started=0,provider_calls=0,test_oracle_not_host_authority=True)
    candidate,_=m.packet();g=candidate['Groups'][0];source=g['Core'];native=m.p.native_module();retained=m.retained()
    def create(*args):
        cid=m.p.text('create','--pull','never','--restart','no','--label','lifecycle307.restored-v27=regression01',*args);owners.append(cid);write(CONTROL/('allocation-'+str(len(owners))+'.json'),dict(cid=cid));return cid
    try:
        need(closed(inspect(source)),'source not closed')
        # Reuse the already captured consistent dump; do not reopen a source to
        # repair an isolated test-only marker. Original run01 failure stays.
        prior=ROOT/'docs/acceptance/evidence/issue307-d1/host-path-repair01/restored-core01'
        before=load(prior/'source-readonly-before.json');after=load(prior/'source-readonly-after.json');need(before==after,'original read-only snapshot drift')
        raw=(m.PRIVATE/'v27-restored-core-regression01/source-Core.dump').read_bytes()
        with (CONTROL/'source-Core.dump').open('xb') as f:f.write(raw)
        write(PUBLIC/'source-snapshot-reference.json',dict(reused_from='restored-core01',fresh_source_probe=False,source_started=0,before_sha256=sha(prior/'source-readonly-before.json'),after_sha256=sha(prior/'source-readonly-after.json')))
        allocation=load(m.PRIVATE/'host-path-repair04/allocations.json');network=allocation['networks'][0]
        need(json.loads(docker('network','inspect',network))[0]['Internal'] and network!=g['Network'],'restore network aliases source')
        volume=m.p.text('volume','create','--label','lifecycle307.restored-v27=regression02','dpp307-v27-restored-core02')
        core=create('--network',network,'--network-alias','dpp-lifecycle307-808a6960-postgres','-e','POSTGRES_USER=fixture','-e','POSTGRES_DB=lifecycle307','-e','POSTGRES_HOST_AUTH_METHOD=trust','--mount','type=volume,source='+volume+',target=/var/lib/postgresql/data',m.p.PG)
        docker('start',core)
        for _ in range(40):
            if subprocess.run(['docker','exec',core,'pg_isready','-h','127.0.0.1','-U','fixture','-d','lifecycle307'],capture_output=True).returncode==0:break
            time.sleep(.25)
        m.p.call(['docker','exec','-i',core,'pg_restore','-h','127.0.0.1','-U','fixture','-d','lifecycle307','--exit-on-error','--no-owner','--no-acl'],data=raw)
        docker('exec',core,'psql','-h','127.0.0.1','-U','fixture','-d','lifecycle307','-v','ON_ERROR_STOP=1','-c',"ALTER DATABASE lifecycle307 SET lifecycle.rehearsal_marker='V27_PARTIAL_BASELINE_ISOLATED_NO_AUTHORITY';")
        env=os.environ.copy();env.update(GOOS='linux',GOARCH='amd64',CGO_ENABLED='0')
        for name,args in [('observer',['go','build','-trimpath','-buildvcs=false','-o',str(CONTROL/'observer'),'./cmd/lifecycle-fixture-old-dispatch']),('pilot.test',['go','test','-c','-o',str(CONTROL/'pilot.test'),'./internal/lifecycle/pilot'])]:
            build=subprocess.run(args,cwd=ROOT/'apps/platform',env=env,capture_output=True);(CONTROL/(name+'.build.private')).write_bytes(build.stdout+build.stderr);need(build.returncode==0,'rehearsal build failed')
        test=create('--network','container:'+core,'--mount','type=bind,source='+str(CONTROL/'pilot.test')+',target=/run/test,readonly','-e','TEST_POSTGRES_DSN=postgres://fixture@127.0.0.1:5432/lifecycle307?sslmode=disable','-e','TEST_POST_V27_RESTORED_SOURCE=1','--entrypoint','/run/test',m.p.PG,'-test.v','-test.run','TestPostV(22|27).*Postgres|TestPostV27RestoredCorePublicSchema')
        docker('start',test);code=m.p.text('wait',test);(PUBLIC/'canonical-schema-and-modeled-contract.log').write_bytes(docker('logs',test));need(code=='0','restored canonical schema contract failed')
        m.p.ALLOWED_RELATIONS=m.c.ALLOWED;baseline=native.fingerprints(core);anchors=m.p.append_anchors(native,core)
        write(PUBLIC/'clone-baseline.json',baseline);write(PUBLIC/'clone-baseline-rows.json',m.current_rows(native,core))
        from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
        from cryptography.hazmat.primitives.serialization import Encoding,PublicFormat
        key=Ed25519PrivateKey.generate();audit=str(uuid.uuid4());at=m.now();folder=CONTROL/'worker';folder.mkdir()
        cfg=dict(Enabled=True,AuditID=audit,WorkspaceID=g['Copies'][0]['WorkspaceID'],ActorID=g['Actor'],Purpose='POST_V27_BUSINESS_NOT_SENT_CLOSURE',PostgresDSN='postgres://fixture:ISOLATED_TEST_ONLY@dpp-lifecycle307-808a6960-postgres:5432/lifecycle307?sslmode=disable',Core=source,BinarySHA256=sha(CONTROL/'observer'),PublicKey=key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw).hex(),EvidenceSHA256=sha(PUBLIC/'clone-baseline.json'),Token=secrets.token_hex(32),Owners={'a'*64:sha(CONTROL/'observer')},NotBefore=at.isoformat(),ExpiresAt=(at+dt.timedelta(minutes=30)).isoformat(),MaxProviderDeletes=0)
        write(folder/'closure.json',cfg)
        body=dict(AuditID=audit,Nonce=str(uuid.uuid4()),At=at.isoformat(),Execution=m.EXECUTION,CandidateSHA256=m.CANDIDATE,BindingsSHA256=m.BINDINGS,Core=source,EvidenceSHA256=cfg['EvidenceSHA256'],Targets=m.targets(),Owners=cfg['Owners'],StoppedPIDZero=True,NoRestart=True,ConfigDisabled=True,DeleteIdentityDisabled=True,GatewaySealed=True)
        encoded=json.dumps(body,separators=(',',':')).encode();write(folder/'proof.json',dict(Body=base64.b64encode(encoded).decode(),Signature=base64.b64encode(key.sign(encoded)).decode()))
        for phase in ('first-command','same-key-replay'):
            output=CONTROL/phase;output.mkdir()
            observer=create('--network','container:'+core,'--user','10001:10001','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data:size=1m','--mount','type=bind,source='+str(folder)+',target=/run/private,readonly','--mount','type=bind,source='+str(CONTROL/'observer')+',target=/run/observer,readonly','--mount','type=bind,source='+str(output)+',target=/run/output','--entrypoint','/run/observer',m.p.PG,'post-v27-close')
            docker('start',observer);code=m.p.text('wait',observer);(PUBLIC/(phase+'.log')).write_bytes(docker('logs',observer))
            observed=load(output/'result.json');write(PUBLIC/(phase+'.json'),observed);need(code=='0' and observed['completed'] and len(observed['cases'])==3,'actual restored observer/Command failed')
            fp=native.fingerprints(core);delta=m.c.delta(baseline,fp,anchors,m.p.append_anchors(native,core),complete=False)
            expected=dict(lifecycle_approval_revocation=2,lifecycle_provider_observation=3,lifecycle_dispatch_release=3,command_idempotency=5,audit_event=8,evidence=8,evidence_relation=8,outbox_event=8)
            need(delta['new_row_counts']==expected,'actual restored complete Command fact counts differ')
            write(PUBLIC/(phase+'-delta.json'),delta)
            if phase=='same-key-replay':need(all(x['NOT_SENT']['Replayed'] is True for x in observed['cases']),'same-key replay was not idempotent')
        write(PUBLIC/'clone-final-rows.json',m.current_rows(native,core))
        result.update(status='PASS_ACTUAL_RESTORED_SCHEMA_COMMAND_AND_REPLAY',clone_core=core,clone_network=network,retained_volume=volume,private_dump_sha256=sha(CONTROL/'source-Core.dump'),source_Core_readonly_fingerprint_equal=True,observer_binary_sha256=sha(CONTROL/'observer'),new_revocations=2,new_NOT_SENT=3,new_releases=3)
    except Exception as e:failure=type(e).__name__+': '+str(e)
    finally:
        closure=[]
        for cid in reversed(owners):
            try:docker('stop','-t','10',cid)
            except Exception:pass
            try:closure.append(dict(cid=cid,stopped_PID0=closed(inspect(cid))))
            except Exception:closure.append(dict(cid=cid,stopped_PID0=False))
        try:unchanged=m.retained()==retained
        except Exception:unchanged=False
        result.update(failure=failure,closure=closure,source_retained_unchanged=unchanged)
        if failure or not unchanged or not all(x['stopped_PID0'] for x in closure):result['status']='FAILED_OR_UNCONFIRMED_NO_REPLAY_KEEP_FACTS'
        write(PUBLIC/'terminal.json',result);write(CONTROL/'terminal.json',result)
    print(json.dumps(result));return result['status']=='PASS_ACTUAL_RESTORED_SCHEMA_COMMAND_AND_REPLAY'

if __name__=='__main__':raise SystemExit(0 if main() else 1)
