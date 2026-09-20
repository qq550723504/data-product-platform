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
- **本次 delivery context 必须被该 Certification 冻结的 CertificationProfile snapshot 显式覆盖**：
  - Profile 必须显式声明 purpose applicability：`ANY` 或 `EXPLICIT`；EXPLICIT 时 requested purpose 必须命中冻结集合；
  - Profile 必须显式声明 action applicability：`ANY` 或 `EXPLICIT`；EXPLICIT 时 requested action（如 USE / SHARE / RAW_EXPORT / RESALE / AI_TRAINING）必须命中冻结集合；
  - Profile 必须显式声明 consumer applicability：`ANY` 或 `EXPLICIT`；EXPLICIT 时 requested consumer 必须命中冻结 consumer 集合/规则；
  - Profile 必须显式声明 delivery channel/mode applicability：`ANY` 或 `EXPLICIT`；EXPLICIT 时 requested delivery channel/mode 必须命中冻结集合；
  - **字段缺失、NULL、UNKNOWN 或无法从 frozen snapshot 证明覆盖，都不是 wildcard，必须 fail closed**（例如 `CERTIFICATION_PROFILE_CONTEXT_UNSPECIFIED`）；
  - 只有显式 `ANY` 才代表该维度不限；
- Rights 允许某动作不代表 CertificationProfile 已对该动作做过质量/合规/合同认证；例如 INTERNAL_USE profile 不能用于 RAW_EXPORT；
- 若 C1 被 C2=REJECTED supersede，C1 不能再用于 delivery，C2 也因 decision != CERTIFIED 不能通过；
- 不允许用 created_at/latest 作为“当前认证”选择规则。

CurrentEntitlementGate 按“现在”重新检查至少：

- 每个绑定 RightsDeclaration / verification 是否 VERIFIED、其自身 validity window 覆盖 as_of、resource/consumer/purpose/action/scope 与本次 delivery context 匹配，且未被有效 INVALIDATED / SUPERSEDED；
- 每个 Authorization 是否存在当前有效 AuthorizationProvenanceBinding 支撑 grantor_ref，并且该 binding 在 as_of 时点未被 AuthorizationProvenanceBindingDisposition INVALIDATED / SUPERSEDED；Authorization 自身还必须 ACTIVE、未过期/撤销，并且其 grantee/consumer、DataResource、purpose、action、scope 全部覆盖本次 delivery context；
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
2. **在把 consumer 用于任何 CurrentCertificationGate / CurrentEntitlementGate 之前，服务端必须先建立可信 caller principal。** principal 必须来自已认证 session/token、mTLS、受控 API credential 或等价不可由普通请求字段伪造的服务端信任边界；原始 `consumer` 请求参数只能表达期望目标，不能自证“我就是该 consumer”；
3. 服务端必须把 caller principal 映射到其允许代表的 consumer/workspace 集合。若 caller 代表不同 consumer 执行 on-behalf-of delivery，必须存在由服务端验证的显式 delegation/impersonation grant，并把 principal、delegation、effective consumer 一起写入 Audit/Evidence；任意客户端 header、query/body 中的 actor/consumer ID、demo actor 配置都不得直接构成该授权；
4. principal 未认证、consumer 不在其允许集合内、delegation 缺失/过期/撤销时，必须在进入 CurrentDeliveryGate 前 fail closed（稳定 blocker 例如 `CALLER_IDENTITY_UNTRUSTED` / `CONSUMER_PRINCIPAL_MISMATCH`）。第一阶段不要求完整企业 IAM，但 **#135 必须实现最小可信 principal→consumer 解析边界**；没有该边界的 HTTP/demo 路径不得对外启用真实数据交付；
5. 服务端在实际返回数据、生成下载链接、签发对象存储 URL、token 或其他访问凭证**之前**，使用同一可信 principal + effective consumer 请求上下文重新执行完整 `CurrentDeliveryGate`；
6. 不接受客户端传入的“已通过 eligibility”布尔值或旧 gate result 作为授权依据；
7. gate 与 credential/data issuance 必须属于同一个 Application Command 的受控边界，避免 query→delivery 之间的 TOCTOU 绕过；
8. gate 失败时不得产生可用下载链接、token、credential 或数据响应；
9. 若交付形态需要签发访问凭证，凭证必须有明确有限有效期；第一阶段不得签发无期限凭证；
10. 凭证 `expires_at` 必须满足：

