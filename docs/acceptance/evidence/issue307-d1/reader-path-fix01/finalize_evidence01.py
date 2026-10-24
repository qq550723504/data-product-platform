"""Record actual read-only subagent conclusion and final original-byte index."""
import hashlib,json
from pathlib import Path
root=Path(__file__).resolve().parents[5];out=Path(__file__).parent
def ref(path):
    raw=path.read_bytes();return dict(sha256=hashlib.sha256(raw).hexdigest(),bytes=len(raw))
def save(path,value):
    with path.open('x',encoding='utf-8',newline='\n') as f:json.dump(value,f,ensure_ascii=False,indent=2)
index=out/'review-inputs-index01.json'
assert ref(index)['sha256']=='d8dc1dc7b0feb7e343331c87f129d2b007d96a68021c5c72da799e7567aac413'
inputs=json.loads(index.read_bytes());assert inputs['count']==407
for name,expected in inputs['files'].items():assert ref(root/name)==expected,name
representations={}
for name in ('.gitattributes','examples/lifecycle-d1/test_retention_composition.py'):
    raw=(root/name).read_bytes();lf=raw.replace(b'\r\n',b'\n')
    representations[name]=dict(raw_sha256=hashlib.sha256(raw).hexdigest(),lf_sha256=hashlib.sha256(lf).hexdigest(),
                               accepted_git_blob='exact raw OR CRLF-to-LF only; no other transformation')
save(out/'independent-final-review01.json',dict(status='PASS_BOUNDED_INDEPENDENT_READER_PATH_PY313_REVIEW',
    reviewer='/root/docker_linux_engine_repair',review_actions='READ_ONLY',
    provenance='Actual read-only subagent final conclusions recorded by parent; not a self-review or GitHub review submission.',
    review_inputs_index_sha256=ref(index)['sha256'],review_inputs_count=407,
    source_sha256={'examples/lifecycle-d1/test_retention_composition.py':'536667d5d50c439e97b60651c40fe2003c795d4b416816a443b0dfdda677dcc9'},
    approved_git_representations=representations,remaining_blockers=[],
    findings=['Fixture canonicalizes root before IO, paths and signatures using existing utility; production gate unchanged.',
              'Real Windows uppercase and 8.3 aliases exercise complete finite success flow; rehashed wrong paths/native still reject before operations.',
              'Both actual CPython3.13.16 platforms: 26suites/52commands, normal and -O each185methods. Windows only existing evidence_indexes skip per mode; composition12 no skip; Linux0skip.',
              '407 reviewed index members and202 independent export members all match original bytes.',
              'Original failure and unknown original runner alias preserved; actual business17 verification remains NOT_RUN.',
              'Locked Draft/Open boundary and failed old CI remain enforced; no runtime/provider/business writes.'],
    new_P_and_publication_binding='PENDING_SEPARATE_REVIEW',new_head_PR_CI='NOT_RUN',business_execution='NOT_AUTHORIZED'))
members=dict(inputs['files'])
for path in out.rglob('*'):
    if path.is_file():members[path.relative_to(root).as_posix()]=ref(path)
save(out/'evidence-index01.json',dict(format='files-map-v1',status='FINAL_ORIGINAL_BYTES_NOT_AUTHORITY',count=len(members),files=dict(sorted(members.items()))))
for name,expected in members.items():assert ref(root/name)==expected,name
save(out/'evidence-index-audit01.json',dict(status='PASS_EXACT_FILE_MAP_AUDIT',index_sha256=ref(out/'evidence-index01.json')['sha256'],
    members=len(members),differences=[],self_reference=False))
print(json.dumps(dict(status='PASS',members=len(members),review_sha256=ref(out/'independent-final-review01.json')['sha256'])))
