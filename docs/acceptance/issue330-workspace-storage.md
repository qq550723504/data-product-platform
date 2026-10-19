# #330 candidate evidence — 2026-10-19

Contract: [workspace object storage](../architecture/workspace-object-storage.md).
This is implementation evidence from an isolated regression environment, before
retained-runtime activation. #307 owns migration 69; this draft owns 70 and
follows the 69 → 70 landing/rebase order.

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

## Rebase after migration 69 landed — 2026-10-19

#307 owner merged the reviewed #323 as
`f22587d780bacc9c865b269fa20b1cb135034f5a`. Its exact
[push/main CI 39250314443](https://github.com/qq550723504/data-product-platform/actions/runs/39250314443)
completed successfully with 13/13 jobs before #331 was rebased. The squash tree
equals the reviewed `8b96cf35` tree; the original feature branch and evidence
tag retain the old frozen source anchors. #307 remains open and full D1 remains
NOT_ACCEPTED.

The protected-storage implementation patch is unchanged by rebase. The only
conflict was the demo workflow, resolved by retaining all main checks; frozen
candidate05 source/checkers/tests and later historical checks come from main.
No old validator, evidence or hash was changed. Windows checkout needed Git's
per-command long-path support; evidence paths were preserved.

The existing, separately owned regression PostgreSQL/MinIO/gateway were reused:

- A new empty database ran the canonical migration runner to 69, then the
  complete rebased migration set to 70, then an idempotent replay at 70.
  All migration records through 69 remained identical. Both owner migration 69
  hashes match the authoritative contract; 69 is inherited from main.
- All four migration-69 policy renewal integration tests passed, including real
  fence waits in both orders, cross-scope refusal and preservation of old facts.
- Protected-object PostgreSQL contracts passed, including both source/first-copy
  orders, denied and uncertain physical calls, custody replay and concurrent
  registration.
- Both native tests passed: finite workspace STS/foreign GET-PUT rejection and
  unchanged retained-fixture versions; actual entity review and frozen workflow
  with four frozen inputs, three dependencies and sixteen mapping usages.
  These are isolated fixtures, not activation of the original trial.
- Trial Python regressions passed in normal and optimized modes: 48 each.

The original run/workspace/Actor registry and 35 bound file hashes remain
unchanged. Its six original containers remain stopped, with the same six volumes
and two networks. No original seed, SQL migration/mutation, custody copy,
credential renewal or policy change occurred. SQL/object content was not freshly
read while those services were stopped. #331 remains Draft; independent re-review
and exact new-HEAD PR CI are recorded separately at the PR closure checkpoint.
