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
#131 QualityAssessment
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

演进现有 quality_result 为正式 QualityAssessment。

必须固定：

- DatasetVersion
- RuleSet ref/version
- RuleSet content hash / snapshot
- evaluator identity
- metrics / findings
- decision
- Evidence / Audit

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

## 7. HQD-4 #134

CertificationProfile + DatasetCertification。

认证回答：

> DatasetVersion X 是否满足 Profile Y？

CertificationProfile 可以要求：

- specific quality dimensions / critical rules
- Rights actions / purpose
- Compliance
- Contract
- Traceability / Evidence

任何 required 条件缺失时不允许 CERTIFIED。

## 8. HQD-5 #135

从 DatasetVersion 页面理解和操作质量/认证。

至少展示：

- Quality Assessment
- Quality Report
- Rights summary
- historical Certification status / issued_at
- Current Delivery Eligibility（ALLOWED / BLOCKED）
- current rights blockers
- Evidence

DatasetVersion V1 认证不能让 V2 自动显示已认证。

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

第一阶段还必须验证：DatasetVersion 已经 CERTIFIED 后，如果对应 Authorization 过期/撤销或 SHARE/RAW_EXPORT 等本次交付动作不再允许，历史 Certification 仍可查询，但 CurrentEntitlementGate 必须阻止实际交付。

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
- 人工工时（真实试点时）

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
