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
- Authorization / AuthorizationProvenanceBinding / RightsSnapshot
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
- CertificationProfile snapshot（#134 起）
- DatasetCertification / CertificationDisposition（#134 起）

ProductRelease 特例：DRAFT / VALIDATING / READY 等发布前阶段允许显式 Command 按状态机更新 status 与 validation bindings；进入 PUBLISHED 后，当前数据库 guard 阻止任何 UPDATE，published row 整体冻结。SUSPENDED / WITHDRAWN 目前只是 schema 枚举中的保留状态，不得声称已有 PUBLISHED → SUSPENDED/WITHDRAWN live transition；未来启用需要独立 migration + Command。

修正错误时不得覆盖历史事实，但要按事实类型追加：

- 数据内容、schema/content identity 或实际生产输出变化 → 新 DatasetVersion；
- 数据内容未变化，仅 Quality 评测错误 → 新 QualityAssessment；
- 权利声明/验证错误 → 新 RightsDeclaration / verification fact / RightsSnapshot（按实际语义）；
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

衍生数据的 Effective Rights 默认 fail closed：任何必要输入不允许某个动作时，输出不得自动获得该动作。

## 5. State Transitions

禁止通过通用 PATCH status 修改关键业务状态。

关键状态变化必须使用显式 Command，例如：

- ApproveAuthorization
- VerifyRightsDeclaration
- InvalidateRightsDeclaration / SupersedeRightsDeclaration
- BindAuthorizationProvenance
- InvalidateDatasetVersion
- RunQualityAssessment
- CertifyDatasetVersion
- RevokeDatasetCertification / SupersedeDatasetCertification
- ValidateProductRelease
- PublishProductRelease

状态迁移和认证判定规则必须由 Domain / Application 边界控制。

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
- CurrentCertificationGate：delivery 必须绑定明确 certification；其 decision 必须为 CERTIFIED，且在 as_of 时点未被 CertificationDisposition REVOKED / SUPERSEDED；requested purpose/action/consumer/delivery context 必须被该 certification 冻结的 CertificationProfile snapshot 覆盖；禁止用 latest created_at 猜当前认证；
- CurrentEntitlementGate：使用当前时间、consumer、purpose、action 检查每一个候选 RightsDeclaration 的 VERIFIED 状态、其自身 validity window 与 scope，并排除已生效 INVALIDATED/SUPERSEDED 的 provenance；同时检查 Authorization 状态/有效期与 Effective Rights。

任一子门禁失败都必须 fail closed，即使历史 Certification 仍为 CERTIFIED。

CurrentDeliveryGate query 只用于展示/预检，不构成交付授权。任何返回数据、下载链接、presigned URL、token 或访问凭证的 server-side delivery Command 都必须在 issuance 前重新执行完整 CurrentDeliveryGate；不得信任客户端缓存的旧 gate result。gate 与 issuance 必须处于同一 Application Command 受控边界。

DeliveryOperation 每个终态都必须产生明确 Domain Event：Issued / Blocked / Failed（事件名由实现固定但语义不得缺失），并与 Audit/Evidence/Outbox、CostEvent（如有）保持一致幂等边界。任何事件或审计 payload 不得包含可用 credential secret。

外部 credential issuance 不能假装与 PostgreSQL 同事务。必须先持久化 DeliveryOperation + stable provider_request_key，再执行外部副作用；provider 必须支持幂等重放/read-after-write 或 revoke/compensation。若 provider 不具备这些能力，则第一阶段只能通过平台 redemption indirection 暴露访问，不得直接签发不可恢复的 bearer credential。ISSUANCE_PENDING 必须可 reconciliation，provider 成功但 DB terminal commit 失败时不得因 retry 产生第二份独立 credential。

若签发 URL/token/credential，`expires_at` 不得晚于 requested TTL、平台最大 TTL、本次 entitlement 链上最早的 RightsDeclaration / Authorization 有效期边界，以及签发时已存在且未来生效的 RightsDisposition / CertificationDisposition 最早 effective_at。支持 redemption-time server check 的 delivery mode 应在 redemption 时再次执行 CurrentDeliveryGate；不可回调的 bearer/presigned credential 必须使用 expiry cap + 明确最大 TTL。

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
