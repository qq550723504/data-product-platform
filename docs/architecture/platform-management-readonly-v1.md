# 新版系统设置：平台管理只读接入

2026-10-25。用户授权：复用 shuomi-ai-commerce 成员目录、角色权限和审计只读能力，适配新版设置，验证跨空间、撤权和分页。PR #363 保持 Draft，不合并。本文件拥有本增量的查询合同。

2026-10-26完成状态核对，代码基线`259eff895bb50bf688b795e52210a062161620ba`：七项管理查询与运行日志的独立查询、权限和新版页面均已完成隔离合成验收。在该历史基线，二者注册在`apps/platform/tests/platformmanagement`独立验收宿主，`apps/platform/cmd/api`尚未装配，真实企业身份/目录/日志来源仍未配置，默认预览保持未接入。代码、隔离验证、主服务装配、真实来源与启用授权分别记录，不能互相推定。

当前状态及证据见[运行日志审查与系统设置整体验收](../product/platform-runtime-logs-readonly-review-20261026.md)；[来源、责任与首个候选核对表](../product/platform-management-source-admission-20261026.md)只拥有材料、确认职责与纸面准入记录，不另立HTTP、权限、状态机或启用合同。

2026-10-26第一实施切片：已在cmd/api接入默认关闭的独立QUERY_ONLY组合，并同步审计独立装配、两个概览及Web DTO校验。[只读装配方案](platform-management-main-service-readonly-v1.md)拥有组合/配置边界，下文“审计独立装配”拥有查询语义；[实施记录](../product/platform-management-main-service-20261026.md)分别记录共享组合的独立合成验收、产品入口拒绝回退及未运行的真实接入。产品profile首片不组装目录/日志，真实来源和默认预览仍未接入，不改旧证据或业务写边界。

