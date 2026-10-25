"""Freeze compact review inputs and private originals without duplicating logs."""
import hashlib,json
from pathlib import Path
root=Path(__file__).resolve().parents[4];out=Path(__file__).parent;private=Path('C:/Users/Henry/.codex/fixture-private/merge-closeout01')
def ref(path):
    raw=path.read_bytes();return dict(sha256=hashlib.sha256(raw).hexdigest(),bytes=len(raw))
def save(path,value):
    with path.open('x',encoding='utf-8',newline='\n') as stream:json.dump(value,stream,indent=2)
assert json.loads((out/'actual-readonly01/result.json').read_bytes())['result']['status']=='PASS_SUCCESSOR_HISTORY_AND_COLD_DEPENDENCIES'
private_files={p.relative_to(private).as_posix():ref(p) for p in private.rglob('*') if p.is_file()}
save(private/'raw-evidence-index01.json',dict(format='files-map-v1',count=len(private_files),files=private_files,self_reference=False))
for name,expected in private_files.items():assert ref(private/name)==expected,name
save(out/'raw-evidence-binding01.json',dict(status='PASS_PRIVATE_RAW_EVIDENCE_BYTES',root=str(private),members=len(private_files),index_sha256=ref(private/'raw-evidence-index01.json')['sha256'],
    source_start=0,business_provider_calls=0,protection_write=0,business_delete=0))
files={p.relative_to(root).as_posix():ref(p) for p in out.rglob('*') if p.is_file()}
names=['.gitattributes','examples/lifecycle-d1/successor_dependencies.py','examples/lifecycle-d1/test_successor_dependencies.py',
       'examples/lifecycle-d1/retired_resources.py','examples/lifecycle-d1/retirement_catalog.py','examples/lifecycle-d1/fixture-runtime-artifact.json',
       'docs/acceptance/issue332-finite-merge-contract.md','docs/acceptance/issue332-closeout-acceptance01.md']
for name in names:files[name]=ref(root/name)
save(out/'review-inputs-index01.json',dict(format='files-map-v1',count=len(files),files=dict(sorted(files.items())),self_reference=False))
print(json.dumps(dict(public_members=len(files),private_original_members=len(private_files),review_index_sha256=ref(out/'review-inputs-index01.json')['sha256'])))
