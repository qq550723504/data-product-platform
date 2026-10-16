"""Finite fresh LC13 continuation. No DELETE, merge, cleanup or old environment writes."""
import argparse, base64, datetime as dt, hashlib, json, secrets, shutil, time
from pathlib import Path
import importlib.util
spec=importlib.util.spec_from_file_location('lc13_readonly',Path(__file__).with_name('complete-lc13-readonly.py'))
b=importlib.util.module_from_spec(spec);spec.loader.exec_module(b)
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives import serialization

CONTROL=b.PRIVATE/'d1-lc13-continuation01'
PUBLIC=b.ROOT/'docs/acceptance/evidence/issue307-d1/lc13-continuation01'
SUBNET='10.234.159.0/24'
b.CONTROL=CONTROL
b.PUBLIC=PUBLIC
load,write,require,digest,call,docker,inspect,ps=b.load,b.write,b.require,b.digest,b.call,b.docker,b.inspect,b.ps

def save(p,v): p.write_text(json.dumps(v,indent=2)+'\n',encoding='utf-8',newline='\n')
def fresh(): return b.fresh()
def step(mode,role=''):
 p,_=fresh();args=['-PrivateDirectory',p,'-Mode',mode]
 if role:args+=['-RoleDirectory',role]
 return ps('run-plane-step.ps1',*args)
def output():
 p,r=fresh();return b.ROOT/'examples/lifecycle-d1/results'/r['run_id']
def prepare():
 CONTROL.mkdir();PUBLIC.mkdir();before=set(b.PRIVATE.glob('dpp-lifecycle307-*'))
 ps('prepare.ps1','-PrivateRoot',b.PRIVATE,'-ArchiveAcceptance','-ProcessingDeliveryAcceptance','-InternalSubnet',SUBNET,'-MinioImage',b.MIIMAGE)
 created=set(b.PRIVATE.glob('dpp-lifecycle307-*'))-before;require(len(created)==1,'Fresh allocation ambiguous');p=created.pop();r=load(p/'resources.json')
 ps('prepare-plane.ps1','-PrivateDirectory',p)
 redis=r['project']+'-redis';vol=redis+'-data'
 docker('volume','create','--label','lifecycle307.run='+r['run_id'],vol)
 docker('create','--pull','never','--name',redis,'--label','lifecycle307.run='+r['run_id'],'--network',r['network'],'--mount','type=volume,source='+vol+',target=/data',b.REDISIMAGE,'redis-server','--appendonly','yes');docker('start',redis)
 for name in ['plane-owner','plane-admin']:
  cfg=load(p/name/'reader.json');cfg['RedisAddr']=redis+':6379';cfg['LC13Continuation']=True;save(p/name/'reader.json',cfg)
 services={name:{'id':inspect(name)['Id'],'image':inspect(name)['Image']} for name in [*r['containers'].values(),redis]}
 write(CONTROL/'plane.json',{'private':str(p),'services':services})
 binding={str(path.relative_to(b.ROOT)).replace('\\','/'):digest(path) for path in sorted((b.ROOT/'apps/platform').rglob('*.go'))}
 for name in ['go.mod','go.sum']:binding['apps/platform/'+name]=digest(b.ROOT/'apps/platform'/name)
 write(PUBLIC/'environment.json',{'run':r['run_id'],'resources':r,'services':services,'redis_volume':vol,'source_head':call(['git','rev-parse','HEAD']).decode().strip(),'binary_sha256':digest(p/'plane-owner/reader'),'schema':68,'internal':True,'host_ports':[],'delete':0,'source_hashes':binding})
def setup(): step('setup');step('resume-init');step('resume-crash');step('resume-membership')
def credential_setup():
 step('setup');step('resume-init');p,r=fresh()
 for name,source in [('credential-owner','plane-owner'),('credential-admin','plane-admin')]:
  shutil.copytree(p/source,p/name)
