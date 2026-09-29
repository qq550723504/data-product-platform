export type QualityCommandConfig = {
  enabled: boolean;
  workspaceId?: string | null;
  actorId?: string | null;
  apiBaseUrl: string;
};

export type QualityAttemptStatus = {
  id: string;
  workspaceId: string;
  datasetVersionId: string;
  ruleSetRef: string;
  engineName: string;
  engineVersion: string;
  outcome: "IN_PROGRESS" | "SUCCEEDED" | "FAILED" | string;
  assessmentId?: string;
  leaseExpiresAt: string;
  leaseExpired: boolean;
};

export type QualityActionState = {
  ok: boolean;
  message: string;
  attemptId?: string;
  assessmentId?: string;
  refreshRequired?: boolean;
};

export class QualityCommandError extends Error {
  constructor(message: string, readonly code: string) {
    super(message);
    this.name = "QualityCommandError";
  }
}

const uuidPattern=/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
export function isQualityAttemptId(v: unknown): v is string { return typeof v === "string" && uuidPattern.test(v) && v !== "00000000-0000-0000-0000-000000000000"; }
function isUUID(v:unknown):v is string { return isQualityAttemptId(v); }
function record(v:unknown):Record<string,unknown>{
  if(!v || typeof v!=="object" || Array.isArray(v)) throw new QualityCommandError("Core API 返回了无效的数据。","INVALID_RESPONSE");
  return v as Record<string,unknown>;
}
function sameId(a:unknown,b:string){ return isUUID(a) && a.toLowerCase()===b.toLowerCase(); }

export function qualityConfigurationError(config:QualityCommandConfig):string|null {
  if(!config.enabled) return "Quality 写入默认关闭；仅在受信任的 POC 环境设置 POC_ENABLE_QUALITY_ACTIONS=true。";
  if(!isUUID(config.workspaceId)) return "请配置有效的 POC_WORKSPACE_ID。";
  if(!isUUID(config.actorId)) return "请配置有效的服务端 POC_QUALITY_ACTOR_ID。";
  return null;
}

export async function executeQualityCheck(
  form:FormData,
  config:QualityCommandConfig,
  request:typeof fetch=fetch,
):Promise<QualityActionState>{
  let attemptedWrite=false;
  const configError=qualityConfigurationError(config);
  if(configError) return {ok:false,message:configError};
  const workspaceId=config.workspaceId as string;
  const actorId=config.actorId as string;
  const datasetId=form.get("datasetId");
  const versionId=form.get("versionId");
  const attemptId=form.get("assessmentAttemptId");
  const ruleSetRefValue=form.get("ruleSetRef");
  const engineNameValue=form.get("engineName");
  const ruleSetRef=typeof ruleSetRefValue==="string"?ruleSetRefValue.trim():"";
  const effectiveRuleSetRef=ruleSetRef || "park/quality/enterprise-activity-quality-v1.yaml";
  const engineName=typeof engineNameValue==="string"?engineNameValue.trim():"";
  if(!isUUID(datasetId)||!isUUID(versionId)||!isUUID(attemptId)) return {ok:false,message:"Dataset、DatasetVersion 和 assessmentAttemptId 必须是有效 UUID。"};
  if(ruleSetRef.length>512||engineName.length>128) return {ok:false,message:"Quality 配置字段过长。"};

  const base=config.apiBaseUrl.replace(/\/$/,"");
  async function raw(path:string,init:RequestInit={}){
    return request(`${base}${path}`,{...init,cache:"no-store",redirect:"error",signal:AbortSignal.timeout(15000),headers:{Accept:"application/json",...init.headers}});
  }
  async function json(path:string,init:RequestInit={}){
    const response=await raw(path,init);
    if(!response.ok){
      let code=`HTTP_${response.status}`;
      try{
        const body=record(await response.json());
        const e=body.error&&typeof body.error==="object"?record(body.error):body;
        if(typeof e.code==="string") code=e.code;
      }catch{}
      throw new QualityCommandError(`Core API 拒绝了本次操作（HTTP ${response.status}，${code}）。`,code);
    }
    try{return record(await response.json());}catch(e){if(e instanceof QualityCommandError) throw e; throw new QualityCommandError("Core API 返回了无法解析的响应。","INVALID_RESPONSE");}
  }

  try{
    const version=await json(`/api/v1/dataset-versions/${versionId}?workspaceId=${encodeURIComponent(workspaceId)}`);
    if(!sameId(version.id,versionId)||!sameId(version.datasetId,datasetId)) throw new QualityCommandError("DatasetVersion 不属于当前 Dataset。","DATASET_VERSION_SCOPE_MISMATCH");
    if(version.status!=="READY"&&version.status!=="SUPERSEDED") return {ok:false,message:"Core 只允许 READY 或 SUPERSEDED DatasetVersion 运行 Quality Check。",refreshRequired:true,attemptId};

    const statusResponse=await raw(`/api/v1/quality-assessment-attempts/${attemptId}?workspaceId=${encodeURIComponent(workspaceId)}`);
    if(statusResponse.status!==404){
      if(!statusResponse.ok) throw new QualityCommandError(`无法读取 Quality attempt（HTTP ${statusResponse.status}）。`,"ATTEMPT_READ_FAILED");
      const status=record(await statusResponse.json());
      if(!sameId(status.id,attemptId)||!sameId(status.workspaceId,workspaceId)||!sameId(status.datasetVersionId,versionId)) {
        throw new QualityCommandError("Quality attempt 与当前 DatasetVersion 不匹配。","ATTEMPT_SCOPE_MISMATCH");
      }
      if(status.ruleSetRef !== effectiveRuleSetRef || (engineName && String(status.engineName ?? "").toLowerCase() !== engineName.toLowerCase())) {
        throw new QualityCommandError("Quality attempt 与当前 Rule Set / Engine 请求不匹配。","ATTEMPT_REQUEST_MISMATCH");
      }
      if(status.outcome==="SUCCEEDED"&&isUUID(status.assessmentId)){
        return {ok:true,message:"Quality Check 已完成。",attemptId,assessmentId:status.assessmentId};
      }
      if(status.outcome==="FAILED"){
        return {ok:false,message:"Quality Check 已失败；如需重新评测，请创建新的 attempt。",attemptId};
      }
      if(status.outcome==="IN_PROGRESS"&&status.leaseExpired!==true){
        return {ok:false,message:"Quality Check 正在运行中。",attemptId,refreshRequired:true};
      }
      // Lease expired: replay the same attempt ID so Core can reconcile it
      // without creating a second physical attempt.
    }

    attemptedWrite=true;
    const result=await json(`/api/v1/dataset-versions/${versionId}/quality-checks`,{
      method:"POST",
      headers:{"Content-Type":"application/json","X-Actor-ID":actorId},
      body:JSON.stringify({workspaceId,ruleSetRef:effectiveRuleSetRef,engineName,assessmentAttemptId:attemptId}),
    });
    if(!isUUID(result.id)||!sameId(result.workspaceId,workspaceId)||!sameId(result.datasetVersionId,versionId)){
      throw new QualityCommandError("Quality Check 返回了不一致的 Assessment。","INVALID_RESPONSE");
    }
    return {ok:true,message:"Quality Check 已完成。",attemptId,assessmentId:result.id as string};
  }catch(error){
    const detail=error instanceof QualityCommandError?error.message:"暂时无法核实 Core API 的响应。";
    return {ok:false,message:attemptedWrite?`${detail} 请求可能已送达；请保留当前 attemptId 并刷新核对，不要生成新 attempt 重试。`:detail,attemptId,refreshRequired:attemptedWrite};
  }
}
