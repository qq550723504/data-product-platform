"use server";
import { revalidatePath } from "next/cache";
import { batch,batchEnabled,core,jobs,outputDataset,receipt,workspace } from "@/lib/batches";
import { platform } from "@/lib/platform";
import type { BatchState } from "@/app/batches/actions";

export async function detectAnomaly(datasetId:string,versionId:string,_old:BatchState):Promise<BatchState>{
  void _old;
  try{
    if(!batchEnabled())throw new Error("受控操作未开放");
    const [dataset,version]=await Promise.all([platform.dataset(datasetId),platform.datasetVersion(versionId)]);
    if(dataset.workspaceId!==workspace()||version.datasetId!==datasetId||dataset.datasetType!=="RAW")throw new Error("只检测当前 workspace 的 RAW 原值");
    const result=await core<{caseId?:string;assessmentId:string;decision:string}>(`/workspaces/${workspace()}/quality-remediations/detect`,"POST",{versionId},`remediation-detect-${versionId}`,"human-decision");
    revalidatePath(`/datasets/${datasetId}/versions/${versionId}/remediation`);
    return {message:result.caseId?"已检测并登记异常；请分派后选择固定规则处理。":`未登记可由负数隔离规则处理的异常；质量门禁为 ${result.decision}，评测事实已保留。`};
  }catch(e){return {message:e instanceof Error?e.message:"检测结果未知；核对原请求"};}
}

export async function remediate(datasetId:string,versionId:string,caseId:string,command:string,_old:BatchState,form:FormData):Promise<BatchState>{
 void _old;
 try{
  if(!batchEnabled()||!["assign","execute","retry","recheck","close"].includes(command))throw new Error("Command 未开放");
  const detail=await core<{case:{version_id:string;workspace_id:string}}>(`/workspaces/${workspace()}/quality-remediations/${caseId}`,"GET",undefined,undefined,"human-decision");
  const version=await platform.datasetVersion(versionId);
  if(detail.case.version_id!==versionId||version.datasetId!==datasetId)throw new Error("处置对象不匹配");
  const key=String(form.get("requestKey")??"");if(!/^[a-zA-Z0-9_-]{1,128}$/.test(key))throw new Error("保留有效请求标识后重试");
  const reason=String(form.get("reason")??"").trim();let body:unknown={reason};
  if(command==="execute"){
   const id=String(form.get("batchId")??"");const op=await batch(id);const rec=await receipt(id);
   if(!rec.bindings.some(b=>b.versionId===versionId))throw new Error("批次不包含本次异常版本");
   const anchor=rec.bindings.find(b=>b.code==="enterprise_raw");if(!anchor)throw new Error("完整 RAW 接纳凭据缺失");
   const resolutions=(await jobs(anchor.versionId)).filter(j=>j.status==="SUCCEEDED"&&j.outputDatasetVersionId);
   if(resolutions.length!==1)throw new Error("先在批次入口完成唯一的实体解析与审核");
   const output=await outputDataset(`${op.request.connectionRef}-curated`,"CURATED");
   const workflow=process.env.POC_INGESTION_WORKFLOW_ID;if(!workflow)throw new Error("固定加工模板未配置");
   body={reason,workflowVersionId:workflow,outputDatasetId:output.id,targetPeriod:"2025-03",inputs:[...rec.bindings.map(b=>({name:b.code,datasetVersionId:b.versionId})),{name:"enterprise_resolution",datasetVersionId:resolutions[0].outputDatasetVersionId}]};
  }
  if(command==="recheck")body={executionLinkId:String(form.get("executionLinkId")??"")};
  await core(`/workspaces/${workspace()}/quality-remediations/${caseId}/${command}`,"POST",body,key,"human-decision");
  revalidatePath(`/datasets/${datasetId}/versions/${versionId}/remediation`);
  return {message:"处置事实已记录；请刷新查看执行或独立评测结果。人工关闭不会更改质量或认证。"};
 }catch(e){return {message:e instanceof Error?e.message:"响应未知；保留请求标识并核对原执行"};}
}
