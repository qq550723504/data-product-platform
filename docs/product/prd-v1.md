# Data Product Platform PRD V1.1

> V1.1 基线：核心 POC 已完成，当前进入 Certified Dataset 受控试点。历史 POC 范围保留在 docs/poc/poc-technical-plan-v1.md。

## 1. 产品目标

将分散、异构、权利边界不清晰的原始数据，持续生产为：

1. 可理解、可治理、可交付、可审计的 Certified Dataset；
2. 基于这些数据集构建的 Data Product / Product Release。

## 2. 非目标

V1.1 第一阶段不定位为：

- 完整元数据平台
- 完整 ETL/湖仓平台
- 完整可信数据空间基础设施
- 数据交易所
- 财务 ERP
- 自动法律所有权裁判系统
- 完整合同管理系统
- 通用 AutoML / 特征平台
- 一次性完成生产 IAM / 灾备 / 大规模性能平台
- AI Gold Dataset 标注平台

## 3. 用户角色

- 产品经理：定义 Use Case、Data Product、Certification 用途
- 数据管理员：资源盘点、平台 Owner / Steward、分类
- 数据工程师：接入、标准化、加工
- 业务专家：指标、实体匹配、人工复核
- 治理人员：质量标准、QualityAssessment
- 合规/法务：RightsDeclaration verification、Authorization、Compliance Review
- 数据交付人员：Certification / Current Entitlement / Delivery
- Product Owner：Data Product Release 与生命周期
- 系统管理员：用户、权限、系统配置

## 4. 核心流程

### Certified Dataset

~~~text
Create Use Case
→ Register Data Resource
→ Record / Verify Rights Provenance
→ Dataset / DatasetVersion
→ Standardization
→ Entity Resolution
→ Processing
→ Quality Assessment
→ Rights / Compliance / Contract
→ Dataset Certification
→ Certified DatasetVersion
~~~

### Data Product

~~~text
Certified or governed DatasetVersion
→ Data Product / ProductVersion
→ Release Readiness
→ ProductRelease
~~~

第一阶段不强制所有 ProductRelease 只能引用 Certified DatasetVersion。

## 5. 一级模块

正式产品规划：

- 工作台
- 场景中心
- 数据资源
- 数据集
- 数据生产
- 实体中心
- 权利中心
- 数据质量
- 数据认证
- 安全合规
- 指标中心
- Data Contract
- 数据产品
- 成本中心
- 证据中心
- 可信流通
- 资产化准备
- 行业模板
- 系统管理

当前试点优先扩展现有 DatasetVersion 页面，不先扩张大量一级导航。

## 6. 核心业务对象

### 基础与生产

- Workspace / Project
- UseCase / ProductOpportunity
- DataResource / ResourceBinding
- Dataset / DatasetVersion / DatasetVersionLineage
- EntityType / Entity / EntityMapping / EntityMappingDecision
- Workflow / WorkflowVersion / Execution
- ExecutionDependencyBinding / MappingUsage

### Rights

- RightsDeclaration（#137）
- RightsVerification / RightsDisposition facts（#137）
- AuthorizationProvenanceBinding / AuthorizationProvenanceBindingDisposition（#137）
- Authorization / ResourceGrant
- RightsSnapshot
- EffectiveRights（#137）

owner_id 仅表示平台资产责任/归属，不作为法律所有权证明。

### Quality

- QualityRuleSet
- QualityAssessment（已通过 #140 / migration 000019 落地，兼容存储名 quality_result）
- QualityFinding
- QualityDimensionSummary

V1.1 六个质量维度：

- Completeness
- Accuracy
- Consistency
- Uniqueness
- Timeliness
- Traceability

不强制全行业统一总分。

### Certification

- CertificationProfile（#134）
- DatasetCertification / CertificationDisposition（#134）

认证判断的是“DatasetVersion X 是否满足 CertificationProfile Y”。

### Product / Governance

- CompliancePolicy / ComplianceResult
- DataContract / ContractVersion
- DataProduct / ProductVersion / ProductAsset / ProductRelease
- DeliveryOperation（#135）
- CostEvent / CostAllocation
- Evidence / EvidenceSnapshot / AuditEvent

