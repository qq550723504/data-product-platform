# #352: unobserved schema requests and exact request/attempt evidence

## Finite contract and ownership

Baseline main: `18cbd1e7dc6650e200241f87135fbc7b31bf8482` (#350).
This chat owns `codex/issue352-schema-unobserved` in the short isolated Windows
worktree `C:/Users/Henry/.codex/worktrees/u352`. User authority is implementation,
necessary isolated tests and a Draft PR; no Ready, merge, deployment or changes to
existing runtime environments. #325 remains with its existing owner.

During implementation main advanced to `549f469967dcfd37d56cb211cbfda5c40b1c028b`
(#351 and #348). Their complete changed-file list has no overlap with this slice.
The isolated branch was rebased onto that main; their existing implementations
are inherited unchanged. The affected build, read and navigation checks were
repeated on the combined candidate, without operating either retained environment.

Before editing, current main, active chats/worktrees and open PRs #295/#332/#348/
#351 were checked. #332's GitHub file list exceeds the API cap, so its exact local
base/head diff was checked for the intended metadata/Web/browser paths. No direct
overlap was found. Their bootstrap/routing, worker, lifecycle, source admission,
trial, dependency/lockfile, CI and deployment write domains are excluded.

Authority is #352, AGENTS, existing metadata/source-schema contracts, #299/C2,
V2, Capability Map and ADR-0014. #337 only informs product presentation.

## Reuse and read boundary

Reuse Core source_schema_request/attempt/observation, PostgreSQL/pgx, existing
metadata authorization and server-only credentials, home/attention sections,
resource exact modes, tables/pagination and exact observation evidence. Native
scanning remains with the existing OpenMetadata Adapter/Connector. No second task,
queue, monitoring system, domain lifecycle, dependency, business table or migration
is introduced; this is a bounded projection of existing platform facts.

- `GET /api/v1/workspaces/{workspaceId}/source-schema-requests/unobserved`:
  view ALL/NO_ATTEMPT/ATTEMPT_RECORDED, default 25/max 100, bounded nonnegative
  offset. One request is one row. Any observation kind excludes the request.
  Classification counts, filtered total/page and readAt share one SQL snapshot;
  requestedAt/id ordering is stable. Separate requests do not freeze a workspace.
- `GET /api/v1/data-resources/{resourceId}/schema-requests/{requestId}` with
  workspaceId and optional explicit attemptId: scope checks require the exact
  workspace/resource/request/attempt. Detail has independently paged attempts and
  observations (default 25/max 100) and a selected attempt independent of its page.
  No implicit latest selection, LIMIT 50 dependency or list membership filter.
- Request identity, binding/source, responsibility, rule/version/hash and baseline
  come from frozen facts. Current resource names are display labels. The API returns
  a field whitelist, never the opaque runtime contract or credentials.
- Attempts are registered before native invocation. registeredAt describes that
  fact, not actual scan start. No runtime, queue, timeout, failure or recovery is
  inferred from missing observations or elapsed time. Requests with an older
  observation and a new pending retry remain outside this first collection.
- Any later observation removes the request from the collection but preserves
  its exact request/attempt link; removal proves neither success nor recovery.
  Observation links return through the exact request to the original list filter/
  offset. Bounded local links disable prefetch; no arbitrary return URL or polling.
- New resource request mode branches before current resource/provider pages and
  the old observation mode. All reads use Core only: no Plan, provider, Scan,
  Stop/reconciliation, source/batch enumeration, queue reads or Commands.
- Home and attention counters for existing current tasks are unchanged. New
  counts are independent. Local read failures show UNKNOWN, not zero/CLEAR.
  Quality, DRIFT/handling, exception history, certification and delivery remain
  separate facts; no read produces domain events, Audit/Evidence/Cost or Outbox.

## Validation (local, 2026-10-22)

- PASS: new real PostgreSQL Query/HTTP test with 105 unobserved requests (53 without
  attempts, 52 with registered attempts), complete pagination, filters, old exact
  requests, separate observed requests of all five kinds and a 32-attempt/31-
  observation retry history. A selected attempt remains exact outside its page.
- PASS: two-connection transaction visibility before/after each observation kind
  commits. Membership changes without losing exact request/attempt/observation
  identities. Older observed requests with pending retry attempts stay excluded.
- PASS: bad pagination/IDs, missing/wrong credentials, nil authorizer, workspace
  allowlist and mismatched resource/request/attempt checks; a different configured
  actor and changed current binding do not replace frozen request facts. Read-only
  connections, history fingerprints, forbidden Plan and private-runtime sentinels
  verify the read boundary; DB read failure returns an explicit error.
- PASS: six relevant Go packages (metadata HTTP/infrastructure/domain/application,
  rights HTTP and principal) with this Issue's PostgreSQL DSN; metadata HTTP/query
  vet and API/worker build. This does not claim a full native helper/provider suite.
- PASS: Web lint/typecheck, production build and its existing unit suites; schema
  navigation/validation unit tests 10/10, fixture contracts 13/13.
- PASS: initial complete browser contracts 60/60. After the final request-to-
  observation round-trip correction, the affected schema suites passed 12/12;
  the final four request journeys also passed with an explicit unchanged current
  task count assertion. Screenshots of the new list and exact evidence were inspected;
  browser fixture evidence is separate from PostgreSQL HTTP evidence.

Only this Issue's new PostgreSQL 16-alpine container and volume are used:
`dpp-u352-pg-20261022`, labelled codex.issue=352/codex.worktree=u352, loopback-only.
Ignored configuration/evidence stays in `.data/u352`; no existing data, provider,
worker, credentials or protected environments are used. At handoff the owned
PostgreSQL is stopped with volume retained; fixture/Web processes are exited.

PR CI is reported for the exact submitted head in the PR checkpoint. Merge-SHA
push/main CI, deployment, native-source/retained-runtime and human explanation
acceptance are separate and NOT_RUN under this delivery authority. Existing
#298/#299 acceptance is not rewritten.

## Merge boundary

Blockers are authority/data leakage, historical substitution/loss, conflicting
authoritative contracts, required path/build/CI failure, or violation of collection,
count/snapshot/identity/read invariants. Full pending/retry monitoring, current
unrecovered projections, live health, timeouts, notifications, IAM and whole-site
redesign remain follow-ups. Keep Draft/open; do not Ready, merge or deploy.
