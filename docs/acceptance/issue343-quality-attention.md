# #343: read-only quality attention and exact evidence navigation

## Finite merge contract / ownership checkpoint

Base: main `a25ebac1c930d749daa085ef63ca3db8f0e7d6b8` (2026-10-22).
Writer: this chat, isolated `codex/issue343-quality-attention` worktree.
User authority: implement #343, run necessary isolated tests, publish a Draft PR;
no merge, deployment, or intervention in #325.

Refreshed active chats and open PRs before editing. #325 remains with its original
owner in #342 (trial controller). #307 remains with its original owner in #332;
its Web changes at this checkpoint are README/package-lock, outside our write set.
No existing app page or remediation source writer was found. The primary checkout
and other worktrees remain unchanged. Windows long-path checkout required a short
isolated path and command-local `core.longpaths=true`; no shared Git setting changed.

Authority: #343, AGENTS, V2/capability map/ADR-0014, existing C1 Quality/Workflow/
principal contracts. #337 and the private reference screenshots guide presentation
only. No third-party attachment or original screenshot is included in this PR.

Reuse: existing immutable remediation associations, Quality reports, Execution,
Human Decision principal, server-only request transport, tables/Badge/pagination.
OpenMetadata owns commodity catalog/incidents/workflows; this change only projects
already-existing Core facts. No second task engine, queue, rules, IAM, state machine,
OSS dependency, schema migration, or provider execution is added.

Write set: remediation read query/HTTP/tests; existing home/attention and focused
case/quality report reading; small Web read helpers/components/tests. Trial code,
write Commands, lifecycle migrations/routing, originals, data and credentials are
excluded. A case closure never changes Quality/Certification/Delivery facts.

## Validation

The read projection uses one PostgreSQL statement/snapshot for the filtered total,
summary and page. It uses lateral latest-fact selections plus EXISTS over **all**
pending rechecks/accepted nonterminal executions. Closing a case cannot hide those
facts. Ordering is created_at/id; separate requests do not freeze a workspace.
The Web table disables detail/report link prefetch, so rendering a page does not
fan out into case/report requests. Focused case navigation bypasses the old
100-case list and batch/receipt enumeration. The focused view exposes no Commands;
the original version-level handling entry remains available.

Report selection checks assessment/workspace/version identities and fails on
mismatch without falling back to latest. Latest quality summaries and certification
defaults still use the latest assessment. Recheck links use that recheck's actual
execution output version, not the source version or latest case execution.

Local validation (Windows, 2026-10-22):

- PASS: `npm run build` in apps/web (existing command regression suites, new
  5 navigation/scope tests, TypeScript/lint, production Next build).
- PASS: browser fixture contracts, 11 tests; complete existing browser regression,
  47 tests, followed by the final 5 quality-attention browser tests after adding
  explicit failed-execution/recheck and different-output-version coverage.
- PASS: real isolated PostgreSQL `go test ./internal/quality/... -count=1`;
  `go vet ./internal/quality/remediation/...`; final remediation regression.
  The C1 fixture reuses existing admission and controlled **in-process** adapters;
  it is not a real external-provider or retained-runtime trial.
- PASS: 105 real cases paged completely; multiple assignments do not multiply
  cases; closed+older pending recheck and closed+accepted active execution remain
  follow-up even when the latest recheck is ASSESSED. Empty/out-of-range pages
  preserve totals. HTTP validates trusted capability/workspace and pagination.
- PASS: before/after workspace-scoped fingerprints of remediation facts,
  assessments, executions, costs, evidence, audit and related outbox are unchanged
  by reads. Browser new-feature journeys make only GET requests, do not expose
  the server token, use one attention request per page and no per-case reads until
  navigation; exact case/report scope mismatches fail explicitly.
- LIMITATION: local `go test ./...` and `go test -p 1 ./...` fail in unchanged
  OpenMetadata `TestNativeSchemaCostsStartupBeforeMarker/exit` (500ms child-process
  deadline, `costs=0`, invocation history unavailable). The **unmodified base**
  `a25ebac1` package extracted with git archive reproduces the same full-package
  failure; the single test passes in both trees. No timing threshold or unrelated
  adapter code is changed. This is not reported as an all-Go PASS.

Main advanced to `810b5a8b1cd63443ac58bfcb02275b0fed62b228` (#342) during validation;
its delta is confined to trial code/docs and does not overlap this write set.
No #325/#307 source, active worktree, retained runtime, original data, credential,
configuration or volume was modified. PostgreSQL uses this Issue's new isolated
container and persistent volume; no shared cleanup is performed.

Initial PR CI `39718462729` found a live-core locator regression: the report now
also identifies its assessment, so a global exact-ID locator became ambiguous.
The existing live-core test now verifies the ID separately inside the latest
assessment and report sections. This retains both assertions without assuming a
business identity appears only once on the page. Ownership for this existing test
was checked against #307; no overlap was found. No runtime/fixture provider behavior
was changed by this correction.

Historical #298/#299 acceptance and deferred human explanation are not promoted
to current PASS. Full retained-runtime/provider/deployment acceptance is NOT_RUN.
PR CI is evaluated for the submitted head; no merge or push/main CI is claimed.
