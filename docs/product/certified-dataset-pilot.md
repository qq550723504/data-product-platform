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

#131 当前 follow-up 已由 migration 000022 与质量评测事务补齐 typed CostAllocation / physical-attempt cost identity；继续复用 `quality_result` / `quality_finding`，不重复设计或迁移 QualityAssessment root。

必须固定：

- DatasetVersion
- RuleSet ref/version
- RuleSet content hash / snapshot
- evaluator identity
- metrics / findings
- decision
- Evidence / Audit
- CostEvent（实际发生的 engine invocation / compute / human-review quantity 或金额；金额未知时不得伪造，可记录 quantity/unit）
- typed CostAllocation → QualityAssessment or QualityAssessmentAttempt

成本幂等必须区分 **same-attempt replay** 与 **new execution attempt**：

- 同一次物理评测 attempt 的网络重放、command replay 或 transaction retry，如果没有再次发生 engine/compute/human-review 外部工作，不得重复记 CostEvent；
- 评测开始前必须 durable persist attempt identity/start fact，并先记录归属于该 attempt 的 CostEvent；成功或失败都必须追加 immutable outcome，成功 outcome 再把该 attempt 的成本查询关联到 QualityAssessment；
- 每个可能产生实际成本的物理 attempt 必须有稳定 `assessment_attempt_id` / activity identity（或等价强类型 attempt identity）；同一 attempt 内使用 `(attempt_identity, component_key/cost_type)` 由 PostgreSQL 唯一约束去重；
- failed/transient attempt 之后若真正再次调用 engine、再次消耗 compute 或再次发生人工 review，这是新的实际 activity，必须分配新的 attempt identity 并追加对应 CostEvent；不得因为属于同一个 QualityAssessment / 同一个顶层 idempotency key 就吞掉第二次真实成本；
- 若实现选择聚合而不是逐 attempt CostEvent，也必须原子累加实际 quantity/amount，并保留可审计 attempt count/identity，能够证明每次真实工作都被计入；不能仅保留第一次成本。

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
- RightsDeclaration 的 resource、consumer applicability/consumer_ref、purpose、action、scope、validity 必须强类型/规范化持久化并可索引查询；CurrentEntitlementGate 不得依赖任意 JSONB 解析这些核心维度；
- RightsDeclaration 必须把**使用许可**与**授予第三方的 grant authority**分开：`allowed_actions/permitted purpose/use scope` 不等于 `grant_authority_mode + grantable_actions + grantable purpose/scope`。只有 use permission、无 grant authority 的 declaration 不得支持 AuthorizationProvenanceBinding；
- Authorization / authorization_resource 的 gate-critical scope 也必须强类型/规范化、可索引查询：至少固定 `scope_type` + `scope_ref`（或等价 normalized relation）。现有 `authorization_resource.scope jsonb` 只能保存受控扩展参数，不能作为 CurrentEntitlementGate / BindAuthorizationProvenance 的唯一 scope identity；legacy row 无可验证 normalized scope 时 fail closed，迁移不得猜测宽 scope；
- 显式 `CreateRightsDeclaration` / `VerifyRightsDeclaration` / `RejectRightsDeclaration` Command；
- 同一 RightsDeclaration 只能有一个 terminal RightsVerification outcome（VERIFIED / REJECTED）；Verify/Reject 互斥，数据库约束禁止同一 declaration 同时出现两个 terminal outcomes；
- verification/rejection 创建后不可 UPDATE/DELETE；错误 VERIFIED 通过 RightsDisposition INVALIDATED/SUPERSEDED 退出 current set，并创建新的 RightsDeclaration + verification 修正；不得在同一 declaration 上追加 REJECTED 覆盖 VERIFIED；
- 未 VERIFIED 或已 REJECTED declaration 不得进入 AuthorizationProvenanceBinding、CurrentEntitlementGate、RightsSnapshot 或 Certification 权利依据；
- declaration create/verify/reject 分别产生 `RightsDeclarationCreated` / `RightsDeclarationVerified` / `RightsDeclarationRejected`（或实现固定的等价事件），并与 Audit/Evidence/Outbox 保持一致事务和幂等语义；
- 至少测试：未验证声明 fail closed、VERIFY 后可参与后续 binding、REJECT 后不可参与、Verify/Reject 并发只能产生一个 terminal outcome、VERIFIED declaration 后续不能追加 REJECTED 翻转结论、历史 verification 不被后续修改；
- append-only `RightsDisposition`，至少支持 `INVALIDATED` / `SUPERSEDED` + `effective_at` + reason + Evidence + actor；
- 显式 `InvalidateRightsDeclaration` / `SupersedeRightsDeclaration` Command，禁止 UPDATE 已 VERIFIED 历史事实；
- Current rights selection 区分 DIRECT_USE 与 DOWNSTREAM_AUTHORIZATION：DIRECT_USE 校验 declaration 的 consumer/permitted-purpose/allowed-action/use-scope；DOWNSTREAM_AUTHORIZATION 校验 declaration current provenance + grant authority、binding/delegation 的 grantable purpose/action/scope 与 Authorization requested context，**不得要求 grantor 自己的 allowed/use scope 匹配 grantee request**；两条路径都按 `as_of` 排除已生效 disposition；
- `BindAuthorizationProvenance`（或等价显式 Command），禁止 ad hoc CRUD 创建安全关键 binding；
- Authorization.grantor_ref 与支持它的 RightsDeclaration / grantor-authority delegation chain 的强类型关系；DELEGATED binding 必须持久化 chain ID/hash + ordered member edge identities，不能只在创建时临时证明存在 delegation；chain 若 DRAFT→FINALIZED，member mutation 与 Finalize 必须共享 parent chain row lock/fence（parent-first），Finalize 持锁校验 ordered members/hash，禁止 finalize 后 late member commit；
- `BindAuthorizationProvenance` 必须证明 declaration 的 **grantable** actions/purpose/scope 覆盖 Authorization 授出的范围；delegation chain 每一跳也必须具有 onward grant authority。`allowed USE` 但 `grantable USE` 为空/禁止时，不能创建支持第三方 USE grant 的 binding；
- AuthorizationProvenanceBinding 创建后不可 UPDATE/DELETE；
- append-only `AuthorizationProvenanceBindingDisposition`，至少支持 `INVALIDATED` / `SUPERSEDED` + `effective_at` + reason + Evidence + actor + optional superseded_by_binding_id；
- 显式 `InvalidateAuthorizationProvenanceBinding` / `SupersedeAuthorizationProvenanceBinding` Command；
- Current binding selection 按 `as_of` 排除已生效 binding disposition；replacement binding 必须独立通过 grantor/resource/actions/scope/declaration-current-validity 校验，不能自动继承有效性；DELEGATED binding 每次 CurrentEntitlementGate 还必须重新验证所有 grantor delegation edges 的 current validity/disposition/continuity/coverage，不能复用 binding-create-time 结论；
- RightsSnapshot 冻结实际使用的 declaration + AuthorizationProvenanceBinding + Authorization IDs；若 grantor authority=DELEGATED，还冻结 grantor delegation chain ID/hash + member edge IDs。snapshot header + 所有 membership rows 一起 immutable；**若采用 DRAFT→FINALIZED，多事务 membership mutation 与 Finalize 必须共享同一个 parent snapshot row lock/fence（parent-first 固定顺序），Finalize 在持锁下验证 membership/hash 后提交；禁止 membership transaction 先看到 DRAFT、Finalize 先提交、membership 后提交的穿越。** FINALIZED 后 membership INSERT/UPDATE/DELETE 必须被 PostgreSQL guard 拒绝；历史 freeze 不替代 delivery-time current chain validation；
- 持久化不可变 `EffectiveRightsSnapshot`（或等价强类型 aggregate），绑定明确 target DatasetVersion、计算 `as_of`/context、calculation_rule_version/hash、lineage/input-set hash，并通过 membership rows 冻结所有**必要输入** DatasetVersion/DataResource + 对应 RightsSnapshot/provenance refs；DRAFT membership/action mutation 与 Finalize 使用同一 parent snapshot lock/fence，Finalize 持锁校验 required-input set/hash 后提交；FINALIZED 后 header/membership/action decision 全部不可改写；
- Effective Rights 计算必须对每个必要输入执行确定性 fail-closed 合成：对 USE/PROCESS/DERIVE/SHARE/RAW_EXPORT/RESALE/AI_TRAINING 等 action，只有所有必要输入都明确 ALLOWED 才允许输出；任一输入 NOT_ALLOWED、UNKNOWN、缺失 rights fact/snapshot 或未出现在冻结 lineage membership 中，输出该 action 均 NOT_ALLOWED。限制项按最严格约束合成；
- 历史 EffectiveRightsSnapshot 只用于认证时解释。对衍生 DatasetVersion 的每次 CurrentDeliveryGate，必须从 target 的 immutable required lineage/input membership 遍历所有 source inputs，并重新验证各 input 的 current declaration/binding/Authorization/grantor-delegation facts，再对 requested action 做 fail-closed 交集；任一 source 在认证后被 revoke/invalidate/expire，derived delivery 立即 BLOCKED；
- `EffectiveRightsSnapshot` 逐 action 持久化 decision + reason/source membership，可查询解释“哪一个输入阻断了 SHARE/RESALE 等动作”；创建/finalize 产生 `EffectiveRightsCalculated` / `EffectiveRightsFinalized`（或实现固定的等价事件）+ Audit/Evidence/Outbox；#134 只能冻结引用 finalized immutable Effective Rights fact，不能认证时临时重新计算后不留事实；
- unrelated grantor 反例：资源/action 相同但无有效 provenance binding 时 CurrentEntitlementGate 必须 BLOCKED；
- delegated grantor 反例：binding 创建时 grantor delegation chain 有效，随后任一上游 edge 过期或 REVOKED/INVALIDATED/SUPERSEDED；即使 binding/declaration/Authorization 本身仍 current，CurrentEntitlementGate 必须 BLOCKED，且后续 credential fresh cap 不得越过 delegation edge 的 valid_to/disposition effective_at；
- Authorization context mismatch 反例：declaration 允许 consumer B / SHARE，但绑定 Authorization 只授予 consumer A / USE 时，B 的 SHARE 请求必须 BLOCKED；Authorization 的 grantee/consumer、resource、purpose、action、scope 必须逐项覆盖 requested context；
- downstream grant 正例：party A 的 `allowed_actions/use scope` 不包含 SHARE，但 declaration 明确 `grantable_actions=[SHARE]`、grantable purpose/scope 覆盖 A→B Authorization；若 binding/delegation/Authorization/current facts 全部有效，consumer B 的 SHARE 不能因为 A 自己不能 SHARE 而被错误 BLOCKED；
- Authorization normalized-scope 反例：`authorization_resource.scope` JSONB 看似包含允许前缀，但 normalized `scope_type/scope_ref` 缺失或与请求不匹配时，BindAuthorizationProvenance / CurrentEntitlementGate 必须 fail closed；不能由不同代码路径各自解释 JSONB；
- Effective Rights 多输入反例：CURATED output 必须绑定至少两个/三个 required inputs；其中一个输入明确禁止 SHARE（其它输入允许）时，finalized EffectiveRightsSnapshot.SHARE=NOT_ALLOWED，并能追溯到该输入。删除/漏掉该 required input membership 必须使计算失败，不能得到更宽结果；
- disposed/expired declaration 反例：即使 Authorization 仍 ACTIVE，CurrentEntitlementGate 仍必须 BLOCKED；
- RightsDeclarationInvalidated / RightsDeclarationSuperseded / AuthorizationProvenanceBound / AuthorizationProvenanceBindingInvalidated / AuthorizationProvenanceBindingSuperseded 等 Domain Event + Audit/Evidence/Outbox/routing obligation。

