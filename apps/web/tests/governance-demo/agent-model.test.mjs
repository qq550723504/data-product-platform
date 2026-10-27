import test from 'node:test';
import assert from 'node:assert/strict';
import {emptyPipeline,initialPipeline,savePlan,verifyRun} from '../../components/governance-demo/pipeline-model.mjs';
import {createAgent,editAgent,approveAgent,advanceAgent,controlAgent,approveRepair,agentStatus,syncAgent,retryAgent,reviewAgentPlan,verifyAgentRun,reviewAgentResult,forkAgent,resolveAgentContext,prepareAgentReport} from '../../components/governance-demo/agent-model.mjs';
const draft={goal:'Agent 企业经营分析方案',purpose:'企业经营分析',tables:['企业基础信息','租赁记录','用电记录'],availableTables:['企业基础信息','租赁记录','用电记录'],standardId:'STD-0001',acceptance:'隔离异常，独立复检，保留依据'};
const task=s=>s.agentTasks.at(-1);
function quality(){let s=createAgent(initialPipeline(),'quality',{...draft,versionId:'VER-0001'});s=approveAgent(s,task(s).id);s=advanceAgent(s,task(s).id);return advanceAgent(s,task(s).id)}
test('unapproved plan has no business effect; revised scope requires fresh approval',()=>{
 let s=createAgent(initialPipeline(),'plan',draft);const id=task(s).id;
 assert.throws(()=>advanceAgent(s,id),/确认/);s=approveAgent(s,id);s=editAgent(s,id,{tables:['企业基础信息']});
 assert.equal(task(s).approvalRevision,null);assert.equal(task(s).history.length,1);assert.throws(()=>advanceAgent(s,id),/确认/);
 s=approveAgent(s,id);for(let n=0;n<4;n++)s=advanceAgent(s,id);
 assert.equal(task(s).status,'待验收');assert.equal(s.demands.length,2);assert.equal(s.plans.at(-1).tables.length,1);assert.equal(s.versions.at(-1).rows,6);assert.equal(s.releases.length,0);
 assert.throws(()=>advanceAgent(s,id),/确认/);assert.ok(task(s).artifacts.some(a=>a.id===s.runs.at(-1).id));
});
test('pause and stop block future steps and preserve already completed records',()=>{
 let s=createAgent(initialPipeline(),'plan',draft);const id=task(s).id;s=approveAgent(s,id);s=advanceAgent(s,id);s=advanceAgent(s,id);const prior=JSON.stringify(s.demands);
 s=controlAgent(s,id,'暂停');assert.throws(()=>advanceAgent(s,id),/确认/);s=controlAgent(s,id,'继续');s=controlAgent(s,id,'停止');assert.equal(JSON.stringify(s.demands),prior);assert.throws(()=>advanceAgent(s,id),/确认/);assert.throws(()=>controlAgent(s,id,'继续'),/确认/);
});
test('quality diagnosis requires a second approval; repair freezes exact source and retains original facts',()=>{
 let s=quality();const id=task(s).id;assert.equal(task(s).status,'待修复确认');assert.equal(s.versions.length,1);assert.equal(s.assessments.length,1);assert.throws(()=>advanceAgent(s,id),/确认/);
 const original=JSON.stringify({v:s.versions[0],run:s.runs[0],plan:s.plans[0],q:s.assessments[0]});
 s=approveRepair(s,id);s=advanceAgent(s,id);s=advanceAgent(s,id);
 assert.equal(task(s).status,'待验收');assert.equal(s.versions[1].rows,30);assert.deepEqual(s.versions[1].inputs,['VER-0001']);assert.equal(s.versions[1].isolated,2);assert.equal(s.assessments[1].passed,true);assert.equal(s.certifications.length,0);assert.equal(s.releases.length,0);
 assert.equal(JSON.stringify({v:s.versions[0],run:s.runs[0],plan:s.plans[0],q:s.assessments[0]}),original);
 assert.equal(s.events.filter(e=>e.cost===1).length,4); // seed run + source assessment + repair run + independent reassessment
});
test('empty workspace cannot approve a version from another space or fabricate sources',()=>{
 let s=createAgent(emptyPipeline(),'quality',{...draft,versionId:'VER-0001'});assert.equal(task(s).status,'待补充');assert.throws(()=>approveAgent(s,task(s).id),/明确版本/);
 s=createAgent(emptyPipeline(),'plan',{...draft,availableTables:[]});assert.throws(()=>approveAgent(s,task(s).id),/当前空间/);assert.equal(s.demands.length,0);
});
test('ended demand is diagnosed but cannot be silently reopened for repair',()=>{
 let s=quality();const d=s.demands[0];s={...s,work:[{demandId:d.id,state:'已归档'}]};s=approveRepair(s,task(s).id);assert.equal(task(s).status,'业务阻断');assert.match(task(s).issue.reason,/已结束/);assert.equal(s.versions.length,1);const blocked=s;s=retryAgent(s,task(s).id);assert.equal(s,blocked);
});
test('a version with no failures completes diagnosis without a redundant repair version',()=>{
 let s=quality();s=approveRepair(s,task(s).id);s=advanceAgent(s,task(s).id);s=advanceAgent(s,task(s).id);s=createAgent(s,'quality',{...draft,versionId:'VER-0002'});const id=task(s).id;s=approveAgent(s,id);s=advanceAgent(s,id);s=advanceAgent(s,id);assert.equal(task(s).status,'待验收');assert.equal(task(s).reports.length,1);assert.equal(s.versions.length,2);assert.throws(()=>approveRepair(s,id),/先完成诊断/);
});
test('repairing an older version keeps that exact input even after later plan revisions exist',()=>{
 let s=quality();s=approveRepair(s,task(s).id);s=advanceAgent(s,task(s).id);s=advanceAgent(s,task(s).id);
 s=createAgent(s,'quality',{...draft,versionId:'VER-0001'});const id=task(s).id;s=approveAgent(s,id);s=advanceAgent(s,id);s=advanceAgent(s,id);s=approveRepair(s,id);s=advanceAgent(s,id);
 assert.deepEqual(s.versions.at(-1).inputs,['VER-0001']);assert.equal(s.runs.at(-1).inputRows,32);assert.equal(s.versions.at(-1).rows,30);assert.equal(s.plans.at(-1).revision,3);
});
test('an existing demand title is a visible blocker before approval or business writes',()=>{
 let s=createAgent(initialPipeline(),'plan',{...draft,goal:'企业经营活跃度分析'});assert.equal(task(s).status,'待补充');assert.throws(()=>approveAgent(s,task(s).id),/名称已存在/);assert.equal(s.demands.length,1);
});
function planned(){let s=createAgent(initialPipeline(),'plan',draft);const id=task(s).id;s=approveAgent(s,id);s=advanceAgent(s,id);return advanceAgent(s,id)}
function changed(s){const t=task(s),old=s.plans.find(p=>p.id===t.planId);return savePlan(s,t.demandId,{...old,tables:['企业基础信息'],acceptance:'仅企业基础信息六条'})}
test('plan drift stops before a physical run; choosing the original pins the approved three sources',()=>{
 let s=planned();const id=task(s).id,pid=task(s).planId,approval=JSON.stringify(task(s).approvals),runs=s.runs.length;s=changed(s);assert.equal(agentStatus(s,task(s)),'待重新确认');
 s=advanceAgent(s,id);assert.equal(task(s).status,'待重新确认');assert.equal(s.runs.length,runs);const once=s;s=syncAgent(s,id);assert.equal(s,once);assert.equal(task(s).attempts.at(-1).outcome,'阻断');
 s=reviewAgentPlan(s,id,'沿用原版');assert.equal(task(s).planId,pid);assert.equal(JSON.stringify(task(s).approvals.slice(0,1)),approval);s=advanceAgent(s,id);assert.equal(s.runs.at(-1).planId,pid);assert.equal(s.versions.at(-1).rows,32);
});
test('adopting a newer business plan creates a revision and approval; later drift requires another review',()=>{
 let s=changed(planned());const id=task(s).id,old=task(s).planId,latest=s.plans.at(-1).id;s=syncAgent(s,id);s=reviewAgentPlan(s,id,'采用新版');assert.equal(task(s).revision,2);assert.equal(task(s).planId,latest);assert.equal(task(s).history[0].revision,1);assert.equal(task(s).approvals.at(-1).businessPlanId,latest);assert.equal(task(s).bindings.at(-1).previousPlanId,old);
 s=changed(s);assert.equal(agentStatus(s,task(s)),'待重新确认');s=syncAgent(s,id);s=reviewAgentPlan(s,id,'采用新版');s=advanceAgent(s,id);assert.equal(s.versions.at(-1).rows,6);assert.equal(s.runs.at(-1).planId,task(s).planId);
});
test('historical task context ignores a globally selected unrelated output, and absent context never falls through',()=>{
 let s=quality();s=approveRepair(s,task(s).id);s=advanceAgent(s,task(s).id);const first=task(s).id;s=advanceAgent(s,first);s=createAgent(s,'plan',{...draft,goal:'另一项需求'});const id=task(s).id;s=approveAgent(s,id);for(let n=0;n<3;n++)s=advanceAgent(s,id);s={...s,selectedAgentId:first,selectedVersionId:s.versions.at(-1).id};
 const c=resolveAgentContext(s,'agent-task');assert.equal(c.source.id,'VER-0001');assert.equal(c.output.id,'VER-0002');assert.deepEqual(c.versions.map(v=>v.id),['VER-0001','VER-0002']);assert.equal(c.primary.id,'VER-0002');
 s={...s,selectedAgentId:'missing'};assert.equal(resolveAgentContext(s,'agent-task').primary,null);assert.equal(resolveAgentContext(s,'agent-task').versions.length,0);assert.equal(resolveAgentContext(s,'agent-workbench').primary,null);assert.equal(resolveAgentContext(emptyPipeline(),'agent-task').versions.length,0);
});
test('missing standard can be repaired by a new revision before confirmation',()=>{
 let s=createAgent(initialPipeline(),'plan',{...draft,standardId:''});const id=task(s).id;assert.equal(task(s).status,'待补充');s=editAgent(s,id,{standardId:'STD-0001'});assert.equal(task(s).status,'待确认');assert.equal(task(s).revision,2);s=approveAgent(s,id);assert.equal(task(s).approvals.at(-1).plan.standardId,'STD-0001');
});
test('ended demand becomes a retained business blocker and repeat checks do not duplicate attempts',()=>{
 let s=planned();const id=task(s).id,demand=task(s).demandId,prior=s.runs.length;s={...s,work:[{demandId:demand,state:'已取消'}]};s=syncAgent(s,id);assert.equal(task(s).status,'业务阻断');const serialized=JSON.stringify(task(s));s=retryAgent(s,id);assert.equal(JSON.stringify(task(s)),serialized);assert.equal(s.runs.length,prior);assert.equal(s.events.at(-1).result,'阻断');assert.equal(task(s).attempts.at(-1).outcome,'阻断');
 s=controlAgent(s,id,'停止');assert.equal(task(s).status,'已停止');assert.equal(s.work.at(-1).state,'已取消');
});
test('failed physical run is preserved and retry makes a separate run and cost activity',()=>{
 let s=planned();const id=task(s).id;s=advanceAgent(s,id,'失败');const failed=s.runs.at(-1),a=task(s).attempts.at(-1),count=s.runs.length,events=s.events.filter(e=>e.cost===1).length;assert.equal(task(s).status,'执行失败');assert.equal(s.versions.length,1);
 s=retryAgent(s,id);s=advanceAgent(s,id,'成功');assert.equal(s.runs.length,count+1);assert.deepEqual(s.runs.find(r=>r.id===failed.id),failed);assert.deepEqual(task(s).attempts.find(x=>x.id===a.id),a);assert.equal(s.versions.length,2);assert.equal(s.events.filter(e=>e.cost===1).length,events+1);assert.equal(task(s).cursor,3);
});
test('unknown physical run requires append-only verification and cannot be stopped or retried',()=>{
 let s=planned();const id=task(s).id;s=advanceAgent(s,id,'待核实');const run=s.runs.at(-1),attempt=task(s).attempts.at(-1),count=s.runs.length;assert.equal(task(s).status,'待核实');assert.throws(()=>controlAgent(s,id,'停止'),/先核实/);assert.throws(()=>retryAgent(s,id),/不需要恢复/);assert.throws(()=>advanceAgent(s,id),/确认/);
 s=verifyAgentRun(s,id,'成功');assert.deepEqual(s.runs.at(-1),run);assert.deepEqual(task(s).attempts.at(-1),attempt);assert.equal(s.runs.length,count);assert.equal(s.versions.length,2);assert.equal(task(s).observations[0].attemptId,attempt.id);assert.throws(()=>verifyAgentRun(s,id,'成功'),/没有待核实/);
});
test('verified failed run allows a fresh retry while preserving original unknown observation',()=>{
 let s=planned();const id=task(s).id;s=advanceAgent(s,id,'待核实');s=verifyAgentRun(s,id,'失败');assert.equal(task(s).status,'执行失败');assert.equal(s.versions.length,1);s=retryAgent(s,id);s=advanceAgent(s,id);assert.equal(s.runs.length,3);assert.equal(task(s).attempts.filter(a=>a.runId).length,2);assert.equal(task(s).observations[0].outcome,'失败');
});
test('result review records acceptance without creating certification, release, delivery or business acceptance',()=>{
 let s=planned();const id=task(s).id;s=advanceAgent(s,id);s=advanceAgent(s,id);const report=JSON.stringify(task(s).reports[0]),versions=JSON.stringify(s.versions);assert.equal(task(s).status,'待验收');assert.throws(()=>reviewAgentResult(s,id,'接受成果',' '),/审阅说明/);
 s=reviewAgentResult(s,id,'接受成果','范围与成果符合本次委托');assert.equal(task(s).status,'已完成');assert.equal(task(s).reviews[0].reportId,task(s).reports[0].id);assert.equal(JSON.stringify(task(s).reports[0]),report);assert.equal(JSON.stringify(s.versions),versions);for(const key of ['certifications','releases','deliveries','work'])assert.equal(s[key].length,0);
});
test('return retains report and decisions; a linked rework starts with new confirmation and independent outputs',()=>{
 let s=planned();const id=task(s).id;s=advanceAgent(s,id);s=advanceAgent(s,id);s=reviewAgentResult(s,id,'退回重办','需要补充验收口径');const old=JSON.stringify(task(s));s=forkAgent(s,id);assert.equal(task(s).parentAgentId,id);assert.equal(task(s).status,'待确认');assert.equal(task(s).approvalRevision,null);assert.equal(task(s).steps.length,0);assert.equal(JSON.stringify(s.agentTasks[0]),old);assert.throws(()=>advanceAgent(s,task(s).id),/确认/);
});
test('closed-source follow-up creates a new demand and preserves lineage, purpose and closed original work',()=>{
 let s=quality();const old=s.demands[0].id,id=task(s).id;s={...s,work:[{demandId:old,state:'已归档'}]};s=approveRepair(s,id);s=forkAgent(s,id,true);const linked=task(s).id;s=approveAgent(s,linked);s=syncAgent(s,linked);assert.equal(task(s).status,'执行中');s=advanceAgent(s,linked);s=advanceAgent(s,linked);s=approveRepair(s,linked);s=advanceAgent(s,linked);assert.notEqual(task(s).demandId,old);assert.equal(s.work.find(w=>w.demandId===old).state,'已归档');assert.deepEqual(s.versions.at(-1).inputs,['VER-0001']);assert.equal(s.versions.at(-1).purpose,s.versions[0].purpose);assert.equal(s.versions.at(-1).demandId,task(s).demandId);
});
test('historical-source diagnosis is allowed but cannot silently repair under the ended demand',()=>{
 let s=initialPipeline();s={...s,work:[{demandId:s.demands[0].id,state:'已取消'}]};s=createAgent(s,'quality',{...draft,versionId:'VER-0001'});const id=task(s).id;s=approveAgent(s,id);s=syncAgent(s,id);assert.equal(task(s).status,'执行中');s=advanceAgent(s,id);s=advanceAgent(s,id);s=approveRepair(s,id);assert.equal(task(s).status,'业务阻断');assert.equal(s.demands.length,1);
});
test('actual retained completed task receives a new report only through an explicit command, with no fabricated approvals',()=>{
 let s=planned();const id=task(s).id;s=advanceAgent(s,id);s=advanceAgent(s,id);const {reports,approvals,attempts,bindings,reviews,observations,...retained}=task(s);s={...s,agentTasks:[{...retained,status:'已完成'}]};s=prepareAgentReport(s,id);assert.equal(task(s).status,'待验收');assert.equal(task(s).reports.length,1);assert.deepEqual(task(s).reports[0].approvalIds,[]);assert.equal(task(s).approvals,undefined);assert.throws(()=>prepareAgentReport(s,id),/无需/);
});
test('business-page verification reconciles back to Agent without duplicating run, version or business observation',()=>{
 let s=planned();const id=task(s).id;s=advanceAgent(s,id,'待核实');const run=s.runs.at(-1);s=verifyRun(s,run.id,'成功');const observations=s.observations.length,runs=s.runs.length,versions=s.versions.length;assert.equal(agentStatus(s,task(s)),'执行中');s=syncAgent(s,id);assert.equal(task(s).status,'执行中');assert.equal(task(s).cursor,3);assert.equal(s.observations.length,observations);assert.equal(s.runs.length,runs);assert.equal(s.versions.length,versions);assert.equal(task(s).observations.length,1);const once=s;s=syncAgent(s,id);assert.equal(s,once);
});
