# AGENTS.md

本文件约束所有 AI Coding Agent、Codex 线程及人工开发者在本仓库中的实现边界。

## 1. Core Domain Independence

Core Domain 不得直接依赖以下具体产品或 SDK：

- OpenMetadata
- Apache Hop
- Splink
- Soda / Great Expectations
- Presidio
- Label Studio / X-AnyLabeling
- MinIO / S3 SDK

外部能力必须通过 Port / SPI + Adapter 接入。

## 2. System of Record

Data Product Platform 自身数据库是以下核心业务事实的 System of Record：

- UseCase
- DataResource
- Dataset / DatasetVersion
- Entity / EntityMapping / immutable mapping decisions
- Authorization / AuthorizationProvenanceBinding / AuthorizationProvenanceBindingDisposition / RightsSnapshot
- RightsDeclaration / rights verification facts（#137 起）
- Workflow / Execution / frozen execution dependencies
- QualityAssessment / Quality findings（已通过 #140 / migration 000019 落地）
- DataContract
- CertificationProfile snapshot / DatasetCertification（#134 起）
- DeliveryOperation（#135 起）
- DataProduct / ProductVersion / ProductRelease
- CostEvent / CostAllocation
- Evidence / EvidenceSnapshot / AuditEvent

OpenMetadata 仅作为 Governance Projection。

外部质量、处理、实体解析或标注系统均不得成为上述核心业务状态的最终真相。

## 3. Immutable Objects and Historical Facts

以下对象创建并进入冻结状态后，不得原地修改业务含义：

- DatasetVersion
- EntityMappingDecision
- Execution dependency preparation / binding / mapping usage
- ProductVersion
- ProductRelease published bindings / published release history
- ContractVersion
- WorkflowVersion
- EvidenceSnapshot
- RightsSnapshot
- QualityAssessment（已实现；兼容存储名 quality_result / quality_finding）
- verified RightsDeclaration / verification fact（#137 起）
- AuthorizationProvenanceBinding / BindingDisposition（#137 起）
- CertificationProfile snapshot（#134 起）
- DatasetCertification / CertificationDisposition（#134 起）

ProductRelease 特例：DRAFT / VALIDATING / READY 等发布前阶段允许显式 Command 按状态机更新 status 与 validation bindings；进入 PUBLISHED 后，当前数据库 guard 阻止任何 UPDATE，published row 整体冻结。SUSPENDED / WITHDRAWN 目前只是 schema 枚举中的保留状态，不得声称已有 PUBLISHED → SUSPENDED/WITHDRAWN live transition；未来启用需要独立 migration + Command。

DeliveryOperation 也是受控 lifecycle row，不得把整行视为创建即 immutable：PREPARED / ISSUANCE_PENDING / CONTAINMENT_PENDING / terminal 状态需要由显式 delivery/reconciliation Command 更新。必须冻结并保护的是 request/idempotency identity、确定后的 provider_request_key、已记录的 transition/gate/issuance history 与 terminal outcome 语义；不要安装会阻止合法恢复迁移的全行 UPDATE guard。

修正错误时不得覆盖历史事实，但要按事实类型追加：

- 数据内容、schema/content identity 或实际生产输出变化 → 新 DatasetVersion；
- 数据内容未变化，仅 Quality 评测错误 → 新 QualityAssessment；
- 权利声明/验证错误 → 同一 RightsDeclaration 的 terminal verification outcome 不可翻转；错误 VERIFIED 先用 RightsDisposition INVALIDATED/SUPERSEDED 退出 current set，再创建新 RightsDeclaration + verification / RightsSnapshot（按实际语义）；
- CertificationProfile 规则变化 → 新 Profile version/snapshot；
- 认证判断错误或重新认证 → 新 DatasetCertification；旧认证退出 current set 时追加 CertificationDisposition（REVOKED / SUPERSEDED），不 UPDATE 旧认证，也不按 latest timestamp 猜当前认证；
- Product 发布事实变化 → 新 ProductVersion / ProductRelease 或显式生命周期 Command。

不得为了修正非内容事实而无意义地创建新的 DatasetVersion。

历史事实对象不得依赖软删除来模拟修正。

## 4. Ownership and Rights Semantics

data_resource.owner_id、dataset.owner_id 只表示平台内资产责任/归属，不自动等同现实世界法律所有权。

权利模型必须区分：

~~~text
RightsDeclaration
→ AuthorizationProvenanceBinding
→ Authorization
→ RightsSnapshot
→ Effective Rights
→ DatasetCertification
~~~

平台记录权利声明、依据、主体角色、允许项、限制项和 Evidence；不得声称平台自动裁定现实世界法律所有权。

