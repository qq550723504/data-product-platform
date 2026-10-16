"""Fixed non-delete LC13 and retained 5d read-only checkpoint. No approval switch."""
import argparse, datetime as dt, hashlib, json, secrets, shutil, subprocess, time
from pathlib import Path
from urllib.parse import urlsplit
ROOT=Path(__file__).resolve().parents[2]
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private')
CONTROL=PRIVATE/'d1-lc13-readonly01'
PUBLIC=ROOT/'docs/acceptance/evidence/issue307-d1/lc13-readonly01'
SOURCE=PRIVATE/'dpp-lifecycle307-5d2274ee'
PG='c85b9ff408ed110f531ac79d27a3b99f66242d12766e1674e3de78067d140bd0'
MINIO='0a344aa06c7bc9c59e5b5ffe8d8d5584e22abf1daeb8b28c549a9735f5b6bf65'
GW='ba90451138af2c980f2ee77b58bfe2908ef67e06b1e03c96fd1a4e0836400af4'
PGIMAGE='sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea'
MIIMAGE='sha256:72476ee7f52a538e561627f7d8746e20c15d50c13e8bcdc58b22fa65cff94df6'
REDISIMAGE='sha256:858f009f9709ce576febc734aa78b8f6d624b82571f9ddb6bda4377c833b3499'
PHASES=('prepare','setup','crash','orphan','recovery','final','seal','5d-readonly','5d-readonly-02','5d-readonly-03','5d-readonly-04')
INSPECT_SCRIPT=b'#!/bin/sh\nset -eu\nexport LIFECYCLE_REFERENCE_PROFILE=5d2274ee-614cd9ad\nexport LIFECYCLE_REFERENCE_MODE=inspect-source\nexec /run/private/executor\n'
def require(ok,message):
 if not ok: raise RuntimeError(message)
def load(p): return json.loads(p.read_text(encoding='utf-8-sig'))
def write(p,v):
 with p.open('x',encoding='utf-8',newline='\n') as f: json.dump(v,f,indent=2);f.write('\n')
def digest(p): return hashlib.sha256(p.read_bytes()).hexdigest()
def call(args,data=None):
 r=subprocess.run(args,input=data,capture_output=True,cwd=ROOT)
 if r.returncode:
  if CONTROL.exists(): (CONTROL/('failure-'+dt.datetime.now(dt.timezone.utc).strftime('%Y%m%dT%H%M%S%f')+'.private-log')).write_bytes(r.stdout+b'\n'+r.stderr)
  raise RuntimeError('Controlled command failed; private diagnostics retained; no automatic retry')
 return r.stdout
def docker(*args,data=None): return call(['docker',*map(str,args)],data)
def inspect(name): return json.loads(docker('inspect',name))[0]
def ps(name,*args): return call(['pwsh','-NoProfile','-File',str(ROOT/'examples/lifecycle-d1'/name),*map(str,args)])
def fresh():
 x=load(CONTROL/'plane.json');p=Path(x['private']);r=load(p/'resources.json')
 for name,fact in x['services'].items():
  i=inspect(name);require(i['Id']==fact['id'] and i['Image']==fact['image'] and i['State']['Running'] and not any(i['NetworkSettings']['Ports'].values()),'Fresh native service drift')
 return p,r
