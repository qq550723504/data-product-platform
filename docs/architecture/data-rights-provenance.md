# Data Rights Provenance 架构

对应：#137。

## 1. 核心原则

### Platform Ownership ≠ Legal Data Rights

data_resource.owner_id / dataset.owner_id 表示平台内资产责任、负责人或归属。

它们不是现实世界法律所有权的自动证明。

平台目标不是裁定“谁法律上拥有数据”，而是记录和验证：

- 谁提供数据
- 谁主张什么权利
- 依据是什么
- 谁授权给谁
- 允许什么动作
- 有什么限制
- 当时用了哪些权利依据
- Evidence 在哪里

## 2. 权利链

~~~text
Party / PartyRef
      ↓
RightsDeclaration
      ↓
AuthorizationProvenanceBinding
      ↓
Authorization
      ↓
RightsSnapshot
      ↓
Effective Rights
      ↓
DatasetCertification
~~~

## 3. Party Roles

V1 至少区分：

- PROVIDER：数据提供方
- RIGHTS_HOLDER：权利主张/持有方
- CUSTODIAN：保管/管理方
- CONTROLLER：决定用途/处理方式的主体
- PROCESSOR：受托加工方
- AUTHORIZED_USER：获授权使用方

第一阶段可使用稳定 party_ref，不要求建设完整组织主数据平台。

## 4. RightsDeclaration

声明针对明确 DataResource，至少表达：

- claimant / parties
- rights role
- basis_type
- basis_ref
- validity
- allowed_actions：该 party 自身被允许执行的使用动作
- grant_authority_mode：NONE / EXPLICIT（或固定等价）；**allowed 不等于 grantable**
- grantable_actions：仅在 grant_authority_mode=EXPLICIT 时生效，表示该 party 有权进一步授予第三方的动作集合
- grantable purposes：强类型 purpose_code / normalized relation；不得默认等于 permitted purposes
- grantable scope_type / grantable scope_ref（或等价 normalized relation）；不得默认等于 use scope
- restrictions / transfer / sublicensing semantics（机器可判断；需要 onward delegation 时显式表达）
- consumer applicability（ANY / EXPLICIT；EXPLICIT 时强类型 consumer_ref / consumer_type）
- purpose / permitted purposes（强类型 purpose_code 或规范化 declaration-purpose relation）
- use scope_type / use scope_ref（用于表达资源内 object/row/prefix/policy 范围；复杂扩展参数可以 JSONB，但 gate 比较所需 identity 必须强类型可查询）
- Evidence
- verification result

声明与 verification 分离。

未经 VERIFIED 的声明不得作为正式 Certification 权利依据。

### Verification outcome 规则

RightsVerification 是 append-only terminal decision fact，但**同一个 RightsDeclaration 只能有一个 terminal verification outcome**：

- VERIFIED
- REJECTED

数据库必须保证一个 declaration_id 最多一个 terminal outcome；VerifyRightsDeclaration 与 RejectRightsDeclaration 互斥且幂等。

因此：
- PENDING/UNVERIFIED → VERIFIED 或 REJECTED；
- 已 VERIFIED 的同一 declaration 不能再追加 REJECTED 来“纠正”；
- 已 REJECTED 的同一 declaration 不能再追加 VERIFIED 来“翻转”；
- 错误 VERIFIED 的纠正使用 RightsDisposition(INVALIDATED / SUPERSEDED)，必要时创建新的 RightsDeclaration 并独立 Verify；
- 错误/过时 REJECTED 若需要重新主张，创建新的 RightsDeclaration + 新 verification，不覆盖旧 outcome。

Current rights selection 只接受“该 declaration 的唯一 terminal outcome = VERIFIED”，不能采用“历史上存在过 VERIFIED fact”这一宽松判定。

## 5. Authorization 与 Provenance Binding

现有 Authorization 继续回答：

