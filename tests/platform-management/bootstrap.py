"""NEW independent synthetic management fixture. Never reuses original DB/session/IdP."""
import json,os,secrets,subprocess,socket,ssl,sys,time,urllib.parse,urllib.request
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT/'tests'))
from fixture_preservation import container_metadata,capture_retained,verify_retained
RUNTIME_LOGS=os.environ.get('MANAGEMENT_RUNTIME_LOG_FIXTURE_ACK')=='ISOLATED_RUNTIME_LOG_READONLY'
if os.environ.get('MANAGEMENT_RUNTIME_LOG_FIXTURE_ACK') and not RUNTIME_LOGS:raise RuntimeError('invalid runtime fixture acknowledgement')
MAIN_SERVICE=os.environ.get('MANAGEMENT_MAIN_SERVICE_FIXTURE_ACK')=='ISOLATED_MAIN_SERVICE_READONLY'
if os.environ.get('MANAGEMENT_MAIN_SERVICE_FIXTURE_ACK') and not MAIN_SERVICE:raise RuntimeError('invalid main service fixture acknowledgement')
COMPAT=os.environ.get('MANAGEMENT_MAIN_SERVICE_COMPAT_FIXTURE_ACK')=='ISOLATED_MAIN_SERVICE_COMPAT_READONLY'
if os.environ.get('MANAGEMENT_MAIN_SERVICE_COMPAT_FIXTURE_ACK') and not COMPAT:raise RuntimeError('invalid main service compatibility acknowledgement')
DIRECTORY=os.environ.get('MANAGEMENT_DIRECTORY_PRODUCT_FIXTURE_ACK')=='ISOLATED_DIRECTORY_PRODUCT_READONLY'
if os.environ.get('MANAGEMENT_DIRECTORY_PRODUCT_FIXTURE_ACK') and not DIRECTORY:raise RuntimeError('invalid directory product fixture acknowledgement')
USER_PRODUCT=os.environ.get('MANAGEMENT_USER_PRODUCT_FIXTURE_ACK')=='ISOLATED_USER_PRODUCT_READONLY'
if os.environ.get('MANAGEMENT_USER_PRODUCT_FIXTURE_ACK') and not USER_PRODUCT:raise RuntimeError('invalid user product fixture acknowledgement')
PRODUCT=DIRECTORY or USER_PRODUCT
if sum([RUNTIME_LOGS,MAIN_SERVICE,COMPAT,DIRECTORY,USER_PRODUCT])>1:raise RuntimeError('fixture modes are disjoint')
RUN=ROOT/('.data/runtime-logs-api-20261026' if RUNTIME_LOGS else '.data/management-readonly-20261025')
PG='dpp-runtime-logs-readonly-20261026' if RUNTIME_LOGS else 'dpp-management-readonly-20261025'
PG_PORT,API_PORT,IDP_PORT,WEB_PORT=(25550,18550,19150,4350) if RUNTIME_LOGS else (25496,18499,19130,4300)
DATABASE='runtime_logs_readonly_20261026' if RUNTIME_LOGS else 'management_readonly_20261025'
if MAIN_SERVICE:
 RUN=ROOT/'.data/management-main-service-20261026';PG='dpp-management-main-service-20261026'
 PG_PORT,API_PORT,IDP_PORT,WEB_PORT=25560,18560,19160,4360
 DATABASE='management_main_service_20261026'
if COMPAT:
 RUN=ROOT/'.data/management-main-service-compat-20261026';PG='dpp-management-main-service-compat-20261026'
 PG_PORT,API_PORT,IDP_PORT,WEB_PORT=25570,18570,19170,4370
 DATABASE='management_main_service_compat_20261026'
if DIRECTORY:
 RUN=ROOT/'.data/management-directory-product-20261026';PG='dpp-management-directory-product-20261026'
 PG_PORT,API_PORT,IDP_PORT,WEB_PORT=25580,18580,19180,4380
 DATABASE='management_directory_product_20261026'
if USER_PRODUCT:
 RUN=ROOT/'.data/management-user-product-20261027';PG='dpp-management-user-product-20261027'
 PG_PORT,API_PORT,IDP_PORT,WEB_PORT=25620,18620,19220,4420
 DATABASE='management_user_product_20261027'

def call(args,**kwargs):
 p=subprocess.run(args,capture_output=True,encoding='utf-8',**kwargs)
 if p.returncode:
  if RUN.exists():(RUN/'prepare-private.log').write_text(p.stdout+'\n'+p.stderr,encoding='utf-8')
  raise RuntimeError('isolated preparation failed in '+Path(args[0]).name+'; private diagnostics retained and withheld')
 return p.stdout
def metadata(name):
 return container_metadata(name)
