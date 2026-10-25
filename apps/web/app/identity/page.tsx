import {businessText,displayText} from '../../lib/business-language.mjs';
import Link from 'next/link';
import {AcceptanceTargets} from '@/components/acceptance-targets';
import {currentBrowserPrincipal} from '@/lib/iam-server';
import {iamEnabled} from '@/lib/iam-mode';
import {workspaces} from '@/lib/iam-workspace';
export const dynamic='force-dynamic';
const labels:Record<string,string>={BUSINESS_ACCEPTANCE_READ:'查看验收责任与标准',BUSINESS_ACCEPTANCE_DRAFT:'提供方草稿资格',BUSINESS_ACCEPTANCE_BASIS_CONFIRM:'接收方标准确认资格',BUSINESS_RECEIPT_RECORD:'接收方签收资格'};
export default async function IdentityPage(){
 if(!iamEnabled())return <p>当前环境未启用多人身份入口。</p>;
 try{
  const {principal:p,csrf}=await currentBrowserPrincipal();
  if(process.env.BROWSER_IAM_PROFILE==='platform-management')return <><div className="identity-heading"><div><h1>身份与工作空间</h1><p>核对当前人员和空间范围，再进入系统设置。</p></div><span className="identity-status">身份已核验</span></div><section className="identity-card"><h2>当前管理范围</h2><p data-testid="management-principal">{p.principal_id}</p><p data-testid="management-workspace">{p.workspace_id}</p><p>可查询当前工作空间的成员、应用角色权限和审计记录。</p><Link className="identity-primary" href="/settings">返回系统设置</Link></section><section className="identity-card"><h2>工作空间</h2><div className="identity-workspaces">{workspaces().map(w=><Link key={w.id} href={'/auth/login?workspaceId='+w.id}>{w.label}{w.id===p.workspace_id?' · 当前':''}</Link>)}</div><p>进入其他空间时重新确认身份与查询资格。</p></section><form method="post" action="/auth/logout" className="identity-signout"><input type="hidden" name="csrf" value={csrf}/><button type="submit">退出当前应用</button></form></>;
  return <><div className="identity-heading"><div><p className="identity-eyebrow">IDENTITY & WORKSPACE</p><h1>身份与工作空间</h1><p>核对本人身份与办理范围，再查看明确成果的责任依据。</p></div><span className="identity-status">● 身份已核验</span></div>
   <div className="identity-grid"><section className="identity-card"><p className="identity-eyebrow">当前平台身份</p><h2>本人身份已绑定</h2><code data-testid="current-principal">{p.principal_id}</code><p>此身份由服务端映射确认，页面角色和资产归属不授予办理权限。</p></section><section className="identity-card"><p className="identity-eyebrow">当前工作空间</p><h2>{displayText(workspaces().find(v=>v.id===p.workspace_id)?.label)}</h2><code data-testid="current-workspace">{p.workspace_id}</code><p>跨空间访问会重新核验成员关系与能力。</p></section></div>
   <section className="identity-card"><h2>当前能力与责任预检</h2><p>模块能力决定入口资格，办理还须匹配成果的有效代表关系及指定责任。</p><div className="identity-capabilities">{displayText(p.capabilities.map(c=><span key={c}>{businessText(labels[c])}</span>))}</div><div className="identity-notice">本阶段不创建验收单、签收或验收决定。身份通过和责任匹配均不代表已签收或验收通过。</div><Link className="identity-primary" href="/core/acceptance-context">进入标准与验收责任</Link></section>
   <AcceptanceTargets/>
   <section className="identity-card"><h2>工作空间</h2><p>选择其他空间后，通过身份源重新确认资格；未获授权的空间会明确拒绝。</p><div className="identity-workspaces">{workspaces().map(w=><Link key={w.id} href={'/auth/login?workspaceId='+w.id}>{displayText(w.label)}{businessText(w.id===p.workspace_id?' · 当前':'')}</Link>)}</div></section>
   <details className="identity-card"><summary>查看本次身份核验依据</summary><p>权威版本：{displayText(p.revision)}</p><p>确认期限：{displayText(p.expires_at)}</p><p>每次读取重新核验；该期限不是后续业务写入授权。</p></details><form method="post" action="/auth/logout" className="identity-signout"><input type="hidden" name="csrf" value={csrf}/><button type="submit">退出当前应用</button></form></>;
 }catch{return <section className="identity-card"><h1>登录状态不可用</h1><p>当前身份、空间资格或身份源尚未确认。请重新登录；没有使用共享账号代办。</p><Link className="identity-primary" href="/auth/login">统一身份登录</Link> <Link href="/auth/logout">清除已有会话</Link></section>}
}
