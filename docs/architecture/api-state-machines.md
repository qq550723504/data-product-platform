# API and State Machines V1.0

## 1. API Rule

业务动作使用 Command API，基础资料使用 CRUD API。

禁止通过通用 `PATCH status` 修改关键状态。

示例：

```text
POST /api/v1/product-releases/{id}/validate
POST /api/v1/product-releases/{id}/publish
POST /api/v1/product-releases/{id}/suspend
POST /api/v1/product-releases/{id}/withdraw
```

而不是：

```text
PATCH /product-releases/{id} { "status": "PUBLISHED" }
```

## 2. Command Flow

```text
HTTP Request
→ Command
→ Application Service
→ Aggregate
→ Repository
→ Domain Event
→ Transactional Outbox
```

关键 Command 需要考虑幂等性。

## 3. DatasetVersion State Machine

```text
CREATED
→ PROCESSING
├→ FAILED
└→ READY
   ├→ INVALID
   └→ SUPERSEDED
```

`READY` 表示数据已经产生并冻结，不代表 Quality / Compliance 通过。

## 4. DataProduct State Machine

```text
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
```

重大变更通过创建新 ProductVersion，而不是修改历史版本。

## 5. ProductRelease State Machine

```text
DRAFT
→ VALIDATING
├→ FAILED
└→ READY
   → PUBLISHED
      ├→ SUSPENDED
      └→ WITHDRAWN
```

Published Release 不允许替换 DatasetVersion、Rights、Quality、Compliance、Contract 或 EvidenceSnapshot。

## 6. Authorization State Machine

```text
DRAFT
→ REVIEWING
├→ REJECTED
└→ APPROVED
   → ACTIVE
      ├→ SUSPENDED
      ├→ REVOKED
      └→ EXPIRED
```

AuthorizationExpired 事件应触发 Impact Analysis。

## 7. Execution State Machine

```text
QUEUED
→ RUNNING
├→ WAITING → RUNNING
├→ FAILED
├→ CANCELLED
└→ SUCCEEDED
```

Retry 必须创建新的 Execution，并记录 `retry_of`。

## 8. Release Readiness

统一检查项：

- production
- rights
- quality
- compliance
- contract
- dataset
- evidence
- delivery
- approval（如适用）

每项状态：

- PASS
- FAIL
- WARNING
- PENDING

整体状态：

- READY
- NOT_READY
- REVIEW_REQUIRED

## 9. Core Command APIs

### Data Resource / Dataset

```text
POST /api/v1/data-resources
POST /api/v1/datasets
POST /api/v1/datasets/{id}/versions
POST /api/v1/dataset-versions/{id}/process
POST /api/v1/dataset-versions/{id}/invalidate
```

### Entity

```text
POST /api/v1/entity-match-jobs
GET  /api/v1/entity-match-reviews?status=PENDING
POST /api/v1/entity-match-reviews/{id}/confirm
POST /api/v1/entity-match-reviews/{id}/reject
```

### Workflow

```text
POST /api/v1/workflows
POST /api/v1/workflows/{id}/versions
POST /api/v1/workflow-versions/{id}/execute
POST /api/v1/executions/{id}/retry
POST /api/v1/executions/{id}/cancel
```

### Rights

```text
POST /api/v1/authorizations
POST /api/v1/authorizations/{id}/submit
POST /api/v1/authorizations/{id}/approve
POST /api/v1/authorizations/{id}/activate
POST /api/v1/authorizations/{id}/suspend
POST /api/v1/authorizations/{id}/revoke
```

### Quality / Compliance

```text
POST /api/v1/dataset-versions/{id}/quality-checks
POST /api/v1/dataset-versions/{id}/compliance-checks
```

### Contract / Product

```text
POST /api/v1/data-contracts
POST /api/v1/data-contracts/{id}/versions
POST /api/v1/contract-versions/{id}/publish

POST /api/v1/data-products
POST /api/v1/data-products/{id}/versions
POST /api/v1/data-products/{id}/releases
POST /api/v1/product-releases/{id}/validate
POST /api/v1/product-releases/{id}/publish
```

## 10. Domain Events

Initial event vocabulary:

```text
UseCaseApproved
DataResourceReady
AuthorizationActivated
AuthorizationExpired
DatasetVersionCreated
DatasetVersionInvalidated
EntityMatchCompleted
WorkflowStarted
WorkflowCompleted
WorkflowFailed
QualityPassed
QualityFailed
CompliancePassed
ComplianceFailed
ContractPublished
ProductReady
ProductReleased
ProductSuspended
CostEventRecorded
EvidenceCreated
```

## 11. Error Model

Business APIs return structured errors, e.g.:

```json
{
  "code": "PRODUCT_RELEASE_NOT_READY",
  "message": "Product release is not ready for publishing.",
  "details": {
    "failedChecks": ["RIGHTS", "QUALITY"]
  },
  "traceId": "..."
}
```

Engine-specific raw errors must not leak as Core business errors.

## 12. Override

Gate Override is allowed only as an explicit audited command.

It must preserve both:

- original decision
- effective decision

and record:

- reason
- approver
- scope
- expiry
- AuditEvent
- Evidence
