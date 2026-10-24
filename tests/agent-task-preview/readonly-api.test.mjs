import test from 'node:test';
import assert from 'node:assert/strict';
const base='http://127.0.0.1:18497';
const workspace='11111111-1111-4111-8111-111111111111';
const version='4c208c9b-d4d7-42ad-a119-101637d99e77';
const assessment='d3667496-f421-497c-be69-e5dec6bb569b';
const headers={Authorization:'Bearer agent-q2-core-only'};
const context=`/api/v1/workspaces/${workspace}/agent-quality-context?limit=25&offset=0`;
test('isolated Core queries enforce identity, exact source and disabled execution routes',async()=>{
 const response=await fetch(base+context,{headers});assert.equal(response.status,200);
 const catalog=await response.json();assert.equal(catalog.workspaceId,workspace);assert.equal(catalog.mode,'READ_ONLY');
 assert.ok(catalog.datasets.items.length>0);assert.ok(catalog.datasets.items.every(d=>d.workspaceId===workspace));
 assert.equal((await fetch(base+context)).status,401);
 assert.equal((await fetch(base+context.replace(workspace,'22222222-2222-4222-8222-222222222222'),{headers})).status,403);
 assert.equal((await fetch(base+context+'&datasetId=not-an-id',{headers})).status,400);
 assert.equal((await fetch(base+context+'&datasetId=33333333-3333-4333-8333-333333333333',{headers})).status,404);
 const report=await fetch(`${base}/api/v1/quality-assessments/${assessment}/report?workspaceId=${workspace}&limit=25&offset=0`,{headers});assert.equal(report.status,200);
 const data=await report.json();assert.equal(data.id,assessment);assert.equal(data.datasetVersionId,version);assert.equal(data.workspaceId,workspace);
 for(const action of ['quality-checks','gold-quality-checks']) {
  const denied=await fetch(`${base}/api/v1/dataset-versions/${version}/${action}`,{method:'POST',headers});assert.ok([404,405].includes(denied.status));
 }
 const task=await(await fetch(`${base}/api/v1/workspaces/${workspace}/agent-tasks/ef6e94ef-23fb-4afe-8296-b9960ce86127`,{headers})).json();
 assert.equal(task.task.revision,4);assert.equal(task.dispatchEnabled,false);assert.equal(task.actions[0].dispatchState,'REQUESTED');
});

test('live assessment costs retain explicit scope and pricing uncertainty',async()=>{
 const detail=await(await fetch(`${base}/api/v1/quality-assessments/${assessment}?workspaceId=${workspace}`,{headers})).json();
 assert.equal(detail.id,assessment);assert.equal(detail.datasetVersionId,version);assert.ok(Array.isArray(detail.costEvents));
 assert.ok(detail.costEvents.every(c=>c.workspaceId===workspace&&typeof c.quantity==='number'));
});
