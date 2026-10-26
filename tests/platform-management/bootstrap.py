"""NEW independent synthetic management fixture. Never reuses original DB/session/IdP."""
import json,os,secrets,subprocess,socket,ssl,time,urllib.parse,urllib.request
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
RUNTIME_LOGS=os.environ.get('MANAGEMENT_RUNTIME_LOG_FIXTURE_ACK')=='ISOLATED_RUNTIME_LOG_READONLY'
if os.environ.get('MANAGEMENT_RUNTIME_LOG_FIXTURE_ACK') and not RUNTIME_LOGS:raise RuntimeError('invalid runtime fixture acknowledgement')
RUN=ROOT/('.data/runtime-logs-api-20261026' if RUNTIME_LOGS else '.data/management-readonly-20261025')
PG='dpp-runtime-logs-readonly-20261026' if RUNTIME_LOGS else 'dpp-management-readonly-20261025'
PG_PORT,API_PORT,IDP_PORT,WEB_PORT=(25550,18550,19150,4350) if RUNTIME_LOGS else (25496,18499,19130,4300)
DATABASE='runtime_logs_readonly_20261026' if RUNTIME_LOGS else 'management_readonly_20261025'
def call(args,**kwargs):
 p=subprocess.run(args,capture_output=True,encoding='utf-8',**kwargs)
 if p.returncode:
  if RUN.exists():(RUN/'prepare-private.log').write_text(p.stdout+'\n'+p.stderr,encoding='utf-8')
  raise RuntimeError('isolated preparation failed in '+Path(args[0]).name+'; private diagnostics retained and withheld')
 return p.stdout
def metadata(name):
 p=subprocess.run(['docker','inspect',name],capture_output=True,encoding='utf-8')
 if p.returncode:return None
 d=json.loads(p.stdout)[0];return {'id':d['Id'],'status':d['State']['Status'],'mounts':d['Mounts'],'startedAt':d['State']['StartedAt']}
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
 (RUN/'retained-before.json').write_text(json.dumps({'original':metadata('dpp-agent-q2-test-postgres-20261023'),'management':metadata('dpp-management-readonly-20261025') if RUNTIME_LOGS else None}),encoding='utf-8')
 cert=RUN/'server.crt';key=RUN/'server.key';call(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-days','3','-keyout',str(key),'-out',str(cert),'-subj','/CN=localhost','-addext','subjectAltName=DNS:localhost,IP:127.0.0.1'])
 p={k:secrets.token_urlsafe(32) for k in ['pgPassword','apiPassword','rpPassword','rpSecret','resourceSecret','readCredential','control','encryptionKey']};p.update({'issuer':f'https://localhost:{IDP_PORT}','origin':f'https://localhost:{WEB_PORT}','runtimeLogs':RUNTIME_LOGS,'cert':str(cert),'key':str(key),'database':DATABASE,'postgres':PG})
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
 api={'DatabaseURL':dsn('management_api',p['apiPassword'])+tls,'Issuer':p['issuer'],'ResourceSecret':p['resourceSecret'],'ReadCredential':p['readCredential'],'Cert':str(cert),'Key':str(key),'CA':str(cert),'Root':str(ROOT),'RuntimeLogs':RUNTIME_LOGS,'APIPassword':p['apiPassword'],'RPPassword':p['rpPassword']}
 api_path=RUN/'api-config.json';api_path.write_text(json.dumps(api),encoding='utf-8')
 binary=RUN/('api.exe' if os.name=='nt' else 'api');call(['go','build','-o',str(binary),'./tests/platformmanagement'],cwd=ROOT/'apps/platform')
 base={'MANAGEMENT_FIXTURE_ACK':'ISOLATED_READONLY_MANAGEMENT','MANAGEMENT_FIXTURE_CONFIG':str(api_path)}
 call([str(binary),'prepare'],env={**env_clean(),**base,'MANAGEMENT_FIXTURE_ADMIN_DSN':p['adminDSN']})
 # API runtime configuration carries no admin DSN or RP password.
 api['APIPassword']='';api['RPPassword']='';api_path.write_text(json.dumps(api),encoding='utf-8')
 processes=[spawn('identity',['node',str(ROOT/'tests/platform-management/fixture.mjs')],{**base,'MANAGEMENT_FIXTURE_PROFILE':str(RUN/'profile.json')})]
 processes.append(spawn('api',[str(binary)],base));(RUN/'processes.json').write_text(json.dumps(processes),encoding='utf-8')
 web={'BROWSER_IAM_MODE':'oidc','BROWSER_IAM_PROFILE':'platform-management','BROWSER_IAM_ORIGIN':p['origin'],'BROWSER_IAM_ISSUER':p['issuer'],'BROWSER_IAM_CLIENT_ID':'dpp-management-rp','BROWSER_IAM_CLIENT_SECRET':p['rpSecret'],'BROWSER_IAM_AUTHORIZATION_ENDPOINT':p['issuer']+'/authorize','BROWSER_IAM_TOKEN_ENDPOINT':p['issuer']+'/token','BROWSER_IAM_JWKS_URI':p['issuer']+'/jwks','BROWSER_IAM_LOGOUT_ENDPOINT':p['issuer']+'/logout','BROWSER_IAM_PRINCIPAL_ENDPOINT':f'https://localhost:{API_PORT}/internal/identity/platform-management-principal','BROWSER_IAM_SCOPE':'openid platform-management','BROWSER_IAM_ENCRYPTION_KEY':p['encryptionKey'],'BROWSER_IAM_DATABASE_URL':dsn('management_rp',p['rpPassword']),'BROWSER_IAM_DATABASE_CA_FILE':str(cert),'BROWSER_IAM_WORKSPACE_ID':'11111111-1111-4111-8111-111111111111','BROWSER_IAM_WORKSPACES':json.dumps([{'id':'11111111-1111-4111-8111-111111111111','label':'管理工作空间 A'},{'id':'12121212-1212-4212-8212-121212121212','label':'管理工作空间 B'}]),'NODE_EXTRA_CA_CERTS':str(cert),'MANAGEMENT_WEB_CERT':str(cert),'MANAGEMENT_WEB_KEY':str(key),'NEXT_TELEMETRY_DISABLED':'1','AGENT_TASKS_UI_WRITE_ENABLED':'false','AGENT_TASKS_WRITE_ENABLED':'false'}
 if RUNTIME_LOGS:web['MANAGEMENT_RUNTIME_LOG_FIXTURE_ACK']='ISOLATED_RUNTIME_LOG_READONLY'
 (RUN/'web-env.json').write_text(json.dumps(web),encoding='utf-8')
 ctx=ssl.create_default_context(cafile=str(cert))
 for port in [IDP_PORT,API_PORT]:
  for _ in range(60):
   try:
    with urllib.request.urlopen(f'https://localhost:{port}/healthz',context=ctx,timeout=2) as response:
     if response.status==200:break
   except Exception:time.sleep(.25)
  else:raise RuntimeError('owned fixture endpoint unavailable; ledgers retained')
 retained=json.loads((RUN/'retained-before.json').read_text())
 assert metadata('dpp-agent-q2-test-postgres-20261023')==retained['original']
 if RUNTIME_LOGS:assert metadata('dpp-management-readonly-20261025')==retained['management']
 print('Independent management PG, synthetic identity and query API prepared. Real sources UNCONFIGURED; business writes/models/dispatch absent.')
if __name__=='__main__':main()
