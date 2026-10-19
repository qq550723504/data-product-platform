# #330 candidate evidence — 2026-10-19

Contract: [workspace object storage](../architecture/workspace-object-storage.md).
This is implementation evidence from an isolated regression environment, before
retained-runtime activation. #307 owns migration 69; this draft reserves 70 and
requires the 69 → 70 landing/rebase order.

## Reuse and ownership

One #330 writer uses `codex/issue330-workspace-storage`; #307 keeps its executor,
P1 recovery, IAM bootstrap and Outbox publisher filtering. Reused capabilities:
DatasetVersion upload/publication, existing lifecycle namespace/copy inventory,
workspace admission fence, physical-call journal, CostAllocation, transactional
Audit/Evidence/Outbox, minio-go SDK, unchanged Core STS policy and existing
create-only gateway. No storage/IAM/queue replacement or provider fork.

## Local validation

Real PostgreSQL with the final migration and native MinIO/gateway + scoped STS:

- New workspace-prefixed conditional write/readback and normal runtime reads.
- Native foreign workspace GET/PUT return 403; ordinary overwrite PUT returns
  403; legacy `datasets/` GET remains denied by the unchanged policy.
- Two retained-fixture STANDARDIZED/CURATED versions copied using the actual
  operator CLI; immutable original version JSON remains identical; command
  replay produces zero extra provider calls. Fifteen confirmed journal calls
  have fifteen strong CostAllocations in this test.
- Actual entity review and frozen native workflow produce readable outputs:
  four frozen inputs, three dependency bindings and sixteen mapping usages.
- Controlled-port PG tests cover concurrent copy registration, both real source
  FK/first-copy lock orders, stop-use/expiry, denied/unknown calls, immutable
  witness facts, overlapping namespace rejection and destructive-path refusal.
- SDK loopback tests cover EOF proof timing and journal failure after success or
  connection failure. All storage and existing OpenMetadata tests passed ten
  consecutive targeted runs. An earlier parallel full run exposed the storage
  error-classification defect (fixed) and the unchanged 500ms Windows process
  startup test timeout; its original failure receipt is retained locally.
- All 48 trial-controller Python tests and `go vet ./...` passed. Module tidy
  verification found no dependency changes after CRLF normalization.

The native regression resources have a distinct owner label, PostgreSQL database,
volumes, network, workspace, Actor and test-only STS. These tests do not use or
renew the original run's STS. No native object DELETE was issued.

## Original retained environment

Read-only preservation verification at `2026-10-19T10:28:03Z` confirmed:

- run `1b7e1949e8224ad8b870d28b96a3c6c2`;
- workspace `e7469d12-05bc-4427-b79a-356a49cfd13c`;
- Actor `0fb05174-99f2-429e-863e-1a841d82330e`;
- byte-identical registry SHA256
  `28ddcabfb560f6400962fd21d6233badecf8629b6e109c342226db34656d0fc9`;
- original containers, processes, volume/network identities and bound artifacts;
- unchanged three source tables, five versions, sixteen processing fact tables,
  48 AuditEvents, 29 Evidence rows and eight CostEvents;
- all five object byte counts and SHA256 values unchanged, using a separate
  operator-owned read-only diagnostic; this is not Core credential acceptance.

No original seed, SQL mutation/migration, copy, activation, credential issuance
or policy change occurred. Original Core STS expired naturally at
`2026-10-19T10:03:42Z`; its current usability is not claimed. Independent review,
PR CI, exact merge/main CI, activation and human acceptance remain separate.
