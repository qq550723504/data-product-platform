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
- 持有当前 gate/status projection；真正的每次 gate decision / blockers / dependency revision 与状态迁移由 append-only DeliveryGateEvaluation / DeliveryTransition facts 冻结；
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
         ├→ BLOCKED
         └→ FAILED
~~~

`ISSUANCE_PENDING` 是 crash-recovery / reconciliation 中间态，不是 terminal failure。若 fresh gate 变为 BLOCKED，但此前 provider outcome 可能已产生访问能力，必须先 reconcile；已签发则先 revoke/contain，无法确认 outcome 或 containment 时进入 `CONTAINMENT_PENDING`。**CONTAINMENT_PENDING 虽非终态，但属于安全关键 transition：状态变更、`DatasetDeliveryContainmentPending`（或固定等价）Domain Event、Audit/Evidence、Outbox 必须同事务提交，幂等重放不重复事件。** 只有确认没有活跃访问能力后，才允许终结：fresh gate 已 BLOCKED 时进入 `BLOCKED`；gate 仍 ALLOWED 但 issuance contract 无法满足（如 credential 无法缩短到 fresh cap）时进入 `FAILED`，并再产生各自 terminal event。外部 provider 调用发生前必须先 durable persist 该状态和 stable provider_request_key。

每次 initial/retry/reconciliation issuance 前必须重新验证 authenticated caller principal 当前仍可代表 effective consumer/workspace（含 binding/membership/delegation），再执行完整 CurrentDeliveryGate 并重新计算 expiry cap；旧 identity/gate snapshot 只保留审计价值，不能授权新的 provider side effect。

每次上述 gate 都必须追加独立 DeliveryGateEvaluation（INITIAL / RETRY / RECONCILIATION / TERMINAL_FINALIZE 等），保存 decision/blockers + dependency fence/revision；初始 ALLOWED 与后续 BLOCKED 必须同时保留。DeliveryOperation.current_gate_decision/status 仅作 projection，不能覆盖历史 evaluation。

provider 返回/恢复 access capability 后，在 terminal ISSUED transaction 中必须使用共享 delivery authorization fence/revision，与 caller binding/membership/delegation lifecycle 以及所有影响 gate 的 entitlement-changing Commands 线性化，并再次验证 caller authority + 完整 re-gate + fresh-cap。该 terminal commit 是 issuance 的线性化点。

provider 成功但 terminal DB commit 失败时，恢复流程必须使用同一 provider_request_key 查询/重放同一 issuance，而不是生成新的 credential。**任何首次返回或恢复出的 credential 在进入 ISSUED 前，都必须验证其实际 expiry/access bound <= 当前 fresh cap。** 若旧 credential 超过 fresh cap，必须先安全 shorten 并验证，或 revoke/contain；不能直接恢复为 ISSUED。direct bearer mode 还必须能在 terminal commit 成功、HTTP response 丢失后通过同一 key 恢复同一 credential/访问能力；否则必须使用 platform redemption indirection。

不能只存在临时 HTTP 请求；也不能把可用 token/credential secret 正文持久化为领域事实。

### DataProduct / ProductRelease

DataProduct 是稳定产品身份。当前公开 API 中 DataProduct 创建为 DRAFT，成功 PublishProductRelease 会将其更新为 PUBLISHED；DESIGNING/DEVELOPING/TESTING/READY/ACTIVE/SUSPENDED/DEPRECATED/RETIRED 虽保留在 domain/schema 枚举中，但当前没有显式 lifecycle Command，因此不视为 API 可达迁移。

ProductRelease 是有显式生命周期的发布聚合。DRAFT/VALIDATING/READY 阶段允许按 Command 更新校验状态与绑定。进入 PUBLISHED 后，领域 invariant 要求 release row 与 dataset membership 都不可被回溯改写；**但当前数据库只由 `guard_product_release_history` 保护 `product_release` 主行，`product_release_dataset` 仍缺 INSERT/UPDATE/DELETE membership guard（#99 open）**。因此当前不能把“published bindings 已由数据库整体冻结”描述为已实现事实；这是已有 Core enforcement gap。SUSPENDED/WITHDRAWN 虽仍存在于 schema 枚举，但当前不是从 PUBLISHED 可达的 live transition；未来启用需要独立 migration + Command，并同时保持/补齐 membership immutability。

Certified Dataset 可独立作为交付对象，不要求必须包装成 DataProduct；实际 standalone delivery 由持久化 DeliveryOperation 表达。进入 CurrentDeliveryGate 前先建立 trusted caller principal → effective consumer/workspace（on-behalf-of 必须有当前有效 delegation），随后再校验 DatasetVersion 当前可用性、当前有效的 CERTIFIED DatasetCertification，以及 effective consumer / purpose / action 的 CurrentEntitlementGate。DatasetCertification 只保留认证时点结论。

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
(header + immutable authorization/declaration/binding membership)
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

### 4.4 RightsVerification outcome

RightsDeclaration 的 gate-relevant context（resource、consumer applicability、purpose、action、scope、validity）必须强类型可查询；JSONB 只承载扩展参数。

RightsVerification 与 RightsDeclaration 分离，但同一个 RightsDeclaration 只允许一个 terminal verification decision：VERIFIED 或 REJECTED。Verify / Reject 互斥；同一 declaration 的 terminal outcome 不允许后续翻转。

错误 VERIFIED 通过 RightsDisposition INVALIDATED / SUPERSEDED 退出 current set；如需修正内容，创建新的 RightsDeclaration 并独立 verification。错误/过时 REJECTED 也通过新的 RightsDeclaration 重新主张，不在原 declaration 上追加 VERIFIED。