~~~text
expires_at
<= min(
     requested_expires_at,
     platform_max_credential_expiry,
     earliest applicable principal→consumer/workspace binding valid_to/expires_at,
     earliest applicable workspace membership valid_to/expires_at,
     earliest applicable delegation valid_to/expires_at,
     earliest applicable RightsDeclaration effective_to,
     earliest applicable Authorization valid_to,
     earliest already-scheduled identity binding/membership/delegation revocation_or_disable effective_at (when exposed by the trusted identity source),
     earliest already-scheduled RightsDisposition effective_at,
     earliest already-scheduled AuthorizationProvenanceBindingDisposition effective_at,
     earliest already-scheduled CertificationDisposition effective_at
   )
~~~

任何参与本次 delivery authorization 的已知有限边界都必须参与上限计算。除了 declaration / authorization validity，还包括 caller principal→consumer/workspace binding、workspace membership、delegation 的有限有效期，以及签发时 trusted identity source 已知的 future-effective revoke/disable（如该源可表达），再加上未来生效的 RightsDisposition / AuthorizationProvenanceBindingDisposition / CertificationDisposition。不能让 URL/token 在调用者代表资格、provenance、rights 或 certification 已按计划退出 current set 后继续有效；
11. 如果 delivery mode 支持 redemption-time server check，则每次 redemption 继续执行 CurrentDeliveryGate；如果是无法在 redemption 时回调平台的 bearer/presigned credential，则必须执行上述 expiry cap，并由 #135 明确该 delivery mode 的最大 TTL；
12. 对签发后才新增的紧急 revocation，只有 redemption-time gate / revocable credential 才能即时阻断；第一阶段若某 delivery mode 不具备此能力，必须在产品/API 中明确该限制，并使用短 TTL，而不能声称签发后的 bearer credential 可即时撤销。

### External credential issuance crash-safety

数据库事务不能把外部 token provider、对象存储签名服务或其它远端 credential issuance 纳入同一个 ACID transaction。第一阶段必须使用显式 crash-safe protocol，而不是声称“外部签发与 PostgreSQL 原子提交”。

推荐状态：

~~~text
PREPARED
├→ BLOCKED
└→ ISSUANCE_PENDING
    ├→ ISSUED
    ├→ FAILED
    ├→ BLOCKED
    └→ CONTAINMENT_PENDING
         ├→ BLOCKED
         └→ FAILED
~~~

协议至少满足：

