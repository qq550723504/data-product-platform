import "server-only";
import { readFile } from "node:fs/promises";
import { configuredWorkspaceId, platform, PlatformError } from "./platform";
import { findAcrossPages } from "./scoped-lookup";

export type Source = { connectionRef:string; instanceRef:string; database:string; reader:string; tables:Array<{Code:string;Schema:string;Name:string;Columns:string[];Rows:number}> };
export type Batch = { id:string;state:string;attemptId:string;remoteId:string;request:{workspaceId:string;batchKey:string;connectionRef:string;instanceRef:string;templateHash:string;schemaHash:string;slots:Array<{code:string;resourceId:string;datasetId:string;columns:string[];rows:number}>};manifest?:{engineVersion:string;observedAt:string;outputs:Array<{code:string;sha256:string;rows:number;bytes:number}>} };
export type Receipt = {id:string;manifestHash:string;bindings:Array<{code:string;datasetId:string;versionId:string;sha256:string}>};
export type Job = {id:string;workspaceId:string;inputDatasetVersionId:string;status:string;outputDatasetVersionId?:string};
export function batchEnabled(){return process.env.POC_ENABLE_INGESTION_ACTIONS==="true";}
export function workspace(){const ws=configuredWorkspaceId();if(!ws)throw new Error("Workspace 未配置");return ws;}
export async function core<T>(path:string,method="GET",body?:unknown,key?:string,rights=false):Promise<T>{
  const tokenFile=rights?process.env.POC_RIGHTS_TOKEN_FILE:process.env.POC_INGESTION_TOKEN_FILE;
  if(!tokenFile)throw new Error("受信任的操作凭据未配置");
  const token=(await readFile(tokenFile,"utf8")).trim();
  const response=await fetch(`${process.env.PLATFORM_API_BASE_URL??"http://localhost:8080"}/api/v1${path}`,{method,cache:"no-store",redirect:"error",signal:AbortSignal.timeout(60000),headers:{Accept:"application/json",Authorization:`Bearer ${token}`,"Content-Type":"application/json",...(key?{"Idempotency-Key":key}:{}),...(process.env.POC_INGEST_ACTOR_ID?{"X-Actor-ID":process.env.POC_INGEST_ACTOR_ID}:{})},...(body===undefined?{}:{body:JSON.stringify(body)})});
  if(!response.ok){let code=`HTTP_${response.status}`;try{const data=await response.json();code=data.code??data.error?.code??code;}catch{}throw new PlatformError(`Core 拒绝操作：${code}`,response.status,code);}
  return await response.json() as T;
}
export async function batches(){return (await core<{items:Batch[]}>(`/ingestions?workspaceId=${workspace()}`)).items;}
export async function source(){return core<Source>(`/ingestion-source?workspaceId=${workspace()}`);}
export async function batch(id:string){const v=await core<Batch>(`/ingestions/${encodeURIComponent(id)}`);if(v.request.workspaceId!==workspace())throw new Error("Workspace 不匹配");return v;}
export async function receipt(id:string){return core<Receipt>(`/ingestions/${encodeURIComponent(id)}/receipt`);}
export async function jobs(input:string){return (await core<{items:Job[]}>(`/workspaces/${workspace()}/entity-match-jobs?inputVersionId=${input}`,"GET",undefined,undefined,true)).items;}
export async function outputDataset(code:string,type:string){const ds=await findAcrossPages(platform.datasets,d=>d.code===code);if(!ds||ds.datasetType!==type)throw new Error("加工输出 Dataset 尚未登记");return ds;}
export async function certificationReferences(version:string,profile:string,quality:string):Promise<Record<string,string>>{
  if(!profile||!quality)return {};
  return core<Record<string,string>>(`/workspaces/${workspace()}/dataset-versions/${version}/certification-inputs?profileId=${profile}&qualityAssessmentId=${quality}`,"GET",undefined,undefined,true);
}