def prepare():
 CONTROL.mkdir(exist_ok=True);PUBLIC.mkdir(exist_ok=True)
 require(not (CONTROL/'plane.json').exists(),'LC13 already allocated; use retained generation')
 before=set(PRIVATE.glob('dpp-lifecycle307-*'))
 ps('prepare.ps1','-PrivateRoot',PRIVATE,'-ArchiveAcceptance','-ProcessingDeliveryAcceptance','-InternalSubnet','10.234.158.0/24','-MinioImage',MIIMAGE)
 created=set(PRIVATE.glob('dpp-lifecycle307-*'))-before;require(len(created)==1,'Fresh allocation ambiguous');p=created.pop();r=load(p/'resources.json')
 ps('prepare-plane.ps1','-PrivateDirectory',p)
 redis=r['project']+'-redis';vol=redis+'-data'
 docker('volume','create','--label','lifecycle307.run='+r['run_id'],vol)
 docker('create','--pull','never','--name',redis,'--label','lifecycle307.run='+r['run_id'],'--network',r['network'],'--mount','type=volume,source='+vol+',target=/data',REDISIMAGE,'redis-server','--appendonly','yes')
 docker('start',redis)
 owner=p/'plane-owner/reader.json';cfg=load(owner);cfg['RedisAddr']=redis+':6379';owner.write_text(json.dumps(cfg,indent=2)+'\n',encoding='utf-8')
 services={name:{'id':inspect(name)['Id'],'image':inspect(name)['Image']} for name in [*r['containers'].values(),redis]}
 write(CONTROL/'plane.json',{'private':str(p),'services':services})
 write(PUBLIC/'plane-environment.json',{'run':r['run_id'],'resources':r,'services':services,'redis_volume':vol,'source_head':call(['git','rev-parse','HEAD']).decode().strip(),'binary_sha256':digest(p/'plane-owner/reader'),'schema':68,'internal':True,'host_ports':[],'delete':0})
def plane(mode):
 p,_=fresh();ps('run-plane-step.ps1','-PrivateDirectory',p,'-Mode',mode)
def source_read(container,query):
 # Every source SQL statement is bracketed by a real READ ONLY transaction.
 q='BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY; SET LOCAL timezone=\'UTC\';\n'+query+'\nROLLBACK;'
 return docker('exec','-i',container,'psql','-X','-qAt','-v','ON_ERROR_STOP=1','-U','fixture','-d','lifecycle307',data=q.encode()).decode().strip()
def fingerprint(container):
 names=source_read(container,"SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE' ORDER BY table_name;").splitlines()
 require(len(names)==194 and all(n.replace('_','').isalnum() for n in names),'Unexpected source relation contract')
 queries=["SELECT json_build_object('relation','"+n+"','rows',COALESCE((SELECT jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text) FROM public.\""+n+"\" t),'[]'::jsonb))::text" for n in names]
 raw=source_read(container,' UNION ALL '.join(queries)+';')
 facts=[json.loads(x) for x in raw.splitlines()]
 return [{'relation':x['relation'],'rows':len(x['rows']),'sha256':hashlib.sha256(json.dumps(x['rows'],sort_keys=True,separators=(',',':')).encode()).hexdigest()} for x in facts]
def role_acl(container):
 role=source_read(container,"SELECT (to_jsonb(r)-'oid')::text FROM pg_authid r WHERE rolname='fixture';")
 acl=source_read(container,"SELECT coalesce(jsonb_agg(jsonb_build_object('name',c.relname,'kind',c.relkind,'owner',pg_get_userbyid(c.relowner),'acl',c.relacl) ORDER BY c.relname),'[]')::text FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public';")
 return {'role_sha256':hashlib.sha256(role.encode()).hexdigest(),'acl_sha256':hashlib.sha256(acl.encode()).hexdigest()}
def validate_source():
 r=load(SOURCE/'resources.json');cfg=load(SOURCE/'config.json')
 require(r['run_id']=='5d2274ee-a671-4bdc-b406-22b8dad8fd61' and cfg['WorkspaceID']=='89eba3fd-9763-4f66-8cec-1a779104f155' and not cfg['Enabled'],'Source identity/window drift')
 for name,ident,image,volume,dest in [('postgres',PG,PGIMAGE,r['project']+'-postgres','/var/lib/postgresql/data'),('minio',MINIO,MIIMAGE,r['project']+'-objects','/data')]:
  i=inspect(ident);require(i['Id']==ident and i['Image']==image and i['State']['Status']=='exited' and i['Config']['Labels']['lifecycle307.run']==r['run_id'] and not any(i['NetworkSettings']['Ports'].values()),'Source retained service drift')
  require(any(m['Type']=='volume' and m['Name']==volume and m['Destination']==dest and m['RW'] for m in i['Mounts']),'Source durable mount drift')
 g=inspect(GW);require(g['Id']==GW and g['State']['Status']=='exited','Source gateway must remain stopped')
 require(digest(SOURCE/'Caddyfile')=='b301fc4fe59f64ebe1e11f1488dde521b774622641ccc8377fe41cb64e839e80','Source gateway seal drift')
 net=json.loads(docker('network','inspect',r['network']))[0];require(net['Internal'] and net['Id']=='33f303a57406f05c4e78b3cdb1355059a8f55f552e438162edc5bff129f4c02c','Source closed network drift')
 return r,cfg
