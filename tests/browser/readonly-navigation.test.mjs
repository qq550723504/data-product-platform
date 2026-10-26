import {createServer} from 'node:http';
import {once} from 'node:events';
import {mkdirSync,existsSync,writeFileSync} from 'node:fs';
import {createRequire} from 'node:module';
import assert from 'node:assert/strict';
import {reloadReadOnly} from './readonly-navigation.mjs';

const {chromium}=createRequire(import.meta.url)('@playwright/test');
const channel=process.env.READONLY_BROWSER_CHANNEL??(process.platform==='win32'?'msedge':'chromium');
assert.ok(['msedge','chromium'].includes(channel));
const output=process.env.READONLY_NAVIGATION_OUTPUT??'readonly-browser-navigation-20261026';
assert.match(output,/^readonly-browser-navigation-[a-z0-9-]+$/);
const out=new URL('../../.data/'+output+'/',import.meta.url);
if(existsSync(out))throw Error('previous document-readiness evidence retained');
mkdirSync(out,{recursive:true});
const results=[];
let release,requests=0,finished=false,premature=false,browser,pending;
const firstFinished=new Promise(r=>release=r);
const server=createServer(async(req,res)=>{
 if(!req.url.startsWith('/records?')){res.writeHead(204);res.end();return}
 requests++;
 if(requests===1){
  res.writeHead(200,{'Content-Type':'text/html','Cache-Control':'no-store'});
  res.write('<!doctype html><title>Records</title><p>Loading');
  await firstFinished;finished=true;res.end('</p>');
 }else{
  if(!finished){premature=true;res.writeHead(409);res.end('DOCUMENT_NOT_READY');return}
  res.writeHead(200,{'Content-Type':'text/html','Cache-Control':'no-store'});
  res.end('<!doctype html><title>Records</title><p>Ready</p>');
 }
});
try{
 server.listen(0,'127.0.0.1');await once(server,'listening');
 browser=await chromium.launch({...(channel==='msedge'?{channel}:{}),headless:true});
 const page=await browser.newPage(),errors=[];
 page.on('pageerror',e=>errors.push(e.message));
 const target='http://127.0.0.1:'+server.address().port+'/records?offset=25&role=viewer#selected';
 await page.goto(target,{waitUntil:'commit'});
 assert.notEqual(await page.evaluate(()=>document.readyState),'complete');
 let resolved=false;
 const observed={waitForLoadState:state=>page.waitForLoadState(state),waitForFunction:fn=>page.waitForFunction(fn),reload:options=>{assert.equal(finished,true,'reload invoked before document completion');return page.reload(options)}};
 pending=reloadReadOnly(observed).then(r=>{resolved=true;return r});pending.catch(()=>{});
 // This response is deliberately unfinished, not an arbitrary timing delay.
 await new Promise(r=>setImmediate(r));
 assert.equal(requests,1);assert.equal(resolved,false);
 release();const response=await pending;
 assert.equal(response.status(),200);assert.equal(requests,2);assert.equal(premature,false);
 assert.equal(page.url(),target);assert.deepEqual(errors,[]);
 results.push({name:'unfinished destination blocks reload until load and preserves exact conditions and focus',status:'PASS'});
 let reloads=0;
 await assert.rejects(reloadReadOnly({async waitForLoadState(){throw Error('DESTINATION_FAILED')},async reload(){reloads++}}),/DESTINATION_FAILED/);
 assert.equal(reloads,0);
 results.push({name:'failed readiness never reloads or retries',status:'PASS'});
 console.log('PASS document-readiness browser contract: 2 cases');
}catch(e){results.push({name:'document-readiness contract',status:'FAIL',error:e.message});process.exitCode=1;console.error('FAIL document-readiness contract: '+e.message)}
finally{
 release();await pending?.catch(()=>{});await browser?.close();server.closeAllConnections();await new Promise(r=>server.close(r));
 writeFileSync(new URL('results.json',out),JSON.stringify({channel,results,scope:'Loopback synthetic document only; no application API, DB or sources',businessWrites:false,models:false,dispatch:false},null,2));
}
