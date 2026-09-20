# Certified Dataset Pilot 路线

## 1. 阶段定位

核心 POC 已完成。本路线是 POC 之后的受控试点，目标是把已经验证的数据生产内核组合成客户可验收的“高质量数据集”交付物。

主 Epic：#129。

第一阶段完成后，平台应能够把一组原始数据生产为：

~~~text
有明确 DatasetVersion
+ 有可解释 QualityAssessment
+ 有可验证 Data Rights Provenance
+ 有 Compliance / Contract
+ 有 Lineage / Evidence
+ 有 CertificationProfile
+ 有 DatasetCertification
= Certified DatasetVersion
~~~

## 2. 第一阶段任务

~~~text
#131 QualityAssessment core ✅ (#140 / migration 000019)
        ↓
#132 Quality Engine
        ↓
#133 Quality Report

#137 Data Rights Provenance
        ↓
#134 DatasetCertification
        ↓
#135 API / UI
        ↓
#136 enterprise-activity E2E Pilot
~~~

#134 明确依赖 #137。

## 3. HQD-1 #131

QualityAssessment 核心已经通过 #140 / migration 000019 落地，继续复用 `quality_result` / `quality_finding` 作为兼容存储/API 名称。

#131 后续只处理在 review 中新增但 #140 未覆盖的 follow-up（例如 typed CostAllocation），不重复设计或迁移 QualityAssessment root。

必须固定：

- DatasetVersion
- RuleSet ref/version
- RuleSet content hash / snapshot
- evaluator identity
- metrics / findings
- decision
- Evidence / Audit
- CostEvent（实际发生的 engine invocation / compute / human-review quantity 或金额；金额未知时不得伪造，可记录 quantity/unit）
- typed CostAllocation → QualityAssessment

同一评测业务重试不得重复记 CostEvent；必须使用稳定 activity_id / operation identity + component_key，并由 PostgreSQL 唯一约束保证幂等。

历史 Assessment 不因规则文件变化而改变解释。

## 4. HQD-2 #132

通用 industry-pack Quality Engine。

V1 六个质量维度：

- Completeness
- Accuracy
- Consistency
- Uniqueness
- Timeliness
- Traceability

不强制全行业统一总分。

## 5. HQD-3 #133

提供可读 Quality Report。

客户能够看到：

- 六维摘要
- 每条规则结果
- affected count
- 失败原因
- 受控问题样例/分页 finding
- RuleSet version/hash
- Evidence/Audit

## 6. HQD-R1 #137

证明“为什么有权”。

~~~text
DataResource
→ RightsDeclaration
→ AuthorizationProvenanceBinding
→ Authorization
→ RightsSnapshot
→ Effective Rights
~~~

平台 owner 不等于法律 RIGHTS_HOLDER。

第一阶段动作至少覆盖：

- USE
- PROCESS
- DERIVE
- SHARE
- RAW_EXPORT
- RESALE
- AI_TRAINING

衍生数据默认 fail closed。

#137 第一阶段最小完成合同：

