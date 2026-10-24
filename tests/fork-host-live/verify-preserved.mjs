import {readFile,writeFile} from 'node:fs/promises';
import {isDeepStrictEqual} from 'node:util';
import path from 'node:path';
const root=process.argv[2];if(!root)throw Error('Baseline directory required');
const before=JSON.parse(await readFile(path.join(root,'protected-before.json'),'utf8'));
const after=JSON.parse(await readFile(path.join(root,'protected-after.json'),'utf8'));
const byId=new Map(after.map(x=>[x.id,x]));
// JSON object property order differs between Docker Go templates and PowerShell
// serialization. Compare values, preserving mount fields and destination scope.
const normalize=x=>({...x,mounts:[...(x.mounts??[])].sort((a,b)=>a.Destination.localeCompare(b.Destination))});
const changed=before.filter(x=>!byId.has(x.id)||!isDeepStrictEqual(normalize(x),normalize(byId.get(x.id)))).map(x=>x.name);
const result={compared:before.length,changed,issue76:after.filter(x=>x.name.includes('issue76')).map(x=>({name:x.name,running:x.running}))};
await writeFile(path.join(root,'protected-comparison.json'),JSON.stringify(result,null,2));
console.log(JSON.stringify({compared:result.compared,changed:changed.length,issue76Running:result.issue76.every(x=>x.running)}));
if(changed.length)process.exitCode=1;
