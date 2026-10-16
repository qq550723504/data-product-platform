"""One-shot composition for explicit candidate05/v15 authorization; no delete retry."""
import datetime as dt, hashlib, importlib.util, json, os, secrets, shutil, subprocess, sys, time, uuid
from pathlib import Path
ROOT=Path('C:/Users/Henry/.codex/worktrees/9400/data-product-platform')
SOURCE=Path('C:/Users/Henry/.codex/fixture-private/dpp-lifecycle307-5d2274ee')
CONTROL=SOURCE/'reference-disposal-execution02'
PUBLIC=ROOT/'docs/acceptance/evidence/issue307-d1/delete-confirmed-v15-execution02'
BASE=ROOT/'docs/acceptance/evidence/issue307-d1/delete-candidate05-preparation01'
PROJECT='dpp-lifecycle307-5d2274ee'
EXECUTION='9ea8f3e2-c805-44b4-b133-a36f50df9d15'
PROFILE='5d2274ee-9ea8f3e2'
GWIMAGE='sha256:d8542f48d34a9cf4e4c11a478865229840e87e4c96ea3f439101f31a5d35f75f'
USER_MESSAGE='授权按已核对的 candidate05/v15 完整精确方案及全部绑定执行，execution 9ea8f3e2-c805-44b4-b133-a36f50df9d15；仅删除方案所列两份源版本，遵守全部次数、窗口和保留限制。#318 保持 Draft，不合并。'
def module(name,path):
 s=importlib.util.spec_from_file_location(name,path);v=importlib.util.module_from_spec(s);s.loader.exec_module(v);return v
m=module('retained',ROOT/'examples/lifecycle-d1/complete-lc13-readonly.py');m.CONTROL=CONTROL
prep=module('candidate05',ROOT/'examples/lifecycle-d1/prepare-reference-candidate05.py')
def need(ok,msg):
 if not ok:raise RuntimeError(msg)
def read(p):return m.load(p)
def write(p,v):m.write(p,v)
def sha(p):return m.digest(p)
def now():return dt.datetime.now(dt.timezone.utc)
def change(p,changes):
 v=read(p);v.update(changes);p.write_text(json.dumps(v,indent=2)+'\n',encoding='utf-8',newline='\n')
def run(args,data=None,cwd=ROOT,env=None):
 r=subprocess.run(list(map(str,args)),input=data,capture_output=True,cwd=cwd,env=env)
 if r.returncode:
  (CONTROL/('failure-'+now().strftime('%Y%m%dT%H%M%S%f')+'.private-log')).write_bytes(r.stdout+b'\n'+r.stderr)
  raise RuntimeError('Controlled command failed; private diagnostics retained; no automatic retry')
 return r.stdout
def docker(*args,data=None):return run(['docker',*args],data)
def inspect(name):return json.loads(docker('inspect',name))[0]
def helper(name,folder,out,binary,network,env=(),extra=(),expected=0):
 args=['create','--pull','never','--name',name,'--label','lifecycle307.run=5d2274ee-a671-4bdc-b406-22b8dad8fd61','--label','lifecycle307.execution='+EXECUTION,'--network',network,'--read-only','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data','--mount',f'type=bind,source={folder},target=/run/private,readonly','--mount',f'type=bind,source={out},target=/run/output']
 for e in env:args+=['--env',e]
 args+=list(extra)+['--entrypoint','/run/private/'+binary,m.PGIMAGE]
 ident=docker(*args).decode().strip();docker('start',ident);code=int(docker('wait',ident).decode().strip());item=inspect(ident)
 log=subprocess.run(['docker','logs',ident],capture_output=True);(folder/(name+'-helper.private-log')).write_bytes(log.stdout+log.stderr)
 write(out/(name.split(PROJECT)[-1].strip('-')+'-helper.json'),{'id':ident,'name':name,'image':item['Image'],'exit':code,'pid':item['State']['Pid'],'network':item['HostConfig']['NetworkMode'],'read_only':item['HostConfig']['ReadonlyRootfs'],'root_payload_mounted':False})
 need(code==expected and not item['State']['Running'],'Helper failed; preserve state, no resend');return ident
def frozen():
 b=read(BASE/'source-binding.json')
 for n,k in [('executor','binary_sha256'),('host.exe','host_binary_sha256'),('recovery','recovery_binary_sha256'),('iam-readonly','iam_readonly_binary_sha256')]:need(sha(CONTROL/'candidate05'/n)==b[k],'Frozen private binary drift')
 for p,h in b['runtime_source_lf_sha256'].items():need(hashlib.sha256((ROOT/p).read_bytes().replace(b'\r\n',b'\n')).hexdigest()==h,'Frozen runtime source drift')
 for p,k in [('bounded-profile.json','profile_sha256'),('delete-executor-policy.json','iam_sha256'),('gateway-active.Caddyfile','active_gateway_sha256'),('gateway-sealed.Caddyfile','sealed_gateway_sha256')]:need(sha(BASE/'render'/p)==b[k],'Frozen render drift')
 c=read(BASE/'preflight/checkpoint.json');need(sha(CONTROL/'checkpoint/core.dump')==c['dump_sha256'] and sha(CONTROL/'checkpoint/globals.sql')==c['globals_sha256'],'Frozen Core checkpoint drift')
 for p,h in read(BASE/'initial-host.json')['private_seals_and_backups_sha256'].items():need(sha(SOURCE/p)==h,'Prior seals/backups drift')
 return b,c
