# 工作空间运行日志只读契约

状态：页面、只读HTTP处理器、日志专属权限与独立PostgreSQL/合成来源验收已实施（2026-10-26）；真实来源未配置。初版核对基线为PR #363的`90643b8ed227c58fe386a5ee09b2758045db6cd6`。本稿拥有运行日志的查询约束与未来真实来源准入，包括身份、空间、投影、查询及恢复；[管理只读契约](platform-management-readonly-v1.md)继续拥有七项管理接口，[页面方案](../product/platform-runtime-logs-page-plan-20261025.md)拥有布局与交互。代码基线整体验收见[审查记录](../product/platform-runtime-logs-readonly-review-20261026.md)，跨模块材料与责任索引见[来源清单](../product/platform-management-source-admission-20261026.md)；主服务已实现默认关闭的第一只读组合，但其中日志来源仍未组装；真实来源和默认预览仍未接入。

本轮按新授权实现共享页面及[独立合成验收](../product/platform-runtime-logs-synthetic-acceptance-20261026.md)，默认Next入口固定未接入。已在独立管理验收宿主注册下述日志API，并在独立库运行migration 000078；原库和默认预览未迁移。没有真实来源Adapter或采集服务，也没有生产合成切换开关。详见[只读API验收记录](../product/platform-runtime-logs-api-acceptance-20261026.md)。真实来源保持UNCONFIGURED，业务写入口继续关闭；保留原库、OIDC/RP、Agent API和新版预览。PR保持Draft，不合并、不部署。

## 1. 目标与事实归属

主服务未来如何选择只读组合、默认关闭及创建依赖，由[系统设置主服务装配方案](platform-management-main-service-readonly-v1.md)拥有，第一切片已实现默认关闭的组合，日志来源配置仍属后续。日志三项GET、双权限、来源状态、最小投影和引用规则仍由本文拥有；审计解耦不扩展管理身份确认的capabilities，也不把日志键加入公开配置六项查询。本轮不新增真实Adapter、产品合成切换或采集服务，不改变默认预览。

用户在当前工作空间内查询一段时间的服务运行记录，查看明确一条记录，再返回相同查询条件和位置。记录仅解释技术运行观察，不据此重试任务、修改配置或判断业务通过。

| 对象 | 权威与含义 | 本片处理 |
|---|---|---|
| 运行日志 | 外部可观测性来源的技术观察；可能受采样、延迟和保留期影响 | 查询最小安全投影，不建立平台日志账本，不承诺永久保留 |
| AuditEvent | 平台业务操作审计事实 | 继续走原审计查询；不能把stdout、异常堆栈或日志级别补成AuditEvent |
| Execution、QualityAssessment、Agent任务 | 各自领域拥有的状态与历史 | 日志不是状态迁移、模型执行或任务成功证明，不从日志反推对象状态 |
| EvidenceSnapshot、签收、验收归档 | 原不可变证据及业务契约 | 日志、日志查询结果、截图与查询引用均不能充当签收、验收或冻结快照 |

没有日志不证明系统正常；异常记录也不自动证明业务失败。运行记录从来源过期后返回不可用，不回退最新日志、不改写原审计/业务历史。

## 2. Reuse Check与开源边界

