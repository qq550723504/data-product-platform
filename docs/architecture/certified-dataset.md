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
→ AuthorizationProvenanceBinding
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

- 每个绑定 RightsDeclaration / verification 是否 VERIFIED、其自身 validity window 覆盖 as_of、resource/consumer/purpose/action/scope 与本次 delivery context 匹配，且未被有效 INVALIDATED / SUPERSEDED；
- 每个 Authorization 是否存在当前有效 AuthorizationProvenanceBinding 支撑 grantor_ref，并且该 binding 在 as_of 时点未被 AuthorizationProvenanceBindingDisposition INVALIDATED / SUPERSEDED；Authorization 自身还必须 ACTIVE、scope 匹配且未过期/撤销；
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
     earliest applicable Authorization valid_to,
     earliest already-scheduled RightsDisposition effective_at,
     earliest already-scheduled AuthorizationProvenanceBindingDisposition effective_at,
     earliest already-scheduled CertificationDisposition effective_at
   )
~~~

任何参与本次 CurrentDeliveryGate 的已知有限边界都必须参与上限计算。除了 declaration / authorization validity，还包括签发时已经存在、将在未来生效的 RightsDisposition / AuthorizationProvenanceBindingDisposition / CertificationDisposition。不能让 URL/token 在 provenance 或 certification 已按计划退出 current set 后继续有效；
8. 如果 delivery mode 支持 redemption-time server check，则每次 redemption 继续执行 CurrentDeliveryGate；如果是无法在 redemption 时回调平台的 bearer/presigned credential，则必须执行上述 expiry cap，并由 #135 明确该 delivery mode 的最大 TTL；
9. 对签发后才新增的紧急 revocation，只有 redemption-time gate / revocable credential 才能即时阻断；第一阶段若某 delivery mode 不具备此能力，必须在产品/API 中明确该限制，并使用短 TTL，而不能声称签发后的 bearer credential 可即时撤销。

### External credential issuance crash-safety

数据库事务不能把外部 token provider、对象存储签名服务或其它远端 credential issuance 纳入同一个 ACID transaction。第一阶段必须使用显式 crash-safe protocol，而不是声称“外部签发与 PostgreSQL 原子提交”。

推荐状态：

~~~text
PREPARED
├→ BLOCKED
└→ ISSUANCE_PENDING
    ├→ ISSUED
    └→ FAILED
~~~

协议至少满足：

1. **prepare first**：在任何外部可用 credential 产生前，先在 PostgreSQL 持久化 DeliveryOperation、当前 gate snapshot/decision、credential expiry cap、稳定 provider_request_key，并提交；
2. gate BLOCKED 时直接在同一 DB transaction 记录 BLOCKED + Audit/Evidence/Outbox，不调用 provider；
3. gate ALLOWED 时将 operation 持久化为 ISSUANCE_PENDING；外部调用必须使用稳定幂等键，默认以 DeliveryOperation ID（或其稳定派生值）作为 provider_request_key；
4. **每一次初始 issuance、retry issuance 或 reconciliation 后决定继续 issuance 之前，都必须重新读取当前事实并重新执行完整 CurrentDeliveryGate，同时重新计算 credential expiry cap。** PREPARED/ISSUANCE_PENDING 中保存的旧 gate snapshot 只用于审计，不可作为后续 issuance 授权；如果当前 gate 已 BLOCKED，则不得调用 provider，并将 operation 安全终结为 BLOCKED；
5. provider 成功后，再用第二个 DB transaction 记录 ISSUED + provider credential reference/hash（不得保存可用 secret 正文）+ Audit/Evidence/CostEvent/Outbox；**只有这个 terminal commit 成功后**才能把可用 credential 返回给客户端；
6. 如果发生“provider 已成功，但 terminal commit 失败/进程崩溃”的不确定窗口，重试必须使用同一 provider_request_key 查询/重放同一 issuance，不得生成第二份独立 credential；
7. 必须存在 reconciliation path，能够把长时间停留在 ISSUANCE_PENDING 的 operation 解析为：
   - provider 已成功 → 恢复并完成同一个 ISSUED terminal fact；
   - provider 明确失败 → FAILED；
   - outcome 无法确认但 provider 支持 revoke/compensation → 先撤销/补偿再 FAILED；
8. **direct bearer mode 的恢复要求更严格**：provider 必须能够基于同一 provider_request_key replay / read-after-write 返回**同一 credential（或等价可重复获取的同一访问能力）**。仅支持 revoke/compensation 但无法恢复同一 bearer secret，不足以支持 direct bearer，因为“terminal ISSUED 已提交但 HTTP response 丢失”后客户端重试无法拿回原 credential；
9. 如果 provider 不能恢复同一 credential，则第一阶段必须使用平台控制的 redemption indirection；也可以在能够证明旧 credential 未交付且已成功 revoke 的协议下执行显式 replacement operation，但不得把同一 DeliveryOperation 的幂等 retry 静默变成第二份 credential；
10. 如果外部 provider **既不支持 idempotency/read-after-write，也不支持 revoke/compensation**，第一阶段不得直接暴露其 bearer credential；必须改用平台控制的 redemption indirection，或将该 delivery mode 判为 unsupported；
11. 本地生成 presigned URL 时，也必须先持久化 PREPARED/ISSUANCE_PENDING，并在每次实际生成前重新执行 CurrentDeliveryGate/expiry cap；terminal DB commit 成功前不得把 URL 返回客户端或写入日志/事件；
12. retries / reconciliation 不得重复 CostEvent、AuditEvent 或 terminal Domain Event。

测试必须覆盖故障注入：
- provider 成功后、terminal DB commit 前 crash；
- terminal commit 成功后、HTTP response 前 crash，并验证 idempotent retry 能恢复同一 credential/访问能力，或通过 redemption indirection 返回稳定访问句柄；
- reconciliation/retry；
- provider timeout 导致 unknown outcome；
- compensation/revocation 路径。

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