def prepare():
 need(not PUBLIC.exists(),'Append-only execution already prepared');m.validate_source();b,c=frozen()
 need(run(['git','rev-parse','HEAD']).decode().strip()=='f22677a51e0cecdaf451e12181c2e71c0d21cf8d','Reviewed checkout drift')
 run(['python','-X','utf8',ROOT/'examples/lifecycle-d1/check-reference-candidate05.py'])
 PUBLIC.mkdir();operations=CONTROL/'operations';operations.mkdir()
 envelope='EXPLICIT_USER_AUTHORIZATION_BY_REFERENCE\nUSER_MESSAGE_VERBATIM:\n'+USER_MESSAGE+'\nAPPROVED_REFERENCED_PLAN (not retyped by user):\n'+(BASE/'confirmation-template.txt').read_text(encoding='utf-8')
 with (CONTROL/'human-confirmation.txt').open('x',encoding='utf-8',newline='\n') as f:f.write(envelope)
 write(PUBLIC/'human-authorization.json',{'status':'RECEIVED_EXPLICIT_USER_AUTHORIZATION_BY_REFERENCE','recorded_at':now().isoformat(),'execution':EXECUTION,'user_message_verbatim':USER_MESSAGE,'referenced_template_sha256':sha(BASE/'confirmation-template.txt'),'source_binding_sha256':sha(BASE/'source-binding.json'),'proposal_sha256':sha(BASE/'proposal.json'),'checkpoint_sha256':sha(BASE/'preflight/checkpoint.json'),'confirmation_envelope_sha256':sha(CONTROL/'human-confirmation.txt'),'user_retyped_template':False,'old_authorization_reused':False,'activation_window':'NOT_OPENED','delete':0})
 # Administrative composition is separate from, and cannot replace, frozen sender.
 old=SOURCE/'reference-disposal-execution01'
 admin=(old/'admin.go').read_text(encoding='utf-8').replace('refdelete614cd9ad','refdelete9ea8f3e2').replace('lifecycle307-reference-exact-614cd9ad','lifecycle307-reference-exact-9ea8f3e2').replace('ab24ab862bf0e21d23ee21989f4140b3c7200cdecee22d4a708cc45621da8b54',b['iam_sha256'])
 admin=admin.replace('madmin.AccountDisabled);e!=nil{return errors.New("executor create unknown; no retry")}', 'madmin.AccountEnabled);e!=nil{return errors.New("executor create unknown; no retry")}\n  defer func(){_ = admin.SetUserStatus(context.WithValue(context.Background(),ctxKey{},&call{}),user,madmin.AccountDisabled)}()')
 admin=admin.replace('if e=probe("executor",user,madmin.AccountDisabled);e!=nil{return e}\n case "inspect":','if e=admin.SetUserStatus(callCtx(),user,madmin.AccountDisabled);e!=nil{return errors.New("post-provision disable unknown")}\n  if e=probe("executor",user,madmin.AccountDisabled);e!=nil{return e}\n case "inspect":',1)
 # Emergency disable does not depend on any unrelated account health or policy read.
 marker='facts:=[]any{};probe:='
 emergency='if c.Mode=="emergency-disable" { info,err:=admin.GetUserInfo(callCtx(),user);absent:=madmin.ToErrorResponse(err).Code=="XMinioAdminNoSuchUser";if !absent {if err!=nil{return errors.New("emergency account read failed")};if info.Status!=madmin.AccountDisabled {if err=admin.SetUserStatus(callCtx(),user,madmin.AccountDisabled);err!=nil{return errors.New("emergency disable failed")}};info,err=admin.GetUserInfo(callCtx(),user);if err!=nil||info.Status!=madmin.AccountDisabled{return errors.New("emergency native disable readback failed")}};f,err:=os.OpenFile("/run/output/admin-result.json",os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600);if err!=nil{return err};defer f.Close();return json.NewEncoder(f).Encode(map[string]any{"status":"PASS_NEW_EXECUTOR_SEALED","absent":absent,"disabled":!absent,"delete":0})};\n facts:=[]any{};probe:='
 need(marker in admin,'Admin source composition changed');admin=admin.replace(marker,emergency)
 (operations/'admin.go').write_text(admin,encoding='utf-8',newline='\n')
 shutil.copyfile(CONTROL/'survivors-source.go',operations/'survivors.go')
 buildenv=dict(os.environ,CGO_ENABLED='0',GOOS='linux',GOARCH='amd64')
 buildenv=dict(os.environ,CGO_ENABLED='0',GOOS='linux',GOARCH='amd64')
 run(['go','build','-trimpath','-o',operations/'admin',operations/'admin.go'],cwd=ROOT/'apps/platform',env=buildenv)
 build_survivors()
 frozen();write(PUBLIC/'composition-build.json',{'source_binding':sha(BASE/'source-binding.json'),'frozen_binaries_unchanged':True,'sender_rebuilt':False,'admin_source_sha256':sha(operations/'admin.go'),'admin_binary_sha256':sha(operations/'admin'),'survivors_source_sha256':sha(operations/'survivors.go'),'survivors_binary_sha256':sha(operations/'survivors'),'root_payload_mount':False,'delete':0})
 print(json.dumps({'status':'PREPARED_DISABLED','delete':0}))
