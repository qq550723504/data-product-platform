import datetime as dt, hashlib, importlib.util, json, os, secrets, shutil, subprocess, sys, time
from pathlib import Path
ROOT=Path('C:/Users/Henry/.codex/worktrees/9400/data-product-platform')
SOURCE=Path('C:/Users/Henry/.codex/fixture-private/dpp-lifecycle307-5d2274ee')
CONTROL=SOURCE/'reference-disposal-execution01'
PUBLIC=ROOT/'docs/acceptance/evidence/issue307-d1/delete-confirmed-v14-execution01'
PROJECT='dpp-lifecycle307-5d2274ee'
EXECUTION='614cd9ad-3f9d-40be-b110-3f7cf95f0790'
SOURCE_SHA='055aa1df063e560c6796e92dfff4e03c7b9061e0'
def module(name,path):
 s=importlib.util.spec_from_file_location(name,path);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
m=module('base',ROOT/'examples/lifecycle-d1/complete-lc13-readonly.py')
m.CONTROL=CONTROL
def need(x,msg):m.require(x,msg)
def read(p):return m.load(p)
def write(p,v):m.write(p,v)
def sha(p):return m.digest(p)
def run(args,data=None,cwd=ROOT,env=None):
 r=subprocess.run(list(map(str,args)),input=data,capture_output=True,cwd=cwd,env=env)
 if r.returncode:
  if CONTROL.exists():(CONTROL/('failed-'+dt.datetime.now(dt.timezone.utc).strftime('%Y%m%dT%H%M%S%f')+'.private-log')).write_bytes(r.stdout+b'\n'+r.stderr)
  raise RuntimeError('Command failed; private diagnostics retained; no automatic retry')
 return r.stdout
def docker(*args,data=None):return run(['docker',*args],data)
def inspect(name):return json.loads(docker('inspect',name))[0]
def helper(name,folder,out,binary,network,env=(),extra=(),expected=0):
 args=['create','--pull','never','--name',name,'--label','lifecycle307.run=5d2274ee-a671-4bdc-b406-22b8dad8fd61','--label','lifecycle307.execution='+EXECUTION,'--network',network,'--read-only','--tmpfs','/tmp','--mount',f'type=bind,source={folder},target=/run/private,readonly','--mount',f'type=bind,source={out},target=/run/output']
 for e in env:args+=['--env',e]
 args+=list(extra)+['--entrypoint','/run/private/'+binary,m.PGIMAGE]
 ident=docker(*args).decode().strip();docker('start',ident);code=int(docker('wait',ident).decode().strip())
 item=inspect(ident);write(out/(name.split(PROJECT)[-1].strip('-')+'-helper.json'),{'id':ident,'name':name,'image':item['Image'],'exit':code,'pid':item['State']['Pid'],'network':item['HostConfig']['NetworkMode'],'read_only':item['HostConfig']['ReadonlyRootfs'],'root_payload_mounted':False})
 need(code==expected and not item['State']['Running'],'Helper failed; preserve state, no resend')
 return ident
CONFIRMATION='''CONFIRM_DELETE_EXACT_TWO；execution 614cd9ad-3f9d-40be-b110-3f7cf95f0790。
确认 project dpp-lifecycle307-5d2274ee、provider 1980751b-c50e-4895-93a8-cb55960cd635、
incarnation be784eda-96e4-49a9-ae47-eaa3203a2dc5，plan 8f0e9340-18f0-4a73-bdd6-622196a9011a、
hash d7db735bf2ea7a8bc8eefca70ad4fc046be18e7e159e6a5231be3073e550bbfc，
仅 dpp-lifecycle307-online/fixture/a.csv@312365a9-f984-43de-b9d5-7e26009ccd82 与
dpp-lifecycle307-quarantine/fixture/b.csv@846d0c7c-289d-4667-bc7c-aa04e44357d4 两份源版本，
绑定 source055aa1df063e560c6796e92dfff4e03c7b9061e0，
Linux binary 01bff6e2a242a202e73519d7b5a4f1fc109430614b25a5189eacfb0ebb2cddf3，
proposal-v14/candidate04 profile 97d565f9c66d8cfcb66997d034e7acb2d1519864898950a9ddb2ced2cdd5c4ee，
target digest 4cbb36e5aebbc9451603efd276aa309a2c4afdefcf252107571b0f8334360a38，
IAM ab24ab862bf0e21d23ee21989f4140b3c7200cdecee22d4a708cc45621da8b54，
active gateway 7b24ad32506bfd189c9289ba85a65ab242069dfee293d61b330104a90306cd0a，
sealed gateway 6a08b8647cc63cb389e2a511a5ad944b82d14e1ec9721f1a5ae45cb984bed4f6；
在所有执行前条件通过后，每项最多一次exact DELETE，
≤40分钟窗口，并执行本方案只读对账/封存。
四份 survivor、全部其他环境和备份保留；cleanup=0、hold-release=0、bypass=0；#318仍Draft、不合并。
'''
def prepare():
 candidate,binding=m.frozen_candidate();m.validate_source()
 need(run(['git','rev-parse','HEAD']).decode().strip()=='2babc930d46c38a3c5241e52b15a13706f081345','Reviewed checkout drift')
 CONTROL.mkdir();PUBLIC.mkdir();(CONTROL/'human-confirmation.txt').write_text(CONFIRMATION,encoding='utf-8',newline='\n')
 write(PUBLIC/'human-authorization.json',{'schema':'lifecycle307-human-exact-authorization-v1','recorded_at':dt.datetime.now(dt.timezone.utc).isoformat(),'execution':EXECUTION,'proposal':'v14/candidate04','status':'RECEIVED_CONDITIONAL_ON_ALL_PRECONDITIONS','confirmation_text':CONFIRMATION,'confirmation_sha256':sha(CONTROL/'human-confirmation.txt'),'candidate_source':SOURCE_SHA,'candidate_binding':binding,'permission_enabled':False,'activation_window':'NOT_OPENED','delete':0,'cleanup':0})
 bin=CONTROL/'bin';bin.mkdir();env=dict(os.environ,CGO_ENABLED='0',GOOS='linux',GOARCH='amd64')
 run(['go','build','-trimpath','-o',bin/'recovery','./cmd/lifecycle-fixture-recovery'],cwd=ROOT/'apps/platform',env=env)
 env.update(GOOS='windows');run(['go','build','-trimpath','-o',bin/'host.exe','./cmd/lifecycle-fixture-delete'],cwd=ROOT/'apps/platform',env=env)
 need(all(sha(ROOT/f)==h for f,h in binding['runtime_source_hashes'].items()),'Frozen runtime changed during build')
 write(PUBLIC/'execution-build.json',{'source_commit':SOURCE_SHA,'runtime_source_files':len(binding['runtime_source_hashes']),'all_source_hashes_match':True,'sender_binary_sha256':sha(candidate),'recovery_binary_sha256':sha(bin/'recovery'),'host_topology_binary_sha256':sha(bin/'host.exe'),'build':'go build -trimpath; CGO_ENABLED=0; linux/amd64 recovery and windows/amd64 host','sender_rebuilt':False,'delete':0})
 print(json.dumps({'status':'PREPARED_DISABLED','recovery_sha256':sha(bin/'recovery'),'sender_unchanged':True,'delete':0}))