Rights verification / invalidation / supersession / provenance binding 等实际人工或外部核验活动必须在发生时记录 CostEvent；这些活动通常没有 Execution，必须通过 typed CostAllocation 关联实际 Rights 业务事实。same-attempt replay 使用稳定 activity/attempt identity + component_key 去重；若 retry 真正再次发生外部核验/人工工作，则使用新的 attempt identity 记录新增实际成本，不能按顶层业务对象全部去重。

缺少 declaration creation/verification lifecycle、withdrawal/current-selection、**use permission vs grant authority 分离**、**Authorization normalized scope**、provenance binding、**delegated grantor chain 强类型引用 + current revalidation/disposition**、**lineage-bound persisted Effective Rights computation/finalization + delivery-time source recomputation**、**snapshot finalize/membership serialization** 任一能力时，#137 不视为完成。

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
- purpose/action/consumer/delivery channel 四个 delivery-context 维度必须显式 ANY / EXPLICIT；EXPLICIT 时冻结对应强类型 membership；缺失/NULL/UNKNOWN 不得被解释成 ANY；
- 显式 `EvaluateDatasetCertification`（或等价 Certify Command），禁止 generic PATCH certification status；
- 每次评估明确绑定并冻结：workspace、DatasetVersion、CertificationProfile snapshot/version/hash、QualityAssessment、finalized immutable RightsSnapshot、**finalized EffectiveRightsSnapshot identity/hash（Rights required 时）**、required ComplianceResult、required ContractVersion、Evidence/EvidenceSnapshot；Effective Rights snapshot 的 required-input membership/input-set hash 必须与 target DatasetVersion 实际 lineage 一致；
- Certification 必须验证 frozen rights evidence context 覆盖 Profile rights applicability：consumer/purpose/action/normalized scope 逐维做“Profile required set ⊆ frozen rights coverage”。Profile 某维度=ANY 时，只能由 rights evidence 显式 ANY/universal coverage 支撑；consumer A / RESEARCH / narrow scope 的 snapshot 不得用于 consumer B、COMMERCIAL 或 ANY profile；
- required quality/rights/compliance/contract/traceability/evidence 任一缺失或不匹配必须 fail closed 为 REJECTED/阻断，不能产生 CERTIFIED；
- CurrentCertificationGate 对 purpose/action/consumer/delivery channel 任一维度无法从 frozen Profile snapshot 明确证明覆盖时必须 BLOCKED；只有显式 ANY 才代表该维度不限；
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