Authorization 不能与 provenance 独立选择。每个进入 CurrentEntitlementGate 的 Authorization / ResourceGrant 都必须通过强类型 AuthorizationProvenanceBinding 证明其 grantor_ref 得到相应 VERIFIED RightsDeclaration 支持；grantor 不匹配或无可验证 delegation chain 时 fail closed。

RightsDeclaration 的 resource / consumer applicability / purpose / action / scope / validity 必须强类型、可索引、可查询；这些 gate-critical 字段不得仅藏在 JSONB。

Authorization 本身也必须逐项覆盖当前 requested context：grantee/consumer、resource、purpose、action、scope、validity/status。RightsDeclaration 或 Effective Rights 的更宽范围不得放大一条更窄的 Authorization。

RightsSnapshot 的 immutable 语义覆盖 header + authorization/declaration/provenance-binding membership；finalize 后 membership INSERT/UPDATE/DELETE 必须由数据库 guard fail closed。

AuthorizationProvenanceBinding 创建后不可 UPDATE/DELETE。错误 binding 通过 append-only BindingDisposition（INVALIDATED / SUPERSEDED + effective_at）退出 current set；CurrentEntitlementGate 必须按 as_of 排除已生效 disposition。replacement binding 必须独立重新校验，历史 RightsSnapshot 继续引用旧 binding。

衍生数据的 Effective Rights 默认 fail closed：任何必要输入不允许某个动作时，输出不得自动获得该动作。

## 5. State Transitions

禁止通过通用 PATCH status 修改关键业务状态。

关键状态变化必须使用显式 Command，例如：

- ApproveAuthorization
- VerifyRightsDeclaration
- InvalidateRightsDeclaration / SupersedeRightsDeclaration
- BindAuthorizationProvenance
- InvalidateAuthorizationProvenanceBinding / SupersedeAuthorizationProvenanceBinding
- InvalidateDatasetVersion
- RunQualityAssessment
- CertifyDatasetVersion
- RevokeDatasetCertification / SupersedeDatasetCertification
- ValidateProductRelease
- PublishProductRelease

状态迁移和认证判定规则必须由 Domain / Application 边界控制。

DataProduct 当前公开 API 的 live lifecycle 只应视为 `DRAFT → PUBLISHED`（由成功 PublishProductRelease 的受控副作用触发）。DESIGNING / DEVELOPING / TESTING / READY / ACTIVE / SUSPENDED / DEPRECATED / RETIRED 目前只是 domain/schema reserved values；在新增显式 Command/API 前，不得把这些 enum 当作已实现可达状态，也不得用 generic PATCH 补出迁移。

## 6. Domain Events and Audit

关键状态变化必须：

1. 产生 Domain Event；
2. 产生或触发 AuditEvent；
3. 需要异步副作用时写入 Transactional Outbox。

不得把普通 application log 当作 AuditEvent。

## 7. Cost and Evidence First

Cost 与 Evidence 是一等业务对象，不允许项目结束后再补录。

生产、人工审核、质量评测、Rights verification、Certification、Delivery、外部服务、Release 等关键活动应按业务需要产生：

- CostEvent
- Evidence
- AuditEvent

非 Execution CostEvent 必须有稳定 activity/idempotency identity，并通过强类型 CostAllocation 关联到 QualityAssessment、Rights verification/disposition、AuthorizationProvenanceBinding、DatasetCertification/Disposition、DeliveryOperation 等实际业务主体。禁止仅把 subject IDs 塞入 JSONB metadata。

## 8. Industry Pack Boundary

园区、制造、医疗、政务等行业逻辑不得硬编码进 Core Domain。

行业能力应通过 industry-packs/<industry>/ 提供：

- Entity Types
- Glossary
- Standardization Rules
- Matching Policies
- Indicators
- Quality Rules
- Compliance Rules
- Certification Profiles
- Product Templates

禁止在核心代码中散落 if industry == "PARK" 逻辑。

## 9. Vertical Slice First

优先完成端到端业务切片，不优先“做完整某一技术层”。

已完成 POC 切片：

~~~text
CSV
→ DataResource
→ RAW DatasetVersion
→ Entity Resolution
→ STANDARDIZED DatasetVersion
→ Workflow
→ CURATED DatasetVersion
→ ProductRelease
→ Evidence
~~~

当前 Certified Dataset Pilot 切片：

~~~text
DataResource
→ Rights Provenance
→ DatasetVersion
→ Processing / Entity Resolution
→ QualityAssessment
→ Effective Rights / Compliance / Contract
→ DatasetCertification
→ Certified DatasetVersion
~~~

## 10. Quality and Certification

QualityAssessment 回答“数据质量如何”；DatasetCertification 回答“该 DatasetVersion 是否满足某个 CertificationProfile”。

