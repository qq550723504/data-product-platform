# Entity Resolution Engine Boundary

## Decision

The Core Platform owns canonical `Entity`, `EntityMapping`, `MatchJob`, Human Review, Evidence, and matching policy history.

Probabilistic systems such as Splink are **candidate generators only**. They may propose ranked links to existing canonical entities, but they do not create, merge, or persist canonical state.

## Decision order

For COMPANY resolution the Core decision pipeline is:

```text
Strong identifier rules
  -> normalized deterministic rules
  -> optional probabilistic candidate engine
  -> policy thresholds
  -> AUTO_MATCH / REVIEW / UNRESOLVED
  -> Core EntityMapping / Human Review
```

Exact Unified Social Credit Code matches therefore remain authoritative and are never overridden by a probabilistic score.

## Provider-neutral contract

`internal/entity/resolution` defines:

- `MatchRecord`
- `ReferenceRecord`
- `CandidateRequest`
- `Candidate`
- `EngineDescriptor`
- `CandidateGenerator`
- `Registry`

Provider configuration, training data, blocking strategies, SQL dialects, model files, and runtime-specific payloads must remain outside the Core Entity domain.

## Provenance

Candidates and accepted mappings persist:

- policy version
- match method / rule
- normalized confidence score
- engine name
- engine version
- model version

This makes probabilistic decisions auditable without making the external engine the system of record.

## Failure / disable semantics

No probabilistic engine is required for the existing rule-only path. When no candidate generator is configured, the existing deterministic behavior is unchanged. Runtime-specific fallback/error behavior is implemented by adapters and application policy, not by canonical Entity state.

## Current POC boundary

The first generic implementation enumerates active canonical entities as the reference set. This is deliberately simple for the reference POC. Production-scale blocking/indexing is an adapter/runtime concern and can be replaced without changing the `Entity` or `EntityMapping` model.