def env_clean():return {k:v for k,v in os.environ.items() if not k.startswith(('POC_','BROWSER_','AGENT_','PLATFORM_','HUMAN_DECISION_','DELIVERY_','SOURCE_ADMISSION_','GOVERNANCE_','OPENAI_','ANTHROPIC_','DASHSCOPE_','PG')) and k not in ['DATABASE_URL','REDIS_URL']}
def spawn(name,args,env):
 with open(RUN/(name+'.log'),'w',encoding='utf-8') as log:p=subprocess.Popen(args,cwd=ROOT,env={**env_clean(),**env},stdout=log,stderr=log,creationflags=getattr(subprocess,'CREATE_NO_WINDOW',0))
 return {'name':name,'pid':p.pid,'path':args[0] if len(args)==1 else args[1],'startedAt':time.time()}
def main():
 if os.environ.get('MANAGEMENT_FIXTURE_ACK')!='ISOLATED_READONLY_MANAGEMENT':raise RuntimeError('explicit independent fixture required')
 if RUN.exists() or metadata(PG):raise RuntimeError('existing fixture retained; refusing overwrite')
 for port in [PG_PORT,API_PORT,IDP_PORT,WEB_PORT]:
  with socket.socket() as s:
   if s.connect_ex(('127.0.0.1',port))==0:raise RuntimeError('fixture port occupied; no process changed')
 os.umask(0o077);RUN.mkdir(parents=True,mode=0o700)
 retained=capture_retained(('original','management','runtime','main-service','main-service-compat','directory-product') if USER_PRODUCT else ('original','management','runtime','main-service','main-service-compat') if DIRECTORY else ('original','management','runtime','main-service') if COMPAT else ('original','management','runtime') if MAIN_SERVICE else ('original','management') if RUNTIME_LOGS else ('original',),RUN/'retained-comparison.json',observer=metadata)
 (RUN/'retained-before.json').write_text(json.dumps(retained),encoding='utf-8')
 cert=RUN/'server.crt';key=RUN/'server.key';call(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-days','3','-keyout',str(key),'-out',str(cert),'-subj','/CN=localhost','-addext','subjectAltName=DNS:localhost,IP:127.0.0.1'])
 p={k:secrets.token_urlsafe(32) for k in ['pgPassword','apiPassword','rpPassword','rpSecret','resourceSecret','readCredential','control','encryptionKey']};p.update({'issuer':f'https://localhost:{IDP_PORT}','origin':f'https://localhost:{WEB_PORT}','runtimeLogs':RUNTIME_LOGS,'mainService':MAIN_SERVICE,'mainServiceCompat':COMPAT,'directoryProduct':DIRECTORY,'userProduct':USER_PRODUCT,'cert':str(cert),'key':str(key),'database':DATABASE,'postgres':PG})
 def dsn(role,password):return 'postgres://'+role+':'+urllib.parse.quote(password,safe='')+f'@127.0.0.1:{PG_PORT}/'+p['database']
 tls='?sslmode=verify-full&sslrootcert='+urllib.parse.quote(str(cert),safe='')
 p['adminDSN']=dsn('postgres',p['pgPassword'])+tls;(RUN/'profile.json').write_text(json.dumps(p),encoding='utf-8')
 (RUN/'pg.env').write_text('POSTGRES_PASSWORD='+p['pgPassword']+'\nPOSTGRES_DB='+p['database']+'\n',encoding='utf-8')
 call(['docker','run','-d','--name',PG,'--label','codex.scope=management-readonly','--env-file',str(RUN/'pg.env'),'-p',f'127.0.0.1:{PG_PORT}:5432','--mount',f'type=bind,source={RUN},target=/fixture,readonly','postgres:17-alpine','sh','-c','cp /fixture/server.key /tmp/server.key && cp /fixture/server.crt /tmp/server.crt && chown postgres:postgres /tmp/server.* && chmod 600 /tmp/server.key && exec docker-entrypoint.sh postgres -c ssl=on -c ssl_cert_file=/tmp/server.crt -c ssl_key_file=/tmp/server.key'])
 for _ in range(60):
  r=subprocess.run(['docker','exec',PG,'pg_isready','-U','postgres','-d',p['database']],capture_output=True)
  if r.returncode==0:break
  time.sleep(.5)
 else:raise RuntimeError('new isolated database unavailable; no automatic restart')
 (RUN/'resources.json').write_text(json.dumps({'postgres':metadata(PG)}),encoding='utf-8')
 api={'DatabaseURL':dsn('management_api',p['apiPassword'])+tls,'Issuer':p['issuer'],'ResourceSecret':p['resourceSecret'],'ReadCredential':p['readCredential'],'Cert':str(cert),'Key':str(key),'CA':str(cert),'Root':str(ROOT),'RuntimeLogs':RUNTIME_LOGS,'MainService':MAIN_SERVICE,'MainServiceCompat':COMPAT,'DirectoryProduct':DIRECTORY,'UserProduct':USER_PRODUCT,'APIPassword':p['apiPassword'],'RPPassword':p['rpPassword']}
 api_path=RUN/'api-config.json';api_path.write_text(json.dumps(api),encoding='utf-8')
 binary=RUN/('api.exe' if os.name=='nt' else 'api');call(['go','build','-o',str(binary),'./tests/platformmanagement'],cwd=ROOT/'apps/platform')
 base={'MANAGEMENT_FIXTURE_ACK':'ISOLATED_READONLY_MANAGEMENT','MANAGEMENT_FIXTURE_CONFIG':str(api_path)}
 if MAIN_SERVICE:base['MANAGEMENT_MAIN_SERVICE_FIXTURE_ACK']='ISOLATED_MAIN_SERVICE_READONLY'
 if COMPAT:base['MANAGEMENT_MAIN_SERVICE_COMPAT_FIXTURE_ACK']='ISOLATED_MAIN_SERVICE_COMPAT_READONLY'
 if USER_PRODUCT:base['MANAGEMENT_USER_PRODUCT_FIXTURE_ACK']='ISOLATED_USER_PRODUCT_READONLY'
 if DIRECTORY:base['MANAGEMENT_DIRECTORY_PRODUCT_FIXTURE_ACK']='ISOLATED_DIRECTORY_PRODUCT_READONLY'
 call([str(binary),'prepare'],env={**env_clean(),**base,'MANAGEMENT_FIXTURE_ADMIN_DSN':p['adminDSN']})
 # API runtime configuration carries no admin DSN or RP password.
 api['APIPassword']='';api['RPPassword']='';api_path.write_text(json.dumps(api),encoding='utf-8')
 processes=[spawn('identity',['node',str(ROOT/'tests/platform-management/fixture.mjs')],{**base,'MANAGEMENT_FIXTURE_PROFILE':str(RUN/'profile.json')})]
 if PRODUCT:
  product=RUN/('api-product.exe' if os.name=='nt' else 'api-product');call(['go','build','-o',str(product),'./cmd/api'],cwd=ROOT/'apps/platform')
  for filename,value in [('database-url',dsn('management_api',p['apiPassword'])),('identity-secret',p['resourceSecret']),('directory-credential',p['readCredential'])]:(RUN/filename).write_text(value,encoding='utf-8')
  roles=[{'id':'management_viewer','name':'管理查询员','permissions':['management.members.read','management.roles.read','management.audit.read','management.users.read'] if USER_PRODUCT else ['management.members.read','management.roles.read','management.audit.read']},{'id':'audit_viewer','name':'审计观察员','permissions':['management.audit.read']}]
  product_profile={'version':'platform-management-query.v1','mode':'QUERY_ONLY','listen':f'127.0.0.1:{API_PORT}','tls':{'certFile':str(cert),'keyFile':str(key)},'identity':{'source':'CONFIGURED','issuer':p['issuer'],'tokenEndpoint':p['issuer']+'/token','introspectionEndpoint':p['issuer']+'/introspect','clientId':'dpp-management-api','clientSecretFile':str(RUN/'identity-secret'),'audience':'dpp-management-api','requiredScope':'platform-management','allowedClientIds':['dpp-management-rp'],'caFile':str(cert)},'database':{'urlFile':str(RUN/'database-url'),'caFile':str(cert),'name':DATABASE,'role':'management_api'},'audit':{'source':'CONFIGURED','workspaces':['11111111-1111-4111-8111-111111111111','12121212-1212-4212-8212-121212121212']},'directory':{'source':'CONFIGURED','provider':'ZITADEL','workspaces':[{'workspaceId':ws,'origin':p['issuer'],'caFile':str(cert),'credentialFile':str(RUN/'directory-credential'),'organizationId':org,'projectId':'governance','roles':roles} for ws,org in [('11111111-1111-4111-8111-111111111111','organization-a'),('12121212-1212-4212-8212-121212121212','organization-b')]]}}
  if USER_PRODUCT:
   for binding in product_profile['directory']['workspaces']:binding['userProfiles']=True
   disabled=dict(product_profile['directory']['workspaces'][1]);disabled.update({'workspaceId':'13131313-1313-4313-8313-131313131313','userProfiles':False})
   product_profile['directory']['workspaces'].append(disabled)
  profile_path=RUN/'product-profile.json';profile_path.write_text(json.dumps(product_profile),encoding='utf-8')
  processes.append(spawn('api-product',[str(product)],{'PLATFORM_MANAGEMENT_PROFILE':str(profile_path)}))
 else:processes.append(spawn('api',[str(binary)],base))
 (RUN/'processes.json').write_text(json.dumps(processes),encoding='utf-8')
 web={'BROWSER_IAM_MODE':'oidc','BROWSER_IAM_PROFILE':'platform-management','BROWSER_IAM_ORIGIN':p['origin'],'BROWSER_IAM_ISSUER':p['issuer'],'BROWSER_IAM_CLIENT_ID':'dpp-management-rp','BROWSER_IAM_CLIENT_SECRET':p['rpSecret'],'BROWSER_IAM_AUTHORIZATION_ENDPOINT':p['issuer']+'/authorize','BROWSER_IAM_TOKEN_ENDPOINT':p['issuer']+'/token','BROWSER_IAM_JWKS_URI':p['issuer']+'/jwks','BROWSER_IAM_LOGOUT_ENDPOINT':p['issuer']+'/logout','BROWSER_IAM_PRINCIPAL_ENDPOINT':f'https://localhost:{API_PORT}/internal/identity/platform-management-principal','BROWSER_IAM_SCOPE':'openid platform-management','BROWSER_IAM_ENCRYPTION_KEY':p['encryptionKey'],'BROWSER_IAM_DATABASE_URL':dsn('management_rp',p['rpPassword']),'BROWSER_IAM_DATABASE_CA_FILE':str(cert),'BROWSER_IAM_WORKSPACE_ID':'11111111-1111-4111-8111-111111111111','BROWSER_IAM_WORKSPACES':json.dumps([{'id':'11111111-1111-4111-8111-111111111111','label':'管理工作空间 A'},{'id':'12121212-1212-4212-8212-121212121212','label':'管理工作空间 B'}]),'NODE_EXTRA_CA_CERTS':str(cert),'MANAGEMENT_WEB_CERT':str(cert),'MANAGEMENT_WEB_KEY':str(key),'NEXT_TELEMETRY_DISABLED':'1','AGENT_TASKS_UI_WRITE_ENABLED':'false','AGENT_TASKS_WRITE_ENABLED':'false'}
 if RUNTIME_LOGS:web['MANAGEMENT_RUNTIME_LOG_FIXTURE_ACK']='ISOLATED_RUNTIME_LOG_READONLY'
 if MAIN_SERVICE:web['MANAGEMENT_MAIN_SERVICE_FIXTURE_ACK']='ISOLATED_MAIN_SERVICE_READONLY'
 if COMPAT:web['MANAGEMENT_MAIN_SERVICE_COMPAT_FIXTURE_ACK']='ISOLATED_MAIN_SERVICE_COMPAT_READONLY'
 if USER_PRODUCT:web['MANAGEMENT_USER_PRODUCT_FIXTURE_ACK']='ISOLATED_USER_PRODUCT_READONLY'
 if PRODUCT:
  if DIRECTORY:web['MANAGEMENT_DIRECTORY_PRODUCT_FIXTURE_ACK']='ISOLATED_DIRECTORY_PRODUCT_READONLY'
  web['BROWSER_IAM_WORKSPACES']=json.dumps([{'id':'11111111-1111-4111-8111-111111111111','label':'隔离合成空间 A'},{'id':'12121212-1212-4212-8212-121212121212','label':'隔离合成空间 B'},{'id':'13131313-1313-4313-8313-131313131313','label':'用户资料未接入的隔离空间 C' if USER_PRODUCT else '未接目录的隔离空间 C'}])
  if USER_PRODUCT:web['BROWSER_IAM_WORKSPACES']=json.dumps(json.loads(web['BROWSER_IAM_WORKSPACES'])+[{'id':'14141414-1414-4414-8414-141414141414','label':'来源未接入的隔离空间 D'}])
 (RUN/'web-env.json').write_text(json.dumps(web),encoding='utf-8')
 ctx=ssl.create_default_context(cafile=str(cert))
 for port in [IDP_PORT,API_PORT]:
  for _ in range(60):
   try:
    with urllib.request.urlopen(f'https://localhost:{port}'+('/health/ready' if (MAIN_SERVICE or COMPAT or PRODUCT) and port==API_PORT else '/healthz'),context=ctx,timeout=2) as response:
     if response.status==200:break
   except Exception:time.sleep(.25)
  else:raise RuntimeError('owned fixture endpoint unavailable; ledgers retained')
 retained=json.loads((RUN/'retained-before.json').read_text())
 verify_retained(retained,RUN/'retained-comparison.json',observer=metadata)
 print('Independent management PG, synthetic identity and query API prepared. Real sources UNCONFIGURED; business writes/models/dispatch absent.')
if __name__=='__main__':main()
