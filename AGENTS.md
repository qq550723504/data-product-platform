# AGENTS.md

本文件约束所有 AI Coding Agent、Codex 线程及人工开发者在本仓库中的实现边界。

## 1. Core Domain Independence

Core Domain 不得直接依赖以下具体产品或 SDK：

- OpenMetadata
- Apache Hop
- Splink
- Soda / Great Expectations
- Presidio
- Label Studio / X-AnyLabeling
- MinIO / S3 SDK

外部能力必须通过 Port / SPI + Adapter 接入。

## 2. System of Record

Data Product Platform 自身数据库是以下核心业务事实的 System of Record：

- UseCase
- DataResource
- Dataset / DatasetVersion
- Entity / EntityMapping / immutable mapping decisions
- Authorization / AuthorizationProvenanceBinding / AuthorizationProvenanceBindingDisposition / RightsSnapshot
- RightsDeclaration / rights verification facts（#137 起）
- Workflow / Execution / frozen execution dependencies
- QualityAssessment / Quality findings（已通过 #140 / migration 000019 落地）
- DataContract
- CertificationProfile snapshot / DatasetCertification（#134 起）
- DeliveryOperation（#135 起）
- DataProduct / ProductVersion / ProductRelease
- CostEvent / CostAllocation
- Evidence / EvidenceSnapshot / AuditEvent

OpenMetadata 仅作为 Governance Projection。

外部质量、处理、实体解析或标注系统均不得成为上述核心业务状态的最终真相。

## 3. Immutable Objects and Historical Facts

以下对象创建并进入冻结状态后，不得原地修改业务含义：

- DatasetVersion
- EntityMappingDecision
- Execution dependency preparation / binding / mapping usage
- ProductVersion
- ProductRelease published bindings / published release history（领域 invariant；当前 `product_release` 行已有 guard，但 `product_release_dataset` membership 的 DB-level freeze 仍是 #99 已知缺口）
- ContractVersion
- WorkflowVersion
- EvidenceSnapshot（领域 immutable invariant；当前 header guard 已有，但 `evidence_snapshot_item` membership INSERT/DELETE/UPDATE 的 DB-level freeze 仍是 #99 已知缺口）
- RightsSnapshot
- QualityAssessment（已实现；兼容存储名 quality_result / quality_finding）
- verified RightsDeclaration / verification fact（#137 起）
- AuthorizationProvenanceBinding / BindingDisposition（#137 起）
- CertificationProfile snapshot（#134 起）
- DatasetCertification / CertificationDisposition（#134 起）

ProductRelease 特例：DRAFT / VALIDATING / READY 等发布前阶段允许显式 Command 按状态机更新 status 与 validation bindings；进入 PUBLISHED 后，当前 `guard_product_release_history` 只保护 `product_release` 主行的 UPDATE/DELETE。**当前 `product_release_dataset` membership 尚无数据库 INSERT/UPDATE/DELETE guard（#99 open），因此不能声称数据库已经完整冻结 published dataset bindings。** 领域 invariant 仍要求 published bindings 不可变；在 #99 补齐 membership guard 前，这是已知 enforcement gap。SUSPENDED / WITHDRAWN 目前只是 schema 枚举中的保留状态，不得声称已有 PUBLISHED → SUSPENDED/WITHDRAWN live transition；未来启用需要独立 migration + Command，同时不得回退 binding freeze。

DeliveryOperation 也是受控 lifecycle row，不得把整行视为创建即 immutable：PREPARED / ISSUANCE_PENDING / CONTAINMENT_PENDING / terminal 状态需要由显式 delivery/reconciliation Command 更新。必须冻结并保护的是 request/idempotency identity、确定后的 provider_request_key、已记录的 transition/gate/issuance history 与 terminal outcome 语义；不要安装会阻止合法恢复迁移的全行 UPDATE guard。