def rotate():
 p,r=fresh();o=output();old=load(p/'plane-owner/reader.json');require(old['Enabled'],'Old owner already sealed')
 for name in ['resume-owner','resume-admin','credential-owner','credential-admin']:
  target=p/name
  if not target.exists():
   target.mkdir();shutil.copyfile(p/'plane-owner/reader',target/'reader');shutil.copyfile(p/'plane-owner/manifest.json',target/'manifest.json');shutil.copytree(p/'plane-owner/rules',target/'rules')
 cfg=load(p/'resume-owner/reader.json') if (p/'resume-owner/reader.json').exists() else dict(old,Access='resume'+r['run_id'].replace('-','')[:12],Secret=secrets.token_hex(32),ProducerAccess='resumeprod'+r['run_id'].replace('-','')[:12],ProducerSecret=secrets.token_hex(32),Epoch=str(__import__('uuid').uuid4()))
 save(p/'resume-owner/reader.json',cfg)
 root=dict(cfg);admin=load(p/'plane-admin/reader.json');root.update(RootAccess=admin['RootAccess'],RootSecret=admin['RootSecret']);save(p/'resume-admin/reader.json',root)
 for name in ['credential-owner','credential-admin']:save(p/name/'reader.json',cfg if name.endswith('owner') else root)
 # The fresh standardized exact version is obtained from already committed Core facts.
 std=json.loads(b.source_read(r['containers']['postgres'],"SELECT to_jsonb(c)::text FROM lifecycle_fixture_copy c WHERE object_key='fixture/standardized.csv';"));require(std['provider_version'],'Standardized exact version missing')
 caddy=(p/'Caddyfile').read_text(encoding='utf-8-sig').replace(old['Access'],cfg['Access']).replace(old['ProducerAccess'],cfg['ProducerAccess'])
 route='''  @standardized {
   path /dpp-lifecycle307-online/fixture/standardized.csv
   method GET
   expression {http.request.header.Authorization}.contains('Credential=ACCESS/') && {http.request.uri.query.versionId} == 'VERSION' && !{http.request.uri.query}.contains('retention') && !{http.request.uri.query}.contains('legal-hold')
  }
  reverse_proxy @standardized minio:9000
'''.replace('ACCESS',cfg['Access']).replace('VERSION',std['provider_version'])
 caddy=caddy.replace('  @inspection {',route+'  @inspection {');(p/'Caddyfile').write_text(caddy,encoding='utf-8',newline='\n')
 docker('exec',r['containers']['gateway'],'caddy','validate','--config','/etc/caddy/Caddyfile','--adapter','caddyfile');docker('restart',r['containers']['gateway'])
 # Unique public receipt locations: no overwriting old policy or call evidence.
 adminout=o/'resume-admin';adminout.mkdir();root['OutputDirectory']=old['OutputDirectory']+'/resume-admin';save(p/'resume-admin/reader.json',root)
 step('resume-provision','resume-admin');step('seal','plane-admin')
 for name in ['plane-owner','plane-admin']:
  path=p/name/'reader.json';shutil.copyfile(path,CONTROL/(name+'-before-seal.private.json'));c=load(path);c['Enabled']=False;save(path,c)
 write(PUBLIC/'rotation.json',{'old_mount_disabled':True,'old_reader_and_producer_native_disabled':True,'new_mounts':['resume-owner','resume-admin','credential-owner','credential-admin'],'standardized_copy_id':std['id'],'standardized_version':std['provider_version'],'delete':0})
