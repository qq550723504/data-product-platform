"""Finish a byte-preserving Git index repair, without changing the old file."""
import hashlib
import json
from pathlib import Path
import subprocess

ROOT=Path(__file__).resolve().parents[5]
HERE=Path(__file__).parent
sha=lambda raw:hashlib.sha256(raw).hexdigest()
def git(*args):return subprocess.check_output(['git','-c','core.longpaths=true',*args],cwd=ROOT)
def save(name,value):
 with (HERE/name).open('xb') as f:f.write((json.dumps(value,indent=2)+'\n').encode('utf-8'))
rel='docs/acceptance/evidence/issue307-d1/post-v28-restored-core01/address-pool-cause.json'
original=(ROOT/rel).read_bytes();previous=git('show','HEAD:'+rel);staged=git('show',':'+rel)
if previous.replace(b'\r\n',b'\n')!=original.replace(b'\r\n',b'\n') or staged!=original:
 raise ValueError('Original content or staged exact bytes differ')
save('publication-repair-wrapper-failure01.json',{'error':'KeyError: old diagnostic file is not a member of reviewed-artifacts01.json',
 'capture_source':'Tool output transcription, not raw subprocess output','failed_script':'repair-publication-index01.py',
 'original_file_changed':False,'commit_or_push':False,'native_delete':0,
 'correction':'Compare original content to its retained HEAD blob and staged raw bytes. Do not claim this diagnostic was an indexed independent input.'})
save('publication-index-repair02.json',{'status':'PASS_ORIGINAL_CONTENT_AND_RAW_GIT_INDEX',
 'path':rel,'worktree_raw_sha256':sha(original),'previous_commit_representation_sha256':sha(previous),
 'staged_blob_sha256':sha(staged),'canonical_content_same_as_previous_commit':True,'staged_equals_original_raw':True,
 'attributes':'-text was already present; explicit reindex required','runtime_candidate_binary_config_changed':False,'native_delete':0})
source=(HERE/'publication01/source-script.py').read_bytes()
if sha(source)!=json.loads((HERE/'publication01/failure.json').read_bytes())['source_script_sha256']:
 raise ValueError('Original publication source snapshot differs')
with (HERE/'publish-preparation02.py').open('x',encoding='utf-8',newline='\n') as f:
 f.write(source.decode('utf-8').replace("OUT=HERE/'publication01'","OUT=HERE/'publication02'"))
print(json.dumps({'status':'PASS_BYTE_PRESERVING_INDEX_REPAIR','native_delete':0}))
