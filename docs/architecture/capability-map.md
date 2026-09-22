# Capability Map

本文档定义 Data Product Platform 的能力归属，防止 Core 重复实现成熟通用基础设施。

核心原则：

> **Core owns semantics; OSS owns commodity capabilities.**

本表描述的是**能力边界与默认选型方向**，不是声明所有候选组件已经部署。实际引入新组件仍需对应 Issue / ADR / Adapter contract。

## 1. Capability ownership

| Capability | Ownership | Current / preferred implementation | Boundary |
|---|---|---|---|
| Identity / Authentication | External commodity | 企业 OIDC；默认候选 ZITADEL / Keycloak | Core 只消费可信 Principal，不实现密码、MFA、Token 服务 |
| Workspace RBAC | External commodity + platform mapping | 优先 Casbin；复杂跨系统 policy 才评估 OPA | Domain 定义“需要什么权限”，Policy Engine 执行通用授权判断 |
| Metadata Ingestion / Governance Catalog | External projection | OpenMetadata | OpenMetadata 发现/投影结构、血缘、使用等元数据；不负责把业务数据搬入 DatasetVersion |
| Data Ingestion / Integration | External commodity + platform acceptance boundary | 当前 Native CSV/File slice；真实异构接入时通过 provider-neutral Adapter 评估 SeaTunnel / InLong / Airbyte / NiFi / Debezium / Flink CDC 等 | 外部系统负责 connector、snapshot/incremental/CDC、checkpoint/offset 与技术重试；Core 负责 DataResource、接入结果验收、不可变 RAW DatasetVersion、业务 provenance、Evidence/Audit/Cost/Rights |
| Object Storage | External commodity | MinIO / S3-compatible | Core 通过 storage Port 使用，不依赖具体 SDK 语义 |
| Async Queue / Worker | Platform infrastructure | **优先复用仓库现有 queue / worker / reconciliation** | 不因存在 Asynq 就平行建设第二套队列 |
| Durable long-running workflow | External commodity when justified | 默认不新增；现有执行模型不足时再评估 Temporal | 只有跨天等待、人工 Signal、复杂恢复真正需要时引入 |
| Entity matching algorithm | External / pluggable engine | Rules + Splink Adapter；未来可替换 | Core 自有 MappingDecision / Evidence / Human Override 语义 |
| Quality execution engine | Pluggable engine | Native / industry-pack；可接外部 engine | Core 自有 QualityAssessment 历史事实与 gate 语义 |
| Annotation | External commodity | Label Studio / X-AnyLabeling（后续阶段） | 标注系统不成为 Core System of Record |
| Observability | External commodity | OpenTelemetry + Prometheus/Grafana（按阶段引入） | Audit/Evidence 不是 observability 的替代品，反之亦然 |
| Resumable large-file upload | External commodity when needed | 先流式 HTTP；GB 级/断点需求再评估 tus/tusd | DatasetVersion 仍由 Core 在上传完成后建立业务事实 |
| Database | External commodity | PostgreSQL | Domain invariant 仍由 Core + DB constraints 共同保证 |
| Tenant DB isolation | Database capability | workspace-scoped repository；生产多租户阶段评估 PostgreSQL RLS | RLS 是兜底，不替代可信 Principal 与应用授权 |

## 2. Core-owned product semantics

以下能力属于平台差异化价值，应由 Core 定义，而不是交给外部组件决定最终业务真相：

- DataResource / Dataset / DatasetVersion；
- Entity / EntityMapping / immutable MappingDecision；
- Workflow / Execution 的业务生命周期、输入输出和 frozen dependencies；
- QualityAssessment 与质量门禁事实；
- RightsDeclaration / Authorization / RightsSnapshot / Effective Rights；
- CostEvent / CostAllocation；
- Evidence / EvidenceSnapshot / AuditEvent；
- CertificationProfile snapshot / DatasetCertification；
- DataProduct / ProductVersion / ProductRelease；
- ReleaseReadiness / CurrentDeliveryGate；
- Industry Pack 中的领域规则、质量规则、认证规则和产品模板。

外部系统可以**执行能力**，但不得取代上述事实：

```text
External Engine / OSS
        │
        │ Adapter result / external reference
        ▼
Data Product Platform Core
        │
        ├── validate domain invariant
        ├── persist platform fact
        ├── append Evidence / Audit / Cost
        └── emit Domain Event / Outbox
```

## 3. Build-vs-Buy / Reuse Check

任何新基础设施需求进入开发前，按以下顺序检查：

```text
仓库已有能力？
   │ yes → 复用
   no
   ↓
当前依赖已有能力？
   │ yes → 复用
   no
   ↓
成熟 OSS / 标准方案？
   │ yes → Port / Adapter 集成
   no
   ↓
是否核心差异化语义？
   │ yes → 实现最小 Core 能力
   no  → 简化需求 / 延后
```

### 引入 OSS 的最低检查项

- License 是否满足产品使用与分发；
- 项目是否持续维护；
- API / protocol 是否稳定；
- 是否支持 self-host / 当前部署环境；
- 数据与身份边界是否可控；
- 是否能通过 Port / Adapter 隔离；
- 是否会与仓库现有能力形成第二套平行运行时；
- 退出/替换成本是否可接受。

## 4. 当前明确的“不重复建设”约束

- 不自建 IAM、密码、MFA、OIDC Provider；
- 不因为 Entity Resolution 需要异步化就再造 Scheduler；先复用现有 queue / worker / reconciliation；
- 不复制 OpenMetadata 的 catalog / lineage / glossary 产品能力到 Core；
- 不在 Core 为每一种数据库/API/消息系统分别自研 connector、增量游标或 CDC runtime；当前 CSV 只是第一条窄接入切片；
- 不在 Core 自研通用标注 UI；
- 不把 Splink 等算法引擎的数据模型直接变成 Core Domain；
- 不为了“未来可能需要”提前引入 Temporal、OPA、Service Mesh 等重量组件；
- 不 Fork OpenMetadata、Label Studio、Keycloak/ZITADEL 等大型项目，除非 ADR 证明 Adapter / Extension 无法满足。

## 5. 变更规则

新增或改变本表中的能力归属时：

1. 更新本文件；
2. 若改变长期架构边界，新增或更新 ADR；
3. PR 中说明是否新增运行时组件；
4. 新 Adapter 必须有 contract test；
5. 不得在未更新边界文档时让第三方 SDK 类型泄漏进 Core Domain。