def stop():
 p,r=fresh();o=output();cfg=load(p/'resume-owner/reader.json');membership=load(o/'resume-pin-membership.json');receipts=[load(x) for x in o.glob('reader-helper-*.json') if load(x)['mode']=='resume-crash'];require(len(receipts)==1,'Original owner ambiguous');receipt=receipts[0];i=inspect(receipt['id'])
 require(i['State']['Status']=='exited' and i['State']['ExitCode']==95 and i['State']['Pid']==0 and i['HostConfig']['RestartPolicy']['Name']=='no' and not i['HostConfig']['Privileged'] and i['HostConfig']['PidMode']=='' and i['HostConfig']['NetworkMode']=='container:'+inspect(r['containers']['gateway'])['Id'],'Native owner not closed')
 mounts=i['Mounts'];require(any(m['Destination']=='/run/private' and Path(m['Source'])==p/'plane-owner' and not m['RW'] for m in mounts) and all('docker.sock' not in m['Destination'] for m in mounts),'Old exact private mount drift')
 require(not load(p/'plane-owner/reader.json')['Enabled'] and not load(p/'plane-admin/reader.json')['Enabled'],'Old mount reopened')
 before=b.source_read(r['containers']['postgres'],"SELECT count(*) FROM lifecycle_storage_call;")
 docker('start',i['Id']);code=docker('wait',i['Id']).decode().strip();require(code=='1' and before==b.source_read(r['containers']['postgres'],"SELECT count(*) FROM lifecycle_storage_call;"),'Old fixed helper restart caused native calls')
 i=inspect(i['Id']);closure={'id':i['Id'],'image':i['Image'],'entrypoint':i['Config']['Entrypoint'],'cmd':i['Config']['Cmd'],'mounts':mounts,'host_config':{'restart':i['HostConfig']['RestartPolicy'],'privileged':False,'pid_mode':'','network':i['HostConfig']['NetworkMode']},'old_config_sha256':digest(p/'plane-owner/reader.json'),'binary_sha256':digest(p/'plane-owner/reader'),'manual_restart_exit':int(code),'storage_calls_unchanged':True}
 closurehash=hashlib.sha256(json.dumps(closure,sort_keys=True,separators=(',',':')).encode()).hexdigest();write(PUBLIC/'owner-runtime-closure.json',closure)
 identity={'Run':r['run_id'],'Workspace':cfg['WorkspaceID'],'Execution':membership['execution'],'HelperID':i['Id'],'ImageSHA256':i['Image'].removeprefix('sha256:'),'BinarySHA256':digest(p/'plane-owner/reader'),'PinsSHA256':membership['sha256'],'RuntimeClosureSHA256':closurehash}
 key=Ed25519PrivateKey.generate();private=key.private_bytes(serialization.Encoding.Raw,serialization.PrivateFormat.Raw,serialization.NoEncryption());(CONTROL/'owner-signing.private').write_bytes(private);public=key.public_key().public_bytes(serialization.Encoding.Raw,serialization.PublicFormat.Raw).hex()
 proof=dict(identity,Nonce=str(__import__('uuid').uuid4()),ObservedAt=dt.datetime.now(dt.timezone.utc).isoformat(),Exited=True,RestartFenced=True,CredentialsDisabled=True,ConfigDisabled=True,ClosedCgroup=True);body=json.dumps(proof,separators=(',',':')).encode()
 cfg.update(OwnerStopPublicKey=public,OwnerStopIdentity=identity);save(p/'resume-owner/reader.json',cfg);write(p/'resume-owner/owner-stop.json',{'Body':base64.b64encode(body).decode(),'Signature':base64.b64encode(key.sign(body)).decode()})
 step('resume-stop','resume-owner')
def retry():step('resume-retry','resume-owner')
def processing_seal():
 p,r=fresh();step('seal','resume-admin')
 for role in ['plane-owner','plane-admin','resume-owner','resume-admin','credential-owner','credential-admin']:
  path=p/role/'reader.json'
  if not path.exists():continue
  if not (CONTROL/(role+'-before-final-seal.private.json')).exists():shutil.copyfile(path,CONTROL/(role+'-before-final-seal.private.json'))
  c=load(path);c['Enabled']=False;save(path,c)
 write(PUBLIC/'final-owner-seal.json',{'run':r['run_id'],'owner_enabled':False,'reader_and_producer_native_disabled':True,'native_delete':0,'cleanup':0})
def alias_seal():
 p,r=fresh();require(r['run_id']=='8120904f-13d9-42f3-ab1f-66eb1a37b2b0','Exact processing closure correction only');facts={}
 for role in ['plane-owner','plane-admin','resume-owner','resume-admin','credential-owner','credential-admin']:
  path=p/role/'reader.json';prior=CONTROL/(role+'-before-alias-seal.private.json');shutil.copyfile(path,prior);c=load(path);c['Enabled']=False;save(path,c);facts[role]={'enabled':False,'sha256':digest(path)}
 write(PUBLIC/'additional-config-seal.json',{'run':r['run_id'],'roles':facts,'native_calls':0,'native_delete':0,'cleanup':0})
