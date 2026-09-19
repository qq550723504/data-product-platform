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

未来如需撤销认证，应追加 Revocation 事实，不修改旧 Certification。

## 7. DataProduct

重大变更通过创建新 ProductVersion，不修改历史版本。

## 8. ProductRelease

~~~text
DRAFT
→ VALIDATING
├→ FAILED
└→ READY
   → PUBLISHED
      ├→ SUSPENDED
      └→ WITHDRAWN
~~~

FAILED 是当前运行时和数据库仍支持的有效 ProductRelease 状态，不在本 docs-only 基线中废弃。

Published Release 不允许替换 DatasetVersion、Rights、Quality、Compliance、Contract 或 EvidenceSnapshot。

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
QueryEffectiveRights
~~~

具体 URL 由实现 PR 固定。

### Quality

当前 quality-checks API 在 #131 起应明确语义为创建新的 QualityAssessment。

### Certification

#134 目标：

~~~text
EvaluateDatasetCertification
GetDatasetCertification
ListDatasetCertifications
~~~

禁止 generic PATCH certification status。

## 12. 领域事件

事件名以实际路由表为准；新增事件必须显式加入 routing obligation。

Pilot 目标事件可能包括：

- RightsDeclarationCreated / Verified / Rejected
- QualityAssessmentCompleted 或兼容现有 QualityPassed/Failed/ReviewRequired
- DatasetCertified / DatasetCertificationRejected

如事件仅用于留存，也必须显式声明 retention-only。

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
