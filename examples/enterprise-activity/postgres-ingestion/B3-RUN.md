# #298/B3 actual browser execution — 2026-10-11

Edge operated source detail → persisted batch/three RAW receipt → existing entity
review → existing Native processing → quality/compliance → prepare actual
certification evidence → certify → explicit supersede → Direct Data download.
It used the full Core API, real PostgreSQL/Hop, Redis worker and bounded object
storage. No database status UPDATE, fake provider result or fixture-created
success substitutes for these business operations.

The [independent Core oracle](results/2026-10-11/b3-report.json) has **31 PASS
checks**. PGI-08 passed. PGI-09 metadata-unavailable presentation passed; human
explanation is **NOT_RUN at the user's explicit request**. Healthy OpenMetadata
binding, stale-after-confirmed-binding and live reconnect remain NOT_RUN. The
outage uses the existing OpenMetadata adapter pointing at an unavailable
loopback endpoint; no positive provider response is claimed.

| Actual browser operation | Observed result |
|---|---|
| Register/start/recover/admit | Batch `fad72553-e862-4d64-9370-d30062629790` ACCEPTED, three RAW receipt, 6/10/14 rows |
| Source detail → batch → processing | Database/schema/table, reader, original schema, Core owner/classification, hashes and same business IDs visible |
| Existing entity review | Explicit reason entered in Edge; job `5e7981d0-96ce-4cca-b3b9-3f04fcea8bf0` SUCCEEDED, immutable STANDARDIZED output |
| Existing Native processing | One execution `915bcb0b-77a1-48b7-bcf9-23ad2f6ad7f5`, five output rows; repeat command recovered the same execution |
| Quarantine | NEGATIVE_ENERGY_KWH, key `M-005@2025-02-28T23:59:00+08:00`, original `-999` visible and retained |
| Quality / compliance | Actual PASS facts on CURATED; refresh recovered original attempts |
| Prepare certification in Edge | Existing rights computation plus FINALIZED four-item EvidenceSnapshot `e1ad815b-c842-4d04-b171-9957da3e105c`; no CLI UUID file required |
| Certify / supersede | New CERTIFIED `54888ac6-16a0-4e65-8756-17e9906c2eaf`; operator explicitly selected old/new facts and reason; old CERTIFIED retained with appended SUPERSEDED |
| Actual download | 640 bytes, SHA-256 `5c47a2c146dd09f42eec989562632856a07e585666f7b557f7fb3a7794fdba88`, equal to Core content hash |
| Revoke with stale ALLOWED page | Existing RevokeAuthorization Command executed; subsequent browser download BLOCKED; Certification ALLOWED, Entitlement BLOCKED |
| Direct-data recovery | Original ISSUED restored after refresh; same key never resends CSV; explicit retry uses new key and retryOf |
| Quality failure → certification | RAW FAIL yielded REJECTED and no CERTIFIED. Other missing prerequisites also appear; this is not a quality-only isolated browser case |
| Metadata unavailable | UNKNOWN + METADATA_PROVIDER_UNAVAILABLE; Core source and quality/certification/history remain accessible |

Final-build screenshots: [source/batch](results/2026-10-11/b3/source-batch-v5.jpg),
[quarantine](results/2026-10-11/b3/quarantine-v5.jpg),
[quality rejected](results/2026-10-11/b3/quality-rejected-v5.jpg),
[metadata unavailable](results/2026-10-11/b3/metadata-unavailable-v5.jpg),
[certification superseded](results/2026-10-11/b3/certification-superseded-v5.jpg),
[revoked delivery](results/2026-10-11/b3/revoked-delivery-v5.jpg).

Two real user-path gaps were fixed. Certification preparation lacked a page
Command; it now composes existing rights/evidence operations. An unconditional
delivery query also broke every version view when current certifications were
ambiguous. History is now independently readable, and explicit supersede
resolves ambiguity without choosing by latest timestamp. Initial failed
evaluations remain historical REJECTED facts and were never edited into success.

## Reuse and verification boundary

