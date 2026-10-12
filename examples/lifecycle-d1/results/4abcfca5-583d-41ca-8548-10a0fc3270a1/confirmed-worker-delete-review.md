# #307 confirmed exact deletion and worker/outbox recovery

2026-10-11. Controlled synthetic acceptance **PASS** for the single human-confirmed
version in `dpp-lifecycle307-4abcfca5`; full D1 remains **NOT_RUN**.

## Consumed authorization and provenance

- Workspace `d9deb37f-0e58-4f00-830e-c7aad21895f8`.
- Provider `b48e5dcf-d466-483d-ac89-f810ba976974`, incarnation
  `57c5084d-d929-4eeb-842d-180734158bb0`.
- Only deleted locator: `dpp-lifecycle307-quarantine/fixture/b.csv`, exact version
  `ce87f789-a4f4-4ce7-bf1f-a044ac53d1d2`, 26 bytes, SHA256
  `aafd9ae024f2444eefea042c59ffd87c29fac3f4675c49124785b417da34b784`.
- Frozen plan `872ca588-6856-420f-b450-e5cd4e8008f4`, hash
  `7184d41a0b1578c09d5f358a31dfce819946154bc6237e259ae5beb680c5c04b`.
- Native deletion composition source: local commit `34ea8193` (full SHA in
  `dpp-lifecycle307-4abcfca5-worker-delete-execute-provenance.json`). Source hashes,
  executable hash, gateway/topology hashes and container identity were recorded
  before start. The object executor mounted no root/admin credentials.
- The recovery worker uses the original accepted read-only binary from `62e8f3aa`
  (SHA256 `0c508dc8780b94a04a65f3a0c15db648c3a92ec3494fb3fbc30613821d10d38f`).
  Per-step `source_head` is the orchestration checkout; it is not a claim that this
  retained binary was rebuilt at that HEAD. The separately built survivor witness
  is bound to `34ea8193` and has no Deleter.
- Four pinned running services, private internal network, no host ports, schema
  59–63. MinIO source/build/SDK pins are in `manifest.json` and the environment
  readback; no runtime migration or image replacement occurred.

The original `pending-delete-plan.json` remains an immutable pre-confirmation
record. This report and the real approval/gate facts record the later confirmation.
This approval is consumed; neither script replay nor another object/version is
authorized by it.

## Fresh gate and real producer interruption

`delete-current-gate-171d1d1b-03dd-4fc9-a801-47044992cb89.json` records a finalized,
eligible gate with **zero blockers**: current caller and approver, exact frozen
scope, explicit synthetic test retention start/due basis, complete native version
inventory, released typed references, current provider retention/hold, restricted
use, drained readers, hold-free content, current journal and no unresolved dispatch
for this copy. Native metadata was refreshed again immediately before the fenced
send claim; the same current dependencies were checked again by `ClaimExactSend`.
The four-member topology and absence of any running acceptance worker were checked
before opening the finite window.

The explicit zero minimum retention basis is for these closed synthetic inputs,
not a business/legal policy. The SQL-only fault input on online/A was not reused
as either this approval or this real deletion gate.

One DB-first attempt `13323038-1024-49f8-b5cd-a8ffe4c4ea4e` and one exclusive send
claim led to **one native DELETE invocation**. IAM and the gateway allowed only
this exact key/version; no protected DELETE probe, bypass, hold release, object
cleanup or volume cleanup was executed. The native observation is
`CONFIRMED/ACCEPTED`; one typed physical delete CostAllocation was persisted.
After that observation/cost commit, the real producer process exited with code
93, before receipt/finalization. This was an explicit acceptance fault, not an
uncertain delete response. At that point receipt/completion counts were both zero.

`delete-send-before-worker.json` records that intermediate state as **absence
UNVERIFIED**. A provider ACK did not establish absence. Before recovery, the native
delete identity was disabled and the original read-only gateway restored; both
the bootstrap and delete authority windows were disabled.

## Worker/outbox evidence

Recovery work `67eaf55d-a4a3-4e05-8f0f-12d1e57001a5` was committed atomically with
the real dispatch. The finite read-only recovery window then used the existing
Asynq/outbox/advisory-lock implementation and current recovery authorization.

