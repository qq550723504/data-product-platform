import { IAMError,open,seal } from './iam-core';
export const workspaceCookie='__Host-dpp-workspace';
export function workspaces():{id:string;label:string}[]{
 try{
  const values=JSON.parse(process.env.BROWSER_IAM_WORKSPACES??'[]');
  if(!Array.isArray(values)||!values.length||values.length>8||new Set(values.map(v=>v.id)).size!==values.length)throw new IAMError();
  for(const v of values)if(!v||Object.keys(v).sort().join(',')!=='id,label'||typeof v.label!=='string'||!v.label.trim()||v.label.length>100||!/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(v.id)||v.id==='00000000-0000-0000-0000-000000000000')throw new IAMError();
  return values;
 }catch{throw new IAMError();}
}
function key(){const v=process.env.BROWSER_IAM_ENCRYPTION_KEY??'';if(!/^[A-Za-z0-9_-]{43}$/.test(v))throw new IAMError();return Buffer.from(v,'base64url');}
export function workspaceSelection(cookie?:string){
 const id=cookie?open<string>(key(),'workspace',Buffer.from(cookie,'base64url')):process.env.BROWSER_IAM_WORKSPACE_ID;
 if(!workspaces().some(v=>v.id===id))throw new IAMError();return id!;
}
export function sealedWorkspace(id:string){if(!workspaces().some(v=>v.id===id))throw new IAMError();return seal(key(),'workspace',id).toString('base64url');}
