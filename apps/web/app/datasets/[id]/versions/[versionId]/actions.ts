"use server";

import { revalidatePath } from "next/cache";
import { configuredWorkspaceId } from "@/lib/platform";
import {
  executeInvalidateDatasetVersion,
  type DatasetVersionActionState,
} from "@/lib/dataset-version-command";
import { executeQualityCheck, type QualityActionState } from "@/lib/quality-command";
import { executeComplianceCheck, type ComplianceActionState } from "@/lib/compliance-command";
import { batchEnabled,core,workspace } from "@/lib/batches";
import { platform } from "@/lib/platform";

export async function evaluateCertification(_old:{message:string},form:FormData):Promise<{message:string}>{
 try{
  if(!batchEnabled())throw new Error("认证操作未开放");
  const version=String(form.get("versionId"));const dataset=String(form.get("datasetId"));const v=await platform.datasetVersion(version);await platform.dataset(dataset);if(v.datasetId!==dataset)throw new Error("版本不属于 Dataset");
  const refs:Record<string,string>={};for(const name of ["profileId","qualityAssessmentId","rightsSnapshotId","effectiveRightsSnapshotId","complianceResultId","contractVersionId","traceabilityEvidenceId","evidenceSnapshotId"]){const value=String(form.get(name)??"").trim();if(value){if(!/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value))throw new Error("证据引用必须是 UUID");refs[name]=value;}}
  const result=await core<{id:string;decision:string;blockers:Array<{code:string;detail:string}>}>(`/workspaces/${workspace()}/dataset-versions/${version}/certifications`,"POST",refs,String(form.get("requestKey")),true);revalidatePath(`/datasets/${dataset}/versions/${version}`);return {message:`${result.decision} · ${result.id} · ${result.blockers.map(b=>b.code).join(", ")}`};
 }catch(e){return {message:e instanceof Error?e.message:"认证结果未知；请核对历史"};}
}

export async function prepareCertification(_old:{message:string},form:FormData):Promise<{message:string}>{
 try{
  if(!batchEnabled())throw new Error("认证操作未开放");
  const version=String(form.get("versionId"));const dataset=String(form.get("datasetId"));const v=await platform.datasetVersion(version);await platform.dataset(dataset);if(v.datasetId!==dataset)throw new Error("版本不属于 Dataset");
  await core(`/workspaces/${workspace()}/dataset-versions/${version}/certification-inputs`,"POST",{profileId:String(form.get("profileId")),qualityAssessmentId:String(form.get("qualityAssessmentId"))},String(form.get("requestKey")),true);
  revalidatePath(`/datasets/${dataset}/versions/${version}`);return {message:"已计算衍生权利并冻结实际加工、质量和合规证据。继续核验认证决定。"};
 }catch(e){return {message:e instanceof Error?e.message:"证据准备结果未知；保留请求标识后刷新核对"};}
}

export async function supersedeCertification(_old:{message:string},form:FormData):Promise<{message:string}>{
 try{
  if(!batchEnabled())throw new Error("认证操作未开放");
  const version=String(form.get("versionId"));const dataset=String(form.get("datasetId"));const v=await platform.datasetVersion(version);await platform.dataset(dataset);if(v.datasetId!==dataset)throw new Error("版本不属于 Dataset");
  const old=String(form.get("oldId"));const replacement=String(form.get("replacementId"));
  const history=await platform.certificationHistory(version,100);if(!history.items.some(c=>c.id===old)||!history.items.some(c=>c.id===replacement))throw new Error("先查看并明确选择同一版本的认证事实");
  await core(`/workspaces/${workspace()}/certifications/${old}/supersede`,"POST",{replacementId:replacement,reason:String(form.get("reason")),effectiveAt:String(form.get("effectiveAt"))},String(form.get("requestKey")),true);
  revalidatePath(`/datasets/${dataset}/versions/${version}`);return {message:"已追加 SUPERSEDED；旧认证事实保留，当前门禁以明确的新事实重新检查。"};
 }catch(e){return {message:e instanceof Error?e.message:"替代结果未知；刷新核对历史"};}
}

export async function invalidateDatasetVersion(
  _previous: DatasetVersionActionState,
  form: FormData,
): Promise<DatasetVersionActionState> {
  const result = await executeInvalidateDatasetVersion(form, {
    enabled: process.env.POC_ENABLE_DATASET_ACTIONS === "true",
    workspaceId: configuredWorkspaceId(),
    actorId: process.env.POC_DATASET_ACTOR_ID?.trim(),
    apiBaseUrl: process.env.PLATFORM_API_BASE_URL ?? "http://localhost:8080",
  });

  const datasetId = form.get("datasetId");
  const versionId = form.get("versionId");
  if ((result.ok || result.refreshRequired) && typeof datasetId === "string" && typeof versionId === "string") {
    revalidatePath(`/datasets/${datasetId}/versions/${versionId}`);
    revalidatePath(`/datasets/${datasetId}`);
    revalidatePath("/datasets");
    revalidatePath("/products");
    revalidatePath("/attention");
    revalidatePath("/");
  }
  return result;
}


export async function runQualityCheck(
  _previous: QualityActionState,
  form: FormData,
): Promise<QualityActionState> {
  const result = await executeQualityCheck(form, {
    enabled: process.env.POC_ENABLE_QUALITY_ACTIONS === "true",
    workspaceId: configuredWorkspaceId(),
    actorId: process.env.POC_QUALITY_ACTOR_ID?.trim(),
    apiBaseUrl: process.env.PLATFORM_API_BASE_URL ?? "http://localhost:8080",
  });

  const datasetId = form.get("datasetId");
  const versionId = form.get("versionId");
  if ((result.ok || result.refreshRequired) && typeof datasetId === "string" && typeof versionId === "string") {
    revalidatePath(`/datasets/${datasetId}/versions/${versionId}`);
    revalidatePath(`/datasets/${datasetId}`);
    revalidatePath("/datasets");
    revalidatePath("/products");
    revalidatePath("/");
  }
  return result;
}


export async function runComplianceCheck(
  _previous: ComplianceActionState,
  form: FormData,
): Promise<ComplianceActionState> {
  const result = await executeComplianceCheck(form, {
    enabled: process.env.POC_ENABLE_COMPLIANCE_ACTIONS === "true",
    workspaceId: configuredWorkspaceId(),
    actorId: process.env.POC_COMPLIANCE_ACTOR_ID?.trim(),
    apiBaseUrl: process.env.PLATFORM_API_BASE_URL ?? "http://localhost:8080",
  });

  const datasetId = form.get("datasetId");
  const versionId = form.get("versionId");
  if ((result.ok || result.refreshRequired) && typeof datasetId === "string" && typeof versionId === "string") {
    revalidatePath(`/datasets/${datasetId}/versions/${versionId}`);
    revalidatePath(`/datasets/${datasetId}`);
    revalidatePath("/datasets");
    revalidatePath("/products");
    revalidatePath("/");
  }
  return result;
}