**通用 frozen aggregate 并发规则：** 任何采用 `DRAFT → FINALIZED/PUBLISHED`、且 parent 下存在可变 child membership/action/binding rows 的聚合，都必须把 child mutation 与 Finalize/Publish 串行化在同一个 parent row lock/fence/revision 上，并采用固定 parent-first 锁顺序。Finalize/Publish 必须在持有 parent lock 时验证完整 membership/content hash 再冻结。仅靠“FINALIZED 后 trigger 拒绝 mutation”不够，因为旧 transaction 可能在 finalize 前读到 DRAFT、却在 finalize 后才提交。该规则适用于 RightsSnapshot、EffectiveRightsSnapshot、GrantorAuthorityDelegationChain、未来新增的 Profile/Evidence/Release membership aggregate 等；若已有对象当前尚未满足，必须明确记录为 open enforcement gap，而不能声称已完整冻结。

修正错误时不得覆盖历史事实，但要按事实类型追加：

- 数据内容、schema/content identity 或实际生产输出变化 → 新 DatasetVersion；
- 数据内容未变化，仅 Quality 评测错误 → 新 QualityAssessment；
- 权利声明/验证错误 → 同一 RightsDeclaration 的 terminal verification outcome 不可翻转；错误 VERIFIED 先用 RightsDisposition INVALIDATED/SUPERSEDED 退出 current set，再创建新 RightsDeclaration + verification / RightsSnapshot（按实际语义）；
- CertificationProfile 规则变化 → 新 Profile version/snapshot；
- 认证判断错误或重新认证 → 新 DatasetCertification；旧认证退出 current set 时追加 CertificationDisposition（REVOKED / SUPERSEDED），不 UPDATE 旧认证，也不按 latest timestamp 猜当前认证；
- Product 发布事实变化 → 新 ProductVersion / ProductRelease 或显式生命周期 Command。

不得为了修正非内容事实而无意义地创建新的 DatasetVersion。

历史事实对象不得依赖软删除来模拟修正。

## 4. Ownership and Rights Semantics

data_resource.owner_id、dataset.owner_id 只表示平台内资产责任/归属，不自动等同现实世界法律所有权。

权利模型必须区分：

~~~text
RightsDeclaration
→ AuthorizationProvenanceBinding
→ Authorization
→ RightsSnapshot
→ Effective Rights
→ DatasetCertification
~~~

平台记录权利声明、依据、主体角色、允许项、限制项和 Evidence；不得声称平台自动裁定现实世界法律所有权。

Authorization 不能与 provenance 独立选择。每个进入 CurrentEntitlementGate 的 Authorization / ResourceGrant 都必须通过强类型 AuthorizationProvenanceBinding 证明其 grantor_ref 得到相应 VERIFIED RightsDeclaration 支持。若 grantor 不是 declaration-supported party 本身，binding 必须强类型引用 finalized GrantorAuthorityDelegationChain（或等价）及其 member edge identities/hash；不能只记录“存在 delegation”。

**Grantor delegation chain 是 current entitlement dependency，不是 binding-create-time 一次性检查。** 每次 CurrentEntitlementGate 都必须按 as_of 重新验证所有 required delegation edges 的 validity、REVOKED/INVALIDATED/SUPERSEDED disposition、delegator→delegate 连续性及 resource/purpose/action/normalized-scope coverage；任一 edge 失效即 fail closed。RightsSnapshot 冻结 chain identity/member IDs只用于历史解释，不把 delegation 永久化。

GrantorAuthorityDelegationChain 的冻结也必须并发安全：若使用 DRAFT→FINALIZED，ordered member writes 与 Finalize 获取同一 parent chain row lock/fence，固定 parent-first；Finalize 在锁内验证 member set/hash 后提交。不得只靠 FINALIZED 后 trigger 拒绝写入，否则 late member transaction 可穿越 finalization。

RightsDeclaration 的 resource / consumer applicability / purpose / action / scope / validity 必须强类型、可索引、可查询；这些 gate-critical 字段不得仅藏在 JSONB。