不得把“评测完成”自动等同于“认证通过”。

V1 不强制全行业统一总分。Critical rule、Rights、Compliance、Contract 或 Traceability 任何 required 条件缺失时，Certification 必须 fail closed。

Certified Dataset 是可独立交付成果，不要求必须包装成 DataProduct；但 DatasetCertification 只证明认证时点结论，不授予永久交付资格。

每次 standalone delivery 必须执行 CurrentDeliveryGate：

- DatasetVersionUsability：至少拒绝 INVALID / FAILED / PROCESSING / CREATED；SUPERSEDED 是否允许按明确历史版本交付遵循现有领域语义和实现验收；
- CurrentCertificationGate：delivery 必须绑定明确 certification；其 decision 必须为 CERTIFIED，且在 as_of 时点未被 CertificationDisposition REVOKED / SUPERSEDED；frozen CertificationProfile 对 requested purpose/action/consumer/delivery channel/mode 每个维度都必须显式 ANY / EXPLICIT 覆盖。缺失、NULL、UNKNOWN 不等于 ANY，必须 fail closed；禁止用 latest created_at 猜当前认证；
- CurrentEntitlementGate：使用当前时间、consumer、purpose、action 检查每一个候选 RightsDeclaration 的 VERIFIED 状态、其自身 validity window 与 scope，并排除已生效 INVALIDATED/SUPERSEDED 的 provenance；同时要求 AuthorizationProvenanceBinding 当前有效且未被 BindingDisposition INVALIDATED/SUPERSEDED；最后逐项校验 Authorization 的 grantee/consumer、resource、purpose、action、scope、状态/有效期，再检查 Effective Rights。

任一子门禁失败都必须 fail closed，即使历史 Certification 仍为 CERTIFIED。

CurrentDeliveryGate query 只用于展示/预检，不构成交付授权。任何返回数据、下载链接、presigned URL、token 或访问凭证的 server-side delivery Command 都必须在 delivery 前重新执行完整 CurrentDeliveryGate；不得信任客户端缓存的旧 gate result。所有 delivery mode 都必须在第一个外部可观察交付副作用前完成共享 delivery authorization fence/revision 下的 terminal finalize。direct-data 必须先提交 ISSUED，再允许写 response 第一字节；commit 前必须 0 bytes。

Delivery Command 的 `consumer` 不得直接信任请求字段。进入 CurrentDeliveryGate 前必须从已认证 caller principal 解析其允许代表的 effective consumer/workspace；on-behalf-of 必须有服务端验证的显式 delegation，并把 principal/delegation/effective consumer 写入 Audit/Evidence。demo actor ID、任意 header/body consumer ID 都不是认证。第一阶段可以不做完整 IAM，但没有最小可信 principal→consumer 边界的 HTTP 路径不得执行真实 delivery。

Direct-data 的 terminal `ISSUED` 只证明该次交付授权已在线性化点提交，不证明客户端收到全部 bytes。若 ISSUED commit 后响应丢失/进程崩溃，同一 idempotency key 不得依据旧 gate 再次发数据；只返回稳定 non-payload replay-required 结果。需要再次取数时创建新的显式 DeliveryOperation/attempt（可关联 retry_of），重新解析 principal→consumer、重新 CurrentDeliveryGate、重新走 fence；期间任何 revocation/invalidation 必须使新 attempt fail closed。

DeliveryOperation 每个终态都必须产生明确 Domain Event：Issued / Blocked / Failed（事件名由实现固定但语义不得缺失），并与 Audit/Evidence/Outbox、CostEvent（如有）保持一致幂等边界。任何事件或审计 payload 不得包含可用 credential secret。

外部 credential issuance 不能假装与 PostgreSQL 同事务。必须先持久化 DeliveryOperation + stable provider_request_key，再执行外部副作用；provider 返回/恢复 capability 后，terminal ISSUED transaction 必须使用与所有影响 CurrentDeliveryGate 的 disposition/invalidation Commands 共享的 delivery authorization fence/revision，再次 re-gate + fresh-cap；direct-data 也必须使用同一 fence 在 ISSUED commit 后才能写第一字节。该 commit 是 delivery linearization point。**每次初始/retry/reconciliation issuance 前都必须重新执行 CurrentDeliveryGate 并重新计算 expiry cap**，旧 gate snapshot 仅供审计。fresh gate BLOCKED 时，如 provider_request_key 可能已经产生外部访问能力，必须先 reconcile 并 revoke/contain；只有确认没有活跃访问能力后才能终结 BLOCKED，否则保持 CONTAINMENT_PENDING。CONTAINMENT_PENDING 在 confirmed containment 后也允许终结 FAILED：用于 gate 仍 ALLOWED、但 credential/issuance contract 无法满足（如实际 expiry 超 fresh cap 且无法安全 shorten）的场景。direct bearer mode 还必须支持按同一 provider_request_key 恢复/重放同一 credential（或等价同一访问能力）；只有 revoke/compensation 但不能恢复原 bearer secret 时，必须走 platform redemption indirection，不能把同一幂等 retry 静默签发成第二份 credential。ISSUANCE_PENDING 必须可 reconciliation。

