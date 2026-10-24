"""Capture the fixed complete entry with exact frozen arguments, never send."""
import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys

ROOT=Path(__file__).resolve().parents[5]
BASE=ROOT/'docs/acceptance/evidence/issue307-d1'
HERE=Path(__file__).parent
PACKET=BASE/'remaining-candidate22'
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private/remaining-candidate22')
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
load=lambda p:json.loads(p.read_bytes())
parser=argparse.ArgumentParser();parser.add_argument('mode',choices=['native','audit'])
args=parser.parse_args()
status=load(PACKET/'status.json')
plan=(PACKET/'preparation-evidence-audit-plan.json' if args.mode=='native' else HERE/'final-static-evidence-audit-plan01.json')
output=(BASE/'remaining-candidate22-readonly01' if args.mode=='native' else HERE/'final-static-audit01')
receipt=HERE/('phase-complete-native01' if args.mode=='native' else 'phase-final-static-audit01')
receipt.mkdir(exist_ok=False)
if output.exists():raise ValueError('O_EXCL output required')
command=['python','-X','utf8','-B','examples/lifecycle-d1/verify-six-native.py',
 '--candidate-sha256',status['candidate_sha256'],'--bindings-sha256',status['bindings_sha256'],
 '--checker-sha256',sha(PRIVATE/'readonly-check.exe'),'--evidence-audit-plan',plan.relative_to(ROOT).as_posix(),
 '--evidence-audit-plan-sha256',sha(plan),'--output',str(output)]
if args.mode=='audit':command.append('--audit-only')
with (receipt/'intent.json').open('x',encoding='utf-8') as f:
 json.dump({'argv':command,'at':dt.datetime.now(dt.timezone.utc).isoformat(),'delete_authority':False},f,indent=2)
env=dict(os.environ,PYTHONUTF8='1',GIT_CONFIG_COUNT='1',GIT_CONFIG_KEY_0='core.longpaths',GIT_CONFIG_VALUE_0='true')
result=subprocess.run(command,cwd=ROOT,capture_output=True,env=env)
for name in ('stdout','stderr'):
 with (receipt/(name+'.txt')).open('xb') as f:f.write(getattr(result,name))
value={'exit':result.returncode,'at':dt.datetime.now(dt.timezone.utc).isoformat(),'mode':args.mode,
 'stdout_bytes':len(result.stdout),'stderr_bytes':len(result.stderr),'native_delete':0,'delete_authority':False}
with (receipt/'result.json').open('x',encoding='utf-8') as f:json.dump(value,f,indent=2)
print(json.dumps(value))
if result.returncode:print(result.stderr.decode('utf-8','replace')[-4500:],file=sys.stderr)
raise SystemExit(result.returncode)
