import test from 'node:test';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const require=createRequire(import.meta.url),{matchingAgentReceipt,submitAgentCommand,refreshAgentViews}=require('../.agent-tests/agent-command-receipt.js');
const task='11111111-1111-4111-8111-111111111111',plan='22222222-2222-4222-8222-222222222222',id='33333333-3333-4333-8333-333333333333',approval='44444444-4444-4444-8444-444444444444',action='55555555-5555-4555-8555-555555555555';
const req={path:`/${task}/approvals`,key:'same-immutable-key',body:{expectedRevision:2,planRevisionId:plan,contentHash:'f'.repeat(64),phase:'REPAIR',reason:'reviewed'}};
const receipt={id,taskId:task,planRevisionId:plan,taskRevision:3,approvalId:approval};
const reply=data=>({ok:true,status:200,json:async()=>data});
test('2xx malformed or mismatched receipts retain original pending request; retries use identical key/body',async()=>{
 const sent=[];let pending=req,confirmations=0,reads=0;
 for(const value of [{},{...receipt,taskId:action},{...receipt,planRevisionId:action},{...receipt,taskRevision:4},{...receipt,approvalId:undefined}]){
  const result=await submitAgentCommand(pending,{post:async(url,init)=>{sent.push({url,key:init.headers['Idempotency-Key'],body:init.body});return reply(value);},confirmed:()=>{confirmations++;pending=null;},refresh:async()=>{reads++;}});
  assert.equal(result.state,'UNKNOWN');assert.equal(pending,req);
 }
 assert.equal(confirmations,0);assert.equal(reads,0);assert.equal(sent.length,5);for(const s of sent){assert.equal(s.key,req.key);assert.equal(s.body,JSON.stringify(req.body));assert.equal(s.url,'/api/agent-tasks'+req.path);}
});
test('confirmed command survives read failures; only readonly recovery runs afterwards, never another POST',async()=>{
 let pending=req,saved=null,posts=0,gets=0;let broken=true;
 const refresh=()=>refreshAgentViews(async()=>{gets++;if(broken)throw Error('list unavailable');},async()=>{gets++;throw Error('detail unavailable');});
 const result=await submitAgentCommand(req,{post:async()=>{posts++;return reply(receipt);},confirmed:r=>{saved=r;pending=null;},refresh});
 assert.equal(result.state,'CONFIRMED');assert.equal(result.readFailed,true);assert.equal(pending,null);assert.deepEqual(saved,receipt);assert.equal(posts,1);assert.equal(gets,2);
 broken=false;await refreshAgentViews(async()=>{gets++;},async()=>{gets++;});assert.equal(posts,1);assert.equal(gets,4);assert.deepEqual(saved,receipt);
});
test('receipt binding handles CREATE, revisions, action intent and historical same-key replay',()=>{
 assert.equal(matchingAgentReceipt({id,taskId:task,planRevisionId:plan,taskRevision:1},{path:'',key:'create',body:{}}),true);
 assert.equal(matchingAgentReceipt({...receipt,approvalId:undefined,taskRevision:1},{path:'',key:'create',body:{}}),true);
 const revise={...req,path:`/${task}/revisions`,body:{expectedRevision:2,basePlanRevisionId:plan}};
 assert.equal(matchingAgentReceipt({...receipt,approvalId:undefined,planRevisionId:action},revise),true);assert.equal(matchingAgentReceipt({...receipt,approvalId:undefined},revise),false);
 const intent={...req,path:`/${task}/actions`,body:{...req.body,approvalId:approval}};
 assert.equal(matchingAgentReceipt({...receipt,actionId:action},intent),true);assert.equal(matchingAgentReceipt({...receipt,actionId:action,approvalId:task},intent),false);
 assert.equal(matchingAgentReceipt(receipt,req),true); // Original receipt remains valid even when a later read has a newer revision.
});
test('ambiguous transport stays pending; authoritative rejection never invokes confirmation or refresh',async()=>{
 for(const response of [{ok:false,status:503,json:async()=>({error:{code:'AGENT_UNAVAILABLE'}})},{ok:false,status:409,json:async()=>({error:{code:'STALE_TASK_REVISION'}})},{ok:false,status:429,json:async()=>({error:{code:'RATE_LIMITED'}})}]){
 const result=await submitAgentCommand(req,{post:async()=>response,confirmed:()=>assert.fail('not confirmed'),refresh:async()=>assert.fail('no read')});assert.equal(result.state,response.status===409?'REJECTED':'UNKNOWN');
 }
});
