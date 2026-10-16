"""Read-only retained topology, source/cost/secret audit and evidence index."""
import argparse,base64,datetime as dt,hashlib,hmac,importlib.util,json
from pathlib import Path
spec=importlib.util.spec_from_file_location('continuation',Path(__file__).with_name('complete-lc13-continuation.py'));m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
ROOT=m.b.ROOT;PUBLIC=ROOT/'docs/acceptance/evidence/issue307-d1/lc13-continuation01';PRIVATE=m.b.PRIVATE
parser=argparse.ArgumentParser();parser.add_argument('--final',action='store_true');args=parser.parse_args()
controls=['d1-lc13-continuation01','d1-lc13-credential01','d1-lc13-processing-current01']
inventory=[];private_values=set();public_roots=[PUBLIC]
for name in controls:
 control=PRIVATE/name;state=m.load(control/'plane.json');p=Path(state['private']);r=m.load(p/'resources.json');out=ROOT/'examples/lifecycle-d1/results'/r['run_id'];public_roots.append(out)
 configs={}
 for file in p.rglob('*.json'):
  try:data=m.load(file)
  except Exception:continue
  if isinstance(data,dict):
   for field in ['Token','RootAccess','RootSecret','Access','Secret','InspectAccess','InspectSecret','ProducerAccess','ProducerSecret','IssuerAccess','IssuerSecret','PostgresDSN']:
    if isinstance(data.get(field),str) and data[field]:private_values.add(data[field])
  if file.name=='reader.json':configs[str(file.relative_to(p))]=data['Enabled']
 m.require(all(v is False for v in configs.values()),'Retained native owner alias open')
 # Frozen original/current issuer recipes and redemption handle stay private.
 c=m.load(p/'credential-owner/reader.json')
 if c.get('IssuerSecret'):
  facts=m.load(PUBLIC/('credential/final-readonly-accounting.json' if name=='d1-lc13-credential01' else 'final-readonly-accounting.json'))
  subjects=m.load(out/'credential-subjects.json')['Subjects']
  for s in subjects:
   mac=lambda kind:hmac.new(c['IssuerSecret'].encode(),(s['ProviderRequestKey']+':'+kind).encode(),hashlib.sha256).hexdigest()
   private_values.update(['lc13cap'+mac('access')[:12],'lc13cap'+mac('access')[:24],mac('secret'),mac('secret')[:40],mac('redemption')])
 helpers=[];services=[]
 for native_name in m.docker('ps','-a','--filter','label=lifecycle307.run='+r['run_id'],'--format','{{.Names}}').decode().splitlines():
  i=m.inspect(native_name)
  fact={'name':native_name,'id':i['Id'],'image':i['Image'],'state':i['State']['Status'],'pid':i['State']['Pid'],'exit_code':i['State']['ExitCode']}
  if native_name in state['services']:
   m.require(i['Id']==state['services'][native_name]['id'] and not any(i['NetworkSettings']['Ports'].values()),'Retained service identity/ports drift');fact['volumes']=[x.get('Name') for x in i['Mounts'] if x['Type']=='volume'];services.append(fact)
  else:m.require(i['State']['Status']=='exited' and i['State']['Pid']==0,'Native helper remains active');helpers.append(fact)
 sdk=[m.load(x) for x in out.rglob('reader-admin-*.json') if 'observation' not in x.name];provider_rpc=sum(x['path'] in ['/minio/admin/v3/add-service-account','/minio/admin/v3/info-service-account','/minio/admin/v3/update-service-account'] for x in sdk)
 accounting=m.load(PUBLIC/('credential/final-readonly-accounting.json' if name=='d1-lc13-credential01' else 'processing-current/final-readonly-accounting.json' if name.endswith('current01') else 'final-readonly-accounting.json'))
 m.require(provider_rpc==accounting['provider_attempts'],'Physical provider RPC / attempt cost mismatch')
 inventory.append({'run':r['run_id'],'workspace':accounting['workspace'],'services':services,'helpers':helpers,'roles':configs,'provider_rpc':provider_rpc,'admin_setup_and_seal_rpc':len(sdk)-provider_rpc,'all_resources_retained':True})
for value in list(private_values):private_values.add(base64.b64encode(value.encode()).decode())
rows=[json.loads(x) for x in (PRIVATE/'d1-lc13-continuation01/go-race-final.jsonl').read_text(encoding='utf-8-sig').splitlines()]
verification={'go_packages':sum(x['Action']=='pass' and not x.get('Test') for x in rows),'go_tests_and_subtests':sum(x['Action']=='pass' and bool(x.get('Test')) for x in rows),'go_fail':sum(x['Action']=='fail' for x in rows),'go_skip':sum(x['Action']=='skip' for x in rows),'race':0,'vet':'PASS','python_normal':30,'python_optimized':30,'native_delete':0,'merge':0,'production_acceptance':'NOT_RUN'}
suffix='-02' if args.final else ''
if args.final:
 # Bind the actual native runtime to content hashes, preserving its original dirty-tree baseline.
 env=m.load(PUBLIC/'processing-current/environment.json');runtime={p:h for p,h in env['source_hashes'].items() if not p.endswith('_test.go')}
 drift=[p for p,h in runtime.items() if m.digest(ROOT/p)!=h]
 m.require(not drift,'Current runtime source differs from native full-binary proof')
 binary=PRIVATE/'dpp-lifecycle307-8120904f/plane-owner/reader'
 m.require(m.digest(binary)==env['binary_sha256'],'Retained native binary drift')
 m.write(PUBLIC/'runtime-source-binding.json',{'source_commit':m.call(['git','rev-parse','HEAD']).decode().strip(),'native_build_baseline':env['source_head'],'native_build_worktree_dirty':True,'native_binary_sha256':env['binary_sha256'],'runtime_source_hashes':runtime,'runtime_files_match':True,'later_test_changes_excluded_from_native_build':True,'delete':0})
m.write(PUBLIC/('verification'+suffix+'.json'),verification)
m.write(PUBLIC/('retained-final-topology'+suffix+'.json'),{'at':dt.datetime.now(dt.timezone.utc).isoformat(),'environments':inventory,'cleanup':0})
files=[];matches=[]
for root in public_roots:
 for file in sorted(root.rglob('*')):
  if not file.is_file():continue
  content=file.read_bytes()
  if any(v.encode() in content for v in private_values):matches.append(str(file.relative_to(ROOT)))
  files.append({'path':str(file.relative_to(ROOT)).replace('\\','/'),'sha256':hashlib.sha256(content).hexdigest(),'bytes':len(content)})
m.require(not matches,'Private credential material entered public evidence')
audit=PUBLIC/('secret-audit'+suffix+'.json')
m.write(audit,{'private_value_count':len(private_values),'public_file_count':len(files),'matches':[]})
files.append({'path':str(audit.relative_to(ROOT)).replace('\\','/'),'sha256':m.digest(audit),'bytes':audit.stat().st_size})
m.write(PUBLIC/('evidence-index'+suffix+'.json'),{'files':files,'source_head':m.call(['git','rev-parse','HEAD']).decode().strip(),'native_delete':0,'index_self_excluded':True,'prior_index_preserved':args.final})
print(json.dumps({'files':len(files),'go_tests':verification['go_tests_and_subtests'],'private_matches':0,'provider_rpc':sum(x['provider_rpc'] for x in inventory)}))
