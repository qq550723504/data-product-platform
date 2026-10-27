import test from 'node:test';
import assert from 'node:assert/strict';
import {demoStorageKeys,readDemoSession,saveDemoSession,resetDemoSession} from '../../components/governance-demo/demo-session.mjs';
test('demo reload preserves workspace and global examples; reset leaves Core pending requests intact',()=>{
 const values=new Map([['agent-task-pending:keep','original-command']]);let destination;
 globalThis.sessionStorage={getItem:key=>values.get(key)??null,setItem:(key,value)=>values.set(key,value),removeItem:key=>values.delete(key)};
 globalThis.location={assign:value=>destination=value};
 const registry={currentId:'demo-2',items:[{id:'demo-1'},{id:'demo-2'}]};
 const data={'global:admin-state':{config:{timezone:'UTC'}},'demo-2:pipeline-state':{plans:[{id:'PLAN-2'}]}};
 assert.equal(saveDemoSession(demoStorageKeys.registry,registry),true);saveDemoSession(demoStorageKeys.data,data);
 assert.deepEqual(readDemoSession(demoStorageKeys.registry,{}),registry);assert.deepEqual(readDemoSession(demoStorageKeys.data,{}),data);
 resetDemoSession();assert.equal(destination,'/#home');assert.deepEqual([...values],[['agent-task-pending:keep','original-command']]);
});
test('invalid or unavailable browser storage cannot become a workspace or block the demo',()=>{
 const fallback={currentId:'seed',items:[{id:'seed'}]};
 globalThis.sessionStorage={getItem:()=>'{"currentId":"missing","items":[]}',setItem:()=>{throw Error('unavailable')}};
 assert.equal(readDemoSession(demoStorageKeys.registry,fallback),fallback);assert.equal(saveDemoSession(demoStorageKeys.data,{}),false);
 globalThis.sessionStorage={getItem:()=>'{broken'};assert.equal(readDemoSession(demoStorageKeys.data,fallback),fallback);
});
