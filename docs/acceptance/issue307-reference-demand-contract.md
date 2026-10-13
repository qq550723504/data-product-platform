# #307 finite reference-demand implementation contract

Candidate base: origin/main 074a33bace7edbaf481759aac3a84708363ffd02.
Writer: this isolated codex/issue307-reference-demand-lc11-lc12 checkout only.
Migration: 67 (main through 66; refreshed live before assignment).

Authority: AGENTS.md, lifecycle contract sections 4 and 6, ADR-0015,
Issue #307 and issue307-d1-confirmed.md. This document records this PR's finite
implementation/validation boundary, without replacing those authorities.

Goal: distinguish immutable business reference identity from its owner's ongoing
exact-copy byte requirement for the eight observed controlled-fixture reference
classes. Every undisposed source row remains blocking. Known source identity,
copy, workspace, owner and lost capability are typed and checked. Declaration
requires bytes; explicit release appends another fact, preserving the source FK.
Release requires current module-owner capability, explicit owner verification of
unmanaged/background/cached work, durable stop-new-use, and verified termination
of every active pin for that version. Status, cancellation request, heartbeat and
lease time cannot release a pin. Same-key replay freshly authorizes and compares
the fingerprint. No timestamp chooses current demand.

Dataset, Entity, Workflow, Quality, Certification and Delivery own the loss
contract and validate their own source relationships. Lifecycle persists their
decision and enforces shared admission/fence. No general HTTP release endpoint
or user-selected table/locator is exposed. Current fixture composition is the
only integration; all other consumers remain out of scope.

Concurrency: shared workspace delivery fence, sorted physical copy parents,
then reference facts; source insertion/update and pending-copy registration
must take that same fence. Source writes lock before checking whether the first
fixture copy exists, serializing with first-copy registration; an uncommitted
pre-registration reference cannot cross registration and escape inspection.
FK discovery checks installed admission guards and
locks schema evolution. Unknown/new/unmanaged dependencies fail closed.
Metadata exemptions name the exact public schema/table/column; another schema's
same-name table or a new FK column does not inherit an exemption.
Dispatch waits for real processing/transfer owner stop; disposition-first blocks
new GET, bytes and output admission. Final business identities and ISSUED history
remain intact; transfer observations are appended.

Lost capability: source/Execution replay, mapping input verification, entity
matching replay, Quality recomputation/attempt replay, Certification content
reverification and Delivery payload replay become unavailable for a released
copy. Actual byte removal remains a separately authorized provider action.
Current rights over immutable source lineage remain required for derived use.

Reuse: existing deliveryfence, pins, Storage Port/MinIO adapter, native engine,
Quality/Certification/current rights, Asynq queue handler/reconciliation,
transaction/outbox, Audit/Evidence/Cost. PostgreSQL FK/trigger/locking supplies
commodity integrity; no second policy/runtime/storage infrastructure. S3 native
lifecycle and Temporal do not own these platform reference semantics, so they
cannot replace module-owner decisions. No new dependency or fork.

Required verification: domain/reference validation, application rejection and
idempotency, independent named-volume real PG barriers (both orders; pending
copy; active/unknown owner), native Processing baseline and queued/start/retry/
recovery refusal, real Quality/Certification/current-rights DirectData slow TCP
transfer versus disposition. SQL/fake/native evidence is reported separately.

Merge blockers: authority/tenant or bytes bypass, mutable historical facts,
unfenced source/pending-copy writes, false quiescence, broken fingerprint/event
atomicity, candidate build or declared required tests failing. This PR stays
Draft; no automatic Ready/merge/deploy/Issue close.

Non-goals: transfer to another copy (unsupported; no inference), general D2
integration, real deletion without fresh exact-list confirmation, archive/restore
rerun, backup recovery, Annotation, original retained environments and old C2
recovery. Full D1 remains unaccepted. Provider DELETE/cleanup is NOT_RUN; existing
two-version approval is consumed and cannot be reused.

