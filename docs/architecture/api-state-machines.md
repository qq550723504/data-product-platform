# API 与状态机 V1.1

## 1. API 规则

业务动作使用 Command API，基础资料使用 CRUD API。

禁止通过通用 PATCH status 修改关键状态。

~~~text
HTTP Request
→ Command
→ Application Service
→ Domain Rule
→ Repository
→ Domain Event
→ Audit / Evidence
→ Transactional Outbox
~~~

关键 Command 必须考虑幂等与 workspace 边界。

## 2. DatasetVersion

~~~text
CREATED
→ PROCESSING
├→ FAILED
└→ READY
   ├→ INVALID
   └→ SUPERSEDED
~~~

READY 表示数据已产生并冻结，不代表 Quality / Compliance / Certification 通过。

## 3. Authorization

~~~text
DRAFT
→ REVIEWING
├→ REJECTED
└→ APPROVED
   → ACTIVE
      ├→ SUSPENDED
      ├→ REVOKED
      └→ EXPIRED
~~~

## 4. RightsDeclaration（#137）

RightsDeclaration 与 verification 分离。

推荐业务动作：

~~~text
CreateRightsDeclaration
→ VerifyRightsDeclaration
或
→ RejectRightsDeclaration
~~~

VERIFIED 历史声明不得通过通用 UPDATE 改成另一份权利事实；修正应创建新声明/新验证事实。

平台不提供暗示自动法律裁判的 SetLegalOwner 命令。

## 5. QualityAssessment（#131）

~~~text
RunQualityAssessment
        ↓
immutable assessment result
~~~

每次运行产生新的评测事实。规则版本/hash 与 DatasetVersion 必须冻结。

不要通过 PATCH 把旧 Assessment 从 FAIL 改成 PASS。

## 6. DatasetCertification（#134）

~~~text
EvaluateDatasetCertification
        ↓
CERTIFIED
或
REJECTED
~~~

认证结果是不可变事实。

DatasetVersion V2 不继承 V1 Certification。

第一阶段必须支持认证退出 current set，而不是以后再补：

~~~text
DatasetCertification C1
├── append CertificationDisposition(REVOKED)
└── append CertificationDisposition(SUPERSEDED → C2)
~~~

旧 Certification 不修改、不删除；CurrentCertificationGate 按 as_of 排除已生效的 REVOKED / SUPERSEDED disposition。

## 7. DataProduct

~~~text
DRAFT
→ DESIGNING
→ DEVELOPING
→ TESTING
→ READY
→ PUBLISHED
→ ACTIVE
↔ SUSPENDED
→ DEPRECATED
→ RETIRED
~~~

以上状态仍由当前 Domain / 数据库 / Read Model 支持，本 docs-only 基线不废弃任何现有 DataProduct lifecycle 状态。

重大规格变更通过创建新 ProductVersion，不修改历史 ProductVersion。

## 8. ProductRelease

当前 live 可达状态机：

~~~text
DRAFT
→ VALIDATING
└→ READY
   → PUBLISHED
~~~

Readiness 不满足时，当前 `ValidateRelease` 返回 non-ready `ReadinessResult` 并保持状态为 `VALIDATING`；当前没有任何 Command 写入 `FAILED`。

`FAILED` / `SUSPENDED` / `WITHDRAWN` 仍是 domain/schema reserved values，但当前不是 live reachable transitions：

- `FAILED`：没有 `VALIDATING → FAILED` Command；
- `SUSPENDED` / `WITHDRAWN`：`guard_product_release_history` 拒绝 OLD.status=PUBLISHED 的任何 UPDATE，且没有 suspend/withdraw Command。

本 docs-only 基线不删除这些枚举，但客户端不得等待或假设当前 Command 会产生这些状态。

未来若要启用 reserved states，必须由独立实现同时提供：

- 显式 Domain/Application Command；
- 必要 migration / database guard 调整；
- published bindings 继续冻结；
- Domain Event / Audit / Outbox / 幂等 / 并发测试。

Published Release 当前整行受历史 guard 保护，不允许普通 UPDATE。

## 9. Execution

~~~text
QUEUED
→ RUNNING
├→ WAITING → RUNNING
├→ FAILED
├→ CANCELLED
└→ SUCCEEDED
~~~