1. **prepare first**：在任何外部可用 credential 产生前，先在 PostgreSQL 持久化 DeliveryOperation、当前 gate snapshot/decision、credential expiry cap、稳定 provider_request_key，并提交；
2. gate BLOCKED 时直接在同一 DB transaction 记录 BLOCKED + Audit/Evidence/Outbox，不调用 provider；
3. gate ALLOWED 时将 operation 持久化为 ISSUANCE_PENDING；外部调用必须使用稳定幂等键，默认以 DeliveryOperation ID（或其稳定派生值）作为 provider_request_key；
4. **每一次初始 issuance、retry issuance 或 reconciliation 后决定继续 issuance 之前，都必须重新验证已持久化 caller principal 当前仍可代表 effective consumer（含 workspace membership / delegation 的当前有效性），再重新读取当前事实并执行完整 CurrentDeliveryGate，同时重新计算 credential expiry cap。** PREPARED/ISSUANCE_PENDING 中保存的旧 principal→consumer 结论和旧 gate snapshot 都只用于审计，不可作为后续 issuance 授权；
5. **所有 delivery mode 与 entitlement-changing commands 必须有共享线性化机制。** 第一阶段采用（或实现等价强度的）delivery authorization fence/revision。这里不仅包括 URL/token/credential，也包括直接返回数据 bytes 的 direct-data delivery：
   - delivery authorization 依赖的 caller-principal status、principal→consumer/workspace binding、delegation（如有），以及 CurrentDeliveryGate 依赖的 DatasetVersion usability、selected Certification、RightsDeclaration / RightsDisposition、AuthorizationProvenanceBinding / BindingDisposition、Authorization 等关键 subject，都必须落到稳定 fence/revision identity（或等价可串行化机制）；
   - revoke/disable principal binding、workspace membership/delegation，Invalidate/Supersede RightsDeclaration、Invalidate/Supersede AuthorizationProvenanceBinding、Revoke/Supersede DatasetCertification、DatasetVersion invalidate，以及其它会改变实际 delivery authorization 的 Command，在提交业务变更前必须获取/推进对应 fence；
   - 外部 provider 模式在 provider 调用前记录本次 gate dependency revision vector / fence token；
   - **任何 delivery mode 在产生第一个外部可观察交付副作用前，都必须在 terminal DB transaction 内重新获取相同 fence（固定顺序锁定），重新验证 caller principal→effective consumer/delegation 当前仍有效，再重新执行 CurrentDeliveryGate，并验证 dependency revision/token 未被并发变更穿越。** credential/provider 模式还必须重新计算 fresh cap；
   - provider 模式：provider 返回或 reconciliation 恢复 capability 后执行上述 fenced finalize；只有 finalize/ISSUED commit 成功后才能把 credential 返回客户端；
   - direct-data 模式：在发送 HTTP body、stream chunk、文件字节或任何数据 payload 的**第一字节之前**执行上述 fenced finalize，并先提交 ISSUED terminal fact + Audit/Evidence/Outbox/CostEvent（如有）；commit 成功后才允许开始写 response body；
   - direct-data 不得为了整个大文件/stream 生命周期持有数据库 fence/row lock；锁只覆盖 re-gate + terminal commit。commit 后的 response 是已经在线性化点获准的一次交付，后续 disposition 在线性顺序上发生在该交付之后；
   - 若任何影响 gate 的变更先完成，finalize 必须看到新 revision/current facts，不能 ISSUED；provider capability 已产生时转 containment/block/fail，direct-data 则不得发送任何 byte；
   - 若 finalize 先完成，则并发 disposition/invalidation 在线性顺序上发生在 issuance 之后；provider credential 按 delivery mode 的撤销语义处理，direct-data 已被授权的这一响应不能被描述为“在 disposition 之前未发生”。
6. 如果 fresh gate 已 BLOCKED：
   - 若 operation 还未发生任何 provider 调用，可直接 BLOCKED；
   - 若 operation 曾进入可能已调用 provider 的 ISSUANCE_PENDING/retry/reconciliation 窗口，**必须先用同一 provider_request_key reconciliation 既有 provider outcome**，不能直接记 BLOCKED；
   - provider 明确“未签发” → BLOCKED；
   - provider 已存在 credential/access capability → 必须先 revoke / compensate / contain，并在确认该访问能力已不可用后才能 BLOCKED；
   - provider outcome unknown、查询失败、或 revoke/contain 未确认成功 → 进入 CONTAINMENT_PENDING，不得对外声称 BLOCKED，也不得发出 DatasetDeliveryBlocked terminal event；
   - CONTAINMENT_PENDING 必须由 reconciliation/人工告警持续处理，直到确认 access capability 不存在或已被安全失效；
