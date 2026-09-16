# Database Design V1.0

Target: PostgreSQL 16+.

## 1. General Conventions

- Primary key: UUID
- Business code: `varchar(64)`
- Time: `timestamptz`
- Extension fields: JSONB
- Optimistic lock for mutable aggregates: `revision bigint`
- Soft delete only for mutable business master objects
- Immutable facts must not be soft-deleted or overwritten

## 2. Multi-Tenant Boundary

Core business objects reserve:

- `workspace_id`
- `project_id` where applicable

Workspace represents organization / tenant boundary; Project represents a concrete initiative or product workspace.

## 3. Core Tables

First migrations should cover:

```text
workspace
project
use_case

data_resource
resource_binding

dataset
dataset_version
dataset_version_lineage

entity_type
entity
entity_mapping

workflow
workflow_version
task
workflow_run
execution
execution_dataset

data_product
product_version
product_asset
product_release
product_release_dataset

evidence
evidence_relation
evidence_snapshot
audit_event
outbox_event
```

Second batch:

```text
authorization
authorization_resource
authorization_action
authorization_scope

quality_rule
quality_result

compliance_policy
compliance_result

data_contract
contract_version

cost_event
cost_allocation
```

## 4. DataResource

DataResource is a business-level resource, not a physical table.

Key fields:

- id
- workspace_id
- project_id
- code / name / description
- domain_code
- resource_type
- owner
- sensitivity_level
- rights_status
- quality_status
- lifecycle_status
- business_metadata JSONB

## 5. ResourceBinding

Separates Core from metadata engines and physical systems.

Key fields:

- resource_id
- provider (`OPENMETADATA`, etc.)
- entity_type
- external_id
- external_fqn
- connection_ref
- binding_metadata JSONB
- is_primary

No database FK to external systems.

## 6. Dataset / DatasetVersion

Dataset is logical identity. DatasetVersion is an immutable production fact.

Dataset types:

- RAW
- STANDARDIZED
- CURATED
- PRODUCT

DatasetVersion stores:

- version_no
- storage_type / storage_uri
- schema_version
- row_count / byte_size
- checksum
- generated_by_execution_id
- rights_snapshot_id
- quality_status
- compliance_status
- snapshot window
- metadata JSONB

DatasetVersion must not be updated after it reaches frozen state.

## 7. Production Lineage

`dataset_version_lineage` records input/output lineage independent of OpenMetadata technical lineage.

This is the platform Production Graph.

## 8. Entity

Core model:

```text
EntityType → Entity → EntityMapping
```

Entity fields:

- canonical_key
- canonical_name
- attributes JSONB
- status

EntityMapping stores:

- source_type
- source_ref
- source_key
- source_name
- match_method
- policy_version
- confidence
- status
- reviewer
- evidence

## 9. Execution

Execution is the platform business execution record, independent of engine job IDs.

Stores:

- workflow / workflow_version / task
- execution_type
- executor_type
- engine_execution_id
- status
- timing
- rows / bytes in/out
- runtime_metrics JSONB
- error code/message

Execution may generate:

- DatasetVersion
- CostEvent
- Evidence
- AuditEvent

## 10. DataProduct / ProductVersion / ProductRelease

DataProduct: stable identity.

ProductVersion: immutable product specification.

ProductRelease: immutable published snapshot.

ProductRelease references exact:

- product_version
- dataset versions
- contract version
- rights snapshot
- quality result
- compliance result
- evidence snapshot

Published Release must never be edited in place.

## 11. Evidence

Evidence stores evidence metadata and optional artifact location/hash.

EvidenceRelation links evidence to arbitrary business objects using `(object_type, object_id)`.

EvidenceSnapshot freezes the manifest of a release/case at a point in time.

## 12. Cost

CostEvent supports both monetary and quantity-based events.

Examples:

- amount=12.5 CNY, category=COMPUTE
- quantity=2.5 HOUR, category=HUMAN

Accounting classification is a later professional review and must not be conflated with production cost collection.

## 13. JSONB Policy

Use JSONB for:

- engine-specific metadata
- runtime metrics
- schemas and snapshots
- industry extension attributes
- delivery config
- evidence manifest

Do not use JSONB for:

- IDs / FKs
- status
- version numbers
- owner
- timestamps
- fields frequently joined or constrained

## 14. Deletion Policy

Soft-delete allowed:

- UseCase
- DataResource
- Dataset
- DataProduct
- Entity

Do not delete immutable facts:

- DatasetVersion
- Execution
- ProductVersion
- ProductRelease
- EvidenceSnapshot
- AuditEvent
- CostEvent
