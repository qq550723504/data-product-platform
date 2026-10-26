# 系统设置接入主服务：只读装配方案

2026-10-26。状态：**前两片共享组合及第三片目录产品配置已完成隔离合成验证；主服务可选组合默认关闭，真实运行未启用**。设计源码基线为`48cde34853f8da73bfb07936e9906252c1ce2f09`，设计文档提交`2a6f6099e7716dc4693db51147cfc88ab863b95c`的16项CI已完成并成功。后续用户授权实施第一切片，当前结果见[实施及验收记录](../product/platform-management-main-service-20261026.md)；本次实现HEAD的CI单独记录，不继承前述绿色。第二切片复用既有目录/日志Port完成共享组合兼容及来源观察门禁修复，见[第二切片验收](../product/platform-management-main-service-compat-20261026.md)；第三片已增加默认关闭的组织/成员/应用角色目录配置，见[产品入口验收](../product/platform-management-directory-product-20261026.md)；用户资料/身份关联和日志产品配置仍未开放。

真实来源UNCONFIGURED，默认新版预览未接入；原库、OIDC/RP和Agent API保留，业务写入口、模型和真实执行关闭，PR #363保持Draft、不合并、不部署。

## 1. 权威、当前范围与事实归属

本文件拥有组合根、默认关闭、配置/依赖及分片验收。七项管理GET、身份确认、DTO及审计独立装配由[管理只读契约](platform-management-readonly-v1.md)拥有；日志三项GET、双权限和引用由[日志契约](platform-runtime-logs-readonly-v1.md)拥有；真实对象、材料及确认职责由[来源清单](../product/platform-management-source-admission-20261026.md)拥有。装配、配置有效、合成验收和真实接纳不互相替代。

| 层次 | 当前事实 | 限制 |
|---|---|---|
| 主服务入口 | [cmd/api](../../apps/platform/cmd/api/main.go)在全量配置、主库、outbox、存储/队列/engine创建前选择独立QUERY_ONLY组合 | 代码已接入可选分支，不代表现有服务已切换或全量主服务成为只读 |
| 共享查询组合 | [Compose](../../apps/platform/internal/platformmanagement/service/composition.go)只注册既有设置/日志GET、私有身份确认和health | tests注入合成Port，产品profile不提供合成开关；不混挂业务mux |
| 当前产品配置 | [Profile](../../apps/platform/internal/platformmanagement/service/profile.go)支持身份、专用资格库、可选审计及可选组织/成员目录 | 目录仅显式配置时装配；用户资料/身份关联、日志产品配置未开放，真实来源仍未启用 |
| 审计 | 独立workspace→AuditBinding，Handler、workspace/configuration投影及Web校验同步解耦 | 不再依赖目录/Catalog；仍需当前身份、空间和PLATFORM_MANAGEMENT_READ |
| 实际验证 | 共享组合在独立PG/TLS/合成OIDC/RP与新版Web验证；产品入口另有真实main子进程的拒绝回退回归 | 不把测试宿主当真实部署；产品正向真实来源启动NOT_RUN |

## 2. Reuse Check与组合决策

复用已有principal/oidc、bindings.Store/ValidateInTx、PostgresGate、PostgresAudit、管理/日志Handler、BrowserIAM/PostgresSessionStore及httpserver.NewMuxWithReadiness。现有ZITADEL OIDC v3.45.0、Casbin v2.128.0、pgx v5.9.2和securecookie v1.1.2保持，无新依赖。按[ADR-0010](../adr/0010-reuse-commodity-capabilities.md)和[能力归属](capability-map.md)，不新增IAM、OPA、HTTP框架、审计账本、日志存储或采集服务，不Fork源项目。

现有全量cmd/api会创建多领域路由、存储/队列并执行健康入队，所以不能通过“最后追加管理Handler”获得只读保证。首片复用同一cmd/api入口的独立组合分支，创建自己的专用连接和封闭mux；现有Agent宿主和预览保持原路径。测试prepare/seed、控制端及合成源仅在tests，不进入产品组合。第三片目录配置复用该组合根；同进程全量与管理并存、用户/身份关联及日志产品配置、真实来源启用仍是后续范围。