def credentials_prepare():
 p,r=fresh();cfg=load(p/'credential-owner/reader.json');require(not (p/'credential-owner/credential-commands.json').exists(),'Credential window already allocated')
 cfg.update(IssuerAccess='issuer'+r['run_id'].replace('-','')[:12],IssuerSecret=secrets.token_hex(32));save(p/'credential-owner/reader.json',cfg)
 root=load(p/'credential-admin/reader.json');root.update(IssuerAccess=cfg['IssuerAccess'],IssuerSecret=cfg['IssuerSecret'],OutputDirectory=cfg['OutputDirectory']+'/credential-admin');(output()/'credential-admin').mkdir();save(p/'credential-admin/reader.json',root)
 manifest=load(p/'plane-owner/manifest.json');beta=manifest['objects'][1];caddy=(p/'Caddyfile').read_text(encoding='utf-8')
 route='''  @issuer_admin {
   path /minio/admin/v3/add-service-account /minio/admin/v3/info-service-account /minio/admin/v3/update-service-account
   method PUT POST GET
   expression {http.request.header.Authorization}.contains('Credential=ISSUER/')
  }
  reverse_proxy @issuer_admin minio:9000
  @credential_payload {
   path /dpp-lifecycle307-online/fixture/b.csv
   method GET
   expression {http.request.header.Authorization}.contains('Credential=lc13cap') && {http.request.uri.query.versionId} == 'VERSION' && !{http.request.uri.query}.contains('retention') && !{http.request.uri.query}.contains('legal-hold')
  }
  reverse_proxy @credential_payload minio:9000
'''.replace('ISSUER',cfg['IssuerAccess']).replace('VERSION',beta['version_id'])
 caddy=caddy.replace('  @write method',route+'  @write method');(p/'Caddyfile').write_text(caddy,encoding='utf-8',newline='\n');docker('exec',r['containers']['gateway'],'caddy','validate','--config','/etc/caddy/Caddyfile','--adapter','caddyfile');docker('restart',r['containers']['gateway']);step('credential-provision','credential-admin')
def credentials_issue():
 p,r=fresh();cfg=load(p/'credential-owner/reader.json');manifest=load(p/'plane-owner/manifest.json');beta=manifest['objects'][1];now=dt.datetime.now(dt.timezone.utc)
 cmds=[]
 # MinIO service accounts enforce a native minimum expiry of 15 minutes.
 # Observe the real provider clock; do not shorten or alter Core/native time.
 for name,seconds in [('expiry',920),('containment',1200)]:
  cmds.append({'WorkspaceID':cfg['WorkspaceID'],'DatasetVersionID':beta['dataset_version_id'],'ProviderName':'LC13_MINIO_REDEMPTION','PrincipalRef':'lifecycle-fixture:'+r['run_id'],'EffectiveConsumerRef':'lifecycle-pilot-consumer','Purpose':'lifecycle-pilot-read','Action':'READ','ScopeRef':beta['dataset_version_id'],'DeliveryChannel':'REDEMPTION','DeliveryMode':'PLATFORM_REDEMPTION','RequestedExpiresAt':(now+dt.timedelta(seconds=seconds)).isoformat(),'IdempotencyKey':'lc13-native-'+name,'ActorID':cfg['ActorID']})
 cmds[1]['RequestedExpiresAt']=(dt.datetime.fromisoformat(cfg['ExpiresAt'].replace('Z','+00:00'))+dt.timedelta(minutes=5)).isoformat()
 # Use existing pilot constants instead of accepting caller-selected consumer/purpose.
 import re
 constants=(b.ROOT/'apps/platform/internal/lifecycle/pilot/plane_payload.go').read_text(encoding='utf-8-sig')
 for cmd in cmds:
  for field,const in [('EffectiveConsumerRef','PlaneConsumer'),('Purpose','PlanePurpose')]:cmd[field]=re.search(const+r'\s*=\s*"([^"]+)"',constants).group(1)
 path=p/'credential-owner/credential-commands.json'
 if path.exists():
  count=b.source_read(r['containers']['postgres'],"SELECT count(*) FROM delivery_operation WHERE provider_name='LC13_MINIO_REDEMPTION';");require(count in ('0','1','2','3'),'Existing native acceptance window cannot be replaced')
  if count=='1':
   prior=load(path);write(p/'credential-admin/credential-aborted-command.json',prior[0]);cmds[0]['IdempotencyKey']+='-02';cmds[1]['IdempotencyKey']+='-02'
  if count=='2':
   prior=load(path);write(p/'credential-admin/credential-aborted-command-02.json',prior[0]);cmds[0]['IdempotencyKey']+='-03';cmds[1]['IdempotencyKey']+='-03'
  if count=='3':
   prior=load(path);write(p/'credential-admin/credential-aborted-command-03.json',prior[0]);cmds[0]['IdempotencyKey']+='-04';cmds[1]['IdempotencyKey']+='-04'
  shutil.copyfile(path,CONTROL/('failed-preissue-window-'+str(__import__('uuid').uuid4())+'.private.json'));save(path,cmds)
 else:write(path,cmds)
 step('credential-issue','credential-owner')
