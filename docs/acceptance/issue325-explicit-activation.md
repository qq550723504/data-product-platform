# #325 explicit protected-storage activation

## Finite contract and reuse

This slice adds an operator entry point for the already-upgraded retained trial.
It does not execute that entry point against the original run during development.
The source baseline is merged #331, `cf11592d688303cf4ae1126c431aab87736b75e2`.
The original trial, #307 executor/IAM/Outbox, existing migrations and frozen
historical validators/evidence are outside this implementation's write set.

Reuse `runtime` ownership/exclusive lock, `upgrade.build`, `resume` journals and
same-policy STS helper, the canonical migration runner, workspace-object-copy,
ProtectedObjects/SDK and existing costs/Audit/Evidence/Outbox. PostgreSQL JSON
queries and pg_dump provide inventory/backup; no second workflow, storage, IAM,
queue or deployment framework is introduced. A general deployment engine such as
Ansible would not own the trial's immutable bindings or same-run admission and
would add another control plane; this is only a bounded composition command.

## Plan and admission

`activation-plan` reads only. It requires a committed working tree, the same
origin and legacy business manifest, exact Docker daemon/CIDs/images/volumes/
networks/private artifacts and stopped containers/host owners. The recorded
phase may be RUNNING only when all native owners are proved exited; the plan
records this mismatch, it does not edit the registry. Foreign/reused/live PIDs,
occupied ports, UNKNOWN/transitional phases and unfinished journals fail closed.

Non-controller sources must exactly equal the approved #331 baseline. Only
`deploy/trial/`, `apps/platform/cmd/trial-resume/` and
`apps/platform/cmd/trial-activate/` may differ, and must match
the selected committed HEAD. Migration files, fixed workflow and industry inputs
are not admitted by a broad path whitelist.

A plan binds its target HEAD, origin/registry/legacy-manifest hashes, native
resource observations, a prior five-version fact receipt and exactly two explicit
custody files. Their version IDs, SHA256 and byte counts must match the original
STANDARDIZED/CURATED datasets; three RAW IDs come from existing receipt slots.
Custody is explicit local bytes, not a runtime root/legacy-prefix read fallback.
The digest contains stable admission material only, without generated timestamps.
The plan is an inspection result, not an execution authorization.

## Explicit execution (separately authorized)

`activate --plan <file> --plan-sha256 <digest>` re-derives the plan under the
existing exclusive lock before a mutation. A digest/HEAD/identity/byte/native
drift refuses execution. It never calls start, prepare, seed, ingestion, review,
workflow preparation/execution, certification, delivery or destructive commands.

1. Allocate a new journal and preserve before-state bytes. Record ACTIVATING
   before starting the exact retained containers; do not replace or recreate them.
2. With host writers stopped, inspect the same Core and source. Save pg_dump and
   a repeatable-read inventory of every public table, retaining original column
   projections and canonical JSON row multisets. Require original version facts
   and migration head 68.
3. Build API/worker/production web and existing helper CLIs in a new private
   directory. Recheck committed sources before any migration or provider action.
   Then apply only pinned 69/70 SQL using the existing runner. No bootstrap or
   seed; all old rows must remain unchanged.
4. Call existing renew-core once with its unchanged one-workspace Core policy,
   retaining old credentials and its existing
   `private/resume/<uuid>/core-identity.json` output constraint.
   Write a protected profile for the same workspace,
   Actor/principal, MinIO incarnation and bucket; no IAM/user/policy mutation.
5. Use workspace-object-copy for the two exact custody files with stable keys.
   Verify all five existing versions through the common ProtectedObjects reader,
   using finite credentials and its existing physical-call journal/cost ledger.
   Snapshot preservation is checked before host processes are released.
6. Append an immutable activation manifest referencing the old business binding.
   A distinct active binding selects new artifacts; old business/origin/receipts
   remain unchanged. Launch the same API/worker/web roles and ports. Require
   health plus original RAW/processing facts before recording PASS/RUNNING.

Normal resume subsequently restores the explicitly accepted activation artifacts;
it still cannot admit arbitrary business-source changes. Renewal may change only
credential reference/expiry, never protected scope or old profile bytes. A same
completed activation replay is read-only; it does not migrate, copy or reissue.

## Failure and recovery

Write each action start before invoking it and append its result separately.
Failures/interruptions record UNKNOWN and retain backup, artifacts, new copies,
physical-call history and all old state. There is no automatic rollback, delete,
new attempt or credential replay. An orphan journal also refuses a new execution.
The existing exact-owner stop remains available for containment; diagnosis and
any recovery from UNKNOWN require a separately scoped command/authorization.
Automated UNKNOWN recovery is an explicit follow-up, not claimed here.

## Acceptance and authority

Required: no-side-effect plan; source/identity/port/custody tamper refusal;
recorded/native state reconciliation only during explicit execution; old row
multiset preservation across added columns and new rows; strict migration order;
ordered stage/journal and UNKNOWN failures; active binding/restart profile
scope preservation; completed replay with no external effects; read-only helper
against isolated PostgreSQL; normal and optimized Python tests; current-HEAD CI
and independent review.

Original-run deployment, database migration, STS renewal, copy and activation are
NOT_RUN in this implementation task. No human business acceptance is inferred
from isolated tests, review or CI. All original resources/data remain retained.
