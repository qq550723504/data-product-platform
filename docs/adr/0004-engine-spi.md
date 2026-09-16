# ADR-0004: External Capabilities Use Engine SPIs

- Status: Accepted

## Context

The platform may use OpenMetadata, Apache Hop, Python, Spark, Splink, Soda, Great Expectations or Presidio, but none of these should define Core business semantics.

## Decision

Core depends only on ports such as:

- MetadataEngine
- ProcessingEngine
- EntityResolutionEngine
- QualityEngine
- ComplianceEngine
- EvidenceStore

Concrete products are implemented as adapters.

## Consequences

- Engines can be replaced independently.
- Adapter contract tests are required.
- Core tables store external execution/reference IDs only as external references.
- Engine-specific configuration must remain outside Core domain objects wherever possible.