def credentials_expiry():
 facts=load(output()/'credential-issue.json');deadline=dt.datetime.fromisoformat(facts['facts'][0]['native_expiry'].replace('Z','+00:00'))+dt.timedelta(seconds=2)
 require((deadline-dt.datetime.now(dt.timezone.utc)).total_seconds()<16*60,'Deadline outside bounded native acceptance')
 while dt.datetime.now(dt.timezone.utc)<deadline:time.sleep(min(15,max(0,(deadline-dt.datetime.now(dt.timezone.utc)).total_seconds())))
 step('credential-expiry','credential-owner')
def credentials_continue():step('credential-issue','credential-owner')
def credentials_contain():step('credential-contain','credential-owner')
def credentials_confirm():step('credential-confirm','credential-owner')
def forensic():
 p,r=fresh();require(r['run_id']=='f3b0c9cd-ee53-42d5-a5e4-48c9ede5e4c3','Forensic only exact retained failed run')
 cfg=load(p/'credential-admin/reader.json');cfg['OutputDirectory']=load(p/'credential-owner/reader.json')['OutputDirectory'];cfg['ExpiresAt']='2026-10-14T14:10:00Z';save(p/'credential-admin/reader.json',cfg)
 cmds=[load(p/'credential-admin'/name) for name in ['credential-aborted-command.json','credential-aborted-command-02.json','credential-aborted-command-03.json']]+[load(p/'credential-owner/credential-commands.json')[0]];write(p/'credential-admin/credential-forensic-commands.json',cmds)
 caddy=(p/'Caddyfile').read_text(encoding='utf-8').replace('method PUT POST\n   expression','method PUT POST GET\n   expression');(p/'Caddyfile').write_text(caddy,encoding='utf-8',newline='\n');docker('exec',r['containers']['gateway'],'caddy','validate','--config','/etc/caddy/Caddyfile','--adapter','caddyfile');docker('restart',r['containers']['gateway']);step('credential-forensic','credential-admin')
def closure_prepare():
 p,r=fresh();observer=p/'resume-owner'
 if not observer.exists():
  shutil.copytree(p/'plane-owner',observer);shutil.copyfile(p/'credential-owner/reader',observer/'reader')
 cfg=load(observer/'reader.json');cfg.update(Enabled=True,IssuerAccess='',IssuerSecret='',RootAccess='',RootSecret='')
 if r['run_id']=='f3b0c9cd-ee53-42d5-a5e4-48c9ede5e4c3':cfg['ExpiresAt']='2026-10-14T14:10:00Z'
 save(observer/'reader.json',cfg)
 step('credential-restrict','resume-owner');step('credential-subjects','resume-owner')
 # Disable every payload/management owner identity before signing its closure.
 for role in ['plane-admin','resume-admin','credential-admin']:
  if not (p/role/'reader.json').exists():continue
  c=load(p/role/'reader.json')
  if not c['Enabled']:continue
  if r['run_id']=='f3b0c9cd-ee53-42d5-a5e4-48c9ede5e4c3':c['ExpiresAt']='2026-10-14T14:10:00Z';save(p/role/'reader.json',c)
  step('credential-seal' if role=='credential-admin' else 'seal',role)
 for role in ['plane-owner','plane-admin','resume-admin','credential-owner','credential-admin']:
  path=p/role/'reader.json'
  if not path.exists():continue
  if not (CONTROL/(role+'-before-credential-seal.private.json')).exists():shutil.copyfile(path,CONTROL/(role+'-before-credential-seal.private.json'))
  c=load(path);c['Enabled']=False;save(path,c)
 write(PUBLIC/'credential-windows-sealed.json',{'run':r['run_id'],'payload_and_root_owners_disabled':True,'issuer_native_disabled':True,'native_delete':0})
