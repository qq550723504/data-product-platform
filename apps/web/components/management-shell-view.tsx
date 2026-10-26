
/* eslint-disable @next/next/no-html-link-for-pages -- Management navigation intentionally revalidates the current session on every full request. */
/* eslint-disable @next/next/no-img-element -- Reuses the local shared brand asset without exposing an image optimizer endpoint in this restricted identity profile. */

import {managementMenus} from '../lib/platform-management';
export function ManagementShellView({children,identityEnabled=false,path,icons}:{children:React.ReactNode;identityEnabled?:boolean;path:string;icons:React.ElementType[]}){
 return <div className="governance-demo"><div className="app page-settings management-shell"><a className="skip" href="#management-main">跳到内容</a><header className="topbar"><a href={identityEnabled?'/settings':'/'} className="brand"><img src="/design/brand.png" alt=""/><strong>数据治理</strong></a><nav className="centers" aria-label="业务中心">{!identityEnabled&&<><a href="/">工作台</a><a href="/#develop">数据开发中心</a><a href="/#manage">数据管理中心</a></>}</nav><div className="environment"><span>系统设置</span>{identityEnabled?<a href="/identity">身份与工作空间</a>:<a href="/">返回工作台</a>}</div></header><aside className="section-sidebar"><div className="section-caption">平台管理</div><nav aria-label="平台管理">{managementMenus.map(([key,label],i)=>{const href='/settings'+(key?'/'+key:''),Icon=icons[i];return <a key={key} href={href} aria-current={path===href?'page':undefined} className={path===href?'active':''}><Icon size={19}/><span>{label}</span></a>})}</nav></aside><main id="management-main" className="management-page">{children}</main></div></div>;
}
