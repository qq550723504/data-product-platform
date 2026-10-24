// Resolve local subjects from the shared pipeline; never infer Core identities.
export function activityDestination(s,event){
 const find=(key,id)=>s[key]?.find(x=>x.id===id);
 const finding=find('findings',event.ref),assessment=find('assessments',finding?.assessmentId||event.ref);
 const observation=find('observations',event.ref),run=find('runs',observation?.runId||event.ref);
 const transfer=find('transfers',event.ref),delivery=find('deliveries',transfer?.deliveryId||event.ref);
 const decision=find('requestDecisions',event.ref),request=find('requests',delivery?.requestId||decision?.requestId||event.ref);
 const work=find('work',event.ref),task=find('tasks',run?.taskId||event.ref),plan=find('plans',task?.planId||event.ref);
 const version=find('versions',assessment?.versionId||request?.versionId||work?.versionId||event.versionId);
 const demand=find('demands',work?.demandId||task?.demandId||plan?.demandId||version?.demandId||event.ref);
 const publication=find('definitionPublications',event.ref),definition=find('definitions',publication?.definitionId||event.ref);
 const patch={},views={};let route=event.route||'my-work';
 if(version)patch.selectedVersionId=version.id;
 if(demand)patch.selectedDemandId=demand.id;
 if(assessment){patch.selectedAssessmentId=assessment.id;patch.selectedStandardId=assessment.standardId;route='quality'}
 if(run){patch.selectedReportId=run.id;patch.reportDemandFilter=run.demandId;route=run.stage==='汇聚'?'ingestion-report':run.stage==='处理'?'processing-report':'publish-report'}
 else if(task){patch.selectedTaskId=task.id;patch.taskDemandFilter=task.demandId;route=task.stage==='汇聚'?'ingestion':task.stage==='处理'?'processing':'production-tasks'}
 else if(plan)views.pipeline='方案编制';else if(demand)views.pipeline='需求清单';
 if(work){patch.workDemandFocus=work.demandId;views.work='我的流程';route='my-work'}
 if(request){patch.selectedRequestId=request.id;views.publish='取数与交付';views.work='我的取数';route=delivery?'publish':'my-work'}
 if(delivery){patch.selectedDeliveryId=delivery.id;views.publish='交付记录'}
 if(definition){if(definition.type==='标准'){patch.selectedStandardId=definition.id;views.standard='标准检验';route='standards'}else route='model'}
 return {patch,route,views};
}