def release():
 p,r=fresh();o=output();cfg=load(p/'resume-owner/reader.json');subjects=load(o/'credential-subjects.json');helpers=[]
 # Every actual one-shot owner must have stopped. No general status/lease claim.
 names=docker('ps','-a','--filter','label=lifecycle307.run='+r['run_id'],'--format','{{.Names}}').decode().splitlines()
 services=set(load(CONTROL/'plane.json')['services'])
 for name in names:
  if name in services:continue
  i=inspect(name);require(i['State']['Status']=='exited' and i['State']['Pid']==0 and i['HostConfig']['RestartPolicy']['Name']=='no' and not i['HostConfig']['Privileged'] and i['HostConfig']['PidMode']=='','Credential native owner not closed')
  require(all('docker.sock' not in m['Destination'] for m in i['Mounts']),'Untracked native daemon owner')
  helpers.append({'id':i['Id'],'image':i['Image'],'entrypoint':i['Config']['Entrypoint'],'cmd':i['Config']['Cmd'],'mounts':i['Mounts'],'state':'exited','exit':i['State']['ExitCode'],'network':i['HostConfig']['NetworkMode']})
 sealed={}
 for role in ['plane-owner','plane-admin','resume-admin','credential-owner','credential-admin']:
  path=p/role/'reader.json'
  if path.exists():require(not load(path)['Enabled'],'Old owner configuration reopened');sealed[role]=digest(path)
 receipts=[load(x) for x in o.rglob('reader-helper-*.json')];parent=[x for x in receipts if x['mode']=='credential-seal'];require(len(parent)==1 and parent[0]['exit_code']==0 and not inspect(parent[0]['id'])['State']['Running'],'Native parent disable receipt unavailable')
 old=[x for x in receipts if x['mode']=='credential-issue'];require(old,'Native issue owner unavailable');i=inspect(old[-1]['id']);before=b.source_read(r['containers']['postgres'],"SELECT count(*) FROM lifecycle_storage_call;");docker('start',i['Id']);code=docker('wait',i['Id']).decode().strip();require(code=='1' and before==b.source_read(r['containers']['postgres'],"SELECT count(*) FROM lifecycle_storage_call;"),'Closed credential owner restarted I/O')
 for helper in helpers:
  current=inspect(helper['id']);require(current['State']['Status']=='exited' and current['State']['Pid']==0,'Owner reopened during closure capture');helper['exit']=current['State']['ExitCode']
 nonce=str(__import__('uuid').uuid4());closure={'run':r['run_id'],'helpers':sorted(helpers,key=lambda x:x['id']),'sealed_configs':sealed,'parent_seal':parent[0],'gateway_sha256':digest(p/'Caddyfile'),'old_fixed_restart_exit':1,'storage_calls_unchanged':True,'subjects_sha256':subjects['SHA256']};h=hashlib.sha256(json.dumps(closure,sort_keys=True,separators=(',',':')).encode()).hexdigest();write(PUBLIC/('credential-runtime-closure-'+nonce+'.json'),closure)
 identity={'Run':r['run_id'],'Workspace':cfg['WorkspaceID'],'RuntimeClosureSHA256':h,'SubjectsSHA256':subjects['SHA256']};key=Ed25519PrivateKey.generate();(CONTROL/('credential-owner-signing-'+nonce+'.private')).write_bytes(key.private_bytes(serialization.Encoding.Raw,serialization.PrivateFormat.Raw,serialization.NoEncryption()));public=key.public_key().public_bytes(serialization.Encoding.Raw,serialization.PublicFormat.Raw).hex()
 path=p/'resume-owner/credential-stop.json'
 if path.exists():shutil.copyfile(path,CONTROL/('failed-credential-stop-'+nonce+'.private.json'));shutil.copyfile(p/'resume-owner/reader.json',CONTROL/('failed-observer-'+nonce+'.private.json'))
 cfg.update(CredentialStopPublicKey=public,CredentialStopIdentity=identity);save(p/'resume-owner/reader.json',cfg)
 proof=dict(identity,Nonce=nonce,ObservedAt=dt.datetime.now(dt.timezone.utc).isoformat(),OwnerExited=True,ConfigDisabled=True,ParentDisabled=True,ClosedCgroup=True,Subjects=subjects['Subjects']);body=json.dumps(proof,separators=(',',':')).encode();save(path,{'Body':base64.b64encode(body).decode(),'Signature':base64.b64encode(key.sign(body)).decode()});step('credential-release','resume-owner')
 cfg['Enabled']=False;save(p/'resume-owner/reader.json',cfg)
 write(PUBLIC/'final-owner-seal.json',{'observer_enabled':False,'native_delete':0,'cleanup':0})