Current rights selection 必须读取该 declaration 的唯一 terminal outcome，并要求 decision = VERIFIED；不能只判断历史上“存在过 VERIFIED”。

### 4.5 RightsDisposition

VERIFIED RightsDeclaration 的历史不可改写，但当前有效性可以通过 append-only disposition 事实退出 current set：

- INVALIDATED
- SUPERSEDED（显式指向 replacement declaration）

Current rights selection 必须根据 as_of 和 disposition 判断，不能用 created_at/latest 猜测。

### 4.7 AuthorizationProvenanceBinding

把 Authorization / ResourceGrant 强类型绑定到支持它的 RightsDeclaration provenance。

必须证明：

- Authorization.grantor_ref 与 declaration 中可授权 party_ref 匹配，或存在明确强类型 delegation chain；delegation chain 每一跳都必须携带 current-valid onward grant authority，不能把 use permission 当作 sublicensing authority；
- 同一 DataResource；
- declaration 必须有显式 grant authority；grantable actions / grantable purpose / grantable scope 覆盖 Authorization 授出的 actions/purpose/scope。allowed/use permission 不产生 grant authority；
- declaration 当前 VERIFIED、validity 与 disposition 条件有效；
- 同一 workspace。

不得把互不相关的 VERIFIED declaration 和 ACTIVE Authorization 独立拼接。

AuthorizationProvenanceBinding 一旦创建即为不可变 provenance fact；不得 UPDATE/DELETE 后把历史 binding ID 重连到另一 declaration/grantor/actions/scope。

错误/失效 binding 通过 append-only AuthorizationProvenanceBindingDisposition 退出 current set：
- INVALIDATED
- SUPERSEDED（显式 superseded_by_binding_id）
- effective_at / reason / Evidence / actor

Current binding selection 按 as_of 排除已生效 disposition。replacement binding 必须独立满足 grantor/resource/actions/scope/declaration-current-validity 约束，不能因 supersession 自动获得有效性。历史 RightsSnapshot 继续引用旧 binding，不被回溯改写。

RightsSnapshot 的不可变性覆盖 membership：finalized snapshot 的 authorization/declaration/provenance-binding 成员关系禁止后续插入、删除或重连。

### 4.6 Authorization

回答：

~~~text
Grantor + Grantee + Resource + Purpose + Action + Scope + Validity → Decision
~~~

Authorization 不是所有权证明；Grantor 的授权资格应能追溯至 Rights Provenance。

CurrentEntitlementGate 不能只检查 Authorization 是否 ACTIVE/未过期。每个被使用的 Authorization 必须同时覆盖当前请求的 grantee/consumer、DataResource、purpose、action 与 scope；窄授权不能因为绑定的 RightsDeclaration 范围更宽而被“放大”。

### 4.7 EffectiveRights

衍生 DatasetVersion 的有效权利由输入资源权利、授权、Purpose 和生产 lineage 共同决定。

EffectiveRights 不是瞬时 query value，而是可冻结的 immutable aggregate。至少具有：

- target DatasetVersion；
- calculation rule version/hash 与 calculation context/as_of；
- actual required lineage/input membership；
- 每个 input 使用的 RightsSnapshot / provenance identity；
- 每个 action 的 ALLOWED / NOT_ALLOWED decision + reason/source；
- finalized identity/hash，供 DatasetCertification 强类型引用。

所有 required inputs 参与交集合成；任一必要输入 deny/unknown/missing 时对应 action NOT_ALLOWED。调用方不能通过少传输入获得更宽 Effective Rights。FINALIZED 后 header/input/action membership 不可修改，修正产生新 aggregate。

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

CertificationProfile 定义 purpose、action、consumer、delivery channel/mode、quality、rights、compliance、contract、traceability/evidence 等要求。

purpose/action/consumer/delivery channel 四个 delivery-context 维度都必须显式使用 ANY / EXPLICIT（或实现固定的等价枚举）表达覆盖范围；缺失/NULL/UNKNOWN 不表示不限，而是“无法证明认证覆盖”，CurrentCertificationGate 必须 fail closed。Profile snapshot/hash 冻结这些 mode 及其 explicit membership。

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
- ProductRelease published bindings / published release history（目标 immutable invariant；`product_release_dataset` DB membership guard 仍由 #99 跟踪）
- EvidenceSnapshot（目标 immutable invariant；当前 header guard 已有，但 `evidence_snapshot_item` membership DB guard 仍由 #99 跟踪）
- RightsSnapshot（目标 immutable invariant；现有 header guard 已有，但 `rights_snapshot_authorization` membership guard 仍待 #137 实现）
- QualityAssessment（已实现）
- DeliveryOperation 的固定 request/idempotency identity、已冻结 gate/issuance history 与 terminal outcome（#135）；DeliveryOperation lifecycle row 本身不是从创建起 immutable
- verified RightsDeclaration / verification / disposition facts（#137）
- AuthorizationProvenanceBinding / BindingDisposition（#137）
- CertificationProfile snapshot（#134）
- DatasetCertification / CertificationDisposition（#134）

DeliveryOperation 特例：它与 Execution 类似，是受控 lifecycle row。PREPARED / ISSUANCE_PENDING / CONTAINMENT_PENDING / terminal 状态允许由显式 delivery/reconciliation Command 更新；不得给整行安装“创建后禁止 UPDATE”的不可变 guard。不可回写的是已经确定的 request/idempotency identity、provider_request_key（确定后稳定）、已提交的 gate/issuance transition history，以及 terminal outcome 的业务含义。需要审计每次迁移时使用 append-only Event/Audit/Evidence/Outbox/transition facts。

错误通过追加新事实修正，不覆盖历史。