def preflight():
 need((PUBLIC/'execution-build.json').exists(),'Build binding required');r,cfg=m.validate_source();candidate,binding=m.frozen_candidate()
 out=PUBLIC/'preflight';out.mkdir();stage=CONTROL/'preflight';stage.mkdir()
 for ident in [m.PG,m.MINIO]:docker('start',ident)
 for _ in range(30):
  try:before=m.fingerprint(m.PG);break
  except RuntimeError:time.sleep(1)
 else:raise RuntimeError('Source PostgreSQL unavailable')
 previous=read(ROOT/'docs/acceptance/evidence/issue307-d1/delete-readonly-v13-refresh01/5d-13/source-after.json')
 need(before==previous['relations'] and m.role_acl(m.PG)==previous['roles_acl'],'Source facts drift; no authorization activation')
 write(out/'source-before.json',{'relations':before,'roles_acl':m.role_acl(m.PG),'at':dt.datetime.now(dt.timezone.utc).isoformat()})
 collector=module('collector',ROOT/'docs/acceptance/evidence/issue307-d1/delete-readonly-candidate04/retry02/5d-04/core-readonly-collector.py');collector.collect(m,out)
 m.binary_dump(['exec',m.PG,'pg_dump','-U','fixture','-d','lifecycle307','--format=custom','--create'],stage/'core.dump')
 m.binary_dump(['exec',m.PG,'pg_dumpall','-U','fixture','--globals-only'],stage/'globals.sql')
 audit=PROJECT+'-delete-checkpoint-core-01';vol=audit+'-data';(stage/'audit-password').write_text(secrets.token_hex(32),encoding='ascii')
 docker('volume','create','--label','lifecycle307.run='+r['run_id'],vol)
 ident=docker('create','--pull','never','--name',audit,'--label','lifecycle307.run='+r['run_id'],'--network',r['network'],'--mount',f'type=volume,source={vol},target=/var/lib/postgresql/data','--mount',f'type=bind,source={stage},target=/run/backup,readonly','-e','POSTGRES_USER=checkpoint_admin','-e','POSTGRES_DB=postgres','-e','POSTGRES_PASSWORD_FILE=/run/backup/audit-password','-e','POSTGRES_HOST_AUTH_METHOD=scram-sha-256',m.PGIMAGE).decode().strip();docker('start',ident);m.wait_audit_core(audit)
 docker('exec',audit,'psql','-X','-U','checkpoint_admin','-d','postgres','-v','ON_ERROR_STOP=1','-f','/run/backup/globals.sql');docker('exec',audit,'pg_restore','--exit-on-error','--create','-U','checkpoint_admin','-d','postgres','/run/backup/core.dump')
 need(m.fingerprint(audit)==before and m.role_acl(audit)==m.role_acl(m.PG),'Fresh checkpoint restore mismatch')
 native=stage/'native';native.mkdir();payload=stage/'payload';payload.mkdir();shutil.copyfile(candidate,native/'executor')
 ref=read(SOURCE/'reference-disposal-preparation01/native-after/reference.json');ref['Fixture']['PostgresDSN']=ref['Fixture']['PostgresDSN'].replace(PROJECT+'-postgres:5432',audit+':5432');ref['PreparationEnabled']=True;ref['PreparationUntil']=(dt.datetime.now(dt.timezone.utc)+dt.timedelta(minutes=40)).isoformat();write(native/'reference.json',ref);write(native/'reference-permission.json',{'Enabled':False})
 helper(PROJECT+'-delete-checkpoint-native-01',native,out,'executor','container:'+m.MINIO,['LIFECYCLE_REFERENCE_PROFILE=5d2274ee-614cd9ad','LIFECYCLE_REFERENCE_MODE=inspect-source'],['--mount',f'type=bind,source={payload},target=/run/payload'])
 m.six_equal(read(out/'native-six-version.json'),read(ROOT/'docs/acceptance/evidence/issue307-d1/delete-readonly-v13-refresh01/5d-13/native-six-version.json'))
 need(m.fingerprint(m.PG)==before,'Source changed during native preflight')
 ref['PreparationEnabled']=False;(native/'reference.json').write_text(json.dumps(ref,indent=2)+'\n',encoding='utf-8')
 docker('stop',audit)
 write(out/'checkpoint.json',{'status':'PASS_EXACT_SIX_AND_CURRENT_CORE','source_rows':sum(x['rows'] for x in before),'restored_core':audit,'restored_core_id':ident,'fresh_dump_sha256':sha(stage/'core.dump'),'source_business_mutations':0,'native_read_calls':20,'native_delete':0,'activation':'NOT_OPENED','cleanup':0})
 print(json.dumps({'status':'PASS_NATIVE_PREFLIGHT','source_rows':sum(x['rows'] for x in before),'native_versions':6,'native_delete':0,'window':'NOT_OPENED'}))
