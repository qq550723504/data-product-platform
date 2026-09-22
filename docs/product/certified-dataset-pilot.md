# Certified Dataset Pilot 路线

## 1. 阶段定位

核心 POC 已完成。本路线是 POC 之后的受控试点，目标是把已经验证的数据生产内核组合成客户可验收的“高质量数据集”交付物。

主 Epic：#129（OPEN；第一阶段纵向验收完成后再关闭）。

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

第一阶段已经完成：

~~~text
#131 QualityAssessment / Cost identity            ✅
        ↓
#132 Quality Engine                               ✅
        ↓
#133 Quality Report                               ✅

#137 Data Rights Provenance / Effective Rights    ✅
        ↓
#134 DatasetCertification                         ✅
        ↓
#135A Read/API/UI                                 ✅
#135B trusted DIRECT_DATA                         ✅
        ↓
#136 enterprise-activity E2E Pilot                ✅ E2E1–E2E20
~~~

关键交付：

- #149 DatasetCertification / CertificationProfile；
- #152 Read/API/UI + Current Delivery Eligibility；
- #173 / #174 trusted server-side DIRECT_DATA；
- #176 / #178 / #179 / #180 Pilot reliability closeout；
- #182–#194 enterprise-activity vertical acceptance；
- docs/product/certified-dataset-pilot-acceptance.md 最终验收报告。

第一阶段不把 bearer/presigned provider、完整生产 IAM、provider containment/recovery、T4/T5/T6、灾备、性能 SLA 或 AI Gold Dataset 作为未完成项。

## 3. HQD-1 #131

QualityAssessment 核心已经通过 #140 / migration 000019 落地，继续复用 `quality_result` / `quality_finding` 作为兼容存储/API 名称。

#131 当前 follow-up 已由 migration 000022 与质量评测事务补齐 typed CostAllocation / physical-attempt cost identity；继续复用 `quality_result` / `quality_finding`，不重复设计或迁移 QualityAssessment root。质量 attempt 还必须带有 durable lease；重放或 reconciliation 在 lease 过期且没有 terminal outcome 时追加 FAILED outcome，不能永久停留在 IN_PROGRESS。

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

V1 QualityRuleSet 由 industry-pack 提供，Core 只识别有限的通用 `type`，不按行业字段或 rule id 写分支。规则必须显式声明 `id`、`dimension`、`type`、`severity`、`required`（且必须是非空布尔值）；需要时声明 `target`、`threshold`、`parameters` 和 `description`。`severity` 只接受 `CRITICAL`、`HIGH`、`WARNING`（大小写和外围空白会规范化）。`not_null`、`completeness_ratio`、`unique`、`duplicate_ratio` 的比例阈值必须在 `[0,1]`；`freshness`、`reference_match`、`reconciliation` 必须显式提供 threshold；`reference_match` 和 `reconciliation` 必须显式提供合法字符串 `operator`，未知或拼写错误的参数不能静默回退，metric/threshold 比较保留精确数字；每种 rule type 只接受其定义的 `parameters` 键，未知或拼写错误的键直接 fail closed；`range` 使用精确十进制比较，无法可靠表达的浮点边界拒绝加载；`range`、`enum`、`regex` 的 `parameters.allowNull` 若声明则必须是合法布尔值，不能用拼写错误静默回退。当前 native evaluator 支持：`not_null`、`completeness_ratio`、`unique`、`duplicate_ratio`、`range`、`enum`、`regex`、`freshness`、`reference_match`、`reconciliation`、`conditional_consistency`、`lineage_present`、`evidence_present`。

每条 finding 都在既有 `quality_finding.observed` 快照中记录统一的 `observedValue`、`threshold`、`affectedCount`、仅含行号/字段引用的受限 `sample` 与规则专属诊断字段；不得把原始数据单元格写入 finding、Evidence 或查询响应。六维摘要（Completeness、Accuracy、Consistency、Uniqueness、Timeliness、Traceability）随既有 QualityAssessment `metrics` 保存并通过查询 API 返回，查询必须返回该冻结摘要，不能按当前 evaluator 重新计算；没有可比较事实的可选规则（包括缺失 target/condition field）为 `NOT_APPLICABLE`，未知 rule type、缺失必要参数或缺失 required evidence 均 fail closed。

## 5. HQD-3 #133

提供可读 Quality Report。

当前报告查询路由为 `GET /api/v1/quality-assessments/{assessmentId}/report`，要求显式 `workspaceId`，并使用 `limit` / `offset` 返回受控、稳定排序的 finding page。报告读取已冻结的六维摘要和 RuleSet hash/evaluator provenance，不重新执行当前规则；Evidence 与 Audit 仅以最多 100 条 header/hash 引用字段返回，不加载 metadata、per-rule metrics 或完整 audit state。报告层将历史 `SKIPPED` 映射为 `NOT_APPLICABLE`，不改变 `quality_finding` 中的历史存储值。

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
- 每次评估明确绑定并冻结：workspace、DatasetVersion、CertificationProfile snapshot/version/hash、QualityAssessment、**finalized EffectiveRightsSnapshot identity/hash + frozen declaration/binding provenance（Rights required 时）**、required ComplianceResult、required ContractVersion、Evidence/EvidenceSnapshot；显式 RightsSnapshot 为可选加强证据，若提供则必须 finalized 且覆盖 EffectiveRights 冻结的 provenance；Effective Rights snapshot 的 required-input membership/input-set hash 必须与 target DatasetVersion 实际 lineage 一致；
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