Core owns Command semantics and immutable references. This reuses existing
Workflow/Entity/Quality/Compliance/Certification/Delivery services, PostgreSQL
transaction/session advisory lock, command idempotency, delivery fence,
EvidenceSnapshot, Audit, CostAllocation and Outbox. Hop/JDBC and the existing
MinIO adapter own extraction/storage. No parallel queue, workflow runtime,
catalog, policy engine, migration or fork was added. Temporal or another
ingestion runtime would duplicate capabilities already exercised here; new OSS
adoption is unnecessary for this slice.

Preparation requires an explicit single-purpose/single-consumer finalized
profile, one published matching contract and one actual compliance result;
ambiguous prerequisites fail closed. Six concurrent same-key replays recovered
the original snapshot or reported lock contention; subsequent same-key recovery
returned the original snapshot with no added costs, snapshots, decisions, events
or audits. New read endpoints reject forged actors and foreign workspaces.
Actual evaluations/dispositions have allocated costs. Business/audit/event facts
and rendered pages contain none of the known trial credential secrets.

The fixed synthetic text/NOT NULL source contract remains the B1/B2 contract;
arbitrary PostgreSQL types and concurrent-source snapshots are unaccepted. The
loopback authenticated-operator gateway is a browser fixture, not production
IAM. Existing POC surfaces are not claimed as a production security perimeter.
The existing MinIO trial mitigation/provider-admission limits still apply.

## Inspect or reproduce

Retained final page: `http://127.0.0.1:61762/batches`. Ports/private config paths
are local run details. Credentials stay in a generated private `dpp-pgi-b3-*`
directory, never browser forms or artifacts. Scoped storage credentials expire;
after expiry use a fresh trial rather than treating health as working storage.

From the repository root, build and start a new isolated trial:

```powershell
Push-Location apps/platform
go build -o ../../.data/pgi-b3-api.exe ./cmd/api
go build -o ../../.data/pgi-b3-worker.exe ./cmd/worker
go build -o ../../.data/pgi-b3-fixture.exe ./cmd/ingestion-browser-fixture
Pop-Location
python examples/enterprise-activity/postgres-ingestion/b3_runtime.py --output .data/pgi-b3-fresh
```

Open the printed URL in Edge. Register the three sources, then prepare/start/
recover/accept using page controls. In another terminal, prepare synthetic
rights/profile/published contract prerequisites through existing Commands:

```powershell
$trialState = Get-Content .data/pgi-b3-fresh/runtime.json -Raw | ConvertFrom-Json
$env:TEST_INGESTION_B3_CONFIG = Join-Path $trialState.private 'test.json'
.data/pgi-b3-fixture.exe rights
```

Then operate entity review, processing, quarantine, quality/compliance,
per-version evidence preparation and certification through the pages. The
retired CLI per-version evidence shortcut is absent. A RAW quality check using
the existing CURATED rules exercises rejection. Download the CURATED output,
refresh recovery, create an explicit retry, revoke its fixture Authorization via
the existing Rights API and click the stale download page. Metadata refresh
uses the unavailable real adapter and must remain UNKNOWN.

The recorded oracle includes two issued downloads, two revoked retries and one
explicit supersede. Reproducing it requires the same operations, including a
same-scope replacement fixture authorization (`renew-rights`) followed by explicitly reusing frozen
evidence, a new certification request and explicit supersede, then another revoked retry.
Pass the copied actual browser download to the independent verifier:

```powershell
python examples/enterprise-activity/postgres-ingestion/b3_verify.py --runtime .data/pgi-b3-run5/final-v5/runtime.json --download .data/pgi-b3-run5/download-v5.csv --output .data/pgi-b3-recheck.json
```

`b3_recompose.py` starts new API/web processes against retained providers and
owns only those new processes. Stop its processes before writing the original
runtime's `STOP` file, which cleans only its isolated providers. Never remove
other projects' containers/volumes or use project-wide `down -v`.

Validation passed: `go test ./...`, `go vet ./...`, complete Next build (204 web
command tests + lint/typecheck/bundle), separate restored-config typecheck/lint,
Python compile and the live PostgreSQL/HTTP oracle. Ordinary Go runs can skip
TEST_DSN suites; the 31 live checks are actual PostgreSQL evidence. Remote CI,
merge, production deployment, healthy OpenMetadata and human explanation have
not been substituted by local checks.
