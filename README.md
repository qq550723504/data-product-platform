# Data Product Platform

数据产品与高质量数据集生产治理平台（Data Product Production & Governance Platform）。

平台的核心目标，是把分散、异构、权利边界不清晰的原始数据，持续生产为可理解、可治理、可追溯、可认证、可交付的数据集与数据产品。

## 当前阶段

核心 POC 与 Certified Dataset MVP 主链已经完成，#136 enterprise-activity 纵向 E2E Pilot 现在是当前主线。此前为 Pilot 定义的 #99 / #100 定向可靠性 blocker 已全部收敛，不再继续扩底座范围。

~~~text
核心 POC                              ✅
T1/T2/T3-B2 可靠性与生产事实冻结       ✅
Certified Dataset Quality/Rights       ✅
DatasetCertification + Read/API/UI      ✅
Trusted DIRECT_DATA Delivery            ✅
#99 / #100 定向 Reliability Closeout   ✅
        ↓
#136 enterprise-activity E2E Pilot      ← 当前
        ↓
AI 高质量数据集 / Gold Dataset          后续
        ↓
可信数据空间 / 数据产品规模化交付        后续
~~~

当前主 Epic：#129 高质量数据集生产与认证（OPEN；#136 第一阶段纵向验收完成后再关闭）。

第一阶段任务状态（截至 2026-09-22）：

- #131 QualityAssessment follow-up / typed CostAllocation ✅ 完成（核心 QualityAssessment 与规则快照由 #140 落地）
- #132 通用 industry-pack Quality Engine ✅ 完成
- #133 Quality Report ✅ 完成
- #137 Data Rights Provenance / Effective Rights ✅ 完成
- #134 DatasetCertification / CertificationProfile ✅ 完成（PR #149）
- #135A Certified Dataset Read/API/UI ✅ 完成（PR #152）
- #135B Trusted server-side DIRECT_DATA delivery ✅ 完成（PR #173；PR #174 补齐 retry authorization context）
- #159 Pre-production compatibility cleanup ✅ 完成
- #99 / #100 与 #136 直接相关的 Reliability Closeout ✅ 完成（#176 / #178 / #179 / #180）
- #136 enterprise-activity 纵向试点验收 🚧 **当前主线**
- #161 DatasetCertification history pagination ⏳ P2 follow-up，**不阻塞 #136**

#136 前的定向 Reliability Closeout 已完成：

- #99：EvidenceSnapshot header/hash/membership freeze — #180；
- #100：Native RUNNING crash recovery + output ↔ terminal convergence — #176；
- #100：Entity Match finalize claim/idempotency — #178；
- #100：Authorization expected-status CAS — #179。

#99 / #100 的其它 ProductRelease/ProductVersion freeze、Metadata stale projection、Hop unknown submit、manual upload idempotency 等继续作为 Core reliability debt 跟踪，**不阻塞当前 Certified Dataset DIRECT_DATA Pilot**，除非 #136 实际复现相关问题。

T4/T5/T6、完整生产 IAM、bearer/presigned credential provider、provider containment/recovery、灾备、性能压测以及 Label Studio / X-AnyLabeling 不作为 Certified Dataset MVP 第一阶段前置条件。

## 本地试用入口

当前仓库仍保留 POC 合成数据演示和 CSV 接入切片。CI 通过不等于商业上线批准。

~~~sh
node deploy/demo/demo.mjs doctor
node deploy/demo/demo.mjs up
~~~

- 接入合成 CSV：http://127.0.0.1:3180/ingest
- POC CSV 说明：docs/poc/csv-ingestion-v1.md
- 本地演示：docs/poc/local-demo.md
- 停止并保留数据：node deploy/demo/demo.mjs down

当前 POC 身份配置仅用于本地演示，不应用于公网或真实客户数据。

## Start here

建议按以下顺序阅读：

1. docs/product/product-vision.md
2. docs/product/prd-v1.md
3. docs/product/certified-dataset-pilot.md
4. docs/architecture/certified-dataset.md
5. docs/architecture/data-rights-provenance.md
6. docs/architecture/domain-model.md
7. docs/architecture/system-architecture.md
8. docs/poc/poc-technical-plan-v1.md
9. examples/enterprise-activity/README.md

