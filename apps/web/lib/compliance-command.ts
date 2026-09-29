export type ComplianceCommandConfig = {
  enabled: boolean;
  workspaceId?: string | null;
  actorId?: string | null;
  apiBaseUrl: string;
};

export type ComplianceResult = {
  id: string;
  assessmentAttemptId: string;
  workspaceId: string;
  datasetVersionId: string;
  policyRef: string;
  policyVersion: string;
  gateDecision: string;
  summary: Record<string, unknown>;
  findings: Array<{
    id: string;
    resultId: string;
    fieldName: string;
    category: string;
    action: string;
    status: string;
    message: string;
    createdAt: string;
  }>;
  createdAt: string;
};

export type ComplianceActionState = {
  ok: boolean;
  message: string;
  attemptId?: string;
  resultId?: string;
  refreshRequired?: boolean;
};

export class ComplianceCommandError extends Error {
  constructor(message: string, readonly code: string) {
    super(message);
    this.name = "ComplianceCommandError";
  }
}

const uuidPattern=/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
export function isComplianceAttemptId(v:unknown):v is string {
  return typeof v==="string" && uuidPattern.test(v) && v!=="00000000-0000-0000-0000-000000000000";
}
function isUUID(v:unknown):v is string { return isComplianceAttemptId(v); }
function record(v:unknown):Record<string,unknown>{
  if(!v || typeof v!=="object" || Array.isArray(v)) throw new ComplianceCommandError("Core API 返回了无效的数据。","INVALID_RESPONSE");
  return v as Record<string,unknown>;
}
function sameId(a:unknown,b:string){ return isUUID(a) && a.toLowerCase()===b.toLowerCase(); }

export function complianceConfigurationError(config:ComplianceCommandConfig):string|null {
  if(!config.enabled) return "Compliance 写入默认关闭；仅在受信任的 POC 环境设置 POC_ENABLE_COMPLIANCE_ACTIONS=true。";
  if(!isUUID(config.workspaceId)) return "请配置有效的 POC_WORKSPACE_ID。";
  if(!isUUID(config.actorId)) return "请配置有效的服务端 POC_COMPLIANCE_ACTOR_ID。";
  return null;
}

export async function executeComplianceCheck(
  form:FormData,
  config:ComplianceCommandConfig,
  request:typeof fetch=fetch,
):Promise<ComplianceActionState>{
  let attemptedWrite=false;
  const configError=complianceConfigurationError(config);
  if(configError) return {ok:false,message:configError};

  const workspaceId=config.workspaceId as string;
  const actorId=config.actorId as string;
  const datasetId=form.get("datasetId");
  const versionId=form.get("versionId");
  const attemptId=form.get("assessmentAttemptId");
  const policyValue=form.get("policyRef");
  const policyRef=typeof policyValue==="string"?policyValue.trim():"";
  const effectivePolicyRef=policyRef || "park/compliance/enterprise-activity-compliance-v1.yaml";

  if(!isUUID(datasetId)||!isUUID(versionId)||!isUUID(attemptId)) {
    return {ok:false,message:"Dataset、DatasetVersion 和 assessmentAttemptId 必须是有效 UUID。"};
  }
  if(effectivePolicyRef.length>1024) return {ok:false,message:"Compliance Policy Ref 过长。"};

  const base=config.apiBaseUrl.replace(/\/$/,"");
  async function raw(path:string,init:RequestInit={},timeoutMs=15000){
    return request(`${base}${path}`,{
      ...init,
      cache:"no-store",
      redirect:"error",
      signal:AbortSignal.timeout(timeoutMs),
      headers:{Accept:"application/json",...init.headers},
    });
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
      throw new ComplianceCommandError(`Core API 拒绝了本次操作（HTTP ${response.status}，${code}）。`,code);
    }
    try{return record(await response.json());}catch(e){
      if(e instanceof ComplianceCommandError) throw e;
      throw new ComplianceCommandError("Core API 返回了无法解析的响应。","INVALID_RESPONSE");
    }
  }

  try{
    const version=await json(`/api/v1/dataset-versions/${versionId}?workspaceId=${encodeURIComponent(workspaceId)}`);
    if(!sameId(version.id,versionId)||!sameId(version.datasetId,datasetId)) {
      throw new ComplianceCommandError("DatasetVersion 不属于当前 Dataset。","DATASET_VERSION_SCOPE_MISMATCH");
    }
    if(version.status!=="READY"&&version.status!=="SUPERSEDED") {
      return {ok:false,message:"Core 只允许 READY 或 SUPERSEDED DatasetVersion 运行 Compliance Check。",refreshRequired:true,attemptId};
    }

    const recovery=await raw(`/api/v1/compliance-assessment-attempts/${attemptId}?workspaceId=${encodeURIComponent(workspaceId)}`);
    if(recovery.status!==404){
      if(!recovery.ok) throw new ComplianceCommandError(`无法读取 Compliance attempt（HTTP ${recovery.status}）。`,"ATTEMPT_READ_FAILED");
      const result=record(await recovery.json());
      if(!sameId(result.assessmentAttemptId,attemptId)||!sameId(result.workspaceId,workspaceId)||!sameId(result.datasetVersionId,versionId)) {
        throw new ComplianceCommandError("Compliance attempt 与当前 DatasetVersion 不匹配。","ATTEMPT_SCOPE_MISMATCH");
      }
      if(result.policyRef!==effectivePolicyRef) {
        throw new ComplianceCommandError("Compliance attempt 与当前 Policy 请求不匹配。","ATTEMPT_REQUEST_MISMATCH");
      }
      if(!isUUID(result.id)) throw new ComplianceCommandError("Compliance attempt 返回了无效 Result。","INVALID_RESPONSE");
      return {ok:true,message:"Compliance Check 已完成。",attemptId,resultId:result.id};
    }

    attemptedWrite=true;
    const response=await raw(`/api/v1/dataset-versions/${versionId}/compliance-checks`,{
      method:"POST",
      headers:{"Content-Type":"application/json","X-Actor-ID":actorId},
      body:JSON.stringify({workspaceId,policyRef:effectivePolicyRef,assessmentAttemptId:attemptId}),
    },30000);
    if(!response.ok){
      let code=`HTTP_${response.status}`;
      try{
        const body=record(await response.json());
        const e=body.error&&typeof body.error==="object"?record(body.error):body;
        if(typeof e.code==="string") code=e.code;
      }catch{}
      throw new ComplianceCommandError(`Core API 拒绝了本次操作（HTTP ${response.status}，${code}）。`,code);
    }
    const result=record(await response.json());
    if(!sameId(result.assessmentAttemptId,attemptId)||!sameId(result.workspaceId,workspaceId)||!sameId(result.datasetVersionId,versionId)||!isUUID(result.id)) {
      throw new ComplianceCommandError("Compliance Check 返回了不一致的 Result。","INVALID_RESPONSE");
    }
    if(result.policyRef!==effectivePolicyRef) {
      throw new ComplianceCommandError("Compliance Check 返回了不一致的 Policy。","INVALID_RESPONSE");
    }
    return {ok:true,message:"Compliance Check 已完成。",attemptId,resultId:result.id};
  }catch(error){
    const detail=error instanceof ComplianceCommandError?error.message:"暂时无法核实 Core API 的响应。";
    return {
      ok:false,
      message:attemptedWrite?`${detail} 请求可能已送达；请保留当前 attemptId 并刷新核对，不要生成新 attempt 重试。`:detail,
      attemptId,
      refreshRequired:attemptedWrite,
    };
  }
}
