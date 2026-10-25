"""Fresh exact GitHub and actual human reference checks; no source starts."""
from pathlib import Path
import concurrent.futures
import datetime as dt
import hashlib
import json
import subprocess
ROOT=Path.cwd();OUT=Path(__file__).resolve().parent;PARENT=OUT.parent
CONTROL=Path(r'C:\Users\Henry\.codex\fixture-private\remaining-candidate19\authorized-execution01')
HEAD='caab46da8761e71e1d80d0c0931707a410c13a4d'
def now():return dt.datetime.now(dt.timezone.utc).isoformat()
def need(ok,message):
    if not ok:raise ValueError(message)
def load(p):return json.loads(p.read_bytes())
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def save(p,v):
    with p.open('xb') as f:f.write((json.dumps(v,ensure_ascii=False,indent=2)+'\n').encode())
def raw(p,v):
    with p.open('xb') as f:f.write(v)
at=dt.datetime.now(dt.timezone.utc)
need(dt.datetime(2026,10,22,6,0,tzinfo=dt.timezone.utc)<=at<dt.datetime(2026,10,22,6,40,tzinfo=dt.timezone.utc),'Fixed window closed')
need(not (CONTROL/'terminal.json').exists(),'Execution already terminal')
need(subprocess.check_output(['git','-c','core.longpaths=true','rev-parse','HEAD']).decode().strip()==HEAD and not subprocess.check_output(['git','-c','core.longpaths=true','diff','--name-only','HEAD']).strip(),'Tracked source/HEAD changed')
a=load(PARENT/'human-authorization.json')
need(sha(PARENT/'human-authorization.json')=='c4e37256edd4138f5d443287373efb6b0e1810c559fb0899caf014762840d858' and a['execution']=='a5d1e569-f80e-4b38-8f32-d8139a30a077','New authority differs')
for name,key in [('user-message-verbatim.txt','user_message_sha256'),('authorization-envelope.txt','authorization_envelope_sha256'),('approved-host-bindings.txt','additional_host_bindings_sha256'),('approved-reference-manifest.json','approved_reference_manifest_sha256')]:
    need(sha(CONTROL/name)==a[key],'Original private authority byte drift')
need((CONTROL/'user-message-verbatim.txt').read_bytes()==a['user_message_verbatim'].encode(),'Original human text differs')
for x in a['approved_reference_artifacts']:
    need(sha(ROOT/x['path'])==x['sha256'] and (ROOT/x['path']).stat().st_size==x['bytes'],'Approved byte drift: '+x['path'])
need(sha(PARENT/'schedule-human-authority.json')=='8d6aa007f7f3308e0997ac50fe8f46853c121e9bacc0710a904c2f29f84e7504','Schedule authority differs')
queries={'pr':['api','repos/qq550723504/data-product-platform/pulls/332'],'issue':['api','repos/qq550723504/data-product-platform/issues/307'],'ci':['api','repos/qq550723504/data-product-platform/actions/runs/39710200629'],'jobs':['api','repos/qq550723504/data-product-platform/actions/runs/39710200629/jobs?per_page=100'],'checks':['api','repos/qq550723504/data-product-platform/commits/'+HEAD+'/check-runs?per_page=100'],'threads':['api','graphql','-f','query={repository(owner:"qq550723504",name:"data-product-platform"){pullRequest(number:332){reviewThreads(first:100){nodes{id isResolved} pageInfo{hasNextPage}}}}}']}
def fetch(pair):
    name,args=pair;start=now();r=subprocess.run(['gh',*args],capture_output=True,timeout=40)
    raw(OUT/(name+'-fresh.stdout.json'),r.stdout);raw(OUT/(name+'-fresh.stderr.txt'),r.stderr)
    need(r.returncode==0,'Fresh GitHub read failed: '+name)
    return name,{'started_at':start,'observed_at':now(),'value':json.loads(r.stdout)}
with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:values=dict(pool.map(fetch,queries.items()))
pr=values['pr']['value'];issue=values['issue']['value'];ci=values['ci']['value'];jobs=values['jobs']['value'];checks=values['checks']['value'];threads=values['threads']['value']['data']['repository']['pullRequest']['reviewThreads']
need(pr['head']['sha']==HEAD and pr['state']=='open' and pr['draft'] and not pr['merged'] and issue['state']=='open','Fresh PR/Issue status differs')
need(ci['head_sha']==HEAD and ci['event']=='pull_request' and ci['status']=='completed' and ci['conclusion']=='success','Exact HEAD PR CI differs')
need(jobs['total_count']==len(jobs['jobs'])==13 and all(x['status']=='completed' and x['conclusion']=='success' and x['head_sha']==HEAD for x in jobs['jobs']),'Exact HEAD CI jobs incomplete')
need(checks['total_count']==len(checks['check_runs'])==13 and all(x['status']=='completed' and x['conclusion']=='success' and x['head_sha']==HEAD for x in checks['check_runs']),'Current checks incomplete')
need(not threads['pageInfo']['hasNextPage'] and all(x['isResolved'] for x in threads['nodes']),'Open review threads')
save(OUT/'fresh-github-and-authorization.json',{'status':'PASS_FRESH_EXACT_HEAD_CI_DRAFT_OPEN_AND_CURRENT_AUTHORIZATION','at':now(),'execution':a['execution'],'head':HEAD,'approved_artifacts':len(a['approved_reference_artifacts']),'PR_CI':{'id':ci['id'],'event':ci['event'],'jobs':13,'success':True,'main_ci_claimed':False},'unresolved_threads':0,'new_source_start':0,'new_provider_io':0,'native_delete':0})
print(json.dumps({'status':'PASS_FRESH_EXACT_HEAD_CI_DRAFT_OPEN_AND_CURRENT_AUTHORIZATION','at':now(),'native_delete':0}))

