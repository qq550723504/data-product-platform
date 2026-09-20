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

#137 第一阶段必须同时实现：
- `BindAuthorizationProvenance`（或等价显式 Command），禁止 ad hoc CRUD 创建安全关键 binding；
- Authorization.grantor_ref 与支持它的 RightsDeclaration / 可验证 delegation chain 的强类型关系；
- unrelated grantor 反例：资源/action 相同但无有效 provenance binding 时 CurrentEntitlementGate 必须 BLOCKED。

Rights verification / invalidation / supersession / provenance binding 等实际人工或外部核验活动必须在发生时记录 CostEvent；这些活动通常没有 Execution，必须通过 typed CostAllocation 关联实际 Rights 业务事实，并使用稳定 activity_id/component_key 防止重试重复记账。

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

#134 第一阶段必须同时实现：
- append-only `CertificationDisposition`；
- `RevokeDatasetCertification` / `SupersedeDatasetCertification`；
- CurrentCertificationGate 按 as_of 排除已生效 REVOKED / SUPERSEDED；
- C1=CERTIFIED 被 C2=REJECTED supersede 后，C1/C2 历史均保留，但当前交付不得继续使用 C1；
- 不允许以 latest created_at 推断当前认证。

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

#135 不能只交付 UI / eligibility query，还必须实现：
- 持久化 `DeliveryOperation`（每次交付尝试的稳定业务 ID / 幂等主体）；
- `DeliverDatasetVersion` / `IssueDatasetAccess`（最终命名由实现 PR 固定）；
- server-side delivery command 在返回数据或签发 URL/token/credential 前重新执行完整 CurrentDeliveryGate；
- query→delivery 之间 Rights/Certification/DatasetVersion 状态变化的 TOCTOU 测试；
- gate 失败不得产生可用数据、URL、token、credential；
- credential TTL 受 validity / future-effective disposition 边界约束；
- delivery CostEvent 必须 typed allocate 到 DeliveryOperation。

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
