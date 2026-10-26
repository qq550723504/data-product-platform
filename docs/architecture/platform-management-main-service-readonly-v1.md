# 系统设置接入主服务：只读装配方案

2026-10-26。状态：**设计完成，未实施、未装配、未启用**。源码核对基线为PR #363的`48cde34853f8da73bfb07936e9906252c1ce2f09`。本轮只修改文档；真实来源UNCONFIGURED，默认新版预览未接入，原库、OIDC/RP和Agent API保留，业务写入口、模型及真实执行关闭，PR保持Draft、不合并、不部署。

## 1. 权威、交付范围和当前缺口

本文件只拥有主服务组合、配置关闭与启动失败边界、组件依赖和未来验收条件。七项管理GET、身份确认、DTO及审计解耦目标由[管理只读契约](platform-management-readonly-v1.md)拥有；日志三项GET、双权限和引用由[日志契约](platform-runtime-logs-readonly-v1.md)拥有；真实对象、材料及确认职责由[来源与责任清单](../product/platform-management-source-admission-20261026.md)拥有。本文不是第二份权限或HTTP合同，不将材料确认变成接入授权。

| 已核对的事实 | 当前结论与源码入口 |
|---|---|
| 独立宿主已有管理/日志查询 | [验收组合](../../apps/platform/tests/platformmanagement/main.go)组装Resolver、PG Gate、目录、Catalog、AuditReader与RuntimeLogHandler；其中prepare、合成人员、凭据及来源只属于tests |
| 主服务尚未接入管理组合 | [cmd/api](../../apps/platform/cmd/api/main.go)当前组装多领域Handler，并初始化对象存储、队列及健康入队；不能直接照搬这个启动过程声称只有查询 |
| 主服务尚无管理profile配置 | [Config](../../apps/platform/internal/platform/config/config.go)没有本方案的配置选择；下文profile及字段均为待实施设计，不是已有环境开关 |
| 审计存在不必要的目录依赖 | [Handler](../../apps/platform/internal/platformmanagement/http.go)在audit分支之前要求Directory/Catalog/organization/project；[配置投影](../../apps/platform/internal/platformmanagement/workspace.go)也把audit布尔值放在目录ready分支内 |
| RP已有单profile约束 | [Web身份配置](../../apps/web/lib/iam-configuration.ts)、[会话入口](../../apps/web/lib/iam-server.ts)及[管理transport](../../apps/web/lib/iam-core.ts)可以复用；同一Web进程选择一个身份profile，不能在现有预览上切换profile冒充全站已统一接入 |

本片目标是形成可复现的主服务只读组合，而非启用真实来源。未来首片实施/验证仅限另行获准的独立合成环境；当前没有主服务运行、数据库、凭据、来源或预览变更。

## 2. Reuse Check与装配决策

复用仓库已有`principal/oidc`、`bindings.Store`、`PostgresGate`、`PostgresAudit`、管理/日志Handler、ZITADEL Adapter、BrowserIAM/PostgresSessionStore及`httpserver.NewMuxWithReadiness`。现有`go.mod`已包含ZITADEL OIDC v3.45.0、Casbin v2.128.0、pgx v5.9.2及securecookie v1.1.2；本轮没有依赖变更。它们分别承担标准身份、既有角色投影、数据库与短时引用能力，不重写这些基础设施。

按[ADR-0010](../adr/0010-reuse-commodity-capabilities.md)和[能力归属](capability-map.md)，已有OIDC/RP及目录Adapter满足本片复用要求；不新增第二套IdP/IAM、OPA策略服务、审计账本、日志存储或采集服务。不Fork源项目，不复制tests中的迁移、seed、写控制端点和合成来源到产品装配。只需最小组合与依赖选择代码；本文不实施。

