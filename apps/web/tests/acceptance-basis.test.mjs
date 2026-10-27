import test from 'node:test';import assert from 'node:assert/strict';import {createRequire} from 'node:module';
import {acceptanceBindingFixture} from './fixtures/acceptance-bindings.mjs';
const {validMaterial,validMaterialDetail,validBusinessBasis,basisSelection,basisHref,basisContextHref}=createRequire(import.meta.url)('../.iam-tests/acceptance-basis.js');
const {acceptanceSelection}=createRequire(import.meta.url)('../.iam-tests/acceptance-context.js');
const ws='11111111-1111-4111-8111-111111111111',id='44444444-4444-4444-8444-444444444444',at='2026-10-24T00:00:00Z';
const principal={principal_id:id,canonical_subject:id,workspace_id:ws,revision:'1:1:1',capabilities:['BUSINESS_ACCEPTANCE_READ']};
const selection={deliveryOperationId:id,datasetVersionId:id,bindingOffset:0,offset:0,materialOffset:0,materialId:id};
const ref=()=>({id,workspaceId:ws,envelopeStatus:'VALID',envelopeSha256:'a'.repeat(64),recordedAt:at,recordedBy:id,claimKind:'RECEIPT_CLAIM'});
const material=()=>({evidence:ref(),contentStatus:'UNATTESTED',recipientStatus:'UNATTESTED',businessUse:'BLOCKED',blockers:['RECEIPT_SOURCE_UNCONFIGURED','CONTENT_NOT_ATTESTED','RECIPIENT_NOT_ATTESTED']});
function basis(){const c=acceptanceBindingFixture({ids:{workspace:ws,version:id,deliveryOperation:id,goldVersion:id},stamp:at,standards:[],scenario:'acceptance-sources'});c.admission={profileVersion:'business-acceptance.v1',principalId:id,canonicalSubject:id,workspaceId:ws,authorityRevision:'1:1:1',evaluatedAt:at,expiresAt:'2026-10-24T00:05:00Z',capabilities:principal.capabilities,writeEnabled:false,actions:[['CreateBusinessAcceptance','PROVIDER','BUSINESS_ACCEPTANCE_DRAFT'],['ReviseAcceptanceDraft','PROVIDER','BUSINESS_ACCEPTANCE_DRAFT'],['ConfirmAcceptanceBasis','BASIS_CONFIRMER','BUSINESS_ACCEPTANCE_BASIS_CONFIRM'],['RecordBusinessReceipt','RECEIVER','BUSINESS_RECEIPT_RECORD']].map(([command,role,capability],i)=>({command,role,capability,responsibilityEligible:false,blockers:['ACCEPTANCE_WRITES_DISABLED','CURRENT_TARGET_RESPONSIBILITY_REQUIRED','ACTION_CAPABILITY_REQUIRED','DELIVERY_NOT_ISSUED','STANDARD_APPLICABILITY_REQUIRED',...(i?['CASE_COMMAND_NOT_IMPLEMENTED']:[])]}))};return {profileVersion:'business-basis.v1',context:c,identity:{principalId:id,workspaceId:ws,subjectBindingId:id,workspaceGrantId:id,authorityRevision:'1:1:1',authentication:'OIDC_CONFIRMED',directorySource:'UNCONFIGURED'},responsibilitySource:'UNCONFIGURED',standardSource:'CORE_RECORD_ONLY',receiptSource:'UNCONFIGURED',evidence:[],materials:{items:[material()],page:{limit:25,offset:0,total:1}},writeEnabled:false,blockers:['ACCEPTANCE_WRITES_DISABLED','DIRECTORY_SOURCE_UNCONFIGURED','RESPONSIBILITY_SOURCE_UNCONFIGURED','STANDARD_AUTHORITY_UNCONFIGURED','RECEIPT_SOURCE_UNCONFIGURED']};}
test('a valid envelope and receipt claim never imply recipient/content/source authority',()=>{
 const m=material();assert.equal(validMaterial(m,ws),true);
 for(const patch of [{businessUse:'ALLOWED'},{contentStatus:'VERIFIED'},{recipientStatus:'VERIFIED'},{metadata:{verified:true}},{downloadUrl:'https://untrusted.invalid'}])assert.equal(validMaterial({...m,...patch},ws),false);
 for(const b of m.blockers)assert.equal(validMaterial({...m,blockers:m.blockers.filter(v=>v!==b)},ws),false);
 assert.equal(validMaterial({...m,evidence:{...m.evidence,title:'private'}},ws),false);
 assert.equal(validMaterial({...m,evidence:{...m.evidence,workspaceId:id}},ws),false);
 assert.equal(validMaterial({...m,evidence:{...m.evidence,envelopeStatus:'INVALID'}},ws),false);
});
test('basis rejects spoofed identities, source promotion, hidden fields and replaced pages',()=>{
 const b=basis();assert.equal(validBusinessBasis(b,principal,selection),true);
 for(const patch of [{writeEnabled:true},{receiptSource:'VERIFIED'},{responsibilitySource:'VERIFIED'},{standardSource:'VERIFIED'},{storageUri:'private'}])assert.equal(validBusinessBasis({...b,...patch},principal,selection),false);
 assert.equal(validBusinessBasis({...b,identity:{...b.identity,principalId:ws}},principal,selection),false);
 assert.equal(validBusinessBasis({...b,materials:{...b.materials,page:{limit:25,offset:25,total:26}}},principal,selection),false);
 const d={profileVersion:'business-basis.v1',workspaceId:ws,deliveryOperationId:id,datasetVersionId:id,principalId:id,authorityRevision:'1:1:1',evaluatedAt:at,material:material(),writeEnabled:false};
 assert.equal(validMaterialDetail(d,principal,selection),true);
 for(const patch of [{principalId:ws},{datasetVersionId:ws},{authorityRevision:'1:1:2'},{material:{...material(),evidence:{...ref(),id:ws}}}])assert.equal(validMaterialDetail({...d,...patch},principal,selection),false);
});
test('exact material/page/standard return survives the existing context and history flow',()=>{
 const s=basisSelection({deliveryOperationId:id,datasetVersionId:id,contractVersionId:id,materialId:id,materialOffset:'25'});
 const context=acceptanceSelection(Object.fromEntries(new URL('https://example.invalid'+basisContextHref(s)).searchParams));
 assert.equal(context.basisMaterialOffset,25);assert.equal(context.basisMaterialId,id);
 assert.ok(basisHref(s,'material-detail').endsWith('#material-detail'));
 assert.ok(basisHref({...s,materialId:undefined},'receipt-materials').endsWith('#receipt-materials'));
 assert.equal(basisHref({...context,materialOffset:context.basisMaterialOffset,materialId:context.basisMaterialId}),basisHref(s));
 for(const raw of [{materialOffset:'25'},{materialId:id},{...Object.fromEntries(new URL('https://example.invalid'+basisHref(s)).searchParams),actorId:id},{deliveryOperationId:id,datasetVersionId:id,offset:'0'},{deliveryOperationId:id,datasetVersionId:id,materialOffset:['0','25']},{deliveryOperationId:id,datasetVersionId:id,materialOffset:'2147483648'}])assert.throws(()=>basisSelection(raw));
});
