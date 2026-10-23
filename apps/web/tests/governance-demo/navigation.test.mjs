import test from 'node:test';
import assert from 'node:assert/strict';
import {activityDestination} from '../../components/governance-demo/demo-navigation.mjs';
import {initialPipeline,runTask,createRepairTask,verifyRun} from '../../components/governance-demo/pipeline-model.mjs';
import {assessVersion,noteFinding,workCommand} from '../../components/governance-demo/lifecycle-model.mjs';

test('quality repair keeps the exact source and old report, then requires independent reassessment',()=>{
 let s=initialPipeline();const source=s.versions[0];s=assessVersion(s,source.id,'STD-0001');const original=JSON.stringify(s.assessments[0]);
 s=createRepairTask(s,source.id,'隔离负值后汇总',{assessmentId:s.assessments[0].id});const task=s.tasks.at(-1);
 assert.equal(task.sourceAssessmentId,s.assessments[0].id);assert.throws(()=>createRepairTask(s,source.id,'隔离负值后汇总',{assessmentId:s.assessments[0].id}),/已有修复/);assert.equal(task.parentRunId,source.runId);assert.notEqual(task.planId,source.planId);assert.equal(task.status,'待运行');
 s=runTask(s,task.id,'成功');const output=s.versions.at(-1);
 assert.deepEqual(output.inputs,[source.id]);assert.equal(s.assessments.length,1);
 s=assessVersion(s,output.id,'STD-0001');assert.equal(s.assessments.at(-1).passed,true);
 assert.equal(JSON.stringify(s.assessments[0]),original);assert.equal(s.assessments[0].passed,false);
});
test('message for finding opens its exact historical assessment, not latest selection',()=>{
 let s=initialPipeline();s=assessVersion(s,s.versions[0].id,'STD-0001');s=noteFinding(s,s.assessments[0].id,'保留原值');const event=s.events.at(-1);
 s=runTask(s,'TASK-0002','成功');s=assessVersion(s,s.versions.at(-1).id,'STD-0001');
 const dest=activityDestination(s,event);assert.equal(dest.route,'quality');assert.equal(dest.patch.selectedAssessmentId,'QA-0001');assert.equal(dest.patch.selectedVersionId,'VER-0001');
});
test('task, unknown observation, and work history follow their actual demand and run',()=>{
 let s=initialPipeline();s=assessVersion(s,s.versions[0].id,'STD-0001');s=createRepairTask(s,s.versions[0].id,'隔离负值后汇总',{assessmentId:s.assessments[0].id});const taskEvent=s.events.at(-1),task=s.tasks.at(-1);
 let dest=activityDestination(s,taskEvent);assert.equal(dest.route,'processing');assert.equal(dest.patch.selectedTaskId,task.id);assert.equal(dest.patch.taskDemandFilter,task.demandId);
 s=runTask(s,task.id,'待核实');s=verifyRun(s,s.runs.at(-1).id,'成功');dest=activityDestination(s,s.events.at(-1));assert.equal(dest.patch.selectedReportId,s.runs.at(-1).id);assert.equal(dest.route,'processing-report');
 s=workCommand(s,task.demandId,'分派',{assignee:'USR-02'});dest=activityDestination(s,s.events.at(-1));assert.equal(dest.patch.workDemandFocus,task.demandId);assert.equal(dest.views.work,'我的流程');
});
test('approval and transfer history preserve request and delivery identities',()=>{
 const s={requests:[{id:'REQ-DATA-1',versionId:'VER-1'}],requestDecisions:[{id:'DEC-1',requestId:'REQ-DATA-1'}],deliveries:[{id:'DEL-1',requestId:'REQ-DATA-1'}],transfers:[{id:'TX-1',deliveryId:'DEL-1'}],versions:[{id:'VER-1',demandId:'D-1'}],demands:[{id:'D-1'}]};
 let dest=activityDestination(s,{ref:'DEC-1'});assert.equal(dest.patch.selectedRequestId,'REQ-DATA-1');assert.equal(dest.views.work,'我的取数');
 dest=activityDestination(s,{ref:'TX-1'});assert.equal(dest.patch.selectedDeliveryId,'DEL-1');assert.equal(dest.patch.selectedVersionId,'VER-1');assert.equal(dest.views.publish,'交付记录');
});
