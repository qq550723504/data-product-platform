import React,{useState} from 'react';
import {DemoPage,SafetyDemo} from './DemoPages';
import {DemandWorkspace,PipelineReport} from './PipelineWorkspace';
import {ArrowRight,FileText,Clock,ShieldCheck} from '@phosphor-icons/react';

export const menuGroups={
 '工作台':[['home','治理总览'],['agent-workbench','Agent 工作台'],['framework','架构搭建流程'],['development-flow','数据开发流程'],['improvement-flow','沉淀提升流程'],['my-work','我的工作']],
 '数据开发中心':[['develop','开发概览'],['plan','开发方案'],['ingestion','数据汇聚'],['processing','数据处理'],['publish','数据发布'],['business-receipts','业务签收'],['business-acceptances','业务验收'],['business-archives','验收归档'],['toolbox','交付工具箱'],['support','技术支持']],
 '数据管理中心':[['manage','管理概览'],['research','调研规划'],['architecture','系统架构'],['model','数据模型'],['quality-list','数据质量'],['metadata','元数据管理'],['security','数据安全'],['assets','数据资产'],['standards','标准建设'],['knowledge','知识沉淀']]
};
const stages={
 framework:[['调研规划','梳理现状、职责与治理目标','research','可交互演示'],['架构设计','明确技术架构、主题域与仓库分层','architecture','可交互演示'],['环境搭建','登记存储、引擎和运行环境','support','可交互演示'],['数据建模','定义业务模型、维度与指标','model','可交互演示']],
 'development-flow':[['开发方案','确定业务需求、输入与验收条件','plan','可交互演示'],['数据汇聚','登记来源并查看汇聚任务','ingestion','可交互演示'],['数据处理','查看输入、输出与异常处置','processing','可交互演示'],['数据发布','查看发布依据与当前交付资格','publish','可交互演示']],
 'improvement-flow':[['数据监测','跟踪质量问题与来源变化','quality-list','可交互演示'],['数据资产','查找资产及版本依据','assets','可交互演示'],['标准建设','统一数据元、编码与检验规则','standards','可交互演示'],['知识沉淀','归档治理案例与方法文档','knowledge','可交互演示']]
};
export const sectionDefs={
 'business-receipts':['数据开发中心','业务签收','登记收到情况及签收依据。'],
 'business-acceptances':['数据开发中心','业务验收','按冻结标准核对明确提交。'],
 'business-archives':['数据开发中心','验收归档','保留通过决定与完整演示档案。'],
 'entity-review':['数据管理中心','实体审核','逐项核对并保留人工决定。'],
 'source-admissions':['数据开发中心','来源登记','登记来源用途，不建立真实连接。'],
 batches:['数据开发中心','汇聚批次','追溯同一演示流水线的汇聚尝试。'],
 'agent-workbench':['工作台','Agent 工作台','按目标提出计划，确认后保存执行意图；实际执行保持关闭。'],
 'agent-task':['工作台','Agent 任务详情','查看冻结计划、确认记录与未派发的执行意图。'],
 'production-tasks':['数据开发中心','发布准备任务','按当前方案汇总发布准备材料；不执行真实发布。'],
 framework:['工作台','架构搭建流程','从现状调研到数据建模，组织治理工作的准备阶段。'],
 'development-flow':['工作台','数据开发流程','连接需求、汇聚、加工与发布，沿业务路径跟踪工作。'],
 'improvement-flow':['工作台','沉淀提升流程','将质量处置与数据资产转化为可复用的标准和经验。'],
 'my-work':['工作台','我的工作','集中查看工作空间待办、流程与取数记录。'],
 profile:['工作台','个人资料','管理演示资料与工作空间展示信息。'],
 messages:['工作台','我的消息','集中查看任务提醒与关联事项。'],
 plan:['数据开发中心','开发方案','把业务目标、来源范围与交付条件放在同一份方案中。'],
 toolbox:['数据开发中心','交付工具箱','统一组织交付所需的模板与辅助工具。',['交付检查清单','开发辅助','方案模板']],
 support:['数据开发中心','技术支持','登记和观察承载数据生产的技术环境。',['存储管理','引擎和中间件','服务器管理']],
 research:['数据管理中心','调研规划','围绕现状、组织、制度与治理计划建立工作底稿。',['现状调研','组织职责','制度规范','治理计划']],
 architecture:['数据管理中心','系统架构','梳理技术架构、管理架构与主题域。',['技术架构','管理架构','数据仓库','主题域管理']],
 model:['数据管理中心','数据模型','用一致的业务定义连接实体、维度与指标。'],
 security:['数据管理中心','数据安全','集中查看使用依据、安全关注与操作留痕。'],
 standards:['数据管理中心','标准建设','将业务口径沉淀为可查阅、可检验的数据标准。'],
 knowledge:['数据管理中心','知识沉淀','归档可复用的治理案例、知识和工作方法。',['内容导入','内容导出','案例集','知识文档']],
 'asset-map':['数据管理中心','数据地图','按主题域呈现资产分布和关联。'],
 cost:['数据管理中心','成本控制','按活动和资产查看成本归集与分配。'],
 value:['数据管理中心','价值评估','定义资产使用价值的评估口径。'],
 'metadata-monitor':['数据管理中心','元数据监控','跟踪目录观察、结构变化和监控任务。'],
 'metadata-discovery':['数据管理中心','元数据探查','探查来源结构及数据元信息。'],
 'quality-monitor':['数据管理中心','质量监控','查看质量监控任务与规则覆盖。'],
 'ingestion-report':['数据开发中心','汇聚报告','按需求和任务查看每次汇聚运行的结果。'],
 'processing-report':['数据开发中心','处理报告','查看每次处理的输入、输出、规则和结果。'],
 'processing-services':['数据开发中心','汇总与服务定制','管理数据汇总方案和服务定制。'],
 destinations:['数据开发中心','数据去向','登记发布目的地与交付方式。'],
 'api-publish':['数据开发中心','推送与接口发布','管理数据推送和接口发布配置。'],
 'publish-report':['数据开发中心','发布准备报告','按需求追溯发布准备运行与材料，不执行实际发布。']
};
const parentKeys={'entity-review':'model','source-admissions':'ingestion',batches:'ingestion','production-tasks':'publish','asset-map':'assets',cost:'assets',value:'assets','metadata-monitor':'metadata','metadata-discovery':'metadata','quality-monitor':'quality-list','ingestion-report':'ingestion','processing-report':'processing','processing-services':'processing',destinations:'publish','api-publish':'publish','publish-report':'publish',profile:'my-work',messages:'my-work'};
export function parentSection(key){return key==='agent-task'?'agent-workbench':parentKeys[key]||key}
export const scopes={
 assets:[['资产目录','assets',true],['数据地图','asset-map'],['成本控制','cost'],['价值评估','value']],
 metadata:[['检索与血缘','metadata',true],['元数据探查','metadata-discovery'],['元数据监控','metadata-monitor']],
 ingestion:[['来源与任务','ingestion',true],['汇聚报告','ingestion-report']],
 processing:[['处理任务','processing',true],['汇总与服务定制','processing-services'],['处理报告','processing-report']],
 publish:[['发布与交付','publish',true],['数据去向','destinations'],['推送与接口发布','api-publish'],['发布报告','publish-report']],
 'quality-list':[['问题处置','quality-list',true],['监控任务与规则','quality-monitor']]
};
export function ScopeLinks({scope,navigate}){return <div className="capability-links" aria-label="模块功能">{(scopes[scope]||[]).map(([label,key,ready])=><button key={key} onClick={()=>navigate(key)}><span>{label}</span><ArrowRight size={15}/></button>)}</div>}

