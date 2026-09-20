# 数据库设计 V1.1

目标数据库：PostgreSQL 16+。

> 本文同时描述已实现核心表与 #129 Certified Dataset Pilot 已批准的目标逻辑模型。具体迁移以各子 Issue PR 为准。

## 1. 通用约定

- 主键：UUID
- 时间：timestamptz
- 扩展字段：JSONB
- 可变聚合可使用 revision bigint
- 仅对可变业务主对象使用软删除
- 不可变历史事实不得软删除或覆盖
- 破坏历史事实的 migration down 必须 fail closed
- 核心业务关系使用强类型列 / FK，不藏入 JSONB

## 2. 多租户边界

核心业务对象使用 workspace_id 作为组织 / 租户边界。

关键跨表引用应校验同一 workspace，而不是仅依赖单列 FK 存在性。

## 3. 核心表族

### 已有核心

~~~text
data_resource
resource_binding
governance_projection
dataset
dataset_version
dataset_version_lineage

entity_type
entity
entity_mapping
entity_mapping_decision
entity_match_job

workflow
workflow_version
execution
execution_input
execution_dependency_preparation
execution_dependency_binding
execution_mapping_usage

data_authorization
authorization_resource
rights_snapshot

quality_result        # QualityAssessment compatibility storage; formalized by 000019
quality_finding
compliance_result
compliance_finding

data_contract
contract_version

data_product
product_version
product_asset
product_release
product_release_dataset

cost_event
evidence
evidence_relation
evidence_snapshot
audit_event
outbox_event
~~~

### Certified Dataset Pilot 目标逻辑对象

具体表名可由实现确定，但业务事实必须可查询：

~~~text
RightsDeclaration
RightsVerification
RightsDisposition (INVALIDATED / SUPERSEDED)
AuthorizationProvenanceBinding
EffectiveRights / EffectiveRightsSnapshot

CertificationProfile snapshot
DatasetCertification
CertificationDisposition (REVOKED / SUPERSEDED)

DeliveryOperation

CostEvent activity identity extension
CostAllocation
~~~

QualityAssessment 核心已通过 #140 / migration 000019 落地，继续复用 `quality_result` / `quality_finding` 作为兼容存储名；后续 #132/#133 必须在该已实现模型上扩展，不得再次创建平行 QualityAssessment 表族。

## 4. DataResource

DataResource 是业务资源，不是物理表。

owner_id 仅代表平台资产责任/归属。法律权利来源由 RightsDeclaration / Evidence 表达。

## 5. ResourceBinding / Governance Projection

`resource_binding` 是现有已落库的 adapter binding 模型，用于把 Core `DataResource` 与 OpenMetadata 等外部治理实体解耦。

现有关键字段：

- resource_id
- provider
- entity_type
- external_id
- external_fqn
- binding_metadata
- is_primary
- created_at / updated_at

约束原则：

- `resource_id` 关联 Core DataResource；
- provider / external_id / external_fqn 是外部引用，不对外部系统建立数据库 FK；
- 外部 FQN/ID 不得写回 DataResource 成为 Core 业务主键；
- provider-specific 扩展信息放在 binding_metadata，不污染 Core Domain。

`governance_projection` 记录 Core 对外部治理系统的投影尝试和状态。它是 projection/reconciliation 状态，不拥有 DataResource / Dataset / ProductRelease 等 Core 业务真相。

## 6. Dataset / DatasetVersion

Dataset 是逻辑身份。DatasetVersion 是不可变生产事实。

类型：

- RAW
- STANDARDIZED
- CURATED
- PRODUCT

READY 后数据内容不可被改写。

Certification status 不应塞入 DatasetVersion.status；认证是独立历史事实。

## 7. Production Graph

dataset_version_lineage 记录平台生产血缘。

Execution 的实际生产依赖还包括：

- execution_input
- execution_dependency_binding
- execution_mapping_usage
- entity_resolution_output_decision

这些事实共同回答“这个输出真实消费了什么”。

## 8. Entity

~~~text
EntityType → Entity → EntityMapping projection
                     ↘ EntityMappingDecision history
~~~

历史生产/发布必须引用 immutable decision。

## 9. Rights

### Authorization（已实现）

表达 grantor_ref、grantee_ref、purpose、resource、actions、scope、raw_export_allowed 和 validity。

### AuthorizationProvenanceBinding（#137）

目标强类型关系至少表达：

- workspace_id
- authorization_id
- data_resource_id
- rights_declaration_id
- grantor_ref
- supported_actions / scope（如按 grant 粒度绑定）
- created_at / actor

约束：

- grantor_ref 必须与支持声明中的可授权 party_ref 明确匹配，或显式引用可验证 delegation chain；
- authorization/resource/declaration 必须同 workspace、同 DataResource；
- declaration 支持的 actions/scope 必须覆盖 authorization 授出的范围；
- 当前 entitlement 查询必须读取 binding，不允许独立选择 declaration + authorization。