## 7. 状态语义

### DatasetVersion

~~~text
CREATED → PROCESSING → READY
              └──────→ FAILED

READY → INVALID / SUPERSEDED
~~~

READY 表示数据内容已产生并冻结，不等于质量通过或认证通过。

### Authorization

~~~text
DRAFT → REVIEWING → APPROVED → ACTIVE
          └→ REJECTED           ├→ SUSPENDED
                                ├→ REVOKED
                                └→ EXPIRED
~~~

### RightsDeclaration

RightsDeclaration 对 CurrentEntitlementGate 使用的 resource、consumer applicability、purpose、action、scope、validity 必须强类型持久化并可查询；不能只靠 JSONB。

声明与验证事实分离。同一个 RightsDeclaration 只能有一个 terminal RightsVerification outcome：VERIFIED 或 REJECTED，Verify/Reject 互斥且 outcome 不可翻转。错误 VERIFIED 通过 RightsDisposition INVALIDATED/SUPERSEDED 退出 current set，并以新 RightsDeclaration + 新 verification 修正；不得在同一 declaration 上追加 REJECTED 来覆盖 VERIFIED。

### QualityAssessment

每一次 Assessment 是不可变评测事实，不通过通用 PATCH 改写结果。

### DatasetCertification

~~~text
Evaluate Certification
        ↓
CERTIFIED
或
REJECTED
~~~

第一阶段即支持 append-only CertificationDisposition：

- REVOKED：撤销旧认证的当前可用性；
- SUPERSEDED：显式指向 replacement certification。

旧 Certification 不覆盖、不删除；CurrentCertificationGate 按 as_of 排除已生效 disposition。

### ProductRelease

当前 live 可达状态：

~~~text
DRAFT
→ VALIDATING
└→ READY
   → PUBLISHED
~~~

Readiness 不满足时当前实现保持 `VALIDATING`，不会写成 `FAILED`。

`FAILED` / `SUSPENDED` / `WITHDRAWN` 仍是 domain/schema reserved values，但当前没有对应可达 Command：
- FAILED 当前不由 ValidateRelease 产生；
- SUSPENDED/WITHDRAWN 当前受 published-row guard 阻止，且无对应 Command。

V1.1 不把这些状态描述为当前可达迁移；未来启用必须单独设计 migration + Command，并继续冻结 published bindings。

## 8. Quality Assessment

QualityAssessment 必须绑定：

- DatasetVersion
- RuleSet ref/version
- RuleSet content hash / snapshot
- evaluator identity/version
- dimension summaries
- rule findings
- gate decision
- Evidence / Audit

规则文件后续改变不得改变历史 Assessment 的解释。

## 9. Data Rights

目标权利链：

~~~text
DataResource
→ RightsDeclaration
→ AuthorizationProvenanceBinding
→ Authorization
→ RightsSnapshot
→ Effective Rights
~~~

第一阶段动作至少覆盖：

- USE
- PROCESS
- DERIVE
- SHARE
- RAW_EXPORT
- RESALE
- AI_TRAINING

多源衍生数据的 Effective Rights 默认 fail closed：任一必要输入禁止某动作，输出不得自动获得该动作。

平台不自动判断现实世界法律所有权；平台保存可验证的声明、依据和 Evidence。

## 10. Dataset Certification

CertificationProfile 至少定义：

- purpose / applicability
- required quality dimensions / critical rules
- Rights requirements
- Compliance requirements
- Contract requirements
- Traceability / Evidence requirements

DatasetCertification 必须绑定明确 DatasetVersion、QualityAssessment、Profile snapshot/hash 及所需 Rights/Compliance/Contract/Evidence。

任何 required 条件缺失或不匹配时，不得 CERTIFIED。

Certification 表示认证时点结论，不等于永久交付授权。

Certified Dataset 每次实际交付前必须执行 CurrentDeliveryGate：

