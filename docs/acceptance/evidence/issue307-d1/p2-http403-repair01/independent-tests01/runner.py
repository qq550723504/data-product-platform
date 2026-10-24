"""Independent pure Go unit/httptest checks, with native/DB settings removed."""
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys

OUT=Path(__file__).resolve().parent
ROOT=OUT.parents[5]
SOURCES=[
 'apps/platform/cmd/lifecycle-fixture-remaining/send_return.go',
 'apps/platform/cmd/lifecycle-fixture-remaining/send_return_test.go',
 'apps/platform/cmd/lifecycle-fixture-remaining/wire.go',
 'apps/platform/cmd/lifecycle-fixture-remaining/wire_test.go',
 'apps/platform/cmd/lifecycle-fixture-remaining/wire_native_test.go',
 'apps/platform/internal/lifecycle/pilot/remaining_policies.go',
 'apps/platform/internal/lifecycle/pilot/remaining_delete_policy_test.go',
 'apps/platform/internal/platform/storage/lifecycle_delete.go',
 'apps/platform/internal/platform/storage/lifecycle_delete_response.go',
 'apps/platform/internal/platform/storage/lifecycle_delete_response_test.go',
 'examples/lifecycle-d1/check-p2-permission-native.py',
]
def meta(path):
 raw=path.read_bytes()
 return {'sha256':hashlib.sha256(raw).hexdigest(),'bytes':len(raw)}
source={name:meta(ROOT/name) for name in SOURCES}
env=dict(os.environ,GOPROXY='off',GOTOOLCHAIN='local')
for key in list(env):
 if key.startswith('TEST_') or key in ('DATABASE_URL','POSTGRES_DSN','PG_DSN'):del env[key]
commands=[
 ['go','test','-json','-count=1','./internal/platform/storage','-run','^(TestExactDeleter|TestExactDelete)'],
 ['go','test','-json','-count=1','./internal/lifecycle/pilot','-run','^TestRemainingDeletePolicyCoversNativeEntryWithoutExpandingExactScope$'],
 ['go','test','-json','-count=1','./cmd/lifecycle-fixture-remaining','-run','^(TestWire|TestDeniedSendReturnKeepsSafeNativeReasonAndNoResend|TestClaimRejectionKeepsOriginalErrorWithoutProviderObservationOrCost|TestAcceptedProviderThenLostObservationKeepsTransportAndBothErrors|TestCompiledRemainingTargetsExcludeSurvivorsAndOldProfiles)'],
]
rows=[]
for i,command in enumerate(commands,1):
 start=dt.datetime.now(dt.timezone.utc).isoformat()
 run=subprocess.run(command,cwd=ROOT/'apps/platform',env=env,capture_output=True,timeout=180)
 streams={}
 for name in ('stdout','stderr'):
  path=OUT/(str(i).zfill(2)+'.'+name+'.txt')
  with path.open('xb') as target:target.write(getattr(run,name))
  streams[name]=meta(path)
 events=[json.loads(x) for x in run.stdout.splitlines() if x.startswith(b'{')]
 passed=[x['Test'] for x in events if x.get('Action')=='pass' and x.get('Test')]
 rows.append({'command':command,'started_at':start,'exit_code':run.returncode,'passed_tests_and_subtests':passed,**streams})
 print(json.dumps({'package':command[4],'exit_code':run.returncode,'test_and_subtest_passes':len(passed)}),flush=True)
stable=source=={name:meta(ROOT/name) for name in SOURCES}
result={'status':'PASS_INDEPENDENT_P2_REPAIR_UNIT' if stable and all(x['exit_code']==0 for x in rows) else 'FAIL_OR_SOURCE_CHANGED',
        'at':dt.datetime.now(dt.timezone.utc).isoformat(),'source_files':source,'source_stable':stable,'results':rows,
        'native_environment_removed':True,'Docker_calls':0,'database_calls':0,'provider_calls':0,'actual_object_deletions':0}
with (OUT/'result.json').open('x',encoding='utf-8',newline='\n') as target:json.dump(result,target,indent=2);target.write('\n')
print(json.dumps({k:v for k,v in result.items() if k not in ('source_files','results')}),flush=True)
sys.exit(0 if result['status'].startswith('PASS_') else 1)
