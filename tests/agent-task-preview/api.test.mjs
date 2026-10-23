import test from 'node:test';
import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
const base='http://127.0.0.1:18497/api/v1/workspaces/11111111-1111-4111-8111-111111111111/agent-tasks';
const headers={Authorization:'Bearer agent-q2-core-only','Content-Type':'application/json'};
async function post(path,body,key=randomUUID()){const response=await fetch(base+path,{method:'POST',headers:{...headers,'Idempotency-Key':key},body:JSON.stringify(body)});return {status:response.status,data:await response.json()};}
test('real plan-only HTTP acceptance: concurrent replay, revision binding, held action and stop',async()=>{
 const request={phase:'DIAGNOSIS',sourceVersionId:'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',goal:'API 验收诊断',purposeNote:'合成验收',acceptanceText:'只保存范围，不执行',reason:'验收'};
 const key=randomUUID();const created=await Promise.all(Array.from({length:6},()=>post('',request,key)));
 for(const r of created){assert.equal(r.status,200);assert.equal(r.data.id,created[0].data.id);assert.equal(r.data.taskId,created[0].data.taskId);}
 const task=created[0].data.taskId;
 assert.equal((await post('',{...request,goal:'变化'},key)).status,409);
 const detail=await(await fetch(base+'/'+task,{headers})).json();
 const confirm={expectedRevision:1,planRevisionId:detail.plan.id,contentHash:detail.plan.contentHash,phase:'DIAGNOSIS',reason:'确认'};
 const approval=await post('/'+task+'/approvals',confirm);assert.equal(approval.status,200);
 const action={...confirm,expectedRevision:2,approvalId:approval.data.approvalId,commandKind:'DETECT'};
 const intentKey=randomUUID();const intent=await post('/'+task+'/actions',action,intentKey);assert.equal(intent.status,200);
 assert.deepEqual((await post('/'+task+'/actions',action,intentKey)).data,intent.data);
 assert.equal((await post('/'+task+'/actions',{...action,expectedRevision:3})).status,409);
 assert.equal((await post('/'+task+'/stop',{expectedRevision:3,reason:'验收结束，取消未派发意图'})).status,200);
 const stopped=await(await fetch(base+'/'+task,{headers})).json();assert.equal(stopped.task.controlState,'STOPPED');assert.equal(stopped.actions[0].dispatchState,'CANCELLED');assert.equal(stopped.dispatchEnabled,false);
});
test('anonymous direct Next mutation cannot borrow deployment read credential',async()=>{
 const r=await fetch('http://127.0.0.1:4188/api/agent-tasks',{method:'POST',headers:{Origin:'http://127.0.0.1:4188','Content-Type':'application/json','Idempotency-Key':randomUUID()},body:'{}'});assert.equal(r.status,401);
});
test('Core rejects forged identity and cross-workspace requests',async()=>{
 assert.equal((await post('',{actorId:'22222222-2222-4222-8222-222222222222',reason:'forged'})).status,400);
 const r=await fetch(base.replace('11111111-1111-4111-8111-111111111111','99999999-9999-4999-8999-999999999999'),{headers});assert.equal(r.status,403);
});
