import Link from 'next/link';
export function AcceptanceIdentityShell({children}:{children:React.ReactNode}){
 return <div className="identity-shell"><aside className="identity-sidebar"><Link href="/identity" className="identity-brand"><span>◈</span> 数据治理平台</Link><p className="identity-sidebar-caption">业务协作 · 可信身份</p><nav aria-label="验收身份导航"><Link href="/identity">身份与工作空间</Link><Link href="/core/acceptance-context">标准与验收责任</Link></nav><div className="identity-sidebar-foot">1A 独立验收环境<br/>业务写入与执行保持关闭</div></aside><div className="identity-content"><header className="identity-topbar"><span>业务验收 / 身份与责任</span><span className="identity-pill">只读准入</span></header><main>{children}</main></div></div>;
}
