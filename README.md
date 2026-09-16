# Data Product Platform

数据产品生产与治理平台（Data Product Production & Governance Platform）。

本项目目标不是重新实现一个元数据平台、ETL 平台、可信数据空间或财务系统，而是提供一套通用的数据产品生产内核，把分散、异构、权利边界不清晰的原始数据，持续生产为可理解、可治理、可交付、可流通、可审计的数据产品。

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

Reference Implementation 将用于验证：

- Data Resource → Dataset → DatasetVersion
- Company Entity Resolution
- Processing Workflow
- Quality / Compliance Gate
- Data Contract
- Product Release
- Cost Ledger
- Evidence Graph

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

- Sprint 0：工程骨架、PostgreSQL、Redis、MinIO、Outbox、Audit
- Sprint 1：DataResource / Dataset / DatasetVersion / Entity / Evidence
- Sprint 2：Workflow / Execution / DataProduct / ProductRelease
- Sprint 3：Rights / Quality / Compliance / Data Contract
- Sprint 4：OpenMetadata Adapter
- Sprint 5：Apache Hop Adapter
- Sprint 6：Splink Adapter

POC 成功标准不是“组件全部部署成功”，而是能从三组原始数据真实生产出一个不可变、可追溯的 `Product Release V1.0`，并能回答：

1. 这个产品怎么生产出来的？
2. 为什么这些记录属于同一个实体？
3. 发布时使用了哪些 Dataset / Rule / Contract / Rights 版本？
4. 产品生产发生了哪些 Cost Event？
5. 支撑这些结论的 Evidence 在哪里？

## 当前阶段

当前处于：**Reference Implementation 规格冻结 + Core POC 工程初始化**。

详细设计见 `docs/`。