import test from 'node:test';
import assert from 'node:assert/strict';
import {seedBusinessAcceptance,businessCommand,approvalBlockers,decisionInvalid} from '../../components/governance-demo/business-acceptance-model.mjs';
const caseId='DEMO-ACC-1';
const run=(s,type,actor,extra={})=>businessCommand(s,{type,actor,caseId,expectedRevision:s.cases[0].revision,note:'核对说明',...extra},'2026-10-24T06:00:00Z');
const receipt=s=>run(s,'RECEIPT','receiver',{outcome:'RECEIVED',evidence:'演示收件核对记录'});
const results={C1:'PASS',C2:'PASS',C3:'PASS'};
test('missing/partial receipt, wrong role and stale revision cannot advance business acceptance',()=>{
 const s=seedBusinessAcceptance();assert.throws(()=>run(s,'SUBMIT','provider'),/完整签收/);assert.throws(()=>run(s,'RECEIPT','provider',{outcome:'RECEIVED',evidence:'说明'}),/收件方/);
 const partial=run(s,'RECEIPT','receiver',{outcome:'PARTIAL',evidence:'说明'});assert.throws(()=>run(partial,'SUBMIT','provider'),/完整签收/);
 assert.throws(()=>businessCommand(partial,{type:'SUBMIT',actor:'provider',caseId,expectedRevision:0}),/记录已变化/);
 assert.equal(s.receipts.length,0);
});
test('return and resubmission preserve original criteria, receipt and decision',()=>{
 let s=run(receipt(seedBusinessAcceptance()),'SUBMIT','provider');const first=structuredClone(s.submissions[0]);
 assert.throws(()=>run(s,'APPROVE','provider',{results}),/验收人/);
 assert.throws(()=>run(s,'APPROVE','reviewer',{results:{C1:'PASS'}}),/逐项/);
 s=run(s,'RETURN','reviewer',{results:{C1:'FAIL'}});const old=s.decisions[0];
 s=run(s,'SAVE','provider',{title:'修订后的验收',criteria:s.cases[0].criteria.map(x=>({...x,description:x.description+' 修订'}))});
 s=run(s,'SUBMIT','provider');assert.deepEqual(s.submissions[0],first);assert.deepEqual(s.decisions[0],old);assert.equal(s.submissions[1].number,2);assert.notEqual(s.submissions[1].criteria[0].description,first.criteria[0].description);
});
test('receipt changes after submission require a new submission; no silent rebind',()=>{
 let s=run(receipt(seedBusinessAcceptance()),'SUBMIT','provider');const frozen=s.submissions[0].receipt.id;s=receipt(s);assert.notEqual(s.cases[0].receiptId,frozen);assert.ok(approvalBlockers(s,s.cases[0],results).some(x=>x.includes('依据已变化')));assert.throws(()=>run(s,'APPROVE','reviewer',{results}),/依据已变化/);
});
test('archive is separate, repeat is idempotent and later invalidation preserves immutable history',()=>{
 let s=run(receipt(seedBusinessAcceptance()),'SUBMIT','provider');s=run(s,'APPROVE','reviewer',{results});assert.equal(s.archives.length,0);assert.throws(()=>run(s,'ARCHIVE','provider'),/归档经办人/);
 s=run(s,'ARCHIVE','archivist');const archive=structuredClone(s.archives[0]);assert.equal(s.cases[0].status,'ACCEPTED');assert.equal(run(s,'ARCHIVE','archivist').archives.length,1);
 s=run(s,'INVALIDATE','reviewer');assert.deepEqual(s.archives[0],archive);assert.equal(s.decisions[0].outcome,'ACCEPTED');assert.equal(decisionInvalid(s,s.decisions[0].id),true);assert.throws(()=>run(s,'ARCHIVE','archivist'),/有效的验收/);
});
test('withdraw preserves a frozen submission; cancel is explicit and terminal',()=>{
 let s=run(receipt(seedBusinessAcceptance()),'SUBMIT','provider');s=run(s,'WITHDRAW','provider');assert.equal(s.submissions.length,1);s=run(s,'CANCEL','provider');assert.equal(s.cases[0].status,'CANCELLED');assert.throws(()=>run(s,'SUBMIT','provider'),/不可提交/);
});
