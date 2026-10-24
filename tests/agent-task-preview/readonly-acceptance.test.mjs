import test from 'node:test';
import assert from 'node:assert/strict';
const base='http://127.0.0.1:18497',ws='11111111-1111-4111-8111-111111111111',version='bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',release='30303030-3030-4030-8030-303030303030',missing='33333333-3333-4333-8333-333333333333';
const token=process.env.READONLY_API_TOKEN;if(!token)throw Error('explicit local preview credential required');
const headers={Authorization:'Bearer '+token};
async function get(path,status=200,auth=true){const r=await fetch(base+path,{headers:auth?headers:{},redirect:'error'});assert.equal(r.status,status,path);return r;}
test('current Agent/context/quality/history queries remain available with explicit identity',async()=>{
 const root=`/api/v1/workspaces/${ws}`;
 const authority=await(await get(root+'/agent-task-authority')).json();assert.equal(authority.writeEnabled,false);assert.equal(authority.dispatchEnabled,false);
 for(const path of [root+'/agent-tasks?offset=0',root+'/agent-quality-context?limit=25&offset=0',root+`/dataset-versions/${version}/certifications?limit=25&offset=0`,root+'/product-releases?limit=25&offset=0',`/api/v1/dataset-versions/${version}/quality-assessments?workspaceId=${ws}&limit=25&offset=0&summary=true`,`/api/v1/quality-assessments/cccccccc-cccc-4ccc-8ccc-cccccccccccc/report?workspaceId=${ws}&limit=25&offset=0`])await get(path);
});
test('Trace returns the exact synthetic frozen evidence and scoped safe projection',async()=>{
 const path=`/api/v1/traceability/product-releases/${release}?workspaceId=${ws}`;
 const r=await get(path);assert.equal(r.headers.get('cache-control'),'private, no-store');const t=await r.json();
 assert.equal(t.releaseId,release);assert.equal(t.workspaceId,ws);assert.equal(t.status,'PUBLISHED');assert.equal(t.evidenceSnapshot.integrityValid,true);assert.equal(t.evidenceSnapshot.items.length,1);assert.equal(t.datasetVersions[0].id,version);assert.equal(t.executions.length,0);assert.deepEqual(t.entityMappings,[]);
 for(const field of ['storageUri','metadata','manifest','sourceKey','q1-storage-private'])assert.ok(!JSON.stringify(t).includes(field),field);
 await get(path,401,false);await get(`/api/v1/traceability/product-releases/${release}?workspaceId=${missing}`,403);await get(`/api/v1/traceability/product-releases/${missing}?workspaceId=${ws}`,404);await get(path+'&workspaceId='+ws,400);await get(`/api/v1/traceability/DATASET_VERSION/${version}?workspaceId=${ws}`,400);
});
test('API and BFF refuse commands while GETs and authority remain read only',async()=>{
 const r=await fetch(base+`/api/v1/workspaces/${ws}/agent-tasks`,{method:'POST',headers:{...headers,'Content-Type':'application/json'},body:'{}'});assert.equal(r.status,403);assert.ok((await r.text()).includes('AGENT_WRITES_DISABLED'));
 const bff=await fetch('http://127.0.0.1:4188/api/agent-tasks',{method:'POST',headers:{...headers,'Content-Type':'application/json',Origin:'http://127.0.0.1:4188'},body:'{}'});assert.equal(bff.status,403);
 const a=await fetch('http://127.0.0.1:4188/api/agent-tasks/authority',{headers});assert.equal(a.status,200);assert.equal((await a.json()).writeEnabled,false);
});

test('gateway blocks non-read methods before reverse proxy',async()=>{
 const auth=process.env.READONLY_GATEWAY_AUTH;if(!auth)throw Error('explicit local gateway credential required');
 const r=await fetch('http://127.0.0.1:4189/api/agent-tasks',{method:'POST',headers:{Authorization:auth,'Content-Type':'application/json'},body:'{}'});assert.equal(r.status,405);
 const page=await fetch('http://127.0.0.1:4189/core/releases',{headers:{Authorization:auth}});assert.equal(page.status,200);
});
