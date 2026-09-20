# 核心领域模型 V1.1

> V1.1 增加 Certified Dataset 与 Data Rights Provenance 目标模型。QualityAssessment 核心已通过 #140 / migration 000019 落地；#134/#137/#135 新对象在对应 Issue 实现前属于已批准目标模型。

## 1. 主业务链

~~~text
Workspace
  ↓
Project / UseCase
  ↓
DataResource
  ↓
RightsDeclaration (#137)
  ↓
AuthorizationProvenanceBinding (#137)
  ↓
Authorization
  ↓
Dataset
          ↓
    DatasetVersion
          │
          ├── Entity Resolution
          ├── Workflow / Execution
          ├── Lineage
          ├── QualityAssessment (implemented via #140)
          ├── Compliance
          ├── EffectiveRights (#137)
          └── DatasetCertification (#134)
                    ↓
             Certified DatasetVersion
                    │
                    ├── DataProduct / ProductRelease
                    ├── Trusted Data Offering
                    └── AI Dataset（后续）
~~~

横向能力：Cost · Evidence · Audit · Version · Rights · Quality。

## 2. 重要区分

### DataResource

回答“业务数据资源是什么、由谁提供/管理、权利来源是什么”。

DataResource 不等于物理 Table。

owner_id 表示平台资产责任/归属，不自动等于法律 RIGHTS_HOLDER。

### Dataset

平台中的逻辑数据集身份。

### DatasetVersion

某一次生产真实产生的不可变数据事实。

READY 只表示内容已冻结，不表示 Quality 或 Certification 通过。

### QualityAssessment

针对一个明确 DatasetVersion 和明确规则快照的不可变质量评测事实。

### DatasetCertification

针对一个明确 DatasetVersion 和 CertificationProfile 的不可变认证结果。

### DeliveryOperation

DeliveryOperation 是每次 standalone delivery 尝试的稳定业务事实/操作身份，用于：
- 承载 delivery command 的幂等 identity；
- 记录 datasetVersion/certification/consumer/purpose/action/delivery mode；
- 冻结 gate result / blockers 与 issuance result；
- 作为 Audit/Evidence/CostAllocation 的强类型 subject。

受控生命周期至少表达：

~~~text
PREPARED
├→ BLOCKED
└→ ISSUANCE_PENDING
    ├→ ISSUED
    ├→ FAILED
    ├→ BLOCKED
    └→ CONTAINMENT_PENDING
         └→ BLOCKED
~~~

`ISSUANCE_PENDING` 是 crash-recovery / reconciliation 中间态，不是 terminal failure。若 fresh gate 变为 BLOCKED，但此前 provider outcome 可能已产生访问能力，必须先 reconcile；已签发则先 revoke/contain，无法确认 outcome 或 containment 时进入 `CONTAINMENT_PENDING`。只有确认没有活跃访问能力后，才允许终结为 `BLOCKED`。外部 provider 调用发生前必须先 durable persist 该状态和 stable provider_request_key。

每次 initial/retry/reconciliation issuance 前必须重新执行完整 CurrentDeliveryGate 并重新计算 expiry cap；旧 gate snapshot 只保留审计价值，不能授权新的 provider side effect。

provider 成功但 terminal DB commit 失败时，恢复流程必须使用同一 provider_request_key 查询/重放同一 issuance，而不是生成新的 credential。direct bearer mode 还必须能在 terminal commit 成功、HTTP response 丢失后通过同一 key 恢复同一 credential/访问能力；否则必须使用 platform redemption indirection。

不能只存在临时 HTTP 请求；也不能把可用 token/credential secret 正文持久化为领域事实。

### DataProduct / ProductRelease

DataProduct 是稳定产品身份。当前公开 API 中 DataProduct 创建为 DRAFT，成功 PublishProductRelease 会将其更新为 PUBLISHED；DESIGNING/DEVELOPING/TESTING/READY/ACTIVE/SUSPENDED/DEPRECATED/RETIRED 虽保留在 domain/schema 枚举中，但当前没有显式 lifecycle Command，因此不视为 API 可达迁移。

ProductRelease 是有显式生命周期的发布聚合。DRAFT/VALIDATING/READY 阶段允许按 Command 更新校验状态与绑定；进入 PUBLISHED 后，当前数据库 history guard 阻止任何 UPDATE，published bindings 与历史行整体冻结。SUSPENDED/WITHDRAWN 虽仍存在于 schema 枚举，但当前不是从 PUBLISHED 可达的 live transition；未来启用需要独立 migration + Command。

Certified Dataset 可独立作为交付对象，不要求必须包装成 DataProduct；实际 standalone delivery 由持久化 DeliveryOperation 表达，并必须通过 CurrentDeliveryGate：校验 DatasetVersion 当前可用性、当前有效的 CERTIFIED DatasetCertification，以及当前 consumer / purpose / action 的 CurrentEntitlementGate。DatasetCertification 只保留认证时点结论。

## 3. 实体模型

~~~text
EntityType
   ↓
Canonical Entity
   ↓
EntityMapping projection
   ↓
EntityMappingDecision history
~~~

EntityMapping 当前投影可以变化；生产与 Release trace 应绑定实际使用的 immutable decision。

## 4. 数据权利模型

### 4.1 平台归属与法律权利分离

~~~text
Platform owner / steward
≠
Legal rights proof
~~~

### 4.2 目标权利链

~~~text
Party / PartyRef
      ↓
RightsDeclaration
      ↓
RightsVerification / RightsDisposition
      ↓
AuthorizationProvenanceBinding
      ↓
AuthorizationProvenanceBindingDisposition (optional)
      ↓
Authorization
      ↓
RightsSnapshot
      ↓
EffectiveRights
~~~

角色语义至少区分：

- PROVIDER
- RIGHTS_HOLDER
- CUSTODIAN
- CONTROLLER
- PROCESSOR
- AUTHORIZED_USER

第一阶段允许使用稳定 party_ref，不要求先建设完整组织主数据平台。

### 4.3 RightsDeclaration

回答：

- 谁声明有什么权利？
- 针对哪个 DataResource？
- 依据是什么？
- 允许哪些动作？
- 有哪些限制？
- 有哪些 Evidence？
- 是否已经 VERIFIED？

### 4.4 RightsDisposition

VERIFIED RightsDeclaration 的历史不可改写，但当前有效性可以通过 append-only disposition 事实退出 current set：

- INVALIDATED
- SUPERSEDED（显式指向 replacement declaration）

Current rights selection 必须根据 as_of 和 disposition 判断，不能用 created_at/latest 猜测。

### 4.5 AuthorizationProvenanceBinding

把 Authorization / ResourceGrant 强类型绑定到支持它的 RightsDeclaration provenance。

必须证明：

- Authorization.grantor_ref 与 declaration 中可授权 party_ref 匹配，或存在明确可验证 delegation chain；
- 同一 DataResource；
- declaration allowed/grantable actions 与 scope 覆盖 Authorization 授出的 actions/scope；
- declaration 当前 VERIFIED、validity 与 disposition 条件有效；
- 同一 workspace。

不得把互不相关的 VERIFIED declaration 和 ACTIVE Authorization 独立拼接。

AuthorizationProvenanceBinding 一旦创建即为不可变 provenance fact；不得 UPDATE/DELETE 后把历史 binding ID 重连到另一 declaration/grantor/actions/scope。

错误/失效 binding 通过 append-only AuthorizationProvenanceBindingDisposition 退出 current set：
- INVALIDATED
- SUPERSEDED（显式 superseded_by_binding_id）
- effective_at / reason / Evidence / actor

Current binding selection 按 as_of 排除已生效 disposition。replacement binding 必须独立满足 grantor/resource/actions/scope/declaration-current-validity 约束，不能因 supersession 自动获得有效性。历史 RightsSnapshot 继续引用旧 binding，不被回溯改写。

### 4.6 Authorization

回答：

~~~text
Grantor + Grantee + Resource + Purpose + Action + Scope + Validity → Decision
~~~

Authorization 不是所有权证明；Grantor 的授权资格应能追溯至 Rights Provenance。

CurrentEntitlementGate 不能只检查 Authorization 是否 ACTIVE/未过期。每个被使用的 Authorization 必须同时覆盖当前请求的 grantee/consumer、DataResource、purpose、action 与 scope；窄授权不能因为绑定的 RightsDeclaration 范围更宽而被“放大”。

### 4.7 EffectiveRights

衍生 DatasetVersion 的有效权利由输入资源权利、授权、Purpose 和生产 lineage 共同决定。

V1 默认 fail closed。

## 5. Quality 模型

~~~text
QualityRuleSet
      ↓
QualityAssessment
      ├── DimensionSummary
      └── Findings
~~~

V1 维度：

- Completeness
- Accuracy
- Consistency
- Uniqueness
- Timeliness
- Traceability

QualityAssessment 保存规则版本及内容 hash/snapshot。规则当前文件变化不能改变历史 Assessment。

## 6. Certification 模型

~~~text
CertificationProfile
      ↓
DatasetCertification
      ↓
CertificationDisposition (optional: REVOKED / SUPERSEDED)
      ↓
Certified DatasetVersion / current certification eligibility
~~~

CertificationProfile 定义 purpose、quality、rights、compliance、contract、traceability/evidence 等要求。

DatasetCertification 绑定实际使用的 Profile snapshot/hash 和所有认证证据。CertificationDisposition 是 append-only 历史事实，用于让错误或被替代的认证退出 current set；current certification 不能通过 latest timestamp 推断。

## 7. 工作流模型

~~~text
Workflow
  ↓
WorkflowVersion
  ↓
Execution
  ├── immutable inputs
  ├── frozen dependency bindings
  ├── mapping usages
  └── output DatasetVersion
~~~

业务 Workflow 不等于 Apache Hop Workflow。

## 8. 产品模型

~~~text
DataProduct
  ↓
ProductVersion
  ├── ProductAsset(DATASET/API/REPORT/...)
  ↓
ProductRelease
~~~

ProductRelease 必须经过 ReleaseReadiness。

ProductRelease 和 DatasetCertification 是不同事实：

- Certification：DatasetVersion 是否满足某标准/用途；
- Release：DataProduct 版本是否满足发布条件并实际发布。

## 9. 证据模型

~~~text
Business Fact / Claim
          ↓
      Evidence
          ↓
 EvidenceRelation
          ↓
 EvidenceSnapshot（需要冻结时）
~~~

Evidence 用于证明事实；AuditEvent 用于记录“谁做了什么”。两者不能混用。

Rights verification、QualityAssessment、DatasetCertification 都应将 Evidence 纳入业务边界。

## 10. 版本与不可变原则

独立版本/不可变历史至少包括：

- DatasetVersion
- EntityMappingDecision
- WorkflowVersion
- execution dependency facts
- ContractVersion
- ProductVersion
- ProductRelease published bindings / published release history
- EvidenceSnapshot
- RightsSnapshot
- QualityAssessment（已实现）
- DeliveryOperation（#135）
- verified RightsDeclaration / verification / disposition facts（#137）
- AuthorizationProvenanceBinding / BindingDisposition（#137）
- CertificationProfile snapshot（#134）
- DatasetCertification / CertificationDisposition（#134）

错误通过追加新事实修正，不覆盖历史。
