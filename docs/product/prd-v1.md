# Data Product Platform PRD V1.1

> V1.1 基线：核心 POC 已完成，当前进入 Certified Dataset 受控试点。历史 POC 范围保留在 docs/poc/poc-technical-plan-v1.md。

## 1. 产品目标

将分散、异构、权利边界不清晰的原始数据，持续生产为：

1. 可理解、可治理、可交付、可审计的 Certified Dataset；
2. 基于这些数据集构建的 Data Product / Product Release。

## 2. 非目标

V1.1 第一阶段不定位为：

- 完整元数据平台
- 完整 ETL/湖仓平台
- 完整可信数据空间基础设施
- 数据交易所
- 财务 ERP
- 自动法律所有权裁判系统
- 完整合同管理系统
- 通用 AutoML / 特征平台
- 一次性完成生产 IAM / 灾备 / 大规模性能平台
- AI Gold Dataset 标注平台

## 3. 用户角色

- 产品经理：定义 Use Case、Data Product、Certification 用途
- 数据管理员：资源盘点、平台 Owner / Steward、分类
- 数据工程师：接入、标准化、加工
- 业务专家：指标、实体匹配、人工复核
- 治理人员：质量标准、QualityAssessment
- 合规/法务：RightsDeclaration verification、Authorization、Compliance Review
- 数据交付人员：Certification / Current Entitlement / Delivery
- Product Owner：Data Product Release 与生命周期
- 系统管理员：用户、权限、系统配置

## 4. 核心流程

### Certified Dataset

~~~text
Create Use Case
→ Register Data Resource
→ Record / Verify Rights Provenance
→ Dataset / DatasetVersion
→ Standardization
→ Entity Resolution
→ Processing
→ Quality Assessment
→ Rights / Compliance / Contract
→ Dataset Certification
→ Certified DatasetVersion
~~~

### Data Product

~~~text
Certified or governed DatasetVersion
→ Data Product / ProductVersion
→ Release Readiness
→ ProductRelease
~~~

第一阶段不强制所有 ProductRelease 只能引用 Certified DatasetVersion。

## 5. 一级模块

正式产品规划：

- 工作台
- 场景中心
- 数据资源
- 数据集
- 数据生产
- 实体中心
- 权利中心
- 数据质量
- 数据认证
- 安全合规
- 指标中心
- Data Contract
- 数据产品
- 成本中心
- 证据中心
- 可信流通
- 资产化准备
- 行业模板
- 系统管理

当前试点优先扩展现有 DatasetVersion 页面，不先扩张大量一级导航。

## 6. 核心业务对象

### 基础与生产

- Workspace / Project
- UseCase / ProductOpportunity
- DataResource / ResourceBinding
- Dataset / DatasetVersion / DatasetVersionLineage
- EntityType / Entity / EntityMapping / EntityMappingDecision
- Workflow / WorkflowVersion / Execution
- ExecutionDependencyBinding / MappingUsage

### Rights

- RightsDeclaration（#137）
- RightsVerification / RightsDisposition facts（#137）
- AuthorizationProvenanceBinding（#137）
- Authorization / ResourceGrant
- RightsSnapshot
- EffectiveRights（#137）

owner_id 仅表示平台资产责任/归属，不作为法律所有权证明。

### Quality

- QualityRuleSet
- QualityAssessment（演进现有 quality_result，#131）
- QualityFinding
- QualityDimensionSummary

V1.1 六个质量维度：

- Completeness
- Accuracy
- Consistency
- Uniqueness
- Timeliness
- Traceability

不强制全行业统一总分。

### Certification

- CertificationProfile（#134）
- DatasetCertification / CertificationDisposition（#134）

认证判断的是“DatasetVersion X 是否满足 CertificationProfile Y”。

### Product / Governance

- CompliancePolicy / ComplianceResult
- DataContract / ContractVersion
- DataProduct / ProductVersion / ProductAsset / ProductRelease
- CostEvent
- Evidence / EvidenceSnapshot / AuditEvent

## 7. 状态语义

### DatasetVersion

~~~text
CREATED → PROCESSING → READY
              └──────→ FAILED

READY → INVALID / SUPERSEDED
~~~

READY 表示数据内容已产生并冻结，不等于质量通过或认证通过。

### Authorization

~~~text
DRAFT → REVIEWING → APPROVED → ACTIVE
          └→ REJECTED           ├→ SUSPENDED
                                ├→ REVOKED
                                └→ EXPIRED
~~~

### RightsDeclaration

声明与验证事实分离。VERIFIED 历史声明不得被覆盖；修正通过新声明/新验证事实完成。

### QualityAssessment

每一次 Assessment 是不可变评测事实，不通过通用 PATCH 改写结果。

### DatasetCertification

~~~text
Evaluate Certification
        ↓
CERTIFIED
或
REJECTED
~~~

第一阶段即支持 append-only CertificationDisposition：

- REVOKED：撤销旧认证的当前可用性；
- SUPERSEDED：显式指向 replacement certification。

旧 Certification 不覆盖、不删除；CurrentCertificationGate 按 as_of 排除已生效 disposition。

### ProductRelease

FAILED 是当前 Domain / 数据库 / Read Model 仍支持的有效状态，本 docs-only 基线不废弃该分支。

