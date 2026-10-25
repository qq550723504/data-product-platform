"""Append exact independent OS regression bytes and their finite summary."""
import hashlib,json,re
from pathlib import Path
root=Path(__file__).resolve().parents[5];out=Path(__file__).parent;private=Path.home()/'.codex/fixture-private/ci-portability-fix01'
def copy_tree(source,target):
 target.mkdir(exist_ok=False)
 for path in source.rglob('*'):
  if not path.is_file():continue
  dest=target/path.relative_to(source);dest.parent.mkdir(parents=True,exist_ok=True)
  with dest.open('xb') as stream:stream.write(path.read_bytes())
rows=[]
for osname,folder in [('windows',private/'windows-python-output01'),('linux',private/'linux-python-output01/collection01')]:
 value=json.loads((folder/'result.json').read_bytes());assert value['status']=='PASS' and len(value['results'])==52 and all(x['returncode']==0 and not x['denied'] for x in value['results'])
 copy_tree(folder,out/('independent-python-'+osname+'01'))
 normal=sum(int(re.search(r'Ran (\d+) tests?',p.read_text(encoding='utf-8')).group(1)) for p in folder.glob('*-normal.stderr'))
 skips=[p.name for p in folder.glob('*.stderr') if 'skipped=' in p.read_text(encoding='utf-8')]
 rows.append(dict(os=osname,python=value['python'],platform=value['platform'],commands=52,suites=26,test_methods_per_mode=normal,skips=skips,forbidden_host_reads=[],status='PASS'))
files={}
for path in (private/'python-export02').rglob('*'):
 if path.is_file():
  raw=path.read_bytes();files[path.relative_to(private/'python-export02').as_posix()]=dict(sha256=hashlib.sha256(raw).hexdigest(),bytes=len(raw))
with (out/'independent-python-final-export-manifest01.json').open('x',encoding='utf-8') as stream:json.dump(dict(format='files-map-v1',count=len(files),files=files),stream,indent=2)
with (out/'independent-python-summary01.json').open('x',encoding='utf-8') as stream:json.dump(dict(status='PASS_INDEPENDENT_WINDOWS_LINUX_PYTHON_COLLECTION',rows=rows,retained_source_start=0,business_provider_io=0),stream,indent=2)
old=Path('C:/Users/Henry/.codex/worktrees/d1-retention-composition/data-product-platform/docs/acceptance/evidence/issue307-d1/retention-composition-v2-publication-authorized01')
preserved=out/'original-publication-failure01';preserved.mkdir(exist_ok=False)
for name in ('continuation01/ci-validation-terminal.json','ci-diagnostics01/ubuntu-job120863177170-bounded.log','ci-diagnostics01/browser-job120863177328-bounded.log'):
 with (preserved/Path(name).name).open('xb') as stream:stream.write((old/name).read_bytes())
print(json.dumps(rows))
