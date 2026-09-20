# 产品愿景

## 产品定位

Data Product Platform 是一套数据产品与高质量数据集生产治理平台。

它围绕不可变 DatasetVersion，把数据接入、标准化、实体对齐、加工、质量评测、权利证明、合规、认证、交付和数据产品发布串成可验证的生产链。

平台核心数据库负责业务真相；OpenMetadata、Hop、Splink、Soda/GX、Label Studio 等外部系统通过 Adapter 提供能力，不拥有核心业务状态。

## 当前阶段

核心 POC 已完成。

当前进入 #129 Certified Dataset 受控试点，主要验证：

1. 原始数据能否被加工成可验收的高质量数据集；
2. 质量报告是否能解释为什么通过或失败；
3. 数据权利来源和授权是否能证明为什么可以使用和交付；
4. Certified Dataset 是否能支撑数据产品、AI 数据或可信数据空间交付；
5. 与原人工流程相比能减少多少工时与交付周期。

## 核心问题

平台必须能回答：

1. 为什么生产这份数据？
2. 数据来自哪些 DataResource / DatasetVersion？
3. 为什么这些记录属于同一个实体？
4. 实际使用了哪些 Workflow / Policy / Rule / Contract 版本？
5. 数据质量如何，哪些规则失败，影响多少记录？
6. 权利由谁声明、基于什么依据、谁授权谁、允许什么、禁止什么？
7. 衍生 Dataset 的 Effective Rights 是什么？
8. 为什么某个 DatasetVersion 可以被认证？
9. Data Product 为什么可以发布？
10. 成本与 Evidence 在哪里？

## 核心链路

~~~text
Use Case
→ Data Resource
→ Rights Provenance
→ Dataset / DatasetVersion
→ Standardization
→ Entity Resolution
→ Transformation
→ Quality Assessment
→ Rights / Compliance / Contract
→ Dataset Certification
→ Certified Dataset
~~~

Certified Dataset 后可以进入：

~~~text
Certified Dataset
├── Data Product / Product Release
├── Trusted Data Offering
├── AI Dataset → Gold / Benchmark（第二阶段）
└── External Delivery
~~~

## 产品能力结构

~~~text
Industry Solution Layer
        ↑
Industry Packs
        ↑
Certified Dataset / Data Product Core
        │
        ├── Rights Provenance
        ├── Dataset / Version / Lineage
        ├── Entity Resolution
        ├── Workflow / Execution
        ├── QualityAssessment
        ├── Compliance / Contract
        ├── DatasetCertification
        ├── ProductRelease
        └── Cost / Evidence / Audit
        ↓
Engine Adapter Layer
~~~

## Certified Dataset

平台不新增另一套 HighQualityDataset 主实体。

~~~text
Immutable DatasetVersion
+ QualityAssessment
+ Effective Rights
+ Compliance
+ Contract
+ Traceability / Evidence
+ CertificationProfile
+ DatasetCertification
= Certified DatasetVersion
~~~

认证是围绕明确 DatasetVersion 和明确 CertificationProfile 的不可变事实。

DatasetVersion 内容变化后必须创建新版本并重新评测、重新认证。

## 数据权利原则

平台内 owner_id 表示资产责任/归属，不自动等于现实世界法律上的数据所有权。

目标权利链：

~~~text
RightsDeclaration
→ AuthorizationProvenanceBinding
→ Authorization
→ RightsSnapshot
→ Effective Rights
→ DatasetCertification
~~~

平台记录声明、依据、授权、限制和 Evidence，不自动裁定现实世界法律所有权。

## 通用 Core 与行业包

Core 不包含园区专属业务词汇。

Industry Pack 可以提供：

- Entity Types
- Glossary
- Standardization Rules
- Matching Policies
- Indicators
- Quality Rules
- Compliance Rules
- Certification Profiles
- Product Templates

第一行业包为 Park。未来增加制造、政务、金融、医疗等行业包时，不应修改核心 Domain 语义。

## 第一 Reference Implementation

- Use Case：企业融资风险辅助
- Product：企业经营活跃度
- Source：企业基础信息、租赁、能耗
- POC 目标：Raw Data → Product Release → Cost/Evidence
- Pilot 目标：Raw Data → Certified Dataset → 可解释认证

activity_score 等 V1 指标仅用于验证数据生产生命周期和可解释性，不是经过验证的授信模型。