> **存储兼容说明**：现有 `authorization_resource.scope jsonb` 不是 gate-critical scope 的权威表示。#137 必须新增/补齐强类型、可索引的 Authorization scope identity（至少 `scope_type + scope_ref`，或等价 normalized child relation）。JSONB 只保留受控扩展参数。BindAuthorizationProvenance、CurrentEntitlementGate、RightsSnapshot 都读取同一 normalized scope 语义；legacy Authorization 若不能无歧义 backfill normalized scope，必须 fail closed / 标记不可用于 entitlement，不得把缺失当作 ALL_RESOURCE。

~~~text
Grantor
→ Grantee
→ DataResource
→ Purpose
→ Actions
→ Scope
→ Validity
~~~

Authorization 本身不证明 Grantor 为什么有权授权。

第一阶段必须建立强类型 `AuthorizationProvenanceBinding`（具体表名可由 #137 实现确定），把一个 Authorization / ResourceGrant 显式绑定到支持它的 RightsDeclaration provenance，而不是分别独立挑选两组事实。

绑定必须通过显式业务 Command 创建，例如：

- `BindAuthorizationProvenance` / `CreateAuthorizationProvenanceBinding`

该 Command 在写入 binding 前完成 grantor/resource/actions/scope/workspace/verification/validity/disposition 校验；禁止通过通用 CRUD 或直接持久化绕过这些规则。

Binding 至少表达：

- workspace_id
- authorization_id
- data_resource_id
- rights_declaration_id
- grantor_ref
- grantor_authority_mode：DIRECT_DECLARATION_PARTY / DELEGATED（或固定等价枚举）
- grantor_delegation_chain_id / chain_hash（DELEGATED 时必填；强类型引用，不得只放 metadata/JSONB）
- supported_grantable_actions / supported_grantable_purpose / normalized grant scope（如按 grant 粒度绑定）
- created_at / actor

建立/验证 Binding 时必须 fail closed：

1. Authorization.grantor_ref 与声明中承担可授权角色的 party_ref 明确匹配；若依赖 delegation，则必须引用**强类型 GrantorAuthorityDelegationChain**（名称可由实现固定），链条从 declaration-supported delegator 到 Authorization.grantor_ref 可验证且不可缺边；
   - 每个 delegation edge 至少有 delegator_ref、delegate_ref、resource、**grantable_actions / grantable purpose / normalized grant scope / onward_grant_mode**、valid_from/valid_to、Evidence；如果还需要表达 delegate 自己的 use permission，应使用独立 use-permission 字段/事实，不能与 grant authority 共用一个 actions 集合；
   - delegation edge / chain 的撤销或纠正使用 append-only disposition（至少 REVOKED / INVALIDATED / SUPERSEDED + effective_at），不能覆盖历史；
   - chain identity + ordered member edge IDs/hash 必须可查询/可冻结；如果 chain 采用 DRAFT→FINALIZED，member mutation 与 Finalize 必须共享同一个 parent chain row lock/fence（parent-first 固定顺序），Finalize 持锁校验 ordered members/hash 后提交；FINALIZED 后 member mutation 全部拒绝，不能出现 late member commit 改写已冻结 grant authority；
2. 声明覆盖同一 DataResource；
3. **Authorization 的授予必须由 grant authority 支撑，不能只看 allowed/use permission。** declaration.grant_authority_mode 必须允许 grant，且 grantable_actions、grantable purposes、grantable scope 必须逐项覆盖 Authorization 授出的 action/purpose/scope；declaration 只有 allowed USE/PROCESS 而 grantable_actions 为空时，不能作为任何对第三方 Authorization 的 grant source；
4. 若通过 delegation chain 传递 grant authority，每一 edge 必须显式表达其**可继续授予的 grantable actions/purpose/scope**；只有 use permission、但没有 onward grant authority 的 edge 会在该处终止授权链；
5. 声明自身 VERIFIED、validity、disposition 条件满足；
6. 跨 workspace 引用拒绝。

