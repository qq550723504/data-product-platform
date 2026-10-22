# #325 protected five-version continuation

Goal: continue the same already activated trial with its original five READY
versions and #339 preparation binding. Ordinary stop/status observation is SQL
and native metadata only. A separately authorized resume renews the unchanged
Core policy exactly once, then verifies all five objects through the existing
ProtectedObjects.ReadVersion path before launching the original host artifacts.

Authority: this contract owns continuation assembly; AGENTS.md owns immutable
history/cost/workspace rules; activation and business-prerequisites contracts
own their original bindings; Core owns protected reads and their actual call,
observation, CostAllocation, Evidence, Audit and Outbox facts. Source/CI success
does not authorize original-runtime execution.

Reuse check: reuse trial-activate's SQL snapshot/admission and five-version
verifier, existing native STS helper, private ACL checks, operator lock and
append-only journals. Reuse Core's protected SDK adapter and physical-call
ledger; no second storage client/queue/IAM/provider journal or schema is added.
Ansible or Temporal would duplicate the existing bounded operator lifecycle and
cannot replace the Core business invariants. No shared Core/storage/worker or
#307 changes are needed.

The protected path requires the accepted activation and its exact schema71,
five-version, custody and workspace/Actor/principal identity. Observation builds
an admitted helper into a new private path once per committed controller, binds
its hash, and writes local SQL snapshot inputs/receipts. It never invokes an
object SDK. Advisory plans do not invoke this observation and remain read-only.
Stop retains its containment ability if inspection fails, but a protected resume
requires a successful matching protected before-stop baseline before starting
any container or issuing STS. No stale three-RAW receipt may be adopted.

Before renewal and before host launch, compare the full public-table inventory,
original source rows, and frozen five-version/ingestion/review/execution/lineage/
rights facts. The existing SDK performs HEAD then GET per version: probe success
requires exactly five PROTECTED_HEAD and five PROTECTED_GET calls using each
version's preexisting intent, plus ten exact observations and typed invocation
costs/allocations and twenty Evidence/relations, Audit and Outbox facts. HEAD is
covered by the unchanged GetObject policy; its empty-body hash and each GET's
content hash are checked separately. Other new rows, missing old rows, altered
historical facts, an extra call, PUT, foreign scope, missing costs or mismatching
hashes fail closed. Known
Outbox dispatcher lifecycle columns may change while frozen event content and
membership remain exact; consumption must reference an explicitly required
handler of a retained event.

The physical call's existing Core journal persists before SDK work. Failed or
hash-mismatching reads retain their real costs and observations; the local
attempt remains UNKNOWN and is never retried automatically. Probe failure must
retain an after-snapshot, and no host may launch on that failure. Status and
same-running resume never repeat a probe or renew credentials. Legacy trials
without accepted activation retain their separate existing contract; incomplete
activation remains blocked and cannot opt into the legacy branch.

Write set: deploy/trial, cmd/trial-activate and this contract. Non-goals: original
deployment/stop/resume/renewal/object access, business preparation changes,
assessment/certification/delivery/remediation, seed, migration, artifact copy,
rights renewal, merge or Issue closure. Blockers: scope/root fallback, secret
leaks, lost history/cost, broad ledger exceptions, duplicate provider work,
incomplete-stage admission, or a failing required path/CI.

Validation: Python lifecycle/ledger mutation regressions and real isolated
PostgreSQL + controlled SDK five HEAD/GET-pair/hash/cost success/failure tests. Independent
review and CI bind the exact candidate HEAD. Original-runtime actions remain
NOT_RUN until separately authorized after merge/deployment.
