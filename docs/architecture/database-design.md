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

quality_result
compliance_result

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
QualityAssessment
  - rule snapshot/hash
  - dimension summaries
  - findings

RightsDeclaration
RightsVerification
RightsDisposition (INVALIDATED / SUPERSEDED)
AuthorizationProvenanceBinding
EffectiveRights / EffectiveRightsSnapshot

CertificationProfile snapshot
DatasetCertification
CertificationDisposition (REVOKED / SUPERSEDED)
~~~

优先演进现有 quality_result，不得无理由复制一套平行 Quality 表族。

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

## 10. QualityAssessment（#131）

优先扩展现有 quality_result，至少持久化：

- workspace_id
- dataset_version_id
- rule_set_ref
- rule_set_version
- rule_set_content_sha256
- immutable rule content/snapshot or content-addressed ref
- evaluator identity/version
- metrics / dimension summaries
- findings
- gate decision
- created_at / actor

完成的 Assessment 是不可变事实。

大量 failing rows 不应全部塞入单个 JSONB；应使用分页 finding、artifact 或适合的数据结构。

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

## 13. Evidence / Audit

Evidence 保存可验证证据元数据和可选 artifact/hash。

EvidenceRelation 关联业务对象；EvidenceSnapshot 在需要冻结时保存 manifest。

AuditEvent 记录“谁做了什么”，不是 Evidence 的替代品。

## 14. Mutable vs Immutable

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
| ProductVersion | immutable history |
| ProductRelease | stateful lifecycle row before publication; explicit validation/publish transitions may update status and frozen references; after publication, release bindings are frozen and terminal history is retained |

## 15. JSONB 使用策略

JSONB 可用于：

- 引擎元数据
- runtime metrics
- schema / manifests
- 行业扩展属性
- finding diagnostic metadata
- 受控 rights/certification parameters

JSONB 不用于 ID/FK、状态、版本号、核心 party/resource/certification 关系或需要约束的字段。

## 16. 删除策略

允许软删除的可变主对象可以包括 UseCase、DataResource、Dataset、DataProduct、Entity。

Execution 行在生命周期内会通过显式状态迁移更新 status、engine/output、metrics、errors 与 timestamps，因此不能把整行视为内容不可变；但 Execution 历史必须保留，终态记录不得删除。真正不可变的是其已冻结的 input/dependency/mapping-usage 等生产事实。

ProductRelease 不是“从创建起整行不可变”：在 DRAFT/VALIDATING/READY 等发布前生命周期内，显式 Command 可以更新 status 以及 validation 绑定；进入 PUBLISHED 后，DatasetVersion、Rights、Quality、Compliance、Contract、EvidenceSnapshot 等发布绑定必须冻结，后续仅允许受状态机约束的生命周期动作（如 SUSPENDED/WITHDRAWN），且历史记录不得删除。

不可变事实不得软删除或覆盖，包括 DatasetVersion、MappingDecision、execution dependency facts、ProductVersion、EvidenceSnapshot、RightsSnapshot、QualityAssessment、verified RightsDeclaration/verification/disposition facts、DatasetCertification、CertificationDisposition、AuditEvent、CostEvent。
