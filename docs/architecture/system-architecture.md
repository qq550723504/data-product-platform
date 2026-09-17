# 系统架构 V1.0

## 1. 架构风格

POC 采用 **模块化单体 + 引擎适配器 + 事务性 Outbox**。

```text
Web / Next.js
      │
      ▼
platform-api (Go)
      │
      ├── Core Modules
      │   ├── usecase
      │   ├── resource
      │   ├── dataset
      │   ├── entity
      │   ├── workflow
      │   ├── rights
      │   ├── quality
      │   ├── compliance
      │   ├── contract
      │   ├── product
      │   ├── cost
      │   └── evidence
      │
      ├── PostgreSQL
      ├── Redis
      └── MinIO/S3

platform-worker (Go)
      │
      ├── Outbox handlers
      ├── Workflow orchestration
      ├── Engine reconciliation
      └── Maintenance jobs

Engine Adapter Layer
      ├── MetadataEngine → OpenMetadata
      ├── ProcessingEngine → Native/Python/Hop
      ├── EntityResolutionEngine → Rules/Splink
      ├── QualityEngine → Native/Soda/GX
      └── ComplianceEngine → Rules/Presidio
```

## 2. 核心 / 引擎边界

Core Platform 管理业务真相：

- DatasetVersion
- EntityMapping
- Authorization
- WorkflowRun / Execution
- Quality / Compliance decision
- DataContract
- ProductVersion / ProductRelease
- Cost / Evidence / Audit

Engine 只提供执行能力。

外部 Engine 状态不得直接替代 Core 状态。例如：

- `execution.id` 是平台业务 ID；
- `engine_execution_id` 是 Hop/Python/Spark 外部引用。

## 3. 控制面 / 数据面

### 控制面（Control Plane）

PostgreSQL 保存：

- UseCase
- DataResource metadata
- Dataset metadata / versions
- Entity / Mapping
- Rights
- Workflow definition / execution metadata
- Contract
- Product / Release
- Cost / Evidence metadata

### 数据面（Data Plane）

真实数据放在：

- Object Storage
- PostgreSQL / Doris / warehouse
- 其他外部数据系统

平台核心数据库不用于承载大规模 Dataset 内容。

## 4. 治理投影

OpenMetadata 作为治理投影（Governance Projection）：

```text
Core Platform
    │
    ├── ResourceBinding
    └── ProductReleased event
             ↓
       Metadata Adapter
             ↓
       OpenMetadata
```

OpenMetadata 负责：

- 技术元数据（Technical Metadata）
- 技术血缘（Technical Lineage）
- 域 / 术语表 / 分类（Domain / Glossary / Classification）
- 数据产品的治理视图

它不拥有：

- Rights
- DatasetVersion
- Production Workflow
- Cost
- Evidence
- ProductRelease

## 5. 事件模型

关键状态变化产生 Domain Event，通过 Transactional Outbox 异步处理副作用。

示例：

```text
PublishProductReleaseCommand
        ↓
DB Transaction
  ├── Release = PUBLISHED
  └── outbox(ProductReleased)
        ↓
Worker
  ├── OpenMetadata projection
  ├── Evidence finalize
  ├── Search/index
  └── Notification
```

外部系统故障不得破坏核心发布事务。

## 6. Worker 职责

- Outbox 消费
- Workflow 任务调度
- Engine 轮询 / 对账（reconciliation）
- 授权过期处理
- 证据生成
- 成本聚合
- 元数据投影重试

## 7. 行业包

行业 Pack 只提供：

- Entity Type
- Glossary
- 标准化规则
- 实体匹配策略
- 指标（Indicators）
- 质量 / 合规规则
- 产品模板

禁止行业逻辑污染核心领域。