**首片选择：同一`cmd/api`入口的独立QUERY_ONLY组合。** 未来在独立进程/独立验收环境选择该组合，复用主服务代码入口；它不是第二套通用HTTP框架。现有全量服务和Agent预览继续使用原启动路径，不替换它们，也不将只读组合混挂到全量mux。若以后需要同进程并存，另行评审完整路由与凭据隔离，本片不承诺已完成。

不采用“在现有全量mux最后追加管理Handler”作为首片：这会继承业务路由、对象存储/队列初始化和健康入队，读数据库角色也不能阻止所有外部副作用。也不直接运行tests宿主作为正式入口：它具有独立库prepare和合成写控制逻辑。需要隔离的是组合根与依赖创建，不能只靠前端隐藏按钮。

## 3. 默认关闭、配置和启动顺序

拟议唯一启动选择为私有配置位置`PLATFORM_MANAGEMENT_PROFILE`：**未设置/空值时不选择管理组合，不读取该profile、不创建管理依赖、不注册其GET与身份确认端点**。它是本次设计提出的名称，当前代码尚不识别；不是本轮可执行的操作说明。现有默认启动路径和Agent API不变，也不因此获得“全量主服务只读”的声明。

非空时未来须先验证受控profile并选择QUERY_ONLY分支，且发生在现有全量`config.Load`、主库连接、outbox配置、存储/队列及engine初始化之前。配置错误使该候选进程启动失败；不得回退到全量服务、开发默认DSN、静态actor或合成来源。配置变更采用明确停用/新候选进程验证，不设计热重载、管理保存或启用API。

| 私有配置组（拟议） | 最小内容与规则 |
|---|---|
| 组合声明 | 精确`mode=QUERY_ONLY`、配置版本、专属监听/HTTPS部署边界；未知键/枚举、重复workspace和非法值拒绝。不能从Environment=dev推定合成资格 |
| 身份 | 显式IdentitySource、现有Resolver所需issuer、token/introspection endpoint、resource client、audience、scope和允许的RP client；全部沿用现有HTTPS/无缓存/无重定向验证。身份配置是必需依赖，缺失不能伪装成未配置来源的200响应 |
| 平台资格库 | 明确目标数据库与专用只读凭据引用，验证连接目标、数据库/角色与TLS证书，不允许明文或校验失败后回退；读取当前actor/binding/grant，不使用延迟副本或测试用户替代当前资格。禁止继承全量服务DSN或凭据 |
| 可选目录 | 每个获准workspace显式organization/project、Directory/Catalog及来源声明；Users/Organizations/IdentityLinks按原合同单独选择。完全不配置是合法关闭；声明接入却缺必需项/协议非法时启动失败，不能当未配置掩盖错误 |
| 可选审计 | 该部署明确允许的workspace集合、独立AuditSource和AuditReader；见管理权威契约的待实施审计解耦节。不存在目录绑定不影响审计来源配置判定 |
| 可选运行日志 | 独立RuntimeSources；缺失则按原合同UNCONFIGURED。已声明来源但无法满足稳定记录、空间与分页等准入时不得装成CONFIGURED；协议不受支持沿用UNSUPPORTED，不增加默认Loki/主机日志Reader |
| 凭据与合成边界 | 仅引用受控配置，不写入仓库/公开DTO/错误正文；产品组合拒绝SYNTHETIC_FIXTURE来源。独立合成验证通过tests专属组合注入已有Port，与真实装配规则分别验收；没有产品合成切换开关 |

未来启动顺序：选择组合 → 严格校验配置和装配一致性 → 创建专用最小权限连接 → 检查本组合必需schema/读权限 → 创建Resolver和显式来源依赖 → 注册封闭路由 → readiness可用。这里只做必要的PG/schema/权限检查，不在启动/health中读取目录、审计业务记录或日志，不自动迁移、创建角色、绑定人员或授予权限。所需既有迁移（包括000077/000078）只在另行获准的独立验收库准备；启动缺schema即失败。