HQD-5 第一阶段 MVP 已完成并关闭。

### HQD-5A — Read/API/UI

已交付：

- DatasetVersion QualityAssessment / Quality Report；
- 六维质量摘要与 findings；
- Certification history / profile / evidence；
- Rights summary；
- Current Delivery Eligibility：
  - DatasetVersion usability
  - Current Certification
  - Current Entitlement

历史 CERTIFIED 与“当前仍可交付”明确分离。

### HQD-5B — trusted DIRECT_DATA

已交付：

- trusted principal → effective consumer/workspace；
- fresh CurrentDeliveryGate；
- DeliveryOperation；
- commit-before-first-byte；
- BLOCKED = 0 bytes；
- same-key replay 不重放 payload；
- explicit replacement attempt + fresh re-gate；
- Audit / Evidence / Outbox / Cost；
- real PostgreSQL delivery-fence linearization。

### Deferred，不属于 HQD-5 第一阶段未完成项

- bearer / presigned credential provider；
- provider revoke / containment / recovery；
- credential expiry-cap / capability narrowing matrix；
- full enterprise IAM / delegation management；
- NDI / external publication protocol。

如真实产品需要这些能力，应从具体 Delivery Hardening / Productionization 需求单独立项，不重新打开 #135。

## 9. HQD-6 #136

enterprise / lease / energy Reference Implementation 已完成纵向验收，E2E1–E2E20 全部 PASS。

完整链路：

~~~text
RAW
→ Entity Resolution
→ STANDARDIZED
→ Native Execution
→ CURATED
→ QualityAssessment / Quality Report
→ Rights / Compliance / Contract
→ EffectiveRightsSnapshot
→ DatasetCertification
→ Current Delivery Eligibility
→ trusted caller
→ fresh CurrentDeliveryGate
→ DeliveryOperation
→ DIRECT_DATA
→ real UI / trace
~~~

关键失败路径已经覆盖：

- CRITICAL Quality FAIL；
- required Rights / Compliance / Contract missing；
- new DatasetVersion 无旧 Certification 继承；
- Certification revoke/supersede；
- Authorization revoke；
- DatasetVersion INVALID；
- profile context mismatch；
- Authorization/provenance context mismatch；
- principal spoofing；
- eligibility→delivery TOCTOU；
- DIRECT_DATA response-loss；
- revoke/finalize 双事务线性化；
- frozen EvidenceSnapshot / RightsSnapshot membership tamper。

最终真实 browser gate #194 / CI #998 使用：

- real Core / Worker；
- PostgreSQL / Redis / MinIO；
- standalone Next；
- Chromium。

并验证同一 immutable DatasetVersion 的：

- exact producing Execution；
- checksum / MinIO bytes；
- Quality / six dimensions；
- Certification history；
- EffectiveRights；
- EvidenceSnapshot；
- Current Delivery Eligibility 三个 ALLOWED gate；
- ProductRelease trace。

详细证据与已知限制见 docs/product/certified-dataset-pilot-acceptance.md。

## 10. 试点 KPI

第一阶段 Reference fixture 的可重复 KPI：

| 指标 | Pilot 结果 |
| --- | --- |
| RAW records | 30（enterprise 6 + lease 10 + energy 14） |
| enterprise 标准化 | 6 / 6，100% |
| 人工复核触达率 | 1 / 6，16.7% |
| 无人工复核路径 | 5 / 6，83.3% |
| Quality RuleSet | 11 rules / 6 dimensions |
| happy-path blocking Quality FAIL | 0，Gate PASS |
| targeted negative Quality FAIL | 1 个 CRITICAL `QA-FRESHNESS`，Gate FAIL |
| 最终 Certification | CERTIFIED |
| Current Delivery Eligibility | ALLOWED |
| trusted DIRECT_DATA | ISSUED |
| browser gate | PASS — #194 / CI #998 |

成本按真实 activity 记录：

- 1 × QUALITY_ENGINE_INVOCATION；
- 3 × RIGHTS_DECLARATION_VERIFICATION；
- 1 × EFFECTIVE_RIGHTS_COMPUTE；
- 1 × CERTIFICATION_EVALUATION（same-key replay 不重复）；
- 每个新 DeliveryOperation 1 × DIRECT_DATA_DELIVERY_ATTEMPT（same-key replay 不重复）。

以上数字只描述固定 synthetic fixture，不是生产 SLA、生产匹配率或商业 KPI 基准。

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

#136 E2E1–E2E20 已全部通过，Certified Dataset MVP 第一阶段完成。

完成证据：

- #136 acceptance checklist；
- #182–#194 merged Pilot slices；
- CI #998 final required gate；
- docs/product/certified-dataset-pilot-acceptance.md。

下一阶段不自动启动。应基于 Pilot 结果在 AI / Gold Dataset、Delivery Hardening、Core reliability debt 或真实客户/行业需求之间选择优先级。

**第一阶段完成不等于生产上线批准。**
