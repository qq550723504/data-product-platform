"""Append final encoding guard review inputs; preserve the prior byte view."""
import hashlib,json
from pathlib import Path
root=Path(__file__).resolve().parents[4];out=Path(__file__).parent
def ref(path):
    raw=path.read_bytes();return dict(sha256=hashlib.sha256(raw).hexdigest(),bytes=len(raw))
def save(path,value):
    with path.open('x',encoding='utf-8',newline='\n') as stream:json.dump(value,stream,indent=2)
previous=json.loads((out/'review-inputs-index01.json').read_bytes())
for name,expected in previous['files'].items():
    path=out/'reviewed-attributes01.txt' if name=='.gitattributes' else root/name
    assert ref(path)==expected,name
representations={}
for name in ('.gitattributes','examples/lifecycle-d1/successor_dependencies.py','examples/lifecycle-d1/test_successor_dependencies.py'):
    raw=(root/name).read_bytes();representations[name]=dict(raw_sha256=ref(root/name)['sha256'],lf_sha256=hashlib.sha256(raw.replace(b'\r\n',b'\n')).hexdigest())
save(out/'encoding-guard-addendum01.json',dict(status='PRECOMMIT_EXACT_BYTE_POLICY_CORRECTION_NO_RUNTIME_CHANGE',
    reason='Git add warned new issue332 evidence would use checkout CRLF; issue307-only old rule did not cover this new folder.',
    old_index_is_historical_pre_encoding_guard=True,old_attributes_snapshot='reviewed-attributes01.txt',
    correction='One -text rule for this exact new evidence folder; existing files/rules, source behavior and original evidence unchanged.',
    original_public_members_replayed=18,source_regressions_unchanged=True,approved_git_representations=representations))
files={p.relative_to(root).as_posix():ref(p) for p in out.rglob('*') if p.is_file()}
for name in previous['files']:
    if not name.startswith('docs/acceptance/evidence/issue332-closeout01/'):files[name]=ref(root/name)
save(out/'review-inputs-index02.json',dict(format='files-map-v1',count=len(files),files=dict(sorted(files.items())),self_reference=False))
for name,expected in files.items():assert ref(root/name)==expected,name
print(json.dumps(dict(members=len(files),index_sha256=ref(out/'review-inputs-index02.json')['sha256'])))
