// Additional fresh Host workspace: mid-flight authorization and real Fork
// quarantine. Retained corrupt objects from earlier cases are never repaired.
import {spawn} from 'node:child_process';
import {createServer,request} from 'node:http';
import {readFile,writeFile,appendFile,unlink} from 'node:fs/promises';
import {createWriteStream} from 'node:fs';
import {fileURLToPath} from 'node:url';
import path from 'node:path';
import assert from 'node:assert/strict';
if(process.env.FORK_HOST_LIVE_SYNTHETIC!=='1')throw Error('Explicit synthetic gate required');
const root=process.env.HFRAW_OUTPUT,repo=fileURLToPath(new URL('../../',import.meta.url));
const read=async n=>JSON.parse(await readFile(path.join(root,n),'utf8'));
const t=await read('target.json'),b=await read('fork-bundles.json'),f=await read('fork-fixture.json');
const token=await readFile(path.join(root,'host-token'),'utf8'),secret=await readFile(path.join(root,'control-token'),'utf8');
const dsn=await readFile(path.join(root,'postgres-dsn'),'utf8');
const results=[];let child,meta,revoke=false;const trace=[];
const done=c=>new Promise((r,j)=>{c.once('error',j);c.once('exit',n=>n===0?r():j(Error('test process failed: '+n)));});
async function api(url,options={}){const r=await fetch(url,{...options,signal:AbortSignal.timeout(60000)});return{status:r.status,body:await r.json().catch(()=>({}))};}
const control=async action=>{const r=await api('http://127.0.0.1:25513/__live/control',{method:'POST',headers:{'X-Live-Control':secret},body:JSON.stringify({action})});assert.equal(r.status,204);};
const state=async()=>{const r=await api('http://127.0.0.1:25513/__live/state',{headers:{'X-Live-Control':secret}});assert.equal(r.status,200);return r.body;};
const check=async(name,fn)=>{await fn();results.push({name,status:'PASS'});console.log('PASS '+name);await writeFile(path.join(root,'integrity-results.json'),JSON.stringify(results,null,2));};
const source=createServer((req,res)=>{const p=request({hostname:'127.0.0.1',port:25510,path:req.url,method:req.method,headers:{...req.headers,host:'127.0.0.1:25510'}},r=>{const chunks=[];r.on('data',v=>chunks.push(v));r.on('end',async()=>{const data=Buffer.concat(chunks);const item={method:req.method,path:req.url,status:r.statusCode,bytes:data.length};trace.push(item);await appendFile(path.join(root,'source-transport.jsonl'),JSON.stringify(item)+'\n');if(revoke&&req.url.endsWith('/manifest/')){revoke=false;await control('deny');}res.writeHead(r.statusCode,r.headers);res.end(data);});});p.on('error',()=>{res.writeHead(502);res.end();});req.pipe(p);});
await new Promise(r=>source.listen(25517,'127.0.0.1',r));
try{
 const binary=process.env.HFRAW_LIVE_BINARY;if(!binary)throw Error('Verified compiled fixture binary required');
 try{await unlink(path.join(root,'live-ready.json'));}catch(e){if(e.code!=='ENOENT')throw e;}
 const log=createWriteStream(path.join(root,'host-live.log'));
 child=spawn(binary,['-test.run=^TestLiveForkHostFixture$','-test.timeout=15m','-test.v'],{cwd:path.join(repo,'apps/platform/internal/sourceadmission'),env:{...process.env,TEST_POSTGRES_DSN:dsn},windowsHide:true,stdio:['ignore','pipe','pipe']});child.stdout.pipe(log);child.stderr.pipe(log);child.once('exit',()=>log.end());
 for(let i=0;i<150;i++){try{meta=await read('live-ready.json');break;}catch{}if(child.exitCode!==null)throw Error('fixture startup failed');await new Promise(r=>setTimeout(r,200));}if(!meta)throw Error('readiness timeout');
 const base=`http://127.0.0.1:25513/api/v1/workspaces/${meta.workspaceId}/source-admissions`,s=b.normal.snapshots[3];
 const body={datasetId:meta.datasetId,connectionId:meta.connectionId,releaseId:b.normal.releaseId,snapshotId:s.snapshotId,manifestSha256:s.manifestSha256};
 const admit=(key,input=body)=>api(base,{method:'POST',headers:{Authorization:'Bearer '+token,'Idempotency-Key':key,'Content-Type':'application/json'},body:JSON.stringify(input)});
 await check('real Manifest read racing Host revocation denies before native fetch; original key resumes',async()=>{const before=await state();revoke=true;assert.equal((await admit('live-midflight')).status,403);const denied=await state();assert.deepEqual(denied,{...before,calls:before.calls+1,costs:before.costs+1,allocations:before.allocations+1});assert.equal((await api(base+'?key=live-midflight',{headers:{Authorization:'Bearer '+token}})).status,403);await control('enable');const resumed=await admit('live-midflight');assert.equal(resumed.status,200,JSON.stringify(resumed));assert.equal(resumed.body.rowCount,3);});
 await check('wrong expected Manifest hash rejects before artifact fetch/copy/version allocation',async()=>{const before=await state();const r=await admit('live-wrong-hash',{...body,manifestSha256:'0'.repeat(64)});assert.equal(r.status,409);const after=await state();assert.deepEqual(after,{...before,calls:before.calls+1,costs:before.costs+1,allocations:before.allocations+1,rejected:before.rejected+1});});
 await check('real Fork artifact corruption quarantines frozen snapshot; Host rejects without repair',async()=>{
  const code=`import os,json; from django.conf import settings; from pathlib import Path; from data_export.snapshot_models import ReleaseSnapshot; from data_export import snapshots; assert os.environ.get('FORK_HOST_LIVE_SYNTHETIC')=='1' and settings.DATABASES['default']['NAME']=='hfraw_fork'; f=json.loads(Path('/trial/private/fork-fixture.json').read_text()); s=ReleaseSnapshot.objects.get(pk='${s.snapshotId}',project_id=f['project'],state='ready'); m=snapshots.verify(s); assets=snapshots.assets(s,m); a=next(v for v in assets.values() if v['name']=='records.jsonl'); p=Path(a['path']).resolve(); assert p.is_relative_to(Path(settings.APPROVED_EXPORT_ROOT).resolve()); p.write_bytes(b'synthetic-frozen-source-corruption'); print('Corrupted one explicitly selected synthetic private artifact; retained for diagnosis')`;
  const log=createWriteStream(path.join(root,'source-corruption.log'));const c=spawn('docker',['exec',t.app,'python','manage.py','shell','-c',code],{windowsHide:true,stdio:['ignore','pipe','pipe']});c.stdout.pipe(log);c.stderr.pipe(log);await done(c);log.end();
  const before=await state();assert.equal((await admit('live-source-corrupt')).status,409);const after=await state();assert.deepEqual(after,{...before,calls:before.calls+1,costs:before.costs+1,allocations:before.allocations+1,rejected:before.rejected+1});
  const checkCode=`from data_export.snapshot_models import ReleaseSnapshot; s=ReleaseSnapshot.objects.get(pk='${s.snapshotId}'); assert s.state=='quarantined'; print('Frozen source is quarantined; no recapture or repair')`;
  const out=createWriteStream(path.join(root,'source-quarantine.log'));const q=spawn('docker',['exec',t.app,'python','manage.py','shell','-c',checkCode],{windowsHide:true,stdio:['ignore','pipe','pipe']});q.stdout.pipe(out);q.stderr.pipe(out);await done(q);out.end();
  const historical=await api(base+'?key=live-midflight',{headers:{Authorization:'Bearer '+token}});assert.equal(historical.body.state,'ACCEPTED_SOURCE');
 });
 await check('additional fault/denial physical source calls retain exact costs and allocations',async()=>{const s=await state();assert.equal(s.calls,trace.length);assert.equal(s.costs,s.calls);assert.equal(s.allocations,s.calls);await writeFile(path.join(root,'accounting.json'),JSON.stringify(s,null,2));});
 const stopped=done(child);await control('stop');await stopped;child=null;
}finally{if(child&&child.exitCode===null){try{await control('stop');}catch{}await new Promise(r=>setTimeout(r,500));if(child.exitCode===null)child.kill();}await new Promise(r=>source.close(r));}