~~~text
CurrentDeliveryGate
├── DatasetVersionUsability
├── CurrentCertificationGate
└── CurrentEntitlementGate
~~~

- DatasetVersionUsability 至少阻断 INVALID / FAILED / PROCESSING / CREATED；SUPERSEDED 按平台既有“明确历史版本”语义处理，不在本 docs-only 基线中自动等同 INVALID；
- CurrentCertificationGate 要求本次 delivery 绑定明确 DatasetCertification，decision = CERTIFIED 且未在 as_of 时点被 CertificationDisposition REVOKED / SUPERSEDED；requested purpose/action/consumer/delivery context 必须被该 Certification 冻结的 CertificationProfile snapshot 覆盖；禁止以 latest created_at 猜当前认证；
- CurrentEntitlementGate 对**每一个绑定的 RightsDeclaration**按当前 `as_of` 校验：VERIFIED、declaration 自身 `effective_from/effective_to` 覆盖 `as_of`、resource/consumer/purpose/action/scope 与本次 delivery context 匹配、且未被已生效 INVALIDATED/SUPERSEDED disposition 排除；每个 Authorization 必须通过**当前有效、未被 AuthorizationProvenanceBindingDisposition INVALIDATED/SUPERSEDED 的** AuthorizationProvenanceBinding 证明其 grantor_ref 得到该 provenance 支持，并且 Authorization 自身也必须覆盖**同一 requested context**：grantee/consumer、resource、purpose、action、scope、validity/status。Declaration 允许某 consumer/action 不代表 Authorization 已授权该 consumer/action；任一层不匹配都 fail closed。

任一子门禁失败时，历史 Certification 保留，但当前交付必须 BLOCKED。第一阶段不要求周期性后台重认证。

Current Delivery Eligibility 查询仅用于展示/预检，不是授权凭证。第一阶段必须有真正的 server-side delivery command；服务端在返回数据或签发下载链接、presigned URL、token、credential 前必须重新执行完整 CurrentDeliveryGate。query 与 delivery 之间状态发生变化时，以 delivery command 内重新计算的当前事实为准。

签发 credential 时，`expires_at` 不得晚于 requested TTL、平台最大 TTL、本次 entitlement 所依赖所有 RightsDeclaration / Authorization 中最早的有限 `valid_to/effective_to`，以及签发时已存在且将在未来生效的 RightsDisposition / AuthorizationProvenanceBindingDisposition / CertificationDisposition 中最早的 `effective_at`。支持 redemption-time server check 的 delivery mode 应在 redemption 时再次执行 gate；不能回调平台的 bearer/presigned credential 必须严格执行该 expiry cap 和明确的短最大 TTL。

外部 credential issuance 必须 crash-safe：先持久化 DeliveryOperation + stable provider_request_key；每次 initial/retry/reconciliation 真正调用 provider 前重新执行 CurrentDeliveryGate 并重新计算 expiry cap，再决定是否允许外部 side effect。

同时必须存在 delivery/entitlement 线性化机制：所有 delivery mode 的 terminal ISSUED DB transaction 都必须获取与 Rights/Binding/Certification disposition、DatasetVersion invalidation 等 Command 共享的 delivery authorization fence/revision，重新执行 CurrentDeliveryGate，并验证 dependency revision 未被并发变更穿越；provider/credential 模式同时重算 fresh cap。该 terminal commit 是 delivery linearization point。

direct-data delivery 也不得例外：在 terminal ISSUED commit 成功之前，不得向 HTTP response/body/stream 写出任何数据字节；commit 成功后才开始传输。数据库 fence 只覆盖 terminal re-gate + commit，不在整个 stream 生命周期持续持锁。

如果 fresh gate 变为 BLOCKED，但该 DeliveryOperation 此前已经进入可能调用过 provider 的 ISSUANCE_PENDING/retry/reconciliation 窗口，**不能直接记录 BLOCKED**。必须先使用同一 provider_request_key reconciliation 既有 provider outcome：
- 明确未签发 → 可 BLOCKED；
- 已签发 → 必须先 revoke / compensate / contain，并确认外部访问能力已不可用后才能 BLOCKED；
- outcome unknown 或 containment 未确认成功 → 保持 CONTAINMENT_PENDING，不得发 DatasetDeliveryBlocked terminal event，也不得向用户声称不存在活跃访问能力。

