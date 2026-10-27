import type {Principal} from './iam-core';
export const managementMenus=[['','设置总览'],['workspaces','工作空间'],['members','空间成员'],['users','用户目录'],['organizations','组织机构'],['roles','角色权限'],['audit','审计记录'],['logs','运行日志'],['configuration','基础配置']];
export type ManagementSection='members'|'roles'|'audit'|'users'|'workspaces'|'organizations'|'configuration';
export type Member={id:string;userId:string;organizationId:string;projectId:string;displayName:string;loginName:string;roles:string[];state:'active'|'inactive';createdAt:string;changedAt:string;permissions:string[]};
export type Role={id:string;name:string;permissions:string[]};
export type Audit={id:string;actorId:string|null;actorType:'USER'|'SYSTEM'|'SERVICE';action:string;objectType:string;objectId:string;occurredAt:string};
export type ManagementResult={profileVersion:'platform-management.v1';workspaceId:string;principalId:string;authorityRevision:string;evaluatedAt:string;source:'SYNTHETIC_FIXTURE'|'CONFIGURED';writeEnabled:false;data:{items:(Member|Role|Audit)[];total?:number;offset?:number;limit?:number;selected?:Member|null;roleDefinitions?:Role[];policySource?:string;next?:string}};
type ManagementOrigin='workspace'|'organization'|'configuration';
export type MemberNavigation={offset:number;q?:string;role?:string;state?:string;memberId:string;from?:ManagementOrigin};
export type ManagementSelection={offset:number;q?:string;role?:string;state?:string;memberId?:string;cursor?:string;userId?:string;from?:ManagementOrigin;memberContext?:MemberNavigation};
export type ConfigurationManagementResult=Omit<ManagementResult,'data'>&{data:{scope:'CURRENT_WORKSPACE_MANAGEMENT';mode:'QUERY_ONLY';directory:{state:'CONFIGURED'|'UNCONFIGURED';source:'CONFIGURED'|'SYNTHETIC_FIXTURE'|'UNCONFIGURED';availability:'NOT_CHECKED'};queries:{workspace:true;organization:boolean;members:boolean;users:boolean;roles:boolean;audit:boolean};rolePolicySource:'APPLICATION_CATALOG'|null}};
export type OrganizationManagementResult=Omit<ManagementResult,'data'>&{data:{organization:{id:string;name:string|null;state:'active'|'inactive'|'removed';createdAt:string|null;changedAt:string|null};departmentHierarchyState:'UNCONFIGURED'}};
export type WorkspaceManagementResult=Omit<ManagementResult,'data'>&{data:{recordState:'UNCONFIGURED';confirmationExpiresAt:string;directory:{state:'CONFIGURED'|'UNCONFIGURED';source:'CONFIGURED'|'SYNTHETIC_FIXTURE'|'UNCONFIGURED';organizationId:string|null;projectId:string|null;availability:'NOT_CHECKED'};queries:{members:boolean;users:boolean;roles:boolean;audit:boolean}}};
export type UserReference={id:string;displayName:string;loginName:string;membershipCount:number};
export type UserProfile={id:string;organizationId:string;username:string;displayName:string;accountState:'active'|'inactive'|'locked'|'initial'|'deleted';createdAt:string;changedAt:string};
export type UserIdentityLink={workspaceId:string;state:'UNCONFIGURED'|'UNLINKED'|'LINKED';actorId:string|null;revision:string;basisRef:string;expiresAt:string|null};
export type UserDetail={profile:UserProfile;memberships:Member[];identity:UserIdentityLink};
export type UserManagementResult=Omit<ManagementResult,'data'>&{data:{items:UserReference[];total:number;offset:number;limit:25;selected:UserDetail|null;roleDefinitions:Role[]}};
const uuid=(v:unknown)=>typeof v==='string'&&/^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(v);
const id=(v:unknown)=>typeof v==='string'&&/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/.test(v);
const text=(v:unknown,n=512)=>typeof v==='string'&&new TextEncoder().encode(v).length<=n;
const time=(v:unknown)=>typeof v==='string'&&Number.isFinite(Date.parse(v));
const exact=(v:object,keys:string)=>Object.keys(v).sort().join(',')===keys.split(',').sort().join(',');
export const permissionLabels:Record<string,string>={'management.members.read':'查看空间成员','management.roles.read':'查看应用角色权限','management.audit.read':'查看空间审计','management.users.read':'查看用户目录'};
const permissions=(v:unknown):v is string[]=>Array.isArray(v)&&new Set(v).size===v.length&&v.every(p=>typeof p==='string'&&Object.hasOwn(permissionLabels,p));
const roles=(v:unknown):v is Role[]=>Array.isArray(v)&&v.length>0&&v.length<=64&&new Set(v.map(r=>r.id)).size===v.length&&v.every(r=>r&&exact(r,'id,name,permissions')&&id(r.id)&&text(r.name,200)&&r.name.trim()&&permissions(r.permissions));
function member(v:Member):boolean{return !!v&&exact(v,'id,userId,organizationId,projectId,displayName,loginName,roles,state,createdAt,changedAt,permissions')&&[v.id,v.userId,v.organizationId,v.projectId].every(id)&&text(v.displayName)&&text(v.loginName)&&Array.isArray(v.roles)&&v.roles.length<=32&&new Set(v.roles).size===v.roles.length&&v.roles.every(id)&&['active','inactive'].includes(v.state)&&time(v.createdAt)&&time(v.changedAt)&&permissions(v.permissions);}
export function managementSelection(section:ManagementSection,q:Record<string,string|string[]|undefined>):ManagementSelection{
 const allowed=section==='members'?['offset','q','role','state','memberId','from']:section==='users'?['offset','q','userId','from','memberContext']:section==='audit'?['cursor','from']:['roles','workspaces','organizations'].includes(section)?['from']:[];
 for(const [k,v]of Object.entries(q))if(!allowed.includes(k)||typeof v!=='string'||!v&&!['q','state','role'].includes(k))throw Error('INVALID_QUERY');
 const offset=Number(q.offset??0),s:ManagementSelection={offset};
 if(!Number.isInteger(offset)||offset<0||offset>10000||q.offset!==undefined&&String(offset)!==q.offset)throw Error('INVALID_QUERY');
 for(const k of ['q','role','state','memberId','cursor','userId']as const){const v=q[k];if(typeof v==='string'&&v)s[k]=v;}
 if(s.q&&(!text(s.q,200)||!s.q.trim()||/[\u0000-\u001f\u007f]/.test(s.q))||s.role&&!id(s.role)||s.memberId&&!id(s.memberId)||s.userId&&!id(s.userId)||s.state&&!['active','inactive'].includes(s.state)||s.cursor&&(!/^[A-Za-z0-9_-]+$/.test(s.cursor)||s.cursor.length>1024))throw Error('INVALID_QUERY');
 if(q.from!==undefined){if(q.from!=='workspace'&&q.from!=='organization'&&q.from!=='configuration'||(section==='workspaces'||section==='organizations')&&q.from!=='configuration')throw Error('INVALID_QUERY');s.from=q.from}
 if(q.memberContext!==undefined){
  if(!s.userId||s.offset!==0||s.q||s.from||typeof q.memberContext!=='string'||!text(q.memberContext,2000))throw Error('INVALID_QUERY');
  const values:Record<string,string>=Object.create(null);
  for(const [key,value] of new URLSearchParams(q.memberContext)){if(Object.hasOwn(values,key))throw Error('INVALID_QUERY');values[key]=value}
  const member=managementSelection('members',values);if(!member.memberId)throw Error('INVALID_QUERY');
  s.memberContext={...member,memberId:member.memberId};
 }
 return s;
}
export function managementQuery(s:ManagementSelection){const q=new URLSearchParams();if(s.offset)q.set('offset',String(s.offset));for(const k of ['q','role','state','memberId','cursor','userId']as const)if(s[k])q.set(k,s[k]!);return q.size?'?'+q:'';}
// Navigation context never enters an API query or selects a caller/workspace.
export function managementPageQuery(s:ManagementSelection){const q=new URLSearchParams(managementQuery(s));if(s.from==='workspace'||s.from==='organization'||s.from==='configuration')q.set('from',s.from);if(s.userId&&s.memberContext)q.set('memberContext',managementPageQuery(s.memberContext).slice(1));return q.size?'?'+q:'';}
export function workspaceQueryHref(kind:'members'|'users'|'roles'|'audit',from?:ManagementSelection['from']){if(!['members','users','roles','audit'].includes(kind))throw Error('INVALID_QUERY');return '/settings/'+kind+managementPageQuery({offset:0,from:from??'workspace'});}
export function organizationQueryHref(kind:'members'|'users',from?:ManagementSelection['from']){if(!['members','users'].includes(kind))throw Error('INVALID_QUERY');return '/settings/'+kind+managementPageQuery({offset:0,from:from??'organization'});}
export function configurationQueryHref(kind:'workspaces'|'organizations'|'members'|'users'|'roles'|'audit'){if(!['workspaces','organizations','members','users','roles','audit'].includes(kind))throw Error('INVALID_QUERY');return '/settings/'+kind+managementPageQuery({offset:0,from:'configuration'});}
function memberListSelection(s:ManagementSelection):ManagementSelection{return {offset:s.offset,q:s.q,role:s.role,state:s.state,from:s.from};}
export function managementMemberListHref(s:ManagementSelection,focusMemberId?:string){
 if(focusMemberId&&!id(focusMemberId))throw Error('INVALID_QUERY');
 return '/settings/members'+managementPageQuery(memberListSelection(s))+(focusMemberId?'#member-'+encodeURIComponent(focusMemberId):'');
}
export function managementMemberDetailHref(s:ManagementSelection,memberId:string){
 if(!id(memberId))throw Error('INVALID_QUERY');
 return '/settings/members'+managementPageQuery({...memberListSelection(s),memberId});
}
export function validManagement(raw:ManagementResult,p:Principal,kind:ManagementSection,s:ManagementSelection):boolean{
 try{
  if(kind==='users'||kind==='workspaces'||kind==='organizations'||kind==='configuration')return false;
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
export function validConfigurationManagement(value:unknown,p:Principal):value is ConfigurationManagementResult{
 try{
  const r=value as ConfigurationManagementResult;
  if(!r||!exact(r,'profileVersion,workspaceId,principalId,authorityRevision,evaluatedAt,source,writeEnabled,data')||r.profileVersion!=='platform-management.v1'||r.workspaceId!==p.workspace_id||r.principalId!==p.principal_id||r.authorityRevision!==p.revision||!time(r.evaluatedAt)||!['SYNTHETIC_FIXTURE','CONFIGURED'].includes(r.source)||r.writeEnabled!==false)return false;
  const d=r.data;if(!d||!exact(d,'scope,mode,directory,queries,rolePolicySource')||d.scope!=='CURRENT_WORKSPACE_MANAGEMENT'||d.mode!=='QUERY_ONLY')return false;
  const directory=d.directory,q=d.queries;if(!directory||!exact(directory,'state,source,availability')||directory.availability!=='NOT_CHECKED'||!q||!exact(q,'workspace,organization,members,users,roles,audit')||q.workspace!==true||!Object.values(q).every(v=>typeof v==='boolean'))return false;
  if(directory.state==='UNCONFIGURED')return directory.source==='UNCONFIGURED'&&d.rolePolicySource===null&&Object.entries(q).every(([k,v])=>k==='workspace'||k==='audit'||v===false);
  return directory.state==='CONFIGURED'&&['CONFIGURED','SYNTHETIC_FIXTURE'].includes(directory.source)&&q.members===true&&q.roles===true&&d.rolePolicySource==='APPLICATION_CATALOG';
 }catch{return false;}
}
export function validOrganizationManagement(value:unknown,p:Principal):value is OrganizationManagementResult{
 try{
  const r=value as OrganizationManagementResult;
  if(!r||!exact(r,'profileVersion,workspaceId,principalId,authorityRevision,evaluatedAt,source,writeEnabled,data')||r.profileVersion!=='platform-management.v1'||r.workspaceId!==p.workspace_id||r.principalId!==p.principal_id||r.authorityRevision!==p.revision||!time(r.evaluatedAt)||!['SYNTHETIC_FIXTURE','CONFIGURED'].includes(r.source)||r.writeEnabled!==false)return false;
  const d=r.data;if(!d||!exact(d,'organization,departmentHierarchyState')||d.departmentHierarchyState!=='UNCONFIGURED')return false;
  const o=d.organization;return !!o&&exact(o,'id,name,state,createdAt,changedAt')&&id(o.id)&&(o.name===null||text(o.name)&&!!o.name.trim()&&!/[\u0000-\u001f\u007f]/.test(o.name))&&['active','inactive','removed'].includes(o.state)&&(o.createdAt===null||time(o.createdAt))&&(o.changedAt===null||time(o.changedAt));
 }catch{return false;}
}
export function managementUserListHref(s:ManagementSelection,focusUserId?:string){
 if(focusUserId&&!id(focusUserId))throw Error('INVALID_QUERY');
 return '/settings/users'+managementPageQuery({offset:s.offset,q:s.q,from:s.from})+(focusUserId?'#user-'+encodeURIComponent(focusUserId):'');
}
export function managementUserDetailHref(s:ManagementSelection,userId:string){
 if(!id(userId))throw Error('INVALID_QUERY');return '/settings/users'+managementPageQuery({offset:s.offset,q:s.q,userId,from:s.from});
}
export function managementUserFromMemberHref(s:ManagementSelection,memberId:string,userId:string){
 if(!id(memberId)||!id(userId))throw Error('INVALID_QUERY');
 return '/settings/users'+managementPageQuery({offset:0,userId,memberContext:{...memberListSelection(s),memberId}});
}
export function managementUserReturnHref(s:ManagementSelection,focusUserId?:string){
 return s.memberContext?managementMemberListHref(s.memberContext,s.memberContext.memberId):managementUserListHref(s,focusUserId);
}
export function validWorkspaceManagement(value:unknown,p:Principal):value is WorkspaceManagementResult{
 try{
  const r=value as WorkspaceManagementResult;
  if(!r||!exact(r,'profileVersion,workspaceId,principalId,authorityRevision,evaluatedAt,source,writeEnabled,data')||r.profileVersion!=='platform-management.v1'||r.workspaceId!==p.workspace_id||r.principalId!==p.principal_id||r.authorityRevision!==p.revision||!time(r.evaluatedAt)||!['SYNTHETIC_FIXTURE','CONFIGURED'].includes(r.source)||r.writeEnabled!==false)return false;
  const d=r.data;if(!d||!exact(d,'recordState,confirmationExpiresAt,directory,queries')||d.recordState!=='UNCONFIGURED'||!time(d.confirmationExpiresAt)||Date.parse(d.confirmationExpiresAt)<=Math.max(Date.now(),Date.parse(r.evaluatedAt)))return false;
  const directory=d.directory,q=d.queries;
  if(!directory||!exact(directory,'state,source,organizationId,projectId,availability')||directory.availability!=='NOT_CHECKED'||!q||!exact(q,'members,users,roles,audit')||!Object.values(q).every(v=>typeof v==='boolean'))return false;
  if(directory.state==='UNCONFIGURED')return directory.source==='UNCONFIGURED'&&directory.organizationId===null&&directory.projectId===null&&Object.entries(q).every(([k,v])=>k==='audit'||v===false);
  return directory.state==='CONFIGURED'&&['CONFIGURED','SYNTHETIC_FIXTURE'].includes(directory.source)&&id(directory.organizationId)&&id(directory.projectId)&&q.members===true&&q.roles===true;
 }catch{return false;}
}
export function validUserManagement(value:unknown,p:Principal,s:ManagementSelection):value is UserManagementResult{
 try{
  const r=value as UserManagementResult;
  if(!r||!exact(r,'profileVersion,workspaceId,principalId,authorityRevision,evaluatedAt,source,writeEnabled,data')||r.profileVersion!=='platform-management.v1'||r.workspaceId!==p.workspace_id||r.principalId!==p.principal_id||r.authorityRevision!==p.revision||!time(r.evaluatedAt)||!['SYNTHETIC_FIXTURE','CONFIGURED'].includes(r.source)||r.writeEnabled!==false)return false;
  const d=r.data;if(!d||!exact(d,'items,total,offset,limit,selected,roleDefinitions')||!roles(d.roleDefinitions)||d.offset!==s.offset||d.limit!==25||!Number.isInteger(d.total)||d.total<0||d.total>10000||!Array.isArray(d.items)||d.items.length!==Math.min(25,Math.max(0,d.total-s.offset))||new Set(d.items.map(u=>u.id)).size!==d.items.length||!d.items.every(u=>u&&exact(u,'id,displayName,loginName,membershipCount')&&id(u.id)&&text(u.displayName)&&text(u.loginName)&&Number.isInteger(u.membershipCount)&&u.membershipCount>0&&u.membershipCount<=10000))return false;
  if(!s.userId)return d.selected===null;
  const selected=d.selected;if(!selected||!exact(selected,'profile,memberships,identity'))return false;
  const profile=selected.profile;if(!profile||!exact(profile,'id,organizationId,username,displayName,accountState,createdAt,changedAt')||profile.id!==s.userId||!id(profile.id)||!id(profile.organizationId)||!text(profile.username)||!profile.username.trim()||!text(profile.displayName)||!['active','inactive','locked','initial','deleted'].includes(profile.accountState)||!time(profile.createdAt)||!time(profile.changedAt))return false;
  const ms=selected.memberships;if(!Array.isArray(ms)||!ms.length||ms.length>10000||new Set(ms.map(m=>m.id)).size!==ms.length||!ms.every(m=>member(m)&&m.userId===profile.id&&m.organizationId===profile.organizationId&&m.projectId===ms[0].projectId))return false;
  if(s.memberContext&&!ms.some(m=>m.id===s.memberContext!.memberId))return false;
  const link=selected.identity;if(!link||!exact(link,'workspaceId,state,actorId,revision,basisRef,expiresAt')||link.workspaceId!==p.workspace_id)return false;
  if(link.state==='UNCONFIGURED'||link.state==='UNLINKED')return link.actorId===null&&link.revision===''&&link.basisRef===''&&link.expiresAt===null;
  return link.state==='LINKED'&&uuid(link.actorId)&&typeof link.revision==='string'&&/^[1-9][0-9]*:[1-9][0-9]*:[1-9][0-9]*$/.test(link.revision)&&text(link.basisRef,200)&&!!link.basisRef.trim()&&time(link.expiresAt)&&Date.parse(link.expiresAt!)>Date.now();
 }catch{return false;}
}
