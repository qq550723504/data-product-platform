import {businessText,displayText} from '../lib/business-language.mjs';
import Link from 'next/link';
import {currentAcceptanceClient} from '@/lib/iam-server';
import {isIdentity} from '@/lib/quality-attention';
export async function AcceptanceTargets({destination='context'}:{destination?:'context'|'basis'}){
 try{
  const client=await currentAcceptanceClient();const response=await client.request(`${client.apiBaseUrl}/api/v1/workspaces/${client.principal.workspace_id}/acceptance-targets`);
  if(!response.ok)throw Error();const data=await response.json();
  if(data.workspaceId!==client.principal.workspace_id||!Array.isArray(data.items)||data.items.length>100||new Set(data.items.map((v:{deliveryOperationId:string})=>v.deliveryOperationId)).size!==data.items.length)throw Error();
  for(const t of data.items)if(!t||Object.keys(t).sort().join(',')!=='datasetVersionId,deliveryOperationId,name,roles'||!isIdentity(t.deliveryOperationId)||!isIdentity(t.datasetVersionId)||typeof t.name!=='string'||!t.name.trim()||!Array.isArray(t.roles)||!t.roles.length||t.roles.some((v:unknown)=>typeof v!=='string'||!['PROVIDER','RECEIVER','BASIS_CONFIRMER','REVIEWER','ARCHIVIST','DISPOSER'].includes(v)))throw Error();
  return <section className="identity-card"><h2>本人负责的成果</h2>{!data.items.length?<p>当前空间没有匹配本人的有效成果责任。</p>:<div className="basis-targets">{data.items.map((t:{deliveryOperationId:string;datasetVersionId:string;name:string})=><Link key={t.deliveryOperationId} href={(destination==='basis'?'/core/acceptance-basis?':'/core/acceptance-context?')+new URLSearchParams({deliveryOperationId:t.deliveryOperationId,datasetVersionId:t.datasetVersionId})}><strong>{businessText(t.name)}</strong><span>{businessText(destination==='basis'?'核对业务依据':'查看责任与标准')}</span><small>交付：{displayText(t.deliveryOperationId)}</small></Link>)}</div>}</section>;
 }catch{return <section className="identity-card"><h2>本人负责的成果</h2><p>无法确认 · 当前成果责任列表未读取成功。</p></section>}
}