## 3. 默认关闭和私有配置

唯一选择为`PLATFORM_MANAGEMENT_PROFILE`。未设置/空白时RunProfile立即返回未选择：不读取profile，不创建新增管理连接，不注册管理/确认端点；主服务原启动路径保持。非空却无效时，所选候选进程以固定安全错误退出，不回退全量配置、默认DSN、静态actor、原库或合成来源。

| 配置组 | 当前严格合同 |
|---|---|
| 根对象 | version=platform-management-query.v1，mode=QUERY_ONLY，listen为显式loopback IP和1024—65535端口；拒绝未知键、大小写别名、重复键、尾随JSON、非法UTF-8和过深结构 |
| tls | certFile/keyFile为绝对私有文件路径，使用标准TLS证书/密钥校验；不关闭证书验证 |
| identity | source=CONFIGURED及issuer、tokenEndpoint、introspectionEndpoint、clientId、clientSecretFile、audience、requiredScope=platform-management、allowedClientIds、caFile；沿用现有HTTPS、无缓存introspection及客户端约束 |
| database | urlFile、caFile、name、role均明确；URL必须匹配数据库/角色，有显式主机端口和密码，不接受查询参数覆盖；固定验证TLS、无fallback，覆盖环境PG连接参数，专用只读连接不继承主服务DSN |
| audit（可省略/null） | 配置时必须source=CONFIGURED及非空、无重复/零值的workspaces；独立AuditReader使用本资格库的audit_event最小投影；缺省不授audit_event读取权限 |
| directory（可省略/null） | 第三片支持显式组织/成员/应用角色查询配置，字段和边界见第10节；缺省不构造目录客户端或读凭据 |
| 未开放的配置 | 产品profile不接受用户资料/身份关联、日志Reader或SYNTHETIC_FIXTURE及夹具开关；独立产品入口验收可连接本地合成协议服务，但不是接纳真实来源 |

运行响应/错误不序列化profile，常规fmt诊断隐藏其私有字段。文件读取有大小/类型边界，启动先验证配置与私有材料，再创建专用连接、核验schema/最小权限、创建Resolver并注册路由。没有自动迁移、创建角色、seed、身份导入、授权修改、热重载或配置保存接口。运行配置变更需要明确停用并验证新候选。

GET /health/live仅表示进程存活；GET /health/ready执行必需数据库/schema/权限检查，不读取业务记录、不探测目录或日志，不证明可选来源已配置、健康或业务通过。错误配置不能报ready；运行故障不回退旧数据或其他来源。

## 4. 组件与数据库权限

| 组件 | 依赖和边界 |
|---|---|
| 身份/管理确认 | 复用OIDC资源端和当前actor/binding/grant；ManagementPrincipal仍只投影管理读，私有确认事务已显式READ COMMITTED/READ ONLY |
| Gate | 每次读取复用当前资格/fence；读取前后核验同一actor、issuer/subject、workspace、revision及最短期限；原期限不得被后查延长 |
| workspace/configuration | 身份可用即能解释来源未配置；不需要目录，不生成正式空间档案或健康断言；configuration仍为六个查询键 |
| audit | 当前空间显式AuditBinding.Source/Reader，独立于Directory/Catalog；成功及失败观察均经后置身份检查，错误无data，输出前保留原期限 |
| directory | 七项管理Handler复用现有Port和原生权限规则，第二切片已通过tests注入完成共享组合兼容；第三片产品profile仅在显式directory下组装Directory/Catalog/Organizations，Users/IdentityLinks仍不组装 |
| logs | 产品默认RuntimeSources=nil；第二切片仅tests注入合成Reader验证共享日志Handler，source/记录继续独立双权限及引用规则，不接真实日志 |
| Web | 复用现有管理RP、加密临时会话、同API origin的严格GET transport；无第二套IAM、无浏览器Bearer代理或业务写入口 |

