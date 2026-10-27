import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {randomUUID} from 'node:crypto';
const fixture=JSON.parse(await readFile(new URL('../../.data/agent-q2-preview/repair-fixture.json',import.meta.url),'utf8'));
const base='http://127.0.0.1:18497/api/v1/workspaces/11111111-1111-4111-8111-111111111111/agent-tasks';
const headers={Authorization:'Bearer agent-q2-core-only','Content-Type':'application/json'};
async function post(path,body,key=randomUUID()){const r=await fetch(base+path,{method:'POST',headers:{...headers,'Idempotency-Key':key},body:JSON.stringify(body)});return {status:r.status,data:await r.json()};}
async function detail(id){return (await fetch(base+'/'+id,{headers})).json();}
test('REPAIR bindings, independent approval, same-key intent and persisted reload',async()=>{
 const common={sourceVersionId:fixture.sourceVersionId,goal:'REPAIR API acceptance',purposeNote:'synthetic only',acceptanceText:'no dispatch',reason:'isolated acceptance'};
 const created=await post('',{...common,phase:'DIAGNOSIS'});assert.equal(created.status,200);const id=created.data.taskId;
 let d=await detail(id);let request={...common,...fixture,phase:'REPAIR',expectedRevision:d.task.revision,basePlanRevisionId:d.plan.id};
 const uiRejected=await fetch('http://127.0.0.1:4189/api/agent-tasks/'+id+'/revisions',{method:'POST',headers:{Authorization:'Basic '+Buffer.from('operator:agent-q2-preview').toString('base64'),Origin:'http://127.0.0.1:4189','Content-Type':'application/json','Idempotency-Key':randomUUID()},body:JSON.stringify({...request,inputs:fixture.inputs.slice(0,1)})});assert.equal(uiRejected.status,409);assert.equal((await uiRejected.json()).error.code,'INPUT_BINDING_INCOMPLETE');
 const missing=await post('/'+id+'/revisions',{...request,inputs:fixture.inputs.slice(0,1)});assert.equal(missing.status,409);assert.equal(missing.data.code,'INPUT_BINDING_INCOMPLETE');assert.equal((await detail(id)).task.revision,1);
 const repaired=await post('/'+id+'/revisions',request);assert.equal(repaired.status,200);d=await detail(id);assert.equal(d.plan.phase,'REPAIR');assert.equal(d.plan.inputs.length,4);assert.equal(d.plan.caseId,fixture.caseId);assert.equal(d.approvals.length,0);
 const confirmation={expectedRevision:d.task.revision,planRevisionId:d.plan.id,contentHash:d.plan.contentHash,phase:'REPAIR',reason:'confirm repair'};
 const approval=await post('/'+id+'/approvals',confirmation);assert.equal(approval.status,200);d=await detail(id);assert.equal(d.approvals.length,1);
 const action={...confirmation,expectedRevision:d.task.revision,approvalId:approval.data.approvalId,commandKind:'ASSIGN_SELF'};const key=randomUUID();const intent=await post('/'+id+'/actions',action,key);assert.equal(intent.status,200);assert.deepEqual((await post('/'+id+'/actions',action,key)).data,intent.data);
 d=await detail(id);assert.equal(d.actions.length,1);assert.equal(d.actions[0].commandKind,'ASSIGN_SELF');assert.equal(d.actions[0].dispatchState,'REQUESTED');assert.equal(d.dispatchEnabled,false);
 assert.equal((await post('/'+id+'/actions',{...action,expectedRevision:d.task.revision})).status,409);
 assert.equal((await post('/'+id+'/actions',{...action,expectedRevision:d.task.revision,commandKind:'EXECUTE_REPAIR'})).status,409);
 assert.equal((await post('/'+id+'/stop',{expectedRevision:d.task.revision,reason:'end synthetic acceptance'})).status,200);
});