**使用权与授予权必须分离。** `allowed_actions/permitted purpose/use scope` 回答 party 自己能做什么；`grant_authority_mode + grantable_actions + grantable purpose + grantable scope` 回答 party 能否把这些权利授给别人。AuthorizationProvenanceBinding 必须证明后者覆盖 Authorization；只有 USE/PROCESS permission 而无 grant authority 时，不能作为 grant source。delegation chain 每一跳也必须显式携带 onward grant authority，不能把 use permission 当 sublicensing authority。

Authorization 的 gate-critical scope 也必须强类型/规范化、可索引、可查询（`scope_type` + `scope_ref` 或等价 relation）。现有 `authorization_resource.scope` JSONB 只能做扩展参数；BindAuthorizationProvenance / CurrentEntitlementGate 不得各自解析任意 JSONB 决定 allow。legacy Authorization 无法可靠归一化 scope 时 fail closed，不得把缺失 scope 当作全资源。

Authorization 本身也必须逐项覆盖当前 requested context：grantee/consumer、resource、purpose、action、scope、validity/status。RightsDeclaration 或 Effective Rights 的更宽范围不得放大一条更窄的 Authorization。

RightsSnapshot / EffectiveRightsSnapshot 的 immutable 语义覆盖 header + 全部 membership/action rows。若采用 DRAFT→FINALIZED，多事务 membership mutation 与 Finalize 必须获取同一个 parent snapshot row lock/fence（固定顺序 parent-first）：mutation 持锁检查 DRAFT 后写成员；Finalize 持同锁验证 membership/hash 后改 FINALIZED。不得只在 trigger 中无锁读取 parent status，否则可能出现 finalize 提交后旧 membership transaction 再提交的历史穿越。FINALIZED 后 INSERT/UPDATE/DELETE 全部 fail closed。

AuthorizationProvenanceBinding 创建后不可 UPDATE/DELETE。错误 binding 通过 append-only BindingDisposition（INVALIDATED / SUPERSEDED + effective_at）退出 current set；CurrentEntitlementGate 必须按 as_of 排除已生效 disposition。replacement binding 必须独立重新校验，历史 RightsSnapshot 继续引用旧 binding。

衍生数据的 Effective Rights 默认 fail closed：必须从 target DatasetVersion 的实际 required lineage/input facts 计算并持久化 immutable EffectiveRightsSnapshot（或等价 aggregate），冻结 calculation rule/hash、required input membership + source RightsSnapshot/provenance、逐 action decision/reason。任何必要输入不允许、未知、缺失或未被纳入冻结 input membership 时，输出不得获得该动作；只有所有 required inputs 明确 ALLOWED 才 ALLOWED。#134 只能引用 finalized immutable Effective Rights identity/hash。

**历史 EffectiveRightsSnapshot 不替代 delivery-time current rights。** 对衍生 DatasetVersion 的每次 CurrentDeliveryGate，必须遍历 target 的全部 immutable required inputs，对每个 input 重新验证当前 declaration/binding/Authorization/grantor-delegation facts，再对 requested action 做 fail-closed 交集。任一 source input 后续 revoke/expire/disposition 都必须立即阻断 derived delivery；这些 source dependencies 参与同一 delivery fence/revision 与 credential TTL cap。

## 5. State Transitions

禁止通过通用 PATCH status 修改关键业务状态。

关键状态变化必须使用显式 Command，例如：

- ApproveAuthorization
- VerifyRightsDeclaration
- InvalidateRightsDeclaration / SupersedeRightsDeclaration
- BindAuthorizationProvenance
- InvalidateAuthorizationProvenanceBinding / SupersedeAuthorizationProvenanceBinding
- InvalidateDatasetVersion
- RunQualityAssessment
- CertifyDatasetVersion
- RevokeDatasetCertification / SupersedeDatasetCertification
- ValidateProductRelease
- PublishProductRelease

状态迁移和认证判定规则必须由 Domain / Application 边界控制。

DataProduct 当前公开 API 的 live lifecycle 只应视为 `DRAFT → PUBLISHED`（由成功 PublishProductRelease 的受控副作用触发）。DESIGNING / DEVELOPING / TESTING / READY / ACTIVE / SUSPENDED / DEPRECATED / RETIRED 目前只是 domain/schema reserved values；在新增显式 Command/API 前，不得把这些 enum 当作已实现可达状态，也不得用 generic PATCH 补出迁移。

