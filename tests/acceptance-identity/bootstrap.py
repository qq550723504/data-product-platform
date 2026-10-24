"""Reproducible NEW isolated 1A fixture. Existing resources cause refusal."""
import json,os,subprocess,ssl,time,urllib.request,urllib.parse,re
from pathlib import Path
from provision import ROOT,RUN,main as provision
def step(name,args,cwd=ROOT,env=None):
 with open(RUN/(name+'.log'),'w',encoding='utf-8') as output:
  p=subprocess.run(args,cwd=cwd,env={**os.environ,**(env or {})},stdout=output,stderr=output)
 if p.returncode:
  safe=(RUN/(name+'.log')).read_text(encoding='utf-8',errors='replace')
  profile=json.loads((RUN/'profile.json').read_text(encoding='utf-8'))
  secrets=[v for k,v in profile.items() if isinstance(v,str) and any(t in k.lower() for t in ['password','secret','encryptionkey'])]+[v['password'] for v in profile['people'].values()]
  for value in secrets:safe=safe.replace(value,'[REDACTED]')
  safe=re.sub(r'(?:postgres(?:ql)?://|Bearer |Basic )\S+','[REDACTED]',safe)
  safe=re.sub(r'eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+','[REDACTED]',safe)
  print('\n'.join(safe.splitlines()[-35:]),flush=True)
  raise RuntimeError(name+' failed; protected original log retained')
 print(name+' PASS',flush=True)
def wait(url):
 ctx=ssl.create_default_context(cafile=str(RUN/'ca.crt'));deadline=time.monotonic()+120
 while time.monotonic()<deadline:
  try:
   with urllib.request.urlopen(url,context=ctx,timeout=3) as response:
    if response.status==200:return
  except Exception:time.sleep(1)
 # Fixed owned fixture only, with generated secrets redacted before diagnostics.
 p=json.loads((RUN/'profile.json').read_text(encoding='utf-8'))
 logs=subprocess.run(['docker','logs','--tail','30',p['keycloak']],capture_output=True,encoding='utf-8')
 safe=logs.stdout+logs.stderr
 for key,value in p.items():
  if isinstance(value,str) and any(t in key.lower() for t in ['password','secret','encryptionkey']):safe=safe.replace(value,'[REDACTED]')
 for value in p['people'].values():safe=safe.replace(value['password'],'[REDACTED]')
 print(safe,flush=True)
 raise RuntimeError('isolated endpoint did not become ready; no automatic restart')
def main():
 if os.environ.get('ACCEPTANCE_IDENTITY_PREPARE_ACK')!='NEW_ISOLATED_1A_ONLY':raise RuntimeError('new isolated fixture authorization required')
 provision();p=json.loads((RUN/'profile.json').read_text(encoding='utf-8'))
 wait(p['issuer']+'/.well-known/openid-configuration')
 ack={'ACCEPTANCE_IDENTITY_ACK':'ISOLATED_READONLY_1A'}
 step('idp-configure',['python' if os.name=='nt' else 'python3',str(ROOT/'tests/acceptance-identity/configure-idp.py')],env=ack)
 seed=RUN/('prepare.exe' if os.name=='nt' else 'prepare')
 step('seed-build',['go','build','-o',str(seed),'./tests/acceptanceidentityprepare'],ROOT/'apps/platform')
 admin='postgres://postgres:'+urllib.parse.quote(p['pgPassword'],safe='')+'@127.0.0.1:25495/'+p['database']
 pgdsn=admin+'?sslmode=verify-full&sslrootcert='+urllib.parse.quote(str(RUN/'ca.crt'),safe='')
 step('seed',[str(seed)],env={'ACCEPTANCE_IDENTITY_PREPARE_ACK':'NEW_ISOLATED_1A_ONLY','ACCEPTANCE_IDENTITY_PREPARE_DSN':pgdsn})
 py='python' if os.name=='nt' else 'python3'
 step('runtime-configure',[py,str(ROOT/'tests/acceptance-identity/configure-runtime.py')],env=ack)
 step('api-start',[py,str(ROOT/'tests/acceptance-identity/start-api.py')],env=ack)
 wait('https://localhost:18498/healthz')
 web=ROOT/'apps/web';snapshot=web/'.tsconfig.acceptance-identity-1a.json'
 snapshot.write_text((web/'tsconfig.json').read_text(encoding='utf-8'),encoding='utf-8')
 buildenv={'NEXT_DIST_DIR':'.next-acceptance-identity-1a','NEXT_TSCONFIG_PATH':snapshot.name}
 step('web-build',['cmd','/c','npm','run','build'] if os.name=='nt' else ['npm','run','build'],web,buildenv)
 step('web-start',[py,str(ROOT/'tests/acceptance-identity/start-web.py')],env=ack)
 wait(p['origin']+'/auth/login')
 step('unit-db',['docker','exec',p['postgres'],'psql','-U','postgres','-d','postgres','-v','ON_ERROR_STOP=1','-c','CREATE DATABASE acceptance_1a_unit'])
 iamenv={'IAM_TEST_POSTGRES_DSN':admin.replace('/'+p['database'],'/acceptance_1a_unit'),'IAM_TEST_POSTGRES_CA':str(RUN/'ca.crt')}
 step('iam-pg',['cmd','/c','npm','run','test:iam'] if os.name=='nt' else ['npm','run','test:iam'],web,iamenv)
 step('authority-pg',['go','test','./internal/platform/principal/bindings','-count=1','-v'],ROOT/'apps/platform',{'TEST_POSTGRES_DSN':pgdsn})
 step('responsibility-pg',['go','test','./internal/contract/infrastructure','-run','TestResponsibility','-count=1','-v'],ROOT/'apps/platform',{'ACCEPTANCE_IDENTITY_TEST_DSN':pgdsn})
 print('Fresh 1A fixture ready. No enterprise IdP, model, business write route or real execution enabled.')
if __name__=='__main__':main()
