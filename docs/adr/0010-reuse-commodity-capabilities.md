# ADR-0010：优先复用成熟能力，Core 只拥有产品语义

- 状态：已接受
- 日期：2026-09-21

## 背景

Data Product Platform 已经具备较完整的 Core Domain，同时也会持续接入认证、元数据、对象存储、实体解析、异步执行、标注、可观测性等通用能力。

如果每个需求都在仓库内重新实现，会产生两类问题：

1. 重复建设成熟基础设施，把研发资源消耗在非差异化能力上；
2. 直接依赖第三方 SDK / 数据模型，又会让 Core 被某个产品锁定。

因此需要同时避免两种极端：

- **重复造轮子**；
- **无边界堆叠开源组件**。

## 决策

采用原则：

> **Core owns semantics; OSS owns commodity capabilities.**

### 决策顺序

新增基础能力时必须按以下优先级：

1. 复用仓库现有能力；
2. 复用当前依赖已提供的能力；
3. 通过 Port / SPI + Adapter 集成成熟 OSS / 标准协议；
4. 仅在前三项无法满足领域 invariant 时，实现最小必要自研能力；
5. Fork 是最后手段。

第三方集成优先级固定为：

```text
Native API / Standard Protocol
→ Plugin / Extension
→ Adapter
→ Sidecar
→ Fork
```

### Core 必须拥有的内容

平台自己拥有业务事实和不变量，包括：

- DatasetVersion；
- EntityMapping / MappingDecision；
- Execution lifecycle / frozen dependencies；
- QualityAssessment；
- Rights / Effective Rights；
- Cost / Evidence / Audit；
- DatasetCertification；
- ProductVersion / ProductRelease / ReleaseReadiness；
- Industry Pack 领域语义。

外部产品不得成为这些事实的最终 System of Record。

### 默认不自研的内容

以下属于 commodity capability，默认优先采用成熟实现：

- Identity / OIDC / OAuth2 / MFA / SSO；
- 通用 RBAC / ABAC / Policy Engine；
- 对象存储；
- 通用任务队列与 durable workflow runtime；
- Metadata Catalog；
- 通用标注系统；
- tracing / metrics / dashboards；
- resumable upload；
- 通用搜索基础设施。

### 不为“用开源”而引入开源

成熟 OSS 只是一种复用方式，不是目标。

如果仓库已有 queue / worker / reconciliation 能满足需求，就不因为 Asynq 或 Temporal 更知名而再建设第二套运行时。

若新组件带来的运维、数据迁移、故障恢复和双写成本高于其收益，应维持现有实现。

## Build-vs-Buy / Reuse Check

任何新增基础设施能力的 Issue / PR 必须回答：

1. 仓库已有能力为什么不足？
2. 当前依赖为什么不足？
3. 可用的成熟 OSS / 标准方案是什么？
4. 为什么选定方案不会把第三方业务语义泄漏进 Core？
5. 若选择自研，为什么这是核心差异化能力或不可避免的领域 invariant？
6. 若选择 Fork，为什么 API / Extension / Adapter 无法完成？

## 后果

### 正面

- 研发资源集中在数据产品核心语义；
- 减少重复基础设施代码；
- 外部系统可替换；
- 不被单一 OSS 产品的数据模型锁定；
- Agent / Codex 在开发前有统一决策门槛。

### 代价

- 每个外部能力需要明确 Port / Adapter contract；
- 某些场景需要额外做映射层；
- 引入 OSS 前仍需评估 license、维护活跃度、部署和退出成本；
- 不能以“已有开源组件”为理由跳过领域 invariant 设计。

## 关联

- `AGENTS.md §1 Core Domain Independence`
- `AGENTS.md §1A Commodity Capability Reuse`
- ADR-0002 OpenMetadata Governance Projection
- ADR-0004 Engine SPI
- `docs/architecture/capability-map.md`
