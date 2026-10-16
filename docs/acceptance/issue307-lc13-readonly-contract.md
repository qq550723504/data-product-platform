# #307 LC-13 native crash and 5d read-only preflight contract

2026-10-14, baseline `b3095ede4bed9a4c7b98ff74a08513243349fde7`.
Root is the sole writer; independent review is read-only. #318 stays Draft.
No merge/Ready/deploy, provider DELETE, hold/retention release/bypass or cleanup.

Authoritative sources: lifecycle architecture, LC-13 in the design acceptance matrix,
the recovery slice, workflow NativeReconciler and lifecycle FixtureSubjectPins.
Existing fixture environments/proofs remain sealed and retained.

LC-13 uses a fresh schema68/base074a33ba fixture, native MinIO724 and real Redis/Asynq.
The existing workflow queue/Handler, native input reader, PinnedEngine, shared fence,
session lock, physical-call journal and Cost/Audit/Evidence/Outbox remain the owners.
A fixture InputReader wrapper exits95 only after an actual journaled native GET.
Three input pins and RUNNING history survive, with no output. Actual orphan task
delivery and duplicate delivery must add0 GET/PUT and not infer successful processing.
Natural10-minute NativeReconciler lease expiry is observed without rewriting clocks,
started_at, status or lease. Recovery of unknown prior pins and recovery after a
current restriction must preserve the old pins/cost/history and produce0 new native I/O.
Replay is checked. Reader use_until expiry alone also must not append termination.

The current closed pin composition rejects any prior execution pins. No general
external-owner termination receipt exists here. This proves safe refusal/reconciliation;
it does not claim positive same-execution resume, proven owner quiescence or pin release.
Those require explicit verified stop/generation fencing through a separately scoped path.

5d verification restarts only the exact retained source Core/MinIO after validating
IDs/images/named volumes/internal network/0 ports and sealed business/config/gateway.
Source Core operations are BEGIN READ ONLY/pg_dump only, with complete before/after
relation fingerprints. A new isolated audit Core restores that export and verifies
identity, roles/ACL and exact facts before writing inspection journal/cost there.
The existing root-free reference checkpoint reader/inspector checks all six exact
versions/bytes/hash/hold/retention/inventory. Existing backups are hash-verified, not
overwritten. Original Core, old restore journals and deletion approvals are not mutated.
Native inspection costs live in the new audit Core; they are not source business facts.
Source services return to their original stopped state; all replicas/helpers/volumes remain.

Reuse check: all required Ports/SPI, queue/runtime, current authorization, journal,
native pg_dump/pg_restore and MinIO inspection already exist. No new dependency,
migration, runtime, public API or production recovery protocol is planned.

Blockers are wrong scope/credential exposure, unsafe lease-based pin release,
new physical read/output on unresolved/restricted recovery, source Core mutation,
lost/rewritten observations/costs/history, missing exact inventory/protection,
required-path/CI failure or unsupported acceptance claims. Failures retain append-only
attempts and diagnostics; no automatic provider resend or cleanup is permitted.

The result updates the matrix and exact confirmation proposal. It creates no human
deletion approval and does not accept full D1/D2 or production DR/IAM.
