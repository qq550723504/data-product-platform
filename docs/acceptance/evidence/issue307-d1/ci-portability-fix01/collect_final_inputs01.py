"""Freeze explicit review inputs after final uninstrumented regressions."""
import hashlib,json,subprocess
from pathlib import Path
root=Path(__file__).resolve().parents[5];out=Path(__file__).parent;private=Path.home()/'.codex/fixture-private/ci-portability-fix01'
def attrs(raw):return dict(sha256=hashlib.sha256(raw).hexdigest(),bytes=len(raw))
def save(path,value):
 with path.open('x',encoding='utf-8',newline='\n') as stream:json.dump(value,stream,ensure_ascii=False,indent=2)
summary=[]
for osname,node,source in [('windows','24.18.1',out/'windows-browser-full04'),('linux','22.23.4',out/'linux-browser-full04')]:
 result=json.loads((source/'result.json').read_bytes());assert result['returncode']==0 and '105 passed' in (source/'stdout').read_text(encoding='utf-8')
 summary.append(dict(os=osname,node=node,playwright='1.63.0',tests=105,retries=0,status='PASS',command_receipt=str((source/'result.json').relative_to(root))))
save(out/'final-browser-summary01.json',dict(status='PASS_WINDOWS_LINUX_UNINSTRUMENTED_COMPLETE_BROWSER_COLLECTION',rows=summary,application='Real Next production build against loopback mocks; not live Core/native acceptance',prior_failures_retained=True))
failures=out/'preserved-browser-failures01';failures.mkdir(exist_ok=False);private_index={}
for label,folder in [('linux-first',private/'linux-browser-full01'),('linux-second',private/'linux-browser-full02'),('windows-third',private/'windows-browser-final03'),('old-query-counterexample',private/'query-context-counterexample01')]:
 for trace in folder.rglob('trace.zip'):
  raw=trace.read_bytes();name=label+'-'+trace.parent.name+'.zip'
  with (failures/name).open('xb') as stream:stream.write(raw)
for name in ('original-browser-artifact.zip','generated-browser-package-lock01.json','retained-sources-inspect01.private.json'):
 private_index[name]=attrs((private/name).read_bytes())
save(out/'private-artifact-bindings01.json',dict(format='files-map-v1',count=len(private_index),files=private_index,scope='Private originals retained, never authority or a CI fixture path'))
files={}
paths=subprocess.check_output(['git','-c','core.longpaths=true','ls-files','--','apps/platform','apps/web','examples/lifecycle-d1','migrations','.github/workflows','tests/browser'],cwd=root).decode().splitlines()
for name in paths:
 raw=(root/name).read_bytes();files[name]=dict(raw_sha256=attrs(raw)['sha256'],lf_sha256=attrs(raw.replace(b'\r\n',b'\n'))['sha256'],bytes=len(raw))
save(out/'reviewed-source-bytes01.json',dict(base_p='1d3e417bd059aaaee099161cf6e12b4c43fed81c',pinned_ci_base='f7a90022aad5da4d67ea594ccb86f8b9925a3840',original_ci_tree='74163482794851f535dc15f649df89552d4d4af2',count=len(files),files=files,operational_runtime='0446fde5985b51de7da5863b45df3f1b47b6167c',source_root=str(root)))
index={}
for path in sorted(out.rglob('*')):
 if path.is_file():index[path.relative_to(root).as_posix()]=attrs(path.read_bytes())
for name in ('docs/acceptance/issue307-d1-ci-portability-matrix01.md','docs/acceptance/.gitattributes'):
 index[name]=attrs((root/name).read_bytes())
for name in ('apps/web/app/attention/page.tsx','tests/browser/schema-requests.spec.mjs','tests/browser/schema-attempts.spec.mjs','examples/lifecycle-d1/test_retention_composition.py','examples/lifecycle-d1/test_six_native_entry.py','examples/lifecycle-d1/test_six_candidate_runtime.py','examples/lifecycle-d1/test_verify_six_native.py'):
 index[name]=attrs((root/name).read_bytes())
save(out/'review-inputs-index01.json',dict(format='files-map-v1',status='FROZEN_REVIEW_INPUTS',count=len(index),files=index))
print(json.dumps(dict(review_inputs=len(index),sources=len(files),browser=summary)))
