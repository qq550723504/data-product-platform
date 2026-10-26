import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {syntheticSource} from './helpers/runtime-log-source.mjs';
const require=createRequire(import.meta.url),{runtimeLogAPI,logSelection,logHref,logListHref,logNow,logTime,loadRuntimeLogs,validLogList,validLogDetail}=require('../.runtime-log-tests/lib/runtime-logs.js');
function setup(){const source=syntheticSource(),p={principal_id:'11111111-1111-4111-8111-111111111111',workspace_id:'33333333-3333-4333-8333-333333333333',revision:'1:1:1',expires_at:new Date(Date.now()+60000).toISOString(),capabilities:['PLATFORM_MANAGEMENT_READ']};return {source,p,reader:source.reader(async()=>p,'A'),q:source.window}}
test('default entry stays unconfigured for every URL; no synthetic mode or data fallback',async()=>{for(const q of [{},{mode:'synthetic'},{start:'bad',entryRef:'forged'}])assert.deepEqual(await loadRuntimeLogs(q,null),{code:'LOG_SOURCE_UNCONFIGURED'})});
test('HTTP adapter preserves management-only profile and excludes navigation markers from API requests',async()=>{
 const {reader,q,p,source}=setup(),paths=[],client={principal:p,apiBaseUrl:'https://api.invalid',confirm:()=>reader.confirm(),async request(url){const u=new URL(url);paths.push(u);let value;try{value=u.pathname.endsWith('/source')?await reader.source():await reader.query(Object.fromEntries(u.searchParams));return new Response(JSON.stringify(value),{status:200})}catch{return new Response(JSON.stringify({code:'LOG_ACCESS_DENIED'}),{status:403})}}};
 const wire=runtimeLogAPI(client);const result=await loadRuntimeLogs({...q,from:'configuration',focus:'A'.repeat(43)},wire);assert.ok(result.result);assert.deepEqual((await wire.confirm()).capabilities,['PLATFORM_MANAGEMENT_READ']);assert.equal(paths.at(-1).searchParams.has('from'),false);assert.equal(paths.at(-1).searchParams.has('focus'),false);source.state.logRead=false;assert.equal((await loadRuntimeLogs(q,wire)).code,'LOG_ACCESS_DENIED');
});
test('query uses canonical UTC nanoseconds and rejects ambiguous selectors or widened windows',()=>{
 const {q}=setup();assert.deepEqual(logSelection({...q,service:'',level:''}),q);assert.equal(logTime('2026-02-30T00:00:00.000000000Z'),false);
 for(const patch of [{start:q.end},{end:'2099-01-01T00:00:00.000000000Z'},{start:'2026-01-01T00:00:00Z'},{start:['a','b']},{tenant:'B'},{service:'constructor'},{cursor:'x'},{from:'//external.invalid'}])assert.throws(()=>logSelection({...q,...patch}));
 assert.equal(logNow(0),'1970-01-01T00:00:00.000000000Z');
});
test('58 same-time/adjacent-nanosecond occurrences paginate without missing or merging identities',async()=>{
 const {reader,q}=setup(),keys=[],times=[];let s=q;do{const v=await loadRuntimeLogs(s,reader);assert.ok(v.result,v.code);keys.push(...v.result.data.items.map(e=>e.rowKey));times.push(...v.result.data.items.map(e=>e.recordedAt));s={...q,cursor:v.result.data.next}}while(s.cursor);assert.equal(keys.length,58);assert.equal(new Set(keys).size,58);assert.equal(new Set(times).size,2);assert.equal(times.filter(t=>t===times[0]).length,28);assert.equal(BigInt(times[0].slice(20,29))-BigInt(times.at(-1).slice(20,29)),1n);
});
test('detail round-trip preserves fixed filters cursor and stable focus despite rotating references',async()=>{
 const {reader,q}=setup(),one=await loadRuntimeLogs(q,reader),two=await loadRuntimeLogs({...q,cursor:one.result.data.next},reader),s=two.selection,e=two.result.data.items.at(-1),url=new URL(logHref(s,{entryRef:e.entryRef,focus:e.rowKey,from:'configuration'}),'https://app.invalid');
 const detail=await loadRuntimeLogs(Object.fromEntries(url.searchParams),reader);assert.equal(detail.detail.data.item.rowKey,e.rowKey);const back=new URL(logListHref(detail.selection,e.rowKey),url);assert.equal(back.searchParams.get('cursor'),s.cursor);assert.equal(back.searchParams.get('from'),'configuration');assert.equal(back.searchParams.has('entryRef'),false);assert.equal(back.hash,'#log-'+e.rowKey);
 const refreshed=await loadRuntimeLogs(s,reader);assert.deepEqual(refreshed.result.data.items.map(e=>e.rowKey),two.result.data.items.map(e=>e.rowKey));assert.notEqual(refreshed.result.data.items[0].entryRef,two.result.data.items[0].entryRef);
});
test('cross-space and cross-person references are rejected by the source before any foreign result',async()=>{
 const {source,reader,q,p}=setup(),one=await loadRuntimeLogs(q,reader),foreign=source.reader(async()=>({...p,workspace_id:'44444444-4444-4444-8444-444444444444'}),'B'),other=source.reader(async()=>({...p,principal_id:'22222222-2222-4222-8222-222222222222'}),'A');
 for(const r of [foreign,other])for(const patch of [{cursor:one.result.data.next},{entryRef:one.result.data.items[0].entryRef}]){const v=await loadRuntimeLogs({...q,...patch},r);assert.equal(v.code,'LOG_ACCESS_DENIED');assert.equal(v.result,undefined);assert.equal(v.detail,undefined)}
});
test('records require independent capability and discard in-flight revocation or identity revision changes',async()=>{
 for(const change of ['capability','revision','expiry','principal','workspace']){const {source,reader,q,p}=setup();source.state.afterRead=async()=>{if(change==='capability')source.state.logRead=false;else if(change==='revision')p.revision='1:1:2';else if(change==='expiry')p.expires_at=new Date(0).toISOString();else if(change==='principal')p.principal_id='other';else p.workspace_id='other'};const v=await loadRuntimeLogs(q,reader);assert.ok(v.code);assert.equal(v.result,undefined)}
 const {source,reader,q}=setup();source.state.logRead=false;assert.equal((await loadRuntimeLogs(q,reader)).code,'LOG_ACCESS_DENIED');assert.equal(source.state.calls.length,0);
});
test('in-flight detail revocation and source version change discard results and keep valid recovery filters',async()=>{
 for(const change of ['permission','source']){const {source,reader,q}=setup(),v=await loadRuntimeLogs(q,reader);source.state.afterRead=async()=>{if(change==='permission')source.state.logRead=false;else source.state.version++};const detail=await loadRuntimeLogs({...q,entryRef:v.result.data.items[0].entryRef},reader);assert.equal(detail.code,change==='permission'?'LOG_ACCESS_DENIED':'LOG_QUERY_EXPIRED');assert.equal(detail.detail,undefined)}
 const {reader,q}=setup(),bad=await loadRuntimeLogs({...q,cursor:'x'},reader);assert.equal(bad.code,'INVALID_LOG_QUERY');assert.deepEqual(bad.selection,q);
});
test('strict projections reject private fields wrong scope timestamps ordering duplicate and excess rows',async()=>{
 const {reader,q,p}=setup(),v=await loadRuntimeLogs(q,reader),principal={...p,capabilities:[...p.capabilities,'PLATFORM_RUNTIME_LOG_READ']};
 assert.equal(validLogList(v.result,principal,q),true);
 for(const mutate of [r=>r.data.items[0].body='PRIVATE',r=>r.workspaceId='foreign',r=>r.authorityRevision='1:1:2',r=>r.writeEnabled=true,r=>r.data.items[0].recordedAt='bad',r=>r.data.items.push(r.data.items[0]),r=>r.data.items[1]=r.data.items[0],r=>r.data.items[0].service='host:/PRIVATE',r=>r.data.window.start=logNow(0)]){const copy=structuredClone(v.result);mutate(copy);assert.equal(validLogList(copy,principal,q),false)}
 const e=v.result.data.items[0],s={...q,entryRef:e.entryRef,focus:e.rowKey},d=await loadRuntimeLogs(s,reader);assert.equal(validLogDetail(d.detail,principal,s),true);assert.equal(validLogDetail({...d.detail,data:{...d.detail.data,item:{...d.detail.data.item,entryRef:'X'.repeat(43)}}},principal,s),false);
});
test('expired changed deleted source errors and empty results retain distinct recovery states',async()=>{
 for(const state of ['expired','changed','deleted','empty','unavailable']){const {source,reader,q}=setup(),one=await loadRuntimeLogs(q,reader),e=one.result.data.items[0];if(state==='expired')source.state.version++;if(state==='changed')source.state.records.find(r=>source.rowKey(r)===e.rowKey).level='FATAL';if(state==='deleted')source.state.records=source.state.records.filter(r=>source.rowKey(r)!==e.rowKey);if(state==='empty')source.state.records=[];if(state==='unavailable')source.state.fault=()=>{throw Error('PRIVATE_PROVIDER_ERROR')};const v=await loadRuntimeLogs({...q,...state==='empty'||state==='unavailable'?{}:{entryRef:e.entryRef}},reader);assert.equal(v.code,({expired:'LOG_QUERY_EXPIRED',changed:'LOG_ENTRY_CHANGED',deleted:'LOG_ENTRY_UNAVAILABLE',empty:undefined,unavailable:'LOG_SOURCE_UNAVAILABLE'})[state]);if(state==='empty')assert.equal(v.result.data.items.length,0)}
});