## 6. Domain Events and Audit

关键状态变化必须：

1. 产生 Domain Event；
2. 产生或触发 AuditEvent；
3. 需要异步副作用时写入 Transactional Outbox。

不得把普通 application log 当作 AuditEvent。

## 7. Cost and Evidence First

Cost 与 Evidence 是一等业务对象，不允许项目结束后再补录。

生产、人工审核、质量评测、Rights verification、Certification、Delivery、外部服务、Release 等关键活动应按业务需要产生：

- CostEvent
- Evidence
- AuditEvent

非 Execution CostEvent 必须有稳定 activity/idempotency identity，并通过强类型 CostAllocation 关联到 QualityAssessment、Rights verification/disposition、AuthorizationProvenanceBinding、DatasetCertification/Disposition、DeliveryOperation 等实际业务主体。禁止仅把 subject IDs 塞入 JSONB metadata。

CostEvent 的幂等边界是**同一次实际 activity attempt**，不是把一个顶层业务对象后续所有真实重试都合并掉。same-attempt 的 command/network/transaction replay 未产生新外部工作时必须去重；如果 failed/transient attempt 后再次真实调用 engine/provider、再次消耗 compute 或再次发生人工审核，则必须使用新的稳定 attempt/activity identity 记录新增 CostEvent，或原子聚合新增 quantity/amount 并保留可审计 attempt identity/count。不得用同一个 QualityAssessment / DeliveryOperation 的顶层 idempotency key 吞掉后来真实发生的成本。

Provider 成本不依赖业务终态。每一次真实 external provider invocation 在调用前先分配/持久化 physical attempt identity；成功、provider failure、timeout/unknown、reconciliation lookup、revoke/compensation 只要实际调用并可能计费，都必须记录该 attempt 的 CostEvent。amount 暂不可知时至少记录真实 invocation quantity/unit；不能等到 ISSUED 才记账，也不能因最终 BLOCKED/FAILED/CONTAINMENT_PENDING 而丢弃已发生成本。

Provider attempt persistence 采用不可变 identity/start fact + 追加式 outcome/observation fact（或等价强度模型）。调用前 start fact 已持久化；返回/timeout/unknown 后 append observation。后续 reconciliation 若重新判定原 attempt outcome，只能追加 resolution/observation，不能覆盖原始 start/首次观察；如果 reconciliation 自身真的调用 provider，则它是新的 provider_attempt_id，并独立计费。

## 8. Industry Pack Boundary

园区、制造、医疗、政务等行业逻辑不得硬编码进 Core Domain。

行业能力应通过 industry-packs/<industry>/ 提供：

- Entity Types
- Glossary
- Standardization Rules
- Matching Policies
- Indicators
- Quality Rules
- Compliance Rules
- Certification Profiles
- Product Templates

禁止在核心代码中散落 if industry == "PARK" 逻辑。

## 9. Vertical Slice First

优先完成端到端业务切片，不优先“做完整某一技术层”。

已完成 POC 切片：

~~~text
CSV
→ DataResource
→ RAW DatasetVersion
→ Entity Resolution
→ STANDARDIZED DatasetVersion
→ Workflow
→ CURATED DatasetVersion
→ ProductRelease
→ Evidence
~~~

当前 Certified Dataset Pilot 切片：

~~~text
DataResource
→ Rights Provenance
→ DatasetVersion
→ Processing / Entity Resolution
→ QualityAssessment
→ Effective Rights / Compliance / Contract
→ DatasetCertification
→ Certified DatasetVersion
~~~

## 10. Quality and Certification

QualityAssessment 回答“数据质量如何”；DatasetCertification 回答“该 DatasetVersion 是否满足某个 CertificationProfile”。

不得把“评测完成”自动等同于“认证通过”。

