# 数据治理参考能力对标与 DPP 演进路线（2026-10）

> **性质：产品评审辅助材料，非新的权威领域契约或已实现清单。**
> 基线：main@bfafe8775b48f427aa9d0051b24da7cd78fbc1ba（2026-10-20）。
> 权威目标仍是 [全生命周期 V2 蓝图](full-lifecycle-governance-v2.md)；能力归属仍是 [Capability Map](../architecture/capability-map.md)；具体验收按对应 Issue/PR 的独立证据判读。
>
> 本文是将外部参考产品能力进行**非逐字的抽象归纳**后，与仓库现状比较。原始材料、详细逐行对标矩阵和附件内容**不上传公开仓库**。

## 1. 产品判断

Data Product Platform 的长期定位继续保持 **全生命周期数据治理与可信数据生产平台**。

外部成熟数据治理产品的参考价值主要体现在：资源发现与目录、治理工作台、数据标准/质量运营、数据交换和共享、数据服务消费、行业流程，以及 AI 数据治理交互。

现有项目的核心差异化是可核对的 DataResource / immutable DatasetVersion、质量评测、Rights/Certification、当前交付门禁、Evidence/Audit/Cost、受控恢复及生命周期处置。

**方向不是重做数据中台，而是在现有 Core 之上补齐治理管理与数据消费体验**。不得将所有目录项直接翻译成独立模块或 Issue，也不得将文档目标/受控测试自动标记为生产已完成。

## 2. 领域能力与归属

| 领域能力 | 现有基础/可复用组件 | 建设判断 |
| --- | --- | --- |
| 治理工作台与职责协作 | 已有 Web 工作台、待办、资源/数据集页面与 Workflow | 需要面向 Data Steward 的统一入口、责任和处理进度；不另造通用工单平台 |
| 资源发现、目录、分类、术语、血缘 | OpenMetadata、MetadataEngine/ResourceBinding | 主要复用成熟开源能力；需要稳定 source identity、同步陈旧状态和资产视图 |
| 数据源接入和交换 | CSV/File、固定 PostgreSQL/Hop 受控切片、既有 Worker/Outbox | 第二来源、通用接入和 CDC 需独立场景及证据；不造第二套调度/offset runtime |
| 数据模型与标准 | Industry Pack、治理元数据、现有数据版本 | 优先选择一个业务域落地标准、代码集、字段对标；通用 PDM/数仓编辑器非首批目标 |
| 质量治理与异常修复 | QualityAssessment、Native/GX、#299 有限闭环 | 扩展治理工作台、报告、责任及标准应用；数据变更产生新 DatasetVersion |
| 数据安全与生命周期 | Rights/CurrentDeliveryGate、Evidence/Audit/Cost、#307 | 完整 IAM、脱敏与生产级全域处置尚未证明；保留当前使用资格、历史事实和物理内容三种边界 |
| 服务与数据共享 | DataProduct/ProductRelease、#84/#86/#87 外部发布及使用事实 | 需要服务目录、申请、订阅、审批和消费看板；基础外部发布事实不重复建模 |
| AI 治理与问数 | 元数据、Quality、Gold/实体解析提供基础上下文 | AI 资产检索和编目是新交互层；Gold 标注能力**不等于** AI 治理助手；先只读、后有界问数 |
| 政务共享/公共开放 | 通用目录、发布、Rights 可部分复用 | 部门共享审批、数据报送、公共开放门户按真实行业需求做可选解决方案 |

### 2.1 不能相互替代的两类路径

- **外部数据纳管**：数据可以留在来源系统；目录/技术扫描不代表已形成可认证的不可变版本，也不自动产生读取/交付权。
- **受控数据生产**：获准接纳明确范围的 RAW → 规范化/实体解析/加工/标注 → 新的冻结版本 → 质量与权利复核 → 认证 → 在交付时重新检查当前使用资格。

两条路径可共用资产身份与目录体验，但不能使用同一条假设所有数据必经的全局状态机。

## 3. 实施顺序（不重开现有主线）

### P0：先收口已有真实合成场景

