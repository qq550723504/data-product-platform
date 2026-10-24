"""Capture one approved non-delete preparation CLI phase, never resume a phase."""
import argparse
import datetime as dt
import json
import os
from pathlib import Path
import subprocess
import sys

root=Path(__file__).resolve().parents[5]
parser=argparse.ArgumentParser()
parser.add_argument('phase',choices=['allocate','materialize','initialize-budgets','run-non-delete','freeze'])
args=parser.parse_args()
source='c79ad0c15062304e959d6dd1fc8baa6362abcf67'
head=subprocess.check_output(['git','-c','core.longpaths=true','rev-parse','HEAD'],cwd=root).decode().strip()
if head!=source:raise ValueError('Exact independently reviewed source HEAD required before allocation/build/freeze')
review=root/'docs/acceptance/evidence/issue307-d1/candidate20-artifacts-review01/independent-final-review01.json'
report=json.loads(review.read_bytes())
if report['status']!='PASS_BOUNDED_C20_DUAL_ARTIFACT_PREPARATION_REVIEW' or report['source_head']!=source or report['remaining_blockers']!=[]:raise ValueError('Source independent review gate denied')
if report['runtime_source']!='72ec972570a6f10a916ace39d90ab0c5593e8dd4':raise ValueError('Reviewed R artifact differs')
env=os.environ.copy();env.update(GIT_CONFIG_COUNT='1',GIT_CONFIG_KEY_0='core.longpaths',GIT_CONFIG_VALUE_0='true')
if args.phase=='allocate':command=['python','-X','utf8','-B','examples/lifecycle-d1/prepare-six-candidate.py','allocate']
elif args.phase=='freeze':command=['python','-X','utf8','-B','examples/lifecycle-d1/freeze-six-candidate.py']
else:command=['python','-X','utf8','-B','examples/lifecycle-d1/six_candidate_runtime.py',args.phase]
out=Path(__file__).with_name('phase-'+args.phase+'01');out.mkdir(exist_ok=False)
with (out/'intent.json').open('x',encoding='utf-8') as stream:json.dump({'source':source,'phase':args.phase,'argv':command,'at':dt.datetime.now(dt.timezone.utc).isoformat(),'delete_authority':False},stream,indent=2)
result=subprocess.run(command,cwd=root,capture_output=True,env=env)
for channel in ('stdout','stderr'):
    with (out/(channel+'.txt')).open('xb') as stream:stream.write(getattr(result,channel))
receipt={'phase':args.phase,'exit':result.returncode,'stdout_bytes':len(result.stdout),'stderr_bytes':len(result.stderr),'source':source,'at':dt.datetime.now(dt.timezone.utc).isoformat(),'delete_authority':False}
with (out/'result.json').open('x',encoding='utf-8') as stream:json.dump(receipt,stream,indent=2)
print(json.dumps(receipt))
if result.returncode:print(result.stderr.decode('utf-8','replace')[-3500:],file=sys.stderr)
raise SystemExit(result.returncode)
