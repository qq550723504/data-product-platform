import {isIdentity} from './quality-attention';
import {validPage} from './workbench-read';
export type Criterion={id:string;clause:string;text:string;required:boolean;method:'MANUAL_CHECK'|'EVIDENCE_ASSERTION';evidenceCategory:string};
export type Standard={id:string;workspaceId:string;contractId:string;name:string;version:string;status:string;sourceSha256:string;createdAt:string;publishedAt:string|null;acceptanceSource:{status:string;schemaVersion:string;setHash:string;criteria:Criterion[]}};
export type StandardPage={workspaceId:string;items:Standard[];page:{limit:number;offset:number;total:number}};
export const responsibilityRoles=['PROVIDER','RECEIVER','BASIS_CONFIRMER','REVIEWER','ARCHIVIST','DISPOSER'];
export const sourceStates=['SUPPORTED','UNSUPPORTED_SOURCE','INVALID_SOURCE','UNSUPPORTED_CRITERION'];
export const responsibilityStates=['UNASSIGNED','INACTIVE','AMBIGUOUS','ASSIGNED','CONFLICT'];
export const applicabilityStates=['UNSELECTED','MISSING','INACTIVE','AMBIGUOUS','MISMATCH','MATCHED'];
export const blockerCodes=['RECEIVING_PARTY_UNVERIFIED','ACCEPTANCE_WRITES_DISABLED','DELIVERY_NOT_ISSUED','EXPLICIT_STANDARD_REQUIRED','CONTRACT_NOT_PUBLISHED',...sourceStates.filter(s=>s!=='SUPPORTED'),...responsibilityRoles.flatMap(r=>responsibilityStates.filter(s=>s!=='ASSIGNED').map(s=>`ROLE_${r}_${s}`)),...applicabilityStates.filter(s=>s!=='MATCHED').map(s=>`APPLICABILITY_${s}`)];
export type BindingFact={id:string;workspaceId:string;kind:'REPRESENTATION'|'RESPONSIBILITY'|'APPLICABILITY'|'DISPOSITION';basisEvidenceId:string;recordedBy:string;createdAt:string;origin?:'CORE_CONFIRMED'|'SYNTHETIC_FIXTURE';validFrom:string;validTo:string|null;actorId?:string;subject?:string;partyRef?:string;deliveryOperationId?:string;datasetVersionId?:string;role?:string;representationId?:string;contractVersionId?:string;sourceSha256?:string;criteriaSetHash?:string;confirmerBindingId?:string;targetId?:string;targetKind?:string;disposition?:string;effectiveAt?:string;reason?:string;state:string};
export type Responsibility={role:string;status:string;reason:string;candidateIds:string[];binding:BindingFact|null;representative:BindingFact|null;isCurrentCaller:boolean};
export type Applicability={status:string;reason:string;candidateIds:string[];binding:BindingFact|null};
export type AcceptanceContext={workspaceId:string;evaluatedAt:string;delivery:{id:string;datasetVersionId:string;status:string};standards:StandardPage;responsibilities:Responsibility[];applicability:Applicability;bindingHistory:{items:BindingFact[];page:{limit:number;offset:number;total:number};selected:BindingFact|null;related:BindingFact[]};receivingPartyStatus:'UNVERIFIED'|'VERIFIED';blockers:string[];writeEnabled:false};
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
const ids=(a:string[])=>Array.isArray(a)&&a.every(isIdentity)&&new Set(a.map(s=>s.toLowerCase())).size===a.length;
export function validBindingFact(f:BindingFact,ws:string,delivery:string,version:string):boolean{
 if(!f||!same(f.workspaceId,ws)||!isIdentity(f.id)||!isIdentity(f.basisEvidenceId)||!isIdentity(f.recordedBy)||!time(f.createdAt)||!['ACTIVE','NOT_YET_VALID','EXPIRED','DISPOSED','INACTIVE','AMBIGUOUS','CONFLICT','MISMATCH','EFFECTIVE','SCHEDULED'].includes(f.state))return false;
 if(f.kind==='DISPOSITION')return isIdentity(f.targetId??'')&&['REPRESENTATION','RESPONSIBILITY','APPLICABILITY'].includes(f.targetKind??'')&&['INVALIDATED','REVOKED','SUPERSEDED'].includes(f.disposition??'')&&time(f.effectiveAt)&&text(f.reason,1000)&&['EFFECTIVE','SCHEDULED'].includes(f.state);
 if(!['CORE_CONFIRMED','SYNTHETIC_FIXTURE'].includes(f.origin??'')||!time(f.validFrom)||!(f.validTo===null||time(f.validTo)&&Date.parse(f.validTo)>Date.parse(f.validFrom)))return false;
 if(f.kind==='REPRESENTATION')return isIdentity(f.actorId??'')&&text(f.subject,255)&&text(f.partyRef,255);
 if(!same(f.deliveryOperationId??'',delivery)||!same(f.datasetVersionId??'',version))return false;
 if(f.kind==='RESPONSIBILITY')return responsibilityRoles.includes(f.role??'')&&isIdentity(f.representationId??'');
 return f.kind==='APPLICABILITY'&&isIdentity(f.contractVersionId??'')&&hash(f.sourceSha256)&&hash(f.criteriaSetHash)&&isIdentity(f.confirmerBindingId??'');
}
export function validAcceptanceContext(c:AcceptanceContext,ws:string,delivery:string,version:string,standard?:string,bindingOffset=0,bindingId?:string):boolean{
 try{
 const fact=(f:BindingFact)=>validBindingFact(f,ws,delivery,version);
 if(!same(c.workspaceId,ws)||!time(c.evaluatedAt)||!same(c.delivery.id,delivery)||!same(c.delivery.datasetVersionId,version)||!['PREPARED','ISSUANCE_PENDING','CONTAINMENT_PENDING','ISSUED','BLOCKED','FAILED'].includes(c.delivery.status)||!validStandards(c.standards,ws,0,standard)||standard&&c.standards.items.length!==1||c.writeEnabled!==false||!Array.isArray(c.responsibilities)||c.responsibilities.length!==6||!responsibilityRoles.every(role=>c.responsibilities.filter(r=>r.role===role).length===1))return false;
 for(const r of c.responsibilities){
  if(!responsibilityStates.includes(r.status)||!text(r.reason)||!ids(r.candidateIds)||typeof r.isCurrentCaller!=='boolean')return false;
  if(r.binding!==null&&(!fact(r.binding)||r.binding.kind!=='RESPONSIBILITY'||r.binding.role!==r.role||!r.candidateIds.includes(r.binding.id)))return false;
  if(r.representative!==null&&(!fact(r.representative)||r.representative.kind!=='REPRESENTATION'||!same(r.binding?.representationId??'',r.representative.id)))return false;
  if(['ASSIGNED','CONFLICT'].includes(r.status)&&(!r.binding||!r.representative||r.candidateIds.length!==1||r.representative.state!=='ACTIVE'))return false;
  if(r.status==='ASSIGNED'&&r.binding?.state!=='ACTIVE')return false;
  if(r.status==='AMBIGUOUS'&&(r.candidateIds.length<2||r.binding!==null||r.representative!==null)||r.status==='UNASSIGNED'&&(r.candidateIds.length!==0||r.binding!==null||r.representative!==null)||r.isCurrentCaller&&!['ASSIGNED','CONFLICT'].includes(r.status))return false;
 }
 const receiver=c.responsibilities.find(r=>r.role==='RECEIVER')!;const confirmer=c.responsibilities.find(r=>r.role==='BASIS_CONFIRMER')!;
 const provider=c.responsibilities.find(r=>r.role==='PROVIDER')!;if(provider.status==='ASSIGNED')for(const r of c.responsibilities.filter(r=>['REVIEWER','BASIS_CONFIRMER'].includes(r.role)&&r.status==='ASSIGNED'))if(provider.representative?.actorId===r.representative?.actorId||provider.representative?.subject===r.representative?.subject)return false;
 if(c.receivingPartyStatus!==(receiver.status==='ASSIGNED'?'VERIFIED':'UNVERIFIED'))return false;
 const a=c.applicability;
 if(!applicabilityStates.includes(a.status)||!text(a.reason)||!ids(a.candidateIds)||Boolean(standard)===(a.status==='UNSELECTED'))return false;
 if(a.binding!==null&&(!fact(a.binding)||a.binding.kind!=='APPLICABILITY'||!same(a.binding.contractVersionId??'',standard??'')||!a.candidateIds.includes(a.binding.id)))return false;
 if(a.status==='MATCHED'&&(!a.binding||a.candidateIds.length!==1||confirmer.status!=='ASSIGNED'||!same(a.binding.confirmerBindingId??'',confirmer.binding?.id??'')||c.standards.items[0]?.status!=='PUBLISHED'||c.standards.items[0]?.acceptanceSource.status!=='SUPPORTED'||a.binding.sourceSha256!==c.standards.items[0]?.sourceSha256||a.binding.criteriaSetHash!==c.standards.items[0]?.acceptanceSource.setHash))return false;
 if(a.status==='AMBIGUOUS'&&(a.candidateIds.length<2||a.binding!==null)||['MISSING','INACTIVE','UNSELECTED'].includes(a.status)&&(a.candidateIds.length!==0||a.binding!==null)||a.status==='MISMATCH'&&(!a.binding||a.candidateIds.length!==1))return false;
 const h=c.bindingHistory;
 if(!validPage(h,25,bindingOffset,fact)||new Set(h.items.map(f=>`${f.kind}:${f.id}`)).size!==h.items.length||(bindingId?(!h.selected||!same(h.selected.id,bindingId)||!fact(h.selected)):h.selected!==null))return false;
 if(!Array.isArray(h.related)||!h.related.every(fact)||!h.selected&&h.related.length!==0||new Set(h.related.map(f=>`${f.kind}:${f.id}`)).size!==h.related.length)return false;
 const closure=h.selected?[h.selected,...h.related]:[];for(const f of closure){if(f.kind==='RESPONSIBILITY'&&!closure.some(p=>p.kind==='REPRESENTATION'&&same(p.id,f.representationId??'')))return false;if(f.kind==='APPLICABILITY'&&!closure.some(r=>r.kind==='RESPONSIBILITY'&&same(r.id,f.confirmerBindingId??'')))return false;if(f.kind==='DISPOSITION'&&!closure.some(t=>t.kind===f.targetKind&&same(t.id,f.targetId??'')))return false;}
 const required=['ACCEPTANCE_WRITES_DISABLED',...(c.delivery.status!=='ISSUED'?['DELIVERY_NOT_ISSUED']:[]),...(!standard?['EXPLICIT_STANDARD_REQUIRED']:[]),...(c.receivingPartyStatus!=='VERIFIED'?['RECEIVING_PARTY_UNVERIFIED']:[]),...c.responsibilities.filter(r=>r.status!=='ASSIGNED').map(r=>`ROLE_${r.role}_${r.status}`),...(a.status!=='MATCHED'?[`APPLICABILITY_${a.status}`]:[])];
 return Array.isArray(c.blockers)&&new Set(c.blockers).size===c.blockers.length&&required.every(code=>c.blockers.includes(code))&&c.blockers.every(code=>blockerCodes.includes(code))&&(!standard||c.standards.items.every(v=>(v.status==='PUBLISHED'||c.blockers.includes('CONTRACT_NOT_PUBLISHED'))&&(v.acceptanceSource.status==='SUPPORTED'||c.blockers.includes(v.acceptanceSource.status))));
 }catch{return false}
}
export type AcceptanceSelection={basisMaterialOffset?:number;basisMaterialId?:string;offset:number;bindingOffset:number;bindingId?:string;contractVersionId?:string;deliveryOperationId?:string;datasetVersionId?:string};
export function acceptanceSelection(q:Record<string,string|string[]|undefined>):AcceptanceSelection{
 for(const [key,value] of Object.entries(q))if(!['offset','bindingOffset','bindingId','contractVersionId','deliveryOperationId','datasetVersionId','basisMaterialOffset','basisMaterialId'].includes(key)||typeof value!=='string')throw Error('INVALID_QUERY');
 const offset=q.offset===undefined?0:Number(q.offset);if(q.offset!==undefined&&!/^\d+$/.test(q.offset as string)||!Number.isSafeInteger(offset)||offset<0||offset>2147483647)throw Error('INVALID_PAGE');
 const bindingOffset=q.bindingOffset===undefined?0:Number(q.bindingOffset);if(q.bindingOffset!==undefined&&!/^\d+$/.test(q.bindingOffset as string)||!Number.isSafeInteger(bindingOffset)||bindingOffset<0||bindingOffset>2147483647)throw Error('INVALID_PAGE');
 const result:AcceptanceSelection={offset,bindingOffset};for(const key of ['contractVersionId','deliveryOperationId','datasetVersionId','bindingId','basisMaterialId'] as const){const value=q[key] as string|undefined;if(value!==undefined){if(!isIdentity(value)||/^0{8}-0{4}-0{4}-0{4}-0{12}$/.test(value))throw Error('INVALID_REFERENCE');result[key]=value.toLowerCase();}}
 if(q.basisMaterialOffset!==undefined){const raw=q.basisMaterialOffset as string;if(!/^\d+$/.test(raw)||!Number.isSafeInteger(Number(raw))||Number(raw)>2147483647)throw Error('INVALID_RETURN_PAGE');result.basisMaterialOffset=Number(raw);}
 if(!result.deliveryOperationId&&(result.basisMaterialId||result.basisMaterialOffset!==undefined))throw Error('INCOMPLETE_RETURN_TARGET');
 if(Boolean(result.deliveryOperationId)!==Boolean(result.datasetVersionId)||!result.deliveryOperationId&&(result.bindingId||result.bindingOffset))throw Error('INCOMPLETE_TARGET');return result;
}
export function acceptanceHref(s:AcceptanceSelection):string{const q=new URLSearchParams();for(const key of ['contractVersionId','deliveryOperationId','datasetVersionId','bindingId','basisMaterialId'] as const)if(s[key])q.set(key,s[key]!);if(s.basisMaterialOffset!==undefined)q.set('basisMaterialOffset',String(s.basisMaterialOffset));if(s.offset)q.set('offset',String(s.offset));if(s.bindingOffset)q.set('bindingOffset',String(s.bindingOffset));return '/core/acceptance-context'+(q.size?'?'+q:'');}
