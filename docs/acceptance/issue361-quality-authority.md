# Issue #361: Quality evidence authority

## Finite contract

The authoritative scope is [Issue #361](https://github.com/qq550723504/data-product-platform/issues/361), together with the existing Quality/Gold Command and immutable attempt contracts. This slice adds no domain state, migration, engine, identity provider, governance count or delivery authority.

All eight routes in `quality/transport/http/handler.go` require the same existing `principal.Resolver` / `HUMAN_DECISION` capability used by remediation:

| Method | Path | Workspace source |
| --- | --- | --- |
| GET | `/api/v1/quality-results/{resultId}` | required query `workspaceId` |
| GET | `/api/v1/quality-assessments/{assessmentId}` | required query `workspaceId` |
| GET | `/api/v1/quality-assessments/{assessmentId}/report` | required query `workspaceId` |
| GET | `/api/v1/quality-assessment-attempts/{attemptId}` | required query `workspaceId` |
| GET | `/api/v1/dataset-versions/{versionId}/quality-assessments` | required query `workspaceId` |
| GET | `/api/v1/dataset-versions/{versionId}/quality-assessments/latest` | required query `workspaceId` |
| POST | `/api/v1/dataset-versions/{versionId}/quality-checks` | required body `workspaceId` |
| POST | `/api/v1/dataset-versions/{versionId}/gold-quality-checks` | required body `workspaceId` |

The composition root injects the existing configured human-decision resolver. Disabled/missing credentials fail closed (401); an authenticated principal without the required workspace/capability fails closed (403). Authorized scope with an unrelated object/version returns 404. Principal errors never expose their private details. The POST actor comes exclusively from the resolver; `X-Actor-ID` has no authority. This is the existing controlled service-principal model, not terminal-user SSO/RBAC.

Authorization precedes repositories and engine calls. Assessment and attempt lookups additionally join the actual DatasetVersion/Dataset workspace. Evidence, Audit and Cost queries use that authorized workspace; attaching another workspace's row to the same object ID cannot expose it. GETs and denied commands add no business facts. Existing internal Evidence/Cost query consumers retain their existing methods; HTTP uses the explicit scoped methods.

Web Quality reads/commands use the existing server-side `HUMAN_DECISION_API_TOKEN`. Core uses existing `HUMAN_DECISION_API_ENABLED`, token, actor and workspace settings. This change does not configure or enable any retained environment. Credentials are never supplied to unrelated generic reads, sent as query parameters, forwarded through redirects or rendered into errors/HTML. Missing credentials do not trigger anonymous requests. The existing Quality action switch still defaults off.

Exact assessment/attempt IDs, report pagination and workbench/case/recheck navigation remain stable after later assessments arrive. Explicit selection never falls back to latest or a sibling. History, latest summary, displayed report and attempt reads each retain their own known/UNKNOWN state. A successful exact historical report remains visible when the list, latest summary or attempt fails. Explicit selection requests only that report, without fetching an unrelated latest report. A failed selected report stays UNKNOWN while known latest/history facts remain visible; it is never replaced with another report. Unavailable attempts retain their exact ID and do not offer a repeat Command. Quality-dependent certification preparation requires a known latest summary; a failure of an independent report does not erase that summary. Provenance evidence failures are also UNKNOWN, with known certification references retained. Existing independent workbench/attention sections and counts are unchanged.

## Reuse and ownership

Reuse: existing resolver/capability, Quality Commands, attempt ledger, scoped Dataset checks, Evidence/Cost repositories, report/case pages and test infrastructure. No second auth system or general workflow runtime is needed.

Worktree: `codex/issue361-quality-trust`, based on main `9ded9892b05341102d0e43bdbdc82ff46a7ca87d`. At write-domain checks, open PR heads #358 `5c376100`, #332 `f4f27c78`, #295 `9328b7cd` had no intersection with the actual changed paths, including the necessary Evidence/Cost scoped helpers and existing Gold/pilot test adaptations. #325/#307 and the separate demo preview retain their existing owners and resources.

## Regression evidence

- HTTP denial matrix: all eight routes × nil/disabled resolver, absent/invalid credential, foreign workspace and missing capability; nil repository/engine dependencies prove denial happens first. Forged/invalid/absent actor headers cannot change the trusted actor.
- PostgreSQL report regression: all six GETs in matching and non-matching authorized scopes; foreign Evidence/Audit attached to the same assessment is excluded. Quality/Attempt/Cost snapshots stay unchanged after reads and both rejected POSTs.
- Standard and Gold integration fixtures exercise the real HTTP POSTs, including forged actor headers. Standard same-attempt replay under another authorized principal returns the original immutable assessment/actor and retains one physical cost. Existing Gold replay and physical-attempt regressions remain in place.
- Web unit contracts cover exact historical URLs, single authenticated reads, 401/403/404/503 without anonymous/latest fallback or upstream body exposure, missing command credentials, and credential scope limited to Quality calls.
- Browser contracts cover original/recheck evidence, later assessment arrival, local denied-evidence UNKNOWN, preserved version identity, existing independent workbench sections and held GET isolation. The fixture requires the authenticated credential on Quality routes; HTML does not contain it.
- PR #362 partial-failure regression executes the actual TSX page with isolated data dependencies, including history/latest-summary/attempt failure with an exact report success, foreign attempts, and exact-report 401/403/404/503 with no latest substitution. Browser fault injection independently targets history, latest summary, exact report, latest report and attempt; it covers partial failure, malformed/denied/not-found report responses, preserved known facts and recovery on the same URL.
- A CI trace showed a correct CLOSED server response while the browser retained FOLLOW_UP; disabling prefetch alone did not resolve the rapid-transition regression. Quality entry/view/pagination now use native document navigation with the existing exact URL builders, preventing unfinished client route transitions from retaining another view. The regression checks repeated CLOSED/ALL changes, exact URL, active view and row counts, followed by the precise recheck output link, without speculative Quality view reads, test retries or longer timeouts. Dataset/case/evidence links retain their existing behavior.

Local validation: targeted Go tests/vet, Web Quality unit tests (31), typecheck/lint/build and browser regression (103 tests). PostgreSQL tests require an isolated `TEST_POSTGRES_DSN`; no retained database is used. The PR CI's disposable PostgreSQL/Redis services execute integration tests separately. Local Windows full `go test ./...` exposed the unchanged `metadata/openmetadata.TestNativeSchemaCostsStartupBeforeMarker/exit` helper deadline failure; it is outside this write domain and is not represented as a full-suite pass.

## Review boundary

Blockers are authorization or credential leakage, incorrect workspace/evidence binding, historical/actor/idempotency corruption, broken mandatory navigation/UNKNOWN behavior, conflicting contracts or failing related checks. Whole-site IAM migration, new states, providers, automatic retry, source admission and retained-runtime/production acceptance are out of scope. This remains a Draft delivery: no merge, deployment or runtime change is authorized.
