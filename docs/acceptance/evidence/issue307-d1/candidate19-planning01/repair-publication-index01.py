"""Preserve the failed publication and prove the sole raw-byte index correction."""
import datetime as dt
import hashlib
import json
from pathlib import Path
import subprocess

ROOT=Path(__file__).resolve().parents[5]
HERE=Path(__file__).parent
OUT=HERE/'publication01'
sha=lambda raw:hashlib.sha256(raw).hexdigest()
def save(path,value):
 with path.open('xb') as f:f.write((json.dumps(value,indent=2)+'\n').encode('utf-8'))
def git(*args):return subprocess.check_output(['git','-c','core.longpaths=true',*args],cwd=ROOT)
raw=(HERE/'publish-preparation01.py').read_bytes()
with (OUT/'source-script.py').open('xb') as f:f.write(raw)
rel='docs/acceptance/evidence/issue307-d1/post-v28-restored-core01/address-pool-cause.json'
save(OUT/'failure.json',{'status':'FAILED_BEFORE_COMMIT_OR_PUSH_STAGED_RAW_AUDIT',
 'at':dt.datetime.now(dt.timezone.utc).isoformat(),'exit':1,'error_type':'ValueError',
 'error':'Indexed evidence bytes changed through Git filters','affected':rel,'source_script_sha256':sha(raw),
 'capture_source':'Tool output transcription; not a captured raw subprocess channel',
 'runtime_candidate_binary_config_changed':False,'commit_created':False,'push_performed':False,'native_delete':0})
content=(ROOT/rel).read_bytes()
expected=json.loads((ROOT/'docs/acceptance/evidence/issue307-d1/candidate19-review01/reviewed-artifacts01.json').read_bytes())['files'][rel]
if sha(content)!=expected['sha256']:raise ValueError('Independently reviewed original bytes changed')
previous=git('show','HEAD:'+rel)
git('add','--renormalize','--',rel)
staged=git('show',':'+rel)
if staged!=content:raise ValueError('Raw staged bytes still differ')
save(HERE/'publication-index-repair01.json',{'status':'PASS_ORIGINAL_REVIEWED_BYTES_PRESERVED_IN_GIT_INDEX',
 'path':rel,'reviewed_worktree_sha256':sha(content),'previous_commit_representation_sha256':sha(previous),
 'staged_blob_sha256':sha(staged),'raw_bytes_unchanged':True,'attributes':'-text already present; explicit reindex required',
 'runtime_candidate_binary_config_changed':False,'native_delete':0})
with (HERE/'publish-preparation02.py').open('x',encoding='utf-8',newline='\n') as f:
 f.write(raw.decode('utf-8').replace("OUT=HERE/'publication01'","OUT=HERE/'publication02'"))
print(json.dumps({'status':'PASS_INDEX_REPAIR_ONLY','original_bytes_unchanged':True,'native_delete':0}))