V1 不推断“同一个资源上任何 VERIFIED 声明都能支持任何 grantor”。如果无法证明 grantor 与 provenance 的关系，则 Authorization 不能进入 CurrentEntitlementGate。

历史 RightsSnapshot 应冻结实际使用的 AuthorizationProvenanceBinding / declaration IDs，使“为什么这个 grantor 有权授权”可追溯。

RightsSnapshot 的冻结范围包括 snapshot header **以及全部 membership rows**（Authorization / RightsDeclaration / AuthorizationProvenanceBinding / grantor delegation chain + edge identities 等）。snapshot finalize 后，membership 不得 INSERT/UPDATE/DELETE；数据库必须有 guard，不能通过替换成员关系而保持 snapshot ID 不变来改写历史 provenance。

### Binding 修正 / 退休

AuthorizationProvenanceBinding 本身不可 UPDATE / DELETE。第一阶段必须提供 append-only AuthorizationProvenanceBindingDisposition：

- INVALIDATED
- SUPERSEDED → 显式 superseded_by_binding_id
- effective_at
- reason
- Evidence
- actor

对应显式 Command：

- InvalidateAuthorizationProvenanceBinding
- SupersedeAuthorizationProvenanceBinding

CurrentEntitlementGate 选择 binding 时必须按 as_of 排除已生效的 INVALIDATED / SUPERSEDED binding；不得因为 Authorization 仍 ACTIVE、declaration 仍 VERIFIED 就继续选择已退休 binding。replacement binding 必须重新通过完整 binding 校验。

如果 binding 的 grantor_authority_mode=DELEGATED，则**每次 CurrentEntitlementGate 都必须重新验证 grantor delegation chain 的当前有效性**，不能把“binding 创建时验证过”当永久授权：
- chain 的所有 required edges 在 as_of 时都存在且 validity 覆盖 as_of；
- 没有已生效 REVOKED / INVALIDATED / SUPERSEDED disposition；
- 每一跳的 delegator→delegate 连续，最终 delegate=Authorization.grantor_ref；
- **每一跳都具有满足下游 Authorization 的 grantable actions/purpose/scope / onward-grant authority；只有“允许自己使用”的动作不能被解释成“允许继续授权”；**
- resource / grantable purpose / grantable action / normalized grant scope 逐跳不得比上游放宽；
- 任一 edge 过期、撤销、缺失或无法验证时，binding 即使自身未被 disposition，也不得进入 CurrentEntitlementGate。

RightsSnapshot 冻结 binding + grantor delegation chain identity/member edge IDs 用于历史解释，但 frozen snapshot **不替代 delivery-time current chain validation**。

## 6. RightsSnapshot

RightsSnapshot 冻结某一时点、某一 Purpose / Consumer 实际使用的授权集合。它是一个整体不可变集合：header、authorization membership、declaration membership、provenance-binding membership 必须一起冻结。

Snapshot 不应在未来通过读取“当前声明”改变历史解释。

目标 manifest / query 至少能解释：

- Authorization
- Grantor / Grantee
- Resource
- Purpose
- Actions
- validity
- relevant verified rights provenance
- restrictions
- Evidence

## 7. Effective Rights

衍生 Dataset 的权利不能简单设置 output.owner_id = platform。

必须基于输入资源和 lineage 求有效动作，而且结果必须是**可持久化、可冻结、可解释的历史事实**，不是每次查询临时拼出的布尔值。

V1 推荐模型为 immutable `EffectiveRightsSnapshot`（名称可由实现固定），至少冻结：

- workspace_id / target_dataset_version_id；
- calculation_as_of、consumer/purpose/context（如计算按 context 区分）；
- calculation_rule_version + rule/content hash；
- target lineage / required-input set 的稳定 hash；
- required input membership：每个必要 input DatasetVersion/DataResource + 对应 RightsSnapshot、RightsDeclaration/Binding/Authorization provenance 引用；
- 每个 action 的 decision（ALLOWED / NOT_ALLOWED；UNKNOWN 不得被解释为 allowed）；
- restriction/result reason 与阻断来源，可追到具体 input membership；
- finalized_at / actor / Evidence/Audit refs。

