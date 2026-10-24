"""Read-only exact failed-base publication observation, never ready/allocation."""
import json,sys
from pathlib import Path
root=Path(__file__).resolve().parents[5];out=Path(__file__).parent
sys.path.insert(0,str(root/'examples/lifecycle-d1'))
import execution_freeze as f
folder=out/'readonly-boundary01';folder.mkdir(exist_ok=False)
api=f.GitHub(folder);s=api.snapshot();f.save(folder/'snapshot.json',s)
f.frozen_rule(s['rule'],f.BRANCH)
f.need(s['head']=='16ec447f2c60ddf08a514207963f478bc70c7e09','remote HEAD drift')
f.need(s['state']=='open' and s['draft'] is True and s['merged'] is False and s['issue_state']=='open','Draft/Open drift')
f.need(s['rule']['id']=='BPR_kwDOUdQLBs4FE_7b','rule drift')
policy=s['protection'];canonical=lambda x:json.dumps(x,sort_keys=True,separators=(',',':')).encode()
full=f.digest(canonical(policy));other=f.digest(canonical({k:v for k,v in policy.items() if k!='lock_branch'}))
f.need(full=='5d0546ee5d181e57fc924155bfb7783ec4578bbd8f0189bec954496fc630b6f7','full policy drift')
f.need(other=='77b0679be0f70f7858a864fd66c90aec78e63dbd387029919ad7a1693c6fc6ab','other policy drift')
ci=s['ci'];counts={k:sum(j['conclusion']==k for j in ci['jobs']) for k in ('success','failure','cancelled')}
f.need(ci['id']==40099260254 and ci['attempt']==1 and ci['event']=='pull_request' and ci['head_sha']==s['head'] and counts==dict(success=11,failure=2,cancelled=0),'known failed CI drift')
try:f.ready(s,s['head'],require_frozen=True)
except ValueError as e:gate=dict(status='REJECTED_EXPECTED_FAILED_CI',reason=str(e))
else:raise ValueError('failed CI incorrectly accepted')
query='query($cursor:String){repository(owner:"qq550723504",name:"data-product-platform"){pullRequest(number:332){reviewThreads(first:100,after:$cursor){nodes{id isResolved isOutdated} pageInfo{hasNextPage endCursor}}}}}'
threads=[];cursor=None
while True:
    page=api.graph(query,dict(cursor=cursor))['repository']['pullRequest']['reviewThreads']
    threads+=page['nodes']
    if not page['pageInfo']['hasNextPage']:break
    cursor=page['pageInfo']['endCursor'];f.need(cursor,'pagination cursor missing')
f.save(folder/'review-threads.json',threads)
f.need(not any(not x['isResolved'] for x in threads),'unresolved threads')
result=dict(status='PASS_READONLY_LOCKED_KNOWN_FAILED_REPAIR_BASE',observed_at=f.now(),head=s['head'],
    full_policy_sha256=full,non_lock_policy_sha256=other,ci_run=ci['id'],ci_counts=counts,
    unresolved_threads=0,allocation_gate=gate,unlock=0,push=0,source_start=0,business_protection_write=0,delete=0)
f.save(folder/'result.json',result);print(json.dumps(result))
