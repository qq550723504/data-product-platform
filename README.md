# Data Product Platform

数据产品生产与治理平台（Data Product Production & Governance Platform）。

本项目目标不是重新实现一个元数据平台、ETL 平台、可信数据空间或财务系统，而是提供一套通用的数据产品生产内核，把分散、异构、权利边界不清晰的原始数据，持续生产为可理解、可治理、可交付、可流通、可审计的数据产品。

## 本地试用入口

本代码包含合成数据演示和 CSV 接入到主体审核的界面化切片。合并状态和验收结果以对应提交的 GitHub 记录为准；CI 通过不等于商业上线批准。
主机需 Node.js 22、本地 Docker Linux 容器与 Compose v2。在完整仓库根目录运行：

```sh
node deploy/demo/demo.mjs doctor
node deploy/demo/demo.mjs up
```

- 接入自己的合成 CSV：`http://127.0.0.1:3180/ingest`；页面有可下载模板。[CSV 接入说明](docs/poc/csv-ingestion-v1.md)包含字段、限额、继续解析入口和验收边界。
- 预置审核/生产/发布演示：[本地演示说明](docs/poc/local-demo.md)。新上传的数据不会自动绑定这个演示产品；`advance` 和 `verify` 也不是新 CSV 的通用处理命令。
- 停止并保留数据：`node deploy/demo/demo.mjs down`。服务仅开放本机回环控制台，不要用当前 POC 身份配置向公网或真实客户数据开放。

## Start here（从这里开始）

建议按这个顺序阅读/执行：

1. 产品定位：`docs/product/product-vision.md`
2. PRD：`docs/product/prd-v1.md`
3. 架构：`docs/architecture/`
4. 架构决策：`docs/adr/`
5. POC 技术计划：`docs/poc/poc-technical-plan-v1.md`
6. 第一个 Reference Implementation：`examples/enterprise-activity/README.md`
7. GitHub POC Epic：`#1`
8. 第一条工程任务：`#2`，随后按 POC Sprint 顺序推进

Reference Implementation 的机器可执行/可验证规范集中在：

- `industry-packs/park/matching/company-match-policy-v1.yaml`
- `industry-packs/park/indicators/enterprise-activity-v1.yaml`
- `industry-packs/park/quality/enterprise-activity-quality-v1.yaml`
- `industry-packs/park/compliance/enterprise-activity-compliance-v1.yaml`
- `examples/enterprise-activity/workflow/workflow-v1.yaml`
- `examples/enterprise-activity/contract/data-contract-v1.yaml`
- `examples/enterprise-activity/test-vectors-v1.yaml`

## 核心业务链

```text
Use Case
   ↓
Rights Gate
   ↓
Data Resource
   ↓
Dataset / Dataset Version
   ↓
Standardization
   ↓
Entity Resolution
   ↓
Transformation
   ↓
Compliance Gate
   ↓
Quality Gate
   ↓
Data Contract
   ↓
Data Product
   ↓
Product Release
   │
   ├── Trusted Data Offering
   └── Assetization Case
```

全生命周期能力：`Rights · Version · Cost · Evidence · Audit`。

## 架构原则

1. Core Platform 是 Data Product 的 System of Record。
2. OpenMetadata 作为 Governance Projection，不拥有核心 Data Product 业务状态。
3. Apache Hop、Python、Spark、Flink 等通过 Processing Engine SPI 接入。
4. Splink 等仅作为 Entity Resolution Engine，不作为主数据系统。
5. DatasetVersion、ProductVersion、ProductRelease 是不可变历史对象。
6. 关键状态变化必须产生 Domain Event 与 AuditEvent。
7. Cost 与 Evidence 从生产开始采集，不做事后补录模型。
8. 行业能力通过 Industry Pack 扩展，不污染 Core Domain。
9. 第一阶段采用 Modular Monolith，验证领域模型后再决定是否拆微服务。

## 第一参考实现

第一个 Reference Implementation 使用园区场景：

- Use Case：企业融资风险辅助
- Data Product：企业经营活跃度
- 输入数据：企业基础信息、租赁数据、能耗数据
- 第一阶段不引入门禁、人脸、停车、视频等高敏数据

Reference Implementation 用于验证：

- Data Resource → Dataset → DatasetVersion
- Company Entity Resolution
- Processing Workflow
- Quality / Compliance Gate
- Data Contract
- Product Release
- Cost Ledger
- Evidence Graph

`activity_score` 等 V1 指标只用于验证数据产品生产生命周期和可解释性，不是经过验证的信用评分/授信模型。