## 核心业务链

~~~text
Use Case
   ↓
Data Resource
   ↓
Rights Provenance
   ↓
Dataset / DatasetVersion
   ↓
Standardization
   ↓
Entity Resolution
   ↓
Transformation
   ↓
Quality Assessment
   ↓
Rights / Compliance / Contract
   ↓
Dataset Certification
   ↓
Certified Dataset
   │
   ├── Data Product / Product Release
   ├── Trusted Data Offering
   ├── AI Dataset（后续）
   └── External Delivery
~~~

全生命周期能力：

~~~text
Rights · Version · Lineage · Quality · Cost · Evidence · Audit
~~~

Certified Dataset 是可独立交付成果，不要求必须包装为 DataProduct；但每次 standalone delivery 都必须通过 CurrentDeliveryGate（DatasetVersion usability + CurrentCertificationGate + CurrentEntitlementGate）。历史 CERTIFIED 结果本身不代表当前仍有交付权限或数据版本仍可用。

## 核心建模原则

### DatasetVersion 是数据事实

不新增一套 HighQualityDataset 主实体。

~~~text
DatasetVersion
+ QualityAssessment
+ Effective Rights
+ Compliance
+ Contract
+ Traceability / Evidence
+ CertificationProfile
+ DatasetCertification
= Certified DatasetVersion
~~~

数据内容变化必须产生新的 DatasetVersion，并重新评测、重新认证。

### 平台归属不等于法律所有权

data_resource.owner_id / dataset.owner_id 表示平台内资产责任或归属，不自动等同现实世界法律上的数据所有权。

目标权利链：

~~~text
RightsDeclaration
→ AuthorizationProvenanceBinding
→ Authorization
→ RightsSnapshot
→ Effective Rights
→ DatasetCertification
~~~

平台记录权利声明、依据、授权、限制与证据，不替代法律机构判断现实世界所有权。

## 架构原则

1. Core Platform 数据库是核心业务事实的 System of Record。
2. OpenMetadata 仅作为 Governance Projection。
3. Processing / Entity / Quality / Annotation 等外部能力通过 Port / SPI + Adapter 接入。
4. DatasetVersion、不可变映射决定、QualityAssessment、RightsSnapshot、DatasetCertification 等历史事实不得原地覆盖。
5. 关键状态变化通过显式 Command；异步副作用使用 Transactional Outbox。
6. Cost 与 Evidence 是一等业务对象。
7. 行业逻辑通过 Industry Pack 扩展，不污染 Core Domain。
8. 当前继续采用 Modular Monolith；是否拆微服务由真实试点需求决定。

## 第一 Reference Implementation

第一 Reference Implementation 使用园区场景：

- Use Case：企业融资风险辅助
- Data Product：企业经营活跃度
- 输入：企业基础信息、租赁、能耗

POC 已验证：

- DataResource → Dataset → DatasetVersion
- Company Entity Resolution
- Processing Workflow / frozen execution dependencies
- Quality / Compliance Gate
- Data Contract
- Product Release
- Cost / Evidence
- Release → Execution → Mapping Decision 冻结追溯

当前使用同一参考场景直接执行 #136 Certified Dataset 纵向 E2E Pilot。Pilot 只验证已经落地的受控 DIRECT_DATA 闭环：RAW → Entity Resolution → CURATED → Quality → Rights → Certification → CurrentDeliveryGate → DIRECT_DATA；不把 bearer/presigned credential provider、containment/recovery 或完整企业 IAM 扩入第一阶段。

activity_score 等 V1 指标仅用于验证数据生产生命周期和可解释性，不是经过验证的授信模型。

## POC 历史基线

原 POC Sprint 0–6 已完成。历史目标与验收边界保留在：

- docs/poc/poc-technical-plan-v1.md
- apps/platform/internal/acceptance/enterprise_activity_poc_test.go

当前试点不反向改写 POC 的历史完成定义。

## 仓库可见性

仓库维持 Public。

不得提交真实客户数据、非公开合同正文、非公开权利证明原件、生产凭证或高敏个人数据。examples/ 和测试使用合成数据；真实试点应使用独立受控环境。
