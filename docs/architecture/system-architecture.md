# System Architecture V1.0

## 1. Architecture Style

POC 采用 **Modular Monolith + Engine Adapters + Transactional Outbox**。

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

## 2. Core / Engine Boundary

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

## 3. Control Plane / Data Plane

### Control Plane

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

### Data Plane

真实数据放在：

- Object Storage
- PostgreSQL / Doris / warehouse
- 其他外部数据系统

平台核心数据库不用于承载大规模 Dataset 内容。

## 4. Governance Projection

OpenMetadata 作为 Governance Projection：

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

- Technical Metadata
- Technical Lineage
- Domain / Glossary / Classification
- Governance view of Data Product

它不拥有：

- Rights
- DatasetVersion
- Production Workflow
- Cost
- Evidence
- ProductRelease

## 5. Event Model

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

## 6. Worker Responsibilities

- Outbox consumption
- Workflow task scheduling
- Engine polling / reconciliation
- Authorization expiry
- Evidence generation
- Cost aggregation
- Metadata projection retry

## 7. Industry Packs

行业 Pack 只提供：

- Entity Type
- Glossary
- Standardization rules
- Entity matching policies
- Indicators
- Quality / Compliance rules
- Product templates

禁止行业逻辑污染 Core Domain。