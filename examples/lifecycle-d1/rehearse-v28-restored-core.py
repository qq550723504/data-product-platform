"""Actual source schema restored to a new PG; test HEAD oracle, zero DELETE.

The source is only used for a consistent read-only dump/fingerprints, never for
Commands. Test oracle signature/HTTP response is not native or host evidence.
"""
import base64,datetime as dt,json,os,secrets,subprocess,time,uuid
from pathlib import Path
import importlib.util
ROOT=Path(__file__).resolve().parents[2]
s=importlib.util.spec_from_file_location('v28_rehearsal_tools',Path(__file__).with_name('close-v28-business-dispatches.py'));m=importlib.util.module_from_spec(s);s.loader.exec_module(m)
CONTROL=m.PRIVATE/'post-v28-restored-core03'
PUBLIC=m.EVIDENCE/'post-v28-restored-core03'

def main():
 m.need(not CONTROL.exists() and not PUBLIC.exists(),'one shot already exists')
 CONTROL.mkdir();PUBLIC.mkdir();m.p.CONTROL=CONTROL;candidate,_=m.packet();g=candidate['Groups'][0];owners=[];failure=None;result=dict(native_delete=0,source_Native_started=False,source_business_mutations=0,HEAD_oracle='TEST_HTTP_ONLY_NOT_NATIVE_ACCEPTANCE')
 def create(*args):
  cid=m.p.text('create','--pull','never','--restart','no','--label','lifecycle307.v28-rehearsal=01',*args);owners.append(cid);m.write(CONTROL/('allocation-'+str(len(owners))+'.json'),dict(cid=cid));return cid
 try:
  m.need(m.closed(m.inspect(g['Core'])),'source Core active')
  try:
   m.docker('start',g['Core']);m.n.wait_pg(g['Core'],'fixture');before=m.n.fingerprints(g['Core']);m.write(PUBLIC/'source-before.json',before)
   raw=m.docker('exec',g['Core'],'pg_dump','-Fc','-U','fixture','-d','lifecycle307','--serializable-deferrable');(CONTROL/'source-Core.dump').write_bytes(raw)
   after=m.n.fingerprints(g['Core']);m.write(PUBLIC/'source-after.json',after);m.need(before==after==m.load(m.EVIDENCE/'post-v28-core-audit01/Core-after.json'),'source dump changed Core')
  finally:m.docker('stop','-t','15',g['Core'])
  network=m.load(m.OLD/'allocation.json')['groups'][0]['restore_network'];network_state=json.loads(m.docker('network','inspect',network))[0];m.need(network_state['Internal'] and not network_state.get('Containers') and network!=g['Network'],'restore network not idle/isolated');m.write(PUBLIC/'reused-network.json',dict(network=network,Internal=True,prior_active_members=[],predefined_pool_failure_preserved='post-v28-restored-core01'));volume=m.p.text('volume','create','--label','lifecycle307.v28-rehearsal=01','dpp307-v28-restored-core03')
  core=create('--network',network,'--network-alias','dpp-lifecycle307-808a6960-postgres','-e','POSTGRES_USER=fixture','-e','POSTGRES_DB=lifecycle307','-e','POSTGRES_HOST_AUTH_METHOD=trust','--mount','type=volume,source='+volume+',target=/var/lib/postgresql/data',m.p.PG)
  m.docker('start',core)
  for _ in range(80):
   if subprocess.run(['docker','exec',core,'pg_isready','-h','127.0.0.1','-U','fixture','-d','lifecycle307'],capture_output=True).returncode==0:break
   time.sleep(.25)
  m.p.call(['docker','exec','-i',core,'pg_restore','-h','127.0.0.1','-U','fixture','-d','lifecycle307','--exit-on-error','--no-owner','--no-acl'],data=raw)
  m.docker('exec',core,'psql','-h','127.0.0.1','-U','fixture','-d','lifecycle307','-v','ON_ERROR_STOP=1','-c',"ALTER DATABASE lifecycle307 SET lifecycle.rehearsal_marker='V28_ACTUAL_SCHEMA_ISOLATED_NO_AUTHORITY';")
  env=os.environ.copy();env.update(GOOS='linux',GOARCH='amd64',CGO_ENABLED='0')
  for name,args in [('observer',['go','build','-trimpath','-buildvcs=false','-o',str(CONTROL/'observer'),'./cmd/lifecycle-fixture-old-dispatch']),('command.test',['go','test','-c','-o',str(CONTROL/'command.test'),'./cmd/lifecycle-fixture-old-dispatch']),('pilot.test',['go','test','-c','-o',str(CONTROL/'pilot.test'),'./internal/lifecycle/pilot'])]:
   b=subprocess.run(args,cwd=ROOT/'apps/platform',env=env,capture_output=True);(PUBLIC/(name+'.build.stdout.txt')).write_bytes(b.stdout);(PUBLIC/(name+'.build.stderr.txt')).write_bytes(b.stderr);m.need(b.returncode==0,'rehearsal build failed')
  modeled=create('--network','container:'+core,'--mount','type=bind,source='+str(CONTROL/'pilot.test')+',target=/run/test,readonly','-e','TEST_POSTGRES_DSN=postgres://fixture@127.0.0.1:5432/lifecycle307?sslmode=disable','--entrypoint','/run/test',m.p.PG,'-test.v','-test.run','TestPostV28NoSendPostgresEligibility|TestPostV28Proof')
  m.docker('start',modeled);code=m.p.text('wait',modeled);(PUBLIC/'modeled-postgres.stdout.txt').write_bytes(m.docker('logs',modeled));m.need(code=='0','modeled eligibility failed')
  from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
  from cryptography.hazmat.primitives.serialization import Encoding,PublicFormat
  key=Ed25519PrivateKey.generate();at=m.now();audit=str(uuid.uuid4());folder=CONTROL/'worker';folder.mkdir();output=CONTROL/'output';output.mkdir()
  cfg=dict(Enabled=True,AuditID=audit,WorkspaceID=g['Copies'][0]['WorkspaceID'],ActorID=g['Actor'],Purpose='POST_V28_BUSINESS_DENIED_AND_NOT_SENT_CLOSURE',PostgresDSN='postgres://fixture:ISOLATED_TEST_ONLY@dpp-lifecycle307-808a6960-postgres:5432/lifecycle307?sslmode=disable',Core=g['Core'],BinarySHA256=m.sha(CONTROL/'command.test'),PublicKey=key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw).hex(),EvidenceSHA256=m.sha(PUBLIC/'source-before.json'),Token=secrets.token_hex(32),Owners={'a'*64:m.sha(CONTROL/'observer')},NotBefore=at.isoformat(),ExpiresAt=(at+dt.timedelta(minutes=30)).isoformat(),MaxProviderDeletes=0)
  m.write(folder/'closure.json',cfg);m.write(folder/'inspection.json',dict(Endpoint='127.0.0.1:9000',AccessKey='TEST_ONLY',SecretKey='TEST_ONLY_NOT_SOURCE'))
  body=dict(AuditID=audit,Nonce=str(uuid.uuid4()),At=at.isoformat(),Execution=m.EXECUTION,CandidateSHA256=m.CANDIDATE,BindingsSHA256=m.BINDINGS,Core=g['Core'],EvidenceSHA256=cfg['EvidenceSHA256'],Targets=m.targets()[1:],Owners=cfg['Owners'],StoppedPIDZero=True,NoRestart=True,ConfigDisabled=True,DeleteIdentityDisabled=True,GatewaySealed=True)
  encoded=json.dumps(body,separators=(',',':')).encode();m.write(folder/'proof.json',dict(Body=base64.b64encode(encoded).decode(),Signature=base64.b64encode(key.sign(encoded)).decode()))
  test=create('--network','container:'+core,'--user','10001:10001','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data:size=1m','--mount','type=bind,source='+str(folder)+',target=/run/private,readonly','--mount','type=bind,source='+str(CONTROL/'command.test')+',target=/run/test,readonly','--mount','type=bind,source='+str(output)+',target=/run/output','-e','TEST_POST_V28_RESTORED_SOURCE=1','--entrypoint','/run/test',m.p.PG,'-test.v','-test.run','TestV28ActualRestoredCommandAndReplay')
  m.p.ALLOWED_RELATIONS=m.ALLOWED;baseline=m.n.fingerprints(core);anchors=m.p.append_anchors(m.n,core)
  m.docker('start',test);code=m.p.text('wait',test);(PUBLIC/'actual-command.stdout.txt').write_bytes(m.docker('logs',test));m.need(code=='0','actual-schema mixed Commands failed')
  delta=m.delta_values(baseline,m.n.fingerprints(core),anchors,m.p.append_anchors(m.n,core));m.write(PUBLIC/'delta.json',delta)
  result.update(status='PASS_ACTUAL_SCHEMA_V28_MIXED_CLOSURE_AND_REPLAY',observer_binary_sha256=m.sha(CONTROL/'observer'),source_dump_sha256=m.sha(CONTROL/'source-Core.dump'),clone_core=core,clone_network=network,retained_volume=volume,modeled_cases=48,actual_schema_replay=True,source_Core_readonly_fingerprint_equal=True)
 except Exception as e:failure=type(e).__name__+': '+str(e)
 finally:
  result['closure']=[]
  for cid in reversed(owners):
   try:m.docker('stop','-t','10',cid)
   except Exception:pass
   try:result['closure'].append(dict(cid=cid,stopped_PID0=m.closed(m.inspect(cid))))
   except Exception:result['closure'].append(dict(cid=cid,stopped_PID0=False))
  result['source_retained_unchanged']=m.closed(m.inspect(g['Core'])) and m.closed(m.inspect(g['Native']))
  result['failure']=failure
  if failure or not result['source_retained_unchanged'] or not all(x['stopped_PID0'] for x in result['closure']):result['status']='FAILED_REHEARSAL_RETAINED_NO_SOURCE_COMMAND'
  m.write(PUBLIC/'terminal.json',result);m.write(CONTROL/'terminal.json',result)
 print(json.dumps(result),flush=True);return result['status']=='PASS_ACTUAL_SCHEMA_V28_MIXED_CLOSURE_AND_REPLAY'
if __name__=='__main__':raise SystemExit(0 if main() else 1)
