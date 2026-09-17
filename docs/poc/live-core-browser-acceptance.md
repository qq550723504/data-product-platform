# Live Core browser acceptance

This increment is additive to the prior readiness/browser-contract work (#88).
It does not replace the mock-transport suite, change Core domain behavior, or
claim every POC capability has passed. No production deployment or IAM changes.

## What runs

`TestBrowserLiveCorePOC` prepares the repository's synthetic enterprise/lease/energy
inputs using the actual Core application commands, PostgreSQL and MinIO. It starts
separately compiled `cmd/api` and `cmd/worker` binaries. It does not register a fake
Core handler and never substitutes an in-memory object store or a mock queue.

The coordinator drives three separately launched Chromium phases against the
built Next **standalone server**, including its static assets:

1. **Review:** enter the required reason and confirm each actual pending candidate.
   The Go coordinator independently reads PostgreSQL to assert reviewer identity,
   reason, mapping, review AuditEvent and integrity-checked Evidence. The real API
   also receives a blank-reason negative control, which must produce no review audit.
2. **Publish:** prepare production using the real Redis queue and native Worker,
   run actual quality/compliance/rights/contract commands, and create one ready and
   one blocked release. The browser must keep the blocked button disabled and
   publish the ready release through its real Server Action, then navigate to trace.
3. **History:** a new browser context and standalone process discover the release
   through Evidence Center and read its exact persisted snapshot ID/root hash.
   It must display the original review reason and keep the published release locked.

The independent database checks require a PUBLISHED release, exactly one matching
publish audit and ProductReleased outbox event, a nonempty integrity-valid evidence
snapshot, complete exact-version lineage, execution/workflow links, and evidence
and cost facts. Every trace DatasetVersion's real MinIO bytes are SHA-256 checked
against PostgreSQL. Frozen ProductVersion/DatasetVersion rows are compared before
and after publish; published Release rows are compared before and after history
reads and a rejected repeat publish. The repeat uses a new key and tests state
rejection, not replay of the original browser-generated idempotency key.

The Go test fails if no pending candidate is produced, if the dedicated database
is not fresh, if any browser phase fails, or if the actual Worker fails/times out.
It does not silently skip a live failure once explicitly enabled.

## Run in a disposable Linux checkout

Requires Node 22, Go 1.24, Docker with Compose v2, and free loopback ports 15432,
16379, 19000, 18080 and 13100. The root layout must support runtime workspace
configuration (provided by #88 or equivalent).

```sh
npm --prefix apps/web install --no-audit --no-fund
npm --prefix tests/live-browser install --no-audit --no-fund
npm --prefix apps/web run build
npm --prefix tests/live-browser run prepare:standalone
(cd tests/live-browser && npx playwright install --with-deps chromium)
LIVE_BROWSER_ACCEPTANCE=1 bash tests/live-browser/run.sh
```

The runner sets dedicated service targets, starts a uniquely named Compose
project, and removes only that project's containers/volumes on exit. PostgreSQL
and MinIO data are ephemeral. It never runs TRUNCATE or deletes another project's
business records. The Go coordinator additionally rejects any database, Redis,
bucket, endpoint or environment not matching the dedicated local test settings.
The original deployment Compose configuration is unchanged.

MinIO uses a historical explicit test image, not a recommended production version;
PostgreSQL/Redis use the same major baselines as the project. CI records resolved
image digests and the exact source commit, since tags and unlocked transitive npm
dependencies can change. This is not a dependency/security audit.

## Evidence and limitations

The `live-core-browser` workflow preserves phase HTML reports, screenshots,
Playwright traces, API/Worker/service logs, the database-derived `persisted-trace.json`
and `verification.json`. The latter is marked verified only after every database
and browser assertion passes. Report conclusions must cite the actual workflow
run and tested SHA, not the existence of these test files. Adding a job does not
automatically make it a required branch-protection check.

This is an integration acceptance slice with real persistence and real native
processing, using synthetic inputs. It is **not**:

- an all-UI CSV upload / workflow-authoring / rights-approval onboarding test;
  preparation still invokes actual Core application commands from the coordinator;
- proof of authentication/authorization: the POC server supplies configured actor
  UUIDs, not identities validated by an identity provider;
- Apache Hop, Splink, OpenMetadata, external publication or NDI runtime acceptance;
- a concurrency/load test, disaster-recovery test or commercial deployment sign-off.

Next standalone packaging follows https://nextjs.org/docs/app/api-reference/config/next-config-js/output.
Browser lifecycle follows https://playwright.dev/docs/test-webserver.
