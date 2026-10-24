from pathlib import Path
import json,hashlib,datetime,sys
root=Path(__file__).resolve().parents[5];out=Path(__file__).parent
sys.path.insert(0,str(root/'examples/lifecycle-d1'));import fixture_artifacts as f
new=Path('C:/Users/Henry/.codex/fixture-private/candidate20-artifact-rehearsal02');old=Path('C:/Users/Henry/.codex/fixture-private/candidate20-artifact-rehearsal01');clone=Path('C:/Users/Henry/.codex/fixture-private/candidate20-schema-compatibility01')
planning=root/'docs/acceptance/evidence/issue307-d1/candidate20-planning01'
a=f.load(planning/'artifact-build02/actual-build.json');e=f.load(planning/'artifact-build02/equivalence.json');se=f.load(planning/'artifact-build02/schema-proof-equivalence02.json');spec=f.specification(root,execution='5a54e01a-657e-4b8a-ae67-a2d6fd5be8f6',profile='candidate20/v30',start='2026-10-22T10:00:00Z',end='2026-10-22T10:40:00Z')
f.verify(root,new,a,dict(execution=spec['execution'],profile=spec['profile'],start=spec['not_before'],end=spec['not_after']))
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
meta=lambda p:{'sha256':sha(p),'bytes':p.stat().st_size}
need=f.need
need(a['source']==spec['source']==e['runtime_source']==se['current_runtime_source']=='72ec972570a6f10a916ace39d90ab0c5593e8dd4','final R')
need(a['host']['source']==e['host_source']=='c79ad0c15062304e959d6dd1fc8baa6362abcf67','final P')
old_a=f.load(planning/'artifact-rehearsal01/actual-build.json')
need(a['build_files']==e['actual_binary_and_log_sha256']==old_a['build_files'],'twelve map equality')
for name,digest in a['build_files'].items():need(sha(new/name)==sha(old/name)==digest,'actual twelve byte equality '+name)
for name,digest in se['schema_sources_identical'].items():
 prior=f.git(root,'show','27cb345c48ea770b9565d8f76ab2b44c5af0b304:'+name);current=f.git(root,'show',spec['source']+':'+name)
 need(prior==current and hashlib.sha256(current).hexdigest()==digest,'schema source equality')
need(sha(clone/'tools/fixture-schema.test')==se['original_actual_test_binary_sha256'],'original native schema binary')
need(sha(planning/'artifact-rehearsal01/terminal.json')==e['prior_native_proof_sha256']==se['original_native_terminal_sha256'],'prior native terminal binding')
changed=f.git(root,'diff','--name-only','27cb345c48ea770b9565d8f76ab2b44c5af0b304',spec['source']).decode().splitlines()
need(changed==e['delta_files']==['apps/platform/cmd/lifecycle-fixture-readonly-check/six_test.go'],'only test expectation delta')
ci=f.load(planning/'ci-rehearsal03/result.json');raw=(planning/'ci-rehearsal03/stdout.txt').read_bytes()
need(ci['exit']==0 and b'PASS_SEPARATE_R_ARCHIVE_BUILD_AND_OFFLINE_CONTRACTS' in raw and sum(l.startswith(b'ok  \t') for l in raw.splitlines())==5,'CI R archive unit contract')
repo=(root/'apps/platform/internal/lifecycle/infrastructure/postgres_repository.go').read_text();need("c.identity_kind='PROVIDER_VERSION'" in repo,'product invariant')
result={'status':'PASS_INDEPENDENT_FINAL_R_P_ARTIFACT_EQUIVALENCE_AUDIT','at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'source_head':a['host']['source'],'runtime_source':a['source'],'runtime_spec_sha256':sha(root/f.SPEC),'archive':{'members':len(spec['files']),'bytes':spec['archive_bytes'],'sha256':spec['archive_sha256']},'host_tracked_files':len(a['host']['files']),'twelve_build_members_all_equal':True,'schema_sources_all_equal':True,'schema_native_binary_sha256':se['original_actual_test_binary_sha256'],'native_witness_source_label_retained':'27cb345c48ea770b9565d8f76ab2b44c5af0b304','equivalence_scope':'New R has only one test expectation delta; all four actual production binaries and eight original build output bytes equal the native-tested prior build; exact schema gate and test sources/program unchanged. No source migration or new native execution claimed.','private_artifacts':{str(new/name):meta(new/name) for name in a['build_files']},'source_migration':False,'provider_delete':0,'reviewer_source_or_docker_io':0,'reviewer_path_correction':'First in-memory attempt used nonexistent artifact-build02 private path and failed before any I/O other than regular-file check; actual retained private directory is artifact-rehearsal02.'}
result['private_artifacts'][str(new/'approved-runtime-source.zip')]=meta(new/'approved-runtime-source.zip')
for name in ['independent-final-R-pilot01.stdout.txt','independent-final-R-readonly-check01.stdout.txt']:
 b=(out/name).read_bytes();result.setdefault('independent_go_counts',{})[name]={'top_level_passes':sum(l.startswith(b'--- PASS:') for l in b.splitlines()),'nested_subcase_passes':sum(l.lstrip().startswith(b'--- PASS:') and not l.startswith(b'--- PASS:') for l in b.splitlines()),'note':'Original regression passed_methods field counts every PASS line including nested subcases; these are separate counts.'}
(out/'independent-final-artifact-equivalence01.json').open('xb').write((json.dumps(result,indent=2)+'\n').encode())
print(json.dumps({k:v for k,v in result.items() if k!='private_artifacts'},indent=2))