实现验证时应让tests专属宿主/启动测试调用同一组合构造与校验函数，并注入合成Port，不另复制一份产品组合逻辑、不增加产品可用的fixture环境开关。产品profile解析器拒绝合成标记的子进程测试，与tests注入的HTTP/浏览器验证分别记录；后者只证明共享组合的隔离合成行为，不能冒充真实配置或产品进程已连接企业来源。当前两类验证均未运行。

`GET /health/live`只表示进程存活；`GET /health/ready`沿用既有就绪检查机制，只证明组合必需依赖，不证明可选来源已配置/健康，也不证明业务通过。关闭某可选来源不会要求其他来源齐备；已声明组件结构非法则不能进入ready。运行中的IdP/PG或来源故障沿用各自不可用响应，不回退配置或旧结果。

## 4. 组件依赖和最小数据库权限

| 组件 | 必需依赖 | 可选/不应依赖 | 不可越过的边界 |
|---|---|---|---|
| Resolver与身份确认 | 现有OIDC资源端、bindings.Store、当前空间grant | 人员目录、Catalog、AuditReader、日志Reader均非前提 | `ManagementPrincipal`仍只投影PLATFORM_MANAGEMENT_READ，不扩成管理员或日志权限 |
| 管理Gate | 专用PG只读Pool、Store、ValidateInTx/fence | 不依赖目录返回的角色 | 每次请求当前资格；外部I/O前后重验原身份/revision，失败丢弃结果 |
| workspace/configuration | 身份来源、Resolver/Gate | 目录/审计均可关闭 | 仅投影已装配状态；不提供真实空间档案或健康断言；configuration仍为六个查询键 |
| organization/members/roles/users | 原完整DirectoryBinding/Catalog；组织/用户另需原Reader | IdentityLinks可保持未配置 | 不借审计解耦顺带拆角色门禁；目录字段/分页/原生权限规则不变 |
| audit（待解耦） | 相同身份/Gate、显式空间范围、独立AuditSource、AuditReader | Directory、organization/project、Catalog不是依赖 | 仅当前空间AuditEvent最小投影；不读取目录、日志或自由metadata |
| runtime-logs | 身份/Gate；记录读另需独立日志资格、已准入RuntimeBinding | Directory/Catalog/AuditReader不是依赖 | source只需管理读；entries/详情双权限，沿用引用到期和读后校验 |
| Web RP | 已有OIDC/RP、独立会话存储、管理principal endpoint及固定origin | 不依赖目录在启动时可用 | Cookie由RP消费，API仅收服务端Bearer；不能接受浏览器注入Authorization或actor |

API最小数据库角色：`CONNECT`/schema `USAGE`及`platform_actor_state`、`platform_subject_binding`、`platform_workspace_grant`的SELECT；启用审计时另需`audit_event` SELECT。无审计时不得为方便预授该表权限。使用现有只读事务与角色级只读默认值，且角色不得有业务DML、DDL、CREATE、BYPASSRLS、可调用写函数、角色继承/切换或提升权限；只读GUC不替代真实GRANT边界。身份确认当前使用`Store`开启普通事务，未来需明确设置事务为只读或在专用连接默认只读下验证，不能宣称其现有代码已显式BeginTx(ReadOnly)。

RP角色只读写现有`platform_browser_transaction`与`platform_browser_session`两张临时表，按当前管理profile所需SELECT/INSERT/DELETE授权；不授予身份/资格/审计表写入。现有管理profile禁止backchannel logout，本片不为通用RP的其他模式预授provider-session表权限。RP技术会话写不等于业务写入口开放。迁移/seed与撤权测试控制属于独立环境准备角色，不能留在API/RP进程。

QUERY_ONLY组合不创建outbox obligation/router、Queue/Redis健康入队、对象存储、worker、engine/provider、业务Command或执行派发。关闭时不创建新增依赖；启用候选组合只创建上表所需依赖。关闭/退出只处理自己拥有的连接和候选进程，不清理原库、旧容器、原预览或其他会话。

## 5. 路由与Web连接边界