export function SectionPage({id,navigate,components,onRules,onRights,onAttention}){
 const {Head,Table,Tag,Btn,Note}=components;
 const [view,setView]=useState(''),[search,setSearch]=useState('');
 const def=sectionDefs[id], flow=stages[id];
 const titles=id==='model'?['业务模型','维度模型','指标管理']:id==='standards'?['数据标准','标准表','标准检验']:id==='security'?['安全概览','使用依据','安全操作','安全活动','安全预警']:id==='my-work'?['待办事项','我的流程','我的取数']:[];
 const current=titles.includes(view)?view:titles[0];
 const action=(label,fn)=><Btn quiet onClick={fn}>{label} <ArrowRight size={16}/></Btn>;
 return <><Head crumb={def[0]+' / '+def[1]} title={def[1]} description={def[2]}/>

 {flow?<><div className="workflow-cards">{flow.map(([title,desc,target,state],i)=><section key={title} className="workflow-card"><span className="stage-number">阶段 0{i+1}</span><h2>{title}</h2><p>{desc}</p>{state!=='可交互演示'&&<Tag tone="blue">{state}</Tag>}<div>{action('进入'+title,()=>navigate(target))}</div></section>)}</div><section className="bottom-section"><h2>阶段衔接</h2><p className="section-sub">准备条件、执行结果和验收依据分别记录；入口可用不等于阶段已完成。</p><Table headers={['阶段','建议交付物','进度依据']} rows={flow.map(([t,d,k,s])=>[t,({'调研规划':'现状底稿与治理计划','架构设计':'架构方案与主题域清单','环境搭建':'环境登记与接入记录','数据建模':'业务模型与指标定义','开发方案':'需求与验收清单','数据汇聚':'批次与输入版本','数据处理':'冻结输入与输出版本','数据发布':'发布检查与证据','数据监测':'独立评测与问题记录','数据资产':'资产责任与版本记录','标准建设':'标准版本与检验结果','知识沉淀':'案例与文档'}[t]),'尚未接入真实项目进度'])}/></section></>:
 <>{titles.length>0&&<div className="segmented module-switch" role="group" aria-label={def[1]+'分类'}>{titles.map(t=><button key={t} className={current===t?'active':''} aria-pressed={current===t} onClick={()=>setView(t)}>{t}</button>)}</div>}
 {id==='plan'?<DemandWorkspace components={components} navigate={navigate}/>:
 ['ingestion-report','processing-report','publish-report'].includes(id)?<PipelineReport key={id} id={id} components={components} navigate={navigate}/>:<DemoPage key={id} id={id} components={components} navigate={navigate}/>}
 </>}
 </>
}
function InfoText(){return <FileText size={18}/>}
function ShieldLabel(){return <ShieldCheck size={21}/>}
