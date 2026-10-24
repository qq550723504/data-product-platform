import {isIdentity} from './quality-attention';
import {validIdentityContext,type IdentityContext} from './acceptance-identity';
import {acceptanceSelection,acceptanceHref,type AcceptanceSelection,type BindingFact} from './acceptance-context';
import type {Principal} from './iam-core';
export type BasisSelection=AcceptanceSelection&{materialOffset:number;materialId?:string};
export type IntegrityReference={id:string;workspaceId:string;envelopeStatus:'VALID'|'INVALID'|'NOT_CHECKED'|'MISSING';envelopeSha256:string;recordedAt:string|null;recordedBy:string|null;claimKind:'RECEIPT_CLAIM'|'GOVERNANCE_RESULT'|'UNCLASSIFIED'};
export type Material={evidence:IntegrityReference;contentStatus:'UNATTESTED';recipientStatus:'UNATTESTED';businessUse:'BLOCKED';blockers:string[]};
export type BusinessBasis={profileVersion:'business-basis.v1';context:IdentityContext;identity:{principalId:string;workspaceId:string;subjectBindingId:string;workspaceGrantId:string;authorityRevision:string;authentication:'OIDC_CONFIRMED';directorySource:'UNCONFIGURED'};responsibilitySource:'UNCONFIGURED';standardSource:'CORE_RECORD_ONLY';receiptSource:'UNCONFIGURED';evidence:{factId:string;kind:string;origin:string;evidence:IntegrityReference}[];materials:{items:Material[];page:{limit:number;offset:number;total:number}};writeEnabled:false;blockers:string[]};
export type MaterialDetail={profileVersion:'business-basis.v1';workspaceId:string;deliveryOperationId:string;datasetVersionId:string;principalId:string;authorityRevision:string;evaluatedAt:string;material:Material;writeEnabled:false};
const keys=(v:object,names:string)=>!!v&&Object.keys(v).sort().join(',')===names.split(',').sort().join(',');
const id=(v:unknown):v is string=>typeof v==='string'&&isIdentity(v)&&v!=='00000000-0000-0000-0000-000000000000';
const stamp=(v:unknown):v is string=>typeof v==='string'&&Number.isFinite(Date.parse(v));
export const basisBlockers=['ACCEPTANCE_WRITES_DISABLED','DIRECTORY_SOURCE_UNCONFIGURED','RESPONSIBILITY_SOURCE_UNCONFIGURED','STANDARD_AUTHORITY_UNCONFIGURED','RECEIPT_SOURCE_UNCONFIGURED','RECEIPT_MATERIAL_UNAVAILABLE'];
const materialBlockers=['RECEIPT_SOURCE_UNCONFIGURED','CONTENT_NOT_ATTESTED','RECIPIENT_NOT_ATTESTED','NOT_A_RECEIPT_CLAIM','EVIDENCE_ENVELOPE_INVALID','EVIDENCE_ENVELOPE_NOT_CHECKED','EVIDENCE_ENVELOPE_MISSING'];
const uniqueCodes=(a:string[],allowed:string[])=>Array.isArray(a)&&new Set(a).size===a.length&&a.every(v=>allowed.includes(v));
export function validIntegrity(v:IntegrityReference,ws:string):boolean{
 try{return keys(v,'id,workspaceId,envelopeStatus,envelopeSha256,recordedAt,recordedBy,claimKind')&&id(v.id)&&v.workspaceId===ws&&['VALID','INVALID','NOT_CHECKED','MISSING'].includes(v.envelopeStatus)&&['RECEIPT_CLAIM','GOVERNANCE_RESULT','UNCLASSIFIED'].includes(v.claimKind)&&(v.recordedBy===null||id(v.recordedBy))&&(v.envelopeStatus==='MISSING'?v.recordedAt===null&&v.recordedBy===null&&v.claimKind==='UNCLASSIFIED':stamp(v.recordedAt))&&(v.envelopeStatus==='VALID'?/^[a-f0-9]{64}$/.test(v.envelopeSha256):v.envelopeSha256==='')}catch{return false;}
}
export function validMaterial(m:Material,ws:string):boolean{
 try{return keys(m,'evidence,contentStatus,recipientStatus,businessUse,blockers')&&validIntegrity(m.evidence,ws)&&m.contentStatus==='UNATTESTED'&&m.recipientStatus==='UNATTESTED'&&m.businessUse==='BLOCKED'&&uniqueCodes(m.blockers,materialBlockers)&&['RECEIPT_SOURCE_UNCONFIGURED','CONTENT_NOT_ATTESTED','RECIPIENT_NOT_ATTESTED'].every(b=>m.blockers.includes(b))&&(m.evidence.claimKind==='RECEIPT_CLAIM'||m.blockers.includes('NOT_A_RECEIPT_CLAIM'))&&(m.evidence.envelopeStatus==='VALID'||m.blockers.includes('EVIDENCE_ENVELOPE_'+m.evidence.envelopeStatus))}catch{return false;}
}
export function validBusinessBasis(b:BusinessBasis,p:Principal,s:BasisSelection):boolean{
 try{
  if(!keys(b,'profileVersion,context,identity,responsibilitySource,standardSource,receiptSource,evidence,materials,writeEnabled,blockers')||b.profileVersion!=='business-basis.v1'||b.writeEnabled!==false||b.responsibilitySource!=='UNCONFIGURED'||b.standardSource!=='CORE_RECORD_ONLY'||b.receiptSource!=='UNCONFIGURED'||!validIdentityContext(b.context,p,{...s,bindingOffset:0,bindingId:undefined,offset:0}))return false;
  const a=b.identity;if(!keys(a,'principalId,workspaceId,subjectBindingId,workspaceGrantId,authorityRevision,authentication,directorySource')||a.principalId!==p.principal_id||a.workspaceId!==p.workspace_id||a.authorityRevision!==p.revision||!id(a.subjectBindingId)||!id(a.workspaceGrantId)||a.authentication!=='OIDC_CONFIRMED'||a.directorySource!=='UNCONFIGURED'||!uniqueCodes(b.blockers,basisBlockers)||!basisBlockers.slice(0,5).every(v=>b.blockers.includes(v)))return false;
  const facts:BindingFact[]=b.context.responsibilities.flatMap(r=>[r.binding,r.representative].filter((f):f is BindingFact=>f!==null));if(b.context.applicability.binding)facts.push(b.context.applicability.binding);
  const expected=new Map(facts.map(f=>[f.kind+':'+f.id,f]));
  if(!Array.isArray(b.evidence)||b.evidence.length!==expected.size||new Set(b.evidence.map(f=>f.kind+':'+f.factId)).size!==expected.size||b.evidence.some(f=>{const original=expected.get(f.kind+':'+f.factId);return !keys(f,'factId,kind,origin,evidence')||!original||f.origin!==original.origin||f.evidence.id!==original.basisEvidenceId||!validIntegrity(f.evidence,p.workspace_id)}))return false;
  const page=b.materials;if(!keys(page,'items,page')||!keys(page.page,'limit,offset,total')||page.page.limit!==25||page.page.offset!==s.materialOffset||!Number.isSafeInteger(page.page.total)||page.page.total<0||!Array.isArray(page.items)||page.items.length!==Math.min(25,Math.max(0,page.page.total-s.materialOffset))||new Set(page.items.map(m=>m.evidence.id)).size!==page.items.length||!page.items.every(m=>validMaterial(m,p.workspace_id)))return false;
  return page.page.total!==0||b.blockers.includes('RECEIPT_MATERIAL_UNAVAILABLE');
 }catch{return false;}
}
export function validMaterialDetail(d:MaterialDetail,p:Principal,s:BasisSelection):boolean{
 try{return keys(d,'profileVersion,workspaceId,deliveryOperationId,datasetVersionId,principalId,authorityRevision,evaluatedAt,material,writeEnabled')&&d.profileVersion==='business-basis.v1'&&d.workspaceId===p.workspace_id&&d.principalId===p.principal_id&&d.authorityRevision===p.revision&&d.deliveryOperationId===s.deliveryOperationId&&d.datasetVersionId===s.datasetVersionId&&stamp(d.evaluatedAt)&&d.writeEnabled===false&&d.material.evidence.id===s.materialId&&validMaterial(d.material,p.workspace_id)}catch{return false;}
}
export function basisSelection(raw:Record<string,string|string[]|undefined>):BasisSelection{
 for(const key of Object.keys(raw))if(!['materialOffset','materialId','deliveryOperationId','datasetVersionId','contractVersionId'].includes(key))throw Error('INVALID_QUERY');
 const {materialOffset,materialId,...rest}=raw;const s=acceptanceSelection(rest);
 if(s.bindingId||s.bindingOffset||s.offset)throw Error('UNSUPPORTED_SELECTION');
 if(materialOffset!==undefined&&(typeof materialOffset!=='string'||!/^\d+$/.test(materialOffset)))throw Error('INVALID_PAGE');
 const n=materialOffset===undefined?0:Number(materialOffset);if(!Number.isSafeInteger(n)||n<0||n>2147483647)throw Error('INVALID_PAGE');
 if(materialId!==undefined&&(typeof materialId!=='string'||!id(materialId)))throw Error('INVALID_MATERIAL');
 if(!s.deliveryOperationId&&(materialId||n))throw Error('TARGET_REQUIRED');
 return {...s,materialOffset:n,...(materialId?{materialId:materialId.toLowerCase()}:{})};
}
export function basisHref(s:Partial<BasisSelection>,section?:'material-detail'|'receipt-materials'):string{
 const q=new URLSearchParams();for(const k of ['deliveryOperationId','datasetVersionId','contractVersionId','materialId'] as const)if(s[k])q.set(k,s[k]!);if(s.materialOffset)q.set('materialOffset',String(s.materialOffset));return '/core/acceptance-basis'+(q.size?'?'+q:'')+(section?'#'+section:'');
}
export function basisContextHref(s:BasisSelection):string{return acceptanceHref({...s,basisMaterialOffset:s.materialOffset,basisMaterialId:s.materialId,offset:0,bindingOffset:0,bindingId:undefined})}