QUERY_ONLY组合仅注册管理七项GET、日志三项GET、私有`POST /internal/identity/platform-management-principal`和原health。路径/方法封闭白名单应在组合边界限制；识别路径的错误方法返回405，其他路径404；不得因某路径使用GET就允许Agent、下载、交付或其他业务处理器。HEAD/OPTIONS不作为GET或跨域放行兜底。身份确认POST是查询，不引入管理通用POST代理。

私有身份确认端点只供既有RP后端调用，沿用精确body/Content-Type/单Bearer/无Cookie约束和可信空间资格；`workspace_id`只是待核验目标，不能授予范围。部署需保留受控HTTPS和内部访问边界，不能因端点有internal前缀就认为已经网络隔离。应用端拒绝任意反代URL、来源URL、客户端tenant/actor及来源凭据，不把浏览器Cookie转发给Go API。Web允许的目录查询POST仅存在于服务端固定Adapter原生协议，不扩大浏览器/平台API白名单。

默认4193新版预览继续保持IAM未接入，显示业务语言的“真实来源未配置”，不请求候选API或填充夹具记录。将来另行获准的独立Web候选复用现有`BROWSER_IAM_PROFILE=platform-management`、principal endpoint同origin的只读API transport及加密会话，不改变现有Agent传输。不能直接在现有Web进程切换profile；同hostname换端口不能隔离Cookie，独立验收使用独立浏览器上下文和会话库，真实运行前还须明确互不干扰的hostname/Cookie部署范围。实际origin、域名、监听地址、配置位置和责任人目前均待定。

所有读取private,no-store，SSR/RSC/刷新/前进后退均重新核验；身份/空间/revision变化先清数据，恢复授权不复活旧会话。链接、原页码和游标只恢复查询条件，不赋予资格。装配关闭后旧RP会话再次确认失败，不能沿用客户端缓存；不批量删除其他profile会话。

## 6. 未配置、失败与页面处理

下表描述**待实施组合/解耦完成后的目标**；原API代码与既有验收结果本轮不变。所有有身份的接口先核验身份和空间资格，再解释来源状态，不能向未授权调用者披露某空间的来源配置。

| 情形 | 目标行为 | 页面和恢复 |
|---|---|---|
| 组合关闭 | 不注册新增管理/确认路由，API为404；不加载管理依赖 | 默认预览未接入，不发起管理登录或查询 |
| 组合被选择但身份/权限库配置不完整 | 候选启动失败、无业务回退；无ready假成功 | 不把配置错误改成正常空目录；不能绕过登录 |
| 已有可信身份，无目录 | workspace/configuration正常解释未配置；原目录查询503 SOURCE_UNCONFIGURED | “人员目录未接入”；无人员/组织/角色假表格 |
| 无独立审计来源或未允许该空间 | queries.audit=false；audit查询503 SOURCE_UNCONFIGURED | “审计记录未接入”；不能显示0条来代表未配置 |
| 只有审计已配置，目录关闭/故障 | queries.audit=true，directory仍UNCONFIGURED或原配置状态；audit独立查询 | 可查询审计，目录入口仍未接入或不可用；无目录探针 |
| 审计来源已配置但读失败 | 通过当前后置资格检查后503 AUDIT_UNAVAILABLE，无data | “审计记录暂时无法查询”，保留合法条件手动重查 |
| 无日志来源 | source为200 UNCONFIGURED/NOT_CHECKED；记录API双权限后503 LOG_SOURCE_UNCONFIGURED | 不创建时间查询/日志表格；不读stdout或Docker日志 |
| 日志不支持/来源读取失败 | 沿用LOG_SOURCE_UNSUPPORTED/LOG_SOURCE_UNAVAILABLE及原恢复规则 | 明确不可用，不降级查询范围、读宿主全量或补空页 |
| 无身份、无空间资格、撤权、到期 | 沿用原401/403/身份不可用语义，无data；错误观察也不能泄漏原结果 | 清除显示，重新登录/核验；不切换成默认管理员 |
| 合法已接纳查询确实无匹配 | 通过当前门禁和完整查询后才可返回空items | 仅表示本次可见范围无匹配，不表示健康、无历史或验收通过 |

