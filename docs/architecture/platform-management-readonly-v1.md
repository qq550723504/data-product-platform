# 新版系统设置：平台管理只读接入

2026-10-25。用户授权：复用 shuomi-ai-commerce 成员目录、角色权限和审计只读能力，适配新版设置，验证跨空间、撤权和分页。PR #363 保持 Draft，不合并。本文件拥有本增量的查询合同。

## Reuse Check

源代码固定 `qq550723504/shuomi-ai-commerce@e29058c56df4294e1c612b3a705ab767bc333188`，本地镜像目录仍名为 task-processor。复用其 `internal/integration/zitadel/membership/{client,query,permissions}.go` 的只读目录、完整过滤分页、前后权限确认，`internal/organization/membership/query.go` 的查询规范化，以及 Casbin 授权模型和角色投影方式。审计复用 accountaudit 的 scope-bound cursor、allowlisted projection 与失败不返回假空页原则；事实来源替换为本平台既有 AuditEvent，不能导入电商账本。

已有 OIDC/RP、PG 加密会话、principal binding、ValidateInTx/fence 和 AuditEvent 足够，继续复用。现有平台没有角色目录投影，使用源项目已采用的 Casbin v2.128.0（Apache-2.0、同进程无独立运维组件）展示应用角色的只读权限映射，不引入第二套 IAM、密码或通用授权服务。Casbin 仅在管理适配层使用；已有业务责任、Rights、交付等领域门禁保持。OPA 对本片固定应用权限映射无必要；不引入新通用 policy runtime。ZITADEL 接口在 Adapter 内，Core 不依赖 SDK。源项目写侧、邀请协议、邮件、资源账本、Auth.js 和 whole-application assembler 均不导入。

## 身份、事实与启用边界

- 新增显式 `platform-management` RP profile / `platform-management.v1` confirmation，复用同一个 RP 实现；scope、principal endpoint、session admission、路径 allowlist 与 business-acceptance 分离，后者契约不变。
- 每个查询需要 canonical actor + 当前 workspace grant 的 `PLATFORM_MANAGEMENT_READ`。Cookie/body/header 的用户、组织、workspace 均不能授予资格。单独增加该读能力，不把验收 READ 扩成管理员。
- DirectoryBinding 为服务器配置的 workspace → provider organization/project 映射；企业不是工作空间。目录返回的 provider user/grant ID 只作外部引用，不能冒充本平台 actor UUID。Native provider 的只读 credential 与人的 RP token 分开。
- 角色目录是应用配置的只读映射，名字/权限来自明确 catalog；不声称已经接入企业自定义角色。返回 `policySource=APPLICATION_CATALOG`。当前目录中未映射角色显示无已配置权限，不能放大 caller 资格。
- 实际目录与角色来源保持 UNCONFIGURED；仅隔离验收显式配置 SYNTHETIC_FIXTURE。无配置、权限缺失、错空间、依赖失败分别返回拒绝/不可用；不能用演示数据补齐，也不能返回假 0。
- 查询前后复用无缓存 OIDC resolver，并在 READ COMMITTED 只读事务复用 ValidateInTx。目录外部 I/O 不持数据库事务；结果公开前再次 introspect 访问令牌，并核对原 canonical actor、issuer/subject、workspace 和 authority revision，然后复查本地资格。读取中身份或版本变化时丢弃结果，不能用新资格追认旧读取；更短的重查有效期不得放大。远端权限在每页读取前后重新确认，无法证明跨系统分布式原子撤销。
- 目录客户端沿用既有OIDC适配器的标准transport克隆、证书校验与响应头边界；拒绝关闭证书验证、绕过标准TLS握手的自定义DialTLS、错主机ServerName或不可确认的自定义RoundTripper，不继承cookie/重定向配置。测试故障注入只在同包测试内进行，不作为产品构造参数。
- AuditEvent 仅按明确 workspace 投影 id、actor reference、action、object type/id、occurredAt；不返回自由文本、metadata、before/after 或 credential。历史不改写；没有新审计 owner 或业务写表。

## 接口与页面

`POST /internal/identity/platform-management-principal` 是私有身份确认查询，严格复用已有 envelope，不是业务 Command。

