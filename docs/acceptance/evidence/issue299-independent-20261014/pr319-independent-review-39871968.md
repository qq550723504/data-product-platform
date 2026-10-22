# PR #319 independent AI review

Reviewed head: `3987196872f7713bb706686d9c3318af3b5cbe11`.
Base: `074a33bace7edbaf481759aac3a84708363ffd02`.
Reviewer: separate read-only agent `pr319_independent_review`, started with a fresh context; the author retained sole write ownership.

The contract is trial image isolation in the existing B1/B2/B3/storage runners and truthful retention/recovery evidence. AGENTS.md, #299, the existing ingestion contract and this acceptance report are authoritative. Production infrastructure, #307, recovery of the lost Core and human explanation acceptance are outside this review. CI and AI review do not constitute human approval or permission to Ready/merge.

## Findings

**BLOCKER / P2 — the new regression requires historical shared aliases.** At [lines 25–27 of the reviewed source](https://github.com/qq550723504/data-product-platform/blob/3987196872f7713bb706686d9c3318af3b5cbe11/examples/enterprise-activity/postgres-ingestion/image_isolation_test.py#L25-L27), both legacy tags are inspected unconditionally. A clean Docker daemon, including one that has only run the corrected launchers, lacks these tags. The runner fails before any build or isolation check. The independent reviewer reproduced this with controlled Docker responses in an isolated temporary directory, with zero native Docker calls.

Record explicitly absent aliases as a valid initial state, preserve presence and ID in the before/after comparison, and continue failing on unrelated inspect errors. Validate absent/present aliases, daemon errors and aliases created, removed or repointed during a trial. Residual blockers at this head: **1**.

## Independent checks

- All immutable-ID callers, 84 artifact Git blob hashes, five candidate source hashes and 759 main baseline source hashes/tree matched their receipts.
- The actual native save archive had all 22 unique OCI blobs, matching size/digest, complete reference closure and archive SHA256. Both minimum provenance statements were recomputed in memory from the original full records and matched the original attestation digests. Read-only Docker inspection resolved both original image IDs and the recorded shared aliases.
- Fifteen actual trial credential values were scanned against all public artifacts with no matches. Thirteen screenshots matched the report.
- Seventeen historical fact groups and sixteen published fact groups across four phases matched. Core/source/metadata backup receipt fingerprints and the 192 PASS / two SKIP test stream matched the report. These are receipt audits, not a rerun of the six business conditions or private database restoration.
- PR CI run `38557969029` was `event=pull_request` at the reviewed head, with 11/11 SUCCESS.

The existing shared MinIO cache concurrency model was not claimed as concurrently validated and is not a new blocker for this scope. No new follow-up issue was required. The reviewer changed no files, submitted no GitHub review, invoked no Core Command, started no services and made no Docker write operation.

The initial finding and FAIL remain historical evidence. Fix validation and a new exact-head review must be recorded separately. Draft/OPEN and the deferred Core/human boundaries remain in force.