def build_survivors():
 operations=CONTROL/'operations';temporary=ROOT/'apps/platform/acceptance-candidate05-v15-local';need(not temporary.exists(),'Owned temporary build directory already exists');temporary.mkdir();source=temporary/'main.go';shutil.copyfile(operations/'survivors.go',source)
 try:run(['go','build','-trimpath','-o',operations/'survivors','./acceptance-candidate05-v15-local'],cwd=ROOT/'apps/platform',env=dict(os.environ,CGO_ENABLED='0',GOOS='linux',GOARCH='amd64'))
 finally:
  need(sha(source)==sha(operations/'survivors.go'),'Temporary build source drift');source.unlink();temporary.rmdir()
def finish_build():
 need((PUBLIC/'human-authorization.json').exists() and not (PUBLIC/'composition-build.json').exists(),'Only incomplete local compilation may resume');build_survivors();frozen();operations=CONTROL/'operations'
 write(PUBLIC/'composition-build.json',{'source_binding':sha(BASE/'source-binding.json'),'frozen_binaries_unchanged':True,'sender_rebuilt':False,'admin_source_sha256':sha(operations/'admin.go'),'admin_binary_sha256':sha(operations/'admin'),'survivors_source_sha256':sha(operations/'survivors.go'),'survivors_binary_sha256':sha(operations/'survivors'),'root_payload_mount':False,'delete':0})
 print(json.dumps({'status':'PREPARED_DISABLED','delete':0}))
def preflight():
 need((PUBLIC/'composition-build.json').exists(),'Prepared composition required');m.validate_source();b,c=frozen()
 out=PUBLIC/'preflight';out.mkdir();stage=CONTROL/'authorized-preflight';stage.mkdir()
 for ident in (m.PG,m.MINIO,c['id']):docker('start',ident)
 m.wait_audit_core(c['id']);before=m.fingerprint(m.PG);roles=m.role_acl(m.PG)
 need(before==read(BASE/'preflight/source-after.json')['relations'] and roles==c['roles_acl'],'Source Core/role drift')
 write(out/'source-before.json',{'relations':before,'roles_acl':roles,'at':now().isoformat()})
 collector=module('collector',ROOT/'examples/lifecycle-d1/candidate05/core-readonly-collector.py');collector.collect(m,out)
 closure=json.loads(m.source_read(m.PG,(ROOT/'examples/lifecycle-d1/candidate05/closure-readback.sql').read_text(encoding='utf-8')));prep.validate_closure(closure);write(out/'prior-execution-closure.json',closure)
 clone_before=m.source_read(c['id'],'SELECT count(*) FROM lifecycle_storage_call;');need(clone_before=='165','Checkpoint baseline drift')
 reader=stage/'native';reader.mkdir();payload=stage/'payload';payload.mkdir();shutil.copyfile(CONTROL/'candidate05/executor',reader/'executor')
 ref=read(CONTROL/'candidate05/reference.json');ref['Fixture']['PostgresDSN']=prep.checkpoint_dsn(ref['Fixture']['PostgresDSN']);ref['PreparationEnabled']=True;ref['PreparationUntil']=(now()+dt.timedelta(minutes=20)).isoformat();write(reader/'reference.json',ref);write(reader/'reference-permission.json',{'Enabled':False})
 nativeout=out/'native';nativeout.mkdir();helper(PROJECT+'-v15-authorized-preflight-native',reader,nativeout,'executor','container:'+m.MINIO,['LIFECYCLE_REFERENCE_PROFILE='+PROFILE,'LIFECYCLE_REFERENCE_MODE=inspect-source'],['--mount',f'type=bind,source={payload},target=/run/payload'])
 m.six_equal(read(nativeout/'native-six-version.json'),read(BASE/'native/native-six-version.json'));change(reader/'reference.json',{'PreparationEnabled':False})
 iam=stage/'iam';iam.mkdir();shutil.copyfile(CONTROL/'candidate05/iam-readonly',iam/'iam-readonly');shutil.copyfile(CONTROL/'iam/iam.json',iam/'iam.json');iamout=out/'iam';iamout.mkdir();helper(PROJECT+'-v15-authorized-preflight-iam',iam,iamout,'iam-readonly','container:'+m.MINIO)
 need(read(iamout/'native-iam-readonly.json')['status']=='PASS','Native IAM drift')
 need(m.fingerprint(m.PG)==before and m.role_acl(m.PG)==roles,'Readonly preflight mutated source')
 need(m.source_read(c['id'],'SELECT count(*) FROM lifecycle_storage_call;')=='185','Checkpoint accounting drift');docker('stop',c['id'])
 write(out/'verification.json',{'status':'PASS_EXACT_SIX_CURRENT_SOURCE_AND_IAM','source_mutation':0,'source_calls':125,'checkpoint_calls_before':165,'checkpoint_calls_after':185,'new_native_reads':20,'admin_get_rpc':5,'new_delete_account':'NOT_PROVISIONED','activation_window':'NOT_OPENED','delete':0})
 print(json.dumps({'status':'PASS_AUTHORIZED_PREFLIGHT','versions':6,'delete':0}))
