import type {Principal} from './iam-core';
export const managementMenus=[['','设置总览'],['workspaces','工作空间'],['members','空间成员'],['users','用户目录'],['organizations','组织机构'],['roles','角色权限'],['audit','审计记录'],['logs','运行日志'],['configuration','基础配置']];
export type ManagementSection='members'|'roles'|'audit';
export type Member={id:string;userId:string;organizationId:string;projectId:string;displayName:string;loginName:string;roles:string[];state:'active'|'inactive';createdAt:string;changedAt:string;permissions:string[]};
export type Role={id:string;name:string;permissions:string[]};
export type Audit={id:string;actorId:string|null;actorType:'USER'|'SYSTEM'|'SERVICE';action:string;objectType:string;objectId:string;occurredAt:string};
export type ManagementResult={profileVersion:'platform-management.v1';workspaceId:string;principalId:string;authorityRevision:string;evaluatedAt:string;source:'SYNTHETIC_FIXTURE'|'CONFIGURED';writeEnabled:false;data:{items:(Member|Role|Audit)[];total?:number;offset?:number;limit?:number;selected?:Member|null;roleDefinitions?:Role[];policySource?:string;next?:string}};
export type ManagementSelection={offset:number;q?:string;role?:string;state?:string;memberId?:string;cursor?:string};
const uuid=(v:unknown)=>typeof v==='string'&&/^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(v);
const id=(v:unknown)=>typeof v==='string'&&/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/.test(v);
const text=(v:unknown,n=512)=>typeof v==='string'&&new TextEncoder().encode(v).length<=n;
const time=(v:unknown)=>typeof v==='string'&&Number.isFinite(Date.parse(v));
const exact=(v:object,keys:string)=>Object.keys(v).sort().join(',')===keys.split(',').sort().join(',');
export const permissionLabels:Record<string,string>={'management.members.read':'查看空间成员','management.roles.read':'查看应用角色权限','management.audit.read':'查看空间审计'};
const permissions=(v:unknown):v is string[]=>Array.isArray(v)&&new Set(v).size===v.length&&v.every(p=>typeof p==='string'&&Object.hasOwn(permissionLabels,p));
const roles=(v:unknown):v is Role[]=>Array.isArray(v)&&v.length>0&&v.length<=64&&new Set(v.map(r=>r.id)).size===v.length&&v.every(r=>r&&exact(r,'id,name,permissions')&&id(r.id)&&text(r.name,200)&&r.name.trim()&&permissions(r.permissions));
function member(v:Member):boolean{return !!v&&exact(v,'id,userId,organizationId,projectId,displayName,loginName,roles,state,createdAt,changedAt,permissions')&&[v.id,v.userId,v.organizationId,v.projectId].every(id)&&text(v.displayName)&&text(v.loginName)&&Array.isArray(v.roles)&&v.roles.length<=32&&new Set(v.roles).size===v.roles.length&&v.roles.every(id)&&['active','inactive'].includes(v.state)&&time(v.createdAt)&&time(v.changedAt)&&permissions(v.permissions);}
export function managementSelection(section:ManagementSection,q:Record<string,string|string[]|undefined>):ManagementSelection{
 const allowed=section==='members'?['offset','q','role','state','memberId']:section==='audit'?['cursor']:[];
 for(const [k,v]of Object.entries(q))if(!allowed.includes(k)||typeof v!=='string'||!v&&!['q','state','role'].includes(k))throw Error('INVALID_QUERY');
 const offset=Number(q.offset??0),s:ManagementSelection={offset};
 if(!Number.isInteger(offset)||offset<0||offset>10000||q.offset!==undefined&&String(offset)!==q.offset)throw Error('INVALID_QUERY');
 for(const k of ['q','role','state','memberId','cursor']as const){const v=q[k];if(typeof v==='string'&&v)s[k]=v;}
 if(s.q&&(!text(s.q,200)||!s.q.trim()||/[\u0000-\u001f\u007f]/.test(s.q))||s.role&&!id(s.role)||s.memberId&&!id(s.memberId)||s.state&&!['active','inactive'].includes(s.state)||s.cursor&&(!/^[A-Za-z0-9_-]+$/.test(s.cursor)||s.cursor.length>1024))throw Error('INVALID_QUERY');
 return s;
}
export function managementQuery(s:ManagementSelection){const q=new URLSearchParams();if(s.offset)q.set('offset',String(s.offset));for(const k of ['q','role','state','memberId','cursor']as const)if(s[k])q.set(k,s[k]!);return q.size?'?'+q:'';}
function memberListSelection(s:ManagementSelection):ManagementSelection{return {offset:s.offset,q:s.q,role:s.role,state:s.state};}
export function managementMemberListHref(s:ManagementSelection,focusMemberId?:string){
 if(focusMemberId&&!id(focusMemberId))throw Error('INVALID_QUERY');
 return '/settings/members'+managementQuery(memberListSelection(s))+(focusMemberId?'#member-'+encodeURIComponent(focusMemberId):'');
}
export function managementMemberDetailHref(s:ManagementSelection,memberId:string){
 if(!id(memberId))throw Error('INVALID_QUERY');
 return '/settings/members'+managementQuery({...memberListSelection(s),memberId});
}
export function validManagement(raw:ManagementResult,p:Principal,kind:ManagementSection,s:ManagementSelection):boolean{
 try{
  if(!raw||!exact(raw,'profileVersion,workspaceId,principalId,authorityRevision,evaluatedAt,source,writeEnabled,data')||raw.profileVersion!=='platform-management.v1'||raw.workspaceId!==p.workspace_id||raw.principalId!==p.principal_id||raw.authorityRevision!==p.revision||!time(raw.evaluatedAt)||!['SYNTHETIC_FIXTURE','CONFIGURED'].includes(raw.source)||raw.writeEnabled!==false||!raw.data||!Array.isArray(raw.data.items))return false;
  const d=raw.data;
  if(kind==='members'){
   if(!exact(d,'items,total,offset,limit,selected,roleDefinitions')||!roles(d.roleDefinitions)||d.offset!==s.offset||d.limit!==25||!Number.isInteger(d.total)||d.total!<0||d.total!>10000||d.items.length!==Math.min(25,Math.max(0,d.total!-s.offset))||!d.items.every(v=>member(v as Member))||new Set(d.items.map(v=>v.id)).size!==d.items.length)return false;
   const items=d.items as Member[];if(items.some(m=>s.state&&m.state!==s.state||s.role&&!m.roles.includes(s.role)))return false;
   if(s.memberId?(!d.selected||d.selected.id!==s.memberId||!member(d.selected)):d.selected!==null)return false;
   return !d.selected||items.every(m=>m.organizationId===d.selected!.organizationId&&m.projectId===d.selected!.projectId);
  }
  if(kind==='roles')return exact(d,'items,policySource')&&d.policySource==='APPLICATION_CATALOG'&&roles(d.items);
  return exact(d,'items,next')&&d.items.length<=25&&typeof d.next==='string'&&(!d.next||/^[A-Za-z0-9_-]{1,1024}$/.test(d.next)&&d.items.length===25)&&new Set(d.items.map(v=>v.id)).size===d.items.length&&d.items.every(v=>{const a=v as Audit;return exact(a,'id,actorId,actorType,action,objectType,objectId,occurredAt')&&uuid(a.id)&&(a.actorId===null||uuid(a.actorId))&&['USER','SYSTEM','SERVICE'].includes(a.actorType)&&text(a.action,128)&&!!a.action&&text(a.objectType,64)&&!!a.objectType&&uuid(a.objectId)&&time(a.occurredAt)});
 }catch{return false;}
}
