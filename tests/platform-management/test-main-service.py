"""Bounded Go/PG regression on the new synthetic database, never the original."""
import json,os,re,subprocess,sys,urllib.parse
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parent))
import bootstrap as b

if not (b.MAIN_SERVICE or b.COMPAT or b.PRODUCT) or os.environ.get('MANAGEMENT_FIXTURE_ACK')!='ISOLATED_READONLY_MANAGEMENT':raise RuntimeError('new isolated main service fixture required')
p=json.loads((b.RUN/'profile.json').read_text(encoding='utf-8'))
u=urllib.parse.urlparse(p['adminDSN'])
if u.hostname!='127.0.0.1' or u.port!=b.PG_PORT or u.path!='/'+b.DATABASE or u.username!='postgres':raise RuntimeError('isolated test target mismatch')
name=os.environ.get('MANAGEMENT_MAIN_SERVICE_GO_OUTPUT','go-regression')
if not re.fullmatch(r'go-regression(?:-[a-z0-9-]+)?',name):raise RuntimeError('invalid regression evidence name')
output=b.RUN/(name+'.json')
if output.exists():raise RuntimeError('previous Go regression evidence retained')
ack={'MANAGEMENT_USER_PRODUCT_FIXTURE_ACK':'ISOLATED_USER_PRODUCT_READONLY'} if b.USER_PRODUCT else {'MANAGEMENT_DIRECTORY_PRODUCT_FIXTURE_ACK':'ISOLATED_DIRECTORY_PRODUCT_READONLY'} if b.DIRECTORY else {'MANAGEMENT_MAIN_SERVICE_COMPAT_FIXTURE_ACK':'ISOLATED_MAIN_SERVICE_COMPAT_READONLY'} if b.COMPAT else {'MANAGEMENT_MAIN_SERVICE_FIXTURE_ACK':'ISOLATED_MAIN_SERVICE_READONLY'}
env={**b.env_clean(),**ack,'MANAGEMENT_FIXTURE_ACK':'ISOLATED_READONLY_MANAGEMENT','MANAGEMENT_FIXTURE_CONFIG':str(b.RUN/'api-config.json'),'TEST_POSTGRES_DSN':p['adminDSN']}
r=subprocess.run(['go','test','-json','./internal/platformmanagement/...','./internal/platform/principal/bindings','./cmd/api','./tests/platformmanagement','-count=1'],cwd=b.ROOT/'apps/platform',env=env,capture_output=True,encoding='utf-8',timeout=180)
(b.RUN/(name+'-private.log')).write_text(r.stdout+'\n'+r.stderr,encoding='utf-8')
packages={};failed=[]
for line in r.stdout.splitlines():
 try:event=json.loads(line)
 except ValueError:continue
 action=event.get('Action');package=event.get('Package','').removeprefix('github.com/qq550723504/data-product-platform/apps/platform/')
 if event.get('Test') and action in ('pass','fail','skip'):
  totals=packages.setdefault(package,{'pass':0,'fail':0,'skip':0});totals[action]+=1
 if not event.get('Test') and action=='fail':failed.append(package)
result={'verdict':'PASS' if r.returncode==0 else 'FAIL','packages':packages,'failedPackages':failed,'realSources':'UNCONFIGURED','businessWrites':False,'models':False,'dispatch':False}
output.write_text(json.dumps(result,indent=2)+'\n',encoding='utf-8');print(json.dumps(result))
if r.returncode:raise RuntimeError('isolated Go regression failed; private diagnostics retained')