V1 不强制全行业统一总分。Critical rule、Rights、Compliance、Contract 或 Traceability 任何 required 条件缺失时，Certification 必须 fail closed。

Certified Dataset 是可独立交付成果，不要求必须包装成 DataProduct；但 DatasetCertification 只证明认证时点结论，不授予永久交付资格。

每次 standalone delivery 必须执行 CurrentDeliveryGate：

- DatasetVersionUsability：至少拒绝 INVALID / FAILED / PROCESSING / CREATED；SUPERSEDED 是否允许按明确历史版本交付遵循现有领域语义和实现验收；
- CurrentCertificationGate：delivery 必须绑定明确 certification；其 decision 必须为 CERTIFIED，且在 as_of 时点未被 CertificationDisposition REVOKED / SUPERSEDED；frozen CertificationProfile 对 requested purpose/action/consumer/delivery channel/mode 每个维度都必须显式 ANY / EXPLICIT 覆盖。缺失、NULL、UNKNOWN 不等于 ANY，必须 fail closed；禁止用 latest created_at 猜当前认证；
- CurrentEntitlementGate 必须区分 entitlement path：
  - **DIRECT_USE**：当前请求直接依赖 RightsDeclaration 允许该 party/consumer 自身使用时，校验 declaration VERIFIED/current validity/disposition、resource、consumer applicability、permitted purpose、allowed_actions、use scope；
  - **DOWNSTREAM_AUTHORIZATION**：当前请求依赖 Authorization 给 grantee/consumer 的授权时，declaration 只需作为 current provenance/grant-authority source（VERIFIED、current validity/disposition、resource 匹配），并通过 AuthorizationProvenanceBinding / current grantor-delegation chain 证明 **grantable purpose/action/scope** 覆盖该 Authorization；随后逐项校验 Authorization 的 grantee/consumer、resource、purpose、action、normalized scope、状态/有效期。**不得再要求 grantor 自己的 allowed_actions/use scope 匹配 grantee 的 delivery request。**
  对 derived DatasetVersion 不能只读取历史 EffectiveRightsSnapshot：必须遍历全部 immutable required source inputs，并按各 source 实际 entitlement path 重新验证 current declaration/binding/Authorization/grantor-delegation facts，再对 requested action 做 fail-closed 交集。

任一子门禁失败都必须 fail closed，即使历史 Certification 仍为 CERTIFIED。

CurrentDeliveryGate query 只用于展示/预检，不构成交付授权。任何返回数据、下载链接、presigned URL、token 或访问凭证的 server-side delivery Command 都必须在 delivery 前重新执行完整 CurrentDeliveryGate；不得信任客户端缓存的旧 gate result。所有 delivery mode 都必须在第一个外部可观察交付副作用前完成共享 delivery authorization fence/revision 下的 terminal finalize。direct-data 必须先提交 ISSUED，再允许写 response 第一字节；commit 前必须 0 bytes。

Delivery Command 的 `consumer` 不得直接信任请求字段。进入 CurrentDeliveryGate 前必须从已认证 caller principal 解析其允许代表的 effective consumer/workspace；on-behalf-of 必须有服务端验证的显式 delegation，并把 principal/delegation/effective consumer 写入 Audit/Evidence。demo actor ID、任意 header/body consumer ID 都不是认证。第一阶段可以不做完整 IAM，但没有最小可信 principal→consumer 边界的 HTTP 路径不得执行真实 delivery。

该 principal→consumer/workspace binding/delegation 不是一次性 precheck。任何 initial/retry/reconciliation issuance 与 terminal finalize 都必须重新验证其当前有效性；可撤销 binding/delegation/principal status 必须与 Rights/Certification/DatasetVersion 等 gate dependency 一样参与 delivery authorization fence/revision（或等价串行化机制），防止“先校验身份、后撤销 delegation、仍然签发”的 TOCTOU。

