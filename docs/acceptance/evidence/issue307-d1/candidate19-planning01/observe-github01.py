"""Record exact PR HEAD/CI/threads and issue state; no GitHub mutation."""
import argparse
import datetime as dt
import json
from pathlib import Path
import subprocess

ROOT=Path(__file__).resolve().parents[5]
HERE=Path(__file__).parent
parser=argparse.ArgumentParser();parser.add_argument('phase',choices=['prepublish','published','final'])
args=parser.parse_args()
folder=HERE/('github-'+args.phase+'01');folder.mkdir(exist_ok=False)
def gh(*argv):
 r=subprocess.run(['gh',*argv],cwd=ROOT,capture_output=True)
 if r.returncode:raise RuntimeError(r.stderr.decode('utf-8','replace'))
 return json.loads(r.stdout)
def save(name,value):
 with (folder/name).open('xb') as f:f.write((json.dumps(value,ensure_ascii=False,indent=2)+'\n').encode('utf-8'))
query='''query { repository(owner:"qq550723504",name:"data-product-platform") {
 pullRequest(number:332) { headRefOid headRefName state isDraft mergedAt url reviewThreads(first:100) {
 pageInfo { hasNextPage endCursor } nodes { id isResolved isOutdated path comments(last:2) { nodes { body author { login } url } } } } }
 issue(number:307) { state url }
} }'''
data=gh('api','graphql','-f','query='+query)
save('pull-request-threads-issue.json',data)
repo=data['data']['repository'];pr=repo['pullRequest']
if pr['reviewThreads']['pageInfo']['hasNextPage']:raise ValueError('Review thread inventory requires pagination')
head=pr['headRefOid']
runs=gh('api','repos/qq550723504/data-product-platform/actions/runs','-X','GET','-f','head_sha='+head,'-f','per_page=100')
save('runs.json',runs)
checks=gh('api','repos/qq550723504/data-product-platform/commits/'+head+'/check-runs','-f','per_page=100','-X','GET')
save('check-runs.json',checks)
unresolved=[x for x in pr['reviewThreads']['nodes'] if not x['isResolved']]
receipt={'phase':args.phase,'at':dt.datetime.now(dt.timezone.utc).isoformat(),'head':head,'draft':pr['isDraft'],
 'PR_state':pr['state'],'merged_at':pr['mergedAt'],'Issue307':repo['issue']['state'],'unresolved_threads':len(unresolved),
 'runs':[{'id':x['id'],'head':x['head_sha'],'event':x['event'],'status':x['status'],'conclusion':x['conclusion']} for x in runs['workflow_runs']],
 'checks':[{'name':x['name'],'head':x['head_sha'],'status':x['status'],'conclusion':x['conclusion']} for x in checks['check_runs']],
 'main_ci_claimed':False,'native_delete':0,'execution_authority':False}
save('receipt.json',receipt)
print(json.dumps(receipt,ensure_ascii=False))
if unresolved:print(json.dumps({'unresolved':unresolved},ensure_ascii=False))