API角色只能读取platform_actor_state、platform_subject_binding、platform_workspace_grant；配置审计时另需audit_event。启动核验目标库/角色、TLS、主库、只读默认、必要字段、数据库/非系统schema创建、ownership、角色继承/提升、表及列级读写、序列与非系统schema函数权限。额外业务表/列SELECT、业务列UPDATE也拒绝。检查非系统schema可执行函数；不声称清空PostgreSQL全部内置系统能力，应用从不提供通用SQL入口，只执行固定查询。只读GUC不替代真实GRANT边界。

RP角色只拥有platform_browser_transaction和platform_browser_session的必要SELECT/INSERT/DELETE，不拥有资格/审计/业务写权限；当前管理profile不启用backchannel logout，不预授其他RP模式权限。会话技术写不等于业务写。迁移、seed、ACL变化、撤权/到期与取消查询的控制仅属于新独立环境准备/验收角色，不进入运行API/RP。

QUERY_ONLY不创建outbox obligation/router、队列/健康入队、对象存储、worker、engine/provider或业务Command。退出只释放自己拥有的连接和候选进程；不删除、重启或迁移原库/旧容器/预览。

## 5. 路由与Web连接

封闭路由为管理七项GET、日志三项GET、私有POST /internal/identity/platform-management-principal及原health；识别路径的其他方法405，其他路径404。HEAD/OPTIONS不是GET兜底，编码/清理路径不得绕过，不能因是GET而允许Agent、下载、交付等副作用路由。私有POST继续严格body/Content-Type/单Bearer/无Cookie与当前空间核验；internal前缀不替代受控HTTPS和部署访问边界。

默认4193预览保持管理IAM未接入，现有Agent运输方式不变。独立验收使用新的API/PG/IdP/Web/浏览器上下文和临时会话，第一切片合成Web为https://localhost:4360，第二切片独立兼容Web为https://localhost:4370。未在现有Web进程切换profile；同hostname换端口不能隔离Cookie，真实运行前仍需确认hostname、会话及部署范围。真实地址/来源/配置位置/责任人继续待定。

查询private,no-store；刷新、返回和前进后退重验当前资格，不复活旧会话或结果。URL/游标只恢复条件和位置，不授予范围；不批量删除其他profile会话。

## 6. 未配置和错误处理

| 情形 | 当前行为 |
|---|---|
| 组合关闭 | 不装配新增管理路由/连接；默认预览不发起管理查询 |
| 所选配置无效、缺身份或库权限 | 候选启动失败，无全量/默认库/fixture回退 |
| 有身份、无目录 | workspace/configuration可解释；目录相关GET返回503 SOURCE_UNCONFIGURED，无假表格/假空页 |
| 无当前空间审计Binding | queries.audit=false，audit为503 SOURCE_UNCONFIGURED |
| 只有审计配置 | queries.audit=true、directory=UNCONFIGURED；审计不探测目录，Envelope.source取独立审计Source |
| 来源观察失败 | 审计、成员、用户、组织的失败/缺失/非法结果均先经共同后置门禁；当前资格有效时返回原来源错误，失效优先401/403，无data或内部错误，原最短期限不能续长 |
| 无日志来源 | source为200 UNCONFIGURED/NOT_CHECKED；entries/详情仍核验日志资格后按原合同拒绝，不能读stdout/Docker日志补齐 |
| 身份/空间失效或读取中撤权 | 原401/403/身份不可用语义，无记录；恢复资格不恢复旧RP会话 |
| 查询确实无匹配 | 只有当前门禁和完整查询成功才可空items，不能表示健康、无历史或验收通过 |

公开配置维持六键、NOT_CHECKED和当前管理范围，不新增日志键、连接信息、环境对象、全站写入/健康声明。Source配置是来源归属声明，不是已接纳真实来源或启用授权。

## 7. 审计独立装配

