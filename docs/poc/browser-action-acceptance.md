# POC Browser Action Acceptance

This suite validates the operator-facing write boundary added for the Enterprise Activity POC. It exercises a real production Next.js build in Chromium and sends the resulting Server Actions to a deterministic loopback Core HTTP fixture.

## What the suite proves

The browser suite covers the path:

```text
Chromium
  -> rendered Next.js UI
  -> hydrated form
  -> Next Server Action
  -> server-side scope/readiness validation
  -> loopback Core HTTP contract
  -> post-submit refresh / conflict handling
```

It verifies:

- Entity Review requires a non-empty reviewer reason before confirm/reject is enabled.
- Confirm and reject use the existing guarded Next Server Action and a server-derived reviewer UUID.
- Read-only deployments render provenance without exposing write controls.
- ProductRelease publishing is enabled only when the shared fail-closed Release Readiness validator accepts the complete Core contract.
- Contradictory readiness (`overall=READY` but a required gate fails) cannot publish.
- A future/additional non-PASS gate cannot be ignored by an older web build.
- A Core `409` is surfaced to the operator and is not automatically retried.
- The publish request carries the server-derived actor and an idempotency key.

## Release Readiness contract

Both the browser button state and the server publish preflight call the same runtime validator. A release is publishable only when all conditions below are true:

1. `ProductRelease.status == READY`;
2. `ReadinessResult.overall == READY`;
3. `blockers` is a valid empty string array;
4. every canonical gate is present and exactly `PASS`:
   - production
   - dataset
   - rights
   - quality
   - compliance
   - contract
   - evidence
   - delivery
5. any future/additional Core gate is also `PASS`.

Missing, malformed, contradictory, or unknown non-PASS state fails closed.

## Fixture topology

The Playwright configuration starts three loopback processes:

```text
127.0.0.1:18080  deterministic Core HTTP fixture
127.0.0.1:3001   production Next app with POC writes enabled
127.0.0.1:3002   production Next app with POC writes disabled
```

The fixture uses only synthetic UUIDs and records. It exposes test-only reset/state endpoints under `/__test/*`; these endpoints are part of the test fixture process and are not application routes.

## Running locally

From `apps/web` after a production build:

```bash
npm install --no-audit --no-fund
npm run build
npm install --no-save --no-package-lock --no-audit --no-fund @playwright/test@1.55.0
npx playwright install chromium
npm run test:browser
```

For Linux environments missing Chromium system libraries, use:

```bash
npx playwright install --with-deps chromium
```

## CI evidence

The `web-console` CI job runs the browser suite after lint, typecheck, command/scope regression tests, production build, and route smoke. Playwright HTML reports plus retained failure traces/videos are uploaded as the `poc-browser-<run-id>` artifact for 14 days.

## Acceptance boundary

This is intentionally an isolated HTTP-contract/browser acceptance layer. It does **not** prove:

- a real Go API + PostgreSQL deployment accepted and persisted the browser commands;
- real Hop or Splink processes executed during the browser test;
- Evidence/Audit rows were persisted by the fixture;
- production IAM, session identity, network authorization, or tenant isolation.

Those concerns remain covered by their respective Go integration tests, Hop/Splink adapter/reference tests, deployment checks, and future IAM work. The loopback suite must not be described as a production end-to-end proof.
