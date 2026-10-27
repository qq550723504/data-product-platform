/* eslint-disable @next/next/no-html-link-for-pages -- Full GET navigation revalidates privileged records. */
import {ManagementRowFocus} from './management-row-focus';
import {managementUserListHref,managementUserReturnHref,managementUserDetailHref,managementMemberDetailHref,managementPageQuery,permissionLabels,type UserManagementResult,type ManagementSelection} from '@/lib/platform-management';

const accountNames={active:'启用',inactive:'停用',locked:'锁定',initial:'待激活',deleted:'已删除'};
export function ManagementUsers({result,selection:s}:{result:UserManagementResult;selection:ManagementSelection}){
 const d=result.data,selected=d.selected,roleName=(id:string)=>d.roleDefinitions.find(r=>r.id===id)?.name??'未映射角色';
 if(selected){const {profile:p,identity:link,memberships}=selected;return <section className="management-detail management-member-view" aria-label="用户详情">
  <a className="management-back" href={managementUserReturnHref(s,p.id)}><span aria-hidden="true">← </span>返回{s.memberContext?'成员列表':'用户目录'}</a><h2>{p.displayName||p.username}</h2>
  <dl className="management-member-facts"><dt>登录名</dt><dd>{p.username}</dd><dt>账号状态</dt><dd>{accountNames[p.accountState]}</dd><dt>身份关联</dt><dd>{link.state==='UNCONFIGURED'?'关联来源未配置':link.state==='LINKED'?'已关联':'未关联'}</dd></dl>
  <h3>当前空间的成员关系</h3>
  <div className="table-wrap"><table aria-label="用户的空间成员关系"><thead><tr><th>成员关系状态</th><th>应用角色</th><th>已配置权限</th><th>操作</th></tr></thead><tbody>{memberships.map(m=><tr key={m.id}><td>{m.state==='active'?'启用':'停用'}</td><td>{m.roles.map(roleName).join('、')||'未配置角色'}</td><td>{m.permissions.map(k=>permissionLabels[k]).join('、')||'暂无映射'}</td><td><a href={managementMemberDetailHref(s.memberContext?.memberId===m.id?s.memberContext:{offset:0,from:s.from},m.id)}>查看成员</a></td></tr>)}</tbody></table></div>
  <p className="section-sub">账号状态、成员关系和应用角色分别核验。角色映射与身份关联不代替当前空间的访问授权。</p><a href={'/settings/roles'+managementPageQuery({offset:0,from:s.from})}>查看角色权限<span aria-hidden="true"> →</span></a>
  <details className="management-evidence"><summary>查看资料与身份关联依据</summary><p>目录用户：{p.id}</p><p>所属组织：{p.organizationId}</p><p>资料更新时间：{p.changedAt}</p>{link.state==='LINKED'&&<><p>平台人员：{link.actorId}</p><p>关联依据：{link.basisRef}</p><p>身份版本：{link.revision}</p><p>有效期至：{link.expiresAt}</p></>}</details>
 </section>}
 return <><form method="get" action="/settings/users" className="management-filters" aria-label="用户筛选">{s.from&&<input type="hidden" name="from" value={s.from}/>}<label>搜索用户<input name="q" defaultValue={s.q} placeholder="姓名或登录名"/></label><button className="button" type="submit">查询</button><a href={managementUserListHref({offset:0,from:s.from})}>清除条件</a></form>
  <div className="management-count">共 {d.total} 名用户</div><div className="table-wrap"><table aria-label="用户目录"><thead><tr><th>用户</th><th>空间成员关系数</th><th>操作</th></tr></thead><tbody>{d.items.map(u=><tr id={'user-'+u.id} tabIndex={-1} key={u.id}><td><strong>{u.displayName||u.id}</strong><small>{u.loginName}</small></td><td>{u.membershipCount}</td><td><a href={managementUserDetailHref(s,u.id)}>查看用户</a></td></tr>)}</tbody></table></div>
  <ManagementRowFocus prefix="user-"/>{!d.items.length&&<p className="empty">当前条件下没有用户</p>}<nav className="pagination" aria-label="用户分页">{s.offset>0?<a href={managementUserListHref({...s,offset:Math.max(0,s.offset-25)})}>上一页</a>:<span>已到首页</span>}<span>第 {Math.floor(s.offset/25)+1} 页</span>{s.offset+25<d.total?<a href={managementUserListHref({...s,offset:s.offset+25})}>下一页</a>:<span>已到末页</span>}</nav>
  {!d.items.length&&s.offset>0&&<a href={managementUserListHref({...s,offset:0})}>保留条件，返回首页</a>}<p className="section-sub">目录仅包含当前空间可见的用户，账号状态在详情中单独核验。</p>
 </>;
}