Direct-data 的 terminal `ISSUED` 只证明该次交付授权已在线性化点提交，不证明客户端收到全部 bytes。若 ISSUED commit 后响应丢失/进程崩溃，同一 idempotency key 不得依据旧 gate 再次发数据；只返回稳定 non-payload replay-required 结果。需要再次取数时创建新的显式 DeliveryOperation/attempt（可关联 retry_of），重新解析 principal→consumer、重新 CurrentDeliveryGate、重新走 fence；期间任何 revocation/invalidation 必须使新 attempt fail closed。

DeliveryOperation 每个终态都必须产生明确 Domain Event：Issued / Blocked / Failed（事件名由实现固定但语义不得缺失），并与 Audit/Evidence/Outbox 保持一致幂等边界。**进入安全关键非终态 `CONTAINMENT_PENDING` 也必须在同一 transition transaction 产生显式 `DatasetDeliveryContainmentPending`（或固定等价）Domain Event + Audit/Evidence + Outbox**；不能因为它“不是终态”而只更新状态。相同 transition/idempotency replay 不重复事件；后续 confirmed containment 再单独产生 Blocked/Failed terminal event。terminal-specific CostEvent（如有）同样遵循该业务事务；provider invocation CostEvent 按 physical provider-attempt 独立记录，不能等到 terminal ISSUED/FAILED/BLOCKED 才决定是否存在。任何事件或审计 payload 不得包含可用 credential secret。

外部 credential issuance 不能假装与 PostgreSQL 同事务。必须先持久化 DeliveryOperation + stable provider_request_key，再执行外部副作用；provider 返回/恢复 capability 后，terminal ISSUED transaction 必须使用与所有影响 delivery authorization 的 identity binding/delegation 与 CurrentDeliveryGate disposition/invalidation Commands 共享的 delivery authorization fence/revision，再次验证 principal→effective consumer/delegation + re-gate + fresh-cap；direct-data 也必须使用同一 fence 在 ISSUED commit 后才能写第一字节。该 commit 是 delivery linearization point。**每次初始/retry/reconciliation issuance 前都必须重新验证 caller authority、执行 CurrentDeliveryGate 并重新计算 expiry cap**，旧 identity/gate snapshot 仅供审计。fresh gate BLOCKED 时，如 provider_request_key 可能已经产生外部访问能力，必须先 reconcile 并 revoke/contain；只有确认没有活跃访问能力后才能终结 BLOCKED，否则保持 CONTAINMENT_PENDING。CONTAINMENT_PENDING 在 confirmed containment 后也允许终结 FAILED：用于 gate 仍 ALLOWED、但 credential/issuance contract 无法满足（如实际 expiry 超 fresh cap 且无法安全 shorten）的场景。direct bearer mode 还必须同时支持按同一 provider_request_key 恢复/重放同一 credential（或等价同一访问能力），以及 fresh credential-replay authorization 被拒绝时 revoke/contain 既有 capability；任一能力缺失都必须走 platform redemption/gateway，不能把同一幂等 retry 静默签发成第二份 credential，也不能在当前授权已失效时重放旧 secret。ISSUANCE_PENDING 必须可 reconciliation。

**Terminal ISSUED credential 的 same-key replay 也必须 fresh authorize。** HTTP response 丢失后再次返回同一 credential/稳定 handle 前，重新认证 caller→effective consumer/delegation，在共享 delivery authorization fence/revision 下重新 CurrentDeliveryGate + fresh-cap，并验证 recovered same capability 仍满足当前 consumer/resource/action/scope/channel/expiry；append 不含 secret 的 replay decision 后，只有 ALLOWED 才能再次返回 secret。若当前已 BLOCKED，0 credential 输出并 revoke/contain 旧 capability；containment 未确认时返回 non-secret containment-pending，不得改写原 ISSUED 历史事实。direct bearer provider 若不能同时做到 same-capability recovery 与 blocked-replay containment，则必须走 platform redemption/gateway。

Credential replay 自身进入 CONTAINMENT_PENDING 时也必须发出显式 non-terminal containment event（例如 `DatasetCredentialReplayContainmentPending`，或统一 containment event + subject_kind/replay_attempt_id），与 replay decision + Audit/Evidence + Outbox 同事务；不得用原 DeliveryOperation 的 terminal Blocked/Failed event 冒充该状态。

