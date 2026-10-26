# Loki 运行日志来源纸面准入评审

状态：DESIGN ONLY，2026-10-25。基线为PR #363的`e287a67e0c6b18c1b3573ed6fa88a4a2f77d288b`。本稿只评估一个开源候选的接口与现有契约是否相容，不指定真实实例或责任人，不选定部署方案。查询/身份/字段规则仍由[运行日志只读契约](../architecture/platform-runtime-logs-readonly-v1.md)拥有，页面仍由[流程方案](platform-runtime-logs-page-plan-20261025.md)拥有。

2026-10-26状态说明：本候选的真实来源准入仍仅为纸面评审、结论未变；后续通用日志查询API、双权限及页面已在独立合成环境[实现并验收](platform-runtime-logs-readonly-review-20261026.md)，不代表Loki Adapter、真实实例或来源准入已经实现。下文评审基线的未实施/NOT_RUN记录不反向改写。

2026-10-26补充：在`d9e3d36263140e28c5287a7281f6949785d7338e`上核对本仓库日志产出，以及原固定复用项目的日志管理器，结果见第7节。只增加源码依据，不读取实例日志、配置secret或数据库，不改变第1节固定的Loki版本及13份上游证据，也不启用日志接入。

**结论：Loki可保留为日志存储与查询候选，当前不准入实际接入。** 原生接口尚不能证明观察时间窗口、稳定occurrence、同时间记录续读与精确详情全部满足本平台契约；真实来源和运维依据也未确定。不能只加HTTP查询就把菜单标为已接入，也不能降低契约、增加日志账本或Fork补齐。

本轮仅查阅公开官方资料和固定版本源码，不安装依赖、不启动Loki/采集器、不读取本机或容器日志、不调用真实日志API，不开发Adapter/API/权限/页面，不更新预览进程。真实来源保持UNCONFIGURED；原库、OIDC/RP、Agent API和业务历史保持，业务写入口、模型与真实执行关闭。PR继续Draft，不合并、不部署。

## 1. 固定评审基线与证据