若签发 URL/token/credential，`expires_at` 不得晚于 requested TTL、平台最大 TTL、本次 entitlement 链上最早的 RightsDeclaration / Authorization 有效期边界，以及签发时已存在且未来生效的 RightsDisposition / AuthorizationProvenanceBindingDisposition / CertificationDisposition 最早 effective_at。支持 redemption-time server check 的 delivery mode 应在 redemption 时再次执行 CurrentDeliveryGate；不可回调的 bearer/presigned credential 必须使用 expiry cap + 明确最大 TTL。

任何 provider 首次返回或 reconciliation 恢复出的 credential，在进入 ISSUED 前必须验证实际 capability 是 requested/current-gate context 的等价或更窄集合：expiry <= fresh cap，resource/DatasetVersion、consumer、action、object/row/prefix scope、channel 不得扩大。命中旧 provider_request_key 不能绕过这条检查；超过 fresh cap、scope 过宽或关键维度不可验证时必须 shorten/narrow+verify 或 revoke/contain，无法安全满足当前 context 时不得 ISSUED。

## 11. Release Readiness

任何 Product Release 发布必须经过统一 ReleaseReadiness 判断。

至少检查：

- Production
- Rights
- Quality
- Compliance
- Contract
- Dataset
- Evidence
- Delivery（如适用）

不得绕过 Readiness 直接设置 PUBLISHED。

DatasetCertification 与 ProductRelease 是不同业务事实；不要用一个对象替代另一个。

## 12. External Engines

外部 Engine 的 ID、状态和错误信息不能成为 Core Domain 的业务真相。

例如：

- execution.id 是平台业务 ID；
- engine_execution_id 只是 Hop / Python / Spark 的外部引用。

Engine Adapter 错误需要映射为平台统一错误模型。

## 13. Database Rules

- 核心可查询业务字段使用强类型列；
- Engine 扩展信息、行业扩展属性和受控参数可使用 JSONB；
- 不要把核心主外键、状态、版本号、权利关系藏入 JSONB；
- 历史事实对象不得软删除或覆盖；
- 对外部系统 ID 不建立数据库 FK；
- 破坏不可变历史的 migration down 必须 fail closed；
- 先检查再 DDL 的 destructive down 要考虑并发业务写入的锁语义。

## 14. Definition of Done

核心业务 Issue 至少考虑：

- Domain Rule
- Migration（如涉及持久化）
- Repository
- Application Command / Query
- HTTP API（如适用）
- Domain Event
- AuditEvent
- Evidence（关键动作）
- Idempotency（关键 Command）
- Workspace / ownership boundary
- Authenticated principal → effective consumer / delegation boundary（涉及交付时）
- Historical immutability
- Current entitlement / delivery gate（涉及交付时）
- Tests

关键业务动作不得仅实现为 CRUD。

## 15. Testing

优先级：

1. Domain tests
2. Application tests
3. Adapter contract tests
4. Real PostgreSQL integration tests（持久化/并发/约束）
5. Vertical slice integration tests
6. Browser / live-core acceptance（用户路径）

外部 Engine Adapter 应尽量复用统一 Contract Test。

## 16. Current Stage and Scope Discipline

核心 POC 已完成。当前阶段是 #129 Certified Dataset 受控试点。QualityAssessment 核心已由 #140 落地；#131 仅保留 review 后新增的 CostAllocation 等 follow-up。

第一阶段主任务：

- #131 QualityAssessment follow-up / CostAllocation
- #132 Quality Engine
- #133 Quality Report
- #137 Data Rights Provenance
- #134 DatasetCertification
- #135 API / UI
- #136 E2E Pilot

除非对应 Issue 明确要求，第一阶段不要主动加入：

- T4/T5/T6 全套生产可靠性路线作为前置
- 完整 Billing / Settlement / ERP
- 自建区块链平台
- 完整 IAM / 灾备 / 大规模性能平台
- 复杂 Service Mesh / 过早微服务拆分
- AI 黑盒风控模型
- 高敏门禁 / 人脸 / 视频数据
- Label Studio / X-AnyLabeling / Gold Dataset 第二阶段实现
- 完整法律合同管理或自动法律推理

文档基线见 docs/product/certified-dataset-pilot.md、docs/architecture/certified-dataset.md、docs/architecture/data-rights-provenance.md。