若签发 URL/token/credential，`expires_at` 不得晚于 requested TTL、平台最大 TTL、caller principal→consumer/workspace binding / workspace membership / caller delegation 的最早有限有效期、**CurrentEntitlementGate 实际依赖的 grantor-authority delegation chain 所有 required edges 的最早 valid_to**，以及本次 entitlement 链上最早的 RightsDeclaration / Authorization 有效期边界；签发时已知且未来生效的 caller identity revoke/disable、**grantor delegation disposition effective_at**、RightsDisposition / AuthorizationProvenanceBindingDisposition / CertificationDisposition 的最早 effective_at 也必须参与 cap。支持 redemption-time server check 的 delivery mode 应在 redemption 时重新验证 caller authority + CurrentDeliveryGate；不可回调的 bearer/presigned credential 必须使用该完整 expiry cap + 明确最大 TTL。

任何 provider 首次返回或 reconciliation 恢复出的 credential，在进入 ISSUED 前必须验证实际 capability 是 requested/current-gate context 的等价或更窄集合：expiry <= fresh cap，resource/DatasetVersion、consumer/grantee、action、object/row/prefix scope、channel 不得扩大。**consumer/grantee 不能因 provider“不支持该字段”而跳过**：direct bearer/presigned capability 必须有 provider-native 或等价可验证的 consumer-binding enforcement；否则必须用 platform redemption/gateway 在 redemption 时重新认证并强制 effective consumer，或标记 direct mode unsupported。命中旧 provider_request_key 不能绕过这条检查；超过 fresh cap、scope 过宽或关键维度不可验证时必须 shorten/narrow+verify 或 revoke/contain，无法安全满足当前 context 时不得 ISSUED。

## 11. Release Readiness

任何 Product Release 发布必须经过统一 ReleaseReadiness 判断。

至少检查：

- Production
- Rights
- Quality
- Compliance
- Contract
- Dataset
- Evidence
- Delivery（如适用）

不得绕过 Readiness 直接设置 PUBLISHED。

DatasetCertification 与 ProductRelease 是不同业务事实；不要用一个对象替代另一个。

## 12. External Engines

外部 Engine 的 ID、状态和错误信息不能成为 Core Domain 的业务真相。

例如：

- execution.id 是平台业务 ID；
- engine_execution_id 只是 Hop / Python / Spark 的外部引用。

Engine Adapter 错误需要映射为平台统一错误模型。

## 13. Database Rules

- 核心可查询业务字段使用强类型列；
- Engine 扩展信息、行业扩展属性和受控参数可使用 JSONB；
- 不要把核心主外键、状态、版本号、权利关系藏入 JSONB；
- 历史事实对象不得软删除或覆盖；
- 对外部系统 ID 不建立数据库 FK；
- 破坏不可变历史的 migration down 必须 fail closed；
- 先检查再 DDL 的 destructive down 要考虑并发业务写入的锁语义。

## 14. Definition of Done

核心业务 Issue 至少考虑：

- Domain Rule
- Migration（如涉及持久化）
- Repository
- Application Command / Query
- HTTP API（如适用）
- Domain Event
- AuditEvent
- Evidence（关键动作）
- Idempotency（关键 Command）
- Workspace / ownership boundary
- Authenticated principal → effective consumer / delegation boundary（涉及交付时）
- Historical immutability
- Current entitlement / delivery gate（涉及交付时）
- Tests

关键业务动作不得仅实现为 CRUD。

## 15. Testing

优先级：

1. Domain tests
2. Application tests
3. Adapter contract tests
4. Real PostgreSQL integration tests（持久化/并发/约束）
5. Vertical slice integration tests
6. Browser / live-core acceptance（用户路径）

外部 Engine Adapter 应尽量复用统一 Contract Test。

## 16. Current Stage and Scope Discipline

核心 POC 已完成。当前阶段是 #129 Certified Dataset 受控试点。QualityAssessment 核心已由 #140 落地；#131 仅保留 review 后新增的 CostAllocation 等 follow-up。