- containment 确认成功后，若终结原因是 fresh gate 已不允许交付，则转 BLOCKED；若 gate 仍 ALLOWED 但 credential 无法满足 fresh cap/issuance contract，则转 FAILED（例如 CREDENTIAL_EXCEEDS_FRESH_CAP）；
7. provider 首次返回或 reconciliation 恢复出 credential/access capability 后，**在写入 ISSUED 前必须验证 provider 实际签发的能力边界，而不只是验证 expiry**：
   - actual expiry / access time bound 不晚于当前 fresh credential expiry cap；
   - actual resource / DatasetVersion 不得比本次 delivery request 更宽；
   - actual consumer/grantee（provider 可表达时）必须匹配本次 consumer；
   - actual action/permission 必须是 requested action 的等价或更窄集合，不能把 READ/USE 请求提升成 bucket-wide write/share/admin；
   - actual object/row/prefix/scope 必须等价或更窄，不能把单 DatasetVersion/对象范围签成整个 bucket、workspace 或无约束 prefix；
   - actual delivery channel/mode constraints（如 provider 模型支持）不得放宽；
   - 上述 capability descriptor 必须通过 provider read-after-write / authoritative lookup（或等价可验证机制）确认，并记录非 secret 的 capability snapshot/hash/evidence；
   - 任何关键维度无法验证，或实际 capability 比 requested/current-gate context 更宽，都不得 ISSUED，必须 revoke/contain，或使用 platform redemption indirection。
该 cap/context 必须来自最近一次 fenced CurrentDeliveryGate，而不是 PREPARED 时的旧值；
8. 若 recovered/returned credential 的实际 expiry 晚于 fresh cap，或 capability scope 比当前 allowed/requested context 更宽：
   - 若 provider 能对**同一 access capability**安全缩短/收窄并可 read-after-write 验证实际 expiry <= fresh cap 且 resource/action/consumer/scope 均满足当前 allowed context，则验证成功后才允许继续 ISSUED；
   - 否则不得提交 ISSUED，必须先 revoke/contain 该 credential；
   - containment 未确认成功时进入 CONTAINMENT_PENDING；
   - containment 成功但无法在同一安全能力上满足 fresh cap 时，当前 DeliveryOperation 终结为 FAILED（例如 CREDENTIAL_EXCEEDS_FRESH_CAP）；如业务仍需交付，必须通过新的显式 delivery attempt/replacement operation 再次完整 re-gate，不得在同一幂等 operation 下静默签发第二份 credential；
   - actual expiry 无法可靠读取/验证时，对 direct bearer 等不可 redemption-time gate 的模式按不安全处理，不得 ISSUED；
9. 只有 credential/access capability 已证明满足 fresh cap 后，才用后续 DB transaction 记录 ISSUED + provider credential reference/hash（不得保存可用 secret 正文）+ **verified actual credential expiry** + Audit/Evidence/CostEvent/Outbox；**只有这个 terminal commit 成功后**才能把可用 credential 返回给客户端；
10. 如果发生“provider 已成功，但 terminal commit 失败/进程崩溃”的不确定窗口，重试必须使用同一 provider_request_key 查询/重放同一 issuance，不得生成第二份独立 credential；
11. 必须存在 reconciliation path，能够把长时间停留在 ISSUANCE_PENDING 的 operation 解析为：
   - provider 已成功 → 恢复并完成同一个 ISSUED terminal fact；
   - provider 明确失败 → FAILED；
   - outcome 无法确认但 provider 支持 revoke/compensation → 先撤销/补偿再 FAILED；
