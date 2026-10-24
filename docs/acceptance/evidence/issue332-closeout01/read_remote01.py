"""Read published PR boundary only; never calls a mutation or local-source ready."""
import json,sys
from pathlib import Path
root=Path(__file__).resolve().parents[4];out=Path(__file__).parent
private=Path('C:/Users/Henry/.codex/fixture-private/merge-closeout01/remote-boundary01');private.mkdir()
sys.path.insert(0,str(root/'examples/lifecycle-d1'))
import execution_freeze as f
api=f.GitHub(private);s=api.snapshot();f.save(private/'snapshot.json',s)
f.ready(s,'1d142fa332b639144c232bb686be214521b9f352',require_frozen=True)
canonical=lambda value:f.digest(json.dumps(value,sort_keys=True,separators=(',',':')).encode())
f.need(s['rule']['id']=='BPR_kwDOUdQLBs4FE_7b' and canonical(s['protection'])=='5d0546ee5d181e57fc924155bfb7783ec4578bbd8f0189bec954496fc630b6f7','complete protection changed')
q='query($cursor:String){repository(owner:"qq550723504",name:"data-product-platform"){pullRequest(number:332){reviewThreads(first:100,after:$cursor){nodes{id isResolved}pageInfo{hasNextPage endCursor}}}}}'
rows=[];cursor=None
while True:
    page=api.graph(q,dict(cursor=cursor))['repository']['pullRequest']['reviewThreads'];rows+=page['nodes']
    if not page['pageInfo']['hasNextPage']:break
    next_cursor=page['pageInfo']['endCursor'];f.need(next_cursor and next_cursor!=cursor,'bad review cursor');cursor=next_cursor
f.need(all(v['isResolved'] for v in rows),'unresolved published review thread')
f.save(private/'review-threads.json',rows)
value=dict(status='PASS_CURRENT_PUBLISHED_HEAD_BOUNDARY_ONLY',observed_at=f.now(),head=s['head'],
    ci_run=s['ci']['id'],event=s['ci']['event'],jobs=len(s['ci']['jobs']),all_success=True,unresolved_threads=0,
    locked=True,draft=True,merged=False,issue307='open',full_policy_sha256=canonical(s['protection']),
    local_DNS_fix_published=False,local_fix_PR_CI='NOT_RUN',unlock=0,push=0,merge=0)
f.save(out/'remote-boundary-summary01.json',value);print(json.dumps(value))
