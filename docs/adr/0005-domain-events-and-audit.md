# ADR-0005: Key State Changes Emit Domain Events and Audit Events

- Status: Accepted

## Context

The platform needs asynchronous projections, impact analysis and traceability across production, rights, quality, compliance and releases.

## Decision

Key state changes emit Domain Events. Human-meaningful or accountability-relevant changes also create AuditEvents.

Domain events are first written to a Transactional Outbox in the same database transaction as the aggregate update.

AuditEvent is business data and is not replaced by application logs.

## Consequences

- External side effects can retry independently.
- Core transactions do not depend on OpenMetadata or other engines being available.
- Event handlers must be idempotent.
- A stable event vocabulary and versioning discipline are required.