def binary_dump(args,path):
 with path.open('xb') as f:
  r=subprocess.run(['docker',*args],stdout=f,stderr=subprocess.PIPE,cwd=ROOT)
 if r.returncode:
  path.with_suffix('.private-log').write_bytes(r.stderr);raise RuntimeError('Read-only dump failed; retained')
def six_equal(current,previous):
 def semantic(proof):
  require(proof['provider_delete']==0 and len(proof['versions'])==6,'Invalid six-version proof')
  return sorted([{'copy':v['copy'],'bytes':v['bytes'],'sha256':v['sha256'],'retention':v['protection']['RetentionMode'],'until':v['protection']['RetainUntil'],'hold':v['protection']['LegalHold']} for v in proof['versions']],key=lambda x:x['copy']['ID'])
 require(semantic(current)==semantic(previous),'Native bytes/version/protection drift')
def shadow_config(original,audit,project):
 ref=json.loads(json.dumps(original));f=ref['Fixture'];u=urlsplit(f['PostgresDSN'])
 require(u.scheme=='postgres' and u.hostname==project+'-postgres' and u.port==5432 and u.path=='/lifecycle307' and u.username=='fixture' and u.query=='sslmode=disable' and u.password and u.fragment=='','Unexpected source DSN contract')
 require(audit in {project+'-readonly-audit-core-'+n for n in ('01','02','03','04','05','06','07','08','09','10','11','12','13')},'Audit hostname outside finite generation')
 for k,v in {'RunID':'5d2274ee-a671-4bdc-b406-22b8dad8fd61','WorkspaceID':'89eba3fd-9763-4f66-8cec-1a779104f155','ProviderInstanceID':'1980751b-c50e-4895-93a8-cb55960cd635','IncarnationID':'be784eda-96e4-49a9-ae47-eaa3203a2dc5','RootAccess':'','RootSecret':'','Endpoint':'127.0.0.1:9000'}.items():require(f[k]==v,'Reference reader identity/root drift')
 f['PostgresDSN']=f['PostgresDSN'].replace(project+'-postgres:5432',audit+':5432')
 require(urlsplit(f['PostgresDSN']).hostname==audit,'Audit DSN substitution failed')
 f['OutputDirectory']='/run/output';ref['PreparationEnabled']=True;ref['PreparationUntil']=(dt.datetime.now(dt.timezone.utc)+dt.timedelta(minutes=40)).isoformat();ref['AttestationPublicKey']=''
 return ref

def frozen_candidate():
 binding=load(ROOT/'docs/acceptance/evidence/issue307-d1/lc13-continuation01/delete-candidate04/source-binding.json')
 candidate=PRIVATE/'d1-lc13-continuation01/delete-candidate04/executor'
 require(binding['source_commit']=='055aa1df063e560c6796e92dfff4e03c7b9061e0' and binding['binary_sha256']=='01bff6e2a242a202e73519d7b5a4f1fc109430614b25a5189eacfb0ebb2cddf3' and digest(candidate)==binding['binary_sha256'],'Frozen candidate identity/binary drift')
 require(all(digest(ROOT/f)==h for f,h in binding['runtime_source_hashes'].items()),'Frozen candidate runtime source drift')
 rendered=candidate.parent/'output'
 for name,field in [('bounded-profile.json','bounded_profile_sha256'),('delete-executor-policy.json','iam_policy_sha256'),('gateway-active.Caddyfile','active_gateway_sha256'),('gateway-sealed.Caddyfile','sealed_gateway_sha256')]:
  require(digest(rendered/name)==binding[field],'Frozen candidate profile/IAM/gateway drift')
 return candidate,binding
