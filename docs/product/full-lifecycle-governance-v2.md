# 全生命周期数据治理平台产品蓝图 V2.0

> 状态：目标产品蓝图 / 新增范围的需求基线，不是已实现功能清单。
> 日期：2026-10-10；核查基线：`main@010a80c30b5a74a638faaaa27ebc7206634606f4`。
> 路线：[Epic #296](https://github.com/qq550723504/data-product-platform/issues/296)；本轮文档工作：[A / #297](https://github.com/qq550723504/data-product-platform/issues/297)。

## 1. 产品定位

Data Product Platform 的长期定位升级为**全生命周期数据治理与可信数据生产平台**，服务企业、政府及行业数据场景。既帮助组织管理已存在于外部系统的数据，也帮助其生产、认证和交付新的可信数据集。

核心价值：数据找得到、含义说得清、责任有归属、规则能执行、加工可追溯、使用有依据、退出有记录。平台软件提供治理能力，不替代组织的数据负责人、业务专家、安全及合规责任人。

Certified Dataset、Data Product 与 Gold Dataset 是其中的成果形态，不是所有数据纳管的前置条件。现有园区场景仍是参考实现，不把 Core 限定为企业经营数据，也不宣称已具备全部政务、医疗等行业能力。

## 2. 文档权威与当前范围

| 内容 | 权威来源 |
|---|---|
| V2 的产品目标、需求、用户场景与首批交付 | 本文 |
| 已有能力证据、缺口与核查限制 | [能力差距评估](full-lifecycle-governance-gap-assessment.md) |
| 新目标的架构边界 | [ADR-0014](../adr/0014-full-lifecycle-governance-boundary.md) |
| 能力归属、复用规则 | [Capability Map](../architecture/capability-map.md)、ADR-0010 |
| 当前运行架构与已有领域契约 | [System Architecture](../architecture/system-architecture.md) 及其专门领域文档 |
| 已完成 POC / Certified / Gold 的验收范围 | [PRD V1.1](prd-v1.md)、各 Pilot 验收报告；不追溯改写 |
| 具体任务的进度与验收证据 | 对应 Issue / PR 的精确 HEAD 记录 |

本轮只交付文档与任务编排，不增加 migration、runtime、UI 或部署。V2 不推翻现有版本不可变、Rights、认证及 CurrentDeliveryGate。已完成的受控 Pilot 不等于 production-ready；新蓝图更不构成生产上线批准。

## 3. 用户与两种治理路径

主要用户是数据负责人、Data Steward、数据工程师、质量/安全治理人员和数据消费者。系统管理员负责连接、身份映射和运行条件，不替代业务人员确认用途与规则。

### 3.1 外部数据纳管

外部数据库/文件/API → 发现元数据 → 绑定资产身份 → 维护责任与业务含义 → 探查/观察 → 跟踪问题、变化与影响。

数据内容可以继续留在外部系统。只采集元数据也须控制权限，Profiler 读取业务内容应另受授权和采样策略约束。界面标注外部对象、观察时间、覆盖范围、同步状态；不得把一次扫描或 live table 描述为不可变、已认证或已获交付授权的数据。

### 3.2 受控数据生产

来源准入 → 有界抽取 → Core 接纳 RAW DatasetVersion → 标准化/实体解析/清洗/加工 → 新 DatasetVersion → 质量、Rights、Contract 与认证 → 当前授权下交付。

治理可从任意阶段介入；这不是必须依次点击的唯一全局状态机。已有数据可以直接纳管；只有进入版本化生产、认证或交付时，才要求相应的冻结事实与接纳契约。

## 4. 全生命周期需求地图

G 表示治理规则与责任，E 表示技术执行；二者可以同时存在。“工作包”表示实施归属，不表示完成状态。

| 编号 / 阶段 | 必须回答的问题与目标能力 | 责任与结果 | 工作包 |
|---|---|---|---|
| V2-01 发现/盘点 | 有哪些数据、在哪、谁负责；目录、schema、术语、分类与技术血缘 | Steward 确认绑定与责任；保留来源/观察范围，不伪造可用版本（G/E） | B 的单来源最小视图；跨来源扩展后续 |
| V2-02 准入 | 谁可连接、读取、处理什么；来源、用途、敏感程度与权利依据 | 负责人确认规则；Core/执行边界验证可信身份、workspace 与范围（G/E） | B；全面策略管理后续 |
| V2-03 接入 | 如何取得同一批次、完整且可恢复的输入 | 外部引擎执行；Core 验证产物/manifest、身份和不可变版本（E） | B |
| V2-04 清洗/加工 | 应用什么规则，哪些记录被修正、隔离或关联 | 工程师使用批准模板；新版本、精确输入/规则、隔离原因（G/E） | B 固定模板，C 问题闭环 |
| V2-05 质量/认证 | 对具体用途是否达标，为什么通过或失败 | Core QualityAssessment、Rights/Contract、明确 Profile 与 Certification（G/E） | 复用既有能力，由 B/C 验证 |
| V2-06 数据组织 | 如何关联实体、版本、指标、标注和派生成果 | 复用 Entity、Workflow、Gold；不以人工标志代替冻结生产事实（G/E） | 既有主线；#294 独立收尾 |
| V2-07 发布/使用 | 谁当前可以获得哪个版本、用途与范围是什么 | 服务端执行 CurrentDeliveryGate；交付/使用事实与限制（G/E） | B 复用 DIRECT_DATA；外部发布归 #84/#86/#87 |
| V2-08 持续监督 | 数据过期、质量异常、schema/权利变化后怎么办 | 责任人受理问题，执行修复与复检，记录影响与未知范围（G/E） | C |
| V2-09 保留/终止 | 何时停止使用、归档、删除；谁批准、哪些副本已处理 | 策略、保全、依赖及执行证明分别管理，不能只设 DELETED（G/E） | D-design；runtime 后续独立立项 |

贯穿各阶段的横向能力：标准与责任、身份/权限、分类分级、血缘、Audit/Evidence/Cost、变更版本和运行可观测性。分类是业务类别，分级是保护要求；本蓝图不规定一套跨所有行业通用的等级枚举。

## 5. 能力组合与边界

Core 持有业务事实与受控 Command；OpenMetadata 提供元数据/目录/术语/分类/技术血缘能力；外部接入与加工引擎通过 Port/Adapter 执行；对象存储和外部数据库承担数据面。继续模块化单体与既有 Worker/Outbox，不按这张能力表拆微服务。

| 能力 | 本轮方向 | 引入门槛 |
|---|---|---|
| 数据发现/治理视图 | 复用 OpenMetadata 与已有 MetadataEngine/ResourceBinding | 固定 instance/object identity、字段写入 owner、同步方向/冲突/失联行为 |
| 数据接入 | SeaTunnel 是优先评估候选之一，不是已选定默认产品 | B 比较既有 Hop/成熟方案，核实具体 connector、版本/edition/license、恢复与输出契约 |
| 清洗/加工 | 优先复用 Native/Hop 与行业模板 | 不把当前园区代码描述为通用清洗工作台 |
| SQL 模型工程化 | dbt 为后续候选 | 有明确 SQL 数据面、模型复用/测试需求；产物必须冻结，不把可变表/view 当 DatasetVersion |
| 调度 | 复用现有 Workflow/Worker/Reconciliation | DolphinScheduler 等仅在实际复杂依赖/补数需求证明现有能力不足时评估；一个链路只有一个调度/重试 owner |
| 身份/授权 | 复用可信 Principal 边界与外部身份能力 | 目录可见性不等于源数据读取、处理或导出权；不能将 demo actor 配置用于真实数据 |
| 标注 | 复用现有 Label Studio 与 Core Review/Gold | #294/#295 继续原任务与跨仓验收，不借新蓝图扩 scope |

通用规则、工单、权限、元数据管理不重新从零实现。平台仅增加当前业务切片不可缺少的接纳、绑定和决策语义。跨系统的单一写入权与具体一致性要求见 ADR-0014。

## 6. 首个可交付切片：独立 PostgreSQL 来源

B / #298 的默认验证对象是独立 PostgreSQL 数据源，装载现有 enterprise/lease/energy 合成 fixtures。复用既有行业指标，不增加新的评分公式；选择有限静止批次，不承诺并发更新下的跨表一致性或 CDC。

操作者应在现有平台入口完成：查看来源/schema/责任/分类 → 提交授权范围内的抽取 → 查看 RAW 与清洗结果 → 查看新 CURATED 的质量、权利和认证 → 发起受控交付。

连接与模板由服务端受控配置，来源只读、表/列允许清单、secret 引用和输出范围明确。不是让操作者输入任意 URL/SQL/shell，也不是把 Core 控制库改成数仓。演示运行使用合成数据；真实数据库运行时不等于真实客户数据。

### B 的验收场景

| 场景 | 期望结果 |
|---|---|
| 正常固定批次 | 同一事实链得到 RAW、加工输出、评测、认证和交付记录 |
| 缺失来源权限/跨 workspace | 不读取越权内容，不接纳错配产物 |
| 输出不完整、checksum/schema 不匹配 | 不因引擎成功而创建可用版本 |
| 提交响应丢失、重复通知、Worker 重启 | 恢复原 operation；不重复启动同次工作或生成同次版本 |
| 预置异常数据 | 说明所用清洗/隔离规则及变化；不无依据填补未知事实 |
| 质量不达标 | 不认证，不用人工勾选代替评测 |
| 源授权撤销 | 后续交付按当前门禁阻断；保留历史解释 |
| OpenMetadata 暂不可用 | 显示陈旧/未知，不伪造发现或授权，不回滚历史发布 |
| 非开发者操作 | 能独立完成固定流程并解释来源、规则、质量和当前可用性 |

记录精确输入/规则/引擎版本、运行身份、产物 hash、错误/恢复证据；自动化与人工验收分别报告 PASS / FAIL / NOT_RUN。B 不以安装了多少组件作为完成标准。

## 7. 分批路线与验收

| 批次 | Issue | 有限交付 | 前置 |
|---|---|---|---|
| A / P0 | [#297](https://github.com/qq550723504/data-product-platform/issues/297) | 本蓝图、差距评估、ADR、愿景与 Capability Map 同步；docs-only | 本轮 |
| B / P0 | [#298](https://github.com/qq550723504/data-product-platform/issues/298) | 单个真实数据库软件 + 合成数据的治理/生产/交付链 | A 合并；开工时先做 B0 选型/契约 checkpoint |
| C / P1 | [#299](https://github.com/qq550723504/data-product-platform/issues/299) | 固定质量异常与时效/schema 变化的分派、修复、复检闭环 | A、B |
| D-design / P1 | [#300](https://github.com/qq550723504/data-product-platform/issues/300) | 保留/归档/销毁专门契约、决策表、负例与实施准入 | A；可与 B 并行设计 |
| D-runtime / 后续 | D-design 收口后再拆执行任务 | 受控合成对象的实际归档/处置及验证 | 专门契约、变更审查、运行环境与操作授权 |

Epic #296 只管理首批有限交付。D-design 完成不代表 runtime 完成；全部长期能力未落地前，不宣传为已支持全生命周期操作。

#294/#295 保持原 writer/分支与验收条件；本轮不改它们。B/C 触及共同 Core 模块时协调唯一 writer，不从未合并功能分支隐式继承契约。外部发布/使用事件继续由 #47 与 #84→#86→#87 跟踪。

## 8. 生命周期终止和安全准入

完整目标必须覆盖保留、归档、销毁，但不能直接修改现有冻结表或配置 bucket 自动删除。D-design 需区分版本事实、当前使用资格、物理内容可用性；分析派生/共享引用、缓存/索引、标注/隔离 payload、备份与外部副本。

历史 Evidence/Audit 也可能携带敏感内容；hash 不是自动匿名证明。允许保留什么、如何删除内容又诚实说明历史验证能力变化，必须由专门契约决定，不能在本蓝图中借“不可变”永久保留一切或借“删除”擦除全部历史。

真实客户/政府数据前必须完成与该场景匹配的身份授权、secret 管理、网络/存储隔离、审计及保留处置方案，并获得环境授权。无需把全套生产平台作为合成 Pilot 前置，但不能把这些安全边界省略为上线后事项。

## 9. 当前非目标

不一次引入所有引擎，不新建通用 ETL/审批/治理 DSL、全量 MDM/BI/数仓、数据交易所、完整 IAM 或微服务平台；不开展自动 AI 修复、全模态标注、无边界 streaming 数据产品、会计入表/估值或法律裁判。没有明确场景不新增大模型和付费服务。

原 POC、Certified 与 Gold 的通过范围不反向扩大。本轮不自动合并 PR、发布镜像、部署、接真实数据或执行删除。

## 10. 组件职责核对资料

以下是一手能力资料，不是本项目已集成或选型完成的证明；实现时应重新固定所用版本/edition/许可证和 connector 行为。

- [OpenMetadata Governance API](https://docs.open-metadata.org/v1.12.x/api-reference/governance)：domains、glossaries、classifications/tags 等治理结构。
- [OpenMetadata Profiler](https://docs.open-metadata.org/v1.12.x/how-to-guides/data-quality-observability/profiler)：分布、空值/重复等观察与质量检查能力。
- [SeaTunnel About](https://seatunnel.apache.org/docs/introduction/about/)：数据集成与 Source/Transform/Sink 作业模型；端到端保证依 connector/配置验证。
- [dbt SQL models](https://docs.getdbt.com/docs/build/sql-models)：在目标 SQL 数据平台执行模型及其依赖；不等于自动冻结平台 DatasetVersion。