第一阶段主任务：

- #131 QualityAssessment follow-up / CostAllocation
- #132 Quality Engine
- #133 Quality Report
- #137 Data Rights Provenance
- #134 DatasetCertification
- #135 API / UI
- #136 E2E Pilot

除非对应 Issue 明确要求，第一阶段不要主动加入：

- T4/T5/T6 全套生产可靠性路线作为前置
- 完整 Billing / Settlement / ERP
- 自建区块链平台
- 完整 IAM / 灾备 / 大规模性能平台
- 复杂 Service Mesh / 过早微服务拆分
- AI 黑盒风控模型
- 高敏门禁 / 人脸 / 视频数据
- Label Studio / X-AnyLabeling / Gold Dataset 第二阶段实现
- 完整法律合同管理或自动法律推理

文档基线见 docs/product/certified-dataset-pilot.md、docs/architecture/certified-dataset.md、docs/architecture/data-rights-provenance.md。

## 17. Review Convergence and Merge Contract

目标不是“让自动 reviewer 零建议”，而是让 PR 在**明确、有限、可验证的 merge contract** 内收敛。任何 substantial PR / architecture baseline 在进入重复 review 前，必须明确：

1. **Goal**：本 PR 要交付什么业务结果；
2. **Authoritative sources**：哪些文件/Issue 是该主题唯一或主要 contract，避免同一 invariant 在多处各自演化；
3. **In-scope invariants**：本 PR 必须保证的安全、历史一致性、事务/并发、兼容性边界；
4. **Explicit non-goals / follow-ups**：哪些能力明确转后续 Issue；
5. **Merge blockers**：只有以下类别默认阻塞合并：
   - security / authorization bypass、数据或 credential 泄漏；
   - 历史事实、快照、账务等可被静默改写/丢失；
   - 两个 authoritative contracts 明确冲突，按文档实现会得到不同安全语义；
   - 当前 Issue/PR 的必需路径不可实现、CI/build/test 失败；
   - 并发/幂等缺陷会破坏本 PR 已声明的核心 invariant；
6. **Follow-up by default**：性能优化、额外 hardening、未来生命周期、可选 observability、超出当前 Issue 的新实体/新状态/新产品能力，若不满足上述 blocker 条件，应创建/更新 follow-up Issue，而不是继续扩大当前 PR。

Review 处理规则：

- 同一轮出现多条相邻问题时，先按**根因**聚类并横向修复 authoritative contracts，不逐 comment 打补丁；
- 修复 review comment 新引入实体/状态/协议时，必须检查它是否是现有 merge contract 的必要推论；如果不是，转 follow-up，不把“review 建议”自动升级为当前 scope；
- review 累积达到 **3 个 substantive rounds 或 20 条已解决 comments** 后，必须停止机械 comment-by-comment 模式，执行一次 **convergence checkpoint**：
  - 冻结当前 merge contract；
  - 做 dependency/closure audit；
  - 把后续新意见分类为 BLOCKER / FOLLOW-UP / INVALID-OR-OUT-OF-SCOPE；
  - 只有 BLOCKER 可以重新打开当前 scope；
- architecture/docs PR 的 closure audit 至少检查：source-of-truth 一致性、状态机、历史不可变性、幂等、并发线性化、安全边界、跨 Issue ownership；不要无限追求所有未来极端场景都在当前 PR 完成；
- CI 绿色、closure audit 无 blocker、所有 blocker review threads 已解决时，PR 即可视为 merge candidate；**存在非阻塞自动建议不等于不能合并**；
- 如果 reviewer 提出的问题已由现有 open Issue 明确跟踪，且当前 PR 没有错误声称该能力已实现，应引用该 Issue 并保持为 follow-up，不在当前 PR 重复实现；
- 每次 closure checkpoint 应在 PR 留一条简短评论，记录：当前 HEAD、merge contract、剩余 blocker=0/列表、转 follow-up 的 Issue，作为后续 reviewer/执行线程的共同边界。
