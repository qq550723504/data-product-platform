# PostgreSQL 接入 V1：#298 B0 选型与实施契约

> 日期：2026-10-10；源码基线：`87b80de64825f600649106449e3ec2fd60b42d1b`（#301 已合并，#297 已关闭）。
> Parent：[#298](https://github.com/qq550723504/data-product-platform/issues/298)；战略基线：[V2 蓝图](../product/full-lifecycle-governance-v2.md)、[ADR-0014](../adr/0014-full-lifecycle-governance-boundary.md)。
> 本文只完成 B0 的设计决策。B1 连接器运行验证、B2 Core 接纳、B3 用户端到端验收均未因此完成；没有部署或操作真实客户数据。

## 1. 有限目标和执行顺序

使用独立 PostgreSQL 来源、现有企业/租赁/能耗合成样本，在平台内完成一个批次的来源登记、抽取、清洗、质量评估、认证与 DIRECT_DATA 交付。主写者为当前接续 #298 的执行线程；当前独立分支只写本文，不触及 #294/#295 的 annotation 工作。后续代码沿单一串行 writer 推进，每个增量开工核对最新 main/活动 PR；不伪称已与不可见线程通信。

| 增量 | 有限成果 | 阻止进入下一步的条件 |
|---|---|---|
| B0 / 本文 | 固定场景、复用与选型、输出/恢复/安全契约、验收矩阵 | 关键边界有冲突或可实施性前提未记录 |
| B1 / 下一代码增量 | Hop→独立 PostgreSQL→隔离 staging 的真实连接器试验；固定样本、版本清单、负例 | 真实读取、字节/行语义、密钥隔离或恢复试验未通过 |
| B2 | provider-neutral 接入 Port、持久请求/attempt、三产物原子接纳、权限与追溯 | 不完整批次可进入生产、重复接纳、越权、unknown 被误报成功 |
| B3 | 来源详情/批次入口→既有加工/认证/交付；真实浏览器和非开发者解释验收 | 只跑准备脚本、fake adapter 或自动化但声称人工验收通过 |

B1 不依赖先建设完整 ingestion 平台；B2 不通过 mock 结果绕过 B1；B0 合并不关闭 #298。不新增父 Epic/平行调度/通用规则编辑器，不改行业指标和 Gold 主线。

## 2. 选型：先复用 Hop，SeaTunnel 保留为明确替代

**B1 选择 Apache Hop 2.19.0 作为首个验证对象；不是声明 PostgreSQL ingestion 已集成成功，也不是全平台永久默认引擎。** 当前 CI 已固定 `apache/hop:2.19.0` 做参考 CSV 聚合，已有 Hop HTTP client、ManagedProcessingEngine 与状态/错误映射。因此本场景先复用它，比直接增加另一个运行时更符合 ADR-0010。

| 比较项 | Apache Hop 2.19.0 | Apache SeaTunnel 3.0.0 |
|---|---|---|
| 官方能力证据 | Table Input 使用关系数据库连接和 SQL；Text File Output 输出文本/CSV；Hop Server 远程执行 | JDBC PostgreSQL 批抽取、S3File CSV sink、Zeta REST V2 作业接口 |
| 仓库复用 | 已有 client/bridge、ManagedProcessingEngine、CI 版本与参考流水线 | 本次已读基线没有对应接入实现，需新 Adapter 和运行验证 |
| 新增工作 | 数据库元数据包、凭据注入、批次 staging、来源接纳；旧 bridge 不能直接代替 | 上述接纳同样要做，另增加引擎部署、驱动/sink 依赖和协议验证 |
| 本切片决定 | 先做 B1，按下列门禁验证后用于 B2 | 保留替代，不并行部署；Hop 无法满足有限契约时再启用 |
| 不作的推断 | 已有 CSV smoke 不证明 JDBC、S3、远程重启恢复全部可用 | 文档有 jobId/2PC 不证明 Core exactly-once 接纳或授权已经成立 |

核对日官方 Hop 下载页为 2.19.0（2026-08-17），SeaTunnel 下载页为 3.0.0（2026-09-29）；比较仅限所读公开 Apache 发行版，不混用商业 edition 或 Next 文档。Hop 为 Apache-2.0；SeaTunnel 采用 Apache 许可证；分发时还要核对实际制品 LICENSE/NOTICE 与附带依赖，不能用主项目许可证代替全部依赖审查。[S1]、[S2]、[S3]

PostgreSQL 使用与仓库 CI 相同的 16 系列作为隔离来源，不使用 Core 控制库。pgJDBC 验证候选固定 `org.postgresql:postgresql:42.7.14`、driver class `org.postgresql.Driver`；官方下载页已列出该版本，许可为 BSD-2-Clause。**尚未验证它与候选 Hop 镜像的实际组合**；B1 须记录镜像 digest、Java 版本、实际 JDBC jar 版本/hash，保证 classpath 仅一份有效驱动；不能把“官方列出”写成“容器已包含”。[S4]、[S5]

Hop/SeaTunnel 都需要运行和升级维护，本文不编造内存、吞吐或成本比较。dbt 不填补本切片抽取边界，DolphinScheduler 不填补接纳语义；本阶段均不引入。若更换 provider，只替换外部执行 Adapter，保留本文 source/manifest/Core acceptance contract。

## 3. 已核对的复用点与不能直接复用的部分

| 源码证据 | 现状 | 对 B1/B2 的约束 |
|---|---|---|
| `.github/workflows/ci.yml` 的 hop-smoke | 固定 2.19.0、运行本地 CSV 聚合；是否执行还受路径筛选影响 | 新 JDBC smoke 必须进入实际执行条件，并输出自己的验证证据；旧 job SUCCESS 不代替它 |
| `workflow/application/processing_engine.go` | 同步执行与远程生命周期 SPI 分离 | 可复用远程传输接口，不伪造上游 DatasetVersion 来满足加工输入 |
| `workflow/hop/client.go` | register/start/status 分开；start 参数进入 GET URL；RecoverSubmission 调用相同 start helper | 参数只带非敏感 ID/路径；密码不得进入参数/URL；不据函数名推断恢复幂等性 |
| `workflow/hop/bridge.go` | 从既有 DatasetVersion 输入构建 URI，按 Execution 回收一个输出版本，读取上限 128 MiB | 它不是数据库接入与三产物接纳器；复用 client/存储/校验模式，不原样套用 Finalize |
| `metadata/`、DataResource/ResourceBinding | 已有元数据模块和稳定绑定方向 | 延续原模块，不另建 catalog；所需观察字段的实际读写接口由 B2/B3 补齐验证 |
| 三个合成 CSV | enterprise 6 行、lease 10 行、energy 14 行 | 固定源内容，保留别名、空串、日期不同格式及负能耗，不在抽取时清洗 |

以上是源码读取结论，不是本轮真实 runtime 测试。固定证据见文末 R1–R7。

## 4. 来源和 schema 冻结

来源连接由服务端允许清单配置；元数据只保存连接引用，不保存口令。数据库建议名 `dpp_ingestion_source`，schema `dpp_source`，固定三张表：`enterprise`、`lease`、`energy`。名称是本切片配置契约，不是现有部署声明。

| source slot | 允许列，顺序固定 | 导出排序 | 基准行数 |
|---|---|---|---|
| enterprise_raw | source_company_id, company_name, unified_social_credit_code, legal_representative, registered_address, entry_date, company_status, contact_name, mobile, email | source_company_id | 6 |
| lease_raw | lease_id, source_company_id, company_name, contract_start, contract_end, rent_amount, payment_due_date, payment_date, lease_status | lease_id | 10 |
| energy_raw | meter_id, source_company_id, company_name, reading_time, energy_kwh | meter_id, reading_time, source_company_id | 14 |

B1 将固定 CSV 字段按 text NOT NULL 导入独立来源，CSV 空字段保留为空字符串，不自动变 NULL；日期/金额先保留原始字符串。这样验证的是数据库 connector 与值保真，不冒充所有 PostgreSQL 原生类型兼容测试。NULL、额外/缺失列、类型改变分别有负例，当前 contract 拒绝而非静默转换。后续原生 numeric/timestamp/null 支持需明确编码契约。

基准样本的 `2020/06/08`、缺失信用代码/付款日期、负能耗 `-999` 必须在 RAW 保留。抽取只做固定字段选择与序列化，不 trim、不转日期、不填零、不去重。含手机号等列均来自合成 fixture，但日志仍不默认打印行内容。

首次批次是 **fixture-quiescent-v1**：装载后移除装载写权限，批次期间不启用源写入者；核对部署约束和 schema/样本指纹。观测到变更即拒绝本批，不能靠两次 COUNT 相同就声称一致性。本文不支持任意 live source 并发更新下的跨表事务快照。

SQL 由版本化模板固定表和列，不允许客户端传 SQL、连接串、文件路径或 engine job definition。一次只执行本三表集合；不扩展成任意 Connector UI。

## 5. 密钥、当前权限与元数据写入权

B1 使用隔离网络、仅允许来源读取的数据库账号；该账号只获得 CONNECT、目标 schema USAGE、三表 SELECT，不授予源写入/DDL/全库权限。启动/处理/产物接纳/交付都从可信服务端 principal 解析 workspace 和允许用途；技术账号连得上不等于业务请求有权抽取。B2 必须在实际 dispatch、恢复及接纳时重新核对所需当前权限，不沿用过期预检。

数据库密码、Hop 认证和存储凭据通过受控服务端环境/挂载或已有 secret 引用注入，仅在获准执行进程解析。禁止放入 `ManagedSubmitRequest.Parameters`、GET query、WorkflowVersion、manifest、Outbox、诊断/CI artifact 或 OpenMetadata 投影。Hop client 的 HTTP Basic 凭据与 JDBC 凭据分开。测试使用短期合成环境凭据；不读取或更改用户现有长期凭据。

| 字段/事实 | 唯一作者侧 | 同步/失联行为 |
|---|---|---|
| Core workspace、DataResource 归属、准入用途 | Core 受控 Command | OpenMetadata 显示 Owner 不覆盖 Core 权限 |
| 物理 schema/对象身份 | 来源系统；Adapter 记录观察 | 保存 connection/instance/object identity、schema fingerprint、observed_at；同名不自动同物 |
| 描述性术语、分类、Steward 展示 | 本切片由 OpenMetadata 作者侧维护，Core 只读展示 | 带 provider revision/观察时间与陈旧标记；未配置 Steward 显示未指定，不伪造人员 |
| 源抽取允许列、处理规则、质量/认证规则 | Core/版本化行业模板 | 目录分类变化只能触发复核，不能直接扩权或覆盖历史规则 |
| RAW/派生版本、认证/交付/证据 | Core | 向治理侧投影，不接受目录回写业务状态 |

OpenMetadata 不可用时，未知/陈旧状态必须可见；缺少所需授权证据即拒绝新动作。已有有效 Core 权限与冻结规则是否足够，由对应 Command 明确判定，不以目录可用性代替权限，也不因投影失败改写历史发布。

## 6. 输出与原子接纳

B1 先产生隔离 staging 的三个 CSV；B2 引入最小 ingestion operation 身份及三 slot 接纳关系，复用现有 Worker/Outbox 与存储设施。它不属于现有加工 Execution 的伪造输入链，不自建 connector/checkpoint/scheduler 平台。实际表名与 migration 序号在 B2 核对 main 后分配，不能抢占 #295 的迁移。

每次抽取尝试使用独占 staging prefix；CSV 固定 UTF-8、逗号、双引号转义、LF、包含表头、无 BOM，禁止动态时间/随机列进入数据内容。头部顺序按第 4 节；字节 SHA-256 对**实际导出文件**计算，不要求等于原始 fixture 的字节 hash。另以排序后的字段字符串值逐项比对 fixture，证明值保真。

最小产物 manifest 必须包含：
- contract version、workspace、source connection/instance、operation/request identity、批次标识；
- 模板与 source schema fingerprint、引擎版本、候选运行引用、观察时间；
- 恰好三个 source slot，每个 slot 绑定目标 Dataset、单个对象定位、byte size、row count、内容 SHA-256、列结构与顺序；
- manifest 自身的固定编码/hash；不含连接口令、原始行内容或可用下载凭据。

manifest 是 Adapter 的产物声明，不是认证。Core 从自己已持久化的请求取 expected source/workspace/slot/dataset/template，比对 manifest 并独立读取字节验证大小、哈希、CSV/schema/数量。对象仅可来自该次被允许的 staging prefix，不能接受任意外部 URI。

**原子性边界：** 将验证后的字节写入引擎不可覆盖的 Core-owned immutable keys，再在同一业务 finalize transaction 中接纳三项版本与强类型 slot bindings、批次 receipt、Audit/Evidence/Outbox；任一项缺失/冲突则不发布该批任一 RAW 为 READY。现有 UploadVersionService 自带事务时，需复用/抽取可参与该事务的内部步骤，不能顺序调用三次 Handle 发布 READY 后再补批次绑定。

存储写入不与数据库伪装成同一事务：只有所有对象已经写完且重新验证后才提交 READY；DB 失败保留待对账对象，不能宣称批次成功。存储 keys 不可覆盖、权限隔离和实际 provider 能力在 B2 验证，不能仅靠路径加 UUID 声称不可变。

幂等键按 workspace + operation identity；相同请求、相同完整 manifest 返回原 receipt；同键不同 source/config/内容一律冲突，不能靠换 slot 或改 key 悄悄覆盖。新实际抽取为新 attempt；同次结果的接纳重试不再次抽取，成本按真实 provider invocation 追加。

## 7. 提交与恢复决策

外部副作用前持久化 request key 与 physical attempt start fact。调用、异常、状态查询与结果接纳分开留证；状态仅为当前投影，不覆盖原始 start/observation。

| 情况 | 允许动作 | 禁止动作 |
|---|---|---|
| 确认尚未调用 register/start | 在 durable claim 下调用，并记录实际 attempt | 多个 worker 竞争时重复启动 |
| register 成功，已保存 remote ID，尚未 start | 复核权限后对确切 ID 执行 start | 以 pipeline name 代替唯一运行身份 |
| register 响应丢失，无 remote ID | 只做身份恢复/核对；无法确认则保持 UNKNOWN，先排除原工作再明确重试 | 盲目 register/start 第二份工作并把旧任务当不存在 |
| start 响应丢失 | 对 exact remote ID 查询；running 继续观察，terminal success 验证原产物 | 无依据重复调用现有 RecoverSubmission/start helper |
| status 短暂不可达 | 同运行对账，追加新查询 attempt | 改成 success 或伪造无任务 |
| Hop 重启/记录清理后查不到 ID | 只有可信完成证据与完整原产物匹配时才允许接纳；否则 UNKNOWN，确认无活动写者/隔离旧 staging 后再显式新 attempt | “查不到”直接解释成从未执行并重新启动 |
| 确定远端失败 | 保存失败事实；重试使用新 attempt/prefix，重新授权 | 覆盖旧失败记录或拼接两个 attempt 的三份产物 |
| 成功后接纳响应丢失 | 返回同 receipt，不重复版本；无输出数据权限时仅返回允许的状态 | 重新抽取、覆盖已有 READY 或重放旧交付 payload |

B1 必须验证 register/start/status 与真实 Hop Server 的语义。只有在特定版本证明幂等且身份连续时才能增加自动 start replay；本契约默认采用只读对账，不假设 Hop Server 的运行记录跨重启持久。**人工介入也是明确恢复结果，不得伪造自动恢复已完成。**

## 8. B1 的必需试验与 B2/B3 复用

| 验收编号 | 必需证据 | 归属 |
|---|---|---|
| PGI-01 | 真实 PostgreSQL + Hop 三表导出，6/10/14 行，按字符串值与冻结 fixture 比对 | B1 |
| PGI-02 | UTF-8 中文、引号/逗号/换行、空字符串保真；NULL/未知 schema 有拒绝负例 | B1 |
| PGI-03 | 查询 URL、日志、metadata、manifest 和归档 artifacts 不含测试凭据；只读账号写入被拒 | B1 |
| PGI-04 | register/start 响应丢失、状态断连、重启丢失记录的观察与恢复；没有第二次盲启动 | B1/B2 |
| PGI-05 | 缺 slot、额外 slot、重复 slot、错 source/workspace、错 hash/schema/path 全拒绝 | B2 |
| PGI-06 | 双连接并发接纳、任一 slot 失败、DB commit 响应丢失不产生半批 READY/重复版本 | B2 |
| PGI-07 | 源权限撤销、连接配置变更时 dispatch/recovery/admission 重新验证；unknown 不接纳 | B2 |
| PGI-08 | 三个 RAW 进入既有实体解析/Native 加工，质量失败不认证，授权撤销后交付阻断 | B3 |
| PGI-09 | metadata 失联呈现陈旧/未知；非开发者能完成并解释同一事实链 | B3 |

B1 先允许 Hop 写本地隔离目录证明 JDBC 值保真，但这仅满足部分 PGI-01/02；B2 前必须验证实际选定对象存储/staging 访问与对象隔离，不能把本地 CSV smoke 冒充 S3/MinIO 集成。

拟修改范围：B1 的合成来源 SQL/固定 `.hpl`/验证脚本放在 `examples/enterprise-activity/` 下独立接入试验目录，CI 仅增加该真实 smoke 所需路径与步骤；B2 在 `apps/platform/internal/` 增加最小 ingestion application/adapter，并按需要复用 dataset、metadata、platform runtime 与迁移；B3 扩现有 web 资源/生产入口和 acceptance。这些是后续有界代码范围，不是已创建文件清单。

## 9. 本轮结论、限制与停止条件

本轮完成源码/官方文档核对、单来源与 provider 验证顺序选择以及本有限契约。没有真实 connector/数据库/对象存储试验 PASS；PGI-01–09 当前均为 NOT_RUN。

当前执行容器没有 Docker/psql，尝试通过 git 获取仓库时因无法解析 github.com 失败；后续须在可运行的隔离 CI/授权目标执行 B1。该环境限制不改变已合并蓝图，不构成开始造第二套框架的理由。本轮只通过 GitHub connector 读写仓库文档，不部署、不接触真实数据。

若 B1 证明 Hop 的 JDBC/凭据/输出或恢复边界不满足要求，只补最小 Adapter 缺口或切换 SeaTunnel，并更新本文的具体选型段和相同验收矩阵；不继续同时建设两套 provider，也不降低接纳与权限要求。未验证的镜像 digest/JAR hash 是 B1 运行门槛，不伪造为已经冻结的运行制品。

## 一手资料与固定源码

官方链接用于组件能力和候选版本核对；运行制品还需 B1 实测。`latest` 页面读取时展示 Hop 2.19.0，不据此声称 URL 永久固定。

[S1]: https://hop.apache.org/download/
[S2]: https://seatunnel.apache.org/download/
[S3]: https://www.apache.org/licenses/LICENSE-2.0
[S4]: https://jdbc.postgresql.org/download/
[S5]: https://jdbc.postgresql.org/license/

- [Hop Table Input](https://hop.apache.org/manual/latest/pipeline/transforms/tableinput.html)
- [Hop Text File Output](https://hop.apache.org/manual/latest/pipeline/transforms/textfileoutput.html)
- [Hop Server](https://hop.apache.org/manual/latest/hop-server/index.html)
- [Hop PostgreSQL](https://hop.apache.org/manual/latest/database/databases/postgresql.html)
- [SeaTunnel 3.0.0 PostgreSQL](https://seatunnel.apache.org/docs/3.0.0/connectors/source/PostgreSQL/)
- [SeaTunnel 3.0.0 S3File](https://seatunnel.apache.org/docs/3.0.0/connectors/sink/S3File/)
- [SeaTunnel 3.0.0 REST V2](https://seatunnel.apache.org/docs/3.0.0/engines/zeta/rest-api-v2/)
- R1：[现有 Hop CI](https://github.com/qq550723504/data-product-platform/blob/87b80de64825f600649106449e3ec2fd60b42d1b/.github/workflows/ci.yml)
- R2：[Hop client](https://github.com/qq550723504/data-product-platform/blob/87b80de64825f600649106449e3ec2fd60b42d1b/apps/platform/internal/workflow/hop/client.go)
- R3：[Hop bridge](https://github.com/qq550723504/data-product-platform/blob/87b80de64825f600649106449e3ec2fd60b42d1b/apps/platform/internal/workflow/hop/bridge.go)
- R4：[现有 Processing SPI](https://github.com/qq550723504/data-product-platform/blob/87b80de64825f600649106449e3ec2fd60b42d1b/apps/platform/internal/workflow/application/processing_engine.go)
- R5：[enterprise fixture](https://github.com/qq550723504/data-product-platform/blob/87b80de64825f600649106449e3ec2fd60b42d1b/examples/enterprise-activity/data/enterprise.csv)
- R6：[lease fixture](https://github.com/qq550723504/data-product-platform/blob/87b80de64825f600649106449e3ec2fd60b42d1b/examples/enterprise-activity/data/lease.csv)
- R7：[energy fixture](https://github.com/qq550723504/data-product-platform/blob/87b80de64825f600649106449e3ec2fd60b42d1b/examples/enterprise-activity/data/energy.csv)
