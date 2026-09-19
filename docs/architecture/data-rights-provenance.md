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

## 5. Authorization

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

因此正式授权链应能回溯到可验证 RightsDeclaration。

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

## 10. 修正语义

VERIFIED 权利事实不通过 UPDATE 覆盖。

~~~text
old declaration / verification fact remains
        ↓
new declaration or new verification fact
        ↓
new Authorization / Snapshot as needed
~~~

历史 Release / Certification 继续解释当时的事实。

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