### RightsDeclaration（#137）

目标强类型字段至少能表达：

- workspace_id
- data_resource_id
- party / claimant refs
- rights role
- basis_type / basis_ref
- validity
- allowed actions
- restricted actions
- verification status / fact
- append-only disposition facts (INVALIDATED / SUPERSEDED, effective_at, reason, evidence, actor, optional superseded_by)
- evidence association

JSONB 只用于受控扩展参数，不承载主要权利关系。

### Effective Rights（#137）

对一个 DatasetVersion 计算/冻结实际可用动作和限制。

衍生数据默认 fail closed。

## 10. QualityAssessment（已实现：#140 / migration 000019）

领域语义已经正式落地，存储/API 兼容名仍保留 `quality_result`。

当前已实现的 Assessment 至少持久化：

- workspace_id
- dataset_version_id
- rule_set_ref
- rule_set_version
- rule_set_content_sha256
- rule_set_content
- evaluator_name / evaluator_version
- metrics
- gate_decision
- created_at / actor

`quality_finding` 保存逐规则 finding，并由 000019 补强为 Assessment 创建事务内写入、之后不可追加/UPDATE/DELETE 的历史事实。

000019 还提供：

- rule content + SHA-256 一致性约束；
- 新 Assessment 必须有完整 rule snapshot/evaluator identity；
- legacy pre-019 row 通过 NOT VALID 策略保留兼容历史；
- DatasetVersion assessment history 索引；
- Assessment / finding 不可回溯改写。

现有 HTTP 已提供 assessment by ID、DatasetVersion history/latest 等查询。

后续 #132/#133 的工作重点是六维通用规则执行与 Quality Report，不再重复迁移/重建 QualityAssessment 核心。

大量 failing rows 的进一步规模化存储/分页可以由 #133 按已实现 finding 模型演进；不得通过新平行 Assessment root 规避现有历史。

## 11. DatasetCertification（#134）

核心强类型关系至少包括：

- workspace_id
- dataset_version_id
- quality_assessment_id
- certification_profile snapshot/ref/version/hash
- rights_snapshot/effective rights ref
- compliance_result_id（如 required）
- contract_version_id（如 required）
- evidence_snapshot_id 或等价冻结证明
- decision
- blockers / reason
- issued_at / actor

Certification 创建后不可被 UPDATE 成另一种业务含义。

第一阶段还需要 append-only CertificationDisposition，至少表达：

- certification_id
- disposition: REVOKED / SUPERSEDED
- effective_at
- reason
- superseded_by_certification_id（SUPERSEDED 时）
- Evidence / actor

Current certification 查询必须按 disposition + as_of 判断，不得用 created_at/latest 隐式选择。

## 12. DataProduct / ProductVersion / ProductRelease

ProductRelease 精确引用发布时所需 DatasetVersion、Contract、Rights、Quality、Compliance、Evidence。

ProductRelease 与 DatasetCertification 不应合并成同一表或同一 status。

## 13. DeliveryOperation（#135）

第一阶段 standalone delivery 必须持久化稳定的 DeliveryOperation，作为 delivery command、Audit/Evidence 和 CostAllocation 的强类型业务主体；不能让一次交付只存在于临时 HTTP 请求或 JSONB metadata 中。

至少逻辑表达：

- id
- workspace_id
- dataset_version_id
- dataset_certification_id
- consumer_ref
- purpose
- action
- delivery_mode
- idempotency_key / request identity
- requested_at
- gate decision / blockers
- status：PREPARED / ISSUANCE_PENDING / ISSUED / BLOCKED / FAILED（或等价受控状态）
- provider_request_key（稳定幂等键）
- provider_credential_ref/hash（如适用；禁止存可用 secret）
- issuance result
- credential_expires_at（如签发 credential）
- actor / trace

实现可选择 append-only attempt/result 模型或受控 lifecycle row，但必须满足：

- 每次 delivery Command 有稳定 ID；
- 同一幂等请求不会重复签发或重复记账；
- 外部 issuance 前必须先 durable persist PREPARED/ISSUANCE_PENDING；
- provider_request_key 对同一 DeliveryOperation 稳定，支持 crash 后安全 retry/reconcile；
- gate 失败也有可审计 DeliveryOperation / result；
- 不把可用 credential secret/token 正文持久化到 Core 数据库；
- ISSUANCE_PENDING 必须有 reconciliation 查询/索引，不能永久悬空；
- CostAllocation 必须能以 FK 关联 DeliveryOperation。

## 14. CostEvent / CostAllocation

