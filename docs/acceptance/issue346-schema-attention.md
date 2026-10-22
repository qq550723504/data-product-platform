# #346：源端 schema 漂移只读汇总与精确观察导航

## Contract and ownership

Issue #346 is the finite scope: reuse `/` and `/attention` to read Core-owned
DRIFT observations, then navigate to the exact resource/request/observation.
Baseline: main `b902f1c635c791f80dcbc2db50f31edbdff342d4` (#344).
AGENTS, the metadata/source-schema contracts and #299/C2 own the semantics.
This document records this slice; it does not expand historical acceptance.

Before writing, active threads, open PRs and local source write sets were checked.
#325/#345 retain their trial controller owner; #307/#332 retain their lifecycle
owner. Their current changes do not overlap these Web/metadata query paths.
Implementation uses `codex/issue346-schema-attention` in its own worktree.
No trial files, lifecycle Commands, migrations, shared bootstrap, dependencies,
CI workflows, deployment or retained environments are changed. The package.json
change only adds the new pure navigation test to the existing build gate.

## Reuse and read boundary

Reuse `source_schema_request/attempt/observation/handling`, the metadata HTTP
authorizer (server credential/actor and workspace allowlist), existing tables,
pagination, quality attention identity/offset helpers, and source-column
comparison presentation. OpenMetadata/native Connector still owns discovery.
No new OSS, scheduler, queue, task/state model or business table is introduced.

New GETs:

- `/api/v1/workspaces/{workspaceId}/source-schema-observations/attention`
  (`view=UNHANDLED|ALL`, default 25/max 100, `offset`). Counts and rows use one
  PostgreSQL statement/snapshot and stable observation recordedAt/id ordering.
- `/api/v1/data-resources/{resourceId}/schema-observations/{observationId}`
  (`workspaceId`). Exact history lookup is independent of the old LIMIT 50.

One observation is counted once regardless of handling record count. Unhandled
means no handling fact; recorded handling is not closure or recovery. A newer
MATCH or rule version never removes old DRIFT facts. Resource name/code are
current display labels; responsibility, source identity, rules/hashes, baseline,
columns and handling belong to the exact frozen history. No current binding is
joined to substitute historical content.

`/resources/{id}?view=source-schema&requestId=...&observationId=...` resolves before
the normal resource/provider surface. All four scope identities are checked;
missing/mismatched facts fail explicitly, without latest fallback. The exact
mode has no forms, metadata probes, scanner Plan, source/batch enumeration or
Command calls. New links disable prefetch. Credentials remain server-side.
Failures render local UNKNOWN/error; empty success only describes recorded
observations. Quality, certification and current delivery gates are unchanged.

## Local validation

- PASS: real PostgreSQL query/HTTP integration with 105 DRIFT observations and
  a newer MATCH/rule, 3 handling records on one observation, complete pagination,
  capped-history exclusion plus exact retrieval, binding rotation, workspace and
  identity negatives, credential/header-spoof negatives, invalid pagination,
  disabled scanner, immutable history fingerprints. Queries also execute on a
  connection with `default_transaction_read_only=on`; a Plan spy panics if called.
- PASS: six related Go packages (metadata HTTP/infrastructure/application/domain,
  rights HTTP and principal) against this slice's PostgreSQL; metadata `go vet`,
  API/worker builds.
- PASS: Web pure navigation/scope tests (4), typecheck, lint and production build.
- PASS: all 52 browser contracts and 11 fixture unit tests. Browser validation
  covers the 105-row list, precise old evidence,
  reload, no detail fanout/Command/provider endpoints, no forms/secrets, local
  failure/foreign response, request mismatch and missing identities. These are
  isolated fixture-backed UI tests, distinct from real PostgreSQL HTTP tests.

An initial wider Windows metadata test run exposed the unchanged
`TestNativeSchemaCostsStartupBeforeMarker/exit`: its 500ms context expires before
the helper invocation receipt is confirmed (`costs=0`, invocation unknown).
The same test fails on the unchanged #344 source at `87f9097c` (same tree as this
baseline). No scanner, deadline or cost gate is changed to make this slice pass.
The broad Windows run is FAIL; do not describe it as a full local Go-suite PASS.
Exact candidate Linux PR CI remains a separate result.

The fixture initially used a shared replacement FQN and collided when the test
was repeated. It now uses a per-resource UUID replacement FQN; repeated tests
are independent without deleting historical fixtures.

Test PostgreSQL: new named volume/container labelled `codex.issue=346`, only
loopback access, independent credentials in ignored `.data/q346`. Existing
#325/#307/#343 environments, data, identities, scans, budgets and backups are
untouched. No full native-source or human explanation acceptance is claimed.

## Merge boundary

Blockers: authorization/data leakage, evidence misbinding/loss, conflicting
authoritative contracts, required path/build/CI failure, incorrect counts or
snapshot semantics. New health types, recovery lifecycle, personnel/IAM,
notifications and whole-source monitoring remain follow-ups. This delivery is
a Draft PR; Ready, merge, deployment and retained-runtime execution are outside
this authorization.
