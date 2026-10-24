import test from 'node:test';
import assert from 'node:assert/strict';
import {seedBusinessAcceptance,businessCommand,approvalBlockers,decisionInvalid,submissionOutcome} from '../../components/governance-demo/business-acceptance-model.mjs';
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
test('changed receipt can be returned without inventing a failed criterion',()=>{
 let s=run(receipt(seedBusinessAcceptance()),'SUBMIT','provider');const frozen=structuredClone(s.submissions[0]);s=receipt(s);
 s=run(s,'RETURN','reviewer',{results,note:'签收依据变化，重新冻结'});assert.deepEqual(s.decisions[0].results,results);assert.equal(s.decisions[0].outcome,'RETURNED');assert.deepEqual(s.submissions[0],frozen);
 s=run(s,'SUBMIT','provider');assert.notEqual(s.submissions[1].receipt.id,frozen.receipt.id);s=run(s,'APPROVE','reviewer',{results});assert.equal(s.cases[0].status,'ACCEPTED');
});
test('withdrawal identifies only its exact submission across resubmission and cancellation',()=>{
 let s=run(receipt(seedBusinessAcceptance()),'SUBMIT','provider');const first=s.submissions[0];s=run(s,'WITHDRAW','provider');s=run(s,'SUBMIT','provider');
 assert.equal(submissionOutcome(s,s.cases[0],first),'已撤回');assert.equal(submissionOutcome(s,s.cases[0],s.submissions[1]),'待验收');
 const withdrawal=s.events.find(x=>x.type==='WITHDRAW');assert.equal(withdrawal.submissionId,first.id);s=run(s,'WITHDRAW','provider');s=run(s,'CANCEL','provider');assert.equal(s.submissions.length,2);assert.equal(s.decisions.length,0);assert.equal(s.archives.length,0);assert.equal(submissionOutcome(s,s.cases[0],s.submissions[1]),'已撤回');
});

test('editing standard text cannot weaken mandatory checks or replace the verification method',()=>{
 const s=receipt(seedBusinessAcceptance()),before=structuredClone(s);
 for(const change of [{required:false},{required:undefined},{method:'跳过核验'}]){
  const criteria=s.cases[0].criteria.map(x=>({...x,...change}));
  assert.throws(()=>run(s,'SAVE','provider',{title:'修订说明',criteria}),/必需性与核验方式/);
  assert.deepEqual(s,before);
 }
 const next=run(s,'SAVE','provider',{title:'修订说明',criteria:s.cases[0].criteria.map(x=>({...x,description:'  新标准 '+x.id+'  ',source:'  新约定 v2  '}))});
 assert.equal(next.cases[0].criteria[0].description,'新标准 C1');assert.equal(next.cases[0].criteria[0].source,'新约定 v2');
 let pending=run(next,'SUBMIT','provider');assert.throws(()=>run(pending,'APPROVE','reviewer',{results:{}}),/逐项/);
 pending=run(pending,'APPROVE','reviewer',{results});assert.equal(pending.decisions[0].outcome,'ACCEPTED');assert.deepEqual(s,before);
});

test('terminal cases reject every reopening command without changing frozen historical facts',()=>{
 const accepted=run(run(receipt(seedBusinessAcceptance()),'SUBMIT','provider'),'APPROVE','reviewer',{results});
 const archived=run(accepted,'ARCHIVE','archivist'),invalid=run(archived,'INVALIDATE','reviewer');
 const cancelled=run(seedBusinessAcceptance(),'CANCEL','provider');
 const freeze=value=>{if(value&&typeof value==='object'){Object.values(value).forEach(freeze);Object.freeze(value)}return value};
 for(const state of [accepted,archived,invalid,cancelled]){
  const before=structuredClone(state);freeze(state);
  for(const [type,actor,extra] of [['SAVE','provider',{title:'不能复活',criteria:state.cases[0].criteria}],['RECEIPT','receiver',{outcome:'RECEIVED',evidence:'不能补签'}],['SUBMIT','provider',{}],['WITHDRAW','provider',{}],['RETURN','reviewer',{results:{C1:'FAIL'}}],['APPROVE','reviewer',{results}],['CANCEL','provider',{}]]){
   assert.throws(()=>run(state,type,actor,extra));assert.deepEqual(state,before);
  }
 }
 assert.deepEqual(invalid.archives,archived.archives);assert.deepEqual(invalid.decisions,archived.decisions);assert.deepEqual(invalid.submissions,archived.submissions);assert.deepEqual(invalid.receipts,archived.receipts);
});
