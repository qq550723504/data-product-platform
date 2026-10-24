import test from 'node:test';import assert from 'node:assert/strict';import {createRequire} from 'node:module';
import {acceptanceBindingFixture} from './fixtures/acceptance-bindings.mjs';
const {validIdentityContext}=createRequire(import.meta.url)('../.iam-tests/acceptance-identity.js');
const ws='11111111-1111-4111-8111-111111111111',id='44444444-4444-4444-8444-444444444444',at='2026-10-24T00:00:00Z';
const principal={principal_id:id,canonical_subject:id,workspace_id:ws,revision:'1:1:1',capabilities:['BUSINESS_ACCEPTANCE_READ']};
const selection={deliveryOperationId:id,datasetVersionId:id,bindingOffset:0,offset:0};
function value(){const c=acceptanceBindingFixture({ids:{workspace:ws,version:id,deliveryOperation:id,goldVersion:id},stamp:at,standards:[],scenario:'acceptance-sources'});return {...c,admission:{profileVersion:'business-acceptance.v1',principalId:id,canonicalSubject:id,workspaceId:ws,authorityRevision:'1:1:1',evaluatedAt:at,expiresAt:'2026-10-24T00:05:00Z',capabilities:principal.capabilities,writeEnabled:false,actions:[['CreateBusinessAcceptance','PROVIDER','BUSINESS_ACCEPTANCE_DRAFT'],['ReviseAcceptanceDraft','PROVIDER','BUSINESS_ACCEPTANCE_DRAFT'],['ConfirmAcceptanceBasis','BASIS_CONFIRMER','BUSINESS_ACCEPTANCE_BASIS_CONFIRM'],['RecordBusinessReceipt','RECEIVER','BUSINESS_RECEIPT_RECORD']].map(([command,role,capability],i)=>({command,role,capability,responsibilityEligible:false,blockers:['ACCEPTANCE_WRITES_DISABLED','CURRENT_TARGET_RESPONSIBILITY_REQUIRED','ACTION_CAPABILITY_REQUIRED','DELIVERY_NOT_ISSUED','STANDARD_APPLICABILITY_REQUIRED',...(i?['CASE_COMMAND_NOT_IMPLEMENTED']:[])]}))}};}
test('admission binds current canonical person, space, revision, exact four actions and write closure',()=>{
 const c=value();assert.equal(validIdentityContext(c,principal,selection),true);
 for(const patch of [{principalId:ws},{canonicalSubject:'external-sub'},{workspaceId:id},{authorityRevision:'1:1:2'},{writeEnabled:true},{profileVersion:'reviewer'},{expiresAt:at},{expiresAt:'2026-10-24T00:05:01Z'},{capabilities:['BUSINESS_ACCEPTANCE_DRAFT']},{actor:'fake'}])assert.equal(validIdentityContext({...c,admission:{...c.admission,...patch}},principal,selection),false);
 const forged=value();forged.admission.actions[0].responsibilityEligible=true;assert.equal(validIdentityContext(forged,principal,selection),false);
});
test('missing blockers, actor controls, foreign targets and duplicate actions fail closed',()=>{
 for(const reason of ['ACCEPTANCE_WRITES_DISABLED','CURRENT_TARGET_RESPONSIBILITY_REQUIRED','ACTION_CAPABILITY_REQUIRED','DELIVERY_NOT_ISSUED','STANDARD_APPLICABILITY_REQUIRED']){const c=value();c.admission.actions[0].blockers=c.admission.actions[0].blockers.filter(v=>v!==reason);assert.equal(validIdentityContext(c,principal,selection),false,reason);}
 const c=value();c.admission.actions[1]=c.admission.actions[0];assert.equal(validIdentityContext(c,principal,selection),false);
 assert.equal(validIdentityContext(value(),principal,{...selection,datasetVersionId:ws}),false);
});