- 持久化 `RightsDeclaration` 与独立的 append-only `RightsVerification` 事实；Declaration 创建不等于 VERIFIED；
- 显式 `CreateRightsDeclaration` / `VerifyRightsDeclaration` / `RejectRightsDeclaration` Command；
- 同一 RightsDeclaration 只能有一个 terminal RightsVerification outcome（VERIFIED / REJECTED）；Verify/Reject 互斥，数据库约束禁止同一 declaration 同时出现两个 terminal outcomes；
- verification/rejection 创建后不可 UPDATE/DELETE；错误 VERIFIED 通过 RightsDisposition INVALIDATED/SUPERSEDED 退出 current set，并创建新的 RightsDeclaration + verification 修正；不得在同一 declaration 上追加 REJECTED 覆盖 VERIFIED；
- 未 VERIFIED 或已 REJECTED declaration 不得进入 AuthorizationProvenanceBinding、CurrentEntitlementGate、RightsSnapshot 或 Certification 权利依据；
- declaration create/verify/reject 分别产生 `RightsDeclarationCreated` / `RightsDeclarationVerified` / `RightsDeclarationRejected`（或实现固定的等价事件），并与 Audit/Evidence/Outbox 保持一致事务和幂等语义；
- 至少测试：未验证声明 fail closed、VERIFY 后可参与后续 binding、REJECT 后不可参与、Verify/Reject 并发只能产生一个 terminal outcome、VERIFIED declaration 后续不能追加 REJECTED 翻转结论、历史 verification 不被后续修改；
- append-only `RightsDisposition`，至少支持 `INVALIDATED` / `SUPERSEDED` + `effective_at` + reason + Evidence + actor；
- 显式 `InvalidateRightsDeclaration` / `SupersedeRightsDeclaration` Command，禁止 UPDATE 已 VERIFIED 历史事实；
- Current rights selection 按 `as_of` 排除已生效 disposition，并校验每条 declaration 自身 validity window 与 resource/consumer/purpose/action/scope；
- `BindAuthorizationProvenance`（或等价显式 Command），禁止 ad hoc CRUD 创建安全关键 binding；
- Authorization.grantor_ref 与支持它的 RightsDeclaration / 可验证 delegation chain 的强类型关系；
- AuthorizationProvenanceBinding 创建后不可 UPDATE/DELETE；
- append-only `AuthorizationProvenanceBindingDisposition`，至少支持 `INVALIDATED` / `SUPERSEDED` + `effective_at` + reason + Evidence + actor + optional superseded_by_binding_id；
- 显式 `InvalidateAuthorizationProvenanceBinding` / `SupersedeAuthorizationProvenanceBinding` Command；
- Current binding selection 按 `as_of` 排除已生效 binding disposition；replacement binding 必须独立通过 grantor/resource/actions/scope/declaration-current-validity 校验，不能自动继承有效性；
- RightsSnapshot 冻结实际使用的 declaration + AuthorizationProvenanceBinding + Authorization IDs，保证历史解释不随 current facts 变化；
- unrelated grantor 反例：资源/action 相同但无有效 provenance binding 时 CurrentEntitlementGate 必须 BLOCKED；
- Authorization context mismatch 反例：declaration 允许 consumer B / SHARE，但绑定 Authorization 只授予 consumer A / USE 时，B 的 SHARE 请求必须 BLOCKED；Authorization 的 grantee/consumer、resource、purpose、action、scope 必须逐项覆盖 requested context；
- disposed/expired declaration 反例：即使 Authorization 仍 ACTIVE，CurrentEntitlementGate 仍必须 BLOCKED；
- RightsDeclarationInvalidated / RightsDeclarationSuperseded / AuthorizationProvenanceBound / AuthorizationProvenanceBindingInvalidated / AuthorizationProvenanceBindingSuperseded 等 Domain Event + Audit/Evidence/Outbox/routing obligation。

Rights verification / invalidation / supersession / provenance binding 等实际人工或外部核验活动必须在发生时记录 CostEvent；这些活动通常没有 Execution，必须通过 typed CostAllocation 关联实际 Rights 业务事实，并使用稳定 activity_id/component_key 防止重试重复记账。

缺少 declaration creation/verification lifecycle、withdrawal/current-selection、provenance binding 任一能力时，#137 不视为完成。

## 7. HQD-4 #134

CertificationProfile + DatasetCertification + CertificationDisposition。

认证回答：

> DatasetVersion X 是否满足 Profile Y？

CertificationProfile 可以要求：

- specific quality dimensions / critical rules
- Rights actions / purpose
- Compliance
- Contract
- Traceability / Evidence

任何 required 条件缺失时不允许 CERTIFIED。

#134 第一阶段最小完成合同：

- 持久化不可变 `CertificationProfile` version/snapshot/hash；Profile 后续变化不得改变历史 Certification 的解释；
- 显式 `EvaluateDatasetCertification`（或等价 Certify Command），禁止 generic PATCH certification status；
- 每次评估明确绑定并冻结：workspace、DatasetVersion、CertificationProfile snapshot/version/hash、QualityAssessment、RightsSnapshot / Effective Rights、required ComplianceResult、required ContractVersion、Evidence/EvidenceSnapshot；
- required quality/rights/compliance/contract/traceability/evidence 任一缺失或不匹配必须 fail closed 为 REJECTED/阻断，不能产生 CERTIFIED；
- DatasetCertification 是不可变评估事实，decision 至少明确 CERTIFIED / REJECTED；DatasetVersion V2 不继承 V1 Certification；
- 评估结果可在 Profile/规则文件后续变化后重放解释，不能读取当前文件伪造历史；
- `EvaluateDatasetCertification` 幂等重放不得重复产生 Certification / event / CostEvent；
- append-only `CertificationDisposition`；
- `RevokeDatasetCertification` / `SupersedeDatasetCertification`；
- CurrentCertificationGate 按 as_of 排除已生效 REVOKED / SUPERSEDED；
- C1=CERTIFIED 被 C2=REJECTED supersede 后，C1/C2 历史均保留，但当前交付不得继续使用 C1；
- 不允许以 latest created_at 推断当前认证；
- 认证创建/拒绝/撤销/取代必须产生明确 Domain Event：
  - `DatasetCertified`
  - `DatasetCertificationRejected`
  - `DatasetCertificationRevoked`
  - `DatasetCertificationSuperseded`