def restore_stopped_state(helper_id,audit_id,out):
 results={};failures=[]
 # Each exact known resource gets an independent attempt. A helper which failed
 # to create has no ID and cannot prevent restoration of either source service.
 for role,ident in [('helper',helper_id),('audit_core',audit_id),('source_minio',MINIO),('source_pg',PG)]:
  if not ident:results[role]='NOT_CREATED';continue
  try:
   i=inspect(ident)
   if i['State']['Running']:docker('stop',ident)
   results[role]=inspect(ident)['State']['Status']
   require(results[role]=='exited','Resource did not return stopped')
  except Exception:
   results[role]='RESTORE_FAILED';failures.append(role)
 try:
  results['source_gateway']=inspect(GW)['State']['Status'];require(results['source_gateway']=='exited','Source gateway unexpectedly started')
 except Exception:results['source_gateway']='VERIFY_FAILED';failures.append('source_gateway')
 write(out/'retained-final-state.json',{'states':results,'failures':failures,'cleanup':0})
 require(not failures,'State restoration failed after all independent stop attempts; diagnostics retained')
def wait_audit_core(audit):
 # Temporary bootstrap listens on Unix sockets only in the pinned official image.
 # TCP readiness waits for the final server after bootstrap shutdown.
 for _ in range(60):
  test=subprocess.run(['docker','exec',audit,'pg_isready','-h','127.0.0.1','-p','5432','-U','checkpoint_admin','-d','postgres'],capture_output=True)
  if test.returncode==0:return
  time.sleep(1)
 raise RuntimeError('New audit Core final TCP server not ready')

