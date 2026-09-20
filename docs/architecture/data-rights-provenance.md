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
- allowed actions
- restrictions
- purpose（如适用）
- Evidence
- verification result

声明与 verification 分离。

未经 VERIFIED 的声明不得作为正式 Certification 权利依据。

## 5. Authorization 与 Provenance Binding

现有 Authorization 继续回答：

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

Binding 至少表达：

- workspace_id
- authorization_id
- data_resource_id
- rights_declaration_id
- grantor_ref
- supported_actions / scope（如按 grant 粒度绑定）
- created_at / actor

建立/验证 Binding 时必须 fail closed：

1. Authorization.grantor_ref 与声明中承担可授权角色的 party_ref 明确匹配，或存在平台显式支持且可验证的 delegation chain；
2. 声明覆盖同一 DataResource；
3. 声明的 allowed/grantable actions 与 scope 足以支持该 Authorization 授出的 actions/scope；
4. 声明自身 VERIFIED、validity、disposition 条件满足；
5. 跨 workspace 引用拒绝。

V1 不推断“同一个资源上任何 VERIFIED 声明都能支持任何 grantor”。如果无法证明 grantor 与 provenance 的关系，则 Authorization 不能进入 CurrentEntitlementGate。

历史 RightsSnapshot 应冻结实际使用的 AuthorizationProvenanceBinding / declaration IDs，使“为什么这个 grantor 有权授权”可追溯。

## 6. RightsSnapshot

RightsSnapshot 冻结某一时点、某一 Purpose / Consumer 实际使用的授权集合。

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

必须基于输入资源和 lineage 求有效动作。

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

## 8. Restriction Semantics

限制必须能够机器判断，不能全部放自然语言备注。

允许 JSONB 保存扩展参数，但 action / decision / resource / party / purpose 等核心字段应为强类型。

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

1. 存在 VERIFIED verification fact；
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