只读 GET `/api/v1/workspaces/{workspaceId}/management/{members,roles,audit,users,workspace,organization}`；成员和用户固定limit=25，成员offset≤10000，q≤200 UTF-8 bytes、state=active/inactive、role=配置目录中的角色；拒绝未知、重复、空或非法参数。目录详情通过明确 memberId 选择，不以当前页第一条回退。角色无分页（最多64个显式定义），workspace与organization分别只返回当前已授权空间的概览和绑定组织，均不接受参数。审计采用 scope-bound cursor（workspace、caller、asOf、最后 time/id），微秒时间和稳定 ID 顺序，limit+1 判断下一页；不声称跨页数据库快照。

响应绑定 workspaceId、principalId、authorityRevision、evaluatedAt、source、writeEnabled=false；仅已验证字段可进入 UI，权限必须命中显式自有键白名单，Object继承属性不能算权限。目录一次完整查询超过上限、超时，或检测到总数变化、短页、重复/乱序ID、错空间或原生筛选不符时必须失败，不返回部分成功。远端offset接口没有共同快照/修订号；相同总数下的并发替换不保证可检测，目录结果仅为查询视图，不是冻结成员事实，也不能作为后续写入授权。

新版 `/settings`、`/settings/workspaces`、`/settings/organizations`、`/settings/members`、`/settings/roles`、`/settings/audit`、`/settings/users`；筛选、明确详情、刷新和分页均保留 scope。切换空间经现有登录重新核验，旧会话/旧 revision/旧空间游标被拒绝。未配置仍展示可理解的空接入状态与返回入口，不展示合成人员。企业部门树、空间生命周期、运行日志和系统配置写入另属后续范围。

角色→相关成员→成员详情→返回列表沿用同一成员GET接口：角色入口以`role`打开首页，列表组合`q/role/state`筛选，提交筛选显式回到首页；`offset`只在分页和详情往返时保留。`memberId`进入独立详情查看状态，不在列表下重复展示详情。返回链接固定指向`/settings/members`，仅保留原筛选与offset，并用浏览器fragment定位原成员行；fragment不发往API，不接受任意return URL或客户端身份参数。详情失效时提供保留条件的明确返回，超出结果页时由用户显式选择保留筛选回首页，不静默改查其他成员/页面。页面跳转链接继续完整GET重验当前身份，URL状态不构成资格；切换空间从既有身份流程重建上下文，旧详情引用在新空间拒绝。浏览器前进/后退的本轮验收单独覆盖撤权后的拒绝，不把此前已展示的查询视图当新的授权。

默认预览继续无需登录；其管理入口显示真实来源未配置。独立管理 profile 只开放其设置查询和既有技术登录/退出路由，不开放 Agent 或业务 Command；4193 Agent 原运输方式不变。隔离浏览器使用合成 OIDC/目录夹具和独立 PG，不连接企业身份源、原库或 provider。

## 用户目录与身份关联