def admin(mode):
 if mode=='enable':
  need(not (PUBLIC/'send-window-closed.json').exists() and dt.datetime.now(dt.timezone.utc)<dt.datetime(2026,10,15,9,12,tzinfo=dt.timezone.utc),'Closed/expired execution cannot enable IAM')
 need(read(PUBLIC/'preflight/checkpoint.json')['status']=='PASS_EXACT_SIX_AND_CURRENT_CORE','Native preflight required')
 folder=CONTROL/('admin-'+mode);folder.mkdir();out=PUBLIC/('admin-'+mode);out.mkdir()
 if mode=='provision':
  secret=secrets.token_hex(32);(CONTROL/'delete-secret.private').write_text(secret,encoding='ascii')
 else:secret=(CONTROL/'delete-secret.private').read_text(encoding='ascii')
 env=dict(os.environ,CGO_ENABLED='0',GOOS='linux',GOARCH='amd64');run(['go','build','-trimpath','-o',folder/'admin',CONTROL/'admin.go'],cwd=ROOT/'apps/platform',env=env)
 shutil.copyfile(CONTROL/'admin.go',folder/'admin-source.go')
 cfg=read(SOURCE/'config.json');owner=read(SOURCE/'plane-owner/reader.json')
 write(folder/'admin.json',{'RootAccess':cfg['RootAccess'],'RootSecret':cfg['RootSecret'],'DeleteSecret':secret,'Mode':'inspect' if mode.startswith('inspect') else mode,'Users':{'reader':owner['Access'],'producer':owner['ProducerAccess'],'inspector':cfg['InspectAccess']}})
 shutil.copyfile(m.PRIVATE/'d1-lc13-continuation01/delete-candidate04/output/delete-executor-policy.json',folder/'delete-executor-policy.json')
 write(out/'build.json',{'source_sha256':sha(CONTROL/'admin.go'),'binary_sha256':sha(folder/'admin'),'root_admin_only':True,'sender_root_mount':False,'auto_retry':False,'mode':mode})
 helper(PROJECT+'-reference-admin-'+mode,folder,out,'admin','container:'+m.MINIO)
 print(json.dumps({'status':'PASS_NATIVE_ADMIN','mode':mode,'delete':0,'window':'NOT_OPENED' if mode=='provision' else 'SEE_ACTIVATION_RECEIPT'}))
