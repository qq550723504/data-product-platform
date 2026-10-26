/* eslint-disable @next/next/no-html-link-for-pages -- Full GET navigation revalidates privileged reads and avoids restoring cached member records. */
import {ManagementRowFocus} from './management-row-focus';
import {managementMemberDetailHref,managementMemberListHref,managementPageQuery,permissionLabels,type ManagementResult,type ManagementSelection,type Member,type Role} from '@/lib/platform-management';

export function ManagementMembers({result,selection:s}:{result:ManagementResult;selection:ManagementSelection}){
 const definitions=result.data.roleDefinitions!,selected=result.data.selected;
 const roleName=(id:string)=>definitions.find(role=>role.id===id)?.name??'未映射角色';
 if(selected)return <section className="management-detail management-member-view" aria-label="成员详情">
  <a className="management-back" href={managementMemberListHref(s,selected.id)}><span aria-hidden="true">← </span>返回成员列表</a>
  <h2>{selected.displayName||selected.userId}</h2>
  <dl className="management-member-facts"><dt>登录名</dt><dd>{selected.loginName}</dd><dt>状态</dt><dd>{selected.state==='active'?'启用':'停用'}</dd><dt>当前角色</dt><dd>{selected.roles.map(roleName).join('、')||'未配置角色'}</dd><dt>已配置权限</dt><dd>{selected.permissions.map(permission=>permissionLabels[permission]).join('、')||'暂无映射'}</dd></dl>
  <details><summary>查看来源引用</summary><p>外部用户：{selected.userId}</p><p>成员关系：{selected.id}</p><p>所属组织：{selected.organizationId}</p><p>应用项目：{selected.projectId}</p></details>
 </section>;
 return <>
  {s.role&&<p className="management-role-context">角色：<strong>{roleName(s.role)}</strong><a href={'/settings/roles'+managementPageQuery({offset:0,from:s.from})}>返回角色权限 →</a></p>}
  <form method="get" action="/settings/members" className="management-filters" aria-label="成员筛选">
   {s.from&&<input type="hidden" name="from" value={s.from}/>}
   <label>搜索成员<input name="q" defaultValue={s.q} placeholder="姓名或登录名"/></label>
   <label>角色<select name="role" defaultValue={s.role??''}><option value="">全部角色</option>{definitions.map(role=><option key={role.id} value={role.id}>{role.name}</option>)}</select></label>
   <label>状态<select name="state" defaultValue={s.state??''}><option value="">全部</option><option value="active">启用</option><option value="inactive">停用</option></select></label>
   <button className="button" type="submit">筛选</button><a href={managementMemberListHref({offset:0,from:s.from})}>清除条件</a>
  </form>
  <div className="management-count">共 {result.data.total} 名成员</div>
  <div className="table-wrap"><table aria-label="空间成员"><thead><tr><th>成员</th><th>角色</th><th>状态</th><th>操作</th></tr></thead><tbody>{(result.data.items as Member[]).map(member=><tr id={'member-'+member.id} tabIndex={-1} key={member.id}>
   <td><strong>{member.displayName||member.userId}</strong><small>{member.loginName}</small></td><td>{member.roles.map(roleName).join('、')||'未配置角色'}</td><td>{member.state==='active'?'启用':'停用'}</td><td><a href={managementMemberDetailHref(s,member.id)}>查看成员</a></td>
  </tr>)}</tbody></table></div><ManagementRowFocus prefix="member-"/>
  {!result.data.items.length&&<p className="empty">当前条件下没有成员</p>}
  <nav className="pagination" aria-label="成员分页">
   {s.offset>0?<a href={managementMemberListHref({...s,offset:Math.max(0,s.offset-25)})}>上一页</a>:<span>已到首页</span>}
   <span>第 {Math.floor(s.offset/25)+1} 页</span>
   {s.offset+25<result.data.total!?<a href={managementMemberListHref({...s,offset:s.offset+25})}>下一页</a>:<span>已到末页</span>}
  </nav>
  {!result.data.items.length&&s.offset>0&&<a href={managementMemberListHref({...s,offset:0})}>保留筛选，返回首页</a>}
 </>;
}

export function ManagementRoles({roles,selection}:{roles:Role[];selection?:ManagementSelection}){
 return <><p className="section-sub">下表为已配置的应用角色权限，成员资格仍须由当前身份与工作空间授权核验。</p><div className="table-wrap"><table aria-label="角色权限"><thead><tr><th>角色</th><th>可查看内容</th><th>操作</th></tr></thead><tbody>{roles.map(role=><tr key={role.id}><td>{role.name}</td><td>{role.permissions.map(permission=>permissionLabels[permission]).join('、')||'暂无权限'}</td><td><a href={managementMemberListHref({offset:0,role:role.id,from:selection?.from})}>查看相关成员</a></td></tr>)}</tbody></table></div></>;
}
