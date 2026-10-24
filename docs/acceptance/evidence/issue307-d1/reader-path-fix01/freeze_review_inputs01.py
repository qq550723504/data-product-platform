"""Freeze local review inputs with explicit file-map, not approval authority."""
import hashlib,json,subprocess
from pathlib import Path
root=Path(__file__).resolve().parents[5];out=Path(__file__).parent
def ref(path):
    raw=path.read_bytes();return dict(sha256=hashlib.sha256(raw).hexdigest(),bytes=len(raw))
def save(path,value):
    with path.open('x',encoding='utf-8',newline='\n') as f:json.dump(value,f,ensure_ascii=False,indent=2)
source=root/'examples/lifecycle-d1/test_retention_composition.py'
assert ref(source)['sha256']=='536667d5d50c439e97b60651c40fe2003c795d4b416816a443b0dfdda677dcc9'
assert subprocess.check_output(['git','-c','core.longpaths=true','diff','--name-only','HEAD','--','examples/lifecycle-d1/retention_composition.py','examples/lifecycle-d1/evidence_indexes.py','examples/lifecycle-d1/fixture-runtime-artifact.json'],cwd=root)==b''
export=json.loads((out/'independent-export-manifest01.json').read_bytes());snapshot=Path(export['snapshot'])
for name,expected in export['files'].items():assert ref(snapshot/name)==expected,name
tracked=subprocess.check_output(['git','-c','core.longpaths=true','diff','--name-only','HEAD','--'],cwd=root).decode().splitlines()
assert set(tracked)=={'.gitattributes','examples/lifecycle-d1/test_retention_composition.py'},tracked
save(out/'source-boundary01.json',dict(base='16ec447f2c60ddf08a514207963f478bc70c7e09',
    changed_tracked_files=tracked,only_executable_change='test fixture normalization and regression methods',
    production_gate_diff=[],runtime_manifest_diff=[],export_members_verified=len(export['files']),
    source_sha256={source.relative_to(root).as_posix():ref(source)['sha256']},
    root_source_start=0,business_provider_io=0,business_protection_write=0,delete=0,budget_reset=0,pin_release=0))
members={p.relative_to(root).as_posix():ref(p) for p in out.rglob('*') if p.is_file()}
for name in ('.gitattributes','examples/lifecycle-d1/test_retention_composition.py','examples/lifecycle-d1/retention_composition.py',
             'examples/lifecycle-d1/evidence_indexes.py','examples/lifecycle-d1/fixture-runtime-artifact.json',
             'docs/acceptance/issue307-d1-reader-path-matrix01.md'):
    members[name]=ref(root/name)
save(out/'review-inputs-index01.json',dict(format='files-map-v1',status='FROZEN_LOCAL_REVIEW_INPUTS_NOT_AUTHORITY',count=len(members),files=dict(sorted(members.items()))))
print(json.dumps(dict(members=len(members),source=ref(source))))