def resources():
 candidate,binding=m.frozen_candidate();need(read(PUBLIC/'admin-attach/admin-result.json')['status']=='PASS_NATIVE_EXACT_IAM','Native exact disabled IAM required')
 checkpoint=read(PUBLIC/'preflight/source-before.json');need(m.fingerprint(m.PG)==checkpoint['relations'],'Source Core drift before activation')
 need((CONTROL/'bin/survivors').is_file(),'Read-only survivor composition must be built before sending')
 outbox=json.loads(m.source_read(m.PG,'SELECT json_agg(t) FROM (SELECT event_type,status,count(*) FROM outbox_event GROUP BY event_type,status ORDER BY event_type,status)t;'));write(PUBLIC/'outbox-before.json',outbox)
 rendered=candidate.parent/'output';gateway=CONTROL/'gateway';gateway.mkdir();shutil.copyfile(rendered/'gateway-active.Caddyfile',gateway/'Caddyfile')
 need(sha(gateway/'Caddyfile')==binding['active_gateway_sha256'],'Active gateway bytes drift')
 gwname=PROJECT+'-reference-delete-gateway';gwid=docker('create','--pull','never','--name',gwname,'--label','lifecycle307.run=5d2274ee-a671-4bdc-b406-22b8dad8fd61','--label','lifecycle307.execution='+EXECUTION,'--network',PROJECT+'-private','--read-only','--tmpfs','/data','--tmpfs','/config','--mount',f'type=bind,source={gateway},target=/etc/caddy,readonly','sha256:d8542f48d34a9cf4e4c11a478865229840e87e4c96ea3f439101f31a5d35f75f').decode().strip()
 redis=PROJECT+'-delete-recovery-redis';vol=redis+'-data';docker('volume','create','--label','lifecycle307.run=5d2274ee-a671-4bdc-b406-22b8dad8fd61',vol)
 redisid=docker('create','--pull','never','--name',redis,'--label','lifecycle307.run=5d2274ee-a671-4bdc-b406-22b8dad8fd61','--label','lifecycle307.execution='+EXECUTION,'--network',PROJECT+'-private','--read-only','--tmpfs','/tmp','--mount',f'type=volume,source={vol},target=/data',m.REDISIMAGE,'redis-server','--appendonly','yes','--save','').decode().strip()
 # The actual send/recovery authority is strictly shorter than forty minutes
 # from the first recorded receipt of this confirmation, not a rolling window.
 confirmed='2026-10-15T08:37:14Z';expires='2026-10-15T09:12:00Z'
 need(dt.datetime.now(dt.timezone.utc)<dt.datetime.fromisoformat(expires.replace('Z','+00:00'))-dt.timedelta(minutes=6),'Insufficient fixed authority window; do not activate')
 ref=read(candidate.parent/'reference.json');ref['PreparationEnabled']=False;ref['AttestationPublicKey']='';ref['Fixture']['PostgresDSN']=read(SOURCE/'reference-disposal-preparation01/native-after/reference.json')['Fixture']['PostgresDSN']
 dsn=ref['Fixture']['PostgresDSN'];topology={'Execution':EXECUTION,'CoreID':m.PG,'MinioID':m.MINIO,'OriginalGatewayID':m.GW,'GatewayID':gwid,'GatewayName':gwname,'GatewayImage':'sha256:d8542f48d34a9cf4e4c11a478865229840e87e4c96ea3f439101f31a5d35f75f','GatewayConfigSHA256':binding['active_gateway_sha256'],'NetworkID':'33f303a57406f05c4e78b3cdb1355059a8f55f552e438162edc5bff129f4c02c','DSNSHA256':hashlib.sha256(dsn.encode()).hexdigest(),'BinarySHA256':binding['binary_sha256']}
 folders=[]
 for stage in ('a','b'):
  folder=CONTROL/('executor-'+stage);folder.mkdir();out=PUBLIC/('send-'+stage);out.mkdir();shutil.copyfile(candidate,folder/'executor');shutil.copyfile(CONTROL/'human-confirmation.txt',folder/'human-confirmation.txt');write(folder/'reference.json',ref);write(folder/'reference-topology.json',topology)
  permission={'Enabled':False,'RunID':EXECUTION,'WorkspaceID':ref['Fixture']['WorkspaceID'],'PlanHash':binding['plan_hash'] if 'plan_hash' in binding else 'd7db735bf2ea7a8bc8eefca70ad4fc046be18e7e159e6a5231be3073e550bbfc','DeleteAccess':'refdelete614cd9ad','DeleteSecret':(CONTROL/'delete-secret.private').read_text(encoding='ascii'),'BasisRef':'human-confirmed:reference-disposal:'+sha(CONTROL/'human-confirmation.txt'),'TopologySHA256':sha(folder/'reference-topology.json'),'ConfirmationSHA256':sha(CONTROL/'human-confirmation.txt'),'BinarySHA256':binding['binary_sha256'],'ProfileSHA256':binding['target_digest'],'MaxPerTarget':1,'MaxTotal':2,'ConfirmedAt':confirmed,'ExpiresAt':expires}
  write(folder/'reference-permission.json',permission)
  ident=docker('create','--pull','never','--name',PROJECT+'-reference-send-'+stage,'--label','lifecycle307.run=5d2274ee-a671-4bdc-b406-22b8dad8fd61','--label','lifecycle307.execution='+EXECUTION,'--network','container:'+gwid,'--read-only','--tmpfs','/tmp','--mount',f'type=bind,source={folder},target=/run/private,readonly','--mount',f'type=bind,source={out},target=/run/output','--env','LIFECYCLE_REFERENCE_PROFILE=5d2274ee-614cd9ad','--env','LIFECYCLE_REFERENCE_MODE=send-'+stage,'--entrypoint','/run/private/executor',m.PGIMAGE).decode().strip();folders.append({'stage':stage,'id':ident,'name':PROJECT+'-reference-send-'+stage})
 recovery=CONTROL/'recovery';recovery.mkdir();shutil.copyfile(CONTROL/'bin/recovery',recovery/'recovery');shutil.copyfile(CONTROL/'bin/survivors',recovery/'survivors');shutil.copyfile(PUBLIC/'preflight/native-six-version.json',recovery/'preflight.json');write(recovery/'sql.json',{'PostgresDSN':dsn})
 f=ref['Fixture'];write(recovery/'recovery.json',{'Enabled':False,'SyntheticOnly':True,'RunID':EXECUTION,'WorkspaceID':f['WorkspaceID'],'ProviderInstanceID':f['ProviderInstanceID'],'IncarnationID':f['IncarnationID'],'ActorID':f['ActorID'],'Endpoint':f['Endpoint'],'InspectAccess':f['InspectAccess'],'InspectSecret':f['InspectSecret'],'Token':f['Token'],'ManifestSHA256':sha(recovery/'preflight.json'),'ValidUntil':expires})
 for role,state in [('source-core','running'),('source-objects','running'),('generation02-core','exited'),('restored-objects','exited')]:
  result=json.loads(run(['pwsh','-NoProfile','-File',ROOT/'examples/lifecycle-d1/assert-reference-storage.ps1','-Role',role,'-State',state]));write(PUBLIC/('durable-'+role+'.json'),result)
 write(PUBLIC/'execution-resources.json',{'status':'PREPARED_DISABLED','gateway':{'name':gwname,'id':gwid,'active_config_sha256':sha(gateway/'Caddyfile')},'redis':{'name':redis,'id':redisid,'image':m.REDISIMAGE,'volume':vol},'senders':folders,'authority_confirmed_at':confirmed,'expires_at':expires,'window_seconds':(dt.datetime.fromisoformat(expires.replace('Z','+00:00'))-dt.datetime.fromisoformat(confirmed.replace('Z','+00:00'))).total_seconds(),'source_gateway_config':'unchanged deny-all403; topology verifier requires running sealed service','recovery_binary_sha256':sha(recovery/'recovery'),'survivor_binary_sha256':sha(recovery/'survivors'),'survivor_source_sha256':sha(CONTROL/'survivors/main.go'),'frozen_runtime_source_unchanged':True,'root_payload_mount':False,'delete':0,'cleanup':0})
 print(json.dumps({'status':'RESOURCES_PREPARED_DISABLED','expires_at':expires,'senders':2,'delete':0}))