Retry 创建新的 Execution，并保留历史关系。

生产实际使用的输入、解析决定和策略快照通过不可变 dependency facts 记录。

## 10. Release Readiness

统一检查：

- production
- rights
- quality
- compliance
- contract
- dataset
- evidence
- delivery
- approval（如适用）

DatasetCertification 不是 ProductRelease readiness 的替代品。

## 11. Command API 词汇

### Data Resource / Dataset

~~~text
POST /api/v1/data-resources
POST /api/v1/datasets
POST /api/v1/datasets/{id}/versions
POST /api/v1/dataset-versions/{id}/invalidate
~~~

### Entity

~~~text
POST /api/v1/entity-match-jobs
GET  /api/v1/entity-match-reviews?status=PENDING
POST /api/v1/entity-match-reviews/{id}/confirm
POST /api/v1/entity-match-reviews/{id}/reject
~~~

### Workflow

~~~text
POST /api/v1/workflows
POST /api/v1/workflows/{id}/versions
POST /api/v1/workflow-versions/{id}/execute
POST /api/v1/executions/{id}/retry
~~~

### Rights

已有：

~~~text
POST /api/v1/authorizations
POST /api/v1/authorizations/{id}/submit
POST /api/v1/authorizations/{id}/approve
POST /api/v1/authorizations/{id}/activate
POST /api/v1/authorizations/{id}/suspend
POST /api/v1/authorizations/{id}/revoke
~~~

#137 目标命令：

~~~text
CreateRightsDeclaration
VerifyRightsDeclaration
RejectRightsDeclaration
InvalidateRightsDeclaration
SupersedeRightsDeclaration
BindAuthorizationProvenance / CreateAuthorizationProvenanceBinding
QueryEffectiveRights
QueryCurrentEntitlement
~~~

具体 URL 由实现 PR 固定。

### Quality（当前 live API）

~~~text
POST /api/v1/dataset-versions/{versionId}/quality-checks
GET  /api/v1/quality-results/{resultId}
GET  /api/v1/quality-assessments/{assessmentId}
GET  /api/v1/dataset-versions/{versionId}/quality-assessments
GET  /api/v1/dataset-versions/{versionId}/quality-assessments/latest
~~~

其中 `POST .../quality-checks` 在 #131 起语义明确为创建新的 QualityAssessment；历史兼容 query route 仍保留。

### Compliance（当前 live API）

~~~text
POST /api/v1/dataset-versions/{versionId}/compliance-checks
GET  /api/v1/compliance-results/{resultId}
~~~

Quality / Compliance POST routes 产生 Certification 与 ReleaseReadiness 消费的 gate facts，本 docs-only 基线不得省略这些现有 Command API。

### Certification

#134 目标：

~~~text
EvaluateDatasetCertification
GetDatasetCertification
ListDatasetCertifications
RevokeDatasetCertification
SupersedeDatasetCertification
GetCurrentDatasetCertification / QueryCurrentCertification
~~~

Current certification 不能按 latest timestamp 推断；delivery 必须绑定明确有效的 CERTIFIED 事实，并排除已生效 REVOKED / SUPERSEDED disposition。

禁止 generic PATCH certification status。

### Certified Dataset Delivery（#135 目标 Command）

Eligibility query 不是交付授权边界。第一阶段必须有一个真正的服务端交付 Command，例如：

~~~text
DeliverDatasetVersion
或
IssueDatasetAccess
~~~

具体 HTTP URL 由 #135 实现 PR 固定。

该 Command 在返回数据或签发下载 URL / token / credential 前，必须重新执行：

~~~text
CurrentDeliveryGate
├── DatasetVersionUsability
├── CurrentCertificationGate
└── CurrentEntitlementGate
~~~

客户端不能通过先调用 eligibility query 再跳过 gate。query 结果不得作为后续 delivery 的授权凭证。

对于外部 credential provider / 对象存储签名服务，delivery Command 必须使用 crash-safe issuance protocol：

~~~text
DeliveryOperation PREPARED
      ↓ durable DB commit
ISSUANCE_PENDING + stable provider_request_key
      ↓ external idempotent issuance
      ├→ ISSUED
      └→ FAILED
