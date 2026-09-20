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

## 5. QualityAssessment（核心已由 #140 落地；#131 仅 follow-up / CostAllocation）

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

当前 API 可达生命周期：

~~~text
DRAFT
  ↓ PublishProductRelease side effect
PUBLISHED
~~~

DataProduct 创建时为 `DRAFT`。当前没有显式 Domain/Application Command 将 Product 迁移到 `DESIGNING`、`DEVELOPING`、`TESTING`、`READY`、`ACTIVE`、`SUSPENDED`、`DEPRECATED` 或 `RETIRED`。

当前 `PublishProductRelease` 成功时，repository 会把处于 `DRAFT/DESIGNING/DEVELOPING/TESTING/READY` 的 Product lifecycle_status 更新为 `PUBLISHED`；在正常公开 API 流程中，新建 Product 因而表现为 `DRAFT → PUBLISHED`。

其余 lifecycle values 仍由 domain/schema/read model 枚举保留，但属于 **reserved / not currently command-reachable**。本 docs-only 基线不删除这些值，也不向客户端宣称现有 API 可以驱动这些迁移。

未来若启用完整 DataProduct lifecycle，必须新增显式 Command/API、Domain Event/Audit/Outbox、状态迁移约束和测试；不得通过 generic PATCH lifecycle_status 实现。

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
- published bindings 的**领域 invariant**继续冻结；数据库层 `product_release_dataset` membership guard 仍由 #99 补齐，未来 lifecycle migration 不得扩大该现有缺口；
- Domain Event / Audit / Outbox / 幂等 / 并发测试。

Published Release 当前**主行**受 `guard_product_release_history` 保护，不允许普通 UPDATE/DELETE；`product_release_dataset` membership 尚未由同等数据库 guard 保护（#99 open），因此当前实现状态不能描述为“published bindings 已完整 DB-frozen”。

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
InvalidateAuthorizationProvenanceBinding
SupersedeAuthorizationProvenanceBinding
ComputeEffectiveRights / FinalizeEffectiveRights
# 或单一原子 ComputeAndFinalizeEffectiveRights（由实现 PR 固定）
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

其中 `POST .../quality-checks` 创建 QualityAssessment 的核心语义、模型与查询路由已由 #140 落地；#131 当前仅承接 typed CostAllocation / retry-cost 等 review follow-up，不得重新创建 QualityAssessment root、平行表族或重复 migration。历史兼容 query route 仍保留。

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

Current certification 不能按 latest timestamp 推断；delivery 必须绑定明确有效的 CERTIFIED 事实，并排除已生效 REVOKED / SUPERSEDED disposition。frozen CertificationProfile 对 purpose/action/consumer/delivery channel/mode 必须逐维度显式 ANY/EXPLICIT 覆盖；缺失/NULL/UNKNOWN 维度一律 fail closed。

禁止 generic PATCH certification status。

### Certified Dataset Delivery（#135 目标 Command）

Eligibility query 不是交付授权边界。第一阶段必须有一个真正的服务端交付 Command，例如：

~~~text
DeliverDatasetVersion
或
IssueDatasetAccess
~~~

具体 HTTP URL 由 #135 实现 PR 固定。

该 Command 在返回数据或签发下载 URL / token / credential 前，必须重新执行；并且所有 delivery mode 都必须在第一个外部可观察交付副作用前完成共享 fence 下的 terminal finalize。

在 CurrentDeliveryGate 之前必须先建立可信调用者上下文：authenticated caller principal → effective consumer/workspace；on-behalf-of 必须有服务端验证的当前有效 delegation。请求 header/query/body/demo actor ID 不能自证 consumer。principal binding / workspace membership / delegation 也是 delivery authorization dependency，必须参与与 entitlement facts 相同强度的 fence/revision（或等价串行化机制）。

~~~text
TrustedCallerResolution
├── AuthenticatedPrincipal
├── Principal→Consumer/Workspace Binding
└── Delegation (when on-behalf-of)
        ↓
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
      ├→ ISSUED
      ├→ FAILED
      ├→ BLOCKED
      └→ CONTAINMENT_PENDING
             ├→ BLOCKED
             └→ FAILED
~~~