def activate():
 need(not (PUBLIC/'activation.json').exists() and not (PUBLIC/'send-window-closed.json').exists(),'One-shot activation cannot be reopened')
 resources=read(PUBLIC/'execution-resources.json');need(resources['status']=='PREPARED_DISABLED','Disabled composition required')
 need(dt.datetime.now(dt.timezone.utc)<dt.datetime.fromisoformat(resources['expires_at'].replace('Z','+00:00')),'Expired authority cannot start resources')
 for ident in [resources['gateway']['id'],resources['redis']['id'],m.GW]:docker('start',ident)
 need(docker('exec',resources['redis']['id'],'redis-cli','PING').decode().strip()=='PONG','Recovery Redis unavailable')
 docker('exec',resources['gateway']['id'],'caddy','validate','--config','/etc/caddy/Caddyfile','--adapter','caddyfile')
 for stage in ('a','b'):
  p=CONTROL/('executor-'+stage)/'reference-permission.json';v=read(p);v['Enabled']=True;p.write_text(json.dumps(v,indent=2)+'\n',encoding='utf-8')
 p=CONTROL/'recovery/recovery.json';v=read(p);v['Enabled']=True;p.write_text(json.dumps(v,indent=2)+'\n',encoding='utf-8')
 admin('enable')
 write(PUBLIC/'activation.json',{'status':'ACTIVE_EXACT_TWO_ONLY','at':dt.datetime.now(dt.timezone.utc).isoformat(),'expires_at':resources['expires_at'],'max_per_target':1,'max_total':2,'cleanup':0,'bypass':0,'hold_release':0,'business_reader_producer':'DISABLED_NATIVE_READBACK','original_gateway':'RUNNING_DENY_ALL403_UNCHANGED','exact_delete_gateway_sha256':resources['gateway']['active_config_sha256']})
 print(json.dumps({'status':'ACTIVE_EXACT_TWO_ONLY','expires_at':resources['expires_at'],'delete_so_far':0}))
def mounts():
 r=read(PUBLIC/'execution-resources.json');closures=[]
 for stage in ('a','b'):
  name=PROJECT+'-reference-send-'+stage;old=inspect(name)
  need(old['State']['Status']=='created' and not old['State']['Running'] and len(old['Mounts'])==3,'Only never-started staging mounts may be corrected')
  retained=name+'-staging-rejected01';docker('rename',name,retained)
  folder=CONTROL/('executor-'+stage);out=PUBLIC/('send-'+stage)
  ident=docker('create','--pull','never','--name',name,'--label','lifecycle307.run=5d2274ee-a671-4bdc-b406-22b8dad8fd61','--label','lifecycle307.execution='+EXECUTION,'--network','container:'+r['gateway']['id'],'--read-only','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data','--mount',f'type=bind,source={folder},target=/run/private,readonly','--mount',f'type=bind,source={out},target=/run/output','--env','LIFECYCLE_REFERENCE_PROFILE=5d2274ee-614cd9ad','--env','LIFECYCLE_REFERENCE_MODE=send-'+stage,'--entrypoint','/run/private/executor',m.PGIMAGE).decode().strip()
  current=inspect(ident);need(len(current['Mounts'])==2 and all(x['Type']=='bind' for x in current['Mounts']),'Exact two bind mounts required by frozen host verifier')
  closures.append({'stage':stage,'retained_rejected_id':old['Id'],'retained_rejected_name':retained,'rejected_started':False,'current_id':ident,'current_name':name,'mounts':current['Mounts'],'scratch_tmpfs':current['HostConfig']['Tmpfs'],'delete':0})
 write(PUBLIC/'sender-mount-closure.json',{'status':'PASS_TWO_BIND_MOUNTS','closures':closures,'old_helpers_and_anonymous_volumes':'RETAINED_NEVER_STARTED','cleanup':0})
def send(stage):
 need(stage in ('a','b'),'Exact stage only');need((PUBLIC/'activation.json').exists() and (PUBLIC/'sender-mount-closure.json').exists(),'Activation and mount closure required')
 if stage=='b':need((PUBLIC/'recovery-a/receipt-closure.json').exists(),'First target exact absence must precede second send')
 current=inspect(PROJECT+'-reference-send-'+stage);need(current['State']['Status']=='created','Sender may be started at most once; otherwise recovery only')
 env=dict(os.environ,LIFECYCLE_REFERENCE_PROFILE='5d2274ee-614cd9ad',LIFECYCLE_REFERENCE_MODE='topology-host-'+stage)
 run([CONTROL/'bin/host.exe'],env=env)
 folder=CONTROL/('executor-'+stage);signed=read(folder/'current-topology.json');write(PUBLIC/('send-'+stage)/'signed-host-topology.json',signed)
 docker('start',current['Id']);code=int(docker('wait',current['Id']).decode().strip());after=inspect(current['Id'])
 write(PUBLIC/('send-'+stage)/'sender-exit.json',{'id':current['Id'],'exit':code,'pid':after['State']['Pid'],'status':after['State']['Status'],'attempt_replay':'FORBIDDEN','expected_crash_exit':93,'binary_sha256':sha(folder/'executor')})
 need(code==93,'Send stage did not reach expected observed invocation/crash; no retry and no next target')
 result=read(PUBLIC/('send-'+stage)/('send-'+stage+'-returned.json'));need(result['confirmed_invocation_observed'] is True,'Physical observation must commit before process loss')
 print(json.dumps({'status':'OBSERVED_PHYSICAL_INVOCATION_THEN_EXIT93','stage':stage,'attempt':result['attempt']['ID'],'work':result['work_id'],'response_unknown':result['response_unknown'],'next':'READ_ONLY_RECOVERY_ONLY'}))