[管理契约](platform-management-readonly-v1.md#审计独立装配)是唯一查询语义入口。本片已同步Handler、两个入口投影和两个Web DTO校验，不用空Directory/Catalog伪造ready，也不以目录Source给审计背书。角色/组织/成员/用户约束不顺带拆除，不新增audit权限或改变caller grant。

审计游标仍是workspace/caller/asOf/微秒time/id绑定的Base64查询位置，不是签名凭据或历史快照。每页服务端按当前空间过滤并重新核验；25+1及同时间稳定ID排序不变。不UPDATE/DELETE历史AuditEvent，不把查询结果生成签收、验收或EvidenceSnapshot。

## 8. 验收条件与分片状态

当前证据详见[实施记录](../product/platform-management-main-service-20261026.md)。下表不把已有其他宿主的路线当本组合验收；第二切片[兼容证据](../product/platform-management-main-service-compat-20261026.md)与真实来源接纳分别记录。

| 编号 | 最小验收条件 | 当前分片状态 |
|---|---|---|
| A01 | 未设profile不创建新增管理依赖，不改变原主服务路径 | 单元/真实main拒绝回退及代码边界通过；未替换原宿主 |
| A02 | 无效/部分/未知/合成产品配置拒绝启动且不泄漏 | profile与main子进程回归通过 |
| A03 | 只允许封闭查询路由，无业务及队列/存储装配 | 组合HTTP与Edge通过；主入口在全量初始化前分支 |
| A04 | API最小表/列权限，RP仅临时会话，查询无业务事实变化 | 独立PG、真实拒绝DML/DDL与Edge通过 |
| A05 | 伪造caller/空间/Cookie、其他profile或普通用户拒绝 | 单元、OIDC/PG及Edge通过 |
| A06 | 全部可选来源关闭仍解释未配置，无假空记录 | 实际TLS HTTP/OIDC/PG共用组合及DTO单元通过 |
| A07 | audit-only无Directory/Catalog可读，独立Source及两个投影一致 | API、DTO与Edge通过 |
| A08 | 目录未接入/失败不影响审计，目录规则不放宽 | 首片无目录/权限故障及第二片完整51项设置Edge通过；审计仍独立 |
| A09 | 无审计空间/来源false；非法Reader/Source拒绝 | 组合、PG权限及查询回归通过 |
| A10 | 成功/失败读取中撤权、到期、revision及原期限拒绝 | 单元、真实PG与Edge通过，不续原期限 |
| A11 | 同微秒25/25/8无重漏，跨空间/跨caller游标拒绝 | 58条真实PG审计记录与Edge通过，不承诺快照/防篡改 |
| A12 | 本组合完整目录/日志双权限及跨空间引用兼容 | 第二片共享Compose的设置51项、日志19项Edge通过（含React管理的历史清除/重新核验补修）；原生规则、双权限和跨空间/caller引用保留 |
| A13 | 本组合已配置日志的读取中到期、绑定变化、26条核验 | 第二片读取中到期Edge通过；共享Compose binding/rule变化与原期限单元、26条原生lookahead/HTTP回归通过；均仅合成 |
| A14 | 新版审计分页/详情展开/返回/刷新/历史导航，及目录完整往返 | 首片审计18项通过；第二片目录角色→成员→详情→原筛选/第二页/选中行及组织/用户/配置往返Edge通过 |
| A15 | 默认预览/Agent原接入不回退，无新增写入 | 默认配置与Agent代码/transport保留；原4193/4493现场本轮未重演 |
| A16 | 新PG/IdP/API/Web/Edge隔离、原容器保留、旧失败留存 | 本片新环境与前后只读保留检查通过；无原库SQL/DDL |

## 9. 下一切片和授权边界

第一切片后续已获第二切片共享组合兼容的独立合成验证授权，并完成上述有限回归。后续第三片已获准实施目录私有配置及实际产品入口的独立合成验收；用户/身份关联和日志产品配置、同进程全站并存、真实来源/版本/空间资料/责任人落实仍为后续工作，不由本次代码或CI自动启用。实际企业来源、受控地址和部署范围仍待定；产品正向真实来源启动、原库DDL、真实授权变更、默认预览切换、模型、派发、业务写入、合并/部署均未执行。

出现身份/空间泄漏、真实保留元数据变化或UNKNOWN、权限扩大、错误源回退等必须停止，不降低门槛或补造成功。首次本地准备因权限检查SQL在非sequence对象调用sequence权限函数而失败，已改为显式CASE类型保护；实际写权限与元数据变化阻断均保留。失败和修复后验收分别留存，详见实施记录。


## 10. 组织与成员目录的产品配置

本节拥有第三片私有配置/组合根合同；[管理查询契约](platform-management-readonly-v1.md)继续拥有HTTP、原生Adapter及身份/范围语义，[第三片记录](../product/platform-management-directory-product-20261026.md)拥有实际见证。真实来源接纳仍由来源清单拥有，不能从配置状态推定。

| 字段/情形 | 严格行为 |
|---|---|
| directory | 可省略/null；存在时必须完整，source=CONFIGURED、provider=ZITADEL、workspaces为1—32项显式配置；无AUTO、SYNTHETIC_FIXTURE或测试开关 |
| workspaces[].workspaceId | 非零且不重复的UUID；只从已认证当前空间查该键，不接收客户端organization/project/endpoint选择器；无绑定即未配置，不取首项/默认项 |
| organizationId/projectId | 复用现有IsIdentifier；都是来源外部引用，不是caller principal、正式平台空间档案或企业法定身份 |
| origin | 独立HTTPS origin；拒绝userinfo、任意路径（根/空除外）、query（含空问号）、fragment、opaque及转义路径；原生Adapter不跟随重定向，仅执行现有闭集只读查询 |
| caFile/credentialFile | 显式绝对私有文件引用；CA须可解析、凭据非空且为无内部空白/控制字符的ASCII bearer值，受现有文件大小边界约束；缺失/错误即安全退出，无身份密钥、会话Bearer、环境或默认来源回退 |
| roles | 每空间复用现有NewCatalog/Casbin，1—64个不重复角色及既有查询权限白名单；不实现新策略引擎，目录未映射角色没有已配置权限，角色不能生成平台grant |
| 查询装配 | 仅Directory、OrganizationReader及Catalog；Users/IdentityLinks始终为空。audit按独立绑定，RuntimeSources仍为空；配置六键保持原合同 |
| 构造/健康/关闭 | 配置和文件校验先于新库连接；构造不请求来源，health/ready不探测目录或宣称来源健康；关闭只清理本组合拥有的目录空闲连接 |
| 配置修改 | 无热重载、公开保存或测试入口；需要明确新候选和运行授权后再启动，不原地改写业务历史 |

凭据文件名、token格式及CONFIGURED状态不能证明真实来源权限最小；实际只读授权仍需来源方材料和原生权限核验。Adapter每次查询前后确认所需裸read权限，只发既有闭集查询，不由格式校验宣称已经接纳真实凭据。

顶层仍使用PLATFORM_MANAGEMENT_PROFILE，缺省不创建新增依赖。产品没有合成来源开关；实际cmd/api独立验收使用此严格schema的CONFIGURED和loopback合成服务，报告/空间标签另行明确合成环境。CONFIGURED只表示已给出配置，不是可信来源接纳、业务通过或允许执行。

模板见[directory-query.example.json](../../deploy/management/directory-query.example.json)。其空UUID及PRIVATE/REPLACE、.invalid均为不可直接启用的占位符，实际配置须保存在受控位置，路径须按宿主系统替换。不得把凭据或完整私有profile提交仓库。

第三片实际cmd/api启动契约12项、Edge/Chromium各24项及独立PG回归已通过，旧目录测试宿主的结果不能替代此入口证据；当前HEAD完整CI在PR闭环评论核对。来源真实实例/组织/空间/责任/材料、实际启用与原库DDL仍待定且未执行，默认预览保持未接入。