snapshot header、input membership、action decision membership 在 FINALIZED 后必须全部 immutable；修正只能创建新的 EffectiveRightsSnapshot，历史 DatasetCertification 继续引用旧 snapshot。

计算路径必须从 target DatasetVersion 的**实际 required lineage/input membership**出发，不能由调用方传一个缩水后的输入列表。每个必要输入缺少可用 current/frozen rights fact、scope/context 无法比较或 lineage 不完整时，计算 fail closed。

V1 action 合成规则：对每一个 required input 取允许集合交集；只有所有必要输入都明确允许某 action，output 才 ALLOWED。任一输入 deny/restrict/unknown/missing → output NOT_ALLOWED。限制项采用最严格/并集合成（按实现固定的 restriction semantics），不得因其它输入更宽而消除限制。

### Delivery-time Current Effective Rights

`EffectiveRightsSnapshot` 是认证/历史解释事实，**不是永久 delivery authorization**。对衍生 DatasetVersion 的每次 CurrentDeliveryGate，必须基于 target 的 immutable required lineage/input membership，重新计算 `CurrentEffectiveRightsGate`（名称可由实现固定）：

1. 枚举 target DatasetVersion 的**全部 required inputs**；不得只检查顶层 output 的一条 RightsSnapshot，也不得由客户端缩减 input list；
2. 对每个 required input，在当前 `as_of` 下重新选择/验证 RightsDeclaration、AuthorizationProvenanceBinding、Authorization、grantor delegation chain 及其 dispositions/validity/context；
3. 对 requested action/purpose/consumer/scope 执行与 Effective Rights 相同的 fail-closed 交集；任一 required input 当前 BLOCKED / UNKNOWN / missing，则 derived output 当前 action BLOCKED；
4. 历史 EffectiveRightsSnapshot 仍用于证明“认证时为什么允许”，但 delivery-time current result 可以因为任一 source input 后续 revocation/disposition 变为 BLOCKED；
5. 所有 required-input current-rights dependencies 都必须纳入 delivery authorization fence/revision 与 credential expiry cap，防止 source revocation 穿越 terminal finalize。

因此，“认证后 input B 的 declaration 被 INVALIDATED”不能因为历史 snapshot.SHARE=ALLOWED 而继续交付衍生 output SHARE。

V1 规则：fail closed。

~~~text
Input A: PROCESS, DERIVE, NO_RESALE
Input B: PROCESS, DERIVE, RESALE
Input C: PROCESS, DERIVE

Output:
PROCESS = ALLOWED
DERIVE  = ALLOWED
RESALE  = NOT_ALLOWED
~~~

第一阶段至少覆盖：

- USE
- PROCESS
- DERIVE
- SHARE
- RAW_EXPORT
- RESALE
- AI_TRAINING

计算/finalize 必须产生 `EffectiveRightsCalculated` / `EffectiveRightsFinalized`（或固定等价事件）+ Audit/Evidence/Outbox；#134 DatasetCertification 只能引用 finalized immutable Effective Rights identity/hash。

## 8. Restriction Semantics

限制必须能够机器判断，不能全部放自然语言备注。

允许 JSONB 保存扩展参数，但 action / decision / resource / party / consumer applicability / purpose / scope / validity 等 CurrentEntitlementGate 依赖字段必须为强类型、可索引、可查询。不得把 fail-closed 所需的 consumer/purpose/scope 只藏在任意 JSONB。

典型限制：

- NO_RAW_EXPORT
- NO_RESALE
- NO_AI_TRAINING
- PURPOSE_ONLY
- VALID_TO

## 9. Evidence

RightsDeclaration verification 应关联 Evidence。

Public 仓库不得保存真实客户合同正文或非公开权利文件。

测试使用合成 evidence metadata / hash；真实证据保存在受控环境或外部安全存储，只在 Core 保存必要引用与哈希。