def recovery(stage):
 result=read(PUBLIC/('send-'+stage)/('send-'+stage+'-returned.json'));r=read(PUBLIC/'execution-resources.json');folder=CONTROL/'recovery';base=read(folder/'recovery.json');dsn=read(folder/'sql.json')['PostgresDSN'];out=PUBLIC/('recovery-'+stage);out.mkdir()
 work=result['work_id'];need(len(work)==36 and all(c in '0123456789abcdef-' for c in work),'Invalid trusted work ID')
 def step(mode,scenario):
  config={'Mode':mode,'Scenario':scenario,'PostgresDSN':dsn,'RedisAddr':r['redis']['name']+':6379','RecoveryConfig':'/run/private/recovery.json','Manifest':'/run/private/preflight.json','OutputDirectory':'/run/output','Barrier':'','SeedEnabled':False,'CopyRecovery':False,'WorkspaceID':base['WorkspaceID'],'ActorID':base['ActorID'],'CopyID':result['copy']['ID'],'WorkID':work,'AttemptID':result['attempt']['ID'],'ExpiresAt':base['ValidUntil']}
  write(folder/(scenario+'.json'),config)
  return helper(PROJECT+'-reference-'+scenario,folder,out,'recovery','container:'+r['gateway']['id'],['LIFECYCLE_RECOVERY_ACCEPTANCE_CONFIG=/run/private/'+scenario+'.json'],['--tmpfs','/var/lib/postgresql/data'])
 # The retained source has unrelated PENDING business events. Hold row locks
 # on all non-target events while the EXISTING dispatcher uses SKIP LOCKED.
 # No SQL status/claim/receipt writes and no dummy business handler invocation.
 # The exact target goes through its real persisted Outbox obligation/Asynq.
 lock_query="BEGIN; SELECT count(*) FROM (SELECT id FROM outbox_event WHERE NOT (aggregate_id='"+work+"' AND event_type='LifecycleDispositionRecoveryRequested') FOR UPDATE) locked; SELECT 'LOCK_READY';\n"
 process=subprocess.Popen(['docker','exec','-i',m.PG,'psql','-X','-qAt','-v','ON_ERROR_STOP=1','-U','fixture','-d','lifecycle307'],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE,cwd=ROOT)
 try:
  process.stdin.write(lock_query.encode());process.stdin.flush();locked=int(process.stdout.readline().decode().strip());need(process.stdout.readline().decode().strip()=='LOCK_READY','Non-target queue isolation lock unavailable')
  step('outbox',stage+'-outbox')
 finally:
  process.stdin.write(b'ROLLBACK;\n');process.stdin.close();code=process.wait(timeout=15);need(code==0,'Outbox isolation rollback failed')
 write(out/'outbox-isolation.json',{'status':'RELEASED_NO_MUTATION','non_target_rows_locked':locked,'target_work':work,'query':lock_query,'transaction':'ROLLBACK','target_dispatch':'existing SKIP LOCKED dispatcher; actual outbox obligation; no direct queue injection','source_business_event_dispatch':0})
 need(read(out/(stage+'-outbox.json'))['state']=='PUBLISHED','Exact recovery request not published')
 step('worker',stage+'-worker')
 counts_before=json.loads(m.source_read(m.PG,"SELECT json_build_object('storage_calls',(SELECT count(*) FROM lifecycle_storage_call),'attempts',(SELECT count(*) FROM lifecycle_provider_attempt),'claims',(SELECT count(*) FROM lifecycle_provider_send_claim),'receipts',(SELECT count(*) FROM lifecycle_copy_absence_receipt),'completion',(SELECT count(*) FROM lifecycle_plan_completion));"))
 step('once',stage+'-terminal-replay')
 counts_after=json.loads(m.source_read(m.PG,"SELECT json_build_object('storage_calls',(SELECT count(*) FROM lifecycle_storage_call),'attempts',(SELECT count(*) FROM lifecycle_provider_attempt),'claims',(SELECT count(*) FROM lifecycle_provider_send_claim),'receipts',(SELECT count(*) FROM lifecycle_copy_absence_receipt),'completion',(SELECT count(*) FROM lifecycle_plan_completion));"))
 need(counts_before==counts_after,'Terminal replay produced provider I/O or new deletion facts')
 need(counts_after['receipts']==(1 if stage=='a' else 2) and counts_after['completion']==(0 if stage=='a' else 1),'Receipt/completion ordering mismatch')
 write(out/'receipt-closure.json',{'status':'PASS_EXACT_ABSENCE_AND_TERMINAL_REPLAY','stage':stage,'work':work,'attempt':result['attempt']['ID'],'counts':counts_after,'replay_provider_io_delta':0,'native_delete_in_recovery':0})
 print(json.dumps({'status':'PASS_READ_ONLY_RECOVERY','stage':stage,'receipts':counts_after['receipts'],'completion':counts_after['completion'],'terminal_replay_io':0}))