## 计划目录

```text
data-product-platform/
├── apps/
│   ├── web/
│   └── platform/
├── engines/
├── industry-packs/
│   └── park/
├── examples/
│   └── enterprise-activity/
├── migrations/
├── docs/
│   ├── product/
│   ├── architecture/
│   ├── adr/
│   ├── poc/
│   └── reference/
└── deploy/
```

## POC 路线

以下保留原建设任务的状态；适配器已实现不等于所有外部引擎已经完成商业验收。

- Sprint 0：工程骨架、PostgreSQL、Redis、MinIO、Outbox、Audit（`#2`）— 已完成
- Sprint 1：DataResource / Dataset / DatasetVersion / Entity / Evidence（`#3`、`#4`、`#8`）— 已完成
- Sprint 2：Workflow / Execution / DataProduct / ProductRelease（`#5`、`#7`）— 已完成
- Sprint 3：Rights / Quality / Compliance / Data Contract + full release test（`#6`、`#12`、`#14`、`#16`）— 已完成
- POC UI：`#13` — 已完成；后续浏览器和 CSV 切片见下方验收说明
- Sprint 4：OpenMetadata Adapter（`#9`）— 已完成
- Sprint 5：Apache Hop Adapter（`#10`）— 已完成
- Sprint 6：Splink Adapter（`#11`）— 已完成

POC 成功标准不是“组件全部部署成功”，而是能从三组原始数据真实生产出一个不可变、可追溯的 `Product Release V1.0`，并能回答：

1. 这个产品怎么生产出来的？
2. 为什么这些记录属于同一个实体？
3. 发布时使用了哪些 Dataset / Rule / Contract / Rights 版本？
4. 产品生产发生了哪些 Cost Event？
5. 支撑这些结论的 Evidence 在哪里？

## 当前阶段与验收边界

核心 POC 已跑通。当前代码还包含可重复本地演示、CSV → RAW → 主体解析 → 人工审核入口，以及工作区引用、非有限数值和歧义匹配防错检查。整合来源见 `docs/poc/pr-cleanup.md`。

- Sprint 0–3 的全路径验收保留在 `apps/platform/internal/acceptance/enterprise_activity_poc_test.go`。
- POC UI 包含工作台、数据资源、数据集、数据生产、实体复核、数据产品、证据中心，以及 Release → DatasetVersion → Execution → Evidence 追溯。
- 浏览器契约测试、真实 Core/PostgreSQL/MinIO 验收、容器演示生命周期检查已纳入七组 `required` 汇总。结果必须对应当前提交，不能沿用旧提交的绿色结果。文档：`docs/poc/browser-action-contracts.md`、`docs/poc/live-core-browser-acceptance.md`、`docs/poc/csv-ingestion-v1.md`。
- OpenMetadata 治理投影、Apache Hop 处理和 Splink 概率候选适配器已实现；Core 仅通过 Engine SPI 依赖外部引擎。独立 smoke 与真实业务全链路验收需要区分。

仍需独立推进：

- 通用生产任务界面及 Execution 工作流/输入/输出的工作区边界（`#110`），以及持久化成功但入队失败的明确恢复流程。
- `entity_mapping` 来源唯一键缺少工作区的历史问题；当前引用一致性检查不等于完整租户隔离。
- 歧义候选当前不得任意自动匹配；人工指定实体的正式 Command 和界面尚未实现。
- NDI 集成探针（Epic `#47`，子任务 `#84` / `#86` / `#87`）：按 ADR-0009 在 ProductRelease 稳定后推进提供方中立模型、适配器对账与外部使用证据。
- 生产 IAM / 安全加固、真实客户数据接入、并发负载、灾备和对外部署。当前 UI 写操作依赖 POC 开关与服务端配置 actor，不是生产身份认证。

## 仓库可见性

仓库可见性决策（`#15`）已确定：仓库维持 **Public**。因此真实客户数据、合同、非公开规则或凭证不进入仓库。

当前约束：

- `.gitignore` 忽略 `.env` 与 `.env.*`，仅保留示例配置；
- `.env.example` 与 `apps/web/.env.example` 只包含本地开发占位值，外部引擎 token / 密码字段留空；
- `examples/` 与 `engines/splink` 评估集使用合成数据，不含真实企业或个人数据；
- `examples/enterprise-activity/` 的合规规则与 Data Contract 禁止转售与营销用途，并限制原始字段输出。

Public 仓库中不得提交真实 Use Case 数据、租约/能耗原始记录、客户合同或任何生产凭证。