def five_d(generation=1):
 CONTROL.mkdir(exist_ok=True);PUBLIC.mkdir(exist_ok=True);r,cfg=validate_source()
 require(generation in (1,2,3,4,5,6,7,8,9,10,11,12,13),'Unreviewed generation')
 candidate,binding=frozen_candidate() if generation>=4 else (None,None)
 suffix='' if generation==1 else '-'+str(generation).zfill(2)
 stage=CONTROL/('5d-readonly'+suffix);stage.mkdir();out=PUBLIC/('5d'+suffix);out.mkdir()
 old=SOURCE/'reference-disposal-preparation01'
 backups=[old/'native-snapshot/native-store.tar',SOURCE/'predelete-core-checkpoint-04/core.dump']
 require(all(p.is_file() for p in backups),'Retained backup missing')
 backup_before={str(p.relative_to(SOURCE)):digest(p) for p in backups}
 require(backup_before[str(backups[0].relative_to(SOURCE))]=='f72efeab2c49802be1661a431164167d7a34968fef877163f08b25878d9554cc' and backup_before[str(backups[1].relative_to(SOURCE))]=='19dba12b9992fdd11c237d6f0df489c7a961696acd54b8a3403b941e7005c98a','Historical backup hash mismatch')
 sealed=[SOURCE/'config.json',*SOURCE.rglob('reader.json'),*SOURCE.rglob('reference-permission.json')]
 require(all(load(p).get('Enabled') is False for p in sealed),'Source business/deletion window open')
 private_seals={str(p.relative_to(SOURCE)):digest(p) for p in [SOURCE/'Caddyfile',*sealed]}
 audit=r['project']+'-readonly-audit-core-'+str(generation).zfill(2);audit_id=None;helper=None
 try:
  docker('start',PG);docker('start',MINIO)
  for _ in range(60):
   try: before=fingerprint(PG);break
   except RuntimeError: time.sleep(1)
  else: raise RuntimeError('Source Core not ready')
  require(sum(x['rows'] for x in before)==1879,'Source Core fact count drift')
  roles=role_acl(PG);write(out/'source-before.json',{'relations':before,'rows':1879,'roles_acl':roles,'at':dt.datetime.now(dt.timezone.utc).isoformat()})
  binary_dump(['exec',PG,'pg_dump','-U','fixture','-d','lifecycle307','--format=custom','--create'],stage/'core.dump')
  binary_dump(['exec',PG,'pg_dumpall','-U','fixture','--globals-only'],stage/'globals.sql')
  (stage/'audit-password').write_text(secrets.token_hex(32),encoding='ascii')
  volume=audit+'-data';docker('volume','create','--label','lifecycle307.run='+r['run_id'],volume)
  audit_id=docker('create','--pull','never','--name',audit,'--label','lifecycle307.run='+r['run_id'],'--network',r['network'],'--mount','type=volume,source='+volume+',target=/var/lib/postgresql/data','--mount',f'type=bind,source={stage},target=/run/backup,readonly','-e','POSTGRES_USER=checkpoint_admin','-e','POSTGRES_DB=postgres','-e','POSTGRES_PASSWORD_FILE=/run/backup/audit-password','-e','POSTGRES_HOST_AUTH_METHOD=scram-sha-256',PGIMAGE).decode().strip();docker('start',audit)
  wait_audit_core(audit)
  docker('exec',audit,'psql','-X','-U','checkpoint_admin','-d','postgres','-v','ON_ERROR_STOP=1','-f','/run/backup/globals.sql')
  docker('exec',audit,'pg_restore','--exit-on-error','--create','-U','checkpoint_admin','-d','postgres','/run/backup/core.dump')
  restored=fingerprint(audit);restored_roles=role_acl(audit);require(restored==before and restored_roles==roles,'New audit Core restore differs')
  write(out/'audit-restored.json',{'relations':restored,'rows':1879,'roles_acl':restored_roles,'container':audit,'id':audit_id,'image':PGIMAGE,'volume':volume,'source_export_sha256':digest(stage/'core.dump')})
  # Only the root-free reference reader gets this new audit DSN. The source
  # gateway, source business configs and old restore database are untouched.
  read=stage/'native';read.mkdir();payload=stage/'payload';payload.mkdir();shutil.copyfile(candidate if candidate else old/'bin02/executor',read/'executor')
  require(digest(read/'executor')==(binding['binary_sha256'] if binding else 'e1d9d35e48210ffe4400edb3476bdc8a51cb71cab9bd05686aedbc791a0a5b30'),'Fixed inspection binary drift')
  if binding:write(out/'candidate-binding.json',{'candidate':binding,'operator_head':call(['git','rev-parse','HEAD']).decode().strip(),'driver_sha256':digest(Path(__file__)),'mode':'inspect-source','root_mounted':False,'delete_permission_enabled':False})
  ref=shadow_config(load(old/'native-after/reference.json'),audit,r['project'])
  write(read/'reference.json',ref);write(read/'reference-permission.json',{'Enabled':False})
  (read/'inspect.sh').write_bytes(INSPECT_SCRIPT)
  helper=docker('create','--pull','never','--name',audit+'-native-inspect','--label','lifecycle307.run='+r['run_id'],'--network','container:'+MINIO,'--read-only','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data','--mount',f'type=bind,source={read},target=/run/private,readonly','--mount',f'type=bind,source={payload},target=/run/payload','--mount',f'type=bind,source={out},target=/run/output','--entrypoint','/bin/sh',PGIMAGE,'/run/private/inspect.sh').decode().strip();docker('start',helper)
  code=docker('wait',helper).decode().strip();write(out/'native-helper.json',{'id':inspect(helper)['Id'],'exit':int(code),'binary_sha256':digest(read/'executor'),'root_mounted':False,'source_gateway_started':False,'journal_database':audit,'native_delete':0});require(code=='0','Native read-only helper failed; retained')
  after=fingerprint(PG);require(after==before and role_acl(PG)==roles,'Source facts changed during read-only inspection')
  current=load(out/'native-six-version.json');require(current['provider_delete']==0 and len(current['versions'])==6,'Native six-version proof invalid')
  six_equal(current,load(ROOT/'docs/acceptance/evidence/issue307-d1/reference-disposal-preparation01/native-after/native-six-version.json'))
  write(out/'source-after.json',{'relations':after,'rows':1879,'roles_acl':roles,'same_as_before':True,'at':dt.datetime.now(dt.timezone.utc).isoformat()})
  costs=json.loads(source_read(audit,"SELECT json_build_object('calls',(SELECT count(*) FROM lifecycle_storage_call),'observations',(SELECT count(*) FROM lifecycle_storage_observation),'costs',(SELECT count(*) FROM cost_allocation WHERE lifecycle_storage_call_id IS NOT NULL),'unaccounted',(SELECT count(*) FROM lifecycle_storage_observation o WHERE o.invoked_at IS NOT NULL AND (SELECT count(*) FROM cost_allocation a JOIN cost_event e ON e.id=a.cost_event_id WHERE a.lifecycle_storage_call_id=o.call_id AND e.activity_id=o.call_id AND e.quantity=1 AND e.unit='invocation')<>1),'send_claims',(SELECT count(*) FROM lifecycle_provider_send_claim),'absence',(SELECT count(*) FROM lifecycle_copy_absence_receipt),'completion',(SELECT count(*) FROM lifecycle_plan_completion));"))
  require(costs['calls']==costs['observations']==costs['costs']==112 and costs['unaccounted']==costs['send_claims']==costs['absence']==costs['completion']==0,'Audit-only native accounting failed')
  write(out/'audit-costs.json',costs)
  require(backup_before=={str(p.relative_to(SOURCE)):digest(p) for p in backups},'Retained backup bytes changed')
  require(private_seals=={p:digest(SOURCE/p) for p in private_seals},'Source sealed config changed')
  write(out/'readonly-checkpoint.json',{'status':'PASS','source_run':r['run_id'],'source_pg':PG,'source_minio':MINIO,'source_gateway':GW,'source_relations':194,'source_rows':1879,'source_business_mutations':0,'audit_core':audit,'audit_native_call_delta':20,'backups':backup_before,'approval':'NOT_RECEIVED','native_delete':0,'delete_iam':'NOT_ENABLED','old_restore_touched':False})
 finally:
  try:
   if generation>=4:
    # Permanently close only this new maintenance configuration; retain its pre-seal bytes.
    native=stage/'native/reference.json'
    if native.exists():
     write(stage/'reference-before-seal.private.json',load(native));sealed_ref=load(native);sealed_ref['PreparationEnabled']=False;native.write_text(json.dumps(sealed_ref,indent=2)+'\n',encoding='utf-8',newline='\n')
  finally:
   # A failed seal cannot skip independent restoration of either source service.
   restore_stopped_state(helper,audit_id,out)