**Direct bearer delivery 的 provider 必须支持按同一 provider_request_key replay/read-after-write 恢复同一 credential（或等价同一访问能力）。仅支持 revoke/compensation、却无法恢复原 bearer secret，不足以支持 direct bearer**：terminal ISSUED commit 成功但 HTTP response 丢失后，客户端幂等 retry 必须仍能获得原访问能力。此类 provider 第一阶段必须使用 platform redemption indirection，或明确不支持 direct bearer mode。

provider 成功但 terminal DB commit 失败时，retry/reconciliation 复用同一 key，不得产生第二份独立 credential。

任何首次返回或 reconciliation 恢复出的 credential，在 DeliveryOperation 进入 ISSUED 前都必须验证其**实际 provider capability 与当前 allowed/requested context 等价或更窄**：expiry 不晚于 fresh cap，resource/DatasetVersion、consumer、action、object/row/prefix scope、delivery channel 不得扩大。如果 disposition/validity 在 prepare 后缩短了 cap，或 provider 实际 capability 比请求更宽，则不能直接恢复为 ISSUED：必须安全 shorten/narrow 并通过 read-after-write/authoritative lookup 验证，或 revoke/contain；任何关键 capability 维度无法验证时 fail closed。

## 11. CostEvent / CostAllocation

QualityAssessment、Rights verification / invalidation / supersession、Authorization provenance binding、DatasetCertification evaluation / human approval、Delivery 等实际活动发生时必须记录 CostEvent。

- 金额未知时不伪造金额，可记录 quantity/unit；
- 成本必须与实际活动同时记录，不在试点 KPI 阶段事后反推；
- 非 Execution 成本必须通过 typed CostAllocation 关联实际业务主体，禁止仅把 subject ID 放 JSONB metadata；
- CostEvent 使用稳定 activity_id / operation identity，并以 component_key / cost_type 区分同一业务活动内不同成本组件；
- 相同 activity_id + component_key 的幂等业务重放不得重复产生 CostEvent。

## 12. Product Release Readiness

Release 发布前统一检查：

- Production
- Rights
- Quality
- Compliance
- Contract
- Dataset
- Evidence
- Delivery

ProductRelease Readiness 与 DatasetCertification 不互相替代。

## 13. 第一 Reference Implementation

POC 已验证 Raw / Standardized / Curated、Entity Resolution、Workflow / Execution、Quality / Compliance、Contract、Product Release、Cost / Evidence 和生产 decision trace。

Pilot 在同一链路增加：

- QualityAssessment rule snapshot
- 六维 Quality Report
- Data Rights Provenance
- Effective Rights
- CertificationProfile
- DatasetCertification
- Certified Dataset UI/API

最终至少产生一个由 reference fixture 驱动的 CERTIFIED CURATED DatasetVersion，并同时验证失败路径。

## 14. 第一阶段非目标

- T4/T5/T6 全部生产可靠性实现作为前置
- 完整 Billing / Settlement / Asset Accounting
- 区块链平台
- 高敏人脸/门禁/视频场景
- 复杂 AI 黑盒评分
- Label Studio / X-AnyLabeling / Gold Dataset
- 通用 CertificationProfile 在线设计器
- 完整法律合同管理与自动法律推理

## 15. 技术方向

- Frontend：React / Next.js
- Backend：Go
- Control DB：PostgreSQL
- Queue：Redis
- Object Storage：MinIO / S3 compatible
- Metadata Projection：OpenMetadata
- Processing Adapter：Native/Python/Hop
- Entity Adapter：Rules/Splink
- Quality Adapter：Native，未来可接 Soda/GX
- Compliance Adapter：Rules，未来可接 Presidio
- Annotation Adapter：第二阶段可接 Label Studio / X-AnyLabeling

Core Platform 必须与具体 Engine 解耦。
