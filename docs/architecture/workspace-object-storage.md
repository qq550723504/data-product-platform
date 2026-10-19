# Workspace object storage — #330

## Finite implementation contract

Core owns DatasetVersion identity, workspace, publication, physical-location
admission and immutable evidence. The existing MinIO SDK owns S3 and STS.
No second storage service, IAM system, queue, provider fork or URI inventory is
introduced. The reference provider remains the existing controlled local trial.

New UploadVersion objects use
`core/<workspace>/datasets/<dataset>/v<version>/<write-attempt>/<filename>`.
Workspace comes from the persisted Dataset, never a body/header or filename.
The existing producer/idempotency allocation and winning READY publication are
retained. Each actual write attempt has its own create-only key.

The runtime profile binds one workspace, Actor/principal, provider instance and
incarnation, bucket, immutable `core/<workspace>/` prefix and finite credential
file. Full STS credentials include the session token. Invalid/expired profiles
fail closed; runtime cannot fall back to root or call EnsureBucket. Preparing a
bucket remains an operator responsibility. The Core STS policy is unchanged.

## Ownership and authoritative sources

On 2026-10-19 the owner of #307 (chat `实现 #307 引用需求与业务处置竞争`)
confirmed that #330 has one implementation writer in
`codex/issue330-workspace-storage`. #307 retains its remaining executor, P1
recovery, IAM bootstrap and Outbox event filtering. Its pushed reference is
`f59bf8b09aa1bbcf9cae3847d7ebc3389596f577`; #330 starts from main
`d00344c748c4ea313f06ebc51dd5c73210527ee8`, and reserves migration 70.
Migration 69 belongs to #307 / #323. The migration runner cannot insert 69 after
70 has already been applied. This candidate must remain draft until 69 lands
and the branch is rebased and tested in that order; 70 must not be applied first
to the retained database. No #307 migration is duplicated or renumbered here.
The candidate05 public frozen-snapshot checker and its tests are reused byte for
byte from the same #307 reference commit. CI invokes this existing checker;
the old validator, 317 source hashes and all historical evidence stay unchanged.

This document owns the protected non-versioned storage contract. DatasetVersion
publication continues to follow `native-execution-recovery-and-output-idempotency.md`.
`data-lifecycle.md`, `data-lifecycle-storage-reuse.md` and ADR-0015 continue to
own destructive lifecycle admission and versioned-copy semantics.

## Physical identity

The existing namespace/copy inventory is the only location authority. Identities
are explicitly distinguished:

* `PROVIDER_VERSION`: existing exact nonempty provider version, unchanged.
* `PROTECTED_CREATE`: provider instance/incarnation, bucket, workspace prefix,
  key, content SHA256/size and immutable native conditional-create/readback
  observations. There is no provider version and no invented replacement ID.

Namespace registration serializes on workspace admission and the physical
provider/incarnation/bucket. A whole-bucket `PROVIDER_VERSION` namespace cannot
overlap a `PROTECTED_CREATE` prefix; disjoint workspace prefixes can coexist.

The second identity is supported only behind the existing gateway which rejects
unconditional PUT, COPY and DELETE under `core/`. Native conditional creation,
bounded readback and content verification precede registration/publication.
Provider calls have durable start identities and append-only observations and
per-physical-call CostEvent/CostAllocation, independently of business success.
An uncertain create can be reconciled by a verified read of the same protected
key; its verification records `RECONCILED`, preserving the original uncertainty.
An observation/journal failure refuses publication. SDK body proof is persisted
at EOF before returning bytes, including SDKs whose deferred Close ignores errors.

Both source admission and first-copy registration acquire the existing workspace
delivery fence before checking the inventory, then follow parent-first ordering.
Current DatasetVersion usability and stop-new-use restrictions are rechecked for
reads and at publication. Uncertain calls/objects remain recorded, never guessed
absent. Native I/O must not run under a long-held database transaction.

`PROTECTED_CREATE` grants no delete, archive, restore, version-listing, retention
or hold capability. The existing versioned DTO and adapters reject this branch.
Unknown relations and qualified-admission gaps remain fail closed.

## Retained versions

The real retained trial has two published objects outside the Core prefix.
Their version IDs, original URI/checksum, producer identity, review decisions,
WorkflowVersion, execution inputs, lineage and existing evidence are immutable.
An explicit custody command can supply the exact existing bytes and add a
verified protected physical copy for the same DatasetVersion. It does not create
a business version, rerun a producer or mutate the original object.

The shared runtime reader resolves a persisted DatasetVersion and its registered
protected location. A missing location for an old `datasets/` URI is denied:
there is no root fallback, wildcard legacy grant or silent URI rewriting.
Original PGI RAW objects remain readable through their persisted, exact,
workspace-scoped Core URI and checksum.

The explicit operator entry point is `cmd/workspace-object-copy`. It requires
the same private workspace profile, `POSTGRES_DSN`, an existing version UUID,
an absolute custody file path and a stable idempotency key. It initializes the
existing Outbox routing before its first business/journal transaction. The file
must match the immutable SHA256 and byte count; the command has no source/root
credential option. A successful replay returns the same registered location
without another physical invocation. It does not activate the retained runtime.

## Validation and exclusions

Required checks: conditional native writes/readback through unchanged STS;
cross-workspace GET/PUT rejection; replay/concurrent publication; failed/unknown
call accounting; immutable old-version copy and reader resolution; first-copy
and source-admission race; protected identity rejected by destructive paths;
frozen historical versioned fixtures unchanged.

Implementation/tests use a separate worktree and explicitly owned regression
PostgreSQL/MinIO/gateway resources. The original #325 run, workspace, Actor,
containers, volumes, credentials and data are preserved. No seed, retained-object
copy, migration, activation, merge, deployment or deletion is performed by this
implementation task. Tests and PR CI do not establish retained-runtime activation,
production hardening, delivery authorization or human acceptance.