认证评估/人工审批若产生实际成本，必须记录 CostEvent，并通过 typed CostAllocation 关联 DatasetCertification / CertificationDisposition。稳定 activity/attempt identity + component_key 只用于 same-attempt replay 幂等；新的真实评估/审批 attempt 若再次产生费用，必须追加成本或原子聚合新增 quantity。

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
- server-side delivery command 在进入 CurrentDeliveryGate 前先从可信 authenticated caller principal 解析 effective consumer/workspace；on-behalf-of 必须验证当前有效 delegation，不能信任客户端 header/query/body/demo actor ID 自证 consumer；
- initial / retry / reconciliation / terminal finalize 都必须重新验证 principal→consumer/workspace binding/delegation 当前有效性；这些可撤销身份依赖必须进入与 Rights/Certification/DatasetVersion 共享的 delivery authorization fence/revision（或等价串行化机制）；
- 每一次 initial / retry / reconciliation / terminal-finalize / credential-replay gate evaluation 都必须追加 immutable DeliveryGateEvaluation（或等价 child fact），保存 decision/blockers + dependency fence/revision + trusted caller/effective consumer/context；DeliveryOperation.current_gate/status 只是 projection，不得覆盖旧 evaluation；
- server-side delivery command 在返回数据或签发 URL/token/credential 前，使用该 trusted principal + effective consumer 上下文重新执行完整 CurrentDeliveryGate；
- query→delivery 之间 Rights/Certification/DatasetVersion 状态变化，以及 principal binding/delegation revoke 的 TOCTOU 测试；
- gate 失败不得产生可用数据、URL、token、credential；
- credential `expires_at` 必须满足完整最小上限：`<= min(requested_expires_at, platform_max_credential_expiry, caller principal→consumer/workspace binding / workspace membership / caller delegation 的最早有限 valid_to/expires_at, 当前 entitlement 实际依赖的 grantor-authority delegation chain 所有 required edges 的最早 valid_to, trusted identity source 已知 caller future revoke/disable effective_at（如可表达）, grantor delegation future disposition effective_at, RightsDeclaration effective_to, Authorization valid_to, future-effective RightsDisposition / AuthorizationProvenanceBindingDisposition / CertificationDisposition effective_at)`；不可回调 bearer/presigned credential 不得越过 delegated grantor authority 本身的有限边界；
- #135 issuance 测试必须分别覆盖：① caller 请求 5 分钟而 platform/identity/rights 均允许 1 小时，实际 credential <= 5 分钟；② caller 请求 1 小时但 platform max=10 分钟，实际 credential <= 10 分钟；③ identity/rights/disposition 更早时继续取最早边界。replay/reconciliation 的 fresh cap 必须重复应用同一完整 min 公式；
- `DatasetDeliveryIssued` / `DatasetDeliveryBlocked` / `DatasetDeliveryFailed`（或实现固定的等价事件）覆盖三个终态结果；
- `CONTAINMENT_PENDING` 虽非终态，但每次首次进入必须产生显式 `DatasetDeliveryContainmentPending`（或固定等价）Domain Event，并与该 transition 的 Audit/Evidence/Outbox 同事务、幂等提交；reconciliation/alert consumers 不得依赖轮询状态或普通日志才知道存在未确认外部 capability；credential replay containment pending 使用独立 replay subject/event（或统一 containment event + subject_kind），不改写原 ISSUED operation；
- DeliveryOperation 的**数据库 terminal fact** + Audit/Evidence + Outbox + CostEvent（如有）保持一致事务/幂等语义；外部 credential provider 调用不属于 PostgreSQL transaction；
- 外部 issuance 必须先 durable persist PREPARED/ISSUANCE_PENDING + stable provider_request_key；
- **每一次 initial / retry / reconciliation 真正调用 provider 前，都重新验证 caller principal→effective consumer/workspace binding/delegation 当前有效性，再重新执行完整 CurrentDeliveryGate 并重新计算 credential expiry cap**；PREPARED/ISSUANCE_PENDING 中旧 identity/gate snapshot 只用于审计；
- 所有 delivery mode 的 terminal ISSUED transaction 必须获取与 caller identity lifecycle、grantor-authority delegation edge/disposition、Rights/Binding/Certification disposition、DatasetVersion invalidation 等 Command 共享的 delivery authorization fence/revision，再次 re-gate；provider/credential 模式同时 fresh-cap；terminal commit 是 delivery linearization point；
- direct-data 模式必须在 terminal ISSUED commit 成功前保持 response body=0 bytes；commit 后才允许写第一字节，且不能为整个 stream 长时间持有 DB fence/lock；
- direct-data terminal `ISSUED` 只表示该 attempt 已在线性化点获准开始响应，不证明客户端已收到数据；如果 ISSUED commit 后、第一字节前或 streaming 中发生 response loss，同一 idempotency key 只能返回稳定 non-payload replay result（例如 `DIRECT_DATA_REPLAY_REQUIRES_NEW_ATTEMPT` + 原 operation identity），不得依据旧 gate 重放 DatasetVersion bytes；
- direct-data 需要重新传输时必须创建新的显式 DeliveryOperation/attempt（新 idempotency key，可用 `retry_of_delivery_operation_id` 关联原 attempt），重新解析 trusted principal/effective consumer/delegation、重新执行 CurrentDeliveryGate、重新进入 terminal fence；期间任何 Rights/Certification/Authorization/DatasetVersion/principal-binding/delegation 失效都必须使新 attempt fail closed；
- crash/fault test 必须覆盖“ISSUED commit 成功、第一字节前 crash”：same-key retry 断言 0 dataset bytes + stable replay-required 结果；以及新 attempt 在 crash 后 entitlement/identity 被撤销时 BLOCKED、仍有效时仅在新的 ISSUED commit 后开始输出；
- **真实 PostgreSQL 并发测试是 #135 完成条件，不允许只用 mock/串行调用证明 fence**：
  - 使用两个独立 transaction/connection + barrier，把竞争窗口固定在“delivery terminal finalize 已进入 fenced re-gate/准备提交 ISSUED”和“gate-changing Command 准备提交 disposition/invalidation”之间；
  - entitlement-change-first：Authorization revoke（并至少再覆盖 Rights/Binding/Certification disposition 或 DatasetVersion INVALID 中一种）先在线性化 fence 上提交，delivery finalize 随后必须观察新 revision/current facts，不能 ISSUED；provider capability 进入 contain/block/fail，direct-data 必须断言 response body 仍为 0 bytes；
  - caller-binding-change-first：principal→consumer/workspace binding / delegation revoke 先在线性化 fence 上提交，delivery finalize 必须观察新 revision 并 fail closed；不能因为入口身份检查曾通过而继续 ISSUED；
  - grantor-delegation-change-first：CurrentEntitlementGate 依赖的 grantor delegation edge/disposition 先在线性化 fence 上提交 revoke/expiry/supersede，delivery finalize 必须观察新 revision 并 fail closed；不能因为 AuthorizationProvenanceBinding 创建时 chain 曾有效而继续 ISSUED；
  - finalize-first：delivery terminal ISSUED 先在线性化 fence 上提交，随后 gate-changing Command 才完成；direct-data 只有在该 commit 之后才能放行第一字节；两者必须形成唯一全序，后续 Command 按 delivery-mode revocation semantics 处理已签发 capability；
  - 验证固定锁顺序/无 deadlock、重复 idempotency retry 不产生第二个 terminal fact/event；CostEvent 按实际 activity-attempt 语义处理：same-attempt replay 去重，但如果 retry/reconciliation 确实再次发生可计费 provider/compute 调用，则必须以新的稳定 attempt identity 记录新增实际成本（或原子聚合 quantity），不能被顶层 DeliveryOperation idempotency 吞掉；