def restop():
 p,r=fresh();cfg=load(p/'resume-owner/reader.json');key=Ed25519PrivateKey.from_private_bytes((CONTROL/'owner-signing.private').read_bytes())
 prior=load(p/'resume-owner/owner-stop.json')
 closure=load(PUBLIC/'owner-runtime-closure.json');i=inspect(closure['id'])
 require(i['State']['Status']=='exited' and i['State']['Pid']==0 and i['Image']==closure['image'] and i['Config']['Entrypoint']==closure['entrypoint'] and i['Config']['Cmd']==closure['cmd'] and i['Mounts']==closure['mounts'] and i['HostConfig']['RestartPolicy']==closure['host_config']['restart'] and not i['HostConfig']['Privileged'] and i['HostConfig']['PidMode']=='' and i['HostConfig']['NetworkMode']==closure['host_config']['network'],'Restop owner closure drift')
 require(not load(p/'plane-owner/reader.json')['Enabled'] and not load(p/'plane-admin/reader.json')['Enabled'] and digest(p/'plane-owner/reader.json')==closure['old_config_sha256'] and digest(p/'plane-owner/reader')==closure['binary_sha256'],'Restop old mount or binary drift')
 seals=[load(x) for x in output().glob('reader-helper-*.json') if load(x)['mode']=='seal'];require(len(seals)==1 and seals[0]['exit_code']==0 and not inspect(seals[0]['id'])['State']['Running'],'Native disable owner receipt unavailable')
 bodyhash=hashlib.sha256(json.dumps(closure,sort_keys=True,separators=(',',':')).encode()).hexdigest();require(bodyhash==cfg['OwnerStopIdentity']['RuntimeClosureSHA256'],'Restop signed closure drift')
 capture={'at':dt.datetime.now(dt.timezone.utc).isoformat(),'helper':i['Id'],'runtime_closure_sha256':bodyhash,'old_config_sha256':closure['old_config_sha256'],'seal_helper':seals[0]['id'],'current_state':'exited','fresh_host_capture':True}
 write(PUBLIC/('owner-restop-capture-'+str(__import__('uuid').uuid4())+'.json'),capture)
 if not (CONTROL/'failed-owner-stop.private.json').exists():shutil.copyfile(p/'resume-owner/owner-stop.json',CONTROL/'failed-owner-stop.private.json')
 proof=json.loads(base64.b64decode(prior['Body']));proof.update(Nonce=str(__import__('uuid').uuid4()),ObservedAt=dt.datetime.now(dt.timezone.utc).isoformat());body=json.dumps(proof,separators=(',',':')).encode();save(p/'resume-owner/owner-stop.json',{'Body':base64.b64encode(body).decode(),'Signature':base64.b64encode(key.sign(body)).decode()})
 step('resume-stop','resume-owner')
def export():
 o=output()
 for x in o.rglob('*.json'):
  rel=x.relative_to(o);target=PUBLIC/rel;target.parent.mkdir(parents=True,exist_ok=True)
  if not target.exists():shutil.copyfile(x,target)
