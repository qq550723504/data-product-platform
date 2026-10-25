"""Sequential fixed non-delete entry points. Any failed phase stops this run."""
import datetime as dt
import json
from pathlib import Path
import subprocess
import sys

ROOT=Path(__file__).resolve().parents[5]
HERE=Path(__file__).parent
OUT=HERE/'bounded-preparation-run02';OUT.mkdir(exist_ok=False)
commands=[['python','-X','utf8','-B',str(HERE/'run-preparation-phase02.py'),phase]
          for phase in ('allocate','materialize','initialize-budgets','run-non-delete','freeze')]
commands.append(['python','-X','utf8','-B',str(HERE/'run-complete-entry01.py'),'native'])
results=[]
for index,command in enumerate(commands,1):
    with (OUT/f'{index:02d}-intent.json').open('x',encoding='utf-8') as stream:
        json.dump({'argv':command,'at':dt.datetime.now(dt.timezone.utc).isoformat(),
                   'native_delete_authority':False},stream,indent=2)
    print(json.dumps({'phase':command[-1],'status':'STARTING_FIXED_NONDELETE_ENTRY'}),flush=True)
    result=subprocess.run(command,cwd=ROOT,capture_output=True)
    for channel in ('stdout','stderr'):
        with (OUT/f'{index:02d}-{channel}.txt').open('xb') as stream:stream.write(getattr(result,channel))
    receipt={'phase':command[-1],'exit':result.returncode,'at':dt.datetime.now(dt.timezone.utc).isoformat()}
    results.append(receipt)
    with (OUT/f'{index:02d}-result.json').open('x',encoding='utf-8') as stream:json.dump(receipt,stream,indent=2)
    print(json.dumps(receipt),flush=True)
    if result.returncode:
        print(result.stderr.decode('utf-8','replace')[-4000:],file=sys.stderr,flush=True)
        break
status='PASS_FIXED_C22_NONDELETE_PREPARATION_AND_COMPLETE_NATIVE' if len(results)==6 and all(x['exit']==0 for x in results) else 'STOPPED_FAILED_NONDELETE_ENTRY_RETAIN_ORIGINALS'
with (OUT/'result.json').open('x',encoding='utf-8') as stream:
    json.dump({'status':status,'results':results,'delete_authority':False,
               'full_D1':'NOT_ACCEPTED','PR332':'DRAFT_UNMERGED','Issue307':'OPEN'},stream,indent=2)
print(json.dumps({'status':status,'delete_authority':False}),flush=True)
raise SystemExit(any(x['exit'] for x in results))