- 如果 re-gate 已 BLOCKED：
  - 确认此前未产生 provider access capability 时可直接 BLOCKED；
  - 若既有 provider_request_key 可能已签发，必须先 reconcile；
  - recovered credential/access 必须先 revoke/contain，确认失效后才能 BLOCKED；
  - unknown outcome 或 containment 未确认成功时进入 CONTAINMENT_PENDING，不能发 terminal Blocked/Failed event；但必须在同一 transition transaction 发 `DatasetDeliveryContainmentPending` + Audit/Evidence/Outbox；
- direct bearer provider 必须支持基于同一 provider_request_key replay/read-after-write 恢复同一 credential（或等价同一访问能力），并支持 fresh credential-replay authorization 被拒绝时 revoke/contain 该既有 capability；任一能力缺失都不足以支持 direct bearer，必须 platform redemption/gateway 或 unsupported；
- 无法安全恢复同一 credential，或 replay 被当前授权拒绝后无法 revoke/contain 旧 capability 的 provider，必须使用 platform redemption/gateway，或明确 unsupported；
- provider 成功但 terminal DB commit 前 crash 时，retry/reconciliation 必须复用同一 provider_request_key，不得签发第二份独立 credential；
- terminal ISSUED commit 已成功但 credential HTTP response 丢失时，same-key retry 在再次返回同一 credential/handle 前必须 fresh caller→effective consumer/delegation resolution + shared fence + CurrentDeliveryGate + fresh cap + recovered capability verify，并 append non-secret replay decision；fresh ALLOWED 才返回。若期间 delegation/Rights/Certification/Authorization/DatasetVersion 已失效，same-key retry 必须 0 credential/secret 输出并 revoke/contain 原 capability；containment 未确认只返回 non-secret pending，原 ISSUED 历史不改写；
- 首次返回或 recovered credential 在 ISSUED 前必须 read-after-write/authoritative verify 实际 provider capability：expiry <= fresh cap，并且 resource/DatasetVersion、consumer/grantee enforcement、action、object/row/prefix scope、**delivery mode/channel enforcement** 均不得比 requested/current-gate context 更宽；consumer/grantee 或受约束 channel/mode 任一无法被 provider 原生或等价机制权威表达/验证/强制时，direct bearer/presigned 不得 ISSUED，只能 platform redemption/gateway 在使用时强制，或标记 unsupported；
- recovered/returned credential 超出 fresh cap **或 capability scope 过宽/不可验证**时必须安全 shorten/narrow+verify，或 revoke/contain；无法满足当前 context 时 operation 不得成功；
- ISSUANCE_PENDING / CONTAINMENT_PENDING 必须有 reconciliation path 和告警/恢复机制；
- CONTAINMENT_PENDING confirmed containment 后允许两种终结：fresh gate 已 BLOCKED → BLOCKED；fresh gate 仍 ALLOWED 但 credential/issuance contract 无法满足（如无法缩短到 fresh cap）→ FAILED；
- 每个 delivery event_type 显式进入 routing table，声明 required handlers 或 retention-only；
- event/Audit/Evidence payload 不得包含可用 credential secret；
- delivery CostEvent 必须通过 typed CostAllocation FK 关联 DeliveryOperation；每次真实 provider invocation 在调用前建立 durable immutable provider-attempt start fact，并按 attempt 记账。success / provider failure / timeout/unknown 后追加 outcome/observation fact；后续 reconciliation 对原 attempt 的新认知只能追加 resolution observation，不能覆盖原始 start/首次 observation。reconciliation / revoke / compensation 若真实调用 provider，则它们各自是新的 provider_attempt_id。只要实际外部调用并可能计费，都必须保留 CostEvent；same-attempt 无新调用 replay 才去重。

缺少 server-side gate-at-issuance、DeliveryOperation、terminal delivery events、共享 fence/revision，或上述真实 PostgreSQL 双顺序并发测试任一项时，#135 不视为完成。

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
