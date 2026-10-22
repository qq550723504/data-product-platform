# #356: workbench and attention local read isolation

## Scope and authority

Baseline main: `8b9cf7a3d3af3916be763d1c0d851198d7c82627` (#354).
The user authorized implementation, necessary isolated tests and a Draft PR only.
The sole writer for this slice uses `codex/issue356-workbench-isolation` in
`C:/Users/Henry/.codex/worktrees/w356`. The managed worktree attempt hit Windows
path limits; the native short-path worktree was created at the same exact base
without changing shared/global Git configuration.

Before writing, active chats, worktrees and open PRs #355/#332/#295 were checked.
Exact base/head diffs for the intended pages, helper, fixture/tests and this
document showed no direct overlap. #325 and #307 keep their existing owners,
worktrees, environments, data and acceptance records. No messages or work were
dispatched to them. The primary checkout was not changed.

Authority is #356, AGENTS, V2/Capability Map and the existing API contracts.
#343/#346/#349/#352 retain their respective collection, count and evidence rules.

## Reuse and root change

Previously, one failed prerequisite Promise.all read caused the host page to
return a whole-page LoadError before rendering the quality/schema components.
Both host pages now start independent, shared-per-render read promises and render
their consumers behind native React Suspense boundaries. Existing quality/schema
sections render outside those dependencies.

Reuse existing React/Next server components, API clients, paging, tables, badges
and evidence navigation. The small read helper returns an explicit unavailable
value for rejected/invalid reads. It does not return raw upstream exceptions.
No new dependency, API, Core model, migration, cache, queue, worker, retry system
or persistent state is introduced.

| Existing read | What depends on it | What stays independent |
| --- | --- | --- |
| Home workbench summary | Metrics, review count, reference-chain counters | Recent/failed executions, release facts, all governance sections |
| Recent executions | Recent execution list only | Current unresolved count and all other sections |
| Failed-execution attention | That queue and its contribution to current total | Other queues and evidence |
| Product list | Display names only | Validated release identities, counts and links |
| Release attention snapshot | Default status pages and available status counts | Reviews, executions, governance, explicitly requested nonzero release pages |
| Explicit release page | That status/page; its successful total when available | Other status pages; snapshot count can remain visible if only the selected page fails |
| One selected release readiness | Supplementary blocker details only | Known VALIDATING fact, exact release navigation, other sections |
| Quality / DRIFT / exception / request query | Its own existing section | All other sections and existing current-task total |

A shared API response is validated as one result; a missing field does not
invent known sibling values. Host summaries/pages check required count shapes,
page/filter membership and the primary identifiers used by their renderers.
Workspace-bearing results must match the configured workspace. Release reads
continue to use the existing workspace-scoped API. Existing server-only
credentials, metadata authorizer, Commands and feature switches are unchanged;
this is not a new multi-user IAM boundary.

The helper bounds the UI wait at 60 seconds. This is a read-display deadline,
not a source scan timeout or a persisted business/runtime state. It does not
cancel the existing underlying GET, retry it or invoke a provider. Timers are
cleared after resolution. Other Suspense regions can render while that read is
still pending; product-label hold tests verify this before releasing the GET.

## Count and evidence contract

- Missing, failed, denied or invalid necessary counts keep the complete current
  total UNKNOWN / incomplete. Unknown is never coalesced to zero. A fully known
  zero preserves the existing empty/CLEAR meaning within that queue's scope.
- Quality, DRIFT, scanning exception history and unobserved requests remain
  separate and are not added to current review/execution/release totals.
- On nonzero release pages, a failed shared snapshot does not prevent the
  explicit page GET. Conversely, a successful snapshot count can remain visible
  when the selected page GET fails; that page is visibly unavailable. No automatic
  fallback to a different page/object/route occurs. A first-page link is an
  explicit user navigation and preserves the other filters.
- Names can degrade to the already validated product ID. Readiness failure does
  not hide a known release or assert a new readiness decision. Different reads
  are not described as one frozen snapshot.
- Existing bounded filters, offsets and exact request/attempt/observation/case/
  assessment navigation are reused. Explicit evidence identity never becomes
  latest/current/provider data. Links keep the existing read-only/prefetch boundary.
- No new action, automatic polling, source/batch enumeration, provider call,
  Command, business write, event, cost, audit/evidence or outbox effect is added.

## Validation (local, 2026-10-22)

- PASS: Web lint, typecheck, production build and all unit suites in that build.
- PASS: trace/paging/scope/read suite 26/26, including five new read/count/identity
  tests for rejected/synchronously throwing/invalid reads, bounded read wait,
  complete/unknown/overflow counters, shared summary scope, filtered pages and
  exact readiness identity.
- PASS: browser fixture contracts 14/14. Faults are token-controlled and limited
  to named GET routes; unknown keys/modes are rejected. They do not alter business
  Commands. A held read is released explicitly or by fixture reset.
- PASS: complete browser contracts 78/78, including 18 new isolation journeys.
  These cover each existing host prerequisite failure, denied/malformed reads,
  reverse governance failures, all reads unavailable, successful empty versus
  unknown totals, recovery on refresh, selected release page/snapshot independence,
  failed readiness and product names, held product GETs, and exact attempt ->
  arriving observation -> original evidence/filter/page navigation during an
  unrelated release failure. New read-only journeys assert GET-only, no forms,
  no provider/source/batch reads and no credential/private-error sentinels in HTML.
- The first browser pass was 77/78: the pending-label assertion required an
  isolated exact text node while it was part of a release row. It was replaced
  with a deterministic held-GET assertion against the containing section, then
  the full suite passed. The wait is released in finally.
- Screenshots of all-UNKNOWN and partially available attention pages were
  inspected; available sections and local errors remain readable.

Evidence logs stay in ignored `.data/w356`; browser outputs are local generated
artifacts. Only this worktree's own Web/fixture test processes and ports were
used, and those processes exited. No Core/database/Docker/provider or retained
runtime was started, stopped or modified. No PostgreSQL/Core test claim is needed
for this Web-only slice.

PR CI belongs to the exact submitted HEAD and is recorded in the PR checkpoint.
Merge-SHA push/main CI, deployment, native source, retained-runtime and human
acceptance remain separate and NOT_RUN under this authority.

## Merge boundary

Blockers: authority/data leakage, evidence substitution/loss, breaking existing
count rules, reporting unavailable facts as zero/CLEAR/success, hiding a required
usable section behind an unrelated failure, or required path/build/CI failure.
No new follow-up was needed during implementation. Pending/retry dashboards,
health/scan timeouts, notifications, complete IAM, global data-layer redesign
and generic retry/circuit-breaking remain outside #356.

Keep Draft/open. No Ready, merge, deployment or runtime operation; do not
intervene in #325 or #307.