| Check | Result | Evidence |
|---|---|---|
| Redis transport unavailable | PASS: outbox FAILED, zero handler confirmations; no receipt | `real-delete-outbox-unavailable.json`, `db-real-delete-outbox-failed.json` |
| Redis restored | PASS: same outbox wake PUBLISHED, one handler confirmation | `real-delete-outbox-publish.json` |
| Duplicate enqueue | PASS: same work/task identity, one actual recovery run | `provenance-real-delete-duplicate-enqueue.json`, `db-real-delete-worker-complete.json` |
| Real Asynq worker after producer exit | PASS: ABSENT_VERIFIED, one scoped receipt and one plan completion | `real-delete-worker.json`, `real-delete-exact-facts.json` |
| Completed work and outbox replay | PASS: no additional run, provider call, receipt, completion or DELETE; still one handler confirmation | `db-real-delete-replays.json`, `real-delete-outbox-replay.json` |
| Surviving versions and protections | PASS: all three exact identities and lock values unchanged; complete namespace membership verified | `real-delete-survivors-after.json` |
| Core content history | PASS: both original DatasetVersions remain READY with original URI/checksum/size | `real-delete-exact-facts.json` |
| Per-invocation accounting | PASS: 97 native metadata starts/observations/typed allocations match; one real DELETE allocation | `db-real-delete-final.json`, `real-delete-exact-facts.json` |

The worker performed one fresh complete native namespace inventory and committed
the exact-version receipt under current authority, then completed the one-target
frozen plan. It sent **zero DELETEs**. Total deletion attempts remain two rows:
one prior SQL-only synthetic input with zero send/observation/cost, plus this one
actual physical invocation. The online/A UNKNOWN reservation remains unresolved;
no release or inferred no-send fact was added. The previous metadata-only faults
remain separately documented in `recovery-review.md`.

## Preserved objects and scope

| Namespace/key | Exact version | Current protection |
|---|---|---|
| online/fixture/a.csv | `4a225512-f6ac-4fc9-b0c0-bd6e016a2a01` | NONE; original UNKNOWN reservation retained |
| online/fixture/b.csv | `7cd23e55-140c-4566-b881-e58c423c7a8a` | legal hold ON |
| quarantine/fixture/a.csv | `936c8741-87f6-4279-87b9-742936cb2dd2` | GOVERNANCE until `2026-10-12T13:39:45Z` |

Namespaces above have the `dpp-lifecycle307-` prefix. No other version was removed.
Protected objects received metadata and domain-block checks only, without approval
or a native DELETE request. All authority windows are now disabled and all native
mutations/admin/payload reads are closed; gateway SHA256 is
`81b26869b3ee1074035798a4bb6ba8c455ec71d69b57b7a7434d0da4d6011662`.
Native IAM preparation/disable has separate infrastructure receipts in
`real-delete-admin-receipts.json`; these are not Core deletion costs.

Scoped absence covers this version in the provider-accessible namespace only.
It does not claim physical media erasure, provider internal backups/replicas,
external recipients or global destruction.

## Docker preservation exception

**PASS:** all 11 owned named fixture volumes across the five retained environments
exist, and all 16 previously running fixture services retain identity/image/state
and mounts. The three current survivor objects/protections were independently
verified through read-only native metadata.

**FAIL for exited-helper retention; UNKNOWN caller:** the final Docker snapshot
no longer contains 44 pre-existing exited helper containers. The new exited
acceptance helpers are also absent. The global volume inventory changed from
2433 to 2431; the two missing anonymous volume names had no mounted relationship
to any fixture container in the captured baseline. No cleanup command was issued
by this run. Docker's available event query did not identify a caller. Do not
claim that global Docker inventory or every helper was preserved; public binary
provenance, native journal, SQL facts and readback evidence remain archived.
Details are in `real-delete-preservation.json` and `docker-retention-drift.json`.

## Validation and remaining delivery gates

Local Go validation passed for the real PostgreSQL lifecycle application suite
(88.284s), lifecycle Domain/pilot/queue, native Storage Adapter contracts, both
fixture commands, routing and worker. The pinned manifest retarget/duplicate test
also passed. PowerShell parsing and whitespace checks passed. Native Redis worker
and native MinIO acceptance above are distinct from these local test results.

#307 remains OPEN. This is a local controlled acceptance slice; no PR publication,
push, merge, exact-HEAD PR CI or matching main CI occurred. COPY/RESTORE UNKNOWN
worker recovery and full D1 acceptance remain NOT_RUN. Publication dependency order
is C1/59 first, then renumber only unpublished D1 candidate files to 60–64 and
validate a fresh isolated database upgrade; this retained 59–63 fixture and its
historical binaries/evidence must not be migrated or rewritten.
