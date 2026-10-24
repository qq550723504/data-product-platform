// Pure in-memory presentation model. No service, storage or engine integration.
import {lifecycleInitial,activity,captureRun,definitionPublished,workState} from './lifecycle-model.mjs';
export const stages=['汇聚','处理','发布'];
export const tableRows={'企业基础信息':6,'租赁记录':12,'用电记录':14};
const requireThat=(ok,message)=>{if(!ok)throw new Error(message)};
const id=(prefix,list)=>prefix+String(list.length+1).padStart(4,'0');
function event(s,kind,ref,message){const run=s.runs.find(r=>r.id===ref);return activity(s,kind,ref,message,{route:run?(run.stage==='汇聚'?'ingestion-report':run.stage==='处理'?'processing-report':'publish-report'):'plan',versionId:s.versions.find(v=>v.runId===ref)?.id,cost:kind==='运行记录'?1:0,unit:'次模拟运行',result:run&&run.outcome!=='成功'?'阻断':'成功'})}
export function emptyPipeline(){return {...lifecycleInitial(),demands:[],plans:[],tasks:[],runs:[],observations:[],events:[],agentTasks:[],selectedAgentId:null,sequence:0,selectedDemandId:null,selectedReportId:null}}
export function addDemand(s,draft){
 const clean={title:draft.title?.trim(),background:draft.background?.trim(),owner:draft.owner,purpose:draft.purpose?.trim(),priority:draft.priority,dueDate:draft.dueDate};
 requireThat(clean.title&&clean.background&&clean.owner&&clean.purpose&&clean.dueDate,'请填写需求名称、业务背景、负责人、用途和预计完成日期');
 requireThat(['高','中','低'].includes(clean.priority),'请选择开发优先级');
 requireThat(!s.demands.some(d=>d.title===clean.title),'需求名称已存在，请使用其他名称');
 const demand={...clean,id:id('REQ-',s.demands),status:'草稿'};
 return event({...s,demands:[...s.demands,demand],selectedDemandId:demand.id},'需求创建',demand.id,'创建需求草稿');
}
export function editDemand(s,demandId,draft){
 const old=s.demands.find(d=>d.id===demandId);requireThat(old?.status==='草稿','已确认需求保留原确认内容，请新建需求表达变更');
 const checked=addDemand({...s,demands:s.demands.filter(d=>d.id!==demandId)},draft).demands.at(-1);
 return event({...s,demands:s.demands.map(d=>d.id===demandId?{...checked,id:demandId}:d)},'需求编辑',demandId,'更新需求草稿');
}
export function confirmDemand(s,demandId){const d=s.demands.find(d=>d.id===demandId);requireThat(d?.status==='草稿','只有需求草稿可以确认开发');return event({...s,demands:s.demands.map(d=>d.id===demandId?{...d,status:'已确认'}:d)},'开发确认',demandId,'确认开发，允许编制方案')}
export function latestPlan(s,demandId){return s.plans.filter(p=>p.demandId===demandId).at(-1)}
export function savePlan(s,demandId,draft){
 requireThat(!['已取消','已归档'].includes(workState(s,demandId).state),'该需求已结束，不能继续编制方案');
 requireThat(s.demands.find(d=>d.id===demandId)?.status==='已确认','先确认需求，再编制方案');
 requireThat(Array.isArray(draft.tables)&&draft.tables.length>0&&draft.tables.every(t=>t in tableRows),'至少选择一张演示来源表');
 requireThat(['手动触发','每日一次'].includes(draft.schedule),'请选择调度频率');
 requireThat(['隔离负值后汇总','保留全部记录'].includes(draft.rule),'请选择处理规则');
 requireThat(['受控下载','接口查询','数据推送'].includes(draft.channel),'请选择成果提供方式');
 requireThat(draft.description?.trim()&&draft.metadata?.trim()&&draft.emergency?.trim()&&draft.acceptance?.trim(),'请补齐元数据定义、处理说明、异常保障和验收条件');
 if(draft.definitionId)requireThat(s.definitions.some(d=>d.id===draft.definitionId&&d.type!=='标准'&&definitionPublished(s,d)),'请选择已发布的模型或指标定义');
 const plan={id:id('PLAN-',s.plans),demandId,revision:(latestPlan(s,demandId)?.revision||0)+1,tables:[...new Set(draft.tables)],schedule:draft.schedule,rule:draft.rule,channel:draft.channel,definitionId:draft.definitionId||null,description:draft.description.trim(),metadata:draft.metadata.trim(),emergency:draft.emergency.trim(),acceptance:draft.acceptance.trim()};
 return event({...s,plans:[...s.plans,plan]},'方案保存',plan.id,'保存方案 r'+plan.revision+'，既有任务仍绑定原方案版本');
}
export function effectiveRun(s,run){const observation=s.observations.filter(o=>o.runId===run.id).at(-1);return observation?{...run,...observation,id:run.id,originalOutcome:run.outcome}:run}
export function createRepairTask(s,versionId,rule,options={}){
 const v=s.versions.find(v=>v.id===versionId);requireThat(v,'修复源版本不存在');
 requireThat(rule==='隔离负值后汇总','当前演示只支持隔离负值模板');
 const assessment=s.assessments.find(q=>q.id===options.assessmentId);requireThat(assessment?.versionId===v.id&&!assessment.passed,'请选择源版本未通过的明确评测');requireThat(s.definitions.find(d=>d.id===assessment.standardId)?.rule==='非负','该评测不适用负值隔离模板');requireThat(!s.tasks.some(t=>t.sourceAssessmentId===assessment.id),'此评测已有修复任务，请打开已有任务');
 const original=s.plans.find(p=>p.id===v.planId);requireThat(original,'源版本的方案依据缺失');
 const demandId=options.demandId||v.demandId;const target=s.demands.find(d=>d.id===demandId);
 requireThat(target?.purpose===v.purpose,'后续需求须明确沿用源版本用途；用途变化需独立治理依据');
 let next=savePlan(s,demandId,{...original,rule,acceptance:options.acceptance||original.acceptance,description:'基于 '+v.id+' 隔离负值后重新处理；保留原始版本。'});
 const plan=next.plans.at(-1);const task={id:id('TASK-',next.tasks),demandId,planId:plan.id,stage:'处理',status:'待运行',parentRunId:v.runId,sourceAssessmentId:assessment.id};
 return event({...next,tasks:[...next.tasks,task]},'任务创建',task.id,'修复任务固定输入 '+v.id+' / '+v.runId+'，来源评测 '+assessment.id);
}
export function createTask(s,demandId,stage,planId=null){
 requireThat(!['已取消','已归档'].includes(workState(s,demandId).state),'该需求已结束，不能继续创建任务');
 const plan=planId?s.plans.find(p=>p.id===planId&&p.demandId===demandId):latestPlan(s,demandId);requireThat(plan,planId?'明确绑定的方案不存在或不属于该需求':'请先保存开发方案');requireThat(stages.includes(stage),'未知任务阶段');
 requireThat(!s.tasks.some(t=>t.planId===plan.id&&t.stage===stage),'此方案版本已有该阶段任务，请进入任务查看或重试');
 const previous=stages[stages.indexOf(stage)-1];
 const parent=previous?s.runs.map(r=>effectiveRun(s,r)).filter(r=>r.planId===plan.id&&r.stage===previous&&r.outcome==='成功').at(-1):null;
 requireThat(!previous||parent,'请先完成当前方案版本的'+previous+'任务');
 const task={id:id('TASK-',s.tasks),demandId,planId:plan.id,stage,status:'待运行',parentRunId:parent?.id||null};
 return event({...s,tasks:[...s.tasks,task]},'任务创建',task.id,stage+'任务绑定 '+plan.id+(parent?' / '+parent.id:''));
}
function runPayload(s,task,runId){
 const plan=s.plans.find(p=>p.id===task.planId);const parent=task.parentRunId?effectiveRun(s,s.runs.find(r=>r.id===task.parentRunId)):null;
 const inputRows=parent?parent.outputRows:plan.tables.reduce((n,t)=>n+tableRows[t],0);
 const inputAnomalies=parent?parent.anomalies:(plan.tables.includes('用电记录')?2:0);
 const isolated=task.stage==='处理'&&plan.rule==='隔离负值后汇总'?inputAnomalies:0;
 return {inputRows,outputRows:inputRows-isolated,isolated,anomalies:inputAnomalies-isolated,output:task.stage==='发布'?'发布材料清单 · '+runId:(task.stage==='汇聚'?'RAW':'CURATED')+' 演示输出 · '+runId};
}
export function runTask(s,taskId,outcome){
 const task=s.tasks.find(t=>t.id===taskId);requireThat(task,'任务不存在');requireThat(task.status!=='待核实','原运行结果待核实，请先核实后再决定是否重试');requireThat(['成功','失败','待核实'].includes(outcome),'请选择运行情境');
 requireThat(!['已取消','已归档'].includes(workState(s,task.demandId).state),'该需求已结束，不能继续运行');
 const runId=id('RUN-',s.runs);const payload=runPayload(s,task,runId);
 const run={id:runId,taskId:task.id,demandId:task.demandId,planId:task.planId,stage:task.stage,attempt:s.runs.filter(r=>r.taskId===task.id).length+1,outcome,step:s.sequence+1,inputRows:payload.inputRows,...(outcome==='成功'?payload:{outputRows:null,isolated:null,anomalies:null,output:null}),reason:outcome==='成功'?'本地合成样例运行完成':outcome==='失败'?'模拟执行失败，未形成可用输出':'模拟响应未确认，输出状态未知'};
 return event(captureRun({...s,runs:[...s.runs,run],tasks:s.tasks.map(t=>t.id===taskId?{...t,status:outcome==='成功'?'已完成':outcome}:t),selectedReportId:run.id},run),'运行记录',run.id,task.stage+'第'+run.attempt+'次运行：'+outcome);
}
export function verifyRun(s,runId,resolution){
 const run=s.runs.find(r=>r.id===runId);requireThat(run&&effectiveRun(s,run).outcome==='待核实','该运行不需要再次核实');requireThat(['成功','失败'].includes(resolution),'请选择核实结果');
 const task=s.tasks.find(t=>t.id===run.taskId);const observation={id:id('OBS-',s.observations),runId,outcome:resolution,step:s.sequence+1,...(resolution==='成功'?runPayload(s,task,run.id):{outputRows:null,output:null,isolated:null,anomalies:null}),reason:resolution==='成功'?'人工模拟核实：确认原运行已完成':'人工模拟核实：确认原运行未产出，可以重试'};
 return event(captureRun({...s,observations:[...s.observations,observation],tasks:s.tasks.map(t=>t.id===task.id?{...t,status:resolution==='成功'?'已完成':'失败'}:t)},{...run,...observation,id:run.id}),'核实追加',observation.id,'为 '+run.id+' 追加核实观察，保留首次结果');
}
export function initialPipeline(){
 let s=addDemand(emptyPipeline(),{title:'企业经营活跃度分析',background:'整合企业、租赁和用电信息，形成经营分析成果。',owner:'业务分析组',purpose:'企业经营分析',priority:'中',dueDate:'2026-10-30'});
 s=confirmDemand(s,s.demands[0].id);
 s=savePlan(s,s.demands[0].id,{tables:['企业基础信息','租赁记录','用电记录'],schedule:'手动触发',rule:'隔离负值后汇总',channel:'受控下载',description:'汇聚固定来源，隔离负值，形成经营分析材料。',metadata:'enterprise_id 为企业标识；period 为统计期间；usage_kwh 单位为 kWh。',emergency:'失败先检查报告；待核实先核对原运行。',acceptance:'独立质量评测、权利依据、适用认证和发布检查分别确认。'});
 s=createTask(s,s.demands[0].id,'汇聚');s=runTask(s,s.tasks[0].id,'成功');s=createTask(s,s.demands[0].id,'处理');
 return {...s,selectedReportId:null};
}