- Certification / CertificationDisposition + Audit/Evidence + Outbox 在数据库事务内一致提交；
- 每个 certification event_type 显式进入 routing table，声明 required handlers 或 retention-only；
- 幂等重放不重复产生认证事实、事件或 CostEvent。

缺少 Profile snapshot、certification evaluation/create path、fail-closed input binding、CertificationDisposition、withdrawal/current-selection 或 certification outcome events 任一项时，#134 不视为完成。

认证评估/人工审批若产生实际成本，必须记录 CostEvent，并通过 typed CostAllocation 关联 DatasetCertification / CertificationDisposition，使用稳定 activity_id/component_key 保证重试幂等。

## 8. HQD-5 #135

从 DatasetVersion 页面理解和操作质量/认证，并提供第一阶段真正的 server-side delivery path。

至少展示：

- Quality Assessment
- Quality Report
- Rights summary
- historical Certification status / issued_at
- Current Delivery Eligibility（ALLOWED / BLOCKED）
- current rights blockers
- Evidence

DatasetVersion V1 认证不能让 V2 自动显示已认证。

#135 不能只交付 UI / eligibility query，还必须实现以下最小完成合同：

- 持久化 `DeliveryOperation`（每次交付尝试的稳定业务 ID / 幂等主体）；
- `DeliverDatasetVersion` / `IssueDatasetAccess`（最终命名由实现 PR 固定）；
- server-side delivery command 在返回数据或签发 URL/token/credential 前重新执行完整 CurrentDeliveryGate；
- query→delivery 之间 Rights/Certification/DatasetVersion 状态变化的 TOCTOU 测试；
- gate 失败不得产生可用数据、URL、token、credential；
- credential TTL 受 validity / future-effective RightsDisposition / AuthorizationProvenanceBindingDisposition / CertificationDisposition 边界约束；
- `DatasetDeliveryIssued` / `DatasetDeliveryBlocked` / `DatasetDeliveryFailed`（或实现固定的等价事件）覆盖三个终态结果；
- DeliveryOperation 的**数据库 terminal fact** + Audit/Evidence + Outbox + CostEvent（如有）保持一致事务/幂等语义；外部 credential provider 调用不属于 PostgreSQL transaction；
- 外部 issuance 必须先 durable persist PREPARED/ISSUANCE_PENDING + stable provider_request_key；
- **每一次 initial / retry / reconciliation 真正调用 provider 前，都重新执行完整 CurrentDeliveryGate 并重新计算 credential expiry cap**；PREPARED/ISSUANCE_PENDING 中旧 gate snapshot 只用于审计；
- provider 返回/恢复 capability 后，terminal ISSUED transaction 必须获取与 Rights/Binding/Certification disposition、DatasetVersion invalidation 等 Command 共享的 delivery authorization fence/revision，再次 re-gate + fresh-cap；terminal commit 是 issuance linearization point；
- 如果 re-gate 已 BLOCKED：
  - 确认此前未产生 provider access capability 时可直接 BLOCKED；
  - 若既有 provider_request_key 可能已签发，必须先 reconcile；
  - recovered credential/access 必须先 revoke/contain，确认失效后才能 BLOCKED；
  - unknown outcome 或 containment 未确认成功时进入 CONTAINMENT_PENDING，不能发 terminal Blocked event；
