# 产品愿景

## 产品定位

Data Product Platform 是一套数据产品与高质量数据集生产治理平台。

它围绕不可变 DatasetVersion，把数据接入、标准化、实体对齐、加工、质量评测、权利证明、合规、认证、交付和数据产品发布串成可验证的生产链。

平台核心数据库负责业务真相；OpenMetadata、Hop、Splink、Soda/GX、Label Studio 等外部系统通过 Adapter 提供能力，不拥有核心业务状态。能力规划不等同于这些适配器都已实现。

## 当前阶段

核心 POC 与 #129 Certified Dataset 第一阶段 MVP / enterprise-activity 纵向 Pilot 均已完成，E2E1–E2E20 全部 PASS。

第一阶段已经验证：

1. 原始数据可以被加工成可验收的 Certified Dataset；
2. 质量报告可以解释为什么通过或失败；
3. 数据权利来源和授权可以解释为什么允许或拒绝使用与交付；
4. Certified Dataset 可以完成受控的 trusted DIRECT_DATA 交付与 UI/trace 验收。

第一阶段完成不等于生产上线批准。第二阶段 #203 AI / Gold Dataset Epic 已完成主要工程闭环：#209、#204–#207 均已完成，#208 的 real Label Studio、Human Review、Gold build/quality/certification、browser/live-core、DIRECT_DATA 与 Cost/Evidence/Audit 自动化验收均已通过。当前 External Explanation Test 仍待未参与实现的人员仅凭产品 UI 执行，因此第二阶段尚不能描述为最终 Pilot PASS 或 production-ready；#203 仍保持开放。

本阶段只选择一个受控、合成数据、单标签、独立人工审核的 Gold 生产闭环。Delivery Hardening / external provider、完整 IAM/SLA/灾备和其他行业能力仍由真实需求独立决定，不跟随 Gold 自动启动。

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

第二阶段进一步回答：谁标注、谁审核、选中了哪个不可变结果，哪些任务被拒绝，为什么这份具体输出满足明确 Gold Profile？

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
├── Annotation / Review → Gold Dataset（#204–#208 自动化纵向验收已完成；External Explanation Test 待执行）
└── External Delivery
~~~

Gold 不是绝对正确或通用 benchmark 声明；模型训练、train/test split、统计代表性与 benchmark 评估不属于当前 Pilot。

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
        ├── Annotation / Review / Snapshot（真实 Label Studio + trusted review + frozen snapshot 已验证）
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

## Gold Dataset 第二阶段基线

当前实现状态：#204–#207 已完成 Annotation Core、Label Studio reference adapter、trusted Human Review、Gold Quality、Gold DatasetVersion build 与 Gold Certification；#208 已完成 real browser/live-core 的自动化纵向验收，并验证 DIRECT_DATA fresh re-gate 以及按阶段回溯的 Cost / Evidence / Audit。External Explanation Test 仍为最终人工验收项。

Gold 候选仍是新的 DatasetVersion，不是独立 GoldDataset 主实体。它绑定完整冻结的 AnnotationSnapshot、规范版本、实际 producer 与完整源数据/标注贡献权利依赖，再经新 QualityAssessment 和明确 Gold Profile 认证；不能把 input certification 复制给 output。

通用标注交互和分发由 Label Studio reference adapter 复用，Core 持有结果接纳、审核权威性、冻结事实与认证语义。provider 当前状态不得改写历史；历史 CERTIFIED 不替代当前交付授权。

唯一产品范围、16 个关键问题定案及实现归属见 [gold-dataset.md](gold-dataset.md)；自动化验收事实见 [Gold Dataset Pilot 验收报告](../poc/gold-dataset-pilot-acceptance.md)，最终人工证伪协议见 [Gold External Explanation Test](../poc/gold-external-explanation-test.md)；跨模块决策见 [ADR-0012](../adr/0012-gold-dataset-annotation-boundary.md)。本文只提供产品摘要，不另定义一套状态机。

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

平台记录声明、依据、授权、限制和 Evidence，不自动裁定现实世界法律所有权。完成标注、拥有引擎账号或引擎的开源许可均不自动产生数据使用权。

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

activity_score 等 V1 指标仅用于验证数据生产生命周期和可解释性，不是经过验证的授信模型。Gold Reference 延续其合成内容验证记录标注/审核，不改变原契约用途，也不生成信用或违约判断。
