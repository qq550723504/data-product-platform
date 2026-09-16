# Data Product Platform POC Technical Plan V1.0

## 1. POC Goal

Validate that the Core Platform can produce an immutable, explainable and traceable Data Product Release from raw data.

The POC is successful when the system can answer:

1. What source data produced this release?
2. Why were source records resolved to a canonical entity?
3. Which workflow / contract / rules / rights versions were used?
4. What quality and compliance decisions were made?
5. Which Cost Events occurred?
6. Which Evidence supports the release?

## 2. Reference Scenario

- Use Case: 企业融资风险辅助
- Data Product: 企业经营活跃度 V1.0
- Sources: enterprise, lease, energy
- Excluded from first POC: face, detailed access-control records, parking/video data

## 3. Technical Baseline

POC runtime:

- Web: React / Next.js
- API: Go
- Worker: Go
- PostgreSQL
- Redis
- MinIO/S3-compatible storage

Adapters are introduced after Core vertical slices are stable.

## 4. Sprint Plan

### Sprint 0 — Foundation

Deliver:

- repository skeleton
- API/worker entrypoints
- migrations
- PostgreSQL connection
- Redis queue/outbox worker
- object storage abstraction
- transaction manager
- error model
- audit foundation
- structured logs / trace ID

Acceptance:

- API starts
- worker starts
- migrations run
- object upload/download works
- outbox event is consumed

### Sprint 1 — Resource / Dataset / Entity / Evidence

Deliver:

- DataResource
- Dataset / DatasetVersion
- EntityType / Entity / EntityMapping
- CSV upload
- RAW Dataset V1
- company standardization
- exact/manual entity matching
- STANDARDIZED Dataset V1
- Evidence

Acceptance:

The system can trace Standardized V1 back to source CSV and explain each reviewed company mapping.

### Sprint 2 — Workflow / Product / Release

Deliver:

- Workflow / WorkflowVersion
- Task / Execution
- CURATED Dataset
- DataProduct / ProductVersion
- ProductRelease / ReleaseReadiness

Rights/Quality/Compliance may use mock providers in this sprint, but the real state machines must be used.

Acceptance:

A Product Release V1.0 can be validated and published, then becomes immutable.

### Sprint 3 — Real Gates / Contract / Cost

Deliver:

- Authorization basics
- Native Quality Engine
- Native Compliance rules
- DataContract / ContractVersion
- CostEvent
- EvidenceSnapshot

Acceptance:

Release publishing is blocked by failed rights/quality/compliance/contract readiness checks.

### Sprint 4 — OpenMetadata Adapter

Deliver:

- MetadataEngine interface
- OpenMetadata adapter
- ResourceBinding
- Product governance projection

Acceptance:

OpenMetadata outage does not invalidate Core Product Release; sync retries asynchronously.

### Sprint 5 — Apache Hop Adapter

Deliver:

- ProcessingEngine interface
- Hop adapter
- execution status reconciliation

Acceptance:

A native processing task can be switched to Hop without changing Dataset/Product/Release semantics.

### Sprint 6 — Splink Adapter

Deliver:

- EntityResolutionEngine interface
- Splink candidate generation
- confidence thresholds
- review queue integration

Acceptance:

Entity resolution metrics can report exact/probabilistic/manual/unresolved results and compare policy versions.

## 5. POC UI

First-level navigation:

- 工作台
- 场景
- 数据资源
- 数据集
- 数据生产
- 实体中心
- 数据产品
- 证据中心

Rights / Quality / Compliance / Contract / Cost appear as contextual tabs during POC.

## 6. First Vertical Slice

```text
enterprise.csv
→ DataResource
→ RAW Dataset V1
→ Company standardization
→ Entity Mapping / Human Review
→ STANDARDIZED Dataset V1
→ Evidence
```

No OpenMetadata, Hop or Splink dependency is required for this slice.

## 7. Second Vertical Slice

```text
enterprise + lease + energy standardized datasets
→ Workflow
→ indicators
→ CURATED Dataset
→ Quality / Compliance
→ Data Contract
→ Data Product 1.0
→ Product Release
→ EvidenceSnapshot + Cost
```

## 8. Final Demo

The demo must also simulate one failure condition, such as authorization expiry, and demonstrate impact analysis / health degradation or suspension.

## 9. POC Non-Goals

Do not implement during Core POC unless explicitly approved:

- full Billing / Settlement
- full accounting/ERP
- blockchain platform
- complex microservice infrastructure
- black-box financial risk scoring
- high-risk personal surveillance datasets