def audit():
 p,r=fresh();query="""SELECT json_build_object(
 'copies',(SELECT count(*) FROM lifecycle_fixture_copy),
 'pins',(SELECT count(*) FROM lifecycle_reader_pin),
 'terminations',(SELECT count(*) FROM lifecycle_reader_termination),
 'active_pins',(SELECT count(*) FROM lifecycle_reader_pin p WHERE NOT EXISTS(SELECT 1 FROM lifecycle_reader_termination t WHERE t.pin_id=p.id)),
 'calls',(SELECT count(*) FROM lifecycle_storage_call),
 'observations',(SELECT count(*) FROM lifecycle_storage_observation),
 'storage_costs',(SELECT count(*) FROM cost_allocation WHERE lifecycle_storage_call_id IS NOT NULL),
 'unaccounted_storage',(SELECT count(*) FROM lifecycle_storage_call c WHERE (SELECT count(*) FROM cost_allocation a JOIN cost_event e ON e.id=a.cost_event_id WHERE a.lifecycle_storage_call_id=c.id AND e.activity_id=c.id AND e.quantity=1 AND e.unit='invocation')<>1),
 'provider_attempts',(SELECT count(*) FROM delivery_provider_attempt),
 'provider_observations',(SELECT count(*) FROM delivery_provider_observation),
 'provider_costs',(SELECT count(*) FROM cost_allocation a JOIN cost_event e ON e.id=a.cost_event_id WHERE e.cost_type='DELIVERY_PROVIDER_INVOCATION'),
 'unaccounted_provider',(SELECT count(*) FROM delivery_provider_attempt p WHERE (SELECT count(*) FROM cost_allocation a JOIN cost_event e ON e.id=a.cost_event_id WHERE a.delivery_operation_id=p.delivery_operation_id AND e.activity_id=p.id AND e.quantity=1 AND e.unit='invocation')<>1),
 'native_deliveries',(SELECT coalesce(jsonb_agg(jsonb_build_object('id',id,'status',status,'key',idempotency_key) ORDER BY id),'[]') FROM delivery_operation WHERE provider_name='LC13_MINIO_REDEMPTION'),
 'unreleased_credential_demands',(SELECT count(*) FROM lifecycle_fixture_reference_requirement r JOIN delivery_operation d ON d.id=r.delivery_operation_id WHERE d.provider_name='LC13_MINIO_REDEMPTION' AND NOT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_release x WHERE x.requirement_id=r.id)),
 'replay_decisions',(SELECT coalesce(jsonb_agg(jsonb_build_object('operation',delivery_operation_id,'decision',decision,'at',created_at) ORDER BY created_at,id),'[]') FROM delivery_credential_replay_decision),
 'native_delete',(SELECT count(*) FROM lifecycle_storage_call WHERE operation='DELETE_EXACT'),
 'send_claims',(SELECT count(*) FROM lifecycle_provider_send_claim),
 'absence',(SELECT count(*) FROM lifecycle_copy_absence_receipt),
 'completion',(SELECT count(*) FROM lifecycle_plan_completion),
 'executions',(SELECT coalesce(jsonb_agg(jsonb_build_object('id',id,'status',status,'retry_of',retry_of_execution_id,'output',output_dataset_version_id) ORDER BY created_at,id),'[]') FROM execution));"""
 facts=json.loads(b.source_read(r['containers']['postgres'],query));require(facts['calls']==facts['observations']==facts['storage_costs'] and facts['unaccounted_storage']==facts['unaccounted_provider']==facts['unreleased_credential_demands']==facts['active_pins']==facts['native_delete']==facts['send_claims']==facts['absence']==facts['completion']==0 and facts['provider_observations']>=facts['provider_attempts']==facts['provider_costs'],'Final accounting/occupancy failed')
 configs={str(x.relative_to(p)):load(x).get('Enabled') for x in p.rglob('reader.json')};require(all(x is False for x in configs.values()),'Unsealed owner alias')
 facts.update(run=r['run_id'],workspace=load(p/'config.json')['WorkspaceID'],roles=configs,cleanup=0,all_resources_retained=True)
 write(PUBLIC/'final-readonly-accounting.json',facts);(PUBLIC/'final-readonly-accounting.sql').write_text('BEGIN READ ONLY;\n'+query+'\nROLLBACK;\n',encoding='utf-8',newline='\n');export()
def main():
 global CONTROL,PUBLIC,SUBNET
 a=argparse.ArgumentParser();a.add_argument('--fixture',choices=['processing','credential','processing-current'],default='processing');a.add_argument('phase',choices=['prepare','setup','credential_setup','rotate','stop','restop','retry','processing_seal','alias_seal','credentials_prepare','credentials_issue','credentials_continue','credentials_expiry','credentials_contain','credentials_confirm','forensic','closure_prepare','release','audit','export']);args=a.parse_args()
 if args.fixture=='credential':
  CONTROL=b.PRIVATE/'d1-lc13-credential01';PUBLIC=b.ROOT/'docs/acceptance/evidence/issue307-d1/lc13-continuation01/credential';SUBNET='10.234.160.0/24';b.CONTROL=CONTROL;b.PUBLIC=PUBLIC
 if args.fixture=='processing-current':
  CONTROL=b.PRIVATE/'d1-lc13-processing-current01';PUBLIC=b.ROOT/'docs/acceptance/evidence/issue307-d1/lc13-continuation01/processing-current';SUBNET='10.234.161.0/24';b.CONTROL=CONTROL;b.PUBLIC=PUBLIC
 globals()[args.phase]()
if __name__=='__main__':main()