2026-10-26第三实施切片：产品profile现支持可省略的组织/成员/应用角色目录私有配置，实际cmd/api完成独立合成验收，见[记录](../product/platform-management-directory-product-20261026.md)和[配置合同](platform-management-main-service-readonly-v1.md#10-组织与成员目录的产品配置)。不开放用户资料/身份关联、日志配置、管理写入或真实来源启用。

## Reuse Check

源代码固定 `qq550723504/shuomi-ai-commerce@e29058c56df4294e1c612b3a705ab767bc333188`，本地镜像目录仍名为 task-processor。复用其 `internal/integration/zitadel/membership/{client,query,permissions}.go` 的只读目录、完整过滤分页、前后权限确认，`internal/organization/membership/query.go` 的查询规范化，以及 Casbin 授权模型和角色投影方式。审计复用 accountaudit 的 scope-bound cursor、allowlisted projection 与失败不返回假空页原则；事实来源替换为本平台既有 AuditEvent，不能导入电商账本。

已有 OIDC/RP、PG 加密会话、principal binding、ValidateInTx/fence 和 AuditEvent 足够，继续复用。现有平台没有角色目录投影，使用源项目已采用的 Casbin v2.128.0（Apache-2.0、同进程无独立运维组件）展示应用角色的只读权限映射，不引入第二套 IAM、密码或通用授权服务。Casbin 仅在管理适配层使用；已有业务责任、Rights、交付等领域门禁保持。OPA 对本片固定应用权限映射无必要；不引入新通用 policy runtime。ZITADEL 接口在 Adapter 内，Core 不依赖 SDK。源项目写侧、邀请协议、邮件、资源账本、Auth.js 和 whole-application assembler 均不导入。

## 身份、事实与启用边界

- 新增显式 `platform-management` RP profile / `platform-management.v1` confirmation，复用同一个 RP 实现；scope、principal endpoint、session admission、路径 allowlist 与 business-acceptance 分离，后者契约不变。
- 每个查询需要 canonical actor + 当前 workspace grant 的 `PLATFORM_MANAGEMENT_READ`。Cookie/body/header 的用户、组织、workspace 均不能授予资格。单独增加该读能力，不把验收 READ 扩成管理员。
- DirectoryBinding 为服务器配置的 workspace → provider organization/project 映射；企业不是工作空间。目录返回的 provider user/grant ID 只作外部引用，不能冒充本平台 actor UUID。Native provider 的只读 credential 与人的 RP token 分开。
- 角色目录是应用配置的只读映射，名字/权限来自明确 catalog；不声称已经接入企业自定义角色。返回 `policySource=APPLICATION_CATALOG`。当前目录中未映射角色显示无已配置权限，不能放大 caller 资格。
- 真实企业目录、企业自定义角色与真实人员关联来源保持 UNCONFIGURED；已实现的应用角色目录来自 APPLICATION_CATALOG，其具体业务映射仍需明确，不等于企业角色已接入或调用者已有资格。测试宿主可显式配置SYNTHETIC_FIXTURE；第三片实际产品入口只接受CONFIGURED，但其独立loopback来源仍是合成服务，报告和空间标签明确标记合成，不能把配置状态当真实来源接纳。无配置、权限缺失、错空间、依赖失败分别返回拒绝/不可用；不能用演示数据补齐，也不能返回假 0。
- 查询前后复用无缓存 OIDC resolver，并在 READ COMMITTED 只读事务复用 ValidateInTx。目录外部 I/O 不持数据库事务；结果公开前再次 introspect 访问令牌，并核对原 canonical actor、issuer/subject、workspace 和 authority revision，然后复查本地资格。读取中身份或版本变化时丢弃结果，不能用新资格追认旧读取；更短的重查有效期不得放大。远端权限在每页读取前后重新确认，无法证明跨系统分布式原子撤销。
- 目录客户端沿用既有OIDC适配器的标准transport克隆、证书校验与响应头边界；拒绝关闭证书验证、绕过标准TLS握手的自定义DialTLS、错主机ServerName或不可确认的自定义RoundTripper，不继承cookie/重定向配置。测试故障注入只在同包测试内进行，不作为产品构造参数。
- AuditEvent 仅按明确 workspace 投影 id、actor reference、action、object type/id、occurredAt；不返回自由文本、metadata、before/after 或 credential。历史不改写；没有新审计 owner 或业务写表。

## 接口与页面

`POST /internal/identity/platform-management-principal` 是私有身份确认查询，严格复用已有 envelope，不是业务 Command。

只读 GET `/api/v1/workspaces/{workspaceId}/management/{members,roles,audit,users,workspace,organization,configuration}`；成员和用户固定limit=25，成员offset≤10000，q≤200 UTF-8 bytes、state=active/inactive、role=配置目录中的角色；拒绝未知、重复、空或非法参数。目录详情通过明确 memberId 选择，不以当前页第一条回退。角色无分页（最多64个显式定义），workspace、organization与configuration分别只返回当前已授权空间的概览、绑定组织与公开配置，均不接受API参数。审计采用 scope-bound cursor（workspace、caller、asOf、最后 time/id），微秒时间和稳定 ID 顺序，limit+1 判断下一页；不声称跨页数据库快照。

成员、用户和组织读取成功、失败、缺失及非法结果均先经同一无缓存后置身份/资格检查，再输出记录或原来源错误；读取中失效的caller优先401/403且无data。所有管理响应均按初次和后查最短期限输出，后查不能续长原期限。

响应绑定 workspaceId、principalId、authorityRevision、evaluatedAt、source、writeEnabled=false；仅已验证字段可进入 UI，权限必须命中显式自有键白名单，Object继承属性不能算权限。目录一次完整查询超过上限、超时，或检测到总数变化、短页、重复/乱序ID、错空间或原生筛选不符时必须失败，不返回部分成功。远端offset接口没有共同快照/修订号；相同总数下的并发替换不保证可检测，目录结果仅为查询视图，不是冻结成员事实，也不能作为后续写入授权。

字段核验分层沿用现有实现：原生目录响应中的无关扩展字段可由Adapter忽略，但不能投影或据此推定身份、范围或组织归属；组织/项目/人员引用、状态、时间、总数等关键字段按各接口既有规则核验，非法结果拒绝。公开管理DTO及Web字段集合必须精确，额外公开键拒绝；不能把原生无关字段一律改成错误，也不新增“未知字段”错误码。未知目录角色仍按上文投影无已配置权限，不与非法状态或越权混为一类；日志Adapter仍遵循独立日志契约，不沿用本段原生目录规则。

新版 `/settings`、`/settings/workspaces`、`/settings/organizations`、`/settings/members`、`/settings/roles`、`/settings/audit`、`/settings/users`、`/settings/configuration`；筛选、明确详情、刷新和分页均保留 scope。切换空间经现有登录重新核验，旧会话/旧 revision/旧空间游标被拒绝。未配置仍展示可理解的空接入状态与返回入口，不展示合成人员。企业部门树、空间生命周期和系统配置写入仍属后续范围；运行日志页面及独立只读接口已实施，单独由[日志契约](platform-runtime-logs-readonly-v1.md)拥有，真实来源未接入。

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

`GET .../management/workspace`需要同一个`PLATFORM_MANAGEMENT_READ`及前后无缓存身份确认、当前PG只读事务门禁。它不要求人员目录已配置：将目录前置要求限定在成员/用户/角色/组织路径；审计独立配置，空间概览独立使用身份来源。Handler必须显式配置`IdentitySource=CONFIGURED/SYNTHETIC_FIXTURE`，不能用原生目录来源推定身份来源；缺失返回SOURCE_UNCONFIGURED。身份、空间或revision在请求中变化时不公开概览，不接受URL参数、body、cookie或任意header提供身份/空间。

响应沿用身份绑定Envelope。`data.recordState=UNCONFIGURED`明确表示尚未接入正式工作空间档案；没有owner、quota、createdAt或生命周期状态字段。`confirmationExpiresAt`来自最后一次ValidateInTx的ExpiresAt，受caller身份及当前binding/grant有效期约束，必须晚于查询时点及响应组装时点；只解释本次核验，不是后续操作的访问租约。空间名称仅在Web侧由已验证的当前空间配置匹配，标记“名称来源：接入配置”；不得把企业组织、UUID外观、目录负责人或成员状态补成空间档案。负责人、配额显示未提供，创建、邀请、修改授权与归档不开入口。

`directory`只描述当前服务器配置是否满足既有查询路径要求：有效organization/project/source、Directory及Catalog齐备时为CONFIGURED，并返回折叠的外部引用；否则为UNCONFIGURED、source=UNCONFIGURED及null引用。`availability`始终NOT_CHECKED。`queries.members/roles`按同一完整配置判断，users另需UserReader；audit按当前空间独立AuditBinding配置判定；这些布尔值仅控制入口是否已配置，不能当作当前调用资格、远端健康或真实来源证明。概览不调用原生目录、账号资料、审计列表或外部provider；点击入口后由原接口独立核验。远端故障时概览仍可解释配置，原查询必须显示不可用，不给出“连接正常”或假空结果。

页面`/settings/workspaces`只展示当前会话空间，不新增全局空间目录。其他空间仍从既有`/identity → /auth/login?workspaceId=...`重做登录和资格核验，配置中的候选名称不证明调用者可进入。空间→成员/用户/角色/审计沿用完整GET，并携带唯一导航标记`from=workspace`；各列表筛选/分页/清除、明确详情、角色相关成员及审计游标分页均保留该标记，返回固定`/settings/workspaces`。标记不传入API、不选择workspace/身份、不接受任意return URL，未知值/重复值拒绝。切空间后返回只查询新会话的当前空间，复制旧用户/游标仍按既有边界拒绝；撤权或到期后刷新及历史导航不能用旧查询结果获取新访问。

## 当前目录组织资料

复用检查固定shuomi-ai-commerce的同一提交：没有找到独立组织资料Reader，复用其已经适配的验证TLS transport、严格路径白名单、原生权限前后确认、错误投影及成员/用户完整查询流程。组织对象由现有ZITADEL负责，新增最小OrganizationReader Port和现有Adapter方法；不自建组织库、组织同步、租户/部门引擎，不引入新的SDK或服务。采用官方[Organization v2 ListOrganizations](https://zitadel.com/docs/reference/api/org/zitadel.org.v2.OrganizationService.ListOrganizations)，请求与状态字段核对官方[query.proto](https://github.com/zitadel/zitadel/blob/main/proto/zitadel/org/v2/query.proto)和[org.proto](https://github.com/zitadel/zitadel/blob/main/proto/zitadel/org/v2/org.proto)；不使用已弃用的Admin GetOrgByID或全局列表回退。

`GET .../management/organization`只接受路径中的当前workspace，继续原PLATFORM_MANAGEMENT_READ、当前grant、读取前后无缓存OIDC与事务内ValidateInTx。组织ID只来自服务器DirectoryBinding.OrganizationID，是当前应用授权关系所在的effective组织，不能取被查询用户的home组织、workspace UUID或客户端organizationId。缺少OrganizationReader返回SOURCE_UNCONFIGURED，不以绑定引用补出资料。

原生请求仅允许固定POST `/v2/organizations/_search`，固定`query={offset:"0",limit:1,asc:true}`、`sortingColumn=ORGANIZATION_FIELD_NAME_NAME`及唯一`queries=[{idQuery:{id:当前绑定组织}}]`；该POST是查询，不是业务写接口。transport要求专用的可比较请求类型与选定组织完全一致，拒绝任意map、额外predicate、其他组织或宽查询。每次读前/后在显式组织上下文确认bare `org.read`，不剥离带冒号的权限范围。总预算10秒，响应有体积上限并沿用验证TLS和不跟随重定向。

响应必须有明确totalResult，0且无结果才是不可见；成功必须total=1、恰一条结果且ID精确匹配。额外/跨组织结果、未知状态、非法名字/日期、权限撤回或源错误均丢弃，不选择第一条回退。仅投影`id/name/state/createdAt/changedAt`；缺失名称/日期为null，不推定名称或创建时间；状态仅支持active/inactive/removed。忽略原生primaryDomain、metadata及resourceOwner等无关字段。object.resourceOwner可以描述组织或实例归属，不用于拼企业父子关系；本片作用域由显式组织ID及当前空间资格确定。

`departmentHierarchyState=UNCONFIGURED`为固定边界。目录组织、企业部门层级、法定主体和平台空间分别对待；目录启停状态是查询资料，不自动创建/归档空间、撤销成员或授予访问。没有企业来源时不返回部门树、负责人、法人关系或企业隶属事实。当前结果是查询视图，不是冻结历史或后续授权。

`/settings/organizations`显示绑定组织资料、明确的目录状态及缺失字段，链接到原当前空间成员和用户列表。导航标记扩展为严格的`from=organization`，与原`from=workspace`并列；只允许成员/用户/角色/审计页面使用，组织资料页自身仅可另接受下节定义的固定`from=configuration`导航参数。搜索、分页、详情、原行定位、角色相关成员及错误页返回继续保持原标记，返回固定`/settings/organizations`，不把标记传到API、不接受任意return URL。切空间后重读新绑定组织，旧成员/用户引用按原接口拒绝。默认无账号预览仍显示真实来源未配置，合成组织只在独立验收profile展示。

## 当前管理入口的公开配置

复用现有Handler、DirectoryBinding、workspaceOverview、OIDC/RP、PLATFORM_MANAGEMENT_READ及ValidateInTx。仅从当前服务器已组装的Reader/Catalog计算公开入口状态，不引入配置中心、环境查看器、第二套IAM或新OSS，不增加配置表、迁移、远端查询和编辑Command。范围是当前空间的管理接口，不是整个平台或所有模块的配置、健康、写入和执行状态。

`GET .../management/configuration`没有参数，复用当前身份前后无缓存确认、同一actor/issuer/subject/workspace/revision比较及两次只读事务门禁。即使没有配置目录，仍可以通过明确`IdentitySource=CONFIGURED/SYNTHETIC_FIXTURE`解释未接入状态；未知身份来源拒绝，不从目录来源猜测。撤权、到期、身份变化或门禁不可用时不返回data；Cookie、body、跨空间路径、未知/重复参数及非GET继续拒绝。

Envelope保持原合同，data严格为`scope=CURRENT_WORKSPACE_MANAGEMENT`、`mode=QUERY_ONLY`、`directory={state,source,availability}`、`queries={workspace,organization,members,users,roles,audit}`及`rolePolicySource`。目录state/source采用现有完整配置判定，availability固定NOT_CHECKED。workspace=true代表当前查询入口；members/roles沿用workspaceOverview判定，organization另需OrganizationReader、users另需UserReader；audit按当前空间独立AuditBinding配置判定。roles已配置时policy来源为APPLICATION_CATALOG，否则null。这些字段不表示远端健康、企业角色接入、当前操作授权或全站写入口状态。配置存在时原生服务故障仍显示已配置，进入各查询后独立核验并显示故障；没有原生读请求，也不生成成功/可用判断。

公开DTO禁止包含原生组织/project ID、原始或掩码URL、环境键值、连接字符串、文件路径、credential/session/token、密钥是否存在、部分密钥或整个配置对象。只允许显式组装本节字段；前端精确校验键集合、scope/mode、当前principal/workspace/revision、source及writeEnabled=false，拒绝额外字段和健康/全站能力宣称。最小身份Envelope中的canonical引用仍放在折叠查询依据，不是secret或外部配置。

`/settings/configuration`只显示当前管理入口范围、登录方式、会话范围、查询方式、应用角色来源及六个查询入口。登录与会话说明来自已经成功建立的现有platform-management身份流程，不显示另设账号或登录开关。没有表单、保存、修改、重载和启用按钮。默认4193未接真实管理服务时仍显示真实来源未配置，不用独立夹具的配置结果填充。

增加闭集导航`from=configuration`：只允许工作空间、组织资料、成员、用户、角色和审计页面使用；配置页本身不接受from。配置→工作空间/组织→下游查询，以及筛选、分页、明确详情、返回原行、角色相关成员、审计游标和错误页恢复都保留该来源，固定返回`/settings/configuration`。原from=workspace/organization行为保持。标记不进入API，不选择caller/workspace，不接受任意返回URL；刷新与历史导航仍完整GET重验资格。切换空间只显示新会话上下文，旧外部详情引用拒绝，恢复已撤权限不会复活旧会话。

## 审计独立装配

本节为第一实施切片已落地的当前合同。原`48cde348`上的audit目录前置检查及两个投影限制已移除，审计使用独立workspace→AuditBinding；目录相关路径和旧验收证据不被改写。Handler、两个概览与Web DTO均按本节同步，默认预览不启用该组合。

**评审决定：审计不依赖Directory、Catalog、organization/project或原生目录权限。** AuditEvent的System of Record是平台数据库，源项目审计复用的是查询模式；以目录是否配置决定平台审计可读，是组合层耦合，不能作为长期前提。查询仍必须具有同一可信caller、当前workspace及`PLATFORM_MANAGEMENT_READ`，不另建身份或权限引擎，不把应用角色显示权限当caller授权。

| 项目 | 当前合同 |
|---|---|
| 来源与作用域 | 由主服务受控装配明确启用的workspace集合、独立`AuditBinding.Source`及`AuditReader`组成；AuditSource仅CONFIGURED或tests专属SYNTHETIC_FIXTURE，不能从DirectoryBinding.Source、角色名或“Reader非nil”猜测。它是私有装配输入，不新增公开DTO字段或审计事实 |
| 启动和未配置 | 未显式启用或当前workspace无审计装配时，配置入口audit=false，实际audit GET在当前身份/空间核验后返回503 SOURCE_UNCONFIGURED。已声明接入却缺Reader、来源枚举非法或范围冲突时，候选装配启动失败，不静默当未配置 |
| 单次查询 | 沿用当前Resolver及PG Gate在读取前后核验原actor、issuer/subject、workspace、revision和有效期；AuditReader只接收服务端ws与caller。成功和读取错误均先通过同一后置身份/资格检查，再决定公开结果或错误；读取错误已改为与成功共用后置门禁；输出前仍按初次和后查的最短期限拒绝，不续期 |
| 运行故障 | 有效配置下Reader故障且当前资格仍有效才返回503 AUDIT_UNAVAILABLE，无data；读取中身份失效优先401/403/既有身份不可用，不能泄漏记录、存在性或内部错误。未知/错误cursor仍沿用400 INVALID_QUERY，不新增宽松恢复 |
| Envelope.source | 该audit响应取独立AuditBinding.Source；workspace/configuration响应仍取IdentitySource，目录响应仍取目录Source。不新增PLATFORM_DB等第三种公开枚举，不沿用未配置目录Source导致DTO非法 |
| 两个入口投影 | workspaceOverview.queries.audit和configurationOverview.queries.audit均按当前空间的独立审计装配判定；目录未配置但审计已配置时audit=true、directory=UNCONFIGURED，其他目录键按原合同false。两个投影、audit实际入口和Web DTO校验已同步修改 |
| 不变部分 | 七项GET、管理profile/version、确认capabilities、Envelope与data键、审计最小字段、25+1及(time,id)微秒排序、当前空间/caller游标检查均不变；只放开必要装配依赖，不生成/改写AuditEvent或任何验收/签收事实 |

审计来源状态是当前部署/空间的显式配置声明，不是连接健康、已接纳真实来源或后续执行授权；公开availability仍NOT_CHECKED。真实审计查询宿主、范围、只读凭据和维护责任继续待定。角色、组织、成员、用户的目录前置依赖不因本节自动拆除；运行日志仍按独立契约查询和双权限判断。

权限与验证范围见[装配方案](platform-management-main-service-readonly-v1.md)和[本片实施记录](../product/platform-management-main-service-20261026.md)。audit-only、目录权限故障、未配置源、独立Source、投影/DTO、跨空间/caller游标、同微秒分页及读取成功/失败过程撤权/到期已获独立合成验证；不继承旧51项结果，不声称Base64游标防篡改或跨页快照。第二切片完整目录/日志在共享组合的兼容回归已获独立合成验证，详见[第二切片记录](../product/platform-management-main-service-compat-20261026.md)；真实来源继续未接入。

## 验证

运行日志已完成共享页面、三项GET（来源状态、列表、精确详情）、独立日志读资格和隔离合成验收，见[独立只读契约](platform-runtime-logs-readonly-v1.md)、[页面流程稿](../product/platform-runtime-logs-page-plan-20261025.md)及[最新代码基线验收](../product/platform-runtime-logs-readonly-review-20261026.md)。日志查询不扩展本文件拥有的七项管理API、公开配置六个查询键或原`platform-management.v1`能力投影；日志记录API独立核验管理读与日志读双能力。migration 000077保持原管理能力，000078独立增加日志资格枚举；这两项实施记录不表示本轮为原库应用DDL。严格GET路径仍按各自合同分开，默认预览和真实来源保持未接入。运行日志、AuditEvent和业务执行事实分别处理，不能复用AuditReader冒充日志来源。

复用源目录查询测试并补齐本平台绑定：跨组织/project 响应、完整筛选后分页、读前/后权限撤回、身份在读取中撤回、旧 revision 不复活、跨空间游标、错误/空页区分、深链接和刷新、只读方法 allowlist、无 secret 字段。真实 PG 核验最小权限和身份 fence；独立 Edge 走真实 RP SDK 登录、多人员/多空间、分页和撤权。测试 fixture 的配置/撤权是隔离测试控制，不作为产品写入口。CI、合成浏览器与真实来源验收分别报告。

材料确认与运行授权分开。候选表中的撤权、到期、错空间和来源故障反例优先复用隔离合成回归；未来的有限真实只读验证不因列有这些反例而获得修改真实grant/来源权限、创建账号、注入故障或调用fixture控制端的许可。责任确认只证明其已确认的材料/范围，不授予caller资格；任何未来实际查询仍按本契约重新核验当前身份、空间与来源权限，材料过期或范围变化也不能用旧纸面结论追认。当前后续授权已覆盖第一实施切片及独立合成验证；真实接入另行授权。原库、默认预览与业务写入口保持。