def admin(mode):
 folder=CONTROL/('admin-'+mode);folder.mkdir();out=PUBLIC/('admin-'+mode);out.mkdir();shutil.copyfile(CONTROL/'operations/admin',folder/'admin');cfg=read(SOURCE/'config.json')
 if mode=='emergency-disable':
  value={'RootAccess':cfg['RootAccess'],'RootSecret':cfg['RootSecret'],'Mode':mode}
 else:
  shutil.copyfile(BASE/'render/delete-executor-policy.json',folder/'delete-executor-policy.json');reader=read(SOURCE/'plane-owner/reader.json')
  value={'RootAccess':cfg['RootAccess'],'RootSecret':cfg['RootSecret'],'DeleteSecret':(CONTROL/'delete-secret.private').read_text(encoding='ascii'),'Mode':mode,'Users':{'reader':reader['Access'],'producer':reader['ProducerAccess'],'inspector':cfg['InspectAccess']}}
 write(folder/'admin.json',value)
 helper(PROJECT+'-v15-admin-'+mode,folder,out,'admin','container:'+m.MINIO)
 return read(out/'admin-result.json')
def resources():
 need((PUBLIC/'preflight/verification.json').exists(),'Fresh preflight required');b,c=frozen();need(m.fingerprint(m.PG)==read(PUBLIC/'preflight/source-before.json')['relations'],'Source drift before resources')
 with (CONTROL/'delete-secret.private').open('x',encoding='ascii') as f:f.write(secrets.token_hex(32))
 need(admin('provision')['status']=='PASS_NATIVE_EXACT_IAM','New IAM not disabled/proven')
 outbox=json.loads(m.source_read(m.PG,'SELECT coalesce(jsonb_agg(to_jsonb(e) ORDER BY id),\'[]\') FROM outbox_event e;'));write(PUBLIC/'outbox-before.json',outbox)
 gateway=CONTROL/'gateway';gateway.mkdir();shutil.copyfile(BASE/'render/gateway-active.Caddyfile',gateway/'Caddyfile')
 gwname=PROJECT+'-reference-delete-gateway-02';gwid=docker('create','--pull','never','--name',gwname,'--label','lifecycle307.run=5d2274ee-a671-4bdc-b406-22b8dad8fd61','--label','lifecycle307.execution='+EXECUTION,'--network',PROJECT+'-private','--read-only','--tmpfs','/data','--tmpfs','/config','--mount',f'type=bind,source={gateway},target=/etc/caddy,readonly',GWIMAGE).decode().strip()
 redis=PROJECT+'-delete-recovery-redis-02';vol=redis+'-data';docker('volume','create','--label','lifecycle307.run=5d2274ee-a671-4bdc-b406-22b8dad8fd61',vol)
 redisid=docker('create','--pull','never','--name',redis,'--label','lifecycle307.run=5d2274ee-a671-4bdc-b406-22b8dad8fd61','--label','lifecycle307.execution='+EXECUTION,'--network',PROJECT+'-private','--read-only','--tmpfs','/tmp','--mount',f'type=volume,source={vol},target=/data',m.REDISIMAGE,'redis-server','--appendonly','yes','--save','').decode().strip()
 # One absolute common window, captured once after fresh prerequisites; no rolling extension.
 confirmed=now();expires=confirmed+dt.timedelta(minutes=35)
 ref=read(CONTROL/'candidate05/reference.json');need(not ref['Fixture']['RootAccess'] and not ref['Fixture']['RootSecret'],'Root-free sender required');ref['PreparationEnabled']=False;ref['AttestationPublicKey']='';dsn=ref['Fixture']['PostgresDSN']
 topology={'Execution':EXECUTION,'CoreID':m.PG,'MinioID':m.MINIO,'OriginalGatewayID':m.GW,'GatewayID':gwid,'GatewayName':gwname,'GatewayImage':GWIMAGE,'GatewayConfigSHA256':b['active_gateway_sha256'],'NetworkID':'33f303a57406f05c4e78b3cdb1355059a8f55f552e438162edc5bff129f4c02c','DSNSHA256':hashlib.sha256(dsn.encode()).hexdigest(),'BinarySHA256':b['binary_sha256']}
 senders=[]
 for stage in ('a','b'):
  folder=CONTROL/('executor-'+stage);folder.mkdir();out=PUBLIC/('send-'+stage);out.mkdir();shutil.copyfile(CONTROL/'candidate05/executor',folder/'executor');shutil.copyfile(CONTROL/'human-confirmation.txt',folder/'human-confirmation.txt');write(folder/'reference.json',ref);write(folder/'reference-topology.json',topology)
  write(folder/'reference-permission.json',{'Enabled':False,'RunID':EXECUTION,'WorkspaceID':ref['Fixture']['WorkspaceID'],'PlanHash':'d7db735bf2ea7a8bc8eefca70ad4fc046be18e7e159e6a5231be3073e550bbfc','DeleteAccess':'refdelete9ea8f3e2','DeleteSecret':(CONTROL/'delete-secret.private').read_text(encoding='ascii'),'BasisRef':'human-confirmed:reference-disposal:'+sha(CONTROL/'human-confirmation.txt'),'TopologySHA256':sha(folder/'reference-topology.json'),'ConfirmationSHA256':sha(CONTROL/'human-confirmation.txt'),'BinarySHA256':b['binary_sha256'],'ProfileSHA256':b['target_digest'],'MaxPerTarget':1,'MaxTotal':2,'ConfirmedAt':confirmed.isoformat(),'ExpiresAt':expires.isoformat()})
  name=PROJECT+'-reference-send-02-'+stage
  ident=docker('create','--pull','never','--name',name,'--label','lifecycle307.run=5d2274ee-a671-4bdc-b406-22b8dad8fd61','--label','lifecycle307.execution='+EXECUTION,'--network','container:'+gwid,'--read-only','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data','--mount',f'type=bind,source={folder},target=/run/private,readonly','--mount',f'type=bind,source={out},target=/run/output','--env','LIFECYCLE_REFERENCE_PROFILE='+PROFILE,'--env','LIFECYCLE_REFERENCE_MODE=send-'+stage,'--entrypoint','/run/private/executor',m.PGIMAGE).decode().strip();item=inspect(ident)
  need(item['State']['Status']=='created' and len(item['Mounts'])==2 and all(x['Type']=='bind' for x in item['Mounts']),'Sender two-bind closure failed');senders.append({'stage':stage,'id':ident,'name':name,'mount_count':2})
 recovery=CONTROL/'recovery';recovery.mkdir()
 for n in ('recovery',):shutil.copyfile(CONTROL/'candidate05'/n,recovery/n)
 shutil.copyfile(CONTROL/'operations/survivors',recovery/'survivors');shutil.copyfile(PUBLIC/'preflight/native/native-six-version.json',recovery/'preflight.json');write(recovery/'sql.json',{'PostgresDSN':dsn})
 f=ref['Fixture'];write(recovery/'recovery.json',{'Enabled':False,'SyntheticOnly':True,'RunID':EXECUTION,'WorkspaceID':f['WorkspaceID'],'ProviderInstanceID':f['ProviderInstanceID'],'IncarnationID':f['IncarnationID'],'ActorID':f['ActorID'],'Endpoint':f['Endpoint'],'InspectAccess':f['InspectAccess'],'InspectSecret':f['InspectSecret'],'Token':f['Token'],'ManifestSHA256':sha(recovery/'preflight.json'),'ValidUntil':expires.isoformat()})
 write(PUBLIC/'execution-resources.json',{'status':'PREPARED_DISABLED','gateway':{'id':gwid,'name':gwname,'active_config_sha256':sha(gateway/'Caddyfile')},'redis':{'id':redisid,'name':redis,'volume':vol},'senders':senders,'authority_confirmed_at':confirmed.isoformat(),'expires_at':expires.isoformat(),'window_seconds':2100,'sender_rebuilt':False,'root_payload_mount':False,'delete':0,'cleanup':0})
 print(json.dumps({'status':'RESOURCES_DISABLED','window_seconds':2100,'expires_at':expires.isoformat(),'delete':0}))
