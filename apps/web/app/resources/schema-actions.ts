"use server";
import { revalidatePath } from "next/cache";
import { batchEnabled, core, workspace } from "@/lib/batches";
import { platform } from "@/lib/platform";

export type SchemaActionState={message:string;requestId?:string};
export async function requestSchema(resourceId:string,_old:SchemaActionState,form:FormData):Promise<SchemaActionState>{
 try{
  if(!batchEnabled())throw new Error("来源操作未开放");
  await platform.resource(resourceId);
  const key=String(form.get("requestKey")??"");
  if(!/^[a-f0-9-]{36}$/.test(key))throw new Error("稳定请求标识缺失");
  const r=await core<{id:string}>(`/data-resources/${resourceId}/schema-observations?workspaceId=${workspace()}`,"POST",undefined,key,true);
  revalidatePath(`/resources/${resourceId}`);
  return {requestId:r.id,message:`原生来源扫描已排队：${r.id}。刷新页面查看 worker 结果；这不会修复数据或改变认证。`};
 }catch(e){return {message:e instanceof Error?e.message:"请求结果未知，请使用原请求重试"};}
}
export async function handleSchema(resourceId:string,observationId:string,_old:SchemaActionState,form:FormData):Promise<SchemaActionState>{
 try{
  if(!batchEnabled())throw new Error("来源操作未开放");
  await platform.resource(resourceId);
  const key=String(form.get("requestKey")??"");
  if(!/^[a-f0-9-]{36}$/.test(key))throw new Error("稳定请求标识缺失");
  await core(`/data-resources/${resourceId}/schema-observations/${observationId}/handle?workspaceId=${workspace()}`,"POST",{reason:String(form.get("reason")??"")},key,true);
  revalidatePath(`/resources/${resourceId}`);return {message:"已记录处理理由与责任人。恢复须再次原生扫描；质量、认证和历史版本保持原结论。"};
 }catch(e){return {message:e instanceof Error?e.message:"处理结果未知，保留原请求重试"};}
}
