# #307 worker/outbox recovery slice

## Reuse and ownership

Core owns recovery request identity, exact frozen scope and append-only recovery
run/result facts. Existing Asynq v0.25.1 owns queue leases, backoff, retries and
orphan recovery. Existing platform outbox owns delivery obligations and per-handler
confirmation; existing PostgreSQL advisory locking owns cross-worker exclusion.
No second scheduler, queue or durable workflow runtime is introduced. Temporal
is a mature alternative, but this bounded read-only reconciliation does not justify
a second runtime or its operating cost; the current Asynq/outbox capabilities suffice.

Unpublished D1 migrations are now **60–64**, after actual main/C1 migration 59
(`1892cd76c4ee578896ddfb3c6f4a7252bfacc51b`, exact C1 candidate `f114e506`).
Migration 64 owns DELETE and COPY recovery facts. Routing is
`c1-v26+ext1+exec1+usage1+rem1+life2`, within the existing 64-character
outbox version contract for both profiles, including `+governance`. Both new recovery-request events owe the
existing `lifecycle-recovery-queue`; existing C1/external/execution/usage obligations
remain intact. The corresponding main push CI `38147297060` is SUCCESS; this is
base-main evidence, not CI or merge evidence for the unpublished D1 candidate.

2026-10-11 coordination checkpoint: D1 has already migrated and accepted the retained
native fixture at **59–63** (including worker recovery 63). Its schema, binaries and
historical evidence stay unchanged. The runner uses the latest applied version as
its upgrade boundary: C1/64 cannot merge first and then safely admit D1/59–63.
The selected publication order is **C1/59 first, then unpublished D1 files 60–64**.
D1 file renumbering is complete. New retained PostgreSQL databases prove main/58
to C1/59 to D1/64, actual-main/59 to final D1/64, and fresh final D1/64 installs.
The prior accepted native environments remain bound to their original 59–63
sources; no migration was applied there. Native COPY recovery runs separately in
new `dpp-lifecycle307-f7fa46ff` at 64. D1 is still unpublished and Issue #307 OPEN.
D1 currently owns migration numbers 60–64 and the lifecycle additions in routing;
later slices must preserve D1/60's CostAllocation single-subject/workspace guards.

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
This is a nonterminal observation: the original DELETE may not have been sent yet
or may still complete. The original deletion and exclusive reservation remain
unresolved. Recovery returns a retryable error for Asynq and remains in the durable
pending scan, without resending DELETE, releasing a hold, bypassing retention or
converting elapsed time into stopped-reader proof. Provider errors and inconclusive
presence remain retryable UNKNOWN; six durable runs bound automatic metadata work.
Exhaustion requires an explicit new request and never releases the reservation.

Exact absence uses the existing post-dispatch scoped inventory receipt. Each receipt
is per copy; a plan completes only with its entire frozen membership. A crash after
receipt commit resumes from that receipt without another lookup and can finish a
fully covered plan. New absence/finalization commits also revalidate current recovery
authority. Original provider observations are never overwritten. Delete markers,
naked 404, provider ACK and incomplete inventories do not establish absence.

The existing pending-work scan repairs a confirmed enqueue subsequently lost with
Redis. Asynq still owns execution. A completed/denied recovery is not re-enqueued;
archived or exhausted UNKNOWN work remains visible in Core.

## COPY / RESTORE UNKNOWN recovery

An original custody call commits its single-send start, strong copy-operation
identity, recovery request and outbox wake before I/O. The existing work table has
an exclusive typed subject: deletion provider attempt OR original COPY call.
The same scanner, Asynq queue, outbox obligation and session advisory-lock mechanism
are reused; COPY workers have no CreateCopy/DELETE/hold-release method or credential.

Read-only discovery requires a complete single-page target-namespace version list,
exactly one non-marker version at the frozen target key, then an exact-version HEAD
with matching native `lifecycle-operation` UUID and byte size. Missing, ambiguous,
truncated, denied or mismatched observations cannot open a copy and never resend the
original request. A separate immutable `lifecycle_copy_discovery` joins original
call, inventory, exact HEAD, operation and recovered version. Materialization uses
this fact; it does not fabricate a successful original COPY observation. The
existing successful-ACK path remains available under its original proof checks.
If a materialization explicitly names a discovery, its operation, original call,
version and time must match even when the original call also has an ACK. The
final guard was validated after native sealing in two further isolated databases;
the native evidence retains its original SQL hash and is not relabelled as that
later schema revision.

`UNKNOWN_COPY_TARGET` is likewise nonterminal and retryable: an early inventory
can run before the original COPY produces its target. Later metadata discovery
can resolve the same work without another COPY. Each lookup keeps a distinct
physical identity/cost and the same six-run budget; terminal resolutions stop
the scanner and make duplicate task deliveries perform no I/O.

Recovery refreshes source and target retention/hold, hashes bounded exact target
bytes through the existing instrumented adapter, then re-authorizes under the
workspace fence. ARCHIVED retains its source. RESTORE needs the latest complete
independent ledger chain, matching Core checkpoint, epoch, state hash and current
dependency revision. The worker's independent-ledger mount is read-only. A missing
or stale witness produces UNKNOWN; a separate authorized witness export plus a new
explicit recovery request can re-evaluate. Current restrictions still produce
BLOCKED. Historical UNKNOWN/ONLINE decisions and original provider observations
remain append-only. ONLINE is a historical fixture custody/use admission; a later
content-use restriction still blocks current use, and this does not implement
production certification/rights delivery.

## Composition and acceptance boundary

`cmd/worker` registers the frozen queue handler and task handler. An absent recovery
configuration fails closed. `LIFECYCLE_RECOVERY_CONFIG` may enable only a finite
synthetic two-namespace environment with pinned workspace/provider/incarnation and
read-only native credentials. This is not production lifecycle policy or IAM.
`LIFECYCLE_COPY_RECOVERY_CONFIG` selects the finite COPY composition, with a
separate read-only native principal, pinned epoch/DatasetVersion and external
read-only witness path. The normal application default remains disabled.

`cmd/lifecycle-fixture-recovery` is the bounded acceptance composition of the same
queue/outbox/authority/journal/recovery implementation. Its one-shot SQL fault input
is labelled `SYNTHETIC_SQL_FAULT_INPUT_NO_NATIVE_DELETE_APPROVAL`; synthetic approval,
dependency and pending-start facts test the SQL protocol, not the real deletion gate.
It has no Deleter; deletion recovery has no payload reader. COPY recovery reads
only bounded frozen target versions for internal checksum verification. Native
metadata recovery of that SQL fault input must be
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

The new COPY recovery evidence is
`examples/lifecycle-d1/results/f7fa46ff-3a93-4e50-97a5-0d68fac8b235/copy-recovery-review.md`.
It covers actual native COPY response loss, process exit after durable lookup,
Asynq orphan recovery, broker failure/outbox repair, independent-ledger UNKNOWN to
fresh ONLINE, current restriction to fresh BLOCKED, duplicate replay and revoked
window with zero additional I/O. All 7 versions and all owned volumes are retained;
70 physical calls have 70 distinct typed cost allocations. DELETE/cleanup are zero.
The 17 pre-turn containers and 11 pre-turn owned volumes are unchanged in the
current preservation snapshot. New DELETE remains NOT_RUN and requires a reviewed
environment/exact key/version list plus explicit confirmation.