def main():
 global CONTROL,PUBLIC
 parser=argparse.ArgumentParser();parser.add_argument('phase',choices=PHASES);a=parser.parse_args()
 if a.phase=='prepare': prepare()
 elif a.phase=='5d-readonly': five_d()
 elif a.phase=='5d-readonly-02': five_d(2)
 elif a.phase=='5d-readonly-03': five_d(3)
 elif a.phase=='5d-readonly-04':
  CONTROL=PRIVATE/'d1-delete-readonly-candidate04';PUBLIC=ROOT/'docs/acceptance/evidence/issue307-d1/delete-readonly-candidate04';five_d(4)
 elif a.phase=='crash': plane('lc13-crash-one');plane('lc13-crash-two')
 elif a.phase=='setup': plane('setup');plane('lc13-init')
 elif a.phase=='orphan': plane('lc13-orphan')
 elif a.phase=='recovery': plane('lc13-lease-recovery')
 elif a.phase=='final': plane('lc13-final')
 elif a.phase=='seal':
  plane('seal');p,r=fresh()
  for name in ('plane-owner','plane-admin'):
   f=p/name/'reader.json';c=load(f);c['Enabled']=False;f.write_text(json.dumps(c,indent=2)+'\n',encoding='utf-8')
  (p/'Caddyfile').write_text(':9000 {\n respond "sealed" 403\n}\n',encoding='ascii');docker('restart',r['containers']['gateway'])
 print('Controlled '+a.phase+' completed; native DELETE=0, cleanup=0')
if __name__=='__main__': main()