- 外部 provider 调用不属于 PostgreSQL transaction；
- **每一次 initial issuance、retry issuance、以及 reconciliation 决定继续 issuance 前，都必须重新验证 authenticated principal 当前仍可代表 effective consumer/workspace（含 delegation/membership/binding），再重新读取当前事实、执行完整 CurrentDeliveryGate，并重新计算 credential expiry cap；PREPARED/ISSUANCE_PENDING 中旧 identity/gate snapshot 仅用于审计；**
- 每一次上述 gate 以及 terminal finalize / credential replay gate 都必须 append immutable `DeliveryGateEvaluation`（或等价 fact），记录 evaluation_kind、decision/blockers、trusted caller/effective consumer/context、dependency fence/revision vector/hash、fresh cap（如适用）；DeliveryOperation 上的 current gate/status 只是 projection。INITIAL=ALLOWED、RECONCILIATION=BLOCKED 等多次判断必须同时保留，不能 UPDATE 覆盖。
- 所有 delivery mode 的 terminal finalize 必须获取共享 delivery authorization fence/revision，并在同一 terminal transaction 内重新验证 principal→consumer/workspace binding/caller delegation + CurrentDeliveryGate；若 CurrentEntitlementGate 使用 delegated grantor authority，grantor delegation chain/member edge/disposition revision 也必须纳入同一 fence/dependency vector。provider/credential 模式还需 fresh-cap；caller identity 或 grantor delegation revoke/expiry 都不得穿越 terminal finalize；
- direct-data 模式必须先提交 ISSUED terminal fact，再允许写出 HTTP body/stream/file 的第一字节；commit 前 response body 必须为 0 bytes；
- direct-data terminal `ISSUED` 只表示该 attempt 已在线性化点获准开始响应，不证明客户端已收到全部数据。若 ISSUED commit 后在第一字节前 crash/socket loss，或 stream 中断，同一 idempotency key 的 retry **不得**使用旧 gate/旧 ISSUED 重新发送 DatasetVersion bytes；必须返回稳定 non-payload replay-required 结果（例如 `DIRECT_DATA_REPLAY_REQUIRES_NEW_ATTEMPT` + 原 DeliveryOperation ID/状态），dataset payload=0 bytes；
- direct-data 需要重新传输时必须创建新的显式 DeliveryOperation/attempt（新的 idempotency key，可记录 `retry_of_delivery_operation_id`）。新 attempt 必须重新解析 authenticated principal→effective consumer/workspace/delegation、重新执行 CurrentDeliveryGate，并重新进入 delivery authorization fence/terminal finalize；如果两次 attempt 之间发生 caller binding/delegation revoke、Rights/Certification/Authorization 变化或 DatasetVersion invalidation，新 attempt 必须 fail closed；
- direct-data 不得在整个 stream 期间持有 fence/DB row lock；锁仅覆盖 terminal re-gate + commit；
- fresh gate BLOCKED 时：
  - 若确认此前未发生 provider issuance，可直接 BLOCKED；
  - 若 provider_request_key 可能已产生访问能力，必须先 reconciliation 查询既有 outcome；
  - 已签发则先 revoke/compensate/contain，确认访问能力已不可用后才能 BLOCKED；
  - outcome unknown 或 containment 未确认成功时进入 CONTAINMENT_PENDING，不能发 terminal DatasetDeliveryBlocked / DatasetDeliveryFailed；**进入 CONTAINMENT_PENDING 的 transition transaction 必须同时追加 `DatasetDeliveryContainmentPending`（或固定等价）+ Audit/Evidence + Outbox，事件幂等 identity 绑定 delivery_operation_id + transition/reason/revision，不泄露 credential secret；**
  - containment 确认成功后：fresh gate 不再允许交付 → BLOCKED；fresh gate 仍 ALLOWED 但 credential/issuance contract 无法满足 → FAILED；
