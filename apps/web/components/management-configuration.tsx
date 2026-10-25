/* eslint-disable @next/next/no-html-link-for-pages -- Query links revalidate identity and current workspace. */
import {configurationQueryHref,type ConfigurationManagementResult} from '@/lib/platform-management';
export function ManagementConfiguration({result}:{result:ConfigurationManagementResult}){
 const d=result.data;
 const rows=[['workspaces','工作空间概览',d.queries.workspace],['organizations','目录组织资料',d.queries.organization],['members','空间成员',d.queries.members],['users','用户目录',d.queries.users],['roles','角色权限',d.queries.roles],['audit','审计记录',d.queries.audit]] as const;
 return <section className="management-detail" aria-label="基础配置概览"><h2>当前空间管理</h2><dl className="management-member-facts"><dt>配置范围</dt><dd>当前工作空间的管理入口</dd><dt>登录方式</dt><dd>统一身份登录</dd><dt>会话范围</dt><dd>当前人员与工作空间</dd><dt>访问方式</dt><dd>仅查询</dd><dt>角色来源</dt><dd>{d.rolePolicySource==='APPLICATION_CATALOG'?'应用角色目录':'未配置'}</dd></dl>
  <h3>查询入口</h3><div className="table-wrap"><table aria-label="管理查询配置"><thead><tr><th>查询项目</th><th>入口配置</th><th>操作</th></tr></thead><tbody>{rows.map(([kind,label,configured])=><tr key={kind}><td>{label}</td><td>{configured?'已配置':'未配置'}</td><td>{configured?<a href={configurationQueryHref(kind)}>查看{label}</a>:<span>尚未接入</span>}</td></tr>)}</tbody></table></div>
  <p className="section-sub">进入查询时重新核验身份与资料可用性。</p>
  <details className="management-evidence"><summary>查看配置范围说明</summary><p>本页展示当前管理入口的配置。其他业务模块由各自的业务规则核验。</p><p>人员目录：{d.directory.state==='CONFIGURED'?'已配置，尚未查询':'未配置'}</p><p>敏感连接资料由服务端保管，本页不提供配置修改。</p></details>
 </section>;
}
