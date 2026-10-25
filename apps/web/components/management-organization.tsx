/* eslint-disable @next/next/no-html-link-for-pages -- Organization and member navigation revalidates current authority. */
import {organizationQueryHref,type OrganizationManagementResult} from '@/lib/platform-management';
const states={active:'启用',inactive:'停用',removed:'已移除'};
export function ManagementOrganization({result}:{result:OrganizationManagementResult}){
 const o=result.data.organization;
 return <section className="management-detail" aria-label="目录组织资料"><h2>{o.name??'目录组织资料'}</h2><dl className="management-member-facts"><dt>组织名称</dt><dd>{o.name??'未提供'}</dd><dt>目录状态</dt><dd>{states[o.state]}</dd><dt>资料来源</dt><dd>当前空间绑定的身份目录</dd><dt>部门层级</dt><dd>来源未配置</dd></dl>
  <div className="management-cards"><a href={organizationQueryHref('members')}><h2>当前空间成员</h2><p>查看当前空间内的成员关系与角色。</p><span>查看成员<span aria-hidden="true"> →</span></span></a><a href={organizationQueryHref('users')}><h2>用户资料</h2><p>核对当前空间可见的账号与身份关联。</p><span>查看用户<span aria-hidden="true"> →</span></span></a></div>
  <a className="management-back" href="/settings/workspaces"><span aria-hidden="true">← </span>返回工作空间</a>
  <details className="management-evidence"><summary>查看组织资料依据</summary><p>目录组织引用：{o.id}</p><p>资料创建时间：{o.createdAt??'未提供'}</p><p>资料更新时间：{o.changedAt??'未提供'}</p><p>以上为身份目录资料；企业部门层级、法定主体及负责人尚无可信来源。</p><p>目录状态不代替当前空间的访问资格，进入成员或用户页面时重新核验。</p></details>
 </section>;
}