~~~

- 外部 provider 调用不属于 PostgreSQL transaction；
- terminal DeliveryOperation + Audit/Evidence + Outbox/CostEvent（如有）在后续 DB transaction 内一致提交；
- provider 成功但 terminal commit 失败时，retry/reconciliation 使用同一 provider_request_key；
- provider 若不支持 idempotency/read-after-write 或 revoke/compensation，则第一阶段不得直接暴露其 bearer credential，只能通过 platform redemption indirection 或标记该 mode unsupported。

### Contract（当前 live API）

~~~text
POST /api/v1/data-contracts/versions
GET  /api/v1/contract-versions/{versionId}
POST /api/v1/contract-versions/{versionId}/publish
~~~

### Product / Release（当前 live API）

~~~text
POST /api/v1/data-products
GET  /api/v1/data-products/{productId}
POST /api/v1/data-products/{productId}/versions
GET  /api/v1/product-versions/{versionId}
POST /api/v1/data-products/{productId}/releases
GET  /api/v1/product-releases/{releaseId}
GET  /api/v1/product-releases/{releaseId}/readiness
POST /api/v1/product-releases/{releaseId}/validate
POST /api/v1/product-releases/{releaseId}/publish
~~~

以上 Contract/Product routes 是当前运行时已注册的 live API 边界；本目录不是所有 GET/query route 的穷举，但不得省略支撑本文状态机和 ReleaseReadiness 的现有 Command routes。

## 12. 领域事件

事件名以实际路由表为准；新增事件必须显式加入 routing obligation。

Pilot 第一阶段事件词汇至少包括：

- RightsDeclarationCreated
- RightsDeclarationVerified
- RightsDeclarationRejected
- RightsDeclarationInvalidated
- RightsDeclarationSuperseded
- AuthorizationProvenanceBound
- QualityAssessmentCompleted（或继续兼容现有 QualityPassed / QualityFailed / QualityReviewRequired）
- DatasetCertified
- DatasetCertificationRejected
- DatasetCertificationRevoked
- DatasetCertificationSuperseded
- DatasetDeliveryIssued
- DatasetDeliveryBlocked
- DatasetDeliveryFailed

Disposition Command 与对应业务事实、AuditEvent、Evidence、Outbox event 应在同一事务边界内提交。

#135 delivery command 的每个终态结果也必须产生明确 Domain Event：
- gate 通过并完成数据/credential issuance → `DatasetDeliveryIssued`；
- CurrentDeliveryGate fail closed、没有签发任何可用访问能力 → `DatasetDeliveryBlocked`；
- gate 通过但实际 delivery/issuance 因系统或外部错误失败 → `DatasetDeliveryFailed`。

DeliveryOperation **terminal database fact** + Audit/Evidence + CostEvent（如有）+ Outbox 必须在同一数据库事务内保持一致，并具备幂等语义；这里不包含外部 provider side effect。外部 issuance 依赖 stable provider_request_key + retry/reconciliation/compensation 协议。事件 payload 不得包含可用 token/credential secret。

每个新增 event_type 都必须显式加入统一 routing 表，明确 required handlers 集合或 retention-only 义务；不得因为“暂时没有异步处理器”而省略 routing declaration。若某个 Issue 引入异步 impact/projection 副作用，则对应 handler 必须成为该事件的 required obligation；同步 CurrentDeliveryGate 仍是交付安全的最终业务门禁。

## 13. 错误模型

Certification / Rights 错误应能够表达明确 blocker，例如：

- RIGHTS_PROVENANCE_MISSING
- RIGHTS_ACTION_NOT_ALLOWED
- QUALITY_ASSESSMENT_MISSING
- QUALITY_CRITICAL_RULE_FAILED
- COMPLIANCE_BLOCKING
- CONTRACT_MISSING
- CERTIFICATION_PROFILE_MISMATCH

具体 code 由实现 Issue 固化。

## 14. Override

Gate Override 必须显式、可审计并保留 original decision、effective decision、reason、approver、scope、expiry、AuditEvent 和 Evidence。

第一阶段 DatasetCertification 不默认支持 override，除非对应 Issue 明确设计。
