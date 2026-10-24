import {validAcceptanceContext,type AcceptanceContext,type AcceptanceSelection} from './acceptance-context';
import type {Principal} from './iam-core';
export type Admission={profileVersion:string;principalId:string;canonicalSubject:string;workspaceId:string;authorityRevision:string;evaluatedAt:string;expiresAt:string;capabilities:string[];actions:{command:string;role:string;capability:string;responsibilityEligible:boolean;blockers:string[]}[];writeEnabled:false};
export type IdentityContext=AcceptanceContext&{admission:Admission};
const actions=[['CreateBusinessAcceptance','PROVIDER','BUSINESS_ACCEPTANCE_DRAFT'],['ReviseAcceptanceDraft','PROVIDER','BUSINESS_ACCEPTANCE_DRAFT'],['ConfirmAcceptanceBasis','BASIS_CONFIRMER','BUSINESS_ACCEPTANCE_BASIS_CONFIRM'],['RecordBusinessReceipt','RECEIVER','BUSINESS_RECEIPT_RECORD']];
const blockers=['ACCEPTANCE_WRITES_DISABLED','CURRENT_TARGET_RESPONSIBILITY_REQUIRED','ACTION_CAPABILITY_REQUIRED','DELIVERY_NOT_ISSUED','STANDARD_APPLICABILITY_REQUIRED','SYNTHETIC_SOURCE_NOT_BUSINESS_AUTHORITY','CASE_COMMAND_NOT_IMPLEMENTED'];
function keys(v:object,expected:string){return Object.keys(v).sort().join(',')===expected.split(',').sort().join(',');}
export function validIdentityContext(raw:AcceptanceContext,p:Principal,s:AcceptanceSelection):raw is IdentityContext{
 try{
  const c=raw as IdentityContext,a=c.admission;
  if(!a||!keys(a,'profileVersion,principalId,canonicalSubject,workspaceId,authorityRevision,evaluatedAt,expiresAt,capabilities,actions,writeEnabled')||a.profileVersion!=='business-acceptance.v1'||a.principalId!==p.principal_id||a.canonicalSubject!==p.canonical_subject||a.workspaceId!==p.workspace_id||a.authorityRevision!==p.revision||a.writeEnabled!==false||a.evaluatedAt!==c.evaluatedAt||!Number.isFinite(Date.parse(a.evaluatedAt))||!Number.isFinite(Date.parse(a.expiresAt))||Date.parse(a.expiresAt)<=Date.parse(a.evaluatedAt)||Date.parse(a.expiresAt)>Date.parse(a.evaluatedAt)+300000||!Array.isArray(a.capabilities)||JSON.stringify([...a.capabilities].sort())!==JSON.stringify([...p.capabilities].sort())||!Array.isArray(a.actions)||a.actions.length!==4)return false;
  const clean={...c};delete (clean as Partial<IdentityContext>).admission;
  if(!validAcceptanceContext(clean,p.workspace_id,s.deliveryOperationId!,s.datasetVersionId!,s.contractVersionId,s.bindingOffset,s.bindingId))return false;
  for(const r of c.responsibilities){if(r.isCurrentCaller&&(!r.representative||r.representative.actorId!==p.principal_id||r.representative.subject!==p.canonical_subject))return false;}
  for(let i=0;i<4;i++){
   const x=a.actions[i],expected=actions[i];
   if(!x||!keys(x,'command,role,capability,responsibilityEligible,blockers')||x.command!==expected[0]||x.role!==expected[1]||x.capability!==expected[2]||typeof x.responsibilityEligible!=='boolean'||x.responsibilityEligible!==c.responsibilities.some(r=>r.role===x.role&&r.status==='ASSIGNED'&&r.isCurrentCaller)||!Array.isArray(x.blockers)||!x.blockers.includes('ACCEPTANCE_WRITES_DISABLED')||new Set(x.blockers).size!==x.blockers.length||x.blockers.some(b=>!blockers.includes(b)))return false;
   if(!x.responsibilityEligible&&!x.blockers.includes('CURRENT_TARGET_RESPONSIBILITY_REQUIRED')||!p.capabilities.includes(x.capability)&&!x.blockers.includes('ACTION_CAPABILITY_REQUIRED')||c.delivery.status!=='ISSUED'&&!x.blockers.includes('DELIVERY_NOT_ISSUED')||c.applicability.status!=='MATCHED'&&!x.blockers.includes('STANDARD_APPLICABILITY_REQUIRED')||c.responsibilities.some(r=>r.representative?.origin==='SYNTHETIC_FIXTURE')&&!x.blockers.includes('SYNTHETIC_SOURCE_NOT_BUSINESS_AUTHORITY')||i>0&&!x.blockers.includes('CASE_COMMAND_NOT_IMPLEMENTED'))return false;
  }
  return true;
 }catch{return false;}
}
