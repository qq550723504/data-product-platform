import hashlib,json,os,subprocess,sys
from pathlib import Path
ROOT=Path(__file__).resolve().parents[5];OUT=Path(__file__).resolve().parent
env=os.environ.copy()
for name in ('TEST_POSTGRES_DSN','DPP307_SIX_NATIVE_CONTRACT'):env.pop(name,None)
args=['go','test','-count=1','-json','./internal/lifecycle/pilot','./cmd/lifecycle-fixture-reader','./cmd/lifecycle-fixture-readonly-check','./cmd/lifecycle-fixture-remaining','./cmd/lifecycle-fixture-remaining-iam']
result=subprocess.run(args,cwd=ROOT/'apps/platform',env=env,capture_output=True)
for name,raw in [('go-tests01.stdout.jsonl',result.stdout),('go-tests01.stderr.txt',result.stderr)]:
    with (OUT/name).open('xb') as f:f.write(raw)
events=[json.loads(x) for x in result.stdout.splitlines() if x.strip()]
counts={action:len([e for e in events if e.get('Action')==action and e.get('Test')]) for action in ('pass','fail','skip')}
record={'command':args,'exit_code':result.returncode,'tests':counts,'packages':[{k:e[k] for k in ('Package','Action') if k in e} for e in events if e.get('Action') in ('pass','fail') and not e.get('Test')],'skips':[{k:e[k] for k in ('Package','Test') if k in e} for e in events if e.get('Action')=='skip'],'real_Postgres_and_native_optin_environment_explicitly_unset':['TEST_POSTGRES_DSN','DPP307_SIX_NATIVE_CONTRACT'],'stdout_sha256':hashlib.sha256(result.stdout).hexdigest(),'stderr_sha256':hashlib.sha256(result.stderr).hexdigest(),'source_Docker_native_provider_DELETE':0,'scope':'Go unit/mock/httptest only; no real PG or native fixture acceptance'}
raw=(json.dumps(record,indent=2)+'\n').encode()
with (OUT/'go-tests01.json').open('xb') as f:f.write(raw)
print(json.dumps(record,indent=2))
sys.exit(result.returncode)