用户读取复用固定源版本的 `internal/integration/zitadel/membership/writer.go` 中 `ReadHuman` 的只读部分：原生 `user.read` 前后确认、精确用户ID、资源归属组织和Human类型核验。没有导入writer组装、邀请、账号配置或写方法。采用官方[GetUserByID](https://zitadel.com/docs/reference/api/user/zitadel.user.v2.UserService.GetUserByID)的User v2资料字段；既有验证TLS的transport仅增加固定 `/v2/users/{id}` GET，不支持任意路径、全局人员搜索或机器账号。没有新依赖、账号表、迁移或身份源。

`users`仅接受`offset/q/userId`。完整读取当前服务器绑定的organization/project成员关系，在10000条和10秒总预算内验证分页、总数、排序、作用域及同用户引用一致性，按原生user ID去重后执行字面搜索和25条分页。列表仅返回`id/displayName/loginName/membershipCount`，不做逐人资料请求，也不从成员关系推断账号状态。不同原生ID即使同名也不能合并。

明确`userId`必须先存在于本次完整可见关系集合，才允许请求原生资料；别的空间ID在请求资料前拒绝。资料精确校验user ID、owner organization、Human类型、有效日期及明确账号状态（active/inactive/locked/initial/deleted）；缺失、未知、错归属或权限撤回均不可用，不回退成员状态、缓存或示例。只投影最小资料，丢弃email、phone和其他原始字段。资料返回后重读该用户原先每条精确成员关系，重新检查作用域和应用角色映射；关系撤销/移出时丢弃整个详情，不能公开孤立账号资料。账号状态、成员状态和应用角色映射为三个独立事实，均不自动授予管理或业务资格。

可选`UserIdentityReader`只接收服务器管理的显式`native user ID → issuer/subject + basisRef`引用，再复用既有bindings.Store对精确workspace的当前actor/binding/grant查询。不得把native ID、登录名、邮箱或UUID外观猜成平台人员。未配置引用来源返回UNCONFIGURED；有来源但无显式匹配或当前关联已撤销/过期返回UNLINKED，均无actor/revision/依据/到期字段；只有当前有效关联返回LINKED及canonical actor/revision/依据/最短有效期。该结果是查询依据，不是当前caller或被查询用户的授权。独立验收仅为一名合成人员提供显式引用，该人员只有验收READ，没有管理READ；真实关联来源继续未配置。未做跨系统原子快照，详情不是冻结历史，后续查询必须重新核验。

用户列表与详情沿用现有设置壳、筛选和分页，不使用浏览器副本。返回固定`/settings/users`，只恢复`q/offset`并用fragment定位原行，不接受任意return URL、subject或workspace参数。用户详情显示独立账号状态、身份关联及当前空间成员关系，提供成员/角色查询；浏览器返回重新核验原详情。页码越界和不可见详情提供保留条件的显式恢复。原生用户引用、平台人员和关联版本放在折叠依据区，未配置关联明示“未关联 / 关联来源未配置”。

## 当前工作空间概览

本片复用已有OIDC/RP、workspace grant、ValidateInTx、查询壳及成员/用户/角色/审计流程；不新增IAM、通用租户组件、账号/空间表或迁移。ZITADEL等现有身份方案继续拥有登录与原生目录；平台只投影已有当前授权与服务器接入配置。没有业务需求要求再引入一套租户运行时，因此不新增OSS组件或自建工作空间生命周期。

`GET .../management/workspace`需要同一个`PLATFORM_MANAGEMENT_READ`及前后无缓存身份确认、当前PG只读事务门禁。它不要求人员目录已配置：将原共用目录前置要求限定在原成员/用户/角色/审计路径，空间概览独立使用身份来源。Handler必须显式配置`IdentitySource=CONFIGURED/SYNTHETIC_FIXTURE`，不能用原生目录来源推定身份来源；缺失返回SOURCE_UNCONFIGURED。身份、空间或revision在请求中变化时不公开概览，不接受URL参数、body、cookie或任意header提供身份/空间。

响应沿用身份绑定Envelope。`data.recordState=UNCONFIGURED`明确表示尚未接入正式工作空间档案；没有owner、quota、createdAt或生命周期状态字段。`confirmationExpiresAt`来自最后一次ValidateInTx的ExpiresAt，受caller身份及当前binding/grant有效期约束，必须晚于查询时点及响应组装时点；只解释本次核验，不是后续操作的访问租约。空间名称仅在Web侧由已验证的当前空间配置匹配，标记“名称来源：接入配置”；不得把企业组织、UUID外观、目录负责人或成员状态补成空间档案。负责人、配额显示未提供，创建、邀请、修改授权与归档不开入口。

`directory`只描述当前服务器配置是否满足既有查询路径要求：有效organization/project/source、Directory及Catalog齐备时为CONFIGURED，并返回折叠的外部引用；否则为UNCONFIGURED、source=UNCONFIGURED及null引用。`availability`始终NOT_CHECKED。`queries.members/roles`按同一完整配置判断，users另需UserReader、audit另需AuditReader；这些布尔值仅控制入口是否已配置，不能当作当前调用资格、远端健康或真实来源证明。概览不调用原生目录、账号资料、审计列表或外部provider；点击入口后由原接口独立核验。远端故障时概览仍可解释配置，原查询必须显示不可用，不给出“连接正常”或假空结果。

页面`/settings/workspaces`只展示当前会话空间，不新增全局空间目录。其他空间仍从既有`/identity → /auth/login?workspaceId=...`重做登录和资格核验，配置中的候选名称不证明调用者可进入。空间→成员/用户/角色/审计沿用完整GET，并携带唯一导航标记`from=workspace`；各列表筛选/分页/清除、明确详情、角色相关成员及审计游标分页均保留该标记，返回固定`/settings/workspaces`。标记不传入API、不选择workspace/身份、不接受任意return URL，未知值/重复值拒绝。切空间后返回只查询新会话的当前空间，复制旧用户/游标仍按既有边界拒绝；撤权或到期后刷新及历史导航不能用旧查询结果获取新访问。

## 当前目录组织资料

复用检查固定shuomi-ai-commerce的同一提交：没有找到独立组织资料Reader，复用其已经适配的验证TLS transport、严格路径白名单、原生权限前后确认、错误投影及成员/用户完整查询流程。组织对象由现有ZITADEL负责，新增最小OrganizationReader Port和现有Adapter方法；不自建组织库、组织同步、租户/部门引擎，不引入新的SDK或服务。采用官方[Organization v2 ListOrganizations](https://zitadel.com/docs/reference/api/org/zitadel.org.v2.OrganizationService.ListOrganizations)，请求与状态字段核对官方[query.proto](https://github.com/zitadel/zitadel/blob/main/proto/zitadel/org/v2/query.proto)和[org.proto](https://github.com/zitadel/zitadel/blob/main/proto/zitadel/org/v2/org.proto)；不使用已弃用的Admin GetOrgByID或全局列表回退。

`GET .../management/organization`只接受路径中的当前workspace，继续原PLATFORM_MANAGEMENT_READ、当前grant、读取前后无缓存OIDC与事务内ValidateInTx。组织ID只来自服务器DirectoryBinding.OrganizationID，是当前应用授权关系所在的effective组织，不能取被查询用户的home组织、workspace UUID或客户端organizationId。缺少OrganizationReader返回SOURCE_UNCONFIGURED，不以绑定引用补出资料。

原生请求仅允许固定POST `/v2/organizations/_search`，固定`query={offset:"0",limit:1,asc:true}`、`sortingColumn=ORGANIZATION_FIELD_NAME_NAME`及唯一`queries=[{idQuery:{id:当前绑定组织}}]`；该POST是查询，不是业务写接口。transport要求专用的可比较请求类型与选定组织完全一致，拒绝任意map、额外predicate、其他组织或宽查询。每次读前/后在显式组织上下文确认bare `org.read`，不剥离带冒号的权限范围。总预算10秒，响应有体积上限并沿用验证TLS和不跟随重定向。

响应必须有明确totalResult，0且无结果才是不可见；成功必须total=1、恰一条结果且ID精确匹配。额外/跨组织结果、未知状态、非法名字/日期、权限撤回或源错误均丢弃，不选择第一条回退。仅投影`id/name/state/createdAt/changedAt`；缺失名称/日期为null，不推定名称或创建时间；状态仅支持active/inactive/removed。忽略原生primaryDomain、metadata及resourceOwner等无关字段。object.resourceOwner可以描述组织或实例归属，不用于拼企业父子关系；本片作用域由显式组织ID及当前空间资格确定。

`departmentHierarchyState=UNCONFIGURED`为固定边界。目录组织、企业部门层级、法定主体和平台空间分别对待；目录启停状态是查询资料，不自动创建/归档空间、撤销成员或授予访问。没有企业来源时不返回部门树、负责人、法人关系或企业隶属事实。当前结果是查询视图，不是冻结历史或后续授权。

`/settings/organizations`显示绑定组织资料、明确的目录状态及缺失字段，链接到原当前空间成员和用户列表。导航标记扩展为严格的`from=organization`，与原`from=workspace`并列；只允许成员/用户/角色/审计页面使用，组织资料页本身仍无参数。搜索、分页、详情、原行定位、角色相关成员及错误页返回继续保持原标记，返回固定`/settings/organizations`，不把标记传到API、不接受任意return URL。切空间后重读新绑定组织，旧成员/用户引用按原接口拒绝。默认无账号预览仍显示真实来源未配置，合成组织只在独立验收profile展示。

## 验证

复用源目录查询测试并补齐本平台绑定：跨组织/project 响应、完整筛选后分页、读前/后权限撤回、身份在读取中撤回、旧 revision 不复活、跨空间游标、错误/空页区分、深链接和刷新、只读方法 allowlist、无 secret 字段。真实 PG 核验最小权限和身份 fence；独立 Edge 走真实 RP SDK 登录、多人员/多空间、分页和撤权。测试 fixture 的配置/撤权是隔离测试控制，不作为产品写入口。CI、合成浏览器与真实来源验收分别报告。