def activate():
 need(not (PUBLIC/'activation.json').exists() and not (PUBLIC/'sealed-state.json').exists(),'One-shot activation cannot reopen');r=read(PUBLIC/'execution-resources.json');need(now()<dt.datetime.fromisoformat(r['expires_at'])-dt.timedelta(minutes=10),'Insufficient fixed window; no extension');frozen()
 need(m.fingerprint(m.PG)==read(PUBLIC/'preflight/source-before.json')['relations'],'Source drift before activation')
 for ident in (r['gateway']['id'],r['redis']['id'],m.GW):docker('start',ident)
 need(docker('exec',r['redis']['id'],'redis-cli','PING').decode().strip()=='PONG','Recovery Redis unavailable');docker('exec',r['gateway']['id'],'caddy','validate','--config','/etc/caddy/Caddyfile','--adapter','caddyfile')
 for stage in ('a','b'):change(CONTROL/('executor-'+stage)/'reference-permission.json',{'Enabled':True})
 change(CONTROL/'recovery/recovery.json',{'Enabled':True});need(admin('enable')['status']=='PASS_NATIVE_EXACT_IAM','Native IAM enable/readback failed')
 write(PUBLIC/'activation.json',{'status':'ACTIVE_EXACT_TWO_ONLY','at':now().isoformat(),'expires_at':r['expires_at'],'max_per_target':1,'max_total':2,'original_gateway':'RUNNING_UNCHANGED_DENY_ALL403','cleanup':0,'hold_release':0,'bypass':0})