- 本仓库`cmd/api`和`cmd/worker`使用`slog`输出；初版核对时management仅有AuditReader、目录与公开配置，没有运行日志查询Port；本轮新增独立查询Port和门禁，仍没有实际日志存储或真实来源Reader。`go.mod`里的间接OpenTelemetry依赖不代表已部署日志采集或查询。
- 复查固定`shuomi-ai-commerce@e29058c56df4294e1c612b3a705ab767bc333188`，其日志输出/轮转设施与accountaudit不构成本平台可复用的空间日志查询API。复用已接的身份、查询前后核验、错误模型和页面往返，不导入文件tail、全局日志管理器或电商运行时。
- 沿用[能力地图](capability-map.md)的外部可观测性归属。OpenTelemetry提供[日志数据模型](https://opentelemetry.io/docs/specs/otel/logs/data-model/)；其中记录时间、发生时间及任意Body/Attributes需分别处理，模型本身不提供本平台的授权或查询存储。
- 优先适配已经部署且满足本稿的日志服务。Grafana Loki是待评估候选，不是本轮选型或部署决定；其[租户隔离](https://grafana.com/docs/loki/latest/operations/multi-tenancy/)和[查询API](https://grafana.com/docs/loki/latest/reference/loki-http-api/)不能替代平台认证和来源接纳。原生tenant header必须由受信服务端生成，禁止浏览器选择tenant或多租户联查。
- 不自建collector、全文检索、索引库、轮转、长期保留、告警或日志任务队列。没有来源时停在未配置；不为了填满菜单增加第二套基础设施。选定新OSS前，另行补足license、版本、维护、部署成本、替换边界与Adapter合同；本稿不推定这些条件已满足。

2026-10-25已完成[Loki v3.7.9纸面准入评审](../product/platform-runtime-logs-loki-admission-20261025.md)，固定上游commit、许可证、维护与协议证据。结论为保留候选、当前不接纳实际接入：观察时间、稳定occurrence/续读及精确详情仍需来源和Adapter证明；真实实例、维护方与责任未定。该评审不弱化本稿规则，不新增部署、Reader或采集服务，真实来源继续UNCONFIGURED。

外部usage证据中的CONNECTOR_LOG仅是已接纳的normalized occurrence，不含原始运行正文，仍由[external usage契约](external-usage-evidence.md)拥有；不能把该导入/worker写链路挂到本片GET路径。

## 3. 来源接纳和当前配置

未来RuntimeLogSourceBinding由服务端配置：绑定当前workspace、单一来源实例/tenant、明确的允许服务集合、来源版本、空间标记规则、字段投影规则版本、时间范围限制和只读访问资料。原生ID、URL、selector和credential不返回Web。当前没有该配置或默认来源，也不从DirectoryBinding、工作空间UUID或演示标签猜测。

| 接纳条件 | 确认责任 | 必需依据与失败处理 |
|---|---|---|
| 来源和维护责任明确 | 来源运维方、平台接入负责人 | 指定真实实例、版本、负责人和只读能力；待定时UNCONFIGURED |
| 当前workspace有可信边界 | 业务空间负责人、身份/安全负责人 | 服务端绑定及可信producer注入空间标记的规则；无空间、跨空间、共享宿主级记录不开放 |
| 只读无业务副作用 | 来源运维方、接入负责人 | 查询不ACK、不提交checkpoint、不清理、不触发采集/任务；有副作用则不准入 |
| 安全投影可实现 | 安全负责人、Adapter负责人 | 最小字段与固定摘要映射，secret/PII/body/stack/URL反例；只有“已脱敏”口头声明不足 |
| 明确记录和分页可实现 | Adapter负责人、验收负责人 | 稳定occurrence身份、确定排序、可界定下一页、精确记录读取；仅有时间戳或任意内容hash不满足 |
| 有限读取与保留边界明确 | 来源运维方 | 时间精度、单次预算、保留规则及读取中删除/变化的返回语义；不得声称完整永久历史 |

来源配置状态仅UNCONFIGURED、CONFIGURED、UNSUPPORTED：CONFIGURED表示满足已登记的接口条件，availability固定NOT_CHECKED，不等于连接健康；存在配置但不能证明空间/身份/投影/分页条件为UNSUPPORTED。当前真实状态仍UNCONFIGURED。任何真实见证、采集、收费查询及部署均需另行明确范围与授权；不能因本稿通过而调用来源。若读取需业务成本/attempt写入，先另行协调该写入契约，本稿GET不得隐式代办。

## 4. 身份与工作空间边界

复用现有OIDC/RP、canonical actor、当前workspace grant、uncached resolver及ValidateInTx，不建设第二套IAM。来源状态查询只需现有PLATFORM_MANAGEMENT_READ；记录列表和详情同时需要PLATFORM_MANAGEMENT_READ与独立实现的PLATFORM_RUNTIME_LOG_READ。

**日志资格由独立增量migration 000078及CapabilityRuntimeLogRead实现。** 000077保持原样，新迁移只扩展严格CHECK，不修改任何已有grant，不授予管理查询员默认日志权；本轮只在独立验收库应用。原`platform-management.v1`身份确认仍只投影原管理读能力，不能悄悄扩展其语义；额外日志资格由实际记录API用同一当前grant独立核验，Web会话或角色名字不能证明它。

1. 在调用来源前无缓存解析本人身份，在同一只读事务核验两项能力、actor/binding/grant状态与revision及最短期限。来源状态接口仍只核验原管理读能力。
2. 从服务端选取当前空间的确切来源绑定；用户的workspace/tenant/organization header、日志属性、trace ID和cursor都不授予资格。producer空间标记必须来自受信注入，不能信任用户请求字段或未经证明的日志标签。
3. 用专用受限凭据查询同一来源和服务集合；不跨tenant、不查询宿主全量后在前端过滤，不跟随重定向，不接受任意provider URL或原生查询语言。
4. 来源I/O后重新解析原身份并比较actor、issuer/subject、workspace和revision，再在只读事务核验两项能力及有效期；重新读取绑定版本和投影规则版本，任一变化即丢弃整次结果。外部I/O不持数据库事务。
   成功和错误观察使用相同的后置门禁：来源失败、精确记录不存在或投影变化，也不能提前跳过当前身份/双权限/来源版本核验。当前资格失效时不得向原调用方披露记录存在、变化或来源错误的细节。
5. 撤权/到期、来源权限撤销、来源绑定变化或核验失败时返回拒绝/不可用且无data；不能凭新资格追认旧读取。此机制不声称实现跨来源与PG的原子撤销。

列表、详情、下一页、刷新和历史导航都走相同边界；不缓存身份、不复用旧门禁决定。日志GET不产生业务事实、Command、Outbox或任务，不在查询中“顺便”记录业务签收/执行/证据；访问遥测仍归现有可观测性owner，不用普通访问日志冒充AuditEvent。

## 5. Port和公开投影

服务端RuntimeReader只提供Query和ReadExact。请求包含服务端解析的Scope、固定时间窗、允许的服务/级别、受保护continuation或精确occurrence引用；不接受通用SQL/LogQL、文件路径、容器名、tenant或任意attributes。Adapter负责来源协议，Core Domain不得依赖Loki/OpenTelemetry SDK。来源配置投影由现有管理层组装，不通过查询原生状态/配置端点生成。

每页固定最多25条，整次来源读取预算5秒、响应上限256KiB；详情上限64KiB，无隐式retry。来源无法证明当前页查询完成、分页被截断、预算超限、重复/乱序记录、错空间或字段核验失败时丢弃整页，不返回部分成功或假空结果。

采用25+1读取的Adapter必须在截成公开页前核验全部至多26条记录，包括第26条的身份、确定排序、空间、窗口/过滤及字段投影；无效的多取记录不能用来证明“还有下一页”。本轮复用同一`ValidRuntimeBatch`校验，再截取25条；没有另造宽松的Adapter规则。

| 字段 | 约束 |
|---|---|
| rowKey | 当前workspace+来源绑定版本内稳定的公开不透明行引用，用于返回定位；不是资格或原生ID |
| entryRef | 服务端保护的短时精确读取引用，绑定caller/workspace/revision、来源/投影版本、occurrence身份、最小投影hash、查询上下文和期限；不含可用credential、不接受任意路径 |
| recordedAt | 必需的UTC纳秒精度字符串，表示来源观察到该记录的时间；缺失则拒绝记录 |
| occurredAt | 可空的实际发生时间；缺失为null，不用查询时刻或recordedAt冒充 |
| service | 固定APP_SERVICE或BACKGROUND_SERVICE；只有服务端绑定允许的来源可映射，不能回显宿主名/容器路径 |
| level | INFO、WARN、ERROR、FATAL、UNKNOWN；未知级别显示未分类，不补成功。V1不开放DEBUG/TRACE正文查询 |
| eventCode | 固定READ_COMPLETED、ACCESS_DENIED、READ_FAILED、DEPENDENCY_UNAVAILABLE、UNCLASSIFIED；必须由已接纳的结构化事件映射，不解析任意正文猜事件 |

摘要由Web固定业务词典生成，不接收来源的自由message或模板参数。首次范围无trace链接、原生属性、stack、请求/响应正文、姓名/邮箱、token/cookie、SQL/DSN、URL或原生配置字段；“掩码后原文”也不作为兜底。未知事件只显示未分类运行记录。rowKey用来源稳定身份做受保护的作用域化引用，不能用正文hash或行号代替来源identity；entryRef可以续期变化，不能作为稳定DOM定位键。

已有业务event_id、aggregate_id、attempt和请求关联ID均不自动构成日志occurrence。平台现有X-Request-ID可由调用方提供，不能据此建立可信日志身份、空间或权限；同一业务事件/attempt也会有多次日志发生。产出/来源责任方须分别证明空间上下文、独立记录身份和观察点，Adapter不能从消息、任意Payload或缺失字段猜测。没有可信空间的启动/全局调度记录不进入某个workspace页面；字段证据与最小责任边界见[产出侧源码核对](../product/platform-runtime-logs-loki-admission-20261025.md#7-现有日志产出侧源码核对)。本补充不修改现有请求关联、业务事件或日志输出实现。

ReadExact还需比较entryRef内的规范最小投影hash：相同原生身份若返回不同时间/服务/级别/事件投影，应报告记录变化，不用新内容冒充列表中那条记录。hash只核验本次短时查询的一致性，不是occurrence身份，也不建立平台历史快照；重新查询列表才可显式取得来源的当前投影。

已实施的响应沿用现有Envelope对caller/workspace/revision/evaluatedAt/source/writeEnabled=false的绑定，并为日志数据使用独立严格字段验证。列表data为`{window,items,next}`；详情为`{window,item}`；window为`{start,end,asOf,timeBasis}`，timeBasis固定OBSERVED，items/item使用上表完整字段。next为受保护cursor或null，无total、页码总数、运行总分或健康结论。

## 6. HTTP与查询上下文

以下路径已由RuntimeLogHandler注册在独立只读管理宿主。它们复用真实OIDC/RP与当前PG授权；正常预览保持未接入，原主服务及原库不因此启用来源。未配置状态不会返回假列表：

| 方法与路径 | 条件与返回 |
|---|---|
| GET `/api/v1/workspaces/{workspaceId}/management/runtime-logs/source` | 仅原管理读资格；不接受参数；data严格为`{state,availability}`，state为第3节三值，availability=NOT_CHECKED，无连接细节 |
| GET `.../management/runtime-logs/entries` | 双能力；只接受start、end、service、level、cursor；start/end必需且成对，最多24小时，start<end且end不晚于服务端当前时刻 |
| GET `.../management/runtime-logs/entries/{entryRef}` | 双能力；无额外查询参数，保护引用内恢复原窗口和过滤，精确读取并核对原记录身份和内容投影；不存在不换读邻近或最新记录 |

service可缺省或取第5节两值；level可缺省或取第5节五值。时间采用严格UTC RFC3339纳秒精度规范串，区间为`[start,end)`，按recordedAt过滤；不接受重复/未知/空参数、workspace/tenant选择、offset/limit、任意搜索表达式。当前UI的最近1小时默认值由Web服务端在首次查询前规范成显式start/end，刷新复用相同窗口，不把每次刷新偷换成新的“最近1小时”。来源状态未配置时不创建这个查询。

cursor绑定caller/workspace/revision、来源/投影版本、完整规范过滤、asOf、确定排序位置及期限；有效期最多15分钟且不晚于当前身份/空间资格/来源访问期限。entryRef不晚于所属查询期限。复用成熟加密/认证能力的用途隔离做法，不新增自研密码学；现有审计cursor的Base64编码不是加密或完整性保护，不能原样用于携带原生日志位置/引用。cursor和entryRef即使被持有，也始终需要当前身份和双能力。

引用在解码时、来源读取及后置门禁后、响应序列化后输出前均须仍在原期限内；到期边界为`now >= expires`。读取过程中到期返回409 `LOG_QUERY_EXPIRED`且无data，不因已经开始读取、生成新引用或重新授权而延长原查询期限。真实来源权限与PG之间仍不声称存在跨系统原子撤销。

排序为`(recordedAt, stable occurrence identity)`降序，精度不能经过JavaScript Number或毫秒截断后再比较。来源只有时间戳、同时间大量记录不能安全续读，或没有稳定身份/精确读取时返回UNSUPPORTED；不能减1纳秒跳过同时间记录，也不能靠内容hash将相同正文的两次真实发生合并。原生query_range的limit/direction本身不是已证明的稳定续读协议；Adapter准入需单独证明。[Loki范围查询](https://grafana.com/docs/loki/latest/reference/loki-http-api/#query-logs-within-a-range-of-time)仅作候选协议参考，不预设可直接满足本稿。

asOf固定本次可见读取边界，但不是跨页快照。晚到、重新索引或按保留策略删除仍可能改变可见结果；刷新恢复的是条件和位置，不承诺集合不变或完整永久历史。可见结果0条只表示本次受限来源查询无匹配，不能证明没有发生或系统健康。已知时间窗被保留策略排除返回范围失效，不用空页掩盖；无法完成读取返回不可用。

## 7. 异常和恢复

| 状态 | HTTP与代码 | 用户恢复 |
|---|---|---|
| 未配置 | source查询200 UNCONFIGURED；记录查询503 LOG_SOURCE_UNCONFIGURED | 无日志表格，返回设置；无样例补齐 |
| 不支持安全查询 | source查询200 UNSUPPORTED；记录查询503 LOG_SOURCE_UNSUPPORTED | 说明来源暂不支持当前查询，不能放宽scope/分页后重试 |
| 登录无效/读取中令牌撤销 | 401 AUTHENTICATION_REQUIRED | 丢弃记录，重新核验身份 |
| 无空间或缺任一读能力 | 403 LOG_ACCESS_DENIED | 丢弃记录；不自动换其他空间或角色 |
| 参数、引用格式被改写 | 400 INVALID_LOG_QUERY | 保留合法时间/筛选，用户显式清除非法位置 |
| cursor/entryRef到期、无法验证或来源版本改变 | 409 LOG_QUERY_EXPIRED | 返回当前条件首页，显式发起新查询；不自动回退 |
| 精确记录不在当前可见scope | 404 LOG_ENTRY_UNAVAILABLE | 返回原列表；不得泄露是否存在于别的空间 |
| 相同记录身份的最小投影已变化 | 409 LOG_ENTRY_CHANGED | 丢弃详情，显式重新查询列表；不覆盖原显示内容 |
| 来源确认已超保留范围 | 410 LOG_WINDOW_EXPIRED | 保留条件，用户显式缩短/修改时间窗 |
| 来源超时、故障、权限撤销或非法结果 | 503 LOG_SOURCE_UNAVAILABLE | 无data，保留合法查询条件，手动重查；不暴露原始provider错误 |

所有响应使用private, no-store；SSR/RSC、前进后退及服务worker/浏览器缓存均不能复活权限。页面不把旧显示结果包装成本次读取成功；来源返回页后身份/绑定变化应先清掉数据再显示错误。重启/密钥轮转导致短时引用不可验证时明确位置失效，不重造原生查询或放宽校验恢复。

## 8. 实施停止点和验收门槛

页面与独立合成验收已获授权并实施，不依赖真实来源准入。Web中的RuntimeLogReader为页面加载的归一化响应接口；IAM关闭的默认预览仍传入null，管理身份模式经runtimeLogAPI请求实际HTTP接口。Web不再补造日志能力，读取资格由服务端API在来源I/O前后用同一当前grant判断。合成Reader和来源宿主仅在tests目录，原生来源Adapter仍未落地。真实Adapter实施前仍需明确：真实来源候选及责任人、可信空间注入、最小字段规则、稳定记录/分页证明、只读无业务副作用、权限扩展与独立环境方案。真实来源接入、业务写入、模型、执行、合并和部署分别授权，不能捆绑推定。

[页面方案中的验收矩阵](../product/platform-runtime-logs-page-plan-20261025.md#4-异常验收矩阵)是未来用例规格，不是测试通过记录。本轮不新增运行对象/表或修改Agent、认证、发布、签收及历史契约。矩阵的合成页面验证与真实来源、PG事务及原生API见证须分别记录，不能相互替代。

2026-10-26后端实施：PostgresGate.ConfirmRuntime在同一READ COMMITTED只读事务、现有IAM共享fence下核对管理读及日志读，外部I/O后再解析OIDC并比较原身份/revision，再做同样事务核对。短时引用复用现有依赖gorilla/securecookie v1.1.2进行加密/MAC并做用途隔离，rowKey用标准HMAC作用域化；进程重启失效，无新引用账本/密码基础设施。sources绑定版本和最小投影hash变化均拒绝旧引用。当前没有真实来源配置入口。
