# #359: unobserved attempts in already observed schema requests

## Finite contract and ownership

Baseline main: `c7ac866aabcc4809833a10342f668a76db93baa6` (#357).
This chat owns `codex/issue359-schema-unobserved-attempts` in the short isolated
Windows worktree `C:/Users/Henry/.codex/worktrees/a359`. The user authorized
implementation, necessary tests with this Issue's own isolated resources and a
Draft PR. Ready, merge, deployment and retained-runtime operations are excluded.

Before editing and before submission, current main and open PRs #358/#332/#295
were checked. Their exact heads remained unchanged; bounded exact base/head file
diffs showed no overlap in this slice's write domain. Active chats/worktrees were
also checked. #325 and #307 retain their original owners, worktrees, environments,
data and acceptance records. No work or messages were dispatched to them. The
primary checkout remains unchanged.

Authority is #359, AGENTS, existing source-schema contracts and #299/C2,
V2/Capability Map/ADR-0014. #352's whole-request collection and #356's independent
read boundaries remain intact. This document records implementation and evidence;
it does not add a parallel product contract.

## Reuse and read boundary

Reuse source_schema_request/attempt/observation, PostgreSQL/pgx, the metadata
authorizer and server-only Core client, existing home/attention sections,
React Suspense, tables/paging and exact resource request/observation modes.
Internal PendingSchemaAttempts carries an opaque runtime contract and is not
exposed. There is no new dependency, infrastructure, table, migration, provider
adapter, scan/worker/recovery behavior or Command.

- `GET /api/v1/workspaces/{workspaceId}/source-schema-requests/observed/unobserved-attempts`:
  default limit 25/max 100; bounded nonnegative offset; invalid values rejected.
  One row is one attempt. The request must have at least one observation and the
  attempt must have none bound to itself. EXISTS avoids multiplication by request
  history. Workspace/resource/request/attempt joins enforce actual ownership.
  Count, ordered page and readAt come from one SQL statement snapshot. Ordering
  is registeredAt DESC/attemptId DESC; separate reads do not freeze a workspace.
- The response whitelists identities and display facts, including frozen request
  responsibility/source/rule references and attempt registration time. Current
  resource names are display labels. No Plan, runtime contract, provider/profile,
  Scan/Stop/Invocation, source/batch enumeration or per-row detail lookup is used.
- The storage field started_at records attempt registration before invocation.
  It is displayed as registeredAt, never as physical scan start. Missing
  observation does not imply RUNNING, QUEUED, timeout, failure, success or recovery.
- The attempt count is independent of old request counts and existing current
  review/execution/release totals. No new current unrecovered/manual task is
  inferred. Unbound observations and observations for sibling attempts do not
  conclude this attempt; only its own bound observation removes it from this set.
- Both host pages give the new section its own Suspense and anchored UNKNOWN
  placeholder. Failure, denial, unavailable credentials and invalid response stay
  local. A successful zero only describes this collection at readAt, not health.
  The other four governance sections and exact evidence links remain usable.
- Exact evidence reuses the authorized request/attempt and observation GETs.
  The selected attempt is independent of the attempt page and collection
  membership. Arrival of its observation preserves the old link and permits
  navigation to that exact observation; no latest/current/provider fallback.
- A typed, allowlisted return origin carries only existing bounded filters and
  offsets through request/attempt/observation paging and back to the same list.
  Observation URLs carry requestAttemptId separately from the observation's
  actual attempt. Viewing a sibling observation returns to the original selected
  attempt. An extra exact scoped detail GET validates that return identity; no
  arbitrary return URL, implicit reselection, polling or link prefetch is added.
- GET/navigation does not change requests, attempts, observations or handling,
  and produces no Execution/DatasetVersion/QualityAssessment, Cost, Domain Event,
  Audit/Evidence or Outbox. Existing authorization and business Commands remain
  authoritative; a recorded responsibleActorId is not caller authority or Steward
  assignment. This slice introduces no new multi-user IAM model.

## Validation (local, 2026-10-22)

- PASS: real PostgreSQL Query/HTTP test with 105 unobserved attempts under one
  request with 31 historical observations. Full pagination preserves unique
  attempt IDs and frozen request actor; history does not multiply rows. Requests
  without observations/attempts remain in the separate old collection.
- PASS: read-only DB connection, forbidden observer and private-runtime sentinel,
  before/after request/attempt/observation/cost fingerprints; bad pagination,
  missing/wrong credentials, nil authorizer, workspace allowlist/foreign workspace
  and mismatched resource/request/attempt are rejected. DB failure is 503, not zero.
- PASS: unbound observation preserves all attempts; two-connection reads before
  and after commit for MATCH/DRIFT/UNAVAILABLE/DENIED/INTERRUPTED only remove each
  bound attempt. Exact detail/observation remain available, the selected attempt
  remains exact outside its page, and all 31 historical observations remain intact.
- PASS: six related Go packages with this Issue's PostgreSQL DSN (metadata
  domain/application/infrastructure/HTTP, rights HTTP, principal), metadata
  infrastructure/HTTP vet, and API/worker build.
- PASS: Web lint, standalone typecheck, production build and its unit suites;
  schema validation/navigation unit suite 13/13. Fixture contracts 15/15.
- PASS: complete browser suite 93/93, including seven new journeys for full
  pagination, count/filter independence, exact/sibling/late-observation return,
  wrong scope/identity/origin, local failure/denial/malformed reads, real zero
  versus UNKNOWN and refresh recovery. Existing four governance sections and
  review/execution/release behavior remain covered.
- PASS: held new GET on each host, with assertions before explicit release:
  local UNKNOWN, all other governance sections, unchanged known current total and
  exact release navigation remain visible. Finally releases the read; recovery
  restores one section anchor. New journeys assert GET-only, no forms/provider
  fallback or private sentinels. The list also asserts one summary GET and no
  request detail fanout before navigation.
- The first list screenshot caught the streaming placeholder. The test now waits
  for the section to be visible and captures that section. The affected seven
  tests passed again (7/7); list and exact post-arrival screenshots were inspected.
  Browser fixture evidence and real PostgreSQL HTTP evidence are separate.

Ignored logs/configuration stay in `.data/a359`; screenshots are generated browser
outputs. Tests used only the new PostgreSQL 16-alpine container/volume
`dpp-a359-pg-20261022`, labelled codex.issue=359/codex.worktree=a359 and loopback-only.
Its exact ID was verified before stopping. At handoff it is stopped (exit 0),
with its named volume retained; all owned Web/fixture/DB ports have no listeners.
The helper completion guard rejected a data-mounted service and retained its
CREATED ticket/resources; no removal/archive completion is claimed or retried.
No retained Core/database/provider environment was used or modified.

PR CI is reported separately for the exact submitted HEAD in the PR checkpoint.
Actual merge-SHA push/main CI, deployment, native provider/retained-runtime and
human acceptance are NOT_RUN under this authority. Existing #298/#299 acceptance
is not rewritten.

## Merge boundary

Blockers are authorization/data leakage, historical or precise identity loss,
broken collection/count/read snapshot rules, UNKNOWN becoming zero/CLEAR/success,
unrelated failure hiding a required available section, or required path/build/CI
failure. Full pending/retry/runtime dashboards, health/timeout inference, new
scan/recovery actions, notifications, IAM and global data-layer redesign stay
outside #359. No new follow-up Issue was necessary during implementation.

Keep Draft/open. No Ready, merge, deployment or retained-runtime operation;
do not intervene in #325 or #307.
