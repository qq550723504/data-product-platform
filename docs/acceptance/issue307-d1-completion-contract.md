# #307 D1 native non-delete completion contract

2026-10-14; user-authorized extension of Draft #318, baseline41d9fda7.
Root is the only writer; the independent reviewer is read-only. No merge/Ready/deploy,
native DELETE, hold release, retention bypass, object/volume/helper cleanup or reuse
of consumed deletion confirmations. Existing environments, backups and receipts stay retained.

Authoritative semantics: `docs/architecture/data-lifecycle.md`,
`docs/product/data-lifecycle-design-acceptance.md`,
`docs/implementation/lifecycle307-recovery-slice.md` and AGENTS.md.
This file fixes the new finite native proof scope; historical matrices keep their original stages.

Fresh isolated fixtures use current schema68, frozen branch source and actual base074a33ba,
the existing pinned PostgreSQL/MinIO/Caddy/Redis images, internal networks without host ports,
and dedicated named volumes verified before start. No old environment is reopened or upgraded.
Copy targets remain the five existing archive/restore slots; provider inspection remains bounded
to the two synthetic namespaces. Private configs/binaries/witness/raw logs never enter Git.

| Scope | Required native proof |
| --- | --- |
| pre-ISSUED | Real authenticated loopback HTTP, real Certification/current rights and transfer pin SPI. Pause after real pin admission in the uncommitted ISSUED transaction; independent SQL sees no candidate operation/pin/events/cost, native GET and TCP response bytes stay0. Positive commit permits one exact native read; same-key replay0 payload/I/O. Restriction-first blocks; ISSUED-first competitor has an observed PostgreSQL fence wait and fresh post-commit restriction blocks new read. Actual process exit before commit rolls candidate facts back. |
| worker | Existing Outbox→Redis/Asynq and PostgreSQL advisory lock. Broker failure then same-volume recovery; duplicate enqueue/held owner gives0 competing I/O; durable run start then pre-lookup exit, post-lookup exit and real orphan/restart recovery; lookup then current authority revocation blocks subsequent I/O while preserving actual costs. Completed/denied work replay0 I/O. Native metadata for synthetic deletion starts stays UNKNOWN_PRESENT with reservation; no fake absence. |
| archive/restore | Actual single-send ARCHIVE retains source, RESTORE targets quarantine, native exact bytes/hash/protection verify; current external ledger/fence admits ONLINE and fresh pinned read. Old/missing/gapped witness stays UNKNOWN; current restriction becomes BLOCKED; no generic admission. Original COPY response loss/producer exit and lookup worker exit retain UNKNOWN facts and recover only through metadata/GET, without COPY resend. |
| processing crash | If exercised, real NATIVE input read then owner process exit, with pins/history retained. Natural lease expiry alone must not release pins or admit another physical read; recovery under current restriction/unknown prior owner has0 new GET/output. Distinguish a queue Handler call from actual Redis task delivery. |

Reuse check: the existing transfer SPI, FixtureSubjectPins, Commands/fence, call journal,
Cost/Audit/Evidence/Outbox, Asynq, COPY recovery and FileRestoreLedger suffice.
OSS owns native MinIO COPY/GET and Redis/Asynq runtime. No new scheduler, ledger engine,
business HTTP route, dependency, migration or crypto scheme is planned.
Only bounded fixture wrappers/barriers and current-schema orchestration are added.
IAM admin helpers remain separate from root-free payload producer/read-only worker mounts.

Blockers: wrong scope/credential exposure, old fixture mutation, provider resend,
pre-commit bytes or native read, unproved stop releasing a pin, stale ledger admission,
lost/overwritten facts or costs, concurrency/idempotency violation, required path/tests failing.
Each phase is append-only; failures retain helpers/logs and require diagnosis plus an explicit
new attempt, never automatic resend. Source/binary/working and Git-blob hashes are recorded.

Real DELETE response loss, in-flight DELETE late hold, three-copy deletion mixed outcomes,
and current-source post-delete absence/completion remain outside this non-delete authority.
Passing this finite extension does not accept full D1, D2, production DR/IAM or close #307.
