"""Actual cmd/api children; private materials and synthetic targets only."""
import copy,json,os,re,socket,ssl,subprocess,sys,time,urllib.request
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parent))
import bootstrap as b
if not b.DIRECTORY or os.environ.get('MANAGEMENT_FIXTURE_ACK')!='ISOLATED_READONLY_MANAGEMENT':raise RuntimeError('independent directory product fixture required')
p=json.loads((b.RUN/'profile.json').read_text(encoding='utf-8'))
if p['database']!='management_directory_product_20261026' or p['issuer']!='https://localhost:19180':raise RuntimeError('fixed isolated target required')
original=json.loads((b.RUN/'product-profile.json').read_text(encoding='utf-8'))
binary=b.RUN/('api-product.exe' if os.name=='nt' else 'api-product')
name=os.environ.get('DIRECTORY_PRODUCT_CONTRACT_OUTPUT','product-contract-results')
if not re.fullmatch(r'product-contract-results(?:-[a-z0-9-]+)?',name):raise RuntimeError('bounded evidence name required')
output=b.RUN/(name+'.json')
if output.exists():raise RuntimeError('previous contract evidence retained')
results=[];child=None
def candidate():
 c=copy.deepcopy(original)
 with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
 c['listen']='127.0.0.1:'+str(port)
 return c,port
def write_profile(c,label):
 f=b.RUN/(name+'-'+label+'-private.json');f.write_text(json.dumps(c),encoding='utf-8');return f
def fail(message):raise RuntimeError(message)
try:
 for fault in ['partial','synthetic','provider','duplicate-workspace','zero-workspace','write-role','missing-credential','invalid-ca','users-unopened','logs-unopened','fixture-switch']:
  c,_=candidate();d=c['directory'];binding=d['workspaces'][0]
  if fault=='partial':c['directory']={}
  elif fault=='synthetic':d['source']='SYNTHETIC_FIXTURE'
  elif fault=='provider':d['provider']='AUTO'
  elif fault=='duplicate-workspace':d['workspaces'].append(copy.deepcopy(binding))
  elif fault=='zero-workspace':binding['workspaceId']='00000000-0000-0000-0000-000000000000'
  elif fault=='write-role':binding['roles'][0]['permissions']=['management.members.write']
  elif fault=='missing-credential':binding['credentialFile']=str(b.RUN/'missing-private-credential')
  elif fault=='invalid-ca':binding['caFile']=str(b.RUN/'directory-credential')
  elif fault=='users-unopened':d['users']=True
  elif fault=='logs-unopened':c['runtimeSources']={}
  elif fault=='fixture-switch':c['directoryProduct']=True
  f=write_profile(c,fault)
  r=subprocess.run([str(binary)],env={**b.env_clean(),'PLATFORM_MANAGEMENT_PROFILE':str(f)},capture_output=True,timeout=5)
  (b.RUN/(name+'-'+fault+'-private.log')).write_bytes(r.stdout+r.stderr)
  if r.returncode!=1 or b'management query-only composition unavailable' not in r.stdout+r.stderr:fail('candidate did not reject '+fault)
  results.append({'name':'actual main rejects '+fault,'status':'PASS'})
 # A valid product process with the optional source omitted must stay source-less.
 c,port=candidate();c['directory']=None;f=write_profile(c,'disabled')
 with open(b.RUN/(name+'-disabled-private.log'),'wb') as log:
  child=subprocess.Popen([str(binary)],env={**b.env_clean(),'PLATFORM_MANAGEMENT_PROFILE':str(f)},stdout=log,stderr=log,creationflags=getattr(subprocess,'CREATE_NO_WINDOW',0))
 context=ssl.create_default_context(cafile=p['cert'])
 for _ in range(50):
  try:
   with urllib.request.urlopen(f'https://localhost:{port}/health/ready',context=context,timeout=1) as r:
    if r.status==200:break
  except Exception:
   if child.poll() is not None:fail('source-less product process exited')
   time.sleep(.1)
 else:fail('source-less product process did not become ready')
 results.append({'name':'actual main accepts an omitted directory without enabling optional sources','status':'PASS'})
except Exception as e:
 results.append({'name':'product entrypoint contract','status':'FAIL','error':type(e).__name__});raise
finally:
 if child is not None:
  child.terminate();child.wait(timeout=5) # Only the retained Popen child handle.
 output.write_bytes((json.dumps({'entrypoint':'cmd/api','results':results,'realSources':'UNCONFIGURED','businessWrites':False,'models':False,'dispatch':False},indent=2)+'\n').encode())
print('PASS actual product entrypoint contracts:',len(results))
