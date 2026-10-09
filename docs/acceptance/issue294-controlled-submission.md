# DPP #294 controlled Submission implementation and acceptance

## Merge contract

Goal: admit immutable controlled-fork Submissions into the existing Core Result/Review/Snapshot/Gold path. Core remains the only business reviewer.

Authorities: [Annotation Domain §3.1/§5–6/§8](../architecture/annotation-domain.md), [Engine integration §5.1](../architecture/annotation-engine-integration.md), [ADR-0013](../adr/0013-controlled-label-studio-submission-admission.md), and [#294](https://github.com/qq550723504/data-product-platform/issues/294). Baseline main is `010a80c30b5a74a638faaaa27ebc7206634606f4`.

Required invariants: workspace-owned connection/source identity; full immutable body/fingerprint; atomic SourceResultBinding + Result/revision/Event/Audit/Evidence; fresh authority and full source validation on every Result replay; acyclic same-task correction closure; exact seal/read/Gold integrity; parent-first fences; immutable late/conflict/quarantine receipts; complete finite quiescent enumeration; actual provider-attempt costs.

Reuse check: existing AnnotationEnginePort, engine operation/attempt/outcome, worker, transactional outbox, Audit/Evidence/CostAllocation, Snapshot and Gold checks are reused. New rows only record source identity/ownership, exact bindings, observations and finite-batch evidence. No second reviewer, workflow engine, queue or external source of business truth is introduced. Mature Label Studio remains the commodity annotation capability through the approved Adapter boundary.

Non-goals: continuously written project enumeration/read-fence, general operator or annotation SaaS UI, production/shared data, mirror publication, trust/credential changes, fork approval/release, and re-running the historical CE Pilot as proof of the new protocol. Public review retains its existing authenticated principal boundary; finite-batch preparation is an internal trusted orchestration Command.

Merge blockers: authorization bypass, incomplete/lost immutable history or cost, source/fingerprint mismatch, unsafe replay/parent-lock race, authoritative contract conflict, required build/test failure, and missing required new-candidate runtime acceptance. Optional performance/hardening beyond this contract belongs in follow-up work. At three substantive review rounds or twenty resolved comments, freeze this contract and classify new findings as BLOCKER/FOLLOW-UP/INVALID per AGENTS §17.

## Implementation

Migration 000050 adds immutable source bytes and typed context, append-only exact SourceResultBinding and observation receipts, immutable connection ownership, a finite expected assignment/revision set, and ordered batch receipts. Completion references its exact scan-start receipt and uses the Campaign lock to reject stale concurrent completion. Required original bindings are checked by a deferred constraint at Result commit. Review and initial sealing require the latest complete receipt.

Source identity excludes author, assignment/revision, task/project, binding/config and hashes; those dimensions enter the full fingerprint. The provider snapshot bytes and canonical label bytes are independently hashed. Stored source reads recompute fingerprints/body hashes and compare the frozen Core context. All Result replay and failure-reread routes use the same current authorization/source transaction.

Snapshot v1 retains its existing header contract and adds `sourceProtocol`, exact `sourceClosure` and the immutable complete batch receipt. CORRECT traces the original provider source without fabricating another Submission. Finalization and GetSnapshotIntegrity verify the entire closure; existing Gold consumers use this integrity gate. FINALIZED roots are not rewritten. Late observations retain physical-attempt context and Audit/Evidence/Outbox without changing selected Results, Decisions or membership.

The controlled Adapter only calls ordinary project/Submission list/detail and assignment read APIs, compares exact label_config SHA-256, independently canonicalizes the Python snapshot, verifies list/detail agreement, and quarantines fork human review. Controlled HTTP calls require the existing physical-attempt observer, reject redirects, and retain distinct invocation costs/outcomes. An unavailable or mismatched controlled endpoint never selects the CE path. Current assignment.version is recorded as OBSERVED_CURRENT with observed_at in an append-only receipt and its Audit/Evidence; it is excluded from source identity/fingerprint and never inferred as the submit-time token. Current reassignment does not rewrite submitted_by.

## Configuration and finite orchestration

Use a dedicated authorized loopback synthetic environment. Freeze these server-side values before preparing the Campaign engine operation:

- `LABEL_STUDIO_ADMISSION_PROTOCOL=controlled-fork-submission-v1`
- `LABEL_STUDIO_CONNECTION_ID`: dedicated UUID, irrevocably owned by one workspace on first binding
- `LABEL_STUDIO_PROVIDER_INCARNATION`: stable incarnation identity
- `LABEL_STUDIO_SOURCE_COMMIT`: exact 40-character source commit
- `LABEL_STUDIO_ENGINE_VERSION`: exact candidate version
- `LABEL_STUDIO_IMAGE_DIGEST`: exact `sha256:...` candidate digest
- `LABEL_STUDIO_NORMALIZER_VERSION=label-studio-single-label-v1`

Existing endpoint/token/instance/timeout settings remain server-side configuration. Candidate changes require a new appropriate binding/incarnation; a worker configuration cannot silently replace a frozen contract.

Trusted finite orchestration completes the planned browser writes, drains in-flight writes and stops new writes before calling `PrepareControlledSubmissionBatch` with the exact Task/assignment/revision set and page budget. The public request body cannot self-declare quiescence. The worker performs full list/detail enumeration twice, detects drift and checks the expected set. Any failure appends UNRESOLVED evidence and blocks subsequent review/seal. Reconciliation of a SEALED Campaign records observations while the existing frozen receipt/root continues to explain the historical snapshot.

## Evidence and remaining dependencies

Local synthetic checks on 2026-10-09:

| Check | Status | Evidence |
| --- | --- | --- |
| Fresh PostgreSQL 16 install through migration 000050 | PASS | Dedicated task container, new empty database |
| Full `go test ./... -count=1` with real PostgreSQL | PASS | Fresh `dpp294_observed` database; includes Gold/rights/delivery/acceptance regressions |
| `go vet ./...`; API/worker/migrate/outbox-replay build; Compose validation | PASS | Native Go and isolated environment |
| Empty down/up and historical down refusal | PASS | Parent-first exclusive locks; immutable source history blocks destructive rollback | PASS | controlled_submissions_integration_test.go |
| Dual-connection review-first barrier, double finalizer, correction closure | PASS | Real PostgreSQL locks and independent review pool |
| Missing/tampered source, finite completeness/drift/budget/revocation, late receipts | PASS | Source read/Gold preflight fail closed; immutable root retained |
| CPython golden vectors and controlled list/detail, pagination, auth/config/downgrade failures | PASS | Independent CPython 3.12 vectors in testdata; httptest contract |
| Exact PR HEAD CI and independent review | PENDING | Record separately in the PR; local self-check is not independent review approval |
| LS #76 new-candidate browser Submission → Core → Review → Snapshot → Gold output | NOT_RUN | New source/version/digest and an accessible authorized target have not been delivered |

One initial full-suite rerun against a previously used database hit an existing fixed CE fixture's external project unique key. A subsequent complete run against a fresh empty database passed. This is not a protocol PASS inferred from skipped integration tests.

No LS candidate is claimed by the synthetic source/version/digest used in fixtures. [LS #76](https://github.com/qq550723504/annotation-engine-label-studio/issues/76) belongs to the original Fork writer. Its fixed source/version/OCI digests, receiving-side availability/checksum and authorized target must be attached before runtime acceptance. The historical #72 PASS is not inherited. #294 stays open pending this evidence and required review/CI; nothing is merged or deployed.

## Independent review P1: COMPLETE membership fence

On 2026-10-09, independent review identified an original Result admitted outside the frozen finite batch after a COMPLETE receipt. A real PostgreSQL run at c2b2668a3db62b36b8a17838c0a6db9d781f520f reproduced the entire bypass: source501/rev1 was accepted and COMPLETE; source502/rev2 was accepted with the current Task revision; Review returned no error, Snapshot became FINALIZED and Gold preflight returned blocking=false.

The correction confines every controlled original Result and replay to the frozen Task/assignment/revision set under the shared Campaign parent lock. The exact SourceResultBinding trigger applies the equivalent guard to direct DB writes. Source-only late/quarantine observations remain permitted. A single SQL predicate compares the receipt IDs/fingerprints and frozen expected slots with the entire current original Result/binding set at receipt creation, review, sealing and read/Gold integrity. CORRECT shares the original source and does not add an expected slot. Initial sealing must bind the latest exact COMPLETE receipt; read/Gold checks the frozen receipt, so later sealed observations do not replace historical evidence.

Real PostgreSQL regressions: the old end-to-end reproducer now rejects the extra Result and permits legal replay/normal review/seal/Gold; direct DB out-of-set bindings fail before and after COMPLETE with full rollback; stale/tampered receipt membership is rejected by Command/direct DB review and seal and by read/Gold. Independent connections and pg_stat_activity Lock barriers cover a writer queued before review/seal; review-first and double-finalizer coverage remains. Runtime/browser acceptance for the LS #76 new candidate remains NOT_RUN. The PR records the exact fix SHA/CI and review recheck status separately.
Fix verification used a fresh dpp294_review_full database: migration 000050 up/down/up, full go test ./... -count=1 with real PostgreSQL, vet and API/worker/migrate/outbox-replay build all passed. Historical down refusal also passed. The strengthened writer-first/review-first lock-barrier cases passed three consecutive runs in the dedicated test database.