## 10. 修正、撤销与当前事实选择

VERIFIED 权利事实不通过 UPDATE 覆盖，但必须能够显式声明它从某一时点起不再是“当前有效依据”。

第一阶段增加 append-only 的 disposition / validity fact（具体表名由 #137 实现确定），至少表达：

- declaration_id
- disposition: INVALIDATED / SUPERSEDED
- effective_at
- reason
- Evidence
- actor
- superseded_by_declaration_id（SUPERSEDED 时）

对应显式 Command 至少包括：

- InvalidateRightsDeclaration
- SupersedeRightsDeclaration

~~~text
verified declaration A
        ↓
append INVALIDATED(A, effective_at, reason)
或
append SUPERSEDED(A → B, effective_at, reason)
~~~

当前权利查询不得按“最新 created_at”猜测，也不得继续选择已经在 as_of 时点生效的 INVALIDATED / SUPERSEDED 声明。

Current selection rule：

对**每一个候选 RightsDeclaration**，都必须在查询 `as_of` 时点同时满足：

1. 该 declaration 存在且仅存在一个 terminal RightsVerification outcome，并且 decision = VERIFIED；
2. declaration 自身的 `effective_from / effective_to`（或等价 validity window）覆盖 `as_of`；
3. declaration 的 resource / purpose / action / consumer / scope 与本次查询匹配；
4. 在 `as_of` 之前不存在已生效的 INVALIDATED disposition；
5. 在 `as_of` 之前不存在使其退出当前集合的 SUPERSEDED disposition；
6. 若 A 被 B supersede，B 必须独立满足上述 VERIFIED / validity / scope 条件，不能因为 supersession 自动继承 VERIFIED；
7. 历史 RightsSnapshot 仍保留并解释当时使用的 A，不被新 disposition 回溯改写。

任何 validity 或 scope 不满足的声明都不得进入 CurrentEntitlementGate，即使其 verification 仍为 VERIFIED。

这样既保留不可变审计历史，又能让 CurrentEntitlementGate 排除已经撤销或取代的 provenance。

## 11. 与 Certification / Delivery 的关系

DatasetCertification 冻结“认证时点”使用的 RightsSnapshot / Effective Rights，用来解释为什么当时可以 CERTIFIED；它不把时限授权永久化。

CertificationProfile 可以要求：

- provenance verified
- Purpose = X
- required actions
- prohibited restrictions absent

#134 Certification 必须 fail closed：

- provenance 缺失
- Authorization 无效
- Purpose 不匹配
- required action 不允许
- required input rights 不完整

均不能 CERTIFIED。

### CurrentEntitlementGate

每次对 Certified Dataset 执行独立交付前，必须使用当前时间和明确的 consumer / purpose / action 重新判断当前权利：

~~~text
historical DatasetCertification
        +
current verified provenance
        +
current Authorization validity/status
        +
current Effective Rights
        +
consumer / purpose / action
        ↓
DELIVERY_ALLOWED / DELIVERY_BLOCKED
~~~

Authorization 过期、暂停、撤销，RightsDeclaration 后续失效，或 required action 当前不再允许时，即使历史 Certification 为 CERTIFIED，也必须 DELIVERY_BLOCKED。

此外，每个 Authorization 必须对本次请求本身成立：其 grantee 必须匹配 consumer（或符合明确支持的主体映射规则），resource 必须是本次 DataResource，purpose/action/scope 必须覆盖 requested purpose/action/scope。Declaration 的宽权限不能扩张一条更窄的 Authorization。

该 gate 是第一阶段必需；周期性后台重认证仍可后置。

## 12. 非目标

第一阶段不做：

- 现实世界法律所有权自动裁判
- 通用法律推理
- 完整合同生命周期系统
- 法律文书 OCR/NLP
- 电子签章/PKI
- 区块链存证
- 跨法域法律意见

平台提供的是可审计的数据权利事实链，不是法律意见。
