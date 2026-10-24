import {readFile,writeFile,mkdir} from 'node:fs/promises';
import path from 'node:path';
import {createHash,randomBytes} from 'node:crypto';
if(process.env.FORK_HOST_LIVE_SYNTHETIC!=='1')throw Error('Explicit synthetic gate required');
const root=process.env.HFRAW_OUTPUT;
const f=JSON.parse(await readFile(path.join(root,'fork-fixture.json'),'utf8'));
const target=JSON.parse(await readFile(path.join(root,'target.json'),'utf8'));
const base=`http://127.0.0.1:${target.ports.fork}`;
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
async function api(url,role='manager',body,key){
 const r=await fetch(base+url,{method:body===undefined?'GET':'POST',headers:{Authorization:`Token ${f.users[role].token}`,'Content-Type':'application/json',...(key?{'Idempotency-Key':key}:{})},body:body===undefined?undefined:JSON.stringify(body),signal:AbortSignal.timeout(60000)});
 const b=await r.text();if(!r.ok)throw Error(`Fork ${r.status} ${url}: ${b.slice(0,500)}`);return JSON.parse(b);
}
async function ready(url,role){for(let n=0;n<180;n++){const r=await api(url,role);if(r.state==='ready')return r;if(['failed_final','quarantined','failed_retryable'].includes(r.state))throw Error(JSON.stringify(r));await sleep(500);}throw Error('RQ readiness timeout');}
for(let n=0;n<60;n++){try{const r=await fetch(base+'/health/');if(r.ok)break;}catch{}if(n===59)throw Error('Fork health timeout');await sleep(500);}
const bundles={};
for(const name of ['normal','budget']){
 const route=`/api/projects/${f.project}/approved-dataset-releases/`;
 const r=await api(route,'manager',{items:name==='normal'?f.approved:f.budget,output:{format:'JSONL',resource_mode:'references_only'}},'live-'+name);
 await ready(route+r.release_id+'/','manager');
 bundles[name]={releaseId:r.release_id,snapshots:[]};
 for(let i=0;i<(name==='normal'?4:1);i++){
  const url=route+r.release_id+'/snapshots/';
  const s=await api(url,'snapshot',{},`live-${name}-snapshot-${i}`);
  const snapshot=await ready(url+s.snapshot_id+'/','snapshot');
  const resp=await fetch(base+url+s.snapshot_id+'/manifest/',{headers:{Authorization:`Token ${f.users.fetch.token}`}});if(!resp.ok)throw Error('real fetch principal cannot read frozen Manifest: '+resp.status);
  const raw=Buffer.from(await resp.arrayBuffer());const manifest=JSON.parse(raw.toString());
  const hash=createHash('sha256').update(raw).digest('hex');if(hash!==snapshot.manifest_sha256)throw Error('Manifest transport checksum mismatch');
  const dir=path.join(root,`${name}-${i}`);await mkdir(dir);await writeFile(path.join(dir,'manifest.json'),raw);
  // Only the small native package is downloaded here. Budget verification
  // must prove the Host declines the actual large package before artifact GET.
  if(name==='normal')for(const a of manifest.artifacts){const v=await fetch(base+url+s.snapshot_id+'/assets/'+a.id+'/download/',{headers:{Authorization:`Token ${f.users.fetch.token}`}});if(!v.ok)throw Error('native download failed');const b=Buffer.from(await v.arrayBuffer());if(createHash('sha256').update(b).digest('hex')!==a.sha256)throw Error('native checksum');await writeFile(path.join(dir,a.name),b);}
  bundles[name].snapshots.push({snapshotId:s.snapshot_id,manifestSha256:hash,dir});
 }
 console.log(`Real Fork/RQ ${name}: READY release and ${bundles[name].snapshots.length} immutable snapshot(s)`);
}
await writeFile(path.join(root,'fork-bundles.json'),JSON.stringify(bundles,null,2));
await writeFile(path.join(root,'fork-token'),f.users.fetch.token,{mode:0o600});
for(const name of ['host-token','control-token','gateway-token','browser-session'])await writeFile(path.join(root,name),randomBytes(32).toString('hex'),{mode:0o600});
console.log('No native bytes were reserialized; private credentials remain outside the repository.');