公开configuration DTO保持六个键及NOT_CHECKED，不增加日志键、全站writeEnabled声明、健康总分、来源URL/配置对象。审计解耦只修正原audit键的计算，字段含义见管理权威契约；运行日志来源继续由原独立source接口解释。

## 7. 审计解耦的实施边界

本轮评审结论：**应拆除audit对Directory/Catalog/organization/project的前置依赖，保留身份、空间、PLATFORM_MANAGEMENT_READ及当前资格全部门禁。** 当前耦合是查询宿主复用目录分支导致的装配限制，不是AuditEvent事实归属或独立审计授权模型。

具体依赖、Envelope来源、未配置/不可用区分、配置布尔值和读后门禁的唯一目标定义见[管理契约的待实施审计解耦节](platform-management-readonly-v1.md#审计独立装配目标待实施)。实施时按该节一次性修改Handler与两个概览投影；不能只去掉前置if、伪造空Directory/Catalog或用目录Source给审计背书。角色、组织和用户仍按既有合同，不新增audit权限或改变角色展示为caller授权。

审计游标仍是当前workspace/caller/asOf/微秒time/id绑定的Base64查询位置，不是签名令牌、访问凭据或不可变快照。不得声称本次解耦已使游标防篡改，或照搬它作为日志受保护引用。所有页均在服务端按当前空间过滤并重验身份，历史AuditEvent不UPDATE/DELETE，不用查询结果生成签收依据或验收结论。

## 8. 未来验收条件（全部NOT_RUN）

下表是未来实现的必需通过条件，不是本轮运行结果。既有[设置/日志验收](../product/platform-runtime-logs-readonly-review-20261026.md)及[夹具保留修复](../product/platform-runtime-logs-fixture-preservation-20261026.md)仅为回归基线，不能替代主服务组合的新见证。

| 编号 | 场景 | 必须观察到的结果 |
|---|---|---|
| A01 | 未设置profile | 主服务原路径行为保持；新增管理/确认路由不可达；新增管理profile读取、管理专用PG连接及目录/IdP探针为0，不影响原Agent；不把原全量服务已有DB连接计为新增管理依赖 |
| A02 | 已选择但无效/部分配置、未知字段或产品配置含合成来源 | 候选启动失败，无全量启动/默认DSN/fixture回退；安全错误不泄漏配置值 |
| A03 | QUERY_ONLY组合启动 | 路由只含第5节；全量Command/Agent/下载/交付/模型/派发及GET副作用路由均不可达，Queue健康入队和storage/engine构造为0 |
| A04 | 专用API和RP权限 | API业务DML/DDL/写函数/越权角色拒绝；RP仅允许两张临时会话表的必要操作，身份grant及AuditEvent内容摘要查询前后不变 |
| A05 | 无可信caller、伪造Cookie/actor/空间、其他profile令牌 | 401/403或原严格参数错误，无记录和来源详情；管理确认不投影日志或业务写能力 |
| A06 | 身份已配置、所有可选来源关闭 | workspace/configuration可说明未配置；六个键符合实际依赖；目录、审计、日志不返回假空成功，无可选外部请求 |
| A07 | audit-only且无Directory/Catalog/organization/project | 当前空间审计可查；queries.audit=true，目录键false；目录调用计数0，Envelope.source取独立AuditSource |
| A08 | 目录正常/故障/权限撤销而审计可用 | 审计查询不受目录状态影响；目录路径仍各自拒绝/失败，不能因解耦绕过原生目录资格 |
| A09 | 审计未配置、未允许该空间、显式声明却缺Reader/非法Source | 未配置为SOURCE_UNCONFIGURED；非法组合启动拒绝；无AUDIT成功或默认目录Source替代 |
| A10 | 审计查询中撤权、到期或身份/revision变化，同时Reader成功/失败 | 后置资格优先，无data；不能披露原记录/存在性或用新资格追认旧读；不延长原期限 |
| A11 | 审计分页/相同微秒多记录/错误cursor | 25+1、稳定time/id顺序无跳漏；跨空间/跨caller或非法格式拒绝；不承诺跨页快照或把Base64称防篡改 |
| A12 | A/B两个workspace、跨空间目录详情与日志引用、缺任一日志能力 | 当前空间服务端范围有效，跨空间/伪造引用拒绝，日志记录双权限保持，不以目录角色补grant |
| A13 | 日志读取中撤权/到期、绑定变化、观察失败和同时间续读 | 复用原日志完整负向回归；不丢第26条核验、不复活旧引用、原期限绝对拒绝保持 |
| A14 | 独立Web角色→成员→详情→返回，审计更早记录/返回，刷新和历史导航 | 条件、页码及原行按原流程恢复；每次重新核验，撤权后无旧数据；目录关闭时审计仍可从配置入口访问 |
| A15 | 默认新版预览及现有Agent/业务演示 | 仍未接管理来源，Agent旧API路径/行为保持；没有替换profile、合成回退或新业务写入口 |
| A16 | 合成环境实际隔离与停止 | 明确独立PG/IdP/API/Web/浏览器上下文和owner；原容器完整保留比较，真实变化或UNKNOWN仍阻断；无原库访问/迁移/重启 |

A01—A13按配置/组合单元、现有Adapter/权限回归、真实独立PG及HTTP逐层验证；A14—A16用独立Edge与保留检查验收，不能仅靠mock或截图推断。静态链接、文档审查、旧HEAD绿色与新代码运行验收分开记录。

## 9. 分片、停止点和后续授权

| 阶段 | 最小交付 | 授权与退出条件 |
|---|---|---|
| 本轮设计 | 本方案、管理权威契约的审计解耦目标、来源清单交叉引用和一致性核对 | 仅文档；本轮结束不运行任何新组合 |
| 后续第一实施切片 | 组合选择/默认关闭、身份与workspace/configuration、封闭路由、审计独立装配；目录/日志无来源保持未配置 | 需明确实施授权；独立合成环境验证A01—A11、A15—A16及A14中的审计/目录未配置部分，完整目录往返不在此阶段报PASS |
| 后续兼容回归 | 在相同组合构造入口复用已有目录/日志Port的合成验收，完成A08、A12—A16及原目录/角色路线 | tests专属合成注入，产品不提供fixture开关；复用已有API，不新增业务能力；完成后才可判定整体验收通过 |
| 真实来源和运行启用 | 按来源清单逐项确认实际对象、责任/材料和有限见证，再明确启用目标 | 当前待定/NOT_RUN；开发、环境准备、真实只读见证、部署/启用分别授权，不由设计或CI继承 |

当前没有架构上无法复用现有能力的阻塞；未来实施的必需缺口是组合根隔离、独立审计来源与两个概览投影同步、最小数据库权限及新组合验收。实际来源/版本、workspace范围、域名、凭据配置位置与责任人仍全部待定。它们阻止真实接入/启用，不妨碍明确上述设计。存在契约冲突、身份/空间泄漏、真实元数据变化、权限扩大、未授权写入/原库访问或日志无法安全续读时必须停止，不降低门槛或以模拟成功替代。

## 10. 本轮核对结果

只修改本方案及三份既有文档。49个相对链接/锚点、A01—A16清单、既有9类来源/12项准入记录与仅文档差异范围核对通过；没有应用代码、测试、依赖、迁移、运行配置或进程变更。本轮未运行API/PG/浏览器验收，未来组合和审计解耦全部NOT_RUN；最新文档提交的CI另按该HEAD记录，不能继承源码基线的绿色结果。