def send(stage):
 need(stage in ('a','b') and (PUBLIC/'activation.json').exists(),'Activation required')
 if stage=='b':need(read(PUBLIC/'recovery-a/receipt-closure.json')['status']=='PASS_EXACT_ABSENCE_AND_TERMINAL_REPLAY','A absence required')
 current=inspect(PROJECT+'-reference-send-02-'+stage);need(current['State']['Status']=='created','Never restart sender; recovery only');frozen()
 run([CONTROL/'candidate05/host.exe'],env=dict(os.environ,LIFECYCLE_REFERENCE_PROFILE=PROFILE,LIFECYCLE_REFERENCE_MODE='topology-host-'+stage))
 write(PUBLIC/('send-'+stage)/'signed-host-topology.json',read(CONTROL/('executor-'+stage)/'current-topology.json'))
 write(PUBLIC/('send-'+stage)/'start-once.json',{'at':now().isoformat(),'sender_id':current['Id'],'execution':EXECUTION,'max_start':1})
 docker('start',current['Id']);code=int(docker('wait',current['Id']).decode().strip());after=inspect(current['Id'])
 log=subprocess.run(['docker','logs',current['Id']],capture_output=True);(CONTROL/('sender-'+stage+'.private-log')).write_bytes(log.stdout+log.stderr)
 write(PUBLIC/('send-'+stage)/'sender-exit.json',{'id':current['Id'],'exit':code,'pid':after['State']['Pid'],'status':after['State']['Status'],'resend':'FORBIDDEN'})
 result=read(PUBLIC/('send-'+stage)/('send-'+stage+'-returned.json'))
 need(code==93 and result['invocation_certainty']=='CONFIRMED' and result['provider_outcome']=='UNKNOWN' and result['confirmed_invocation_observed'] is True,'Unexpected send outcome; stop next target and readonly recovery only')
 print(json.dumps({'stage':stage,'status':'CONFIRMED_UNKNOWN_EXIT93','attempt':result['attempt']['ID'],'work_id':result['work_id'],'resend':False}))
def recovery(stage):
 result=read(PUBLIC/('send-'+stage)/('send-'+stage+'-returned.json'));r=read(PUBLIC/'execution-resources.json');folder=CONTROL/'recovery';base=read(folder/'recovery.json');dsn=read(folder/'sql.json')['PostgresDSN'];out=PUBLIC/('recovery-'+stage);out.mkdir();work=str(uuid.UUID(result['work_id']))
 def step(mode,scenario):
  need(mode in ('outbox','worker','once'),'Finite existing worker modes only')
  write(folder/(scenario+'.json'),{'Mode':mode,'Scenario':scenario,'PostgresDSN':dsn,'RedisAddr':r['redis']['name']+':6379','RecoveryConfig':'/run/private/recovery.json','Manifest':'/run/private/preflight.json','OutputDirectory':'/run/output','Barrier':'','SeedEnabled':False,'CopyRecovery':False,'WorkspaceID':base['WorkspaceID'],'ActorID':base['ActorID'],'CopyID':result['copy']['ID'],'WorkID':work,'AttemptID':result['attempt']['ID'],'ExpiresAt':base['ValidUntil']})
  helper(PROJECT+'-v15-'+scenario,folder,out,'recovery','container:'+r['gateway']['id'],['LIFECYCLE_RECOVERY_ACCEPTANCE_CONFIG=/run/private/'+scenario+'.json'])
 lock_query="BEGIN; SELECT count(*) FROM (SELECT id FROM outbox_event WHERE NOT (aggregate_id='"+work+"' AND event_type='LifecycleDispositionRecoveryRequested') FOR UPDATE) locked; SELECT 'LOCK_READY';\n"
 process=subprocess.Popen(['docker','exec','-i',m.PG,'psql','-X','-qAt','-v','ON_ERROR_STOP=1','-U','fixture','-d','lifecycle307'],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,cwd=ROOT)
 try:
  process.stdin.write(lock_query.encode());process.stdin.flush();locked=int(process.stdout.readline().decode().strip());need(process.stdout.readline().decode().strip()=='LOCK_READY','Outbox isolation lock unavailable');step('outbox',stage+'-outbox')
 finally:
  process.stdin.write(b'ROLLBACK;\n');process.stdin.close();need(process.wait(timeout=15)==0,'Outbox isolation rollback failed')
 write(out/'outbox-isolation.json',{'status':'RELEASED_NO_MUTATION','non_target_rows_locked':locked,'target_work':work,'transaction':'ROLLBACK','target_dispatch':'existing SKIP LOCKED dispatcher','direct_queue_injection':0})
 need(read(out/(stage+'-outbox.json'))['state']=='PUBLISHED','Exact recovery outbox not published');step('worker',stage+'-worker')
 def counts():return json.loads(m.source_read(m.PG,"SELECT json_build_object('storage_calls',(SELECT count(*) FROM lifecycle_storage_call),'attempts',(SELECT count(*) FROM lifecycle_provider_attempt),'claims',(SELECT count(*) FROM lifecycle_provider_send_claim),'receipts',(SELECT count(*) FROM lifecycle_copy_absence_receipt),'completion',(SELECT count(*) FROM lifecycle_plan_completion));"))
 before=counts();step('once',stage+'-terminal-replay');after=counts();need(before==after,'Terminal replay produced provider I/O/facts')
 need(after['receipts']==(1 if stage=='a' else 2) and after['completion']==(0 if stage=='a' else 1),'Absence/completion ordering failed')
 write(out/'receipt-closure.json',{'status':'PASS_EXACT_ABSENCE_AND_TERMINAL_REPLAY','stage':stage,'work':work,'attempt':result['attempt']['ID'],'counts':after,'replay_provider_io_delta':0,'native_delete_in_recovery':0})
 print(json.dumps({'stage':stage,'status':'PASS_EXACT_ABSENCE','receipts':after['receipts'],'completion':after['completion']}))
