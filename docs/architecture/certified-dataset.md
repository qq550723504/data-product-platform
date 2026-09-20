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
- DatasetCertification 本身不 UPDATE。若旧认证 C1 需要退出当前有效集合，第一阶段使用 append-only `CertificationDisposition`：
  - `SUPERSEDED`：显式关联 `superseded_by_certification_id`；
  - `REVOKED`：无 replacement 时显式撤销；
  - 记录 `effective_at`、reason、Evidence、actor。
- 旧 Certification 继续作为 issued-at 历史事实保留，不能因为新 Certification 存在就用“最新 created_at”隐式替换。

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

DatasetCertification 是认证时点的历史事实；它本身不是永久有效的交付授权。ProductRelease 是数据产品发布事实。

Certified Dataset 可以独立成为交付对象，但每一次实际交付都必须通过完整 CurrentDeliveryGate，而不是只检查 Rights。

允许：

~~~text
Certified DatasetVersion
├→ CurrentDeliveryGate → standalone delivery
└→ ProductAsset → ProductVersion → ProductRelease
~~~

CurrentDeliveryGate 是实际交付前的组合门禁：

~~~text
CurrentDeliveryGate
├── DatasetVersionUsability
├── CurrentCertificationGate
└── CurrentEntitlementGate
~~~

DatasetVersionUsability 至少要求：

- 明确检查当前 DatasetVersion.status；
- INVALID 必须 BLOCKED，即使历史 DatasetCertification 为 CERTIFIED；
- CREATED / PROCESSING / FAILED 不得作为可交付版本；
- 对 SUPERSEDED 的处理遵循平台现有“明确历史版本可用性”语义，不在本 docs-only 基线中自动等同 INVALID；具体交付策略由实现测试固定。

CurrentCertificationGate 要求本次 delivery 明确绑定一条 DatasetCertification（或由 API 返回明确的 effective certification ID），并在 `as_of` 时点满足：

- certification 属于同一 workspace / DatasetVersion / requested CertificationProfile；
- decision = CERTIFIED；
- 不存在已生效的 REVOKED disposition；
- 不存在已生效的 SUPERSEDED disposition；
- **本次 delivery context 必须被该 Certification 冻结的 CertificationProfile snapshot 覆盖**：
  - requested purpose 必须满足 profile purpose / applicability；
  - requested action（如 USE / SHARE / RAW_EXPORT / RESALE / AI_TRAINING）必须落在 profile 明确允许/认证覆盖的 action 集合；
  - profile 若定义 consumer / delivery channel / applicability constraints，也必须匹配；
- Rights 允许某动作不代表 CertificationProfile 已对该动作做过质量/合规/合同认证；例如 INTERNAL_USE profile 不能用于 RAW_EXPORT；
- 若 C1 被 C2=REJECTED supersede，C1 不能再用于 delivery，C2 也因 decision != CERTIFIED 不能通过；
- 不允许用 created_at/latest 作为“当前认证”选择规则。

CurrentEntitlementGate 按“现在”重新检查至少：

- 当前 RightsDeclaration / verification 是否 VERIFIED 且未被有效 INVALIDATED / SUPERSEDED；
- Authorization 是否 ACTIVE 且未过期/撤销；
- consumer / purpose 是否匹配；
- 本次 delivery action（例如 SHARE / RAW_EXPORT）是否当前仍允许；
- 衍生数据 Effective Rights 是否仍允许该动作。

任一子门禁失败都必须 fail closed，Current Delivery Eligibility = BLOCKED；历史 DatasetCertification 仍保留其 issued-at 结论。

第一阶段不要求周期性后台重认证，也不要求给 DatasetCertification 本身增加自动过期状态；“历史认证结论”和“当前可交付资格”必须分开查询和展示。

第一阶段不要求所有 ProductRelease 强制只能使用 Certified DatasetVersion。

## 10. API / UI

API 提供 DatasetVersion assessments、assessment report、certification profile summary、历史 certification list/detail、effective certification / disposition，以及面向明确 certification + consumer / purpose / action 的 CurrentDeliveryGate 查询。

**Eligibility 查询只用于展示/预检，不是安全边界。**

第一阶段必须提供一个真正的 server-side delivery command（命名可由 #135 实现固定，例如 `DeliverDatasetVersion` / `IssueDatasetAccess`），并满足：

1. 请求显式包含 workspace、DatasetVersion、Certification、consumer、purpose、action 和 delivery mode；
2. 服务端在实际返回数据、生成下载链接、签发对象存储 URL、token 或其他访问凭证**之前**，使用同一请求上下文重新执行完整 `CurrentDeliveryGate`；
3. 不接受客户端传入的“已通过 eligibility”布尔值或旧 gate result 作为授权依据；
4. gate 与 credential/data issuance 必须属于同一个 Application Command 的受控边界，避免 query→delivery 之间的 TOCTOU 绕过；
5. gate 失败时不得产生可用下载链接、token、credential 或数据响应；
6. 若交付形态需要签发访问凭证，凭证必须有明确有限有效期；第一阶段不得签发无期限凭证；
7. 凭证 `expires_at` 必须满足：

~~~text
expires_at
<= min(
     requested_expires_at,
     platform_max_credential_expiry,
     earliest applicable RightsDeclaration effective_to,
     earliest applicable Authorization valid_to
   )
~~~

任何参与本次 CurrentEntitlementGate 的有限权利边界都必须参与上限计算，不能让 URL/token 在 entitlement 到期后继续有效；
8. 如果 delivery mode 支持 redemption-time server check，则每次 redemption 继续执行 CurrentDeliveryGate；如果是无法在 redemption 时回调平台的 bearer/presigned credential，则至少必须执行上述 expiry cap，并由 #135 明确该 delivery mode 的最大 TTL。

关键写动作使用显式 Command。

UI 在 DatasetVersion 上分别展示：

- 历史 Certification 结果及 issued_at；
- 当前 Delivery Eligibility：ALLOWED / BLOCKED（来自 CurrentDeliveryGate，仅用于展示/预检）；
- 若 BLOCKED，显示当前 rights blocker（例如 AUTHORIZATION_EXPIRED / REVOKED / ACTION_NOT_ALLOWED）。

不得仅凭历史 CERTIFIED 标记显示“当前可交付”，也不把 certification status 塞入 DatasetVersion.status。

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

- Certification 自身的自动有效期/周期性后台复认证（但第一阶段必须有 CertificationDisposition / CurrentCertificationGate，以及每次实际交付的 CurrentDeliveryGate）
- 通用 override
- 电子签章
- PDF 证书
- 数据市场
- Gold Dataset
- 全行业统一质量总分
