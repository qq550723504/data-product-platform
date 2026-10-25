/* eslint-disable @next/next/no-html-link-for-pages -- Full GET navigation revalidates current workspace authority. */
import {workspaceQueryHref,type WorkspaceManagementResult} from '@/lib/platform-management';

export function ManagementWorkspace({result,label}:{result:WorkspaceManagementResult;label:string}){
 const d=result.data;
 return <section className="management-detail" aria-label="工作空间概览"><h2>{label}</h2>
  <dl className="management-member-facts"><dt>访问资格</dt><dd>已核验，可查询当前空间</dd><dt>名称来源</dt><dd>接入配置</dd><dt>空间档案</dt><dd>尚未接入</dd><dt>负责人</dt><dd>—</dd><dt>配额</dt><dd>—</dd></dl>
  <p className="section-sub">负责人、配额尚无可信资料。空间切换通过上方入口重新核验。</p>
  <h3>空间查询</h3><div className="management-cards">{([['members','空间成员','查看当前空间的成员关系'],['users','用户目录','核对账号资料与身份关联'],['roles','角色权限','查看应用角色的权限映射'],['audit','审计记录','追溯当前空间的操作记录']]as const).map(([kind,title,desc])=>d.queries[kind]?<a key={kind} href={workspaceQueryHref(kind)}><h2>{title}</h2><p>{desc}</p><span>查看<span aria-hidden="true"> →</span></span></a>:<section className="management-state" key={kind}><h2>{title}</h2><p>查询入口未配置</p></section>)}</div>
  <p className="section-sub">进入查询时重新核验资料可用性。</p>
  <details className="management-evidence"><summary>查看空间与接入依据</summary><p>空间引用：{result.workspaceId}</p><p>空间名称来自当前应用配置，不代表正式空间档案。</p><p>本次访问资格核验截至：{d.confirmationExpiresAt}</p><p>该期限不授予后续操作权限。</p><p>人员目录：{d.directory.state==='CONFIGURED'?'已配置，尚未查询':'未配置'}</p>{d.directory.state==='CONFIGURED'&&<><p>目录组织引用：{d.directory.organizationId}</p><p>目录应用引用：{d.directory.projectId}</p><p>目录组织不等于工作空间。</p></>}</details>
 </section>;
}
