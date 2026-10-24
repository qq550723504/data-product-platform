import {isIdentity} from './quality-attention';
import {validPage} from './workbench-read';
export const DELIVERY_STATUSES=['PREPARED','ISSUANCE_PENDING','CONTAINMENT_PENDING','ISSUED','BLOCKED','FAILED'];
export type HistoricalRow={id:string;workspaceId:string;createdAt:string;datasetVersionId?:string;certificationId?:string;retryOfId?:string;status?:string;gateDecision?:string;purpose?:string;action?:string;channel?:string;mode?:string;evaluations?:Array<{id:string;stage:string;decision:string;createdAt:string}>;transitions?:Array<{id:string;from:string;to:string;evaluationId?:string;createdAt:string}>;objectType?:string;objectId?:string;rootHash?:string;itemCount?:number;releaseIds?:string[]};
export type HistoricalPage={workspaceId:string;items:HistoricalRow[];page:{limit:number;offset:number;total:number}};
const time=(x:unknown)=>typeof x==='string'&&Number.isFinite(Date.parse(x));
const text=(x:unknown)=>typeof x==='string';
export function validHistoricalPage(p:HistoricalPage,ws:string,archive:boolean,id:string|undefined,offset:number){
 return p.workspaceId===ws&&validPage(p,25,offset,r=>isIdentity(r.id)&&r.workspaceId===ws&&(!id||r.id===id)&&time(r.createdAt)&&(archive?
 text(r.objectType)&&isIdentity(r.objectId!)&&typeof r.rootHash==='string'&&/^[a-f0-9]{64}$/.test(r.rootHash)&&Number.isSafeInteger(r.itemCount)&&r.itemCount!>=0&&Array.isArray(r.releaseIds)&&r.releaseIds.every(isIdentity):
 isIdentity(r.datasetVersionId!)&&(!r.certificationId||isIdentity(r.certificationId))&&(!r.retryOfId||isIdentity(r.retryOfId))&&DELIVERY_STATUSES.includes(r.status!)&&['ALLOWED','BLOCKED'].includes(r.gateDecision!)&&[r.purpose,r.action,r.channel,r.mode].every(text)&&Array.isArray(r.evaluations)&&r.evaluations.every(e=>isIdentity(e.id)&&text(e.stage)&&['ALLOWED','BLOCKED'].includes(e.decision)&&time(e.createdAt))&&Array.isArray(r.transitions)&&r.transitions.every(t=>isIdentity(t.id)&&DELIVERY_STATUSES.includes(t.from)&&DELIVERY_STATUSES.includes(t.to)&&(!t.evaluationId||isIdentity(t.evaluationId))&&time(t.createdAt))));
}
export function deliverySelection(q:Record<string,string|string[]|undefined>){
 for(const [k,v] of Object.entries(q))if(!['id','offset'].includes(k)||typeof v!=='string')throw Error('INVALID_QUERY');
 const id=q.id as string|undefined;if(id!==undefined&&(!isIdentity(id)||/^0{8}-0{4}-0{4}-0{4}-0{12}$/.test(id)))throw Error('INVALID_REFERENCE');
 const offset=q.offset===undefined?0:Number(q.offset);if(q.offset!==undefined&&!/^\d+$/.test(q.offset as string)||!Number.isSafeInteger(offset)||offset<0||offset>2147483647)throw Error('INVALID_PAGE');
 return {id:id?.toLowerCase(),offset};
}
