/* eslint-disable @next/next/no-html-link-for-pages -- Privileged query navigation uses full requests to revalidate identity instead of restoring router-cached records. */
import {currentManagementClient} from '@/lib/iam-server';
import {iamEnabled} from '@/lib/iam-mode';
import {workspaces} from '@/lib/iam-workspace';
import {managementMenus,managementSelection,managementQuery,managementPageQuery,managementMemberListHref,managementUserListHref,validManagement,validUserManagement,validWorkspaceManagement,validOrganizationManagement,type OrganizationManagementResult,type WorkspaceManagementResult,type UserManagementResult,type ManagementSection,type ManagementResult,type Role,type Audit} from '@/lib/platform-management';
import {ManagementMembers,ManagementRoles} from './management-members';
import {ManagementUsers} from './management-users';
import {ManagementWorkspace} from './management-workspace';
import {ManagementOrganization} from './management-organization';
const names=Object.fromEntries(managementMenus),supported=['members','roles','audit','users','workspaces','organizations'];
const errors:Record<string,string>={SOURCE_UNCONFIGURED:'真实来源未配置',AUTHENTICATION_REQUIRED:'登录状态不可用',MANAGEMENT_ACCESS_DENIED:'没有当前空间的管理查询权限',IDENTITY_UNAVAILABLE:'当前身份无法核验',DIRECTORY_UNAVAILABLE:'成员目录暂时无法读取',INVALID_SOURCE_RESPONSE:'来源记录无法核验',AUDIT_UNAVAILABLE:'审计记录暂时无法读取',MEMBER_UNAVAILABLE:'指定成员不可见',INVALID_QUERY:'查询条件无效'};
export async function ManagementPage({section='',query={}}:{section?:string;query?:Record<string,string|string[]|undefined>}){
 const title=names[section]??'系统设置',enabled=iamEnabled()&&process.env.BROWSER_IAM_PROFILE==='platform-management';
 const heading=<><div className="breadcrumb">系统设置 / {title}</div><div className="page-heading"><div><h1>{title}</h1><p>{section==='workspaces'?'核对当前空间的访问范围与接入情况。':section==='organizations'?'核对当前空间绑定的目录组织资料。':'查看当前工作空间的人员、权限与操作记录。'}</p></div><span className="tag green">仅查询</span></div></>;
 if(!enabled)return <>{heading}<section className="management-state" role="status"><h2>真实来源未配置</h2><p>尚未连接人员目录和管理查询服务，当前没有可核验的管理记录。</p><p>现有 Agent 和资产查询保持原有接入。</p></section><Links/></>;
 let client;
 try{client=await currentManagementClient()}catch{return <>{heading}<section className="management-state" role="status"><h2>登录状态不可用</h2><p>当前会话、空间资格或管理查询权限无法确认。</p><a href="/auth/login">重新登录</a></section></>}
 const p=client.principal,label=workspaces().find(w=>w.id===p.workspace_id)?.label??'当前空间';
 const context=<div className="workspace-scope-bar"><span>当前工作空间</span><strong>{label}</strong><a href="/identity">切换与核验身份 →</a></div>;
 if(!section)return <>{context}{heading}<section className="management-summary"><div><span>身份</span><strong>已核验</strong></div><div><span>管理范围</span><strong>{label}</strong></div><div><span>办理方式</span><strong>查询</strong></div></section><Links/><details className="management-evidence"><summary>查看身份与查询依据</summary><p>人员引用：{p.principal_id}</p><p>空间引用：{p.workspace_id}</p><p>权限版本：{p.revision}</p><p>管理查询不会创建账号、修改授权或办理业务签收。</p></details></>;
 if(!supported.includes(section))return <>{context}{heading}<section className="management-state" role="status"><h2>尚未接入</h2><p>当前未配置{title}的可信查询来源。</p><a href="/settings">返回设置总览</a></section></>;
 const kind=section as ManagementSection;let s;
 try{s=managementSelection(kind,query)}catch{return <>{context}{heading}<section role="alert" className="management-state"><h2>查询条件无效</h2><a href={'/settings/'+section}>清除条件</a></section></>}
 let result:ManagementResult|undefined,users:UserManagementResult|undefined,workspace:WorkspaceManagementResult|undefined,organization:OrganizationManagementResult|undefined,error='当前记录无法读取';
 try{
  const response=await client.request(client.apiBaseUrl+`/api/v1/workspaces/${p.workspace_id}/management/${kind==='workspaces'?'workspace':kind==='organizations'?'organization':kind}`+managementQuery(s));
  const raw=await response.json();if(!response.ok)error=({USER_UNAVAILABLE:'指定用户不可见',USER_DIRECTORY_UNAVAILABLE:'用户目录暂时无法读取',ORGANIZATION_UNAVAILABLE:'目录组织不可见',ORGANIZATION_DIRECTORY_UNAVAILABLE:'组织资料暂时无法读取'} as Record<string,string>)[raw.code]??errors[raw.code]??error;else if(kind==='organizations'){if(validOrganizationManagement(raw,p))organization=raw}else if(kind==='workspaces'){if(validWorkspaceManagement(raw,p))workspace=raw}else if(kind==='users'){if(validUserManagement(raw,p,s))users=raw}else if(validManagement(raw,p,kind,s))result=raw;
 }catch{error='当前身份或记录无法核验'}
 const url='/settings/'+section,detail=kind==='members'&&!!s.memberId||kind==='users'&&!!s.userId,loaded=organization??workspace??users??result;
 const currentHeading=detail?<><div className="breadcrumb">系统设置 / {title} / {kind==='users'?'用户详情':'成员详情'}</div><div className="page-heading"><div><h1>{kind==='users'?'用户详情':'成员详情'}</h1><p>查看人员资料与当前空间的角色权限。</p></div><span className="tag green">仅查询</span></div></>:heading;
 return <>{context}{currentHeading}
 {s.from&&<a className="management-back" href={s.from==='organization'?'/settings/organizations':'/settings/workspaces'}><span aria-hidden="true">← </span>返回{s.from==='organization'?'组织资料':'工作空间'}</a>}
 {!loaded?<section className="management-state" role="status"><h2>{error}</h2><p>没有用示例记录替代查询结果。</p><a href={url+managementPageQuery(s)}>重新查询</a>{detail&&<a className="management-back" href={kind==='users'?managementUserListHref(s):managementMemberListHref(s)}>返回{kind==='users'?'用户目录':'成员列表'}</a>}</section>:<>
 {loaded.source==='SYNTHETIC_FIXTURE'&&<p className="management-source">当前展示合成验收数据，真实来源未配置。</p>}
 {workspace&&<ManagementWorkspace result={workspace} label={label}/>}
 {organization&&<ManagementOrganization result={organization}/>}
 {users&&<ManagementUsers result={users} selection={s}/>}
 {kind==='members'&&result&&<ManagementMembers result={result} selection={s}/>}
 {kind==='roles'&&result&&<ManagementRoles roles={result.data.items as Role[]} selection={s}/>}
 {kind==='audit'&&result&&<><div className="table-wrap"><table aria-label="空间审计记录"><thead><tr><th>时间</th><th>操作</th><th>业务对象</th><th>详情</th></tr></thead><tbody>{(result.data.items as Audit[]).map(a=><tr key={a.id}><td>{new Date(a.occurredAt).toLocaleString('zh-CN',{timeZone:'Asia/Shanghai'})}</td><td>{auditName(a.action)}</td><td>{auditName(a.objectType)}</td><td><details><summary>查看记录</summary><dl><dt>记录引用</dt><dd>{a.id}</dd><dt>操作者引用</dt><dd>{a.actorId??'系统'}</dd><dt>对象引用</dt><dd>{a.objectId}</dd><dt>操作标识</dt><dd>{a.action}</dd></dl></details></td></tr>)}</tbody></table></div>{!result.data.items.length&&<p className="empty">当前页没有审计记录</p>}<nav className="pagination" aria-label="审计分页"><a href={url+managementPageQuery({offset:0,from:s.from})}>最新记录</a>{result.data.next?<a href={url+managementPageQuery({offset:0,cursor:result.data.next,from:s.from})}>更早记录</a>:<span>已到末页</span>}</nav><p className="section-sub">展示当前空间已记录的业务审计，不包含程序运行日志。</p></>}
 <details className="management-evidence"><summary>查看本次查询依据</summary><p>查询时间：{loaded.evaluatedAt}</p><p>空间引用：{loaded.workspaceId}</p><p>身份版本：{loaded.authorityRevision}</p></details>
 </>}</>;
}
function Links(){return <div className="management-cards">{[['organizations','组织资料','核对当前空间绑定的目录组织'],['workspaces','工作空间','核对当前空间的访问范围与查询入口'],['users','用户目录','核对账号资料、身份关联与空间成员关系'],['members','空间成员','查看成员资料与当前角色'],['roles','角色权限','核对应用角色的可查看范围'],['audit','审计记录','按原始记录追溯当前空间操作']].map(([key,title,desc])=><a key={key} href={'/settings/'+key}><h2>{title}</h2><p>{desc}</p><span>进入 →</span></a>)}</div>}
function auditName(value:string){return ({MEMBER_READ:'查询成员',ROLE_READ:'查询角色',DATASET_CREATED:'登记数据集',Dataset:'数据集',Workspace:'工作空间',SOURCE_ADMITTED:'接纳数据来源',RESOURCE_REGISTERED:'登记数据资源',SYNTHETIC_REVIEW:'核对材料'} as Record<string,string>)[value]??'业务操作';}
