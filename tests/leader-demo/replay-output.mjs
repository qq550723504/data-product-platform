import {mkdir} from 'node:fs/promises';
import {resolve,relative,isAbsolute,dirname,sep} from 'node:path';
import {pathToFileURL} from 'node:url';
import {randomUUID} from 'node:crypto';

// A replay writes a new evidence set. It never replaces a prior delivery/run.
export async function createReplayOutput(repoRoot,requested){
 const root=resolve(repoRoot);
 const target=requested?resolve(root,requested):resolve(root,'.data/leader-demo-runs',new Date().toISOString().replace(/[:.]/g,'-')+'-'+randomUUID().slice(0,8));
 const local=relative(root,target);
 if(!local||local==='..'||local.startsWith('..'+sep)||isAbsolute(local))throw Error('Replay output must be a new directory inside this checkout');
 await mkdir(dirname(target),{recursive:true});
 await mkdir(target); // EEXIST is deliberate; do not mix screenshots or runs.
 return pathToFileURL(target+sep);
}