- **每一次真实 provider invocation（initial / retry / reconciliation query / revoke / compensation 等，只要实际调用外部 provider）在调用前必须先持久化稳定 physical provider-attempt identity。** 调用成功、显式失败、timeout/unknown outcome 都必须按该 attempt 记录实际 CostEvent；amount 未知时至少记录真实 invocation quantity/unit，后续若 provider 返回收费金额可通过可审计 adjustment/aggregation 补充，但不能因为 terminal outcome 不是 ISSUED 就漏记。same-attempt replay 且没有再次调用 provider 时去重；再次真实调用 provider 必须新 attempt identity；
- terminal DeliveryOperation + Audit/Evidence + Outbox 在后续 DB transaction 内一致提交；terminal business outcome/event 的唯一性与 provider-attempt CostEvent 独立，失败/unknown/containment 路径同样保留已发生 provider attempts 的成本事实；
- provider 成功但 terminal commit 失败时，retry/reconciliation 使用同一 provider_request_key；
- provider 首次返回或 reconciliation 恢复 credential 后，进入 ISSUED 前必须验证**实际 provider capability 是 requested/current-gate context 的等价或更窄集合**：expiry <= fresh cap，resource/DatasetVersion、action/permission、object/row/prefix scope、delivery channel 不得放宽；**consumer/grantee 是必需安全边界，不存在“provider 不表达就跳过”的例外**。direct bearer/presigned capability 必须由 provider 本身或一个可验证的 consumer-binding mechanism 强制绑定 effective consumer/grantee；若 provider capability 无法表达/验证/强制该边界，则不得直接 ISSUED，必须改用 platform redemption/gateway 等在 redemption 时重新认证并绑定 consumer 的 indirection，或将该 direct mode 标为 unsupported；仅命中旧 provider_request_key 不代表 credential 仍满足当前边界；
- recovered credential 超过 fresh cap 时，必须安全 shorten 并 read-after-write 验证，或 revoke/contain；无法确认 containment 时进入 CONTAINMENT_PENDING，**同样必须执行上述 `DatasetDeliveryContainmentPending` + Audit/Evidence/Outbox 原子 transition event 规则**；containment 成功但无法满足 cap 时当前 operation 终结为 FAILED，后续如需重试必须新建显式 delivery attempt 并重新 gate；
- actual provider expiry、consumer/grantee enforcement、**受约束的 delivery channel/mode enforcement** 或其它关键 capability boundary 无法通过 read-after-write/authoritative lookup（或等价可验证机制）确认时，direct bearer / presigned 不得 ISSUED；必须使用能在 redemption/use 时强制这些边界的 platform gateway/indirection，或将 direct mode 标记 unsupported；
- direct bearer provider 必须支持 same-credential replay/read-after-write（或等价同一访问能力恢复），并支持 fresh replay authorization 失败时 revoke/contain 该既有 capability；仅能恢复但不能 containment，或仅有 revoke/compensation 但无法恢复原 bearer secret，都必须使用 platform redemption indirection；
- terminal ISSUED credential 的 same-idempotency-key replay 是一次新的**credential replay authorization**，不是旧 terminal fact 的机械回放：再次返回 credential/handle 前必须 fresh authenticated principal→effective consumer/delegation resolution，获取 shared delivery authorization fence/revision，重新执行 CurrentDeliveryGate + fresh cap，并 authoritative-verify 恢复出的同一 capability 仍满足当前 consumer/resource/action/scope/channel/expiry；
- replay authorization 必须 append 一个不含 secret 的 replay decision fact/audit（replay attempt identity、caller/effective consumer、fence/gate revision、ALLOWED/BLOCKED/CONTAINMENT_PENDING、capability ref/hash）；只有 ALLOWED decision commit 后才可再次暴露同一 credential/handle；
- replay fresh gate/caller authority 已 BLOCKED 或 capability 不再满足当前边界时，same-key retry 必须返回 0 credential secret，并 revoke/contain 原 capability；containment confirmed 后返回稳定 non-secret replay-blocked 结果，未确认时返回 containment-pending 结果。**replay decision 进入 CONTAINMENT_PENDING 时必须追加独立 `DatasetCredentialReplayContainmentPending`（或固定等价/统一 containment event）+ Audit/Evidence/Outbox，subject 指向 replay attempt，不改写原 DeliveryOperation。** 原 DeliveryOperation 的 terminal ISSUED 历史事实不得改写。
- provider 若既不具备可恢复幂等能力，也不能安全补偿，则该 direct bearer mode 在第一阶段 unsupported。

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
- AuthorizationProvenanceBindingInvalidated
- AuthorizationProvenanceBindingSuperseded
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
- gate/fence 通过并提交该 DeliveryOperation 的**授权/访问能力释放线性化点** → `DatasetDeliveryIssued`。对 provider/credential mode，这表示 terminal fact 已提交并允许返回已验证 capability；对 direct-data，这表示服务端从该 commit 之后才被允许开始写第一字节，**不表示客户端已收到任何或全部 bytes，也不表示传输完成**；
- CurrentDeliveryGate fail closed、没有签发任何可用访问能力 → `DatasetDeliveryBlocked`；
- gate 通过但在 terminal authorization/release point 之前无法完成必要 issuance contract → `DatasetDeliveryFailed`。direct-data 在 `DatasetDeliveryIssued` 之后发生 socket loss/stream interruption 不回写 terminal outcome 为 FAILED；如业务需要观测传输完成/中断，使用独立 append-only transfer observation（例如 `DatasetDeliveryTransferObserved` / bytes_sent / completed=false/true），不得改变 `Issued` 的授权线性化语义。

DeliveryOperation **terminal database fact** + Audit/Evidence + Outbox 必须在同一数据库事务内保持一致，并具备幂等语义；这里不包含外部 provider side effect。CostEvent 按实际 activity-attempt 记账：same-attempt replay 去重，retry/reconciliation 若真实新增可计费 provider/compute 工作则追加 attempt 成本（或原子聚合新增 quantity/amount）。外部 issuance 依赖 stable provider_request_key + retry/reconciliation/compensation 协议。事件 payload 不得包含可用 token/credential secret。

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
