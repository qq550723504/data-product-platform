# Native protection repair: finite preparation contract

The human selected retention-extension preparation only. Native source writes, DELETE, branch unlock/publication, allocation, budget initialization and new deletion execution are not authorized. C23 stays terminal; its P1841/R6255 source and all original observations, budget/pin/backup and failure facts remain unchanged.

Root cause: exact historical retention equality does not establish effective protection. Domain expiry semantics are correct and unchanged. This repair adds fixture acceptance checks rather than changing disposal eligibility.

Authoritative implementation: native_protection.py is the shared host acceptance gate for the complete native entry and non-delete preparation/rehearsal. It consumes actual SDK receipts after the existing full bytes/inventory/IAM aggregation. Its five-minute age limit bounds a multi-object receipt. All three source checks precede every backup or business rehearsal; actual RESTORE protection is checked before inspector enable. The Go remaining_protection.go gate independently uses fresh actual inspector facts (one minute), exact Copy identity and the compiled window END + two hours before protected-C command dispatch. RetainUntil may equal the exclusive recovery end; as_of must stay strictly before RetainUntil. Expired, missing, wrong version/provider, wrong mode, legal-hold substitution, stale/future and short coverage fail closed.

Dispatch diagnostics preserve error, admission and attempt independently, without arbitrary error text or secrets. An error does not establish admission or a physical provider send. Original generic C23 error remains preserved.

Renewal preparation uses the exact two existing survivor versions: business control-retention@2606d71f-207b-42a7-b671-4b7f4ca4d194 and mixed m1-c@81a8402e-390f-4247-9d34-552df5eadeb3, GOVERNANCE, LegalHold=false, expiry-only to 2026-10-24T12:40:00Z (Singapore20:40). The pure plan/check entry has no apply mode. Future writes must use existing MinIO SDK PutObjectRetention with VersionID, GovernanceBypass=false and MaxRetries=1, per-target max1/total max2; persist physical starts before calls, append observations/cost quantities, retain UNKNOWN without resending.

Actual write entry, one-shot journal/budget/owner containment, separately authenticated human metadata-write grant and new current-expectation profile bindings are explicitly NOT_IMPLEMENTED / NOT_AUTHORIZED. These are required before any real renewal or next candidate. This bounded slice prepares gates, regression and the precise proposal, not a runnable source write. Do not request execution approval against an unimplemented entry. After later authorized renewal, append new current observation/expectation evidence; never rewrite old frozen inventories. No new DatasetVersion is justified because bytes/content identity are unchanged.

Reuse check: existing lifecycle StorageInspector, MinIO official SDK, evidence journals, stopped-owner containment and Git archive tooling are reused. No second storage/policy/workflow engine or infrastructure is introduced.

Merge/review blockers for this slice: source builds/tests fail, required entrance bypasses protection checks, old facts modified, typed diagnostics conflate distinct outcomes, renewal plan expands exact scope or implies write authority. Actual expired source protection and absent write grant remain operational blockers even if this local code review passes.

No live native acceptance is claimed for this slice. SDK loopback protocol tests and recorded Docker/PG/checker compositions are non-delete regressions only. PR/main CI and current GitHub state are not revalidated by these tests. Remote branch remains untouched.
