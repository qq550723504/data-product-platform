"""Read-only observation for a CI repair proposal; never a ready/allocation gate."""
import hashlib,json,sys
from pathlib import Path
root=Path(__file__).resolve().parents[5];out=Path(__file__).parent
sys.path.insert(0,str(root/'examples/lifecycle-d1'))
import execution_freeze as f
folder=out/'publication-boundary-read01';folder.mkdir(exist_ok=False)
snapshot=f.GitHub(folder).snapshot();f.save(folder/'snapshot.json',snapshot)
f.frozen_rule(snapshot['rule'],f.BRANCH)
f.need(snapshot['head']=='1d3e417bd059aaaee099161cf6e12b4c43fed81c','unexpected remote HEAD')
f.need(snapshot['state']=='open' and snapshot['draft'] is True and snapshot['merged'] is False and snapshot['issue_state']=='open','Draft/Open boundary changed')
f.need(snapshot['rule']['id']=='BPR_kwDOUdQLBs4FE_7b','different protection owner')
policy=snapshot['protection'];full=f.digest(json.dumps(policy,sort_keys=True,separators=(',',':')).encode())
other=f.digest(json.dumps({k:v for k,v in policy.items() if k!='lock_branch'},sort_keys=True,separators=(',',':')).encode())
ci=snapshot['ci'];f.need(ci['id']==40090889280 and ci['attempt']==1 and ci['event']=='pull_request' and ci['head_sha']==snapshot['head'],'CI provenance drift')
counts={key:sum(x['conclusion']==key for x in ci['jobs']) for key in ('success','failure','cancelled')}
f.need(counts==dict(success=9,failure=3,cancelled=1),'known failed CI changed')
try:f.ready(snapshot,snapshot['head'],require_frozen=True)
except ValueError as error:green_gate=dict(status='REJECTED_EXPECTED_KNOWN_FAILED_CI',reason=str(error))
else:raise ValueError('failed CI was incorrectly accepted')
f.save(folder/'result.json',dict(status='PASS_READONLY_LOCKED_REPAIR_BOUNDARY',head=snapshot['head'],full_policy_sha256=full,non_lock_policy_sha256=other,ci_counts=counts,allocation_gate=green_gate,unlock=0,push=0,source_start=0,business_protection_write=0,delete=0))
print(json.dumps(dict(head=snapshot['head'],counts=counts,full_policy_sha256=full,non_lock_policy_sha256=other,allocation_gate=green_gate)))
