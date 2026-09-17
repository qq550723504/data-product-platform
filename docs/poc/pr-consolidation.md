# POC candidate consolidation

## Baseline and choice

Main inspected at `70f4e7af787426ca37f30a2782e3678d9866ed08`.
The consolidated candidate descends from #90 head `0dae5144d0ffb2f251d408ea056dfc48edebaabf`, which already contains #88 head `88d5dabbe6f3add87f6de64e8b34bb0e7063c425`.
Parallel #89 was reviewed at `509ca000533f34153451d5d2ed218867f8fc4d83`.

| Source | Preserved outcome / deliberate choice |
| --- | --- |
| #88 | One runtime readiness guard, server boundary and UI consistency, all-console runtime rendering, isolated HTTP fixture and Chromium suite |
| #89 | Port missing matrix cases: string blockers and future FAIL/REVIEW/null/boolean values; verify readonly provenance, visible review outcome and canonical failed-rights UI; include readiness tests in the ordinary release/build script |
| #90 | Entire real Core/browser slice, real Redis/MinIO/PostgreSQL checks, native entity-resolution lineage repair and regression |
| This increment | Dedicated repeatable local demo, manual review/publish, persistent stop/start, fail-closed incomplete preparation, read-only persisted verification and lifecycle CI |

Do not copy #89's alternate `evaluateReleaseReadiness` implementation over #88's guard: there should be one source of truth, not two competing APIs. #89's diagnostic-code format was private to its unmerged UI/tests; no public Core error contract is changed. Canonical scenarios are covered in the retained guard/server boundary and browser suite. Additional provenance and state-display assertions are retained rather than deleting the behavior with the duplicate branch.

The retained readiness test file has **39 cases** and the fixture browser suite has **13 cases** after consolidation. Earlier #88 artifacts correctly record their earlier 34/12 counts; they are historical results, not claims for this new candidate.

## Review procedure

One consolidated PR targets main. After this candidate's existing CI, fixture browser CI, live Core CI, and demo lifecycle CI are all checked, superseded PRs #88/#89/#90 may be closed with cross-links. Preserve their branches, commits, discussion and old artifacts. Do not merge both parallel readiness implementations, force-push either author's branch, or treat PR closure as issue completion.

This document records the intended handoff; current open/closed state and actual verification are recorded in the consolidated PR. It does not authorize automatic merge or deployment. Existing main remains untouched by candidate creation. Recheck main and the superseded heads immediately before changing their PR metadata; stop on unexpected concurrent changes.
