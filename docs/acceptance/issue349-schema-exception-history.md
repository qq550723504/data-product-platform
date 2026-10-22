# #349: source schema exception history and exact evidence

## Finite contract and ownership

Issue #349 owns this slice. Baseline main is
`5edef9f7f3a9436b3ec75585685f6b914cdac2a0` (#347). AGENTS, the existing
metadata/source-schema contracts and #299/C2 remain authoritative.

Reuse `/` and `/attention` to read Core-owned UNAVAILABLE / DENIED /
INTERRUPTED observations, then open the exact resource/request/observation.
These are historical observations, never current unresolved task counts.
Pending requests/attempts without an observation and live source health are
outside this slice. A later MATCH does not hide or close an earlier exception.
DENIED can have no attempt or an already allocated attempt; it does not prove
zero provider invocations or permanent loss of resource access.

Before writing, main, active threads, worktrees and open PRs #295/#332/#348
were checked. This thread owns `codex/issue349-schema-exception-history` in
the short Windows worktree `e349`. #325 retains its trial owner; #307 retains
its lifecycle owner; controlled source/annotation admission retains its owner.
Their environments, data and worktrees are not used. No shared bootstrap,
routing, worker, migrations, dependencies, lockfiles, CI workflows or deployment
files change. The Web package script only adds the new pure schema tests.

## Reuse and read boundary

- Reuse source_schema_request/attempt/observation, metadata HTTP authorization,
  the exact observation API and history page from #347, server-only credentials,
  existing table/pagination/Badge and column-comparison presentation.
- Existing OpenMetadata/native Connector continues to own scanning. No new OSS,
  scheduler, queue, task, state machine or business table is introduced.
- New GET: `/api/v1/workspaces/{workspaceId}/source-schema-observations/exceptions`
  with `status=ALL|UNAVAILABLE|DENIED|INTERRUPTED`, default ALL, default limit 25,
  maximum 100, and nonnegative offset. One SQL statement/snapshot computes all
  three classification counts, filtered total and page; order is recordedAt/id
  descending. Multiple observations for one request remain separate rows.
- The exception DTO has its own status/nullable-field contract. DRIFT-only
  validation and handling semantics remain unchanged. Historical counts are
  separate from the existing current queue totals.
- Reuse the existing exact GET and resource history mode. Scope checks bind
  workspace/resource/request/observation; attempt comes from that observation.
  Source identity, rule/hash, responsibility and baseline use frozen facts,
  independent of the current binding, rule or scanner configuration. No latest
  fallback or LIMIT 50 dependency is introduced.
- Exception links carry their status/offset for a bounded local return link.
  Existing queue links preserve the independent exception selection. There is
  no caller-controlled arbitrary return URL. Prefetch is disabled on new links.
- No current provider/Plan/source/batch/scan or Command call is made. Exact mode
  has no forms. Null scan times and inconclusive columns show UNKNOWN; empty
  reads describe only the absence of recorded exceptions. Failures show a local
  UNKNOWN/error rather than zero/CLEAR. Rights, quality, certification and current
  delivery semantics are unchanged.

## Validation

- PASS: own PostgreSQL 16, all repository migrations applied to a new database.
  Query/HTTP integration seeds 105 requests and 106 exceptions (35 unavailable,
  35 denied, 36 interrupted), plus later same-request MATCH and a separate DRIFT
  with three handling records. Complete pagination, all filters, the recent-50
  exclusion, exact old evidence, frozen identity under binding/rule changes,
  nullable DENIED attempts/times, actual attempt binding, and unchanged DRIFT
  counts are checked. Foreign workspace/resource/observation, missing/wrong
  credentials, spoofable actor headers, malformed filters/pagination and read
  failures reject explicitly. The GET connection is transaction-read-only;
  scanner Plan panics if invoked; history fingerprints remain unchanged.
- PASS: six related Go packages (metadata HTTP/infrastructure/domain/application,
  rights HTTP and principal) with TEST_POSTGRES_DSN pointing only to this test
  database. Metadata query/HTTP go vet and API/worker builds pass. No broad native
  scanner/helper or provider acceptance is claimed.
- PASS: Web schema navigation/scope tests 8/8, typecheck, lint, production build
  and the existing unit suites invoked by that build.
- PASS: fixture contracts 12/12 and browser contracts 56/56. New cases cover
  >100 pagination without detail fanout, exact old nullable evidence, UNKNOWN
  column/time rendering, return filter/page, separate current queue counts,
  foreign/unavailable/empty reads and request mismatch. Screenshots of list and
  exact evidence were inspected. Browser fixture evidence is distinct from
  real PostgreSQL HTTP evidence.

The final responsibility/attempt binding checks passed on real PostgreSQL. After
the evidence wording and fixture attempt IDs were finalized, the production Web
build and all four new browser cases passed again.

Local test resources belong only to `codex.issue=349`, `codex.worktree=e349`.
The new PostgreSQL container/volume is `dpp-q349-pg-e349-20261022`, loopback-only;
test configuration is ignored under `.data/q349`. No existing PostgreSQL,
source, provider, worker, task, budget, backup or protected content is used.
Browser test processes are owned by this worktree and its fixture runner.
After validation, the labelled PostgreSQL test container is stopped with its
volume retained; the fixture/Web test servers have exited. No other resources
were stopped, removed or cleaned up.

Source/PR CI, merge-SHA main CI, deployment, retained-runtime evidence and human
acceptance are separate facts. At this source checkpoint PR CI is pending;
merge, deployment, native-source/retained-runtime and human explanation
acceptance are NOT_RUN. Existing #298/#299 acceptance is not rewritten.

## Merge boundary

Blockers are authorization/data leakage, historical evidence substitution/loss,
conflicting authoritative contracts, required path/build/CI failure, or violation
of the declared count/snapshot/status semantics. Current unresolved projections,
recovery workflows, live health, new connectors, personnel/IAM, notifications and
whole-workbench redesign remain follow-ups. Deliver a Draft PR; do not Ready,
merge, deploy or operate existing environments under this authorization.
