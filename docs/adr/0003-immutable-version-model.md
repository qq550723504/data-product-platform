# ADR-0003: Versioned Production Facts Are Immutable

- Status: Accepted

## Context

The platform must reproduce historical product releases and prove which data, rules and contracts were used at the time of publication.

## Decision

Treat the following as immutable history once frozen/published:

- DatasetVersion
- WorkflowVersion
- ContractVersion
- ProductVersion
- ProductRelease
- EvidenceSnapshot

Errors are corrected by creating a new version/release rather than editing historical facts.

## Consequences

- Historical releases remain reproducible.
- Audit and Evidence remain trustworthy.
- Storage grows over time and requires retention/archival policies.
- APIs must expose explicit version creation instead of generic update semantics.