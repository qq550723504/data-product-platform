# ADR-0002: OpenMetadata Is a Governance Projection, Not the System of Record

- Status: Accepted

## Context

OpenMetadata is strong at technical metadata, lineage, glossary, classification, ownership and governance views, but the platform also requires DatasetVersion, Rights, production Workflow, Cost, Evidence and ProductRelease semantics.

## Decision

The Core Platform database is the System of Record for Data Product production and lifecycle state.

OpenMetadata is integrated through `MetadataEngine` and `ResourceBinding` as a Governance Projection.

## Consequences

OpenMetadata may own or project:

- physical asset metadata
- technical lineage
- glossary/classification
- governance view of Data Products

OpenMetadata must not own:

- DatasetVersion
- Authorization / Rights state
- Production Workflow truth
- Cost Ledger
- Evidence
- ProductRelease state

OpenMetadata failure must not roll back a valid Core Product Release.