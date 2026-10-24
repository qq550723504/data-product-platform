"""Record the actual independent conclusion and compact immutable byte index."""
import hashlib,json
from pathlib import Path
root=Path(__file__).resolve().parents[4];out=Path(__file__).parent;private=Path('C:/Users/Henry/.codex/fixture-private/merge-closeout01')
def ref(path):
    raw=path.read_bytes();return dict(sha256=hashlib.sha256(raw).hexdigest(),bytes=len(raw))
def save(path,value):
    with path.open('x',encoding='utf-8',newline='\n') as stream:json.dump(value,stream,indent=2)
inputs=out/'review-inputs-index01.json';raw_index=private/'raw-evidence-index01.json'
assert ref(inputs)['sha256']=='4db61fadf918667a895e8d45da7b5ac34166ef6cb09c1ce407f17fc6e76bc638'
assert ref(raw_index)['sha256']=='c07f0f5fe4f9ba24e95fd906c35228e65df9ee1c8ba723b2323c6e19fe809e5b'
for name,expected in json.loads(inputs.read_bytes())['files'].items():assert ref(root/name)==expected,name
for name,expected in json.loads(raw_index.read_bytes())['files'].items():assert ref(private/name)==expected,name
representations={}
for name in ('.gitattributes','examples/lifecycle-d1/successor_dependencies.py','examples/lifecycle-d1/test_successor_dependencies.py'):
    raw=(root/name).read_bytes();representations[name]=dict(raw_sha256=ref(root/name)['sha256'],lf_sha256=hashlib.sha256(raw.replace(b'\r\n',b'\n')).hexdigest())
save(out/'independent-final-review01.json',dict(status='PASS_BOUNDED_LOCAL_CODE_AND_NONDELETE_REVIEW',reviewer='/root/docker_linux_engine_repair',
    provenance='Actual read-only subagent conclusion recorded by parent, not a self-review or GitHub review submission.',review_actions='READ_ONLY',
    public_index_sha256=ref(inputs)['sha256'],public_members=18,private_index_sha256=ref(raw_index)['sha256'],private_members=530,
    source_sha256={name:ref(root/name)['sha256'] for name in ('examples/lifecycle-d1/successor_dependencies.py','examples/lifecycle-d1/test_successor_dependencies.py')},
    approved_git_representations=representations,remaining_code_blockers=[],
    remaining_delivery_blockers=['B1_UNPUBLISHED_FIX_AND_NEW_HEAD_PR_CI_NOT_RUN'],
    findings=['Both raw historical and empty-DNS canonical identities are required; only the one DNS field can vary.',
              'All other identity/config/privilege/netns/mount/closure/volume and budget checks stay fail closed.',
              'Both actual CPython3.13.16 platforms passed52commands and192methods per mode; successor23 no skip.',
              'Actual fixed read-only CLI verified184dependencies/134volumes/58physicalbudgets/54310originals.',
              'Historical failure bytes are unchanged; the one isolated test helper was archived and removed.',
              'Real two-version protection writes and six deletions remainIssue307 NOT_RUN; no candidate/window added.'],
    actual_D1_acceptance='NOT_RUN',commit_binding='PENDING_SEPARATE_GIT_BLOB_READBACK',new_head_PR_CI='NOT_RUN'))
files=dict(json.loads(inputs.read_bytes())['files'])
for path in out.rglob('*'):
    if path.is_file():files[path.relative_to(root).as_posix()]=ref(path)
save(out/'evidence-index01.json',dict(format='files-map-v1',count=len(files),files=dict(sorted(files.items())),self_reference=False))
for name,expected in files.items():assert ref(root/name)==expected,name
save(out/'evidence-index-audit01.json',dict(status='PASS_COMPACT_REVIEW_AND_ACCEPTANCE_BYTES',members=len(files),index_sha256=ref(out/'evidence-index01.json')['sha256'],differences=[]))
print(json.dumps(dict(code_blockers=0,delivery_blockers=['B1'],public_members=len(files),review_sha256=ref(out/'independent-final-review01.json')['sha256'])))
