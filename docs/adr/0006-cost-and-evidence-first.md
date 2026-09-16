# ADR-0006: Cost and Evidence Are Captured During Production

- Status: Accepted

## Context

Cost reconstruction and audit evidence become unreliable when they are collected only after a project or accounting exercise is complete.

## Decision

CostEvent and Evidence are first-class business objects generated throughout production.

Key activities should produce them whenever applicable, including:

- ingestion
- transformation
- entity review
- quality remediation
- compliance review
- external services
- product release
- product usage

## Consequences

- Product Cost Ledger can be derived from production facts.
- Evidence Packages can be assembled instead of manually reconstructed.
- POC implementations must include Cost/Evidence hooks even when the initial monetary values are approximate.