当前 `cost_event` 已落库，现有强类型关联只有可选 `execution_id`。这足以表达 Execution 成本，但不足以表达 QualityAssessment、Rights verification/disposition、Certification、Delivery 等没有 Execution 的活动。

Certified Dataset Pilot 目标模型增加：

### CostEvent activity identity

`cost_event` 需要稳定的 activity/idempotency identity，至少逻辑表达：

- workspace_id
- activity_id（或等价稳定 operation identity）
- component_key / cost_type
- quantity / unit
- amount / currency
- pricing_mode
- occurred_at

同一业务活动的幂等重放必须复用同一 activity identity。建议数据库唯一约束至少覆盖：

~~~text
(workspace_id, activity_id, component_key)
~~~

一个业务活动可以有多个不同 component_key（例如 ENGINE_INVOCATION、HUMAN_REVIEW、DELIVERY），但同一 component 不得因重试重复记账。

### CostAllocation

非 Execution 成本不得仅把 subject IDs 塞入 JSONB metadata。

使用强类型 `CostAllocation`（具体表名可由实现确定）把 CostEvent 关联到实际业务主体。V1 至少支持：

- cost_event_id
- execution_id（兼容现有）
- quality_assessment_id
- rights_declaration_id
- rights_verification_id
- rights_disposition_id
- authorization_provenance_binding_id
- dataset_certification_id
- certification_disposition_id
- delivery_operation_id（第一阶段必需；#135 必须落库 DeliveryOperation）

实现可用一张带 nullable typed FK 的 allocation 表并用 CHECK 保证每条 allocation 仅选择一个 subject，或用等价强类型表族；不得退化为 `subject_type + subject_id` 无 FK 多态字符串，也不得只依赖 metadata。

现有 `cost_event.execution_id` 可继续用于兼容查询；新增非 Execution 成本必须通过 typed allocation 查询到业务主体。

## 15. Evidence / Audit

Evidence 保存可验证证据元数据和可选 artifact/hash。

EvidenceRelation 关联业务对象；EvidenceSnapshot 在需要冻结时保存 manifest。

AuditEvent 记录“谁做了什么”，不是 Evidence 的替代品。

## 16. Mutable vs Immutable

| 对象 | 语义 |
|---|---|
| DataResource owner / lifecycle | mutable aggregate / projection |
| EntityMapping current row | mutable projection |
| Authorization state | explicit state machine |
| DatasetVersion | immutable content fact after READY |
| EntityMappingDecision | immutable history |
| Execution | stateful lifecycle row; transitions update status/output/metrics/timestamps through explicit commands; terminal rows are retained and not deleted |
| Execution dependency facts | immutable history |
| RightsSnapshot | immutable |
| QualityAssessment | immutable |
| verified RightsDeclaration / verification / disposition facts | immutable |
| CertificationProfile snapshot | immutable |
| DatasetCertification / CertificationDisposition | immutable |
| DeliveryOperation | persisted delivery attempt/result with stable idempotency identity; gate/issuance transitions only through delivery command |
| CostEvent / CostAllocation | immutable accounting/history facts |
| ProductVersion | immutable history |
| ProductRelease | stateful lifecycle row before publication; explicit validation/publish transitions may update status and frozen references; after publication, release bindings are frozen and terminal history is retained |

## 17. JSONB 使用策略

JSONB 可用于：

- 引擎元数据
- runtime metrics
- schema / manifests
- 行业扩展属性
- finding diagnostic metadata
- 受控 rights/certification parameters

JSONB 不用于 ID/FK、状态、版本号、核心 party/resource/certification 关系或需要约束的字段。

## 18. 删除策略

允许软删除的可变主对象可以包括 UseCase、DataResource、Dataset、DataProduct、Entity。

Execution 行在生命周期内会通过显式状态迁移更新 status、engine/output、metrics、errors 与 timestamps，因此不能把整行视为内容不可变；但 Execution 历史必须保留，终态记录不得删除。真正不可变的是其已冻结的 input/dependency/mapping-usage 等生产事实。

ProductRelease 不是“从创建起整行不可变”：在 DRAFT/VALIDATING/READY 等发布前生命周期内，显式 Command 可以更新 status 以及 validation 绑定；进入 PUBLISHED 后，当前 `guard_product_release_history` 拒绝所有 UPDATE，整行作为发布历史冻结。SUSPENDED/WITHDRAWN 虽是 schema 枚举值，但当前不构成可达 live transition；未来启用必须先调整 guard 并新增显式 Command。

不可变事实不得软删除或覆盖，包括 DatasetVersion、MappingDecision、execution dependency facts、ProductVersion、EvidenceSnapshot、RightsSnapshot、QualityAssessment、verified RightsDeclaration/verification/disposition facts、DatasetCertification、CertificationDisposition、AuditEvent、CostEvent、CostAllocation。
