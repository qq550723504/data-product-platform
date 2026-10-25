from pathlib import Path
import hashlib,json,subprocess,sys,datetime
root=Path(__file__).resolve().parents[5]
out=Path(__file__).parent
planning=root/'docs/acceptance/evidence/issue307-d1/candidate20-planning01'
public=planning/'artifact-rehearsal01'
private=Path('C:/Users/Henry/.codex/fixture-private/candidate20-artifact-rehearsal01')
clone=Path('C:/Users/Henry/.codex/fixture-private/candidate20-schema-compatibility01')
def load(path):return json.loads(path.read_bytes())
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def metadata(path):return {'sha256':sha(path),'bytes':path.stat().st_size}
def need(value,why):
 if not value:raise ValueError(why)
spec_raw=subprocess.check_output(['git','-c','core.longpaths=true','show','a23316fec0e3085475ee84b5de956ab9123b298c:examples/lifecycle-d1/fixture-runtime-artifact.json'],cwd=root)
spec=json.loads(spec_raw)
a=load(public/'actual-build.json');terminal=load(public/'terminal.json')
need(a['source']==spec['source']==terminal['runtime_source'],'R source cross-binding')
need(a['runtime_artifact_spec_sha256']==hashlib.sha256(spec_raw).hexdigest(),'spec binding')
need(a['build_cwd_relative']=='build-runtime/apps/platform' and a['build_environment']=={'GOWORK':'off','CGO_ENABLED':'0','GOARCH':'amd64'},'build recipe')
need(set(a['build_files'])=={'reader','operator','owner','readonly-check.exe',*[n+'-build.'+c for n in ['reader','operator','owner','checker'] for c in ['stdout','stderr']]},'original12')
for name,digest in a['build_files'].items():need(sha(private/name)==digest,'original build byte drift '+name)
need(sha(private/'approved-runtime-source.zip')==spec['archive_sha256'] and (private/'approved-runtime-source.zip').stat().st_size==spec['archive_bytes'],'archive')
for name,digest in spec['files'].items():need(sha(private/'build-runtime'/name)==digest,'extracted R drift '+name)
need(sha(clone/'tools/fixture-schema.test')==terminal['schema_test_sha256'],'native schema binary')
old=load(root/'docs/acceptance/evidence/issue307-d1/remaining-candidate19/candidate.json')
sources={g[k] for g in old['Groups'] for k in ['Core','Native']}
clone_cid=load(clone/'allocation.json')['cid']
intents=sorted((private/'journal').glob('*.intent.json'))
closed=[];schema=[];counts={};native_calls=[];private_files={}
for path in intents:
 key=path.name.split('.')[0];args=load(path)['args'];process=load(path.with_name(key+'.process.json'))
 for suffix in ['intent.json','process.json','stdout.private','stderr.private']:
  p=path.with_name(key+'.'+suffix);private_files[str(p)]=metadata(p)
 need(args[0]=='docker','unexpected command')
 counts[args[1]]=counts.get(args[1],0)+1
 if args[1]=='inspect':
  item=load(path.with_name(key+'.stdout.private'))[0]
  if item['Id'] in sources:
   need(item['State']['Running'] is False and item['State']['Pid']==0 and item['HostConfig']['RestartPolicy']['Name']=='no' and not item['HostConfig']['PortBindings'] and not item['NetworkSettings']['Ports'],'actual source not closed')
   closed.append(item['Id'])
 elif args[1] in ['start','stop']:
  need(clone_cid in args and not sources.intersection(args),'source start/stop')
 elif args[1]=='exec':
  need(clone_cid in args and not sources.intersection(args),'source exec')
  if '/run/tools/fixture-schema.test' in args:
   expected=next(x.split('=',1)[1] for x in args if x.startswith('D1_SCHEMA_EXPECT='))
   need(expected in ['ALLOW','DENY'] and process['exit']==0,'native test failure')
   raw=path.with_name(key+'.stdout.private').read_bytes()
   need(b'--- PASS: TestRemainingFixtureSchemaNativePostgres' in raw,'native gate did not run')
   schema.append(expected)
 else:raise ValueError('unexpected operation '+args[1])
 need(process['exit']==0 or 'pg_isready' in args,'unexpected command failure')