- 上游：[grafana/loki v3.7.9](https://github.com/grafana/loki/releases/tag/v3.7.9)，GitHub release发布时间为2026-10-21 06:53:57（新加坡时间），非prerelease。固定commit为`d3b4a2f76bfc5625abee7ceb00b8896fa3c31549`，不是浮动latest。
- 同日仍有[v3.6.19发布](https://github.com/grafana/loki/releases/tag/v3.6.19)，可作为当时仍有维护活动的证据；不据此承诺长期支持、无漏洞或某版本适合生产。实际企业版本、镜像digest、SBOM与漏洞评估均待定，本轮不下载镜像或执行扫描。
- [源码清单](platform-runtime-logs-loki-admission-20261025/source-manifest.json)固定每份文件的commit、Git blob、SHA256和官方永久链接；原文仅用于本地只读研究，没有复制上游实现进产品。官方latest说明用于发现接口，涉及结论以固定commit源码为准。

## 2. 契约逐项核对

下表“支持、待证、缺口”只是纸面评审分类，不新增产品状态或表示已做接口/浏览器验收。

| 现有要求 | 固定版本证据与判断 | 本轮结论 |
|---|---|---|
| 身份及双能力门禁 | [鉴权说明](https://github.com/grafana/loki/blob/d3b4a2f76bfc5625abee7ceb00b8896fa3c31549/docs/sources/operations/authentication.md)将认证入口与tenant设置分开；mTLS也不能自动映射平台人员/空间 | 待证：继续由现有OIDC/RP、当前grant和未来日志读能力负责；tenant头不是身份 |
| 当前workspace隔离 | 来源tenant与服务集合只能来自服务器绑定；公开Loki能力不证明某个真实空间已有可信producer标记 | 待证：单tenant、可信空间注入、只读账户及网络入口均无实例证据，不采用演示A/B或目录组织推定 |
| 只读有界查询 | [固定HTTP文档](https://github.com/grafana/loki/blob/d3b4a2f76bfc5625abee7ceb00b8896fa3c31549/docs/sources/reference/loki-http-api.md)有GET query_range；[RangeQuery](https://github.com/grafana/loki/blob/d3b4a2f76bfc5625abee7ceb00b8896fa3c31549/pkg/loghttp/query.go#L442-L516)包含窗口、方向、limit等 | 原语支持，部署待证：只能由Adapter生成固定过滤；不开放原生表达式/interval抽样/指标结果/任意端点；5秒与体积预算仍需独立验证 |
| recordedAt为观察时间 | [OTLP转换](https://github.com/grafana/loki/blob/d3b4a2f76bfc5625abee7ceb00b8896fa3c31549/pkg/loghttp/push/otlp.go#L519-L524)可保留observed_timestamp；[时间选择函数](https://github.com/grafana/loki/blob/d3b4a2f76bfc5625abee7ceb00b8896fa3c31549/pkg/loghttp/push/otlp.go#L642-L652)优先原timestamp，再observed，再接收时刻 | 原生缺口：默认存储/范围过滤的混合时间依据不等于已证明的观察时间；接收点是否为所选来源观察点也须明确，不可直接猜测 |
| 稳定occurrence及rowKey | [Entry](https://github.com/grafana/loki/blob/d3b4a2f76bfc5625abee7ceb00b8896fa3c31549/pkg/loghttp/entry.go#L19-L25)为timestamp、line与metadata等，没有自动保证的平台occurrence身份 | 来源待证：必须已有可信、稳定、唯一的记录身份；同时间/正文hash/trace ID/浏览器行号不能代替；metadata可承载不等于已经具备 |
| 稳定25条续读 | RangeQuery没有opaque continuation；[迭代排序](https://github.com/grafana/loki/blob/d3b4a2f76bfc5625abee7ceb00b8896fa3c31549/pkg/iter/entry_iterator.go#L255-L281)的同时间次序涉及stream hash/labels，并非契约声明的occurrence次序 | 原生缺口：返回25条后自行排序不能证明漏掉的同时间记录可续读，limit也不是hasMore或完整性证明 |
| 精确ReadExact | 固定HTTP接口没有独立的occurrence读取端点；可考虑在已证明的单空间/窗口内对可信UID精确过滤，并核对唯一结果及投影hash | 条件性可适配但未证明：需真实字段/唯一性/保留语义；本轮不声称没有原生端点就永远不可适配，也不直接返回第一条 |
| 最小安全投影 | [OTLP Entry转换](https://github.com/grafana/loki/blob/d3b4a2f76bfc5625abee7ceb00b8896fa3c31549/pkg/loghttp/push/otlp.go#L575-L579)包含正文和metadata | 平台适配必需：只输出既定结构字段和固定业务摘要；不能透传Line/metadata、原始错误或掩码正文，也不能当审计事实 |
| 保留和异常恢复 | [固定保留说明](https://github.com/grafana/loki/blob/d3b4a2f76bfc5625abee7ceb00b8896fa3c31549/docs/sources/operations/storage/retention.md)描述Compactor与对象存储生命周期等因素 | 实例待证：记录可能不可见或被删除；不能把默认设置当企业保留承诺、不可变历史或冻结查询快照 |

原生HTTP成功只证明服务完成该次原生查询，不证明平台查询的时间依据、全量可见范围、身份、投影或下一页已满足。当前既不把某个真实绑定标为UNSUPPORTED（尚无绑定），也不返回CONFIGURED；真实配置状态仍是UNCONFIGURED。

## 3. 必须先解决的三个适配问题

### 3.1 时间依据不能偷换

假设一条记录发生于T减24小时，来源在T才观察到。用户按观察时间查询T附近窗口，而默认OTLP存储时间仍为发生时间；直接按相同窗口调用query_range会漏掉这条记录。把已返回结果按observed_timestamp重新排序无法找回根本没读到的记录。

需要来源维护方证明现有写入/查询路径如何保存并按观察时间完整检索，且保留原发生时间。观察点可由实际来源明确界定，但必须可核验且规则一致，不能将上述三种时间的优先级回退统一改名为recordedAt。不得猜测固定迟到上限、扩大到无界全量扫描，或现在新增自研collector改造。既有路径无法满足时，继续不准入；若业务确实要改为发生时间查询，须先独立评审权威契约和页面文案，不作为本次“接上接口”的隐式改动。

### 3.2 同时间续读不是limit加时间偏移

固定版本[官方LogCLI批处理](https://github.com/grafana/loki/blob/d3b4a2f76bfc5625abee7ceb00b8896fa3c31549/pkg/logcli/query/query.go#L177-L200)明确处理时间边界重叠，并对重叠记录已占满批次的情况报错。它是CLI遍历策略，不是本平台带当前身份、固定预算的安全cursor，可复用的是对边界的认识，不能复制进Web请求或用子进程调用。

假设同一纳秒有27条合法独立记录，原生limit=25。下一页将end设成该时间会排除剩余记录；保持重叠则可能再次命中同一批。客户端只去重或增加1纳秒并不能证明推进，更不能用相同正文hash合并真实发生。提高到某个更大的limit也不是无条件证明，仍可能超过5秒/256KiB预算或再次截在同时间组内。

需要来源/候选协议证明可完整界定边界组、稳定身份与确定推进；无法在既定预算内证明时必须拒绝，不能静默少返回。这里是原生接口与目标语义之间的待证缺口，不将其误称为已复现的Loki缺陷，也不为此增加新的日志索引账本、无限去重缓存或Fork。

### 3.3 UID与精确详情不能从正文拼出来

来源必须说明UID由谁生成、在哪个作用域唯一、重试是否保留身份、字段归一化是否冲突，以及缺失/重复/变化如何处理。可信UID可能通过已有structured metadata携带，但不是Loki自动为每次业务发生分配的事实。

未来ReadExact可在受信绑定和原查询窗口内读取该UID，必须证明结果唯一并核验原最小投影hash；0条为不可见，非唯一或变化拒绝。只要原生接口能通过Adapter满足合同，就无需单独开发记录数据库或Fork；现阶段没有来源样例和协议证明，不能认定已经满足。

## 4. 开源使用和运维成本

| 项目 | 当前核对 | 准入要求 |
|---|---|---|
| License | 固定版本[README](https://github.com/grafana/loki/blob/d3b4a2f76bfc5625abee7ceb00b8896fa3c31549/README.md#license)与[LICENSING](https://github.com/grafana/loki/blob/d3b4a2f76bfc5625abee7ceb00b8896fa3c31549/LICENSING.md)标明默认AGPL-3.0-only，另列Apache-2.0目录例外 | 先按实际使用/分发方式复核；不能把部分客户端目录例外扩成整个Loki或LogCLI。当前无代码导入，不在此给出法律适用结论 |
| 维护 | 已核对v3.7.9及v3.6.19的近期正式发布 | 维护活动不代替版本适用、漏洞/SBOM或升级回退方案，均待实际版本确定 |
| 部署 | [固定部署文档](https://github.com/grafana/loki/blob/d3b4a2f76bfc5625abee7ceb00b8896fa3c31549/docs/sources/get-started/deployment-modes.md)支持单进程运行多个组件，也有其他模式 | 优先已有服务；新部署会增加鉴权入口、持久存储、保留/删除、备份、容量及运维责任，不能当“只多一个GET”或声称零成本 |
| 接入 | 原生HTTP可由现有标准transport和未来Adapter隔离 | 无新增SDK、IAM、队列、日志账本或collector；不得让Core依赖上游类型 |
| 替换 | RuntimeLogReader的Query/ReadExact及最小公开投影是平台边界 | 原生tenant/selector/metadata与版本差异留在Adapter；完整性或时间依据不能映射时拒绝，不弱化合同迁就供应方 |

未评估其他供应方，不推荐因Loki存在缺口就立即部署第二套日志服务。若已有其他服务，仍优先按同一契约检查已有能力，再决定是否需要新OSS。

## 5. 来源维护方的最小材料包

这些是接口说明与字段证明，不要求真实credential、原始日志正文或客户材料。每项当前都没有真实来源证据；公开上游资料不能代填企业确认。

| 材料 | 谁确认 | 本轮状态 |
|---|---|---|
| 已有日志服务的产品/版本、维护方和允许查询的服务 | 来源运维、平台接入负责人 | 待定；Loki v3.7.9只是纸面候选 |
| 首个真实workspace、单tenant和可信空间标记注入链 | 空间负责人、身份/安全负责人 | 待定，不使用演示空间或目录组织推断 |
| 查询账户/网关的只读范围、TLS和禁止跨tenant设置说明 | 来源运维、安全负责人 | 待定，只提供配置位置/权限说明，不交密钥 |
| 观察/发生时间的保存与可查询语义，纳秒精度 | 来源运维、producer负责人 | 待定，不能用原生时间字段猜测 |
| 已有稳定UID、唯一性、完整边界组续读和精确读取说明 | Adapter/来源负责人 | 未证明，为技术准入缺口 |
| 最小结构字段映射及secret/PII/body/stack反例处理 | 安全、Adapter负责人 | 待定，可先用完全合成字段说明 |
| 保留/删除、来源异常及5秒/256KiB预算的有限验证方案 | 来源运维、验收负责人 | 待定；不先运行真实查询或制造任务 |

## 6. 纸面反例与下一步

| 假设输入 | 按现有契约的判断 | 证据等级 |
|---|---|---|
| 只有可查询URL和tenant头，没有当前人员/空间/日志读资格 | 不能接纳或开放UI；URL/tenant头不授予访问 | 官方协议事实加契约推论；未调用真实实例 |
| 发生时间在窗口外、观察时间在窗口内 | 默认原生时间窗口不足，需独立时间依据证明，禁止直接改名 | 固定源码推论；未运行采集器 |
| 同纳秒27条、相同正文多次独立发生 | 不能用25条加时间偏移/正文hash保证续读或身份 | 纸面边界反例，未声称实测厂商缺陷 |
| 有可信UID但重复结果或最小投影改变 | 详情拒绝，不回退最新或第一条 | 未来用例规格；NOT_RUN |
| 来源未配、来源失败、已超保留期、成功无匹配 | 四种状态分开，不用审计/执行记录或假空页替代 | 未来用例规格；NOT_RUN |

下一步最小范围是收齐第5节材料中的接口/字段说明，先核对时间依据、稳定身份和续读三个阻断项；没有实例时可以继续做协议层纸面核对，但不能将合成样例变为企业事实。只有证据证明满足现有契约后，才另行明确Adapter和独立合成验收范围。若已有路径不能满足，保留不接纳结论；不继续堆页面、制造虚假接入状态或建设新日志平台。

在本文2026-10-25评审基线，可核验的是公开资料版本、固定源码、契约一致性和文档链接，当时新日志API、双能力、源查询、浏览器路径和16组日志验收规格尚未实现/未运行。后续独立合成实施见上述状态说明；评审时CI不证明真实来源准入，后续合成验收同样不证明Loki或任何真实来源已接入。

## 7. 现有日志产出侧源码核对

核对日期2026-10-26。平台源码固定`d9e3d36263140e28c5287a7281f6949785d7338e`；复用项目仍固定`e29058c56df4294e1c612b3a705ab767bc333188`，没有切到其后续分支或导入实现。范围是受版本控制的API/worker日志装配、请求关联、Outbox输出、Schema错误处理、依赖/部署样例，以及复用项目的logging包；不从历史验收JSON、进程stdout、本机/容器日志或未提交配置推定实际部署状态。没有探测端口、运行配置加载器或连接服务。

**产出侧结论：现有源码提供了日志输出基础，但不能证明已具备可接纳的工作空间日志来源。** 缺口同时涉及产出上下文和查询协议，不能只在Loki Reader、前端筛选或字段重命名处解决。以下是源码能力判断，不是已经证明有敏感信息泄漏、部署故障或真实人员越权。

| 契约需要什么 | 当前源码证据 | 当前不能推定什么 / 后续边界 |
|---|---|---|
| 统一且可核验的输出规则 | API和worker分别建立局部[slog JSON logger](https://github.com/qq550723504/data-product-platform/blob/d9e3d36263140e28c5287a7281f6949785d7338e/apps/platform/cmd/api/main.go#L87-L89)；[Schema错误](https://github.com/qq550723504/data-product-platform/blob/d9e3d36263140e28c5287a7281f6949785d7338e/apps/platform/internal/metadata/transport/http/schema.go#L87-L91)调用包级slog入口，已查装配未见将两者统一的SetDefault | 不能因主入口使用JSON就假定所有包输出同一schema；复用当前框架，未来来源需证明其实际覆盖和版本，不在本轮改变全局logger |
| 可信workspace | [PublishedEvent](https://github.com/qq550723504/data-product-platform/blob/d9e3d36263140e28c5287a7281f6949785d7338e/apps/platform/internal/platform/outbox/publisher.go#L32-L40)没有强类型WorkspaceID，通用Payload不是可自动当作授权的空间字段；当前多处输出只带event/aggregate引用 | 不能从aggregate UUID、消息正文、任意payload字段、部署名称或目录组织猜空间；缺少可信空间的启动/全局调度记录继续留运维侧，不进入某空间UI |
| 声明清楚的观察时间 | 日志框架的记录时间与来源观察点没有现成接纳规则；复用项目[格式器](https://github.com/qq550723504/shuomi-ai-commerce/blob/e29058c56df4294e1c612b3a705ab767bc333188/internal/platform/logging/manager.go#L214-L246)配置RFC3339输出，未定义观察时间字段或纳秒保留规则 | 不能直接把time/timestamp统一改名为recordedAt，也不能由查询时刻补齐；先由接纳来源明确观察点、精度及与发生时间的关系 |
| 独立occurrence身份 | 同一[Outbox事件](https://github.com/qq550723504/data-product-platform/blob/d9e3d36263140e28c5287a7281f6949785d7338e/apps/platform/internal/platform/outbox/dispatcher.go#L146-L152)会有多种输出，[失败重试](https://github.com/qq550723504/data-product-platform/blob/d9e3d36263140e28c5287a7281f6949785d7338e/apps/platform/internal/platform/outbox/publisher.go#L331-L361)仍引用同一event_id；单次attempt也可能产生多条日志 | event_id/aggregate_id/attempt、event_id+level或正文hash都不能当一次日志发生的ID；不能修改不可变业务事件来凑日志身份或另建日志账本 |
| 关联标识与可信身份分开 | [WithRequestID](https://github.com/qq550723504/data-product-platform/blob/d9e3d36263140e28c5287a7281f6949785d7338e/apps/platform/internal/platform/httpserver/requestid.go#L14-L23)保留调用方X-Request-ID，缺失时才生成UUID；[HTTP装配](https://github.com/qq550723504/data-product-platform/blob/d9e3d36263140e28c5287a7281f6949785d7338e/apps/platform/internal/platform/httpserver/server.go#L25-L46)使用该中间件 | 可复用请求关联，不是认证、空间、唯一日志ID或后续资格；现有行为本身不等于漏洞，本轮不更改其语义 |
| 固定服务/安全事件投影 | [worker输出](https://github.com/qq550723504/data-product-platform/blob/d9e3d36263140e28c5287a7281f6949785d7338e/apps/platform/cmd/worker/main.go#L181)可能带地址/路径变量，错误输出包含error；复用项目[GetLogger](https://github.com/qq550723504/shuomi-ai-commerce/blob/e29058c56df4294e1c612b3a705ab767bc333188/internal/platform/logging/manager.go#L133-L136)仅增加component | component或程序名不自动等于可信服务/空间；原始msg/error/地址不进UI，不能从自由文本或级别推业务成功。无已接纳事件映射时沿用UNCLASSIFIED，不强行扩展事件码或业务状态 |
| 来源查询与保留证明 | 已查受版本控制的deploy装配未找到可交给本Reader的Loki/Collector来源绑定；go.mod只有间接OpenTelemetry相关依赖。复用logging包的[职责](https://github.com/qq550723504/shuomi-ai-commerce/blob/e29058c56df4294e1c612b3a705ab767bc333188/internal/platform/logging/README.md)是日志bootstrap支持，manager还有文件输出/轮转 | 依赖、格式化或轮转不等于日志服务和稳定查询；不能凭源码搜索断言机器上没有别的服务，也不能从默认轮转参数承诺企业保留期。真实实例/维护方仍待定 |

### 7.1 最小适配责任与停止点

| 责任边界 | 可复用内容 | 将来需要的证明，不是本轮实现 |
|---|---|---|
| 应用产出责任方 | 当前slog及已核验的业务调用上下文 | 哪些记录确实属于当前空间；独立日志身份与同一次运输重放/新发生的区别；事件映射来自受控字段/模板，不从业务payload或错误正文猜身份 |
| 来源/运维责任方 | 已有且满足要求的日志服务、成熟采集/传输能力 | 明确观察点、精度、稳定身份保存、可信空间注入、保留和只读查询；未确定前不自建采集器、不搬完整电商日志管理器或新建第二日志平台 |
| 查询Adapter与页面 | 已有OIDC/RP、当前资格、受限transport、固定摘要、查询/详情返回流程 | 对来源事实做边界核验与最小投影，不制造缺失字段、补成功、跨空间搜全量或把关联ID转成授权；前端不承担可信字段生产 |

下一步若进入实施评审，应先冻结一组有可信空间上下文的最小产出记录及其来源字段/运输语义，再明确独立合成环境的有限适配范围；不要求把全部启动、基础设施或第三方日志改成业务空间记录。任何产出字段、配置、Handler、权限、Adapter或采集改动都仍需明确实施范围，不因本次源码核对而执行。

当前源码能提供的字段依据已汇总到本表，不重复增加同类评审文件。此表不能代替真实来源材料：观察点、可信空间链、独立身份/续读和保留仍未确认，Loki继续只保留候选，真实来源UNCONFIGURED。没有读取运行日志、执行业务、修改Outbox事实或创建新审计/证据对象，新的日志接口与浏览器验收仍NOT_RUN。

2026-10-26后续实施边界：已按新授权实现[页面与独立合成验收](platform-runtime-logs-synthetic-acceptance-20261026.md)。这只验证共享页面及合成契约，既不改变本稿的Loki候选结论，也不补足真实来源/producer/分页准入证明。