def survivors():
 need((PUBLIC/'recovery-b/receipt-closure.json').exists(),'Two exact absence receipts required before final survivor verification')
 r=read(PUBLIC/'execution-resources.json');out=PUBLIC/'survivors';out.mkdir();helper(PROJECT+'-reference-four-survivors',CONTROL/'recovery',out,'survivors','container:'+r['gateway']['id'],extra=['--tmpfs','/var/lib/postgresql/data'])
 after=m.fingerprint(m.PG);before=read(PUBLIC/'preflight/source-before.json')['relations'];changed=[v['relation'] for v in after if v!=next(x for x in before if x['relation']==v['relation'])]
 allowed={'audit_event','evidence','outbox_event','outbox_event_consumption','cost_event','cost_allocation','workspace_delivery_fence'}
 need(all(n in allowed or n.startswith('lifecycle_') for n in changed),'Non-lifecycle business facts changed')
 write(PUBLIC/'source-after.json',{'at':dt.datetime.now(dt.timezone.utc).isoformat(),'relations':after,'changed_relations':changed,'old_business_relations_unchanged':True,'old_business_row_rewrites':0})
 print(json.dumps({'status':'PASS_FOUR_SURVIVORS_AND_BUSINESS_FACTS','changed_relations':changed,'cleanup':0}))
def close_send():
 for stage in ('a','b'):
  path=CONTROL/('executor-'+stage)/'reference-permission.json';v=read(path);v['Enabled']=False;path.write_text(json.dumps(v,indent=2)+'\n',encoding='utf-8')
 admin('disable')
 write(PUBLIC/'send-window-closed.json',{'at':dt.datetime.now(dt.timezone.utc).isoformat(),'reason':'First physical invocation is CONFIRMED/DENIED; source version still present; no resend; second target never started','sender_permissions':False,'executor_iam':'DISABLED_NATIVE_READBACK','native_delete_success':0,'attempted_targets':1,'max_per_target_consumed':{'a':1,'b':0}})
def failure_readback():
 need((PUBLIC/'send-window-closed.json').exists(),'Delete authority must close before bounded failure readback')
 folder=CONTROL/'failure-native';folder.mkdir();out=PUBLIC/'failure-native';out.mkdir();payload=CONTROL/'failure-payload';payload.mkdir();candidate,binding=m.frozen_candidate();shutil.copyfile(candidate,folder/'executor')
 ref=read(SOURCE/'reference-disposal-preparation01/native-after/reference.json');ref['PreparationEnabled']=True;ref['PreparationUntil']='2026-10-15T09:12:00Z';write(folder/'reference.json',ref);write(folder/'reference-permission.json',{'Enabled':False})
 try:
  helper(PROJECT+'-reference-failure-six-retained',folder,out,'executor','container:'+m.MINIO,['LIFECYCLE_REFERENCE_PROFILE=5d2274ee-614cd9ad','LIFECYCLE_REFERENCE_MODE=inspect-source'],['--tmpfs','/var/lib/postgresql/data','--mount',f'type=bind,source={payload},target=/run/payload'])
  m.six_equal(read(out/'native-six-version.json'),read(PUBLIC/'preflight/native-six-version.json'))
  write(out/'six-retained-verification.json',{'status':'PASS_ALL_SIX_EXACT_VERSIONS_RETAINED','targets_present':2,'survivors_present':4,'bytes_hash_protection':'MATCH_PREDELETE_PREFLIGHT','successful_delete':0,'second_target_send':0,'provider_delete_in_readback':0})
 finally:
  ref['PreparationEnabled']=False;(folder/'reference.json').write_text(json.dumps(ref,indent=2)+'\n',encoding='utf-8')
 print(json.dumps({'status':'PASS_SIX_RETAINED','targets_present':2,'survivors_present':4,'delete_success':0}))
def resolve_denied():
 need(read(PUBLIC/'failure-native/six-retained-verification.json')['targets_present']==2,'Positive exact retained proof required')
 r=read(PUBLIC/'execution-resources.json');folder=CONTROL/'recovery';out=PUBLIC/'denied-resolution';out.mkdir();shutil.copyfile(CONTROL/'bin/resolve-denied',folder/'resolve-denied')
 result=read(PUBLIC/'send-a/send-a-returned.json');write(folder/'resolution.json',{'PostgresDSN':read(folder/'sql.json')['PostgresDSN'],'AttemptID':result['attempt']['ID'],'WorkID':result['work_id']})
 write(out/'build.json',{'source_sha256':sha(CONTROL/'resolve-denied/main.go'),'binary_sha256':sha(folder/'resolve-denied'),'runtime_source_commit':SOURCE_SHA,'deleter':False,'command':'existing ExactExecution.ReconcileDeniedAttempt','original_facts':'append-only; no rewrite; no absence/completion'})
 helper(PROJECT+'-reference-denied-resolution',folder,out,'resolve-denied','container:'+r['gateway']['id'],extra=['--tmpfs','/var/lib/postgresql/data'])
 original=read(folder/'a-worker.json');original['Mode']='once';original['Scenario']='a-denied-terminal';write(folder/'a-denied-terminal.json',original)
 helper(PROJECT+'-reference-denied-terminal',folder,out,'recovery','container:'+r['gateway']['id'],['LIFECYCLE_RECOVERY_ACCEPTANCE_CONFIG=/run/private/a-denied-terminal.json'],['--tmpfs','/var/lib/postgresql/data'])
 before=m.source_read(m.PG,"SELECT json_build_object('calls',(SELECT count(*) FROM lifecycle_storage_call),'claims',(SELECT count(*) FROM lifecycle_provider_send_claim),'receipts',(SELECT count(*) FROM lifecycle_copy_absence_receipt),'completion',(SELECT count(*) FROM lifecycle_plan_completion));")
 original['Scenario']='a-denied-terminal-replay';write(folder/'a-denied-terminal-replay.json',original)
 helper(PROJECT+'-reference-denied-terminal-replay',folder,out,'recovery','container:'+r['gateway']['id'],['LIFECYCLE_RECOVERY_ACCEPTANCE_CONFIG=/run/private/a-denied-terminal-replay.json'],['--tmpfs','/var/lib/postgresql/data'])
 after=m.source_read(m.PG,"SELECT json_build_object('calls',(SELECT count(*) FROM lifecycle_storage_call),'claims',(SELECT count(*) FROM lifecycle_provider_send_claim),'receipts',(SELECT count(*) FROM lifecycle_copy_absence_receipt),'completion',(SELECT count(*) FROM lifecycle_plan_completion));");need(before==after,'Terminal replay created provider I/O')
 write(out/'terminal-replay.json',{'status':'PASS_DENIED_PRESENT_TERMINAL_REPLAY_ZERO_IO','counts':json.loads(after),'replay_provider_io':0,'delete_success':0,'absence_claimed':False,'second_target_started':False})
 print(json.dumps({'status':'PASS_DENIED_PRESENT_RESOLVED','replay_provider_io':0,'delete_success':0,'second_target_started':False}))
