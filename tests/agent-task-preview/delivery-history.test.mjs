import test from 'node:test';
import assert from 'node:assert/strict';
const token=process.env.READONLY_PREVIEW_TOKEN;
if(!token)throw Error('explicit local acceptance token required');
const base='http://127.0.0.1:18497',ws='11111111-1111-4111-8111-111111111111',id='40404040-4040-4040-8040-404040404040',missing='99999999-9999-4999-8999-999999999999';
async function get(endpoint,query='',status=200,auth=true,workspace=ws){const r=await fetch(`${base}/api/v1/workspaces/${workspace}/${endpoint}${query}`,{headers:auth?{Authorization:`Bearer ${token}`}:{}});assert.equal(r.status,status);return r.json()}
test('readonly delivery projection keeps BLOCKED and explicit gate/transition history; no secret-bearing fields',async()=>{
 const p=await get('delivery-history','?id='+id);assert.equal(p.page.total,1);const o=p.items[0];assert.equal(o.status,'BLOCKED');assert.equal(o.gateDecision,'BLOCKED');assert.equal(o.evaluations[0].decision,'BLOCKED');assert.equal(o.transitions[0].to,'BLOCKED');assert.equal(o.transitions[0].evaluationId,o.evaluations[0].id);
 for(const field of ['credential','provider_request','idempotency','principal_ref','scope_ref','SYNTHETIC_NO_PROVIDER'])assert.ok(!JSON.stringify(p).includes(field));
 const empty=await get('delivery-history','?id='+missing);assert.deepEqual(empty.items,[]);assert.equal(empty.page.total,0);
 const beyond=await get('delivery-history','?offset=25');assert.deepEqual(beyond.items,[]);assert.equal(beyond.page.total,1);
});
test('archive lists frozen snapshot header and exact published reference without manifest/payload leakage',async()=>{
 const p=await get('evidence-archives');const row=p.items.find(x=>x.releaseIds.includes('30303030-3030-4030-8030-303030303030'));assert.ok(row);assert.equal(row.itemCount,1);assert.match(row.rootHash,/^[a-f0-9]{64}$/);const exact=await get('evidence-archives','?id='+row.id);assert.equal(exact.items[0].id,row.id);for(const field of ['manifest','metadata','storageUri','credential'])assert.ok(!JSON.stringify(p).includes(field));
});
test('both history endpoints reject untrusted/foreign/repeated/unsupported input and expose no commands',async()=>{
 for(const endpoint of ['delivery-history','evidence-archives']){
  await get(endpoint,'',401,false);await get(endpoint,'',403,true,missing);
  for(const query of ['?id=bad','?id=','?id='+id+'&id='+id,'?offset=-1','?consumer=forged'])await get(endpoint,query,400);
  const r=await fetch(`${base}/api/v1/workspaces/${ws}/${endpoint}`,{method:'POST',headers:{Authorization:`Bearer ${token}`}});assert.equal(r.status,405);
 }
 const a=await fetch(`${base}/api/v1/workspaces/${ws}/agent-task-authority`,{headers:{Authorization:`Bearer ${token}`}});assert.equal(a.status,200);const authority=await a.json();assert.equal(authority.writeEnabled,false);assert.equal(authority.dispatchEnabled,false);
});