def final_readback():
 need((PUBLIC/'recovery-b/receipt-closure.json').exists(),'Both receipts required');r=read(PUBLIC/'execution-resources.json');out=PUBLIC/'survivors';out.mkdir();helper(PROJECT+'-v15-four-survivors',CONTROL/'recovery',out,'survivors','container:'+m.MINIO)
 before=read(PUBLIC/'preflight/source-before.json')['relations'];after=m.fingerprint(m.PG);changed=[v['relation'] for v in after if v!=next(x for x in before if x['relation']==v['relation'])]
 allowed={'audit_event','evidence','outbox_event','outbox_event_consumption','cost_event','cost_allocation','workspace_delivery_fence'};need(all(n in allowed or n.startswith('lifecycle_') for n in changed),'Unrelated business relation changed')
 old={x['id']:x for x in read(PUBLIC/'outbox-before.json')};cur=json.loads(m.source_read(m.PG,'SELECT coalesce(jsonb_agg(to_jsonb(e) ORDER BY id),\'[]\') FROM outbox_event e;'));need(all(x==old[x['id']] for x in cur if x['id'] in old),'Old outbox rows changed')
 write(PUBLIC/'source-after.json',{'at':now().isoformat(),'relations':after,'changed_relations':changed,'old_business_relations_unchanged':True,'old_outbox_rows_unchanged':len(old)})
 sql="SELECT jsonb_build_object('attempts',(SELECT jsonb_agg(to_jsonb(a) ORDER BY a.started_at) FROM lifecycle_provider_attempt a),'observations',(SELECT jsonb_agg(to_jsonb(o) ORDER BY o.observed_at) FROM lifecycle_provider_observation o),'claims',(SELECT jsonb_agg(to_jsonb(c)) FROM lifecycle_provider_send_claim c),'receipts',(SELECT jsonb_agg(to_jsonb(r)) FROM lifecycle_copy_absence_receipt r),'completion',(SELECT jsonb_agg(to_jsonb(c)) FROM lifecycle_plan_completion c),'work',(SELECT jsonb_agg(to_jsonb(w)) FROM lifecycle_reconciliation_work w),'results',(SELECT jsonb_agg(to_jsonb(r)) FROM lifecycle_reconciliation_result r),'storage_calls',(SELECT jsonb_agg(to_jsonb(c)) FROM lifecycle_storage_call c),'storage_observations',(SELECT jsonb_agg(to_jsonb(o)) FROM lifecycle_storage_observation o),'costs',(SELECT jsonb_agg(to_jsonb(c)) FROM cost_event c WHERE id IN(SELECT cost_event_id FROM cost_allocation WHERE lifecycle_provider_attempt_id IS NOT NULL OR lifecycle_storage_call_id IS NOT NULL)),'allocations',(SELECT jsonb_agg(to_jsonb(a)) FROM cost_allocation a WHERE lifecycle_provider_attempt_id IS NOT NULL OR lifecycle_storage_call_id IS NOT NULL))::text;"
 sql=sql.replace("'attempts',","'releases',(SELECT jsonb_agg(to_jsonb(r)) FROM lifecycle_dispatch_release r),'attempts',",1)
 ledger=json.loads(m.source_read(m.PG,sql));write(PUBLIC/'final-ledger.json',ledger);validation=validate_ledger(ledger);write(PUBLIC/'ledger-validation.json',validation)
 need(m.role_acl(m.PG)==read(PUBLIC/'preflight/source-before.json')['roles_acl'],'Source role/ACL changed')
 write(PUBLIC/'acceptance.json',{'status':'PASS_TWO_EXACT_ABSENCES_AND_FOUR_SURVIVORS','execution':EXECUTION,'native_delete_invocations':validation['new_physical_attempts'],'response_loss_crashes':2,'source_absence_receipts':len(ledger['receipts']),'scope_completion':len(ledger['completion']),'terminal_replay_provider_io':0,'old_history_retained':validation['all_old_ledger_rows_unchanged'],'old_outbox_unchanged':True,'cleanup':0,'hold_release':0,'bypass':0,'pr318':'DRAFT_NO_MERGE'})
