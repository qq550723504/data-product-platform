import test from 'node:test';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {readFileSync} from 'node:fs';
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

test('lost CREATE response followed by auth/write refusal retains original recovery identity until matching receipt',async()=>{
 for(const [status,code] of [[401,'AUTHENTICATION_REQUIRED'],[403,'AGENT_WRITES_DISABLED'],[404,'AGENT_OBJECT_NOT_FOUND'],[409,'STALE_TASK_REVISION']]){
  const create={path:'',key:'original-create-key',body:{phase:'DIAGNOSIS',sourceVersionId:plan,goal:'diagnose'}};
  const saved={id,taskId:task,planRevisionId:plan,taskRevision:1};
  let pending=create,confirmed=null,creates=0,gets=0;const posts=[];
  const attempt=async response=>{
   // The UI persists unresolved before each network attempt, so reloads cannot
   // downgrade a sent request to a fresh submission.
   const request=pending;pending={...request,unresolved:true};
   const result=await submitAgentCommand(request,{post:async(url,init)=>{
    posts.push({url,key:init.headers['Idempotency-Key'],body:init.body});
    return response();
   },confirmed:r=>{confirmed=r;pending=null;},refresh:async()=>{gets++;}});
   return result;
  };
  assert.equal((await attempt(()=>{creates++;throw Error('committed but response lost');})).state,'UNKNOWN');
  pending=JSON.parse(JSON.stringify(pending)); // browser refresh recovery
  assert.equal((await attempt(()=>({ok:false,status,json:async()=>({error:{code}})}))).state,'UNKNOWN');
  assert.equal(pending.key,create.key);assert.deepEqual(pending.body,create.body);assert.equal(confirmed,null);assert.equal(gets,0);
  assert.equal((await attempt(()=>reply(saved))).state,'CONFIRMED');
  assert.equal(pending,null);assert.deepEqual(confirmed,saved);assert.equal(creates,1);assert.equal(posts.length,3);assert.equal(gets,1);
  for(const sent of posts)assert.deepEqual(sent,{url:'/api/agent-tasks',key:create.key,body:JSON.stringify(create.body)});
 }
});

test('actual workbench reload keeps original pending CREATE through auth refusal and disables a new CREATE',async()=>{
 const React=require('react'),ts=require('typescript');
 const stored={path:'',key:'pending-create-original',body:{phase:'DIAGNOSIS',sourceVersionId:plan,goal:'original'}};
 const saved={id,taskId:task,planRevisionId:plan,taskRevision:1};
 const memory=new Map([['agent-q2-pending-command',JSON.stringify(stored)]]),posts=[],states=[],effects=[];
 let cursor=0,mounted=false;
 const hooks={useState(initial){const n=cursor++;if(!(n in states))states[n]=initial;return [states[n],v=>{states[n]=typeof v==='function'?v(states[n]):v;}];},useEffect(fn){if(!mounted)effects.push(fn);}};
 const imports={react:hooks,'react/jsx-runtime':require('react/jsx-runtime'),'next/link':{default:'a'},'@phosphor-icons/react':{Sparkle:'i',MagnifyingGlass:'i',FileText:'i',ArrowRight:'i'},'@/components/ui':{Badge:({value})=>React.createElement('span',null,value)},'@/lib/agent-command-receipt':require('../.agent-tests/agent-command-receipt.js'),'@/lib/agent-quality':require('../.agent-tests/agent-quality.js')};
 const code=ts.transpileModule(readFileSync(new URL('../app/agent/tasks/workbench.tsx',import.meta.url),'utf8'),{compilerOptions:{jsx:ts.JsxEmit.ReactJSX,module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText;
 const mod={exports:{}};new Function('require','module','exports',code)(name=>{if(name.endsWith('/business-language.mjs'))return require('../lib/business-language.mjs');assert.ok(name in imports,name);return imports[name];},mod,mod.exports);
 const originals=Object.fromEntries(['fetch','sessionStorage','location','history'].map(key=>[key,Object.getOwnPropertyDescriptor(globalThis,key)]));
 try{
  Object.defineProperties(globalThis,{
   sessionStorage:{configurable:true,value:{getItem:k=>memory.get(k)??null,setItem:(k,v)=>memory.set(k,v),removeItem:k=>memory.delete(k)}},
   location:{configurable:true,value:{href:'http://local/agent/tasks',search:'',assign:()=>assert.fail('no redirect')}},
   history:{configurable:true,value:{replaceState:()=>{}}},
   fetch:{configurable:true,value:async(url,init)=>{
    if(init?.method==='POST'){posts.push({key:init.headers['Idempotency-Key'],body:init.body});return posts.length===1?{ok:false,status:401,json:async()=>({error:{code:'AUTHENTICATION_REQUIRED'}})}:reply(saved);}
    const data=url.endsWith('/authority')?{writeEnabled:true,subject:'same-user'}:url.includes(`/${task}?`)?{task:{id:task,currentPlanRevisionId:plan,controlState:'ACTIVE'},plan:{id:plan},plans:[],approvals:[],actions:[]}:{items:[],page:{total:0}};
    return {ok:true,json:async()=>data};
   }},
  });
  const render=()=>{cursor=0;return mod.exports.default({});};
  const nodes=tree=>{const all=[];function walk(n){if(Array.isArray(n))n.forEach(walk);else if(n?.props){all.push(n);walk(n.props.children);}}walk(tree);return all;};
  render();mounted=true;effects.forEach(fn=>fn());await new Promise(setImmediate);
  let tree=render();let retry=nodes(tree).find(n=>n.type==='button'&&n.props.children==='重试相同提交');assert.ok(retry);
  retry.props.onClick();await new Promise(setImmediate);
  tree=render();assert.equal(nodes(tree).find(n=>n.type==='button'&&n.props.children==='保存并冻结计划').props.disabled,true);
  const pending=JSON.parse(memory.get('agent-q2-pending-command'));assert.equal(pending.unresolved,true);assert.equal(pending.key,stored.key);assert.deepEqual(pending.body,stored.body);
  retry=nodes(tree).find(n=>n.type==='button'&&n.props.children==='重试相同提交');assert.ok(retry);retry.props.onClick();await new Promise(setImmediate);
  assert.equal(memory.has('agent-q2-pending-command'),false);assert.equal(posts.length,2);
  for(const sent of posts)assert.deepEqual(sent,{key:stored.key,body:JSON.stringify(stored.body)});
 }finally{for(const [key,descriptor] of Object.entries(originals)){if(descriptor)Object.defineProperty(globalThis,key,descriptor);else delete globalThis[key];}}
});
