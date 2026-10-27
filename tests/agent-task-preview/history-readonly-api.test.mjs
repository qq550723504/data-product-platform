import test from 'node:test';
import assert from 'node:assert/strict';
const base='http://127.0.0.1:18497',ws='11111111-1111-4111-8111-111111111111',version='4c208c9b-d4d7-42ad-a119-101637d99e77';
const headers={Authorization:'Bearer agent-q2-core-only'};
const cert=`/api/v1/workspaces/${ws}/dataset-versions/${version}/certifications`,releases=`/api/v1/workspaces/${ws}/product-releases`;
test('live certification/release history queries authenticate, scope and keep writes unavailable',async()=>{
 for(const path of [cert,releases]){
  assert.equal((await fetch(base+path)).status,401);
  assert.equal((await fetch(base+path.replace(ws,'33333333-3333-4333-8333-333333333333'),{headers})).status,403);
  assert.ok([404,405].includes((await fetch(base+path,{method:'POST',headers})).status));
  const r=await fetch(base+path+'?limit=25&offset=0',{headers});assert.equal(r.status,200);
  const h=await r.json();assert.equal(h.workspaceId,ws);assert.equal(h.page.limit,25);assert.equal(h.page.offset,0);assert.ok(Array.isArray(h.items));
 }
 const h=await(await fetch(base+cert+'?limit=25&offset=0',{headers})).json();
 assert.equal(h.datasetVersionId,version);
 const q=new URLSearchParams({limit:'25',offset:'25',asOf:h.asOf,anchorRevision:String(h.page.anchorRevision)});
 const next=await(await fetch(base+cert+'?'+q,{headers})).json();assert.equal(next.page.anchorRevision,h.page.anchorRevision);assert.equal(Date.parse(next.asOf),Date.parse(h.asOf));
 assert.equal((await fetch(base+cert.replace(version,'33333333-3333-4333-8333-333333333333'),{headers})).status,404);
 assert.equal((await fetch(base+releases+'?status=SUSPENDED',{headers})).status,400);
 const unknown=await fetch(base+`/api/v1/workspaces/${ws}/certifications/33333333-3333-4333-8333-333333333333/supersede`,{method:'POST',headers});assert.ok([404,405].includes(unknown.status));
});
