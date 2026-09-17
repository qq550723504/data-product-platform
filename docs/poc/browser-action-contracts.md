# Browser action contracts and Release readiness hardening

Follow-up: #85, #75, #13. This increment preserves the ProductRelease and trace
features from #81/#83. It does not implement a new domain or a production IAM layer.

## Fixed boundary mismatch

A READY summary is insufficient when `checks` is empty/partial, when an additional
Core gate is not PASS, or when `blockers` is absent/malformed/nonempty. The UI and
Server Action now share `releaseReadinessProblem`. All eight named gates must be
explicit own properties with value PASS, all additional gates must also pass,
and the READY response must contain a valid empty blocker list. Missing is shown
as UNKNOWN, not as PASS. The success banner uses the same condition as the button.

Core remains authoritative. This guard does not grant permissions or prove that
the underlying source data satisfy any business rule. The previous web preflight
gap was not a demonstrated bypass of Core's publish validation.

## Test levels

| Layer | What it establishes | What it does not establish |
| --- | --- | --- |
| 34 new Node regressions | Runtime response validation and no publish POST for contradictory/incomplete readiness | Browser hydration or actual Core behavior |
| 7 fixture-server contracts | Deterministic test fixture controls and HTTP request observation | Real database state, audit or evidence |
| 12 Playwright Chromium cases | Built Next UI, populated review/release forms, real Server Actions, HTTP request headers and refresh/error handling | Real Go/PostgreSQL/Hop end-to-end acceptance |

The browser suite covers readonly runtime flags, whitespace/mandatory reasons,
confirm/reject, eight displayed gates, successful publish, empty/missing/null
checks, remaining blockers, additional failing gates, stale readiness after page
render, and review/publish conflicts. A refresh-required state disables resubmission.
The fixture records outgoing commands; these records are NOT Core AuditEvent/Evidence.

The former curl smoke (build with no workspace, then request SetupRequired routes)
is still useful for routing, but is not a substitute for these browser interactions.
The new CI job is `web-browser-contracts`; adding a job does not automatically make
it a required branch-protection check.

## Run locally

Use a disposable checkout with Node 22. From repository root:

```sh
npm --prefix apps/web install --no-audit --no-fund
npm --prefix tests/browser install --no-audit --no-fund
npm --prefix apps/web run build
node --test apps/web/tests/release-readiness.test.mjs
npm --prefix tests/browser run test:fixture
cd tests/browser
npx playwright install --with-deps chromium
npm test
```

Playwright starts its own Core fixture at 127.0.0.1:4400 and two built Next servers
at 127.0.0.1:3100 (enabled) and :3101 (readonly). It never reuses a pre-existing
server or accepts an external target URL. Each test resets synthetic state; one
worker and zero retries keep state isolation and failures explicit. No real data,
credentials, database, or remote mutations are used. Do not deploy the fixture.

CI builds with an empty workspace then supplies the synthetic identity at runtime;
this also exercises build/runtime configuration rather than only development mode.
Reports and failure traces are retained as workflow artifacts. The test-only
Playwright dependency is isolated from the application's production dependencies.

## Remaining live acceptance

A separate disposable live stack must still prove browser review/publish/trace
against Go and PostgreSQL, then verify persisted reviewer identity, audit, evidence,
immutable versions and the full reference POC. Do not treat this fixture suite as
that evidence or close full live POC acceptance solely because it passes.

References: https://playwright.dev/docs/test-webserver and
https://playwright.dev/docs/ci. Playwright is pinned to 1.63.0.
