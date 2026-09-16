# Data Product Platform POC Technical Plan V1.0

## 1. POC Goal

Validate that the Core Platform can produce an immutable, explainable and traceable Data Product Release from raw data.

The POC is successful when the system can answer:

1. What source data produced this release?
2. Why were source records resolved to a canonical entity?
3. Which workflow / indicator / contract / rules / rights versions were used?
4. What quality and compliance decisions were made?
5. Which Cost Events occurred?
6. Which Evidence supports the release?

Primary Epic: GitHub issue `#1`.

## 2. Reference Scenario

- Use Case: 企业融资风险辅助
- Data Product: 企业经营活跃度 V1.0
- Sources: enterprise, lease, energy
- Excluded from first POC: face, detailed access-control records, parking/video data

Canonical Reference Implementation specs:

- `examples/enterprise-activity/README.md`
- `examples/enterprise-activity/indicators-v1.md`
- `examples/enterprise-activity/test-vectors-v1.yaml`
- `examples/enterprise-activity/workflow/workflow-v1.yaml`
- `examples/enterprise-activity/contract/data-contract-v1.yaml`
- `industry-packs/park/matching/company-match-policy-v1.yaml`
- `industry-packs/park/indicators/enterprise-activity-v1.yaml`
- `industry-packs/park/quality/enterprise-activity-quality-v1.yaml`
- `industry-packs/park/compliance/enterprise-activity-compliance-v1.yaml`

The V1 indicator semantics are explainable POC rules, not a validated credit/underwriting model.

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

### Sprint 0 — Foundation (`#2`)

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

### Sprint 1 — Resource / Dataset / Entity / Evidence (`#3`, `#4`, `#8` baseline)

Deliver:

- DataResource
- Dataset / immutable DatasetVersion
- EntityType / Entity / EntityMapping
- CSV upload
- RAW Dataset V1
- company standardization
- exact/manual entity matching
- STANDARDIZED Dataset V1
- Evidence / Audit / baseline CostEvent

Acceptance:

The system can trace Standardized V1 back to source CSV and explain each reviewed company mapping.

### Sprint 2 — Workflow / Product / Release (`#5`, `#7`)

Deliver:

- Workflow / WorkflowVersion
- Task / Execution
- V1 indicator calculations from frozen spec
- CURATED Dataset
- DataProduct / ProductVersion
- ProductRelease / ReleaseReadiness

Rights/Quality/Compliance may use mock providers early in this sprint, but the real state machines must be used.

Acceptance:

A Product Release V1.0 can be validated and published, then becomes immutable.

### Sprint 3 — Real Gates / Contract / Cost (`#6`, `#12`, `#14`, `#16`, `#26`, `#27`)

Deliver:

- Authorization basics
- Native Quality Engine
- Native Compliance rules
- DataContract / ContractVersion
- CostEvent
- EvidenceSnapshot
- deterministic formula test vectors
- full release-path integration test

Acceptance:

Release publishing is blocked by failed rights/quality/compliance/contract readiness checks, and indicator behavior is deterministic for committed fixtures.

### POC UI (`#13`)

Build only the pages needed to operate/demo the vertical slice; do not expose OpenMetadata/Hop/Splink as top-level product concepts.

### Sprint 4 — OpenMetadata Adapter (`#9`)

Deliver:

- MetadataEngine interface
- OpenMetadata adapter
- ResourceBinding
- Product governance projection

Acceptance:

OpenMetadata outage does not invalidate Core Product Release; sync retries asynchronously.

### Sprint 5 — Apache Hop Adapter (`#10`)

Deliver:

- ProcessingEngine interface
- Hop adapter
- execution status reconciliation

Acceptance:

A native processing task can be switched to Hop without changing Dataset/Product/Release semantics.

### Sprint 6 — Splink Adapter (`#11`)

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
→ V1 indicators
→ CURATED Dataset
→ Quality / Compliance
→ Data Contract
→ Data Product 1.0
→ Product Release
→ EvidenceSnapshot + Cost
```

## 8. Final Demo

The demo must also simulate at least one failure condition, such as authorization expiry, and demonstrate impact analysis / readiness failure / health degradation or suspension.

## 9. POC Non-Goals

Do not implement during Core POC unless explicitly approved:

- full Billing / Settlement
- full accounting/ERP
- blockchain platform
- complex microservice infrastructure
- black-box financial risk scoring
- high-risk personal surveillance datasets
- broad generic-platform features not required by the reference vertical slice

## 10. Execution Rule

Generic persistence/infrastructure work can proceed while reference semantics are refined, but indicator/workflow implementation must conform to the committed V1 specs. AI coding agents must not invent scoring formulas, imputation behavior, state transitions or engine-specific domain fields when the specification is silent.