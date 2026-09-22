# 系统架构 V1.2

## 1. 架构风格

当前采用模块化单体 + 引擎适配器 + 事务性 Outbox。

~~~text
Web / Next.js
      │
      ▼
platform-api (Go)
      │
      ├── Core Modules
      │   ├── resource / dataset
      │   ├── entity
      │   ├── workflow
      │   ├── rights
      │   ├── quality
      │   ├── certification   (#134 起)
      │   ├── compliance
      │   ├── contract
      │   ├── product
      │   ├── cost
      │   └── evidence
      │
      ├── PostgreSQL
      ├── Redis
      └── Object Storage

platform-worker (Go)
      │
      ├── Outbox dispatcher / handlers
      ├── Workflow execution
      ├── Maintenance / reconciliation
      └── Projection jobs

Data Ingestion Adapter Layer
      └── DataIngestionProvider → Native CSV/File / future external ingestion engine

Engine Adapter Layer
      ├── MetadataEngine → OpenMetadata
      ├── ProcessingEngine → Native/Python/Hop
      ├── EntityResolutionEngine → Rules/Splink
      ├── QualityEngine → Native / future Soda/GX
      ├── ComplianceEngine → Rules / future Presidio
      └── AnnotationEngine → future Label Studio/X-AnyLabeling
~~~

## 2. Core Platform 业务真相

Core 保存：

- DataResource / DatasetVersion
- RightsDeclaration / verification / disposition / Authorization / RightsSnapshot / Effective Rights
- EntityMappingDecision
- Workflow / Execution / frozen execution dependencies
- QualityAssessment
- Contract / Compliance
- DatasetCertification
- ProductVersion / ProductRelease
- Cost / Evidence / Audit

其中 QualityAssessment 核心已由 #140 / migration 000019 落地；#137 Rights / Effective Rights 已进入 main；#134 Certification 已由 PR #149 进入 main；DeliveryOperation 核心切片已进入 main，但 #135 API/UI 与剩余交付范围仍开放。

外部 Engine 只提供执行能力，不拥有上述核心业务状态。

## 3. 控制面 / 数据面

### 控制面

PostgreSQL 保存业务元数据、不可变版本和证明：

- Resource / Dataset metadata
- DatasetVersion / lineage
- Entity decisions
- Rights provenance / authorization / snapshots
- Workflow / Execution
- Quality assessments
- Certification
- Product / Release
- Cost / Evidence

### 数据面

真实大规模数据放在：

- Object Storage
- PostgreSQL / Doris / warehouse
- 其他外部数据系统

核心控制数据库不承载大规模 Dataset 内容。

### 数据接入边界

Data Ingestion / Integration 是独立的 commodity capability，与 OpenMetadata 的 Metadata Ingestion 明确分离：

~~~text
External Data Sources
        │
        ├── Metadata Ingestion ──→ OpenMetadata（治理投影）
        │
        └── Data Ingestion ──────→ DataIngestionProvider
                                      │
                                      ├── 当前：Native CSV/File slice
                                      └── 后续：可替换外部 ingestion engine
                                      │
                                      ▼
                              Core acceptance boundary
                                      │
                                      ▼
                              RAW DatasetVersion
~~~

当前 `/ingest` CSV 路径只是第一条窄接入切片，不作为未来数据库、API、消息系统、批量同步或 CDC 的通用实现模板。

外部 ingestion engine 可以拥有 connector、technical schema discovery、snapshot/incremental/CDC 执行、checkpoint/offset、分区和 engine-internal retry；这些运行时细节默认不复制成 Core 业务状态。Core 保留 DataResource、接入结果与 workspace/source 的业务绑定、不可变 DatasetVersion、业务 lineage、Evidence/Audit/Cost、Rights 与后续 Certification 的 System of Record。

外部 job 成功不自动等于 DatasetVersion 可用。Core 必须在 acceptance boundary 验证所接纳产物/manifest 的身份、完整性及不可变 content identity，并显式建立或完成 RAW DatasetVersion 事实。具体 contract 在首个真实异构接入 vertical slice 中定义，并要求 Adapter contract test。

持续 CDC stream 本身不是 DatasetVersion。需要进入质量评测、认证或交付链时，必须按明确 snapshot/window/cut 边界发布新的不可变 DatasetVersion；不得让已认证 DatasetVersion 指向持续原地变化的数据。外部 checkpoint/offset 默认由 ingestion engine 持有，仅在解释接入边界确有需要时以 provider-neutral provenance/evidence 引用。

同一同步链路只能有一个调度/重试责任方。Core 可以发起、观察或 reconciliation 外部任务，但不得与外部 ingestion engine 同时维护互相竞争的 scheduler/retry 状态机。

当前不选定 SeaTunnel、InLong、Airbyte、NiFi、Debezium 或 Flink CDC 等具体实现；真实数据库/API/CDC 场景出现时按 ADR-0010 做 Build-vs-Buy / Reuse Check 后选择最小合适方案。

## 4. Certified Dataset 生产链

~~~text
DataResource
  ↓
Rights Provenance
  ↓
DatasetVersion
  ↓
Entity Resolution / Processing
  ↓
QualityAssessment
  ↓
Effective Rights / Compliance / Contract
  ↓
DatasetCertification
  ↓
Certified DatasetVersion
~~~

Certified Dataset 可以作为独立交付对象，也可以继续进入 Data Product / ProductRelease；独立交付必须由 server-side delivery command 执行。该 Command 先从 authenticated caller principal 解析 effective consumer/workspace；on-behalf-of 必须验证当前有效 delegation，不能信任请求 consumer 自证身份。随后在返回数据或签发 URL/token/credential 前重新执行 CurrentDeliveryGate：检查 DatasetVersion 当前可用性、CurrentCertificationGate（明确且未 REVOKED/SUPERSEDED 的 CERTIFIED 事实），再通过 CurrentEntitlementGate 重新校验当前 Rights provenance / Authorization / Effective Rights。Eligibility query 不能替代 delivery-time authorization。

外部 credential issuance 使用 DB-first crash-safe protocol：
1. 先持久化 DeliveryOperation PREPARED/ISSUANCE_PENDING + stable provider_request_key；
2. DB commit 成功后才执行外部 issuance；
3. 每次真实 provider invocation 在调用前先 durable persist physical provider-attempt identity；success / failure / timeout / unknown / reconciliation / revoke / compensation 只要实际外部调用并可能计费，都按该 attempt 记录 CostEvent；provider 成功后再提交 terminal DeliveryOperation + Audit/Evidence/Outbox（terminal-specific CostEvent 如有）；
4. terminal commit 成功后才向客户端暴露 credential；
5. 每次 initial/retry/reconciliation 真正调用 provider 前重新验证 caller principal→effective consumer/workspace binding/delegation，再执行 CurrentDeliveryGate 并重新计算 expiry cap；prepare 阶段的旧 identity/gate snapshot 不授权后续外部 side effect；
6. 所有 delivery mode 的 terminal finalize 都必须在共享 delivery authorization fence/revision 下重新读取 current facts、重新验证 caller authority、重新 gate；credential/provider 模式还要重新计算 fresh cap。影响 delivery authorization 的 principal binding/workspace membership/caller delegation lifecycle、grantor-authority delegation edge/disposition lifecycle、Rights/Binding/Certification disposition 与 DatasetVersion invalidation 等 Command 使用同一 fence/revision，并按固定顺序锁定；
7. terminal ISSUED DB commit 是 delivery 的线性化点：provider/credential 模式只有 commit 后才返回 capability；direct-data 模式只有 commit 后才允许写出第一字节。若 entitlement 变更先提交，finalize 必须看到它且 direct-data 0-byte fail closed；若 finalize 先提交，则后续 entitlement 变更在线性顺序上发生在该 delivery 之后；
8. direct-data 不得在整个 stream 期间持有数据库 lock；fence 只覆盖 terminal re-gate + commit。terminal ISSUED 仅表示该 attempt 已获准开始响应，不证明客户端收到全部 bytes；ISSUED 后第一字节前 crash/socket loss 或 stream 中断时，同一 idempotency key 不得基于旧 gate 重放数据，只能返回稳定 non-payload replay-required 结果。重新传输必须创建新的 DeliveryOperation/attempt（新 idempotency key，可关联 retry_of），重新解析 caller principal→consumer/delegation、重新 CurrentDeliveryGate、重新进入 fence/finalize；两次 attempt 间的任何 revocation/invalidation 必须阻断新 attempt。
9. provider capability 在 ISSUED 前必须通过 read-after-write/authoritative lookup 验证为 requested/current-gate context 的等价或更窄集合，至少覆盖 expiry、resource/DatasetVersion、**consumer/grantee enforcement（必需）**、actions、object/row/prefix scope、delivery channel；consumer/grantee 无法由 provider 原生或等价 holder-bound mechanism 强制/验证时，不得 direct bearer/presigned ISSUED，必须使用 platform redemption/gateway 或标记 direct mode unsupported；其它 scope 过宽或不可验证同样 fail closed 并 contain/narrow；
10. crash/timeout 由 reconciliation 使用同一 provider_request_key 恢复，不盲目重复签发；provider outcome/containment 未确认而进入 CONTAINMENT_PENDING 时，状态 transition + `DatasetDeliveryContainmentPending`（或固定等价）+ Audit/Evidence/Outbox 同 transaction 提交，供 recovery/alert consumers 可靠消费；timeout/unknown 本次真实 provider attempt 的成本事实仍保留，不能因 outcome 未知或最终 FAILED/BLOCKED 而省略；
11. direct bearer delivery 只有在 provider 能按同一 key 恢复同一 credential/访问能力、验证实际 capability scope，且 fresh replay authorization 被拒绝时能 revoke/contain 既有 capability，才允许；否则使用 platform redemption indirection。
12. terminal ISSUED credential 的 same-key response replay 不继承旧授权。再次返回 credential/handle 前必须重新解析 authenticated caller→effective consumer/delegation，在共享 authorization fence 下重新 CurrentDeliveryGate + fresh cap，并验证 recovered same capability 仍满足当前边界；append replay decision 后只有 ALLOWED 才能返回。若已 BLOCKED，则不返回 credential/handle，先 revoke/contain；containment 未确认时只返回 non-secret pending 结果，原 ISSUED 历史事实不改写。

## 5. Governance Projection

OpenMetadata 是治理投影，不拥有：

- Rights
- DatasetVersion
- production execution facts
- QualityAssessment
- DatasetCertification
- Cost
- Evidence
- ProductRelease

Projection 故障不得改变 Core 业务真相。

## 6. 事件模型

关键业务动作在数据库事务内写业务事实、Audit/Evidence 和 Outbox。

外部副作用不属于 PostgreSQL transaction；必须在事务提交后执行，并通过稳定 idempotency key、read-after-write/reconciliation 或 revoke/compensation 处理 crash consistency。Delivery credential issuance 禁止用“外部调用 + DB commit 看起来像一个事务”的假原子模型。

新事件类型必须进入统一 routing 表，并显式声明 required handlers 或 retention-only。

## 7. Worker 职责

- Outbox dispatcher / handler
- Workflow 任务处理
- Engine 对账/维护任务（按已实施范围）
- DeliveryOperation ISSUANCE_PENDING reconciliation / provider outcome recovery（#135 起）
- 授权过期处理
- 元数据投影
- 后续可加入周期性质量/认证维护，但不属于当前 MVP 前置

## 8. Industry Pack

Industry Pack 提供：

- Entity Types
- Glossary
- Standardization Rules
- Matching Policies
- Indicators
- Quality Rules
- Compliance Rules
- Certification Profiles
- Product Templates

Core 不允许出现 PARK 等行业专属分支。

## 9. 当前阶段边界

当前是 #129 Certified Dataset 受控试点。

第一阶段优先验证业务闭环，不以以下事项作为前置：

- T4/T5/T6 全部可靠性实现
- 完整 IAM / 灾备 / 性能平台
- 微服务拆分
- Label Studio / X-AnyLabeling 第二阶段
- 数据市场 / Billing
