"""Run independent filesystem copies of the actual CI collection, no daemon."""
import concurrent.futures,datetime,json,os,platform,subprocess,sys
from pathlib import Path
root=Path(__file__).resolve().parent;folder=Path(sys.argv[1]);folder.mkdir(exist_ok=False)
tests=json.loads((root/'collection.json').read_bytes())
env=os.environ.copy();env.update(PYTHONUTF8='1',PYTHONIOENCODING='utf-8',PYTHONDONTWRITEBYTECODE='1')
for key in ('POSTGRES_DSN','TEST_POSTGRES_DSN','REDIS_ADDR','D1_SCHEMA_TEST_DSN','D1_SCHEMA_EXPECT'):env.pop(key,None)
def case(task):
 test,optimized=task;name=Path(test).stem+('-O' if optimized else '-normal');audit=folder/(name+'-audit.json')
 command=[sys.executable,*(['-O'] if optimized else []),'-B',str(root/'offline_python_runner01.py'),'--root',str(root),'--test',test,'--audit',str(audit)]
 start=datetime.datetime.now(datetime.timezone.utc).isoformat();result=subprocess.run(command,cwd=root,env=env,capture_output=True,timeout=180)
 for suffix,raw in [('stdout',result.stdout),('stderr',result.stderr)]:
  with (folder/(name+'.'+suffix)).open('xb') as stream:stream.write(raw)
 record=json.loads(audit.read_bytes())
 return dict(name=name,returncode=result.returncode,denied=record['forbidden_host_reads'],started_at=start)
with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
 results=list(pool.map(case,[(t,o) for t in tests for o in (False,True)]))
value=dict(status='PASS' if all(x['returncode']==0 and not x['denied'] for x in results) else 'FAIL',platform=platform.platform(),python=sys.version,root=str(root),collection=tests,results=results,retained_source_start=0,business_provider_io=0)
with (folder/'result.json').open('x',encoding='utf-8',newline='\n') as stream:json.dump(value,stream,indent=2)
print(json.dumps(dict(status=value['status'],commands=len(results),failed=[x for x in results if x['returncode'] or x['denied']]),ensure_ascii=False))
raise SystemExit(0 if value['status']=='PASS' else 1)
