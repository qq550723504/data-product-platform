"""Bounded zero-output retry of the unchanged freezer in a longpaths process.

Preserves the empty partial packet and original stderr. Refuses any existing
candidate/archive/key/operator output. Never repeats native work or changes Git
configuration, initial binaries, budgets, permissions or authorization.
"""
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys

root=Path(__file__).resolve().parents[5]
planning=Path(__file__).parent
packet=root/'docs/acceptance/evidence/issue307-d1/remaining-candidate17'
private=Path('C:/Users/Henry/.codex/fixture-private/remaining-candidate17')
sha=lambda path:hashlib.sha256(path.read_bytes()).hexdigest()
before=json.loads((planning/'freeze-failure01/zero-output-readback.json').read_bytes())
source='8cca345525af828a8c9461e75f2dc5bf5f1fb428'
if subprocess.check_output(['git','rev-parse','HEAD'],cwd=root).decode().strip()!=source:raise ValueError('Source HEAD drift')
if before['source']!=source or list(packet.iterdir()):raise ValueError('Expected empty zero-output partial packet')
for name in ('approved-runtime-source.zip','topology-key.private'):
    if (private/name).exists():raise ValueError('Archive/key already exists')
for name,digest in before['initial_build_files'].items():
    if sha(private/name)!=digest:raise ValueError('Initial build changed')
for name,digest in before['permission_sha256'].items():
    if sha(private/name/'permission.json')!=digest:raise ValueError('Permission changed')
for name,digest in before['case_sha256'].items():
    group,stage=name.split('/')
    if sha(private/group/('case-'+stage)/'remaining.json')!=digest:raise ValueError('Case changed')
for name,digest in before['backup_sha256'].items():
    group,filename=name.split('/')
    if sha(private/group/'checkpoint'/filename)!=digest:raise ValueError('Native backup changed')
failure=planning/'freeze-failure01'
target=failure/'partial-packet'
packet.resolve(strict=True).relative_to(root.resolve(strict=True))
target.resolve(strict=False).relative_to(root.resolve(strict=True))
if target.exists():raise ValueError('Preserved partial packet already exists')
out=planning/'phase-freeze02';out.mkdir(exist_ok=False)
intent={'source':source,'at':dt.datetime.now(dt.timezone.utc).isoformat(),'failure_stderr_sha256':before['failure_stderr_sha256'],
        'checked_empty_packet':str(packet),'preserved_partial_packet':str(target),'git_environment_override':{'key':'core.longpaths','value':'true','scope':'child_process_only'},
        'source_binary_gate_or_budget_changes':False,'native_work_replayed':False,'delete_authority':False}
with (out/'intent.json').open('x',encoding='utf-8') as stream:json.dump(intent,stream,indent=2)
packet.rename(target)
env=os.environ.copy();number=int(env.get('GIT_CONFIG_COUNT','0'))
env['GIT_CONFIG_COUNT']=str(number+1);env['GIT_CONFIG_KEY_'+str(number)]='core.longpaths';env['GIT_CONFIG_VALUE_'+str(number)]='true'
result=subprocess.run(['python','-B','examples/lifecycle-d1/freeze-six-candidate.py'],cwd=root,env=env,capture_output=True)
for channel in ('stdout','stderr'):
    with (out/(channel+'.txt')).open('xb') as stream:stream.write(getattr(result,channel))
receipt={'source':source,'exit':result.returncode,'at':dt.datetime.now(dt.timezone.utc).isoformat(),'stdout_bytes':len(result.stdout),'stderr_bytes':len(result.stderr),
         'original_failure_preserved':True,'preserved_partial_packet_empty':not any(target.iterdir()),'source_or_binary_change':False,'native_work_replayed':False,'delete_authority':False}
with (out/'result.json').open('x',encoding='utf-8') as stream:json.dump(receipt,stream,indent=2)
print(json.dumps(receipt))
if result.returncode:print(result.stderr.decode('utf-8','replace')[-3500:],file=sys.stderr)
raise SystemExit(result.returncode)
