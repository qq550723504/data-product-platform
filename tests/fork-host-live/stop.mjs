// Stop only exact containers from this trial. Retain volumes, objects and logs.
import {readFile,writeFile} from 'node:fs/promises';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import path from 'node:path';
if(process.env.FORK_HOST_LIVE_SYNTHETIC!=='1'||!process.env.HFRAW_OUTPUT)throw Error('Explicit synthetic gate/private target required');
const root=process.env.HFRAW_OUTPUT,t=JSON.parse(await readFile(path.join(root,'target.json'),'utf8')),exec=promisify(execFile),stopped=[];
if(!t.synthetic||!/^hfraw-[0-9]+$/.test(t.id))throw Error('Unsafe target');
for(const key of ['app','worker','minio','redis','pg']){
 const name=t[key];if(!name?.startsWith(t.id+'-'))throw Error('Wrong resource scope');
 let v;try{const r=await exec('docker',['inspect','--format','{{json .Name}} {{json .Config.Labels}} {{json .State.Running}}',name],{windowsHide:true});v=r.stdout;}catch(e){if(String(e.stderr).includes('No such object'))continue;throw e;}
 if(!v.includes('"codex.owner":"fork-host-live"')||!v.startsWith('"/'+name+'" '))throw Error('Ownership mismatch');
 await exec('docker',['stop',name],{windowsHide:true});stopped.push(name);
}
await writeFile(path.join(root,'owned-stopped.json'),JSON.stringify({stopped,volumesRetained:true,objectsRetained:true},null,2));console.log(`Stopped ${stopped.length} exact owned containers; all data retained.`);
