"""Read the exact published GitHub checkpoint and frozen authorization Host.check."""
from pathlib import Path
import concurrent.futures
import datetime as dt
import hashlib
import json
import os
import subprocess
import sys

ROOT=Path.cwd(); OUT=Path(__file__).resolve().parent
HEAD='caab46da8761e71e1d80d0c0931707a410c13a4d'
def now():return dt.datetime.now(dt.timezone.utc).isoformat()
def need(ok,message):
    if not ok:raise ValueError(message)
def write(path,raw):
    with path.open('xb') as f:f.write(raw)
def save(path,value):write(path,(json.dumps(value,ensure_ascii=False,indent=2)+'\n').encode())

queries={
 'pr':['api','repos/qq550723504/data-product-platform/pulls/332'],
 'issue':['api','repos/qq550723504/data-product-platform/issues/307'],
 'ci-run':['api','repos/qq550723504/data-product-platform/actions/runs/39710200629'],
 'ci-jobs':['api','repos/qq550723504/data-product-platform/actions/runs/39710200629/jobs?per_page=100'],
 'check-runs':['api','repos/qq550723504/data-product-platform/commits/'+HEAD+'/check-runs?per_page=100'],
 'threads':['api','graphql','-f','query={repository(owner:"qq550723504",name:"data-product-platform"){pullRequest(number:332){reviewThreads(first:100){nodes{id isResolved} pageInfo{hasNextPage endCursor}}}}}']}
def fetch(pair):
    name,args=pair;start=now();r=subprocess.run(['gh',*args],capture_output=True,cwd=ROOT)
    write(OUT/(name+'-readback01.stdout.json'),r.stdout);write(OUT/(name+'-readback01.stderr.txt'),r.stderr)
    need(r.returncode==0,'GitHub read failed: '+name)
    return name,{'started_at':start,'finished_at':now(),'value':json.loads(r.stdout)}

save(OUT/'authorized-prewindow-intent01.json',{'started_at':now(),'execution':'a5d1e569-f80e-4b38-8f32-d8139a30a077',
     'scope':'Frozen Host.check plus current exact HEAD/CI/threads read only; no Source starts or native preflight',
     'permission_enable':0,'source_start':0,'provider_io':0,'native_delete':0})
with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:remote=dict(pool.map(fetch,queries.items()))
pr=remote['pr']['value'];issue=remote['issue']['value'];run=remote['ci-run']['value'];jobs=remote['ci-jobs']['value'];checks=remote['check-runs']['value'];threads=remote['threads']['value']['data']['repository']['pullRequest']['reviewThreads']
need(pr['head']['sha']==HEAD and pr['state']=='open' and pr['draft'] and not pr['merged'] and issue['state']=='open','PR/Issue/head drift')
need(run['head_sha']==HEAD and run['event']=='pull_request' and run['status']=='completed' and run['conclusion']=='success','Exact HEAD CI differs')
need(len(jobs['jobs'])==jobs['total_count']==13 and all(x['conclusion']=='success' and x['status']=='completed' and x['head_sha']==HEAD for x in jobs['jobs']),'CI jobs incomplete')
need(checks['total_count']==len(checks['check_runs']) and all(x['status']=='completed' and x['conclusion']=='success' and x['head_sha']==HEAD for x in checks['check_runs']),'Current checks incomplete')
need(not threads['pageInfo']['hasNextPage'] and all(x['isResolved'] for x in threads['nodes']),'Review threads not closed')
save(OUT/'github-checkpoint01.json',{'status':'PASS_CURRENT_EXACT_HEAD_DRAFT_OPEN_PR_CI_AND_THREADS','observed_at':now(),'head':HEAD,'pr':332,'draft':True,'merged':False,'issue':307,'issue_state':'OPEN','run_id':run['id'],'event':run['event'],'attempt':run['run_attempt'],'jobs_success':13,'checks_success':len(checks['check_runs']),'unresolved_threads':0,'main_ci':'NOT_CLAIMED','read_times':{k:{x:v[x] for x in ('started_at','finished_at')} for k,v in remote.items()},'native_delete':0})
env=os.environ.copy();env.update(GIT_CONFIG_COUNT='1',GIT_CONFIG_KEY_0='core.longpaths',GIT_CONFIG_VALUE_0='true',PYTHONUTF8='1')
start=now();process=subprocess.run([sys.executable,'-X','utf8','-B','examples/lifecycle-d1/window_six_support.py','check'],capture_output=True,cwd=ROOT,env=env)
write(OUT/'host-check01.stdout.json',process.stdout);write(OUT/'host-check01.stderr.txt',process.stderr)
save(OUT/'host-check-process01.json',{'started_at':start,'finished_at':now(),'exit_code':process.returncode,'entry':'examples/lifecycle-d1/window_six_support.py','entry_sha256':hashlib.sha256((ROOT/'examples/lifecycle-d1/window_six_support.py').read_bytes()).hexdigest(),'action':'check','source_start':0,'provider_io':0,'native_delete':0})
need(process.returncode==0,'Frozen actual-authority Host.check failed; original channels preserved')
value=json.loads(process.stdout);need(value['status']=='PASS_PREWINDOW_CLOSED' and value['host_gate']['permissions_false']==30 and value['host_gate']['budgets_empty']==3,'Host check did not prove closed state')
save(OUT/'authorized-prewindow-check01.json',{'status':'PASS_PREWINDOW_CLOSED','observed_at':now(),'execution':value['execution'],'head':HEAD,'github_checkpoint':'github-checkpoint01.json','actual_host_check':value,'new_native_preflight':'NOT_RUN','schedule':'NOT_CREATED','native_delete':0,'source_start':0,'provider_io':0})
print(json.dumps({'status':'PASS_ACTUAL_AUTHORIZATION_PREWINDOW_CLOSED','execution':value['execution'],'permissions_false':30,'new_budgets_empty':3,'PR_CI':'13/13 SUCCESS','schedule':'NOT_CREATED','native_delete':0}))
