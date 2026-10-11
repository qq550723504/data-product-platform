# #307 worker/outbox recovery slice

## Reuse and ownership

Core owns recovery request identity, exact frozen scope and append-only recovery
run/result facts. Existing Asynq v0.25.1 owns queue leases, backoff, retries and
orphan recovery. Existing platform outbox owns delivery obligations and per-handler
confirmation; existing PostgreSQL advisory locking owns cross-worker exclusion.
No second scheduler, queue or durable workflow runtime is introduced. Temporal
is a mature alternative, but this bounded read-only reconciliation does not justify
a second runtime or its operating cost; the current Asynq/outbox capabilities suffice.

Migration 63 follows main's external usage migration 58 and D1 59–62. Routing is
`c1-v26+external-core1+execution1+usage1+lifecycle-recovery1`. Existing event
obligations remain frozen; only new `LifecycleDispositionRecoveryRequested` events
owe `lifecycle-recovery-queue`. Old retention-only events are not reinterpreted.
The idle C1 Draft's migration-58 conflict with main remains a separate owner's gap.

2026-10-11 coordination checkpoint: D1 has already migrated and accepted the retained
native fixture at **59–63** (including worker recovery 63). Its schema, binaries and
historical evidence stay unchanged. The runner uses the latest applied version as
its upgrade boundary: C1/64 cannot merge first and then safely admit D1/59–63.
The selected publication order is **C1/59 first, then unpublished D1 files 60–64**.
D1 file renumbering and fresh isolated PostgreSQL upgrade validation follow the
stable C1 candidate; do not apply that candidate to this retained fixture. Current
native acceptance remains bound to its original 59–63 source. No C1 files or
environment are changed by this lifecycle writer; D1 is not yet a merge candidate.

## Recovery contract

Dispatch commits its provider attempt, recovery request, Audit/Evidence and outbox
wake in the same transaction, before any native side effect. A recovery task carries
only workspace/work IDs; no credentials, caller, object key, version or allow flag.
A duplicate delivery serializes on the original attempt's PostgreSQL session lock.
Recovery I/O holds no workspace/content transaction locks.

Before every physical metadata call the existing instrumented journal checks current
inspection **and reconciliation** authority under the shared workspace fence. Every
native call has its own durable start, observation and CostEvent/typed allocation.
Revocation cannot erase an incurred observation/cost. A crash with only a start
does not prove invocation, stopped I/O, no-send or absence.

Complete native inventory plus positive exact HEAD yields `UNKNOWN_PRESENT`.
The recovery request finishes, while the original deletion and exclusive reservation
remain unresolved. Recovery never resends DELETE, releases a hold, bypasses retention
or converts elapsed time into stopped-reader proof. A new explicit recovery request
is needed to observe a later state. Provider errors remain retryable UNKNOWN; six
durable runs bound automatic work. Exhaustion requires an explicit new request.

Exact absence uses the existing post-dispatch scoped inventory receipt. Each receipt
is per copy; a plan completes only with its entire frozen membership. A crash after
receipt commit resumes from that receipt without another lookup and can finish a
fully covered plan. New absence/finalization commits also revalidate current recovery
authority. Original provider observations are never overwritten. Delete markers,
naked 404, provider ACK and incomplete inventories do not establish absence.

The existing pending-work scan repairs a confirmed enqueue subsequently lost with
Redis. Asynq still owns execution. A completed/denied recovery is not re-enqueued;
archived or exhausted UNKNOWN work remains visible in Core.

## Composition and acceptance boundary

`cmd/worker` registers the frozen queue handler and task handler. An absent recovery
configuration fails closed. `LIFECYCLE_RECOVERY_CONFIG` may enable only a finite
synthetic two-namespace environment with pinned workspace/provider/incarnation and
read-only native credentials. This is not production lifecycle policy or IAM.

`cmd/lifecycle-fixture-recovery` is the bounded acceptance composition of the same
queue/outbox/authority/journal/recovery implementation. Its one-shot SQL fault input
is labelled `SYNTHETIC_SQL_FAULT_INPUT_NO_NATIVE_DELETE_APPROVAL`; synthetic approval,
dependency and pending-start facts test the SQL protocol, not the real deletion gate.
It has no Deleter or payload reader. Native metadata recovery of that input must be
reported separately from actual crash-after-delete recovery.

Objects and all owned named volumes are retained. The confirmed 4abcfca5
quarantine/B exact version was deleted once after a fresh full gate, followed by
real producer exit and read-only worker/outbox recovery to scoped absence.
See `examples/lifecycle-d1/results/4abcfca5-583d-41ca-8548-10a0fc3270a1/confirmed-worker-delete-review.md`.
That confirmation is consumed; additional native DELETE requires a new reviewed
environment/key/version list and explicit human confirmation. The final Docker
snapshot detected missing exited helpers and two unrelated anonymous volumes,
with caller UNKNOWN; global/helper retention is not claimed PASS. All 11 owned
fixture volumes, running services and current survivor objects were verified.