def seal():
 failures=[];states={}
 for stage in ('a','b'):
  path=CONTROL/('executor-'+stage)/'reference-permission.json'
  if path.exists():v=read(path);v['Enabled']=False;path.write_text(json.dumps(v,indent=2)+'\n',encoding='utf-8')
 path=CONTROL/'recovery/recovery.json'
 if path.exists():v=read(path);v['Enabled']=False;path.write_text(json.dumps(v,indent=2)+'\n',encoding='utf-8')
 try:
  if not (PUBLIC/'admin-disable/admin-result.json').exists():admin('disable')
 except Exception:failures.append('IAM_DISABLE_FAILED')
 r=read(PUBLIC/'execution-resources.json')
 for name in [*[PROJECT+'-reference-send-'+s for s in ('a','b')],r['gateway']['id'],r['redis']['id'],m.GW,m.MINIO,m.PG]:
  try:
   item=inspect(name)
   if item['State']['Running']:docker('stop',item['Id'])
   current=inspect(item['Id']);need(not current['State']['Running'] and current['State']['Pid']==0,'Resource remains active');states[current['Name'].lstrip('/')]={'id':current['Id'],'status':current['State']['Status'],'pid':current['State']['Pid']}
  except Exception:failures.append(str(name))
 gateway=CONTROL/'gateway/Caddyfile';shutil.copyfile(m.PRIVATE/'d1-lc13-continuation01/delete-candidate04/output/gateway-sealed.Caddyfile',gateway);need(sha(gateway)=='6a08b8647cc63cb389e2a511a5ad944b82d14e1ec9721f1a5ae45cb984bed4f6','Sealed gateway hash drift')
 backup={p.relative_to(SOURCE).as_posix():sha(p) for p in [SOURCE/'reference-disposal-preparation01/native-snapshot/native-store.tar',SOURCE/'predelete-core-checkpoint-04/core.dump']}
 need(backup=={'reference-disposal-preparation01/native-snapshot/native-store.tar':'f72efeab2c49802be1661a431164167d7a34968fef877163f08b25878d9554cc','predelete-core-checkpoint-04/core.dump':'19dba12b9992fdd11c237d6f0df489c7a961696acd54b8a3403b941e7005c98a'},'Retained backup changed')
 ids=docker('ps','-aq','--filter','label=lifecycle307.run=5d2274ee-a671-4bdc-b406-22b8dad8fd61').decode().split();items=json.loads(docker('inspect',*ids));need(all(not i['State']['Running'] and i['State']['Pid']==0 and not any(i['NetworkSettings']['Ports'].values()) for i in items),'Project has active/exposed resource')
 write(PUBLIC/'sealed-final-state.json',{'status':'PASS_STOPPED_RETAINED' if not failures else 'SEAL_FAILURE','at':dt.datetime.now(dt.timezone.utc).isoformat(),'states':states,'retained_project_containers':len(items),'containers':[{'name':i['Name'].lstrip('/'),'id':i['Id'],'image':i['Image'],'state':i['State']['Status'],'pid':i['State']['Pid']} for i in items],'backups_sha256':backup,'sealed_gateway_sha256':sha(gateway),'permission_enabled':False,'recovery_enabled':False,'executor_iam':'DISABLED_NATIVE_READBACK' if 'IAM_DISABLE_FAILED' not in failures else 'UNKNOWN','cleanup':0,'hold_release':0,'bypass':0,'failures':failures})
 need(not failures,'Independent seal failure; all resources retained')
 print(json.dumps({'status':'PASS_SEALED_RETAINED','retained_containers':len(items),'cleanup':0}))
if __name__=='__main__':
 phase=sys.argv[1]
 if phase in ('provision','inspect','inspect02','attach','enable','disable'):admin(phase)
 elif phase in ('send-a','send-b'):send(phase[-1])
 elif phase in ('recovery-a','recovery-b'):recovery(phase[-1])
 else:{'prepare':prepare,'preflight':preflight,'resources':resources,'mounts':mounts,'activate':activate,'survivors':survivors,'close-send':close_send,'failure-readback':failure_readback,'resolve-denied':resolve_denied,'seal':seal}[phase]()
