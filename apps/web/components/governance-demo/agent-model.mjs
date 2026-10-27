// Local synthetic commands. Business facts remain in the existing pipeline.
import {addDemand,confirmDemand,savePlan,latestPlan,createTask,runTask,createRepairTask,effectiveRun,verifyRun} from './pipeline-model.mjs';
import {activity,assessVersion,definitionPublished,workState} from './lifecycle-model.mjs';
const check=(ok,message)=>{if(!ok)throw Error(message)};
const now=()=>new Date().toLocaleString('zh-CN',{hour12:false});
const rows=(t,key)=>t[key]||[]; // Retained objects in this actual open browser keep their original facts.
export const agentKinds={plan:'需求转方案',quality:'质量诊断'};
export const agentSteps={plan:['查找可用来源与定义','登记需求并保存方案','创建汇聚任务并生成报告','汇总方案与成果'],quality:['对源版本独立评测','整理异常依据与修复建议','按批准模板生成新版本','独立复检并汇总报告']};
export const agentFilters=['全部','待确认','执行中','待修复确认','待重新确认','业务阻断','执行失败','待核实','已暂停','待验收','已退回','已完成','已停止'];
const find=(s,id)=>{const t=s.agentTasks.find(t=>t.id===id);check(t,'Agent 任务不存在');return t};
const replace=(s,t)=>({...s,agentTasks:s.agentTasks.map(x=>x.id===t.id?t:x),selectedAgentId:t.id});
const log=(s,t,message,result='成功')=>activity(replace(s,t),'Agent 演示',t.id,message,{route:'agent-task',result});
const ref=(label,id,route)=>({label,id,route});
const uniqueRefs=refs=>refs.filter((a,i)=>refs.findIndex(x=>x.id===a.id&&x.route===a.route)===i);
const uid=(t,prefix,key)=>t.id+'/'+prefix+String(rows(t,key).length+1).padStart(3,'0');
const sourceOf=(s,t)=>s.versions.find(v=>v.id===t.plan.versionId);
const boundPlan=(s,t)=>s.plans.find(p=>p.id===t.planId);
const taskDemand=(s,t)=>t.demandId||sourceOf(s,t)?.demandId;
export function agentBlockers(s,t){
 const p=t.plan,missing=[],v=sourceOf(s,t);
 if(!p.goal?.trim())missing.push('补充业务目标');
 if(!p.purpose?.trim())missing.push('补充成果用途');
 if(!p.acceptance?.trim())missing.push('补充验收条件');
 if(t.kind==='plan'&&!p.tables.length)missing.push('选择至少一张当前空间的来源表');
 if(t.kind==='plan'&&p.tables.some(x=>!p.availableTables.includes(x)))missing.push('来源表不属于当前空间');
 if(t.kind==='plan'&&!t.demandId&&s.demands.some(d=>d.title===p.goal?.trim()))missing.push('需求名称已存在，请修改业务目标');
 if(t.kind==='quality'&&!v)missing.push('选择当前空间的明确版本');
 if(t.kind==='quality'&&v&&p.purpose!==v.purpose)missing.push('当前诊断须沿用源版本用途：'+v.purpose);
 const std=s.definitions.find(d=>d.id===p.standardId&&d.type==='标准'&&definitionPublished(s,d));
 if(!std)missing.push('选择已发布质量标准');
 if(v&&std&&!v.fields.includes(std.field))missing.push('所选标准字段不在源版本中，请重新选择标准');
 return missing;
}
export function createAgent(s,kind,draft){
 check(kind in agentKinds,'未知演示能力');
 const t={id:'AGT-'+String(s.agentTasks.length+1).padStart(4,'0'),kind,status:'待确认',revision:1,approvalRevision:null,repairApproved:false,cursor:0,steps:[],artifacts:[],history:[],approvals:[],bindings:[],attempts:[],observations:[],reports:[],reviews:[],parentAgentId:draft.parentAgentId||null,plan:{goal:draft.goal||'',purpose:draft.purpose||'',tables:[...(draft.tables||[])],availableTables:[...(draft.availableTables||[])],versionId:draft.versionId||'',standardId:draft.standardId||'',rule:'隔离负值后汇总',followup:!!draft.followup,acceptance:draft.acceptance||'标准规则独立复检；保留异常记录与原始版本。'},createdAt:now()};
 t.status=agentBlockers(s,t).length?'待补充':'待确认';
 return log({...s,agentTasks:[...s.agentTasks,t]},t,'创建'+agentKinds[kind]+'演示计划 r1',t.status==='待补充'?'阻断':'成功');
}
export function editAgent(s,id,patch){
 const old=find(s,id);check(old.cursor===0&&!['已停止','已完成','已退回'].includes(old.status),'执行开始后保留计划，请发起关联新委托');
 const plan={...old.plan,goal:patch.goal??old.plan.goal,purpose:patch.purpose??old.plan.purpose,tables:patch.tables?[...patch.tables]:old.plan.tables,versionId:patch.versionId??old.plan.versionId,standardId:patch.standardId??old.plan.standardId,acceptance:patch.acceptance??old.plan.acceptance};
 const t={...old,plan,revision:old.revision+1,approvalRevision:null,history:[...rows(old,'history'),{revision:old.revision,plan:old.plan}],status:'待确认',issue:null};
 t.status=agentBlockers(s,t).length?'待补充':'待确认';return log(s,t,'计划修改为 r'+t.revision+'；旧确认失效');
}
function approval(t,decision,businessPlanId=null){return {...t,approvalRevision:t.revision,approvals:[...rows(t,'approvals'),{id:uid(t,'APR-','approvals'),decision,revision:t.revision,plan:{...t.plan,tables:[...t.plan.tables]},businessPlanId,actor:'USR-01',time:now()}]}}
export function approveAgent(s,id){
 const t=find(s,id);check(t.cursor===0&&['待确认','待补充','已暂停'].includes(t.status),'当前任务不能再次确认');check(!agentBlockers(s,t).length,agentBlockers(s,t).join('；'));
 return log(s,approval({...t,status:'执行中',issue:null},'确认计划'),'已确认计划 r'+t.revision+' 的动作与范围');
}
export function agentIssue(s,t){
 if(!['执行中','已暂停','业务阻断','待重新确认'].includes(t.status))return null;
 const dId=taskDemand(s,t),w=dId&&workState(s,dId);
 // A follow-up is a new demand using a historical source, never a reopened old demand.
 if(dId&&['已取消','已归档'].includes(w.state)&&(t.kind==='plan'||t.repairApproved||t.demandId||(t.issue?.code==='DEMAND_ENDED'&&t.cursor>=2))&&!(t.kind==='quality'&&t.plan.followup&&!t.demandId))return t.issue?.code==='DEMAND_ENDED'?t.issue:{code:'DEMAND_ENDED',reason:'关联需求 '+dId+' 已结束，后续业务动作被阻断。可停止任务，或以明确源版本发起后续委托。',demandId:dId};
 if(t.kind==='plan'&&t.cursor===2&&t.planId){
  const p=boundPlan(s,t),latest=latestPlan(s,t.demandId);
  if(!p)return {code:'PLAN_MISSING',reason:'明确绑定的业务方案缺失，不能执行。'};
  if(latest?.id!==t.reviewedLatestPlanId&&latest?.id!==t.planId)return {code:'PLAN_CHANGED',reason:'业务方案已从 '+t.planId+' 变更为 '+latest.id+'；请审阅差异并重新确认。',expected:t.planId,current:latest.id};
 }
 return null;
}
function observedRun(s,t){const a=rows(t,'attempts').filter(a=>a.runId&&a.outcome==='待核实').at(-1),r=a&&s.runs.find(r=>r.id===a.runId);return r&&effectiveRun(s,r)}
export function agentStatus(s,t){if(t.status==='待核实'){const r=observedRun(s,t);if(r&&['成功','失败'].includes(r.outcome))t={...t,status:r.outcome==='成功'?'执行中':'执行失败',cursor:r.outcome==='成功'?3:t.cursor}}const issue=agentIssue(s,t);return issue?(issue.code==='PLAN_CHANGED'?'待重新确认':'业务阻断'):t.status}
function appendAttempt(s,t,outcome,reason,refs=[],extra={}){
 const a={id:uid(t,'ATT-','attempts'),index:t.cursor,revision:t.revision,businessPlanId:t.planId||null,sourceVersionId:t.plan.versionId||null,outcome,reason,refs,actor:'USR-01',time:now(),...extra};
 return {...t,attempts:[...rows(t,'attempts'),a]};
}
function block(s,t,issue){
 const status=issue.code==='PLAN_CHANGED'?'待重新确认':'业务阻断';
 if(t.status===status&&JSON.stringify(t.issue)===JSON.stringify(issue))return s;
 const next=appendAttempt(s,{...t,status,issue},'阻断',issue.reason);
 return log(s,next,issue.reason,'阻断');
}
export function syncAgent(s,id){const t=find(s,id);if(t.status==='待核实'){const effective=observedRun(s,t);if(effective&&['成功','失败'].includes(effective.outcome))return verifyAgentRun(s,id,effective.outcome)}const issue=agentIssue(s,t);return issue?block(s,t,issue):s}
export function retryAgent(s,id){
 const t=find(s,id);check(['业务阻断','执行失败'].includes(t.status),'当前任务不需要恢复');
 const issue=agentIssue(s,{...t,status:'执行中'});if(issue)return block(s,t,issue);
 check(t.approvalRevision===t.revision,'先确认当前计划');check(!agentBlockers(s,t).length,agentBlockers(s,t).join('；'));
 return log(s,{...t,status:t.kind==='quality'&&t.cursor===2&&!t.repairApproved?'待修复确认':'执行中',issue:null},'已重新检查依赖，恢复到原步骤；下一次实际模拟运行将形成新尝试');
}
export function controlAgent(s,id,action){
 const t=find(s,id);if(action==='暂停'){check(t.status==='执行中','只有执行中任务可以暂停');return log(s,{...t,status:'已暂停'},'暂停后续步骤；已完成事实保留')}
 if(action==='继续'){check(t.status==='已暂停'&&t.approvalRevision===t.revision,'先确认当前计划');const issue=agentIssue(s,t);return issue?block(s,t,issue):log(s,{...t,status:'执行中'},'继续已确认计划')}
 check(action==='停止'&&!['已完成','已停止','待验收','已退回'].includes(t.status),'当前任务不能停止');check(t.status!=='待核实','结果未知时先核实原运行；不能把未知输出当作已撤销');
 return log(s,{...t,status:'已停止',issue:null},'停止后续步骤；不回滚已完成事实');
}
export function planDifference(s,t){
 const old=boundPlan(s,t),latest=latestPlan(s,t.demandId);if(!old||!latest)return [];
 return [['来源表','tables'],['处理规则','rule'],['交付方式','channel'],['验收条件','acceptance'],['元数据定义','metadata'],['处理说明','description'],['异常保障','emergency'],['引用定义','definitionId']].map(([label,key])=>({label,before:Array.isArray(old[key])?old[key].join('、'):old[key]||'—',after:Array.isArray(latest[key])?latest[key].join('、'):latest[key]||'—'})).filter(x=>x.before!==x.after);
}
export function reviewAgentPlan(s,id,choice){
 let t=find(s,id);check(agentStatus(s,t)==='待重新确认','当前没有待审阅的方案变化');check(['沿用原版','采用新版'].includes(choice),'请选择方案处理方式');
 const current=latestPlan(s,t.demandId);check(current,'最新方案缺失');
 const oldId=t.planId;
 if(choice==='采用新版')t={...t,revision:t.revision+1,history:[...rows(t,'history'),{revision:t.revision,plan:t.plan}],plan:{...t.plan,tables:[...current.tables],rule:current.rule,acceptance:current.acceptance},planId:current.id,operationTaskId:null};
 t=approval({...t,status:'执行中',issue:null,reviewedLatestPlanId:current.id},choice,t.planId);
 t={...t,bindings:[...rows(t,'bindings'),{id:uid(t,'BIND-','bindings'),revision:t.revision,planId:t.planId,decision:choice,previousPlanId:oldId,observedLatestPlanId:current.id,time:now()}],artifacts:choice==='采用新版'?[...t.artifacts,ref('重新确认方案',current.id,'plan')]:t.artifacts};
 return log(s,t,choice+'：执行固定绑定 '+t.planId+'，已明确审阅 '+current.id+'；后续变更仍需重新确认');
}
export function approveRepair(s,id){
 const t=find(s,id),v=sourceOf(s,t);check(t.status==='待修复确认'&&t.approvalRevision===t.revision,'请先完成诊断并审阅修复范围');
 check(v,'源版本缺失');
 if(['已取消','已归档'].includes(workState(s,v.demandId).state)&&!t.plan.followup)return block(s,t,{code:'DEMAND_ENDED',reason:'原需求已结束；请以 '+v.id+' 发起明确的后续委托，保留旧流程。',demandId:v.demandId});
 check(t.plan.rule==='隔离负值后汇总','修复模板不支持');
 return log(s,approval({...t,repairApproved:true,status:'执行中',issue:null},'确认修复',v.planId),'已确认以 '+v.id+' 为输入执行隔离模板并独立复检'+(t.plan.followup?'；将创建新的后续需求':''));
}
function reportFor(s,t){
 const q=s.assessments.find(q=>q.id===t.resultAssessmentId||(!t.resultAssessmentId&&q.id===t.sourceAssessmentId)),v=s.versions.find(v=>v.id===t.versionId)||sourceOf(s,t);
 const checks=[{label:'成果与依据已形成',ok:t.artifacts.length>0},{label:'业务目标与验收条件已记录',ok:!!t.plan.goal&&!!t.plan.acceptance},{label:t.kind==='quality'?'所选标准独立评测通过':'明确方案、任务与报告已关联',ok:t.kind==='quality'?!!q?.passed:!!t.planId&&!!t.versionId}];
 return {id:uid(t,'REPORT-','reports'),taskId:t.id,createdAt:now(),revision:t.revision,goal:t.plan.goal,purpose:t.plan.purpose,acceptance:t.plan.acceptance,standardId:t.plan.standardId,businessPlanId:t.planId||null,demandId:t.demandId||v?.demandId,sourceVersionId:t.plan.versionId||null,outputVersionId:v?.id,sourceAssessmentId:t.sourceAssessmentId||null,resultAssessmentId:t.resultAssessmentId||null,artifacts:uniqueRefs(t.artifacts).map(x=>({...x})),attemptIds:rows(t,'attempts').map(a=>a.id),observationIds:rows(t,'observations').map(o=>o.id),checks,steps:t.steps.map(x=>({...x})),approvalIds:rows(t,'approvals').map(a=>a.id),eventIds:s.events.filter(e=>e.ref===t.id).map(e=>e.id),remaining:['权利、合规与合约依据','独立认证与发布检查','取数审批、交付与业务验收（按实际需要）']};
}
function finish(s,t){const report=reportFor(s,t);return {...t,status:'待验收',reports:[...rows(t,'reports'),report],issue:null}}
export function prepareAgentReport(s,id){
 const t=find(s,id);check(t.status==='已完成'&&!rows(t,'reports').length&&t.steps.length>0,'当前无需重新整理历史成果');
 return log(s,finish(s,t),'根据已保留的历史成果追加报告，提交 Agent 成果审阅；未补造旧审批或业务验收');
}
export function reviewAgentResult(s,id,decision,note){
 const t=find(s,id),report=rows(t,'reports').at(-1);check(t.status==='待验收'&&report,'请先形成报告并提交成果审阅');check(['接受成果','退回重办'].includes(decision),'请选择审阅结论');check(note?.trim(),'请填写审阅说明');
 if(decision==='接受成果')check(report.checks.every(c=>c.ok),'报告仍有未满足的验收条件，不能接受成果');
 const r={id:uid(t,'REVIEW-','reviews'),reportId:report.id,revision:report.revision,decision,note:note.trim(),actor:'USR-01',time:now()};
 return log(s,{...t,status:decision==='接受成果'?'已完成':'已退回',reviews:[...rows(t,'reviews'),r]},decision+'：'+r.note);
}
export function forkAgent(s,id,followup=false){
 const t=find(s,id);check(['已退回','已停止','业务阻断','已完成'].includes(agentStatus(s,t)),'先停止、处理阻断或审阅原任务，再发起关联委托');
 check(!followup||t.kind==='quality','后续需求需要明确源版本');
 let goal=t.plan.goal+' · '+(followup?'后续委托':'重办')+' '+(s.agentTasks.filter(x=>x.parentAgentId===id).length+1);
 while(s.demands.some(d=>d.title===goal))goal+=' 新';
 const next=createAgent(s,t.kind,{...t.plan,goal,parentAgentId:id,followup:followup||t.plan.followup});return log(next,find(next,next.selectedAgentId),'关联原委托 '+id+'；新计划、新确认和后续成果分别保留');
}
function completeStep(s,t,result,refs){return {...t,cursor:t.cursor+1,operationTaskId:null,steps:[...t.steps,{index:t.cursor,title:agentSteps[t.kind][t.cursor],result,refs,revision:t.revision}],artifacts:uniqueRefs([...t.artifacts,...refs])}}
function runOperation(s,t,outcome){
 let next=s,task=s.tasks.find(x=>x.id===t.operationTaskId);
 if(!task){
  if(t.kind==='plan'){check(t.planId,'未绑定明确方案');next=createTask(next,t.demandId,'汇聚',t.planId)}
  else{
   const v=sourceOf(next,t);let demandId=v.demandId;
   if(t.plan.followup){next=addDemand(next,{title:t.plan.goal,background:'以 '+v.id+' 作为固定输入发起新委托；旧需求保持结束状态。',owner:'治理工作组',purpose:v.purpose,priority:'中',dueDate:'2026-10-30'});demandId=next.demands.at(-1).id;next=confirmDemand(next,demandId)}
   next=createRepairTask(next,v.id,t.plan.rule,{demandId,assessmentId:t.sourceAssessmentId,acceptance:t.plan.acceptance});t={...t,demandId,planId:next.plans.at(-1).id};
  }
  task=next.tasks.at(-1);t={...t,operationTaskId:task.id};
 }
 check(task.planId===t.planId,'运行任务与已确认方案不一致');
 next=runTask(next,task.id,outcome);const run=next.runs.at(-1),refs=[ref(t.kind==='plan'?'汇聚任务':'修复任务',task.id,t.kind==='plan'?'ingestion':'processing'),ref('运行报告',run.id,t.kind==='plan'?'ingestion-report':'processing-report')];
 if(t.kind==='quality')refs.push(ref('修复方案',task.planId,'plan'));
 if(t.plan.followup)refs.push(ref('后续需求',task.demandId,'plan'));
 t=appendAttempt(next,t,outcome,run.reason,refs,{runId:run.id,taskId:task.id,businessPlanId:task.planId});
 if(outcome!=='成功')return log(next,{...t,status:outcome==='待核实'?'待核实':'执行失败',issue:{code:outcome==='待核实'?'UNKNOWN':'RUN_FAILED',reason:run.reason},artifacts:uniqueRefs([...t.artifacts,...refs])},run.id+' · '+run.reason,'阻断');
 return runSuccess(next,t,run,refs);
}
function runSuccess(s,t,run,refs){
 const v=s.versions.find(v=>v.runId===run.id);check(v,'成功运行缺少可回查成果版本');
 refs=[...refs,ref(t.kind==='plan'?'RAW 版本':'新版本',v.id,'asset')];
 const result=t.kind==='plan'?'固定方案 '+run.planId+'，汇聚 '+v.rows+' 条，生成 '+v.id+'。':'固定输入 '+t.plan.versionId+'，隔离 '+v.isolated+' 条，生成 '+v.id+'（'+v.rows+' 条）。';
 t=completeStep(s,{...t,status:'执行中',issue:null,versionId:v.id,demandId:v.demandId},result,refs);
 return log(s,t,agentSteps[t.kind][t.cursor-1]+' · '+result);
}
export function verifyAgentRun(s,id,resolution){
 const t=find(s,id);check(t.status==='待核实','当前没有待核实运行');const a=rows(t,'attempts').filter(a=>a.runId&&a.outcome==='待核实').at(-1);check(a,'未知运行依据缺失，请查看关联业务报告');
 const original=s.runs.find(r=>r.id===a.runId),known=original&&effectiveRun(s,original);check(known,'原运行不存在');check(known.outcome==='待核实'||known.outcome===resolution,'原运行已有明确核实结论，请刷新任务记录');
 let next=known.outcome==='待核实'?verifyRun(s,a.runId,resolution):s;const run=effectiveRun(next,next.runs.find(r=>r.id===a.runId));
 const observation={id:uid(t,'OBS-','observations'),attemptId:a.id,runId:a.runId,outcome:resolution,time:now(),actor:'USR-01',reason:'核实原运行：'+resolution};
 const updated={...t,observations:[...rows(t,'observations'),observation]};
 next=log(next,updated,observation.reason+'；保留首次未知观察');
 return resolution==='成功'?runSuccess(next,updated,run,a.refs):log(next,{...updated,status:'执行失败',issue:{code:'VERIFIED_FAILED',reason:'已核实原运行未产出；恢复后可创建新的运行尝试。'}},'原运行未产出，可显式恢复后重试','阻断');
}
export function advanceAgent(s,id,outcome='成功'){
 let t=find(s,id);check(t.status==='执行中'&&t.approvalRevision===t.revision,'请先确认当前计划并继续执行');
 const issue=agentIssue(s,t);if(issue)return block(s,t,issue);
 const missing=agentBlockers(s,t);if(missing.length)return block(s,t,{code:'DEPENDENCY',reason:missing.join('；')});
 check(['成功','失败','待核实'].includes(outcome),'请选择演示运行情境');
 try{
  if(t.cursor===2){check(t.kind!=='quality'||t.repairApproved,'修复需要单独确认');return runOperation(s,t,outcome)}
  let next=s,result='',refs=[];const n=t.cursor;
  if(t.kind==='plan'){
   if(n===0)result='当前空间来源：'+t.plan.tables.join('、')+'；检验依据 '+t.plan.standardId+'。模板填充，不进行模型推理。';
   if(n===1){next=addDemand(next,{title:t.plan.goal,background:'Agent 演示委托：'+t.plan.goal,owner:'业务分析组',purpose:t.plan.purpose,priority:'中',dueDate:'2026-10-30'});const d=next.demands.at(-1);next=confirmDemand(next,d.id);next=savePlan(next,d.id,{tables:t.plan.tables,schedule:'手动触发',rule:t.plan.rule,channel:'受控下载',description:'按明确输入汇聚，后续按批准模板隔离负值。',metadata:'enterprise_id / period / usage_kwh；单位 kWh。',emergency:'失败保留尝试，未知先核实，治理分别检查。',acceptance:t.plan.acceptance});const p=next.plans.at(-1);t={...t,demandId:d.id,planId:p.id,reviewedLatestPlanId:p.id,bindings:[...rows(t,'bindings'),{id:uid(t,'BIND-','bindings'),revision:t.revision,planId:p.id,decision:'固定生成方案',time:now()}]};refs=[ref('需求',d.id,'plan'),ref('开发方案',p.id,'plan')];result='已登记 '+d.id+'，固定绑定 '+p.id+'；后续执行不自动选择最新方案。'}
   if(n===3)result='方案、任务与报告已关联，提交成果审阅。后续质量、治理与业务验收分别办理。';
  }else{
   const v=sourceOf(next,t);
   if(n===0){next=assessVersion(next,v.id,t.plan.standardId);const q=next.assessments.at(-1);t={...t,sourceAssessmentId:q.id};refs=[ref('源版本评测',q.id,'quality')];result=q.id+'：'+q.rows+' 条，'+q.failures+' 条未满足所选标准。'}
   if(n===1){const q=next.assessments.find(q=>q.id===t.sourceAssessmentId);result=q.failures?'事实：规则命中 '+q.failures+' 条。原因待验证；建议隔离负值、保留原值，再独立复检。':'所选标准未发现异常；无需修复。其他业务规则与认证尚未检查。';if(q.failures)t={...t,status:'待修复确认'}}
   if(n===3){next=assessVersion(next,t.versionId,t.plan.standardId);const q=next.assessments.at(-1);t={...t,resultAssessmentId:q.id};refs=[ref('独立复检',q.id,'quality')];result=q.id+'：'+(q.passed?'所选标准通过':'仍有异常，成果审阅将阻断接受')+'；未自动认证、发布或交付。'}
  }
  t=appendAttempt(next,t,'成功',result,refs);t=completeStep(next,t,result,refs);
  next=log(next,t,agentSteps[t.kind][n]+' · '+result);
  if(t.cursor===4||(t.kind==='quality'&&n===1&&next.assessments.find(q=>q.id===t.sourceAssessmentId)?.passed))return log(next,finish(next,t),'执行步骤结束，已形成报告，等待 Agent 成果审阅');
  return next;
 }catch(e){return block(s,t,{code:'COMMAND_BLOCKED',reason:e.message})}
}
export function resolveAgentContext(s,page){
 const task=page==='agent-task'?s.agentTasks.find(t=>t.id===s.selectedAgentId):null;
 if(page==='agent-task'){
  const source=task&&sourceOf(s,task),output=task&&s.versions.find(v=>v.id===task.versionId);
  return {task,source,output,primary:output||source||null,versions:[source,output].filter((v,i,a)=>v&&a.findIndex(x=>x?.id===v.id)===i),demandId:task&&taskDemand(s,task),explicit:true};
 }
 if(page==='plan'){const d=s.demands.find(d=>d.id===s.selectedDemandId);const versions=s.versions.filter(v=>v.demandId===d?.id);return {primary:versions.at(-1)||null,versions,demandId:d?.id,explicit:true}}
 if(['ingestion-report','processing-report','publish-report'].includes(page)){const r=s.runs.find(r=>r.id===s.selectedReportId),v=s.versions.find(v=>v.runId===r?.id);return {primary:v||null,versions:v?[v]:[],demandId:r?.demandId,explicit:true}}
 const primary=['quality','quality-list','asset','assets','metadata','security','publish'].includes(page)?s.versions.find(v=>v.id===s.selectedVersionId)||s.versions.at(-1):null;
 return {primary,versions:primary?[primary]:s.versions,demandId:primary?.demandId,explicit:!!primary};
}
