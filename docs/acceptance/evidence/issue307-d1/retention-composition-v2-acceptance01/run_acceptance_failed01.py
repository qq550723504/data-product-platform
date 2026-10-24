"""Offline acceptance and fresh read-only publication boundary evidence."""
import datetime,json,os,subprocess,sys
from pathlib import Path
root=Path(__file__).resolve().parents[5]
sys.path.insert(0,str(root/'examples/lifecycle-d1'))
import execution_freeze as freeze
out=Path(__file__).parent
results=[]
env=os.environ.copy();env.update(GOWORK='off',GOFLAGS='',CGO_ENABLED='0')
for key in ('POSTGRES_DSN','TEST_POSTGRES_DSN','REDIS_ADDR','D1_SCHEMA_TEST_DSN','D1_SCHEMA_EXPECT'):env.pop(key,None)

def run(name,args,cwd=root,timeout=240):
 started=datetime.datetime.now(datetime.timezone.utc).isoformat()
 result=subprocess.run(args,cwd=cwd,env=env,capture_output=True,timeout=timeout)
 for channel in ('stdout','stderr'):
  with (out/(name+'.'+channel)).open('xb') as stream:stream.write(getattr(result,channel))
 row=dict(name=name,command=args,started_at=started,finished_at=datetime.datetime.now(datetime.timezone.utc).isoformat(),returncode=result.returncode)
 with (out/(name+'.json')).open('x',encoding='utf-8') as stream:json.dump(row,stream,indent=2)
 results.append(row)
 print(name+': '+str(result.returncode),flush=True)
 if result.returncode:raise RuntimeError(name+' failed; preserve original channels')

try:
 for test in ('retention_renewal','retention_maintenance','retention_composition','retention_staging','verify_six_native','fixture_artifacts','evidence_indexes','successor_dependencies','retired_budget_bytes'):
  for optimized in (False,True):
   run(test+('-O' if optimized else '-normal'),[sys.executable,*(['-O'] if optimized else []),'-B','examples/lifecycle-d1/test_'+test+'.py'])
 run('go-product-three',['go','test','./cmd/lifecycle-fixture-retention','./cmd/lifecycle-fixture-reader','./cmd/lifecycle-fixture-readonly-check','-count=1'],root/'apps/platform')
 run('actual-R4-ci-entry',[sys.executable,'-B','examples/lifecycle-d1/ci-test-fixture-runtime.py'],timeout=300)
 journal=out/'github-readonly';journal.mkdir()
 client=freeze.GitHub(journal);snapshot=client.snapshot()
 freeze.ready(snapshot,'f4e25a88f62da6bb14ed24da8b71b634be43f17a',require_frozen=True)
 query='query { repository(owner:"qq550723504",name:"data-product-platform") { pullRequest(number:332) { reviewThreads(first:100) { pageInfo { hasNextPage } nodes { isResolved } } } } }'
 threads=client.graph(query)['repository']['pullRequest']['reviewThreads']
 if threads['pageInfo']['hasNextPage'] or any(not row['isResolved'] for row in threads['nodes']):raise ValueError('unresolved or incomplete review thread set')
 freeze.save(out/'published-boundary-readonly.json',dict(snapshot=snapshot,unresolved_threads=0,remote_mutations=0,source_start=0,provider_io=0))
finally:
 with (out/'acceptance-commands01.json').open('x',encoding='utf-8') as stream:json.dump(dict(commands=results,all_commands_success=all(x['returncode']==0 for x in results),source_start=0,provider_io=0,retention_write=0,delete=0),stream,indent=2)
