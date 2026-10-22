# #325 explicit business rights renewal

Goal: append a finite same-scope rights window for the retained activated trial
after its prior window expires. Original #339 preparation is historical and must
not be reset or rerun with changed input. This contract owns operator assembly;
AGENTS.md and existing Core Commands own rights, immutability, accounting and
current entitlement semantics. Renewal is not delivery or certification.

Reuse: extend trial-business and the existing controller; reuse its exact
five-version/lineage SQL admission, private ACL/hash bindings, Core Rights
Commands, PostgreSQL advisory lock, command_idempotency, stable audit restoration
and activity Cost/Evidence/Outbox. No schema, Core/shared storage/worker changes
or new infrastructure. Temporal/Ansible would duplicate existing bounded
assembly and cannot replace Core rights facts.

The readonly plan binds the original preparation, latest explicitly bound rights
window, same run/workspace/Actor/principal, schema71, accepted activation/custody,
five versions and frozen batch/execution/lineage. Its spec contains a new renewal
UUID, exact finite UTC validity at microsecond precision and verification reason;
lossy sub-microsecond or non-UTC input is refused before any Core write. Scope is inherited from
the original prepared plan, not caller overrides. Window length cannot exceed
the original prepared duration or 24 hours, and cannot overlap its predecessor.
No tool build, lock, journal, registry, Core write or object read occurs in plan.

Execute only the reviewed plan. Reuse the same run advisory lock. A separate
TRIAL.BUSINESS_RIGHTS_RENEWAL claim keyed by run and predecessor plan hash permits
exactly one successor per window: conflicting UUID/window/input fails closed.
Each successor uses new stable trace/activity IDs and a distinct authorization
code. Replay restores its audit facts; interrupted same-plan execution resumes
committed steps, preserving every failed local observation. Completed replay
performs no Commands and cannot reactivate revoked or expired rights.

Each new window creates only one Authorization via Create/Submit/Approve/Activate
and three RightsDeclarations via Create/Verify/BindAuthorizationProvenance. Keep
the original three source resources, consumer, purpose, GRANT READ authority,
per-resource ALL_RESOURCE scope and raw_export_allowed=false. Prior facts must
be naturally expired, VERIFIED and undisposed; suspension/revocation/invalidation
does not become renewal authority. Revalidate facts before execution; partial
failure remains UNKNOWN without automatic compensation or a new window.

Reuse the exact published ContractVersion and finalized CertificationProfile;
verify their identity/content and coverage. Do not create new datasets, profiles,
contracts, snapshots, assessments, certifications, deliveries or executions.
Append a new manifest with exact new IDs and predecessor/preparation references;
advance only the explicit latest renewal binding, retaining every prior manifest
and the original preparation binding/spec/claim. Future windows repeat this
explicit process; they are not automatic renewals.

Blockers: scope/identity widening, lost historical facts or costs, plan side
effects, false receipt adoption, changed-window replay, duplicate same-parent
successors, broken recovery, secret leakage, failing required path/CI.
Non-goals: original deployment/execution, services, STS/policy changes, object
access/copy, entity review/processing, quality/compliance/certification/delivery,
download/revocation, generic renewal policy or new IAM. Keep Draft; no merge.

Validation: real isolated PostgreSQL Commands and entitlement success/fail-closed
tests, preserved old row/content assertions, same-plan replay/interruption,
same-parent concurrency/conflicts, later window, readonly CLI including native
verify/storage_check, and independent exact-HEAD review plus CI. All original
runtime/business actions remain NOT_RUN for this candidate.