def validate_ledger(v):
 before=read(PUBLIC/'ledger-before.json');preserved={}
 for key,rows in before.items():
  identity='attempt_id' if key=='claims' else 'id';current={x[identity]:x for x in v[key] or []};need(all(current.get(x[identity])==x for x in rows or []),'Historical ledger row changed: '+key);preserved[key]=len(rows or [])
 need(v['releases']==read(PUBLIC/'old-dispatch-releases-before.json'),'Old dispatch release facts changed')
 returned=[read(PUBLIC/('send-'+s)/('send-'+s+'-returned.json')) for s in ('a','b')];expected={x['attempt']['ID']:x for x in returned};oldids={x['id'] for x in before['attempts']};attempts={x['id']:x for x in v['attempts'] if x['id'] not in oldids};need(set(attempts)==set(expected),'Unexpected physical attempt count')
 claims=[x for x in v['claims'] if x['attempt_id'] in expected];need(len(claims)==2 and {x['attempt_id'] for x in claims}==set(expected),'Exactly one new claim per attempt required')
 for ident,result in expected.items():
  a=attempts[ident];need(a['copy_id']==result['copy']['ID'] and a['plan_id']=='8f0e9340-18f0-4a73-bdd6-622196a9011a' and a['action']=='DELETE_EXACT','Physical attempt scope changed')
  obs=[x for x in v['observations'] if x['attempt_id']==ident];need(len(obs)==1 and obs[0]['certainty']=='CONFIRMED' and obs[0]['outcome']=='UNKNOWN' and obs[0]['invoked_at'] is not None,'Physical observation mismatch')
  alloc=[x for x in v['allocations'] if x['lifecycle_provider_attempt_id']==ident];need(len(alloc)==1,'Provider attempt cost allocation mismatch');cost=[x for x in v['costs'] if x['id']==alloc[0]['cost_event_id']];need(len(cost)==1 and cost[0]['activity_id']==ident and cost[0]['quantity']==1 and cost[0]['unit']=='invocation','Provider invocation cost mismatch')
 callids={x['id'] for x in v['storage_calls']};obs=v['storage_observations'];alloc=[x for x in v['allocations'] if x['lifecycle_storage_call_id'] is not None];need(len(callids)==len(v['storage_calls'])==len(obs)==len(alloc) and callids=={x['call_id'] for x in obs}=={x['lifecycle_storage_call_id'] for x in alloc},'Storage call/observation/allocation closure failed')
 costs={x['id']:x for x in v['costs']}
 for a in alloc:
  c=costs[a['cost_event_id']];need(c['activity_id']==a['lifecycle_storage_call_id'] and c['quantity']==1 and c['unit']=='invocation','Storage invocation cost mismatch')
 need(len(v['receipts'])==2 and len(v['completion'])==1 and {x['copy_id'] for x in v['receipts']}=={x['copy']['ID'] for x in returned},'Exact absence/completion mismatch')
 return {'status':'PASS_PHYSICAL_COST_HISTORY_AND_ABSENCE_CLOSURE','new_physical_attempts':len(attempts),'new_claims':len(claims),'new_provider_costs':2,'storage_calls':len(callids),'new_storage_calls':len(callids)-len(before['storage_calls']),'old_rows_preserved':preserved,'old_dispatch_releases_preserved':len(v['releases']),'all_old_ledger_rows_unchanged':True}
def seal():
 failures=[]
 for stage in ('a','b'):
  p=CONTROL/('executor-'+stage)/'reference-permission.json'
  if p.exists():
   try:change(p,{'Enabled':False})
   except Exception:failures.append('permission-'+stage)
 p=CONTROL/'recovery/recovery.json'
 if p.exists():
  try:change(p,{'Enabled':False})
  except Exception:failures.append('recovery-authority')
 if (CONTROL/'delete-secret.private').exists():
  try:
   if not inspect(m.MINIO)['State']['Running']:docker('start',m.MINIO)
   need(admin('emergency-disable')['status']=='PASS_NEW_EXECUTOR_SEALED','Native seal failed')
  except Exception:failures.append('new-native-IAM-disable')
 ids=[m.PG,m.MINIO,m.GW]
 try:
  r=read(PUBLIC/'execution-resources.json');ids.extend([r['gateway']['id'],r['redis']['id'],*[x['id'] for x in r['senders']]])
 except Exception:failures.append('known-resource-readback')
 for p in PUBLIC.rglob('*-helper.json'):
  try:ids.append(read(p)['id'])
  except Exception:failures.append('known-helper-readback')
 try:ids.extend(docker('ps','-aq','--filter','label=lifecycle307.run=5d2274ee-a671-4bdc-b406-22b8dad8fd61').decode().split())
 except Exception:failures.append('project-enumeration')
 ids=list(dict.fromkeys(ids));stopped=prep.stop_all(m,ids);failures+=stopped['failures']
 try:
  if (CONTROL/'gateway/Caddyfile').exists():shutil.copyfile(BASE/'render/gateway-sealed.Caddyfile',CONTROL/'gateway/Caddyfile')
  for p in (CONTROL/'authorized-preflight').rglob('reference.json') if (CONTROL/'authorized-preflight').exists() else []:change(p,{'PreparationEnabled':False})
  frozen();states=[]
  for ident in ids:
   i=inspect(ident);need(not i['State']['Running'] and i['State']['Pid']==0 and not any(i['NetworkSettings']['Ports'].values()),'Project container not sealed');states.append({'id':ident,'name':i['Name'],'status':i['State']['Status'],'pid':0,'host_ports':[]})
 except Exception:failures.append('project-preservation-verification')
 write(PUBLIC/'sealed-state.json',{'at':now().isoformat(),'status':'PASS_SEALED' if not failures else 'SEAL_FAILURE_REQUIRES_ATTENTION','failures':failures,'all_project_containers':states if 'states' in locals() else [],'old_private_seals_and_backups_unchanged':not failures,'checkpoint_backup_unchanged':not failures,'new_gateway_sealed_sha256':sha(CONTROL/'gateway/Caddyfile') if (CONTROL/'gateway/Caddyfile').exists() else None,'sender_permissions':False,'cleanup':0,'hold_release':0,'bypass':0})
 need(not failures,'Independent sealing failure requires attention');print(json.dumps({'status':'SEALED','containers':len(ids),'cleanup':0}))
def execute():
 error=None
 try:
  activate();send('a');recovery('a');send('b');recovery('b');final_readback()
 except BaseException as e:
  error=e;write(PUBLIC/'execution-stopped.json',{'at':now().isoformat(),'status':'STOPPED_NO_RESEND','reason':str(e),'remaining_send':'FORBIDDEN_UNTIL_ASSESSED','readonly_recovery_only':True})
 finally:seal()
 if error:raise error
if __name__=='__main__':globals()[sys.argv[1]]()