- direct bearer provider 必须支持基于同一 provider_request_key replay/read-after-write 恢复同一 credential（或等价同一访问能力）；仅支持 revoke/compensation 但不能恢复原 bearer secret 不足以支持 direct bearer；
- 无法恢复同一 credential 的 provider 必须使用平台 redemption indirection，或明确 unsupported；
- provider 成功但 terminal DB commit 前 crash 时，retry/reconciliation 必须复用同一 provider_request_key，不得签发第二份独立 credential；
- 首次返回或 recovered credential 在 ISSUED 前必须验证实际 provider expiry/access bound <= 当前 fresh cap；future-effective disposition 若把 cap 缩短到旧 credential expiry 之前，旧 credential 不得直接恢复为 ISSUED；
- recovered credential 超出 fresh cap 时必须安全 shorten+verify，或 revoke/contain；无法满足 fresh cap 时当前 operation 不得成功；
- ISSUANCE_PENDING / CONTAINMENT_PENDING 必须有 reconciliation path 和告警/恢复机制；
- CONTAINMENT_PENDING confirmed containment 后允许两种终结：fresh gate 已 BLOCKED → BLOCKED；fresh gate 仍 ALLOWED 但 credential/issuance contract 无法满足（如无法缩短到 fresh cap）→ FAILED；
- 每个 delivery event_type 显式进入 routing table，声明 required handlers 或 retention-only；
- event/Audit/Evidence payload 不得包含可用 credential secret；
- delivery CostEvent 必须通过 typed CostAllocation FK 关联 DeliveryOperation。

缺少 server-side gate-at-issuance、DeliveryOperation 或 terminal delivery events 任一项时，#135 不视为完成。

## 9. HQD-6 #136

使用 enterprise / lease / energy Reference Implementation 完成纵向验收。

必须回答：

1. 数据从哪里来？
2. 做了哪些加工？
3. 实体为什么这样对齐？
4. 使用了哪版规则？
5. 质量为什么通过/失败？
6. 哪些记录存在问题？
7. 为什么有权使用/交付？
8. 为什么可以被认证？

同时验证成功与失败路径。

第一阶段还必须验证：

- DatasetVersion 已经 CERTIFIED 后，如果对应 Authorization 过期/撤销、RightsDeclaration 被显式 INVALIDATED/SUPERSEDED，或 SHARE/RAW_EXPORT 等本次交付动作不再允许，历史 Certification 仍可查询，但 CurrentDeliveryGate 中的 CurrentEntitlementGate 必须阻止实际交付；
- 当前有效 AuthorizationProvenanceBinding 被显式 INVALIDATED/SUPERSEDED，而其 RightsDeclaration 仍 VERIFIED、Authorization 仍 ACTIVE 时，历史 RightsSnapshot / Certification 继续解释旧 binding，但 CurrentEntitlementGate 必须排除该 binding 并 BLOCKED；
- provider issuance 已成功但 terminal DB commit 丢失，随后 fresh gate 变为 BLOCKED 时，必须先 reconciliation 既有 provider_request_key 并 revoke/contain 已恢复的 access capability；未确认 containment 时保持 CONTAINMENT_PENDING，不得直接宣称 BLOCKED；
- DatasetVersion 已经 CERTIFIED 后若状态变为 INVALID，历史 Certification 仍保留，但 CurrentDeliveryGate 必须 BLOCKED；
- C1=CERTIFIED 后如果纠错生成 C2=REJECTED，并通过 CertificationDisposition 显式 SUPERSEDE C1，历史 C1/C2 都保留，但 CurrentCertificationGate 必须阻止继续使用 C1；
- Pilot 汇总的成本来自实际 CostEvent + typed CostAllocation，不允许仅在验收报告中事后估算重建，也不得仅从 JSONB metadata 猜业务归属；
- eligibility query 只做展示/预检；真实 delivery command 在签发数据/URL/token/credential 前重新执行 CurrentDeliveryGate，必须覆盖 query 后状态变化的 TOCTOU 场景。

## 10. 试点 KPI

第一轮优先统计：

- 原始记录数
- 标准化成功率
- 自动实体匹配率
- 人工复核率
- Quality rule pass/fail 数
- affected records
- 最终认证状态
- Evidence 覆盖
- 处理耗时
- 人工工时（来自实际 CostEvent / activity records，真实试点时）

商业验证重点是减少人工和交付周期，而不是先追求高 QPS。

## 11. 第一阶段明确非目标

不把以下事项作为 #129 第一阶段前置：

- T4/T5/T6 全套可靠性路线
- 完整 IAM / 灾备 / 性能平台
- Billing / Settlement
- 数据市场
- 完整法律合同管理
- Label Studio / X-AnyLabeling
- Gold / Benchmark Dataset
- 复杂质量漂移平台

这些根据真实试点反馈决定优先级。

## 12. 第一阶段完成定义

#136 验收通过后，Certified Dataset MVP 第一阶段完成。

是否进入 AI 标注数据集第二阶段，应基于真实试点反馈，而不是因为技术路线图中存在该能力。
