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

外部 credential issuance 使用 DB-first crash-safe protocol：
1. 先持久化 DeliveryOperation PREPARED/ISSUANCE_PENDING + stable provider_request_key；
2. DB commit 成功后才执行外部 issuance；
3. provider 成功后再提交 terminal DeliveryOperation + Audit/Evidence/Outbox/CostEvent；
4. terminal commit 成功后才向客户端暴露 credential；
5. 每次 initial/retry/reconciliation 真正调用 provider 前重新执行 CurrentDeliveryGate，并重新计算 expiry cap；prepare 阶段的旧 gate snapshot 不授权后续外部 side effect；
6. provider 返回/恢复 access capability 后，terminal finalize 必须在共享 delivery authorization fence/revision 下重新读取 current facts、重新 gate、重新计算 fresh cap；影响 gate 的 Rights/Binding/Certification disposition 与 DatasetVersion invalidation 等 Command 使用同一 fence/revision，并按固定顺序锁定；
7. terminal ISSUED DB commit 是 issuance 的线性化点：若 entitlement 变更先提交，finalize 必须看到它并 contain/block/fail；若 finalize 先提交，则后续 entitlement 变更在线性顺序上发生在 issuance 之后，并按 delivery mode 的 revocation semantics 处理已签发 capability；
8. crash/timeout 由 reconciliation 使用同一 provider_request_key 恢复，不盲目重复签发；
9. direct bearer delivery 只有在 provider 能按同一 key 恢复同一 credential/访问能力时允许；否则使用 platform redemption indirection。

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

外部副作用不属于 PostgreSQL transaction；必须在事务提交后执行，并通过稳定 idempotency key、read-after-write/reconciliation 或 revoke/compensation 处理 crash consistency。Delivery credential issuance 禁止用“外部调用 + DB commit 看起来像一个事务”的假原子模型。

新事件类型必须进入统一 routing 表，并显式声明 required handlers 或 retention-only。

## 7. Worker 职责

- Outbox dispatcher / handler
- Workflow 任务处理
- Engine 对账/维护任务（按已实施范围）
- DeliveryOperation ISSUANCE_PENDING reconciliation / provider outcome recovery（#135 起）
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
