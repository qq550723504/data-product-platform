# Certified Dataset 架构

## 1. 定义

Certified Dataset 不是新的数据内容实体，而是一个不可变 DatasetVersion 满足某个明确 CertificationProfile 的可证明认证结果。

~~~text
Immutable DatasetVersion
+ QualityAssessment
+ Effective Rights
+ Compliance
+ Contract
+ Traceability / Evidence
+ CertificationProfile
+ DatasetCertification
= Certified DatasetVersion
~~~

平台不新增 HighQualityDataset 主表来复制 Dataset / DatasetVersion。

## 2. 为什么 Certification 独立

QualityAssessment 回答：

> 数据质量如何？

DatasetCertification 回答：

> 这份明确的数据版本，在某个用途/标准下，是否满足全部交付条件？

因此：

- Assessment 完成 ≠ Certified
- Quality PASS ≠ Rights PASS
- Dataset READY ≠ Certified
- ProductRelease PUBLISHED ≠ DatasetCertification

## 3. CertificationProfile

Profile 是认证规范，不是一次认证结果。

至少定义：

- code / name / version
- purpose / applicability
- required quality dimensions
- required / critical rules
- required rights/actions
- compliance requirements
- contract requirements
- traceability/evidence requirements

认证时必须冻结 Profile 的 version 与 content hash/snapshot。

Profile 文件未来变化，不得改变旧 Certification 的含义。

## 4. QualityAssessment

Assessment 必须绑定：

- DatasetVersion
- RuleSet ref/version
- RuleSet content hash/snapshot
- evaluator
- dimension summaries
- findings
- decision
- Evidence/Audit

历史报告只读取冻结 Assessment，不重新读取当前规则文件计算“过去”。

## 5. Rights

Certification 使用 #137 的权利链：

~~~text
RightsDeclaration
→ Authorization
→ RightsSnapshot
→ Effective Rights
~~~

认证需要验证 Profile 要求的 Purpose / Action 是否确实允许。

如果任一必要输入限制 required action，则不得 CERTIFIED。

## 6. 认证判定

~~~text
DatasetVersion usable?
QualityAssessment matches?
Critical quality rules pass?
Rights provenance verified?
Effective rights satisfy profile?
Compliance pass?
Contract matches?
Traceability / Evidence complete?
        ↓
CERTIFIED or REJECTED
~~~

必须 fail closed。

## 7. 不可变性

DatasetCertification 是历史事实。

禁止：

- PATCH certification.status = CERTIFIED
- 修改旧 Certification 以使用新 Profile
- 让 V1 Certification 指向 V2 DatasetVersion

修正必须区分“数据内容修正”和“治理/认证事实修正”：

- 只有 Dataset 的实际内容、schema/content identity 或生产输出发生变化时，才创建新的 DatasetVersion；
- 如果数据字节与 DatasetVersion 身份没有变化，只是 evaluator、QualityAssessment、Rights verification、RightsSnapshot、CertificationProfile 或 Certification 判断有误，则保留原 DatasetVersion，追加新的评测/权利/Profile/认证事实；
- 旧事实继续保留为历史，不通过 UPDATE 改写。若未来需要让已签发认证失效，应引入显式 Revocation / Supersession 事实，而不是伪造一个新的 DatasetVersion。

## 8. 新版本语义

~~~text
DatasetVersion V1 → Certification C1 (CERTIFIED)

DatasetVersion V2
→ NOT_CERTIFIED
→ new Assessment
→ new Certification C2
~~~

V2 不继承 V1 认证。

## 9. 与 ProductRelease 的关系

Certified Dataset 是数据集交付事实；ProductRelease 是数据产品发布事实。

允许：

~~~text
Certified DatasetVersion
→ ProductAsset
→ ProductVersion
→ ProductRelease
~~~

第一阶段不要求所有 ProductRelease 强制只能使用 Certified DatasetVersion。

## 10. API / UI

API 提供 DatasetVersion assessments、assessment report、certification profile summary、certification result / blockers。

关键写动作使用显式 Command。

UI 在 DatasetVersion 上展示 Quality 与 Certification，不把 certification status 塞入 DatasetVersion.status。

## 11. Evidence

认证必须可追到：

- DatasetVersion checksum/storage identity
- production execution/lineage
- mapping decisions（如适用）
- QualityAssessment
- Rights provenance / snapshot
- Compliance
- Contract
- CertificationProfile
- certification decision

## 12. 第一阶段非目标

- Certification 有效期/定期复认证
- 通用 override
- 电子签章
- PDF 证书
- 数据市场
- Gold Dataset
- 全行业统一质量总分