need(len(closed)==12 and set(closed)==sources,'six source CID before and after membership')
need(schema.count('ALLOW')==3 and schema.count('DENY')==3,'native six gates')
query=(public/'R-repository-query.sql').read_text()
Rrepo=(private/'build-runtime/apps/platform/internal/lifecycle/infrastructure/postgres_repository.go').read_text()
start=Rrepo.index('SELECT c.id,c.workspace_id,c.dataset_version_id')
need(query==Rrepo[start:Rrepo.index(chr(96),start)] and 'identity_kind' not in query,'actual R SQL')
P=(root/'apps/platform/internal/lifecycle/infrastructure/postgres_repository.go').read_text()
need("c.identity_kind='PROVIDER_VERSION'" in P,'P product protected invariant removed')
groups=[]
for g in old['Groups']:
 folder=public/g['Group'];before=load(folder/'old-facts-before.json');after=load(folder/'old-facts-after.json')
 need(before==after and len(before)==194,'194 old table projection')
 targets=[cp for cp in g['Copies'] if cp['Key'] in ['fixture/p2.csv','fixture/t1.csv','fixture/t2.csv','fixture/h2-a.csv','fixture/m1-a.csv','fixture/m1-b.csv']]
 for i,cp in enumerate(targets):need((folder/f'copy-{i}.stdout.txt').read_bytes().count(cp['ID'].encode())==1,'exact copy result')
 for expect in ['ALLOW','DENY']:
  raw=(folder/(expect+'-schema-gate.stdout.txt')).read_bytes()
  need(b'--- PASS: TestRemainingFixtureSchemaNativePostgres' in raw and (folder/(expect+'-schema-gate.stderr.txt')).read_bytes()==b'','public native schema stdout')
 groups.append({'group':g['Group'],'exact_copies':len(targets),'unchanged_original_relations':len(before)})
need(sum(g['exact_copies'] for g in groups)==6,'sixcopy membership')
need(terminal['status']=='PASS_ISOLATED_R_P_ARTIFACT_AND_SCHEMA_CONTRACT' and terminal['failure'] is None and terminal['source_migration'] is False and terminal['native_delete']==0 and terminal['sources_remained_closed'] and terminal['clone_closed'],'terminal')
result={'status':'PASS_INDEPENDENT_R_P_ARTIFACT_CLONE_EVIDENCE_AUDIT','at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'runtime_source':spec['source'],'rehearsal_host_source':a['host']['source'],'archive':{'sha256':spec['archive_sha256'],'bytes':spec['archive_bytes'],'members':len(spec['files'])},'build_files':a['build_files'],'groups':groups,'schema_gates':{'ALLOW':schema.count('ALLOW'),'DENY':schema.count('DENY')},'source_closed_observations':len(closed),'command_counts':counts,'journal_commands':len(intents),'source_start_exec':0,'source_migration':False,'provider_delete':0,'reviewed_spec_from_commit':'a23316fec0e3085475ee84b5de956ab9123b298c','new_R72ec_test_only_delta_not_covered_by_historical_native_source_claim':True,'scope':'Saved actual evidence audit only. No source/clone/container operation performed by reviewer. Original role/ACL preservation not claimed by --no-owner --no-acl clone restore. Native build witness used earlier P a233; R unchanged. Final P CI additions reviewed separately.','public_artifacts':{p.relative_to(root).as_posix():metadata(p) for p in sorted(public.rglob('*')) if p.is_file()},'private_artifacts':private_files}
for name in a['build_files']:result['private_artifacts'][str(private/name)]=metadata(private/name)
for p in [private/'approved-runtime-source.zip',clone/'tools/fixture-schema.test']:result['private_artifacts'][str(p)]=metadata(p)
path=out/'independent-artifact-evidence-audit02.json'
with path.open('xb') as stream:stream.write((json.dumps(result,indent=2)+'\n').encode())
print(json.dumps({k:v for k,v in result.items() if k not in ['private_artifacts','public_artifacts','build_files']},indent=2))
