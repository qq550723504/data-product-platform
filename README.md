# Data Product Platform

**全生命周期数据治理与可信数据生产平台**  
Full-Lifecycle Data Governance & Trusted Data Production Platform

面向企业、政府与行业数据场景，长期目标是把数据发现、准入、接入、清洗加工、质量与权利、认证交付、持续监督及保留处置连接起来，让数据含义清楚、责任明确、处理可追溯、使用有依据。

> 当前成果是逐步交付的受控切片，不是完整全生命周期产品或生产上线承诺。已合并代码、技术验收、人工解释验收和生产准入分别记录；不能相互替代。

[产品蓝图 V2](docs/product/full-lifecycle-governance-v2.md) · [能力差距评估](docs/product/full-lifecycle-governance-gap-assessment.md) · [首批交付 Epic #296](https://github.com/qq550723504/data-product-platform/issues/296)

## 产品定位与生命周期目标

~~~text
发现 / 盘点 → 登记 / 准入 → 接入 → 清洗 / 加工
      → 质量 / 权利 / 认证 → 发布 / 使用 → 持续监督 → 保留 / 归档 / 销毁

贯穿各阶段：标准 · 责任 · 分类分级 · 权限 · 血缘 · 成本 · 证据 · 审计
~~~

以上是目标能力地图，不是一条已经全部实现的全局状态机。治理既包括制定规则、落实责任，也包括授权读取、质量检测、受控处理和处置等执行动作。

平台支持方向分为两条：

| 路径 | 目标与边界 |
|---|---|
| **外部数据纳管** | 数据可以留在原系统，通过元数据绑定、业务描述、责任人和治理观察进行管理；不要求把所有业务数据搬入平台。外部 live table、目录条目或一次扫描不自动成为不可变数据集或认证结果。 |
| **受控数据生产** | 接纳有界、可验证的输入为 RAW DatasetVersion，执行清洗、实体解析、加工或标注，形成新的版本，独立评估质量与权利，再认证和受控交付。 |

Certified Dataset、Data Product 和 Gold Dataset 是可交付成果，不是所有数据纳管的前置条件。详细产品要求归 [V2 蓝图](docs/product/full-lifecycle-governance-v2.md)，架构边界归 [ADR-0014](docs/adr/0014-full-lifecycle-governance-boundary.md)；README 只提供入口和摘要，不另定义状态机。

## 当前阶段

以下为 2026-10-13 核对的状态摘要，源码基线为 [`main@c1a3463`](https://github.com/qq550723504/data-product-platform/commit/c1a3463ee110e3cb7dadb0004b0942c001f19ffe)。历史报告只证明各自标注的代码、环境和用例，不表示本次 README 更新重新执行了运行测试；后续进度以对应 Issue / PR 的精确版本记录为准。

| 工作线 | 已交付或已记录的结果 | 尚未覆盖的边界与证据入口 |
|---|---|---|
| 核心 POC / Certified Dataset | 第一阶段受控 Pilot 完成，E2E1–E2E20 验证质量、Rights、认证和 trusted DIRECT_DATA。 | 不等于完整生产 IAM、灾备或性能 SLA；见 [Certified Pilot 验收](docs/product/certified-dataset-pilot-acceptance.md)。 |
| AI / Gold Dataset | #203 历史 Pilot 已验证真实 Label Studio → Core Review → FINALIZED Snapshot → Gold build / quality / certification / DIRECT_DATA 与 live Gold UI。 | External Explanation Test 为 deferred / non-blocking follow-up；受控 fork Submission 的 #294/#295 不继承历史 Pilot 的通过结论。见 [Gold 验收](docs/poc/gold-dataset-pilot-acceptance.md)。 |
| 全生命周期产品基线 A | #301 已合并，#297 已关闭；蓝图、差距评估与 ADR-0014 已进入 main。 | 蓝图是长期范围，不代表所有功能已实现。 |
| PostgreSQL 接入 B | #302 定义 B0；#303/#304/#305/#308 已推进真实 PostgreSQL/Hop、三 RAW 接纳、界面与认证交付，以及 OpenMetadata 健康/断连/恢复的有限技术验收。 | 固定静止合成场景；非开发者操作/解释仍 deferred / NOT_RUN，#298 保持 OPEN。见 [接入契约](docs/architecture/postgres-ingestion-v1.md)、[最终实跑记录](examples/enterprise-activity/postgres-ingestion/CLOSURE-RUN.md)和 [#298](https://github.com/qq550723504/data-product-platform/issues/298)。 |
| 质量修复与持续治理 C | C1 负数用电异常的登记、分派、受控隔离、新版本与独立复检已由 #310 合并。 | 不代表 #299 全部完成；C2 来源 schema 变化工作仍在未合并的 Draft #313，不能计入 main。见 [C1 验收](docs/acceptance/issue299-c1.md)和 [#299](https://github.com/qq550723504/data-product-platform/issues/299)。 |
| 保留与生命周期处置 D | #300 设计由 #306 合并；D1 的受控合成副本处置与对账代码由 #314 合并。 | #307 的完整验收未因此关闭；不宣称全仓 reader、备份、外部副本或敏感正文处置已覆盖。额外原生删除须单独确认环境与精确对象/version 清单。见 [生命周期契约](docs/architecture/data-lifecycle.md)和 [#314 范围](https://github.com/qq550723504/data-product-platform/pull/314)。 |

不要将上述工作重新标为“只有规划”，也不要因为一个实现 PR 合并就勾选整个 Epic 完成。未合并分支、暂缓的人工验收和生产准入继续单列；本次首页更新不改变各工作线的负责人、验收要求或运行环境。

## 本地试用入口

仓库保留 POC 合成数据演示和 CSV 接入切片。先阅读 [本地演示说明](docs/poc/local-demo.md)，在隔离环境中检查前置条件：

~~~sh
node deploy/demo/demo.mjs doctor
node deploy/demo/demo.mjs up
~~~

- 接入合成 CSV：<http://127.0.0.1:3180/ingest>；格式与限制见 [CSV 接入说明](docs/poc/csv-ingestion-v1.md)。
- 停止并保留数据：`node deploy/demo/demo.mjs down`。
- PostgreSQL 接入的运行前提与证据见 [接入契约](docs/architecture/postgres-ingestion-v1.md)和[实跑记录](examples/enterprise-activity/postgres-ingestion/CLOSURE-RUN.md)，不是上述 CSV 演示命令自动部署的全部能力。

当前 POC 身份配置只用于本地合成演示，不应用于公网或真实客户数据。这也不是 Gold / Label Studio 或 D1 已部署的入口。已有保留环境、卷和备份应按各自运行记录维护，不通过重建或重新播种伪装成历史恢复。

## Start here

按问题选择文档；当前进度看 Issue/PR，历史报告保留原验收范围。

| 想了解什么 | 主要入口 |
|---|---|
| 产品定位与全生命周期范围 | [产品愿景](docs/product/product-vision.md)、[V2 蓝图](docs/product/full-lifecycle-governance-v2.md)、[差距评估（固定基线）](docs/product/full-lifecycle-governance-gap-assessment.md) |
| 能力归属与复用原则 | [Capability Map](docs/architecture/capability-map.md)、[ADR-0014](docs/adr/0014-full-lifecycle-governance-boundary.md) |
| 当前核心架构与领域 | [系统架构](docs/architecture/system-architecture.md)、[领域模型](docs/architecture/domain-model.md)、[AGENTS 开发约束](AGENTS.md) |
| 数据库接入与治理操作 | [PostgreSQL 接入 V1](docs/architecture/postgres-ingestion-v1.md)、[最终运行证据](examples/enterprise-activity/postgres-ingestion/CLOSURE-RUN.md) |
| 质量异常修复 | [C1 验收记录](docs/acceptance/issue299-c1.md)、[#299](https://github.com/qq550723504/data-product-platform/issues/299) |
| 保留、归档与销毁边界 | [生命周期契约](docs/architecture/data-lifecycle.md)、[ADR-0015](docs/adr/0015-data-lifecycle-disposition-boundary.md)、[设计验收](docs/product/data-lifecycle-design-acceptance.md) |
| Certified Dataset / Rights | [认证架构](docs/architecture/certified-dataset.md)、[权利溯源](docs/architecture/data-rights-provenance.md)、[试点范围](docs/product/certified-dataset-pilot.md)、[试点验收](docs/product/certified-dataset-pilot-acceptance.md) |
| Gold 生产与标注 | [Gold 产品基线](docs/product/gold-dataset.md)、[Annotation Domain](docs/architecture/annotation-domain.md)、[引擎集成](docs/architecture/annotation-engine-integration.md)、[Gold 生产与认证](docs/architecture/gold-dataset-production.md)、[ADR-0012](docs/adr/0012-gold-dataset-annotation-boundary.md) |
| 历史产品与参考实现 | [PRD V1.1](docs/product/prd-v1.md)、[POC 技术计划](docs/poc/poc-technical-plan-v1.md)、[企业经营活跃度参考实现](examples/enterprise-activity/README.md) |

## 核心业务链

已有版本化生产链是全生命周期目标的基础，不是全部纳管对象必须经过的流程。

~~~text
Use Case → DataResource → Rights Provenance → RAW DatasetVersion
    → Standardization / Entity Resolution / Transformation
    → 新 DatasetVersion → Quality / Rights / Compliance / Contract
    → DatasetCertification → Certified DatasetVersion
        ├── Data Product / Product Release
        ├── Annotation / Review → Gold DatasetVersion → 独立评测与认证
        └── CurrentDeliveryGate → 受控交付
~~~

Certified Dataset 可以独立交付，不要求先包装为 DataProduct。每次交付都必须通过 CurrentDeliveryGate：数据版本当前可用、认证当前适用、权利当前允许。历史 CERTIFIED 不代表永久交付权限。

## 技术与能力分工

| 能力 | 组合方向与当前边界 |
|---|---|
| 元数据与目录 | OpenMetadata 作为治理能力提供者与投影；不拥有 Core DatasetVersion、Rights、认证或发布事实。 |
| 数据接入 | 现有 CSV/File 与受控 PostgreSQL/Hop 切片；SeaTunnel 保留为可替换候选，不是已部署的默认引擎。 |
| 清洗与加工 | 复用 Native/Hop、ProcessingEngine 和行业模板；dbt 仅在明确 SQL 数据面与模型工程化需求时评估。 |
| 任务运行 | 复用现有 Workflow、Worker、Outbox 和 Reconciliation；不与 DolphinScheduler 等再建竞争调度。 |
| 实体解析与标注 | Rules/Splink、Label Studio 通过 Adapter 提供能力；Core 持有映射、已接纳结果、审核和冻结快照。 |
| 控制与数据存储 | Go Core、PostgreSQL 控制库、Redis、MinIO/S3 兼容对象存储；业务数据面与控制事实分开。 |
| 生命周期处置 | Core 管理处置资格与可审计事实，存储 Adapter 执行；实际覆盖范围和证明以 D1 契约/验收记录为准。 |

组件能力、Adapter 已实现、运行已验证和生产准入不是同一回事。具体边界见 [Capability Map](docs/architecture/capability-map.md)及各专门契约。

## 核心建模原则

### DatasetVersion 是数据事实

不新增平行的 HighQualityDataset 或 GoldDataset 主实体。

~~~text
DatasetVersion + QualityAssessment + Effective Rights
+ Compliance / Contract + Traceability / Evidence
+ CertificationProfile + DatasetCertification
= Certified DatasetVersion
~~~

清洗或加工改变数据内容时，生成新 DatasetVersion 并重新评测、重新认证；只修正评测或认证判断时，追加对应的新事实，不无意义复制数据版本。Gold 候选仍是 DatasetVersion，绑定冻结标注/审核和实际生产、权利依赖，由明确 Gold Profile 重新认证。

生命周期处置不等于改写历史：当前使用资格、物理副本可用性和历史版本事实分别处理。不能因为要求不可变就永久保存所有敏感内容，也不能在内容已不可读时仍声称可完整重算历史证明；遵循[生命周期契约](docs/architecture/data-lifecycle.md)。

### 平台归属不等于法律所有权

`data_resource.owner_id` / `dataset.owner_id` 表示平台内责任或归属，不自动等同现实世界法律所有权。

~~~text
RightsDeclaration → AuthorizationProvenanceBinding → Authorization
    → RightsSnapshot → Effective Rights → DatasetCertification
~~~

平台记录声明、依据、授权、限制与证据，不替代法律机构判断。目录可见性、标签、技术账号或一次历史认证都不能代替当前数据读取、处理和交付权限。

## 架构原则

1. Core 数据库是核心业务事实的 System of Record，外部系统不接管版本、权利、认证、交付、成本和证据事实。
2. OpenMetadata 作为 Governance Projection；目录观察与已接纳的业务事实明确区分。
3. 接入、处理、实体解析、质量、标注和存储能力通过 Port / SPI + Adapter 集成，优先复用成熟能力。
4. 冻结的历史事实不得原地覆盖；修正、处置与当前资格变更遵循显式契约。
5. 关键状态变化通过 Command，异步副作用使用 Transactional Outbox；同一链路只有一个调度/重试责任方。
6. Cost、Evidence 与 Audit 随实际活动记录，不以最终成功为前提补造。
7. 行业逻辑通过 Industry Pack 扩展，不把园区词汇或某个引擎类型泄漏进 Core Domain。
8. 继续采用 Modular Monolith；不因目标能力增加就提前拆微服务或新增通用平台。

## 当前明确非目标

长期覆盖全生命周期，不表示当前提供完整 ETL/湖仓、通用工单/审批、全部 MDM/BI、所有数据库连接器、全模态标注、全域副本销毁或生产级 IAM/灾备/SLA。范围扩大须有具体场景、契约和验收，不能由 README 宣告完成。额外删除或清理需单独确认环境、精确对象/version 和操作范围，不因代码合并自动获得授权。

### 数据资产会计入表

Rights / Cost / Evidence 是可审计业务事实，不自动生成数据资产会计确认、估值、摊销、减值、资产负债表结论或法律权利裁定，也不将 CostEvent 自动映射为会计科目和凭证。未来对接专业系统需另行明确口径、责任主体和验收标准。

### Streaming / Continuous Data Product

当前 Core 的生产语义仍是有边界、不可变、版本化的 DatasetVersion。上游未来采用 CDC、Kafka 或 Flink 时，进入质量、认证和交付链仍须形成明确 snapshot / window / cut。

不承诺无边界 stream 直接作为持续变化的 DatasetVersion、checkpoint/window/watermark 的端到端 exactly-once 数据产品语义、永不结束的 Workflow Execution、streaming SLA 或 stateful recovery；真实连续数据产品需求必须另行设计。

## 第一 Reference Implementation

第一参考实现使用园区企业经营活跃度场景，输入为企业基础信息、租赁和能耗，验证来源、实体解析、冻结加工依赖、质量/权利/合规/Contract、认证交付、ProductRelease 及成本证据追溯。

同一场景已从 CSV 扩展出受控 PostgreSQL 接入与负数用电异常修复切片。样本和规则是参考实现，不限制平台只能处理企业或园区数据；`activity_score` 等 V1 指标不是经过验证的授信模型。

原 Certified 阶段的范围见 [第一阶段验收](docs/product/certified-dataset-pilot-acceptance.md)；Gold 自动化与暂缓人工验收分别见 [Gold Pilot](docs/poc/gold-dataset-pilot-acceptance.md)、[External Explanation Test](docs/poc/gold-external-explanation-test.md)。不得将后续工作反向写成旧 Pilot 已验证。

## POC 历史基线

原 POC Sprint 0–6 已完成。历史目标和验收保留在 [POC 技术计划](docs/poc/poc-technical-plan-v1.md)与[参考集成测试](apps/platform/internal/acceptance/enterprise_activity_poc_test.go)；详细阶段进度留在验收报告及 Issue/PR，不在首页重复维护完整历史清单。

## 仓库可见性

仓库维持 Public。不得提交真实客户数据、非公开合同正文、非公开权利证明原件、生产凭证或高敏个人数据。`examples/` 和测试使用合成数据；真实试点须使用独立受控环境，并单独满足其安全、权限、保留处置与运行准入要求。
