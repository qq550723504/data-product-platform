import test from 'node:test';
import assert from 'node:assert/strict';
const base='http://127.0.0.1:18497',ws='11111111-1111-4111-8111-111111111111',other='33333333-3333-4333-8333-333333333333';
const headers={Authorization:'Bearer agent-q2-core-only'};
test('Trace requires server identity and workspace; missing roots stay missing and commands are unavailable',async()=>{
 for(const path of [`/api/v1/traceability/product-releases/${other}`,`/api/v1/traceability/EXECUTION/${other}`]){
 assert.equal((await fetch(base+path+'?workspaceId='+ws)).status,401);
 assert.equal((await fetch(base+path+'?workspaceId='+other,{headers})).status,403);
 const r=await fetch(base+path+'?workspaceId='+ws,{headers});assert.equal(r.status,404);assert.equal(r.headers.get('cache-control'),'private, no-store');
 assert.ok([404,405].includes((await fetch(base+path+'?workspaceId='+ws,{method:'POST',headers})).status));
 }
 assert.equal((await fetch(base+`/api/v1/traceability/DATASET_VERSION/${other}?workspaceId=${ws}`,{headers})).status,400);
});
