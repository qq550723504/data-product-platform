# 系统架构 V1.1

## 1. 架构风格

当前采用模块化单体 + 引擎适配器 + 事务性 Outbox。

~~~text
Web / Next.js
      │
      ▼
platform-api (Go)
      │
      ├── Core Modules
      │   ├── resource / dataset
      │   ├── entity
      │   ├── workflow
      │   ├── rights
      │   ├── quality
      │   ├── certification   (#134 起)
      │   ├── compliance
      │   ├── contract
      │   ├── product
      │   ├── cost
      │   └── evidence
      │
      ├── PostgreSQL
      ├── Redis
      └── Object Storage

platform-worker (Go)
      │
      ├── Outbox dispatcher / handlers
      ├── Workflow execution
      ├── Maintenance / reconciliation
      └── Projection jobs

Engine Adapter Layer
      ├── MetadataEngine → OpenMetadata
      ├── ProcessingEngine → Native/Python/Hop
      ├── EntityResolutionEngine → Rules/Splink
      ├── QualityEngine → Native / future Soda/GX
      ├── ComplianceEngine → Rules / future Presidio
      └── AnnotationEngine → future Label Studio/X-AnyLabeling
~~~

## 2. Core Platform 业务真相

Core 保存：

- DataResource / DatasetVersion
- RightsDeclaration / verification / disposition / Authorization / RightsSnapshot / Effective Rights
- EntityMappingDecision
- Workflow / Execution / frozen execution dependencies
- QualityAssessment
- Contract / Compliance
- DatasetCertification
- ProductVersion / ProductRelease
- Cost / Evidence / Audit

其中 QualityAssessment 核心已由 #140 / migration 000019 落地；#134/#137/#135 的 Certification/Rights/DeliveryOperation 对象在各 Issue 合入前属于目标模型。

外部 Engine 只提供执行能力，不拥有上述核心业务状态。

## 3. 控制面 / 数据面

### 控制面

PostgreSQL 保存业务元数据、不可变版本和证明：

- Resource / Dataset metadata
- DatasetVersion / lineage
- Entity decisions
- Rights provenance / authorization / snapshots
- Workflow / Execution
- Quality assessments
- Certification
- Product / Release
- Cost / Evidence

### 数据面

真实大规模数据放在：

- Object Storage
- PostgreSQL / Doris / warehouse
- 其他外部数据系统

核心控制数据库不承载大规模 Dataset 内容。

## 4. Certified Dataset 生产链

~~~text
DataResource
  ↓
Rights Provenance
  ↓
DatasetVersion
  ↓
Entity Resolution / Processing
  ↓
QualityAssessment
  ↓
Effective Rights / Compliance / Contract
  ↓
DatasetCertification
  ↓
Certified DatasetVersion
~~~

Certified Dataset 可以作为独立交付对象，也可以继续进入 Data Product / ProductRelease；独立交付必须由 server-side delivery command 执行。该 Command 在返回数据或签发 URL/token/credential 前重新执行 CurrentDeliveryGate：检查 DatasetVersion 当前可用性、CurrentCertificationGate（明确且未 REVOKED/SUPERSEDED 的 CERTIFIED 事实），再通过 CurrentEntitlementGate 重新校验当前 Rights provenance / Authorization / Effective Rights。Eligibility query 不能替代 delivery-time gate。

## 5. Governance Projection

OpenMetadata 是治理投影，不拥有：

- Rights
- DatasetVersion
- production execution facts
- QualityAssessment
- DatasetCertification
- Cost
- Evidence
- ProductRelease

Projection 故障不得改变 Core 业务真相。

## 6. 事件模型

关键业务动作在数据库事务内写业务事实、Audit/Evidence 和 Outbox。

外部副作用在事务提交后通过 dispatcher 执行。

新事件类型必须进入统一 routing 表，并显式声明 required handlers 或 retention-only。

## 7. Worker 职责

- Outbox dispatcher / handler
- Workflow 任务处理
- Engine 对账/维护任务（按已实施范围）
- 授权过期处理
- 元数据投影
- 后续可加入周期性质量/认证维护，但不属于当前 MVP 前置

## 8. Industry Pack

Industry Pack 提供：

- Entity Types
- Glossary
- Standardization Rules
- Matching Policies
- Indicators
- Quality Rules
- Compliance Rules
- Certification Profiles
- Product Templates

Core 不允许出现 PARK 等行业专属分支。

## 9. 当前阶段边界

当前是 #129 Certified Dataset 受控试点。

第一阶段优先验证业务闭环，不以以下事项作为前置：

- T4/T5/T6 全部可靠性实现
- 完整 IAM / 灾备 / 性能平台
- 微服务拆分
- Label Studio / X-AnyLabeling 第二阶段
- 数据市场 / Billing
