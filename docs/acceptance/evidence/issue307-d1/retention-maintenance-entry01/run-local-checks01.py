"""Local-only bounded regression evidence. No Docker/native/provider source I/O."""
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys

ROOT=Path(__file__).resolve().parents[5]
DEST=Path(__file__).with_name(sys.argv[1] if len(sys.argv)>1 else 'checks02')
DEST.mkdir()
env=os.environ.copy()
for name in ('POSTGRES_DSN','TEST_POSTGRES_DSN','REDIS_ADDR','D1_SCHEMA_TEST_DSN','D1_SCHEMA_EXPECT'):
    env.pop(name,None)
env.update(GOWORK='off',CGO_ENABLED='0',GOFLAGS='')
results=[]

def check(name,args,cwd=ROOT,overrides=None):
    started=dt.datetime.now(dt.timezone.utc).isoformat()
    result=subprocess.run(args,cwd=cwd,env={**env,**(overrides or {})},capture_output=True,timeout=300)
    for channel in ('stdout','stderr'):
        with (DEST/(name+'.'+channel)).open('xb') as stream:stream.write(getattr(result,channel))
    row={'name':name,'command':args,'cwd':str(cwd),'started_at':started,'finished_at':dt.datetime.now(dt.timezone.utc).isoformat(),'exit_code':result.returncode}
    results.append(row)
    print(name+': '+str(result.returncode),flush=True)

suites=['test_native_protection','test_retention_renewal','test_six_native_entry','test_six_candidate_runtime','test_fixture_artifacts','test_receipt_bytes','test_execution_freeze','test_freeze_six_candidate','test_retention_maintenance']
if '--affected' in sys.argv[2:]:suites=[]
for optimize in (False,True):
    for suite in suites:
        check(suite+('-optimized' if optimize else '-normal'),[sys.executable,*(['-O'] if optimize else []),'-B','examples/lifecycle-d1/'+suite+'.py'])
check('go-host',['go','test','./cmd/lifecycle-fixture-retention','./cmd/lifecycle-fixture-reader','./internal/platform/storage','./internal/lifecycle/pilot','-count=1'],ROOT/'apps/platform')
check('actual-R-archive-CI-entry',[sys.executable,'-B','examples/lifecycle-d1/ci-test-fixture-runtime.py'])
check('linux-disabled-default-build',['go','build','-trimpath','-buildvcs=false','-o',str(DEST/'retention-disabled-linux'), './cmd/lifecycle-fixture-retention'],ROOT/'apps/platform',{'GOOS':'linux','GOARCH':'amd64'})
summary={'status':'PASS_LOCAL_NONDELETE_SOURCE_CHECKS' if all(r['exit_code']==0 for r in results) else 'FAILED_LOCAL_NONDELETE_SOURCE_CHECKS','checks':results,
         'boundary':'Official SDK localhost doubles, file/Git archive and recorded Docker compositions. Not actual native/provider/PG acceptance or remote PR CI.',
         'actual_source_start':0,'actual_provider_calls':0,'retention_write':0,'delete':0,'new_execution_or_budget':0}
with (DEST/'summary.json').open('x',encoding='utf-8',newline='\n') as stream:json.dump(summary,stream,indent=2);stream.write('\n')
print(summary['status'],flush=True)
sys.exit(0 if all(r['exit_code']==0 for r in results) else 1)
