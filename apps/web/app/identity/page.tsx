import Link from 'next/link';
import {currentBrowserPrincipal,currentAcceptanceClient} from '@/lib/iam-server';
import {iamEnabled} from '@/lib/iam-mode';
import {workspaces} from '@/lib/iam-workspace';
export const dynamic='force-dynamic';
const labels:Record<string,string>={BUSINESS_ACCEPTANCE_READ:'查看验收责任与标准',BUSINESS_ACCEPTANCE_DRAFT:'提供方草稿资格',BUSINESS_ACCEPTANCE_BASIS_CONFIRM:'接收方标准确认资格',BUSINESS_RECEIPT_RECORD:'接收方签收资格'};
export default async function IdentityPage(){
 if(!iamEnabled())return <p>当前环境未启用多人身份入口。</p>;
 try{
  const {principal:p,csrf}=await currentBrowserPrincipal();
  let targets:{deliveryOperationId:string;datasetVersionId:string;name:string;roles:string[]}[]|null=null;
  try{const client=await currentAcceptanceClient();const response=await client.request(`${client.apiBaseUrl}/api/v1/workspaces/${p.workspace_id}/acceptance-targets`);if(!response.ok)throw new Error();const data=await response.json();if(data.workspaceId!==p.workspace_id||!Array.isArray(data.items)||data.items.length>100)throw new Error();for(const t of data.items){if(!t||Object.keys(t).sort().join(',')!=='datasetVersionId,deliveryOperationId,name,roles'||!/^([0-9a-f-]{36})$/.test(t.deliveryOperationId)||!/^([0-9a-f-]{36})$/.test(t.datasetVersionId)||typeof t.name!=='string'||!t.name.trim()||!Array.isArray(t.roles)||!t.roles.length||t.roles.some((v:unknown)=>typeof v!=='string'||!['PROVIDER','RECEIVER','BASIS_CONFIRMER','REVIEWER','ARCHIVIST','DISPOSER'].includes(v)))throw new Error();}targets=data.items;}catch{targets=null;}

  return <><div className="identity-heading"><div><p className="identity-eyebrow">IDENTITY & WORKSPACE</p><h1>身份与工作空间</h1><p>核对本人身份与办理范围，再查看明确成果的责任依据。</p></div><span className="identity-status">● 身份已核验</span></div>
   <div className="identity-grid"><section className="identity-card"><p className="identity-eyebrow">当前平台身份</p><h2>本人身份已绑定</h2><code data-testid="current-principal">{p.principal_id}</code><p>此身份由服务端映射确认，页面角色和资产归属不授予办理权限。</p></section><section className="identity-card"><p className="identity-eyebrow">当前工作空间</p><h2>{workspaces().find(v=>v.id===p.workspace_id)?.label}</h2><code data-testid="current-workspace">{p.workspace_id}</code><p>跨空间访问会重新核验成员关系与能力。</p></section></div>
   <section className="identity-card"><h2>当前能力与责任预检</h2><p>模块能力决定入口资格，办理还须匹配成果的有效代表关系及指定责任。</p><div className="identity-capabilities">{p.capabilities.map(c=><span key={c}>{labels[c]}</span>)}</div><div className="identity-notice">本阶段不创建验收单、签收或验收决定。身份通过和责任匹配均不代表已签收或验收通过。</div><Link className="identity-primary" href="/core/acceptance-context">进入标准与验收责任</Link></section>
   <section className="identity-card"><h2>本人负责的成果</h2>{targets===null?<p>UNKNOWN · 成果责任列表尚未读取成功，未使用演示数据补齐。</p>:!targets.length?<p>当前空间没有匹配本人的有效成果责任。</p>:<div className="identity-workspaces">{targets.map(t=><Link key={t.deliveryOperationId} href={'/core/acceptance-context?'+new URLSearchParams({deliveryOperationId:t.deliveryOperationId,datasetVersionId:t.datasetVersionId})}>{t.name} · 查看责任与标准</Link>)}</div>}</section>
   <section className="identity-card"><h2>工作空间</h2><p>选择其他空间后，通过身份源重新确认资格；未获授权的空间会明确拒绝。</p><div className="identity-workspaces">{workspaces().map(w=><Link key={w.id} href={'/auth/login?workspaceId='+w.id}>{w.label}{w.id===p.workspace_id?' · 当前':''}</Link>)}</div></section>
   <details className="identity-card"><summary>查看本次身份核验依据</summary><p>权威版本：{p.revision}</p><p>确认期限：{p.expires_at}</p><p>每次读取重新核验；该期限不是后续业务写入授权。</p></details><form method="post" action="/auth/logout" className="identity-signout"><input type="hidden" name="csrf" value={csrf}/><button type="submit">退出当前应用</button></form></>;
 }catch{return <section className="identity-card"><h1>登录状态不可用</h1><p>当前身份、空间资格或身份源尚未确认。请重新登录；没有使用共享账号代办。</p><Link className="identity-primary" href="/auth/login">统一身份登录</Link> <Link href="/auth/logout">清除已有会话</Link></section>}
}
