import {isIdentity} from './quality-attention';
import {validPage} from './workbench-read';
export type Criterion={id:string;clause:string;text:string;required:boolean;method:'MANUAL_CHECK'|'EVIDENCE_ASSERTION';evidenceCategory:string};
export type Standard={id:string;workspaceId:string;contractId:string;name:string;version:string;status:string;sourceSha256:string;createdAt:string;publishedAt:string|null;acceptanceSource:{status:string;schemaVersion:string;setHash:string;criteria:Criterion[]}};
export type StandardPage={workspaceId:string;items:Standard[];page:{limit:number;offset:number;total:number}};
export const responsibilityRoles=['PROVIDER','RECEIVER','BASIS_CONFIRMER','REVIEWER','ARCHIVIST','DISPOSER'];
export const sourceStates=['SUPPORTED','UNSUPPORTED_SOURCE','INVALID_SOURCE','UNSUPPORTED_CRITERION'];
export const blockerCodes=['RESPONSIBILITIES_NOT_CONFIGURED','RECEIVING_PARTY_UNVERIFIED','ACCEPTANCE_WRITES_DISABLED','DELIVERY_NOT_ISSUED','EXPLICIT_STANDARD_REQUIRED','CONTRACT_NOT_PUBLISHED',...sourceStates.filter(s=>s!=='SUPPORTED')];
export type AcceptanceContext={workspaceId:string;delivery:{id:string;datasetVersionId:string;status:string};standards:StandardPage;responsibilities:Array<{role:string;status:'UNASSIGNED'}>;receivingPartyStatus:'UNVERIFIED';blockers:string[];writeEnabled:false};
const same=(a:string,b:string)=>isIdentity(a)&&isIdentity(b)&&a.toLowerCase()===b.toLowerCase();
const text=(v:unknown,max=2000)=>typeof v==='string'&&v.trim().length>0&&v.length<=max;
const hash=(v:unknown)=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v);
const time=(v:unknown)=>typeof v==='string'&&Number.isFinite(Date.parse(v));
export function validStandard(v:Standard,ws:string,id?:string):boolean{
 const s=v.acceptanceSource;
 if(!same(v.workspaceId,ws)||!isIdentity(v.id)||id!==undefined&&!same(v.id,id)||!isIdentity(v.contractId)||!text(v.name)||!/^\d+\.\d+\.\d+$/.test(v.version)||!['DRAFT','PUBLISHED','DEPRECATED'].includes(v.status)||!hash(v.sourceSha256)||!time(v.createdAt)||!(v.publishedAt===null||time(v.publishedAt))||v.status==='PUBLISHED'&&!time(v.publishedAt)||!sourceStates.includes(s.status)||!Array.isArray(s.criteria))return false;
 if(s.status!=='SUPPORTED')return s.schemaVersion===''&&s.setHash===''&&s.criteria.length===0;
 const ids=s.criteria.map(c=>c.id);
 return s.schemaVersion==='1'&&hash(s.setHash)&&ids.length>0&&ids.length<=100&&new Set(ids).size===ids.length&&s.criteria.some(c=>c.required===true)&&s.criteria.every(c=>text(c.id,64)&&text(c.clause,256)&&text(c.text)&&typeof c.required==='boolean'&&['MANUAL_CHECK','EVIDENCE_ASSERTION'].includes(c.method)&&text(c.evidenceCategory,64));
}
export function validStandards(p:StandardPage,ws:string,offset:number,id?:string):boolean{return same(p.workspaceId,ws)&&validPage(p,25,offset,v=>validStandard(v,ws,id));}
export function validAcceptanceContext(c:AcceptanceContext,ws:string,delivery:string,version:string,standard?:string):boolean{
 return same(c.workspaceId,ws)&&same(c.delivery.id,delivery)&&same(c.delivery.datasetVersionId,version)&&['PREPARED','ISSUANCE_PENDING','CONTAINMENT_PENDING','ISSUED','BLOCKED','FAILED'].includes(c.delivery.status)&&validStandards(c.standards,ws,0,standard)&&(!standard||c.standards.items.length===1)&&c.writeEnabled===false&&c.receivingPartyStatus==='UNVERIFIED'&&Array.isArray(c.responsibilities)&&c.responsibilities.length===responsibilityRoles.length&&responsibilityRoles.every(role=>c.responsibilities.filter(r=>r.role===role&&r.status==='UNASSIGNED').length===1)&&Array.isArray(c.blockers)&&new Set(c.blockers).size===c.blockers.length&&['RESPONSIBILITIES_NOT_CONFIGURED','RECEIVING_PARTY_UNVERIFIED','ACCEPTANCE_WRITES_DISABLED',...(c.delivery.status!=='ISSUED'?['DELIVERY_NOT_ISSUED']:[]),...(!standard?['EXPLICIT_STANDARD_REQUIRED']:[])].every(code=>c.blockers.includes(code))&&c.blockers.every(code=>blockerCodes.includes(code))&&(!standard||c.standards.items.every(v=>(v.status==='PUBLISHED'||c.blockers.includes('CONTRACT_NOT_PUBLISHED'))&&(v.acceptanceSource.status==='SUPPORTED'||c.blockers.includes(v.acceptanceSource.status))));
}
export type AcceptanceSelection={offset:number;contractVersionId?:string;deliveryOperationId?:string;datasetVersionId?:string};
export function acceptanceSelection(q:Record<string,string|string[]|undefined>):AcceptanceSelection{
 for(const [key,value] of Object.entries(q))if(!['offset','contractVersionId','deliveryOperationId','datasetVersionId'].includes(key)||typeof value!=='string')throw Error('INVALID_QUERY');
 const offset=q.offset===undefined?0:Number(q.offset);if(q.offset!==undefined&&!/^\d+$/.test(q.offset as string)||!Number.isSafeInteger(offset)||offset<0||offset>2147483647)throw Error('INVALID_PAGE');
 const result:AcceptanceSelection={offset};for(const key of ['contractVersionId','deliveryOperationId','datasetVersionId'] as const){const value=q[key] as string|undefined;if(value!==undefined){if(!isIdentity(value)||/^0{8}-0{4}-0{4}-0{4}-0{12}$/.test(value))throw Error('INVALID_REFERENCE');result[key]=value.toLowerCase();}}
 if(Boolean(result.deliveryOperationId)!==Boolean(result.datasetVersionId))throw Error('INCOMPLETE_TARGET');return result;
}
export function acceptanceHref(s:AcceptanceSelection):string{const q=new URLSearchParams();for(const key of ['contractVersionId','deliveryOperationId','datasetVersionId'] as const)if(s[key])q.set(key,s[key]!);if(s.offset)q.set('offset',String(s.offset));return '/core/acceptance-context'+(q.size?'?'+q:'');}