1. [#325](https://github.com/qq550723504/data-product-platform/issues/325)：完成同一保留环境中 PostgreSQL/Hop → RAW → 加工/复检/认证/下载的可重复试用和实际结果核查；不用重建/seed 假冒原环境恢复。
2. [#298](https://github.com/qq550723504/data-product-platform/issues/298) / [#299](https://github.com/qq550723504/data-product-platform/issues/299)：有限技术验收记录保留，非开发者操作与解释验收继续独立标记；不得以 CI 代替人工验收。
3. [#307](https://github.com/qq550723504/data-product-platform/issues/307)：在原有限授权和安全契约下推进 D1 生命周期完整验收，不把有限合成对象结果扩大为生产级删除。

### P1：治理人员能实际操作

4. **治理工作台**：统一数据资源、责任人/Steward、治理问题、质量结论、处理进度及证据入口（可考虑独立有界 UI Issue）。
5. **资产目录与元数据视图**：优先 OpenMetadata/既有绑定，不复制元模型、术语或技术血缘产品；清楚标记最近观察时间和失联状态。
6. **一个业务域的标准与质量运营**：标准版本/字段对标 → 质量检测 → 受控修复或隔离 → 新版本 → 独立复检；复用 #299 和现有 Quality/Workflow。

### P2：服务消费与 AI

7. 基于已合并的 [#84](https://github.com/qq550723504/data-product-platform/issues/84) / [#86](https://github.com/qq550723504/data-product-platform/issues/86) / [#87](https://github.com/qq550723504/data-product-platform/issues/87) 实现服务目录/申请/授权/使用统计的产品层；不能将 Core publication intent 视为完整 API Marketplace。
8. 真实业务要求第二类来源/增量时再评估 SeaTunnel、Debezium、Flink CDC 等；流式输入只有在明确快照/window/cut 后才允许成为新的冻结 DatasetVersion。
9. AI MVP 先做**只读数据资产查询和辅助编目**，给出元数据来源、时间和不确定性；应用描述/规则需要人工接纳。之后才评估指标语义层、限定 SQL、权限与预算约束下的问数。

### P3：行业可选产品

10. 政务部门共享审批、数据上报、公共开放/开发者门户以及专项评价，只在行业客户场景明确后单独立项；避免把行业审批语义下沉到通用 Core。

## 4. 新需求的最小验收与复用门槛

- 先核查现有仓库能力、现有 Issue 和成熟 OSS 的可复用性，遵循 AGENTS.md §1A 及 ADR；跨模块能力只有一个写入权和调度 owner。
- 明确业务场景、角色、来源、输入/输出、版本、质量/权利、恢复和负例；新接口应有 Adapter contract test。
- 对目录资源保持 live source 与已冻结 DatasetVersion 的区分。元数据失联不得伪装成功、不得放宽 Core CurrentDeliveryGate。
- 操作记录与交付事实关联可验证的 Audit/Evidence/Cost；内容变化创建新版本，不能篡改历史结论。
- 涉及可信身份、真实客户数据、生产发布、备份、精确对象处置或授权撤销的动作，按专门契约和审批处理。
- 技术验证、人工解释和生产准入分开报告。没有运行证据只写规划/待验证。

## 5. 与现有 Issue / 文档的关系

- 长期 Epic：[#296](https://github.com/qq550723504/data-product-platform/issues/296)。
- 接入、异常治理、试用交付与生命周期：#298、#299、#325、#307 原 owner 和范围保持不变。
- Gold/标注：[ #203 ](https://github.com/qq550723504/data-product-platform/issues/203) 的受控产线不同于拟议 AI 治理助手；不复用其验收结论覆盖智能问数。
- [V2 蓝图](full-lifecycle-governance-v2.md)定义长期目标；[能力差距评估](full-lifecycle-governance-gap-assessment.md)保留自己的固定代码基线；[Capability Map](../architecture/capability-map.md)定义组件归属。本文仅补充外部产品能力的抽象对照与优先级，不创建并行不一致的领域 contract。

## 6. 本次范围

本 PR 仅添加 docs-only 产品评审辅助材料：**不改代码、schema、Runtime、README、原 PR、Issue 状态或正式验收结果**，也不包含参考附件的逐条文本。它不授权部署、真实客户数据接入、外部付费调用或任何存储删除。
