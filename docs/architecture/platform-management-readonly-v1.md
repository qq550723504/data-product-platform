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

只读 GET `/api/v1/workspaces/{workspaceId}/management/{members,roles,audit}`；固定 limit=25，成员 offset≤10000，q≤200 UTF-8 bytes、state=active/inactive、role=配置目录中的角色；拒绝未知、重复、空或非法参数。目录详情通过明确 memberId 选择，不以当前页第一条回退。角色无分页（最多64个显式定义）。审计采用 scope-bound cursor（workspace、caller、asOf、最后 time/id），微秒时间和稳定 ID 顺序，limit+1 判断下一页；不声称跨页数据库快照。

响应绑定 workspaceId、principalId、authorityRevision、evaluatedAt、source、writeEnabled=false；仅已验证字段可进入 UI，权限必须命中显式自有键白名单，Object继承属性不能算权限。目录一次完整查询超过上限、超时，或检测到总数变化、短页、重复/乱序ID、错空间或原生筛选不符时必须失败，不返回部分成功。远端offset接口没有共同快照/修订号；相同总数下的并发替换不保证可检测，目录结果仅为查询视图，不是冻结成员事实，也不能作为后续写入授权。

新版 `/settings`、`/settings/members`、`/settings/roles`、`/settings/audit`；筛选、明确详情、刷新和分页均保留 scope。切换空间经现有登录重新核验，旧会话/旧 revision/旧空间游标被拒绝。未配置仍展示可理解的空接入状态与返回入口，不展示合成人员。用户目录、组织树、空间生命周期、运行日志和系统配置写入另属后续范围。

角色→相关成员→成员详情→返回列表沿用同一成员GET接口：角色入口以`role`打开首页，列表组合`q/role/state`筛选，提交筛选显式回到首页；`offset`只在分页和详情往返时保留。`memberId`进入独立详情查看状态，不在列表下重复展示详情。返回链接固定指向`/settings/members`，仅保留原筛选与offset，并用浏览器fragment定位原成员行；fragment不发往API，不接受任意return URL或客户端身份参数。详情失效时提供保留条件的明确返回，超出结果页时由用户显式选择保留筛选回首页，不静默改查其他成员/页面。页面跳转链接继续完整GET重验当前身份，URL状态不构成资格；切换空间从既有身份流程重建上下文，旧详情引用在新空间拒绝。浏览器前进/后退的本轮验收单独覆盖撤权后的拒绝，不把此前已展示的查询视图当新的授权。

默认预览继续无需登录；其管理入口显示真实来源未配置。独立管理 profile 只开放其设置查询和既有技术登录/退出路由，不开放 Agent 或业务 Command；4193 Agent 原运输方式不变。隔离浏览器使用合成 OIDC/目录夹具和独立 PG，不连接企业身份源、原库或 provider。

## 验证

复用源目录查询测试并补齐本平台绑定：跨组织/project 响应、完整筛选后分页、读前/后权限撤回、身份在读取中撤回、旧 revision 不复活、跨空间游标、错误/空页区分、深链接和刷新、只读方法 allowlist、无 secret 字段。真实 PG 核验最小权限和身份 fence；独立 Edge 走真实 RP SDK 登录、多人员/多空间、分页和撤权。测试 fixture 的配置/撤权是隔离测试控制，不作为产品写入口。CI、合成浏览器与真实来源验收分别报告。
