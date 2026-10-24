import {isIdentity} from './quality-attention';
export type QualityCost={id:string;workspaceId:string;activityId?:string;executionId?:string;costType:string;quantity:number;unit:string;amount?:string;currency?:string;pricingMode:string;occurredAt:string};
const same=(a:unknown,b:string)=>typeof a==='string'&&isIdentity(a)&&a.toLowerCase()===b.toLowerCase();
export function qualityCosts(value:unknown,workspace:string,version:string,assessment:string):QualityCost[]|null{
 if(!value||typeof value!=='object')return null;
 const d=value as Record<string,unknown>;
 if(!same(d.workspaceId,workspace)||!same(d.datasetVersionId,version)||!same(d.id,assessment)||!Array.isArray(d.costEvents))return null;
 const seen=new Set<string>();const result:QualityCost[]=[];
 for(const raw of d.costEvents){
  if(!raw||typeof raw!=='object')return null;const c=raw as Record<string,unknown>;
  if(typeof c.id!=='string'||!isIdentity(c.id)||seen.has(c.id.toLowerCase())||!same(c.workspaceId,workspace)||typeof c.costType!=='string'||typeof c.quantity!=='number'||!Number.isFinite(c.quantity)||c.quantity<0||typeof c.unit!=='string'||typeof c.pricingMode!=='string'||typeof c.occurredAt!=='string'||!Number.isFinite(Date.parse(c.occurredAt)))return null;
  for(const key of ['executionId','activityId'])if(c[key]!==undefined&&(typeof c[key]!=='string'||!isIdentity(c[key] as string)))return null;
  if(c.amount!==undefined&&(typeof c.amount!=='string'||!/^\d+(?:\.\d+)?$/.test(c.amount)))return null;
  if(c.currency!==undefined&&typeof c.currency!=='string')return null;
  seen.add(c.id.toLowerCase());
  result.push({id:c.id,workspaceId:workspace,costType:c.costType,quantity:c.quantity,unit:c.unit,pricingMode:c.pricingMode,occurredAt:c.occurredAt,...(c.activityId?{activityId:c.activityId as string}:{}),...(c.executionId?{executionId:c.executionId as string}:{}),...(c.amount!==undefined?{amount:c.amount as string}:{}),...(c.currency?{currency:c.currency as string}:{})});
 }
 return result;
}
