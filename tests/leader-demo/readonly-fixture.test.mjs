import test from 'node:test';
import assert from 'node:assert/strict';
import {once} from 'node:events';
import {createLeaderFixture,taskId,diagnosisId} from './readonly-fixture.mjs';
import {ids} from '../browser/fixture-server.mjs';
test('leader fixture has no commands/control surface and preserves exact scoped read facts',async()=>{
 const server=createLeaderFixture();server.listen(0,'127.0.0.1');await once(server,'listening');
 const base=`http://127.0.0.1:${server.address().port}`;
 const headers={Authorization:'Bearer review-secret'};
 const root=`/api/v1/workspaces/${ids.workspace}/agent-tasks`;
 try{
  const original=await(await fetch(base+root+'/'+taskId+'?planId='+diagnosisId,{headers})).json();assert.equal(original.plan.id,diagnosisId);
  for(const method of ['POST','PUT','PATCH','DELETE'])for(const path of [root,root+'/'+taskId+'/actions','/__control/reset']){assert.equal((await fetch(base+path,{method,headers})).status,405);}
  assert.equal((await fetch(base+'/__control/state',{headers})).status,404);
  assert.equal((await fetch(base+root)).status,401);
  assert.equal((await fetch(base+root.replace(ids.workspace,taskId)+'/'+taskId,{headers})).status,404);
  assert.equal((await fetch(base+root+'/'+diagnosisId,{headers})).status,404);
  assert.deepEqual(await(await fetch(base+root+'/'+taskId+'?planId='+diagnosisId,{headers})).json(),original);
  const report=await(await fetch(base+`/api/v1/quality-assessments/${ids.goldAssessment}/report?workspaceId=${ids.workspace}`,{headers})).json();
  const current=await(await fetch(base+root+'/'+taskId,{headers})).json();
  assert.equal(report.gateDecision,'FAIL');assert.equal(report.dimensionSummary.VALIDITY.failedCount,1);
  assert.equal(current.plan.sourceAssessmentId,report.id);
  const finding=report.findings.items.find(f=>f.id===current.plan.findingId);assert.equal(finding.ruleId,'negative-energy');assert.equal(finding.status,'FAIL');
  const history=await(await fetch(base+`/api/v1/dataset-versions/${ids.goldVersion}/quality-assessments?workspaceId=${ids.workspace}`,{headers})).json();
  const version=await(await fetch(base+`/api/v1/dataset-versions/${ids.goldVersion}?workspaceId=${ids.workspace}`,{headers})).json();
  assert.equal(history.page.total,2);
  assert.ok(history.items.every(entry=>Date.parse(entry.createdAt)>=Date.parse(version.createdAt)),"a report cannot predate its source version");
  for(const entry of history.items){const response=await fetch(base+`/api/v1/quality-assessments/${entry.id}/report?workspaceId=${ids.workspace}`,{headers});assert.equal(response.status,200);const exact=await response.json();assert.equal(exact.id,entry.id);assert.equal(exact.gateDecision,entry.gateDecision);assert.equal(exact.createdAt,entry.createdAt);assert.deepEqual(exact.dimensionSummary,entry.dimensionSummary);}
 }finally{await new Promise(resolve=>server.close(resolve));}
});
