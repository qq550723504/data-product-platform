"use server";
import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import { batch, batchEnabled, core, jobs, outputDataset, receipt, source, workspace } from "@/lib/batches";
import { platform } from "@/lib/platform";
import { collectAllPages } from "@/lib/pagination";
export type BatchState={message:string};
export async function registerBatch(_old:BatchState,form:FormData):Promise<BatchState>{
  let destination="";
  try{
    if(!batchEnabled())throw new Error("批次操作未启用");
    const key=String(form.get("requestKey")??"");const batchKey=String(form.get("batchKey")??"");
    if(!/^[a-zA-Z0-9_-]{1,128}$/.test(key)||!/^[a-zA-Z0-9_-]{1,128}$/.test(batchKey))throw new Error("批次名称仅支持字母、数字、下划线和连字符");
    const s=await source();const datasets=await collectAllPages(platform.datasets);
    const targets=[];
    for(const table of s.tables){
      const binding=s.targets.find(t=>t.code===table.Code&&t.workspaceId===workspace());
      if(!binding)throw new Error("来源目标尚未由操作者配置");
      const [resource,ds]=await Promise.all([platform.resource(binding.resourceId),platform.dataset(binding.datasetId)]);
      if(ds.workspaceId!==workspace()||resource.workspaceId!==workspace()||ds.datasetType!=="RAW"||ds.sourceResourceId!==resource.id)throw new Error("来源绑定冲突");
      targets.push({code:table.Code,resourceId:binding.resourceId,datasetId:binding.datasetId});
    }
    for(const [suffix,type] of [["standardized","STANDARDIZED"],["curated","CURATED"]]){
      const code=`${s.connectionRef}-${suffix}`;if(!datasets.some(d=>d.code===code))await core("/datasets","POST",{workspaceId:workspace(),code,name:`PostgreSQL ${type}`,datasetType:type,ownerId:process.env.POC_INGEST_ACTOR_ID});
    }
    const result=await core<{id:string}>("/ingestions","POST",{workspaceId:workspace(),requestKey:key,batchKey,targets});destination=`/batches/${result.id}`;
    for(const p of ["/batches","/resources","/datasets"])revalidatePath(p);
  }catch(e){return {message:e instanceof Error?e.message:"批次创建结果未知；保留请求标识后核对"};}
  redirect(destination);
}
export async function batchCommand(id:string,command:string,_old:BatchState):Promise<BatchState>{
  void _old;
  try{if(!batchEnabled()||!["dispatch","recover","accept","retry-failed"].includes(command))throw new Error("Command 未开放");await batch(id);await core(`/ingestions/${id}/${command}`,"POST");revalidatePath(`/batches/${id}`);return {message:"Core 操作完成；以当前事实状态为准。"};}catch(e){return {message:e instanceof Error?e.message:"响应未知；请核对原批次"};}
}
export async function startResolution(id:string,_old:BatchState):Promise<BatchState>{
  void _old;
  try{
    if(!batchEnabled())throw new Error("操作未开放");const op=await batch(id);const rec=await receipt(id);const input=rec.bindings.find(x=>x.code==="enterprise_raw");if(!input)throw new Error("完整 RAW receipt 缺失");
    const prior=await jobs(input.versionId);if(prior.length)return {message:"该 RAW 已有解析任务；请查看审核和输出。"};
    const output=await outputDataset(`${op.request.connectionRef}-standardized`,"STANDARDIZED");
    await core("/entity-match-jobs","POST",{workspaceId:workspace(),inputDatasetVersionId:input.versionId,outputDatasetId:output.id,sourceType:"CSV",sourceRef:"enterprise_raw.csv",sourceRole:"ANCHOR",policyRef:"park/matching/company-match-policy-v1.yaml"});revalidatePath(`/batches/${id}`);return {message:"实体解析已启动；需要人工确认的记录请进入既有实体审核。"};
  }catch(e){return {message:e instanceof Error?e.message:"解析结果未知"};}
}
export async function startProduction(id:string,_old:BatchState):Promise<BatchState>{
  void _old;
  try{
    if(!batchEnabled())throw new Error("操作未开放");const op=await batch(id);const rec=await receipt(id);const input=rec.bindings.find(x=>x.code==="enterprise_raw");if(!input)throw new Error("RAW receipt 缺失");
    const js=(await jobs(input.versionId)).filter(j=>j.status==="SUCCEEDED"&&j.outputDatasetVersionId);if(js.length!==1)throw new Error("先完成唯一的实体解析与审核");
    const output=await outputDataset(`${op.request.connectionRef}-curated`,"CURATED");const workflow=process.env.POC_INGESTION_WORKFLOW_ID;if(!workflow)throw new Error("审核后的加工模板尚未配置");
    const run=await core<{id:string;status:string}>("/executions","POST",{workspaceId:workspace(),workflowVersionId:workflow,outputDatasetId:output.id,targetPeriod:"2025-03",inputs:[...rec.bindings.map(b=>({name:b.code,datasetVersionId:b.versionId})),{name:"enterprise_resolution",datasetVersionId:js[0].outputDatasetVersionId}]},`batch-production-${id}`);
    revalidatePath(`/batches/${id}`);return {message:`加工事实：${run.id} · ${run.status}；请进入数据生产查看结果与隔离记录。`};
  }catch(e){return {message:e instanceof Error?e.message:"加工结果未知；核对原请求"};}
}
export async function refreshMetadata(resourceId:string,fqn:string,_old:BatchState):Promise<BatchState>{
  void _old;
  try{if(!batchEnabled())throw new Error("操作未开放");await platform.resource(resourceId);await core(`/data-resources/${resourceId}/metadata-bindings?workspaceId=${workspace()}`,"POST",{entityType:"TABLE",fullyQualifiedName:fqn,primary:true},undefined,true);revalidatePath(`/resources/${resourceId}`);return {message:"metadata 绑定已由提供方确认。"};}
  catch(e){return {message:`治理信息 UNKNOWN / 不可用：${e instanceof Error?e.message:"刷新失败"}。Core 的数据、质量和认证历史仍保留。`};}
}