~~~text
DRAFT
→ VALIDATING
├→ FAILED
└→ READY
   → PUBLISHED
      ├→ SUSPENDED
      └→ WITHDRAWN
~~~

## 8. Quality Assessment

QualityAssessment 必须绑定：

- DatasetVersion
- RuleSet ref/version
- RuleSet content hash / snapshot
- evaluator identity/version
- dimension summaries
- rule findings
- gate decision
- Evidence / Audit

规则文件后续改变不得改变历史 Assessment 的解释。

## 9. Data Rights

目标权利链：

~~~text
DataResource
→ RightsDeclaration
→ Authorization
→ RightsSnapshot
→ Effective Rights
~~~

第一阶段动作至少覆盖：

- USE
- PROCESS
- DERIVE
- SHARE
- RAW_EXPORT
- RESALE
- AI_TRAINING

多源衍生数据的 Effective Rights 默认 fail closed：任一必要输入禁止某动作，输出不得自动获得该动作。

平台不自动判断现实世界法律所有权；平台保存可验证的声明、依据和 Evidence。

## 10. Dataset Certification

CertificationProfile 至少定义：

- purpose / applicability
- required quality dimensions / critical rules
- Rights requirements
- Compliance requirements
- Contract requirements
- Traceability / Evidence requirements

DatasetCertification 必须绑定明确 DatasetVersion、QualityAssessment、Profile snapshot/hash 及所需 Rights/Compliance/Contract/Evidence。

任何 required 条件缺失或不匹配时，不得 CERTIFIED。

Certification 表示认证时点结论，不等于永久交付授权。

Certified Dataset 每次实际交付前必须执行 CurrentDeliveryGate：

~~~text
CurrentDeliveryGate
├── DatasetVersionUsability
├── CurrentCertificationGate
└── CurrentEntitlementGate
~~~

- DatasetVersionUsability 至少阻断 INVALID / FAILED / PROCESSING / CREATED；SUPERSEDED 按平台既有“明确历史版本”语义处理，不在本 docs-only 基线中自动等同 INVALID；
- CurrentCertificationGate 要求本次 delivery 绑定明确 DatasetCertification，decision = CERTIFIED 且未在 as_of 时点被 CertificationDisposition REVOKED / SUPERSEDED；requested purpose/action/consumer/delivery context 必须被该 Certification 冻结的 CertificationProfile snapshot 覆盖；禁止以 latest created_at 猜当前认证；
- CurrentEntitlementGate 按当前时间、consumer、purpose、action 检查 VERIFIED 且未被有效 INVALIDATED/SUPERSEDED 的 RightsDeclaration；每个 Authorization 必须通过 AuthorizationProvenanceBinding 证明其 grantor_ref 得到该 provenance 支持，并同时满足 Authorization 状态/有效期与 Effective Rights。

任一子门禁失败时，历史 Certification 保留，但当前交付必须 BLOCKED。第一阶段不要求周期性后台重认证。

## 11. CostEvent

QualityAssessment、Rights verification / invalidation / supersession、DatasetCertification evaluation / human approval 等实际活动发生时必须记录 CostEvent。

- 金额未知时不伪造金额，可记录 quantity/unit；
- 成本必须与实际活动同时记录，不在试点 KPI 阶段事后反推；
- 同一幂等业务动作重放不得重复产生 CostEvent。

## 12. Product Release Readiness

Release 发布前统一检查：

- Production
- Rights
- Quality
- Compliance
- Contract
- Dataset
- Evidence
- Delivery

ProductRelease Readiness 与 DatasetCertification 不互相替代。

## 13. 第一 Reference Implementation

POC 已验证 Raw / Standardized / Curated、Entity Resolution、Workflow / Execution、Quality / Compliance、Contract、Product Release、Cost / Evidence 和生产 decision trace。

Pilot 在同一链路增加：

- QualityAssessment rule snapshot
- 六维 Quality Report
- Data Rights Provenance
- Effective Rights
- CertificationProfile
- DatasetCertification
- Certified Dataset UI/API

最终至少产生一个由 reference fixture 驱动的 CERTIFIED CURATED DatasetVersion，并同时验证失败路径。

## 14. 第一阶段非目标

- T4/T5/T6 全部生产可靠性实现作为前置
- 完整 Billing / Settlement / Asset Accounting
- 区块链平台
- 高敏人脸/门禁/视频场景
- 复杂 AI 黑盒评分
- Label Studio / X-AnyLabeling / Gold Dataset
- 通用 CertificationProfile 在线设计器
- 完整法律合同管理与自动法律推理

## 15. 技术方向

- Frontend：React / Next.js
- Backend：Go
- Control DB：PostgreSQL
- Queue：Redis
- Object Storage：MinIO / S3 compatible
- Metadata Projection：OpenMetadata
- Processing Adapter：Native/Python/Hop
- Entity Adapter：Rules/Splink
- Quality Adapter：Native，未来可接 Soda/GX
- Compliance Adapter：Rules，未来可接 Presidio
- Annotation Adapter：第二阶段可接 Label Studio / X-AnyLabeling

Core Platform 必须与具体 Engine 解耦。