12. **direct bearer mode 的恢复要求更严格**：provider 必须能够基于同一 provider_request_key replay / read-after-write 返回**同一 credential（或等价可重复获取的同一访问能力）**。仅支持 revoke/compensation 但无法恢复同一 bearer secret，不足以支持 direct bearer，因为“terminal ISSUED 已提交但 HTTP response 丢失”后客户端重试无法拿回原 credential；
13. 如果 provider 不能恢复同一 credential，则第一阶段必须使用平台控制的 redemption indirection；也可以在能够证明旧 credential 未交付且已成功 revoke 的协议下执行显式 replacement operation，但不得把同一 DeliveryOperation 的幂等 retry 静默变成第二份 credential；
14. 如果外部 provider **既不支持 idempotency/read-after-write，也不支持 revoke/compensation**，第一阶段不得直接暴露其 bearer credential；必须改用平台控制的 redemption indirection，或将该 delivery mode 判为 unsupported；
15. 本地生成 presigned URL 时，也必须先持久化 PREPARED/ISSUANCE_PENDING，并在每次实际生成前重新执行 CurrentDeliveryGate/expiry cap；terminal DB commit 成功前不得把 URL 返回客户端或写入日志/事件；
16. direct-data delivery 不得绕过上述 terminal fence：ISSUED commit 成功前 response body 必须保持 0 bytes；若 commit 失败或 gate 被并发变更阻断，则该请求不得泄露任何数据字节；
17. **direct-data 的 terminal `ISSUED` 只表示该次交付授权在线性化点已提交、服务端随后可以开始写响应；它不是“客户端已收到全部数据”的证明。** 网络/进程在 commit 后、第一字节前或流中断开时，不得把 ISSUED 审计事实解释为客户端完成接收；
18. **terminal ISSUED 的 direct-data operation 不允许用同一 idempotency key 从旧 gate 结果再次发出数据字节。** 同一 key 的 retry 必须返回稳定的 non-payload 结果（例如 `DIRECT_DATA_REPLAY_REQUIRES_NEW_ATTEMPT`，附原 operation ID/ISSUED 状态），response body 中不得包含 DatasetVersion 数据；
19. 如果调用方确实需要重新取得 direct-data，必须创建新的显式 DeliveryOperation/attempt（新的 idempotency key，可用 `retry_of_delivery_operation_id` 关联原 attempt），重新解析可信 caller principal→consumer/delegation，重新执行完整 CurrentDeliveryGate，并重新走 delivery authorization fence/finalize。若期间 Rights/Certification/DatasetVersion/Authorization 已失效，新 attempt 必须 BLOCKED；只有 fresh gate 仍 ALLOWED 才能再次发送数据；
20. retries / reconciliation 不得重复同一 DeliveryOperation 的 terminal Domain Event 或同一业务结果 Audit/Event 事实；**CostEvent 的去重只限于 same physical activity attempt 的 replay**。若 retry/reconciliation 真正再次调用 provider、执行 compute 或发生其它可计费外部工作，必须使用新的稳定 attempt/activity identity 追加实际 CostEvent（或原子聚合新增 quantity/amount 并保留 attempt identity/count）。显式的新 direct-data DeliveryOperation/attempt 是新的业务事实，必须有独立 operation ID 与审计链。

测试必须覆盖故障注入：
- provider 成功后、terminal DB commit 前 crash；
- terminal commit 成功后、HTTP response 前 crash，并验证 idempotent retry 能恢复同一 credential/访问能力，或通过 redemption indirection 返回稳定访问句柄；
- direct-data terminal ISSUED commit 成功后、第一字节前 crash：同一 idempotency key retry 必须返回 non-payload replay-required 结果且保持 0 dataset bytes；不得基于旧 gate 直接重放数据；
- direct-data 以新的显式 attempt 重试：必须重新解析 trusted principal/effective consumer 并 fresh re-gate；若 crash 后发生 revocation/disposition/invalidation，新 attempt BLOCKED 且 0 bytes；若仍 ALLOWED，才可在新的 fenced ISSUED commit 后发送数据；
- reconciliation/retry：same-attempt replay 不重复成本；故障恢复若真实再次调用可计费 provider，则新增 attempt identity 的实际 CostEvent 不得被 DeliveryOperation 顶层幂等吞掉；
- provider 成功后 fresh cap 因 future-effective disposition 缩短，recovered credential 实际 expiry 超过 fresh cap；
- provider actual expiry 不可验证；
- provider timeout 导致 unknown outcome；
- fresh gate 在 ISSUANCE_PENDING 中变为 BLOCKED，但 provider 实际已签发；
- revoke/contain 成功后才能进入 BLOCKED；
- revoke/contain 失败或 outcome unknown 时保持 CONTAINMENT_PENDING；
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
