"""Export the complete CI Python contract collection with exact fixtures."""
import hashlib,json,os,re,shutil,subprocess,sys
from pathlib import Path
root=Path(__file__).resolve().parents[5];out=Path(__file__).parent
private=Path.home()/'.codex/fixture-private/ci-portability-fix01'
tests=list(dict.fromkeys(re.findall(r'python -B examples/lifecycle-d1/(test_[a-z0-9_]+\.py)',(root/'.github/workflows/ci.yml').read_text())))
tests.append('../temporary_containers_test.py')
folder=out/'windows-full-audited-preparation05';folder.mkdir(exist_ok=False)
env=os.environ.copy();env.update(PYTHONUTF8='1',PYTHONIOENCODING='utf-8',PYTHONDONTWRITEBYTECODE='1')
for key in ('POSTGRES_DSN','TEST_POSTGRES_DSN','REDIS_ADDR','D1_SCHEMA_TEST_DSN','D1_SCHEMA_EXPECT'):env.pop(key,None)
observed=set();results=[]
for test in tests:
 for optimized in (False,True):
  name=Path(test).stem+('-O' if optimized else '-normal');audit=folder/(name+'-audit.json')
  previous=out/'windows-full-audited-preparation04'/(name+'-audit.json')
  if test!='../temporary_containers_test.py' and previous.is_file():
   record=json.loads(previous.read_bytes());observed.update(record['reads']);results.append(dict(name=name,returncode=record['code'],denied=record['forbidden_host_reads'],previous_audit=str(previous)));continue
  args=[sys.executable,*(['-O'] if optimized else []),'-B',str(out/'offline_python_runner01.py'),'--root',str(root),'--test',test,'--audit',str(audit)]
  result=subprocess.run(args,cwd=root,env=env,capture_output=True,timeout=180)
  for suffix,raw in [('stdout',result.stdout),('stderr',result.stderr)]:
   with (folder/(name+'.'+suffix)).open('xb') as stream:stream.write(raw)
  record=json.loads(audit.read_bytes());observed.update(record['reads']);results.append(dict(name=name,returncode=result.returncode,denied=record['forbidden_host_reads']))
  print(name+': '+str(result.returncode),flush=True)
if any(x['returncode'] or x['denied'] for x in results):raise ValueError('CI collection failed; original receipt preserved')
tracked=subprocess.check_output(['git','-c','core.longpaths=true','ls-files','--','examples'],cwd=root).decode().splitlines()
members={name for name in tracked if Path(name).parent.as_posix()=='examples/lifecycle-d1' and Path(name).suffix in ('.py','.json')}
members.update(name for name in observed if '__pycache__' not in name and (root/name).is_file())
snapshot=private/'python-export02';snapshot.mkdir(exist_ok=False);files={}
for name in sorted(members):
 source=root/name
 if source.is_symlink() or source.is_junction() or source.stat().st_nlink!=1:raise ValueError('snapshot source not regular')
 target=snapshot/name;target.parent.mkdir(parents=True,exist_ok=True);raw=source.read_bytes()
 with target.open('xb') as stream:stream.write(raw)
 files[name]=dict(sha256=hashlib.sha256(raw).hexdigest(),bytes=len(raw))
shutil.copyfile(out/'offline_python_runner01.py',snapshot/'offline_python_runner01.py')
with (snapshot/'collection.json').open('x',encoding='utf-8') as stream:json.dump(tests,stream)
value=dict(status='PASS_WINDOWS_COMPLETE_CI_PYTHON_AUDIT_AND_EXPORT',snapshot=str(snapshot),members=len(files),files=files,results=results,tests=tests,retained_source_start=0,provider_io=0)
with (out/'independent-python-export02.json').open('x',encoding='utf-8',newline='\n') as stream:json.dump(value,stream,indent=2)
print(json.dumps(dict(status=value['status'],members=len(files),snapshot=str(snapshot))))
