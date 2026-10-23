import {initialPipeline,emptyPipeline,addDemand,confirmDemand,savePlan,createTask,runTask} from './pipeline-model.mjs';
const enterprise=initialPipeline();
let park=addDemand(emptyPipeline(),{title:'园区月度用电趋势',background:'独立空间的合成用电样例，不关联企业经营空间。',owner:'能耗数据组',purpose:'园区能耗分析',priority:'中',dueDate:'2026-10-30'});
park=confirmDemand(park,park.demands[0].id);
park=savePlan(park,park.demands[0].id,{tables:['用电记录'],schedule:'手动触发',rule:'隔离负值后汇总',channel:'受控下载',description:'园区独立合成用电样例。',metadata:'period 为期间；usage_kwh 单位 kWh。',emergency:'待核实先检查原运行。',acceptance:'质量、权利、认证分别判断。'});
park=createTask(park,park.demands[0].id,'汇聚');park=runTask(park,park.tasks[0].id,'成功');
export function seedWorkspacePipeline(w){return w.kind==='enterprise'?enterprise:w.kind==='park'?park:emptyPipeline()}
export function seedWorkspaceAssets(w){return w.kind==='park'?[{id:'PARK-ASSET-001',name:'用电记录',description:'园区独立合成样本 · 14 行',owner:'能耗数据组',version:'RAW v1'}]:[]}
