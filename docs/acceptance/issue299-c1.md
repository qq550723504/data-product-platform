# #299/C1: negative-energy remediation

## Finite contract

Base: main `3a59b591d25e7d07fc38956a7ae729d74967215b`. One writer, branch
`codex/issue299-c1`; isolated synthetic PostgreSQL/Hop/Core/storage/browser runtime.
#307 retains migration 000054, lifecycle/storage and its local routing changes;
C1 reserves 000056 (#309 now reserves 000055). Shared routing/event vocabulary is integrated in independent
candidates and rechecked against main before publication. C1 needs no new cost
allocation subject: existing physical Execution and Quality attempts own costs.
#294/#295 and all retained #298 environments remain with their original owners.

Goal: find an actual negative value in the fixed PostgreSQL numeric text column,
assign it to the authenticated pilot operator, execute the approved existing
quarantine workflow, inspect a new CURATED version and recheck it. Certification
and delivery remain separate existing commands and gates.

Authority: #299/#296, AGENTS, existing workflow/quality/rights/certification/delivery
contracts and industry-pack rules. This document owns only the C1 case association
and its acceptance record. It is not another lifecycle blueprint.

## Reuse check

Existing Native/Hop execution, native regex evaluator, immutable QualityAssessment
and Finding, execution quarantine, attempt costs, idempotency, PostgreSQL locks,
principal/workspace resolver, Audit/Evidence and transactional Outbox are reused.
The RAW-only policy expresses the existing nonnegative-energy rule on PostgreSQL's
fixed numeric decimal text; it introduces no guessed maximum or replacement value.
Output recheck uses the existing complete enterprise-activity quality ruleset.
Existing RAW full-product assessment can fail for unavailable product metrics;
that historical failure alone is not proof that a negative reading was detected.

OpenMetadata already provides native quality tests, alerts and resolution workflows
([official guide](https://docs.open-metadata.org/v1.12.x/how-to-guides/data-quality-observability/quality)).
Those catalog workflows cannot atomically own Core's immutable version/Execution/
Assessment association. C1 adds only those business facts and the operator path;
it does not recreate a ticket engine, scheduler, evaluator, IAM, alert transport or
approval system. No new dependency/provider or paid service is introduced.

## Required invariants

- Server-authenticated workspace/actor; pilot assignment is explicitly to that
  operator. A descriptive catalog Owner, body actor or arbitrary UUID grants no role.
- Detection records an exact failed Finding, frozen rule hash and source version.
  Same detection key replays the same quality attempt and case.
- Immutable case header, assignment, execution association, recheck start/outcome
  and manual-close facts. Commands lock the case parent first. No PATCH status.
- Approved workflow identity/hash is checked server-side; case input must be one
  exact bound input. Case attempt + existing Execution acceptance and its outbox
  obligation commit atomically. Only one active/latest successful execution; a
  failed execution is retried through the existing explicit retry command.
- Same command replay creates no new execution/cost. A real failed invocation and
  explicit new retry have separate existing attempt costs. Unknown outcomes are
  recovered through existing owners, never blindly restarted by the case layer.
- Recheck pins an explicit successful execution/output and stable quality attempt
  before evaluation. Response loss between assessment and association can replay
  that attempt. New physical evaluation uses a new explicit attempt key.
- Manual closure cannot mark Quality PASS or Certification CERTIFIED. Old RAW,
  assessments, certifications and release/evidence history are protected.
- Display energy input/accepted/quarantined records at the same grain; enterprise
  aggregation output rows are a separate grain. Unknown impact stays unknown.
- Repaired/certified output still uses CurrentDeliveryGate; source revocation must
  block fresh delivery from a stale ALLOWED browser page with zero payload.

## Acceptance record

All implementation/runtime checks are initially NOT_RUN. Record exact final HEAD,
provider builds, commands, real PG barriers, browser screenshots, immutable row/
membership hashes and download bytes separately before creating the Draft PR.

Blockers: current C1 path failure, security/tenant bypass, mutable/lost historical
facts or costs, non-atomic acceptance, duplicate real work on replay, concurrency
violating these invariants, required build/test failures.

Follow-ups: #299/C2 source schema observation, multi-operator IAM/assignment,
generic incident/notification UX, full downstream impact, schedules and production
rollout. #298 human explanation remains deferred/NOT_RUN. C1 does not close #299,
does not pass C2/human acceptance, and its PR remains Draft without automatic merge.
