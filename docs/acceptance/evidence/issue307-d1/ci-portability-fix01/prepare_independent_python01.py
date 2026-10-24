"""Audit current tests then export only code + observed immutable fixtures."""
import datetime,hashlib,json,os,shutil,subprocess,sys
from pathlib import Path
root=Path(__file__).resolve().parents[5];out=Path(__file__).parent
private=Path.home()/'.codex/fixture-private/ci-portability-fix01'
tests=('retention_renewal','retention_maintenance','retention_composition','retention_staging','verify_six_native','fixture_artifacts','evidence_indexes','successor_dependencies','retired_budget_bytes')
observed=set();results=[]
env=os.environ.copy();env.update(PYTHONUTF8='1',PYTHONIOENCODING='utf-8',PYTHONDONTWRITEBYTECODE='1')
for key in ('POSTGRES_DSN','TEST_POSTGRES_DSN','REDIS_ADDR','D1_SCHEMA_TEST_DSN','D1_SCHEMA_EXPECT'):env.pop(key,None)
folder=out/'windows-audited-preparation01';folder.mkdir(exist_ok=False)
for test in tests:
 for optimized in (False,True):
  name=test+('-O' if optimized else '-normal');audit=folder/(name+'-audit.json')
  args=[sys.executable,*(['-O'] if optimized else []),'-B',str(out/'offline_python_runner01.py'),'--root',str(root),'--test','test_'+test+'.py','--audit',str(audit)]
  result=subprocess.run(args,cwd=root,env=env,capture_output=True,timeout=180)
  for suffix,raw in [('stdout',result.stdout),('stderr',result.stderr)]:
   with (folder/(name+'.'+suffix)).open('xb') as stream:stream.write(raw)
  record=json.loads(audit.read_bytes());observed.update(record['reads']);results.append(dict(name=name,returncode=result.returncode,denied=record['forbidden_host_reads']))
  print(name+': '+str(result.returncode),flush=True)
  if result.returncode or record['forbidden_host_reads']:raise ValueError('offline fixture test failed; original evidence preserved')
# All top-level fixture modules remain importable without sharing a pycache.
tracked=subprocess.check_output(['git','-c','core.longpaths=true','ls-files','--','examples/lifecycle-d1'],cwd=root).decode().splitlines()
members={name for name in tracked if Path(name).parent.as_posix()=='examples/lifecycle-d1' and Path(name).suffix in ('.py','.json')}
for name in observed:
 if '__pycache__' not in name and (root/name).is_file():members.add(name)
snapshot=private/'python-export01';snapshot.mkdir(exist_ok=False)
files={}
for name in sorted(members):
 source=root/name
 if source.is_symlink() or source.is_junction() or source.stat().st_nlink!=1:raise ValueError('snapshot source not regular')
 target=snapshot/name;target.parent.mkdir(parents=True,exist_ok=True);raw=source.read_bytes()
 with target.open('xb') as stream:stream.write(raw)
 files[name]=dict(sha256=hashlib.sha256(raw).hexdigest(),bytes=len(raw))
shutil.copyfile(out/'offline_python_runner01.py',snapshot/'offline_python_runner01.py')
value=dict(status='PASS_WINDOWS_AUDITED_TESTS_AND_INDEPENDENT_EXPORT',snapshot=str(snapshot),members=len(files),files=files,results=results,tests=list(tests),retained_source_start=0,provider_io=0)
with (out/'independent-python-export01.json').open('x',encoding='utf-8',newline='\n') as stream:json.dump(value,stream,indent=2)
print(json.dumps(dict(status=value['status'],members=len(files),bytes=sum(v['bytes'] for v in files.values()),snapshot=str(snapshot))))
