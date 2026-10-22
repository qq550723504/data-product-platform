# Real Fork to Host RAW: bounded synthetic acceptance

## Finite contract

Goal: exercise the merged #79/#80 producer and #348/#353 Host consumer with
real Fork HTTP/Token/RQ, PostgreSQL and native MinIO, using synthetic data only.
This is an isolated local trial, not deployment or production acceptance.

Authoritative contracts: `fork-approved-source-admission.md`,
`apps/platform/internal/sourceadmission`, migration 000072, and the Fork's
`docs/authorization/approved-dataset-export.md` / `release-manifest-v1.md`.
Source identity is namespace + Submission ID (kind is restricted to submission).
Connection/workspace are authorization bindings, not conflict escape hatches.

Reuse: production `Build`, `Handler`, `HTTPSource`, `ProtectedObjects` and the
existing minio-go adapter; native MinIO STS and prefix policy; existing rights,
transaction fences, invocation ledger, Next UI and controlled gateway contract.
There is no second ingestion service, storage implementation, queue or IAM.

Non-goals: Core Result/Review/Snapshot/Gold (#294 / Draft #295), media or CoNLL
admission, certification, publication, delivery, deletion, production IAM/JWT
adaptation and #307 lifecycle/disposal work. #76/#307 sessions, configuration,
containers, worktrees, volumes and credentials are excluded from the write set.
The composition uses retention-only routing and launches no Core worker/adapter.

Merge blockers are authorization/credential leakage, mutable frozen identity or
history, actual required-path incompatibility, lost/duplicated invocation costs,
or non-idempotent recovery. General infrastructure expansion is out of scope.

## Runtime and permission prerequisites

- Pin the Fork and Host commits and built image identity. The trial Fork image
  uses Python 3.11 and the dependency image ID checked by `prepare.mjs`; its JCS
  dependency is installed with version/hash from poetry.lock. The ordinary
  Dockerfile's Python 3.13 default is not this capability's runtime contract.
- The dependency image installs Label Studio editable at `/label-studio`.
  Overlay that same root, replace coverage-only PYTHONPATH, and assert the
  package import path. Do not mix current top-level Django apps with stale
  `label_studio.*` modules from a second source root.
- Create a NEW `hfraw_fork` database and stable random source namespace. The
  seed command refuses retained/non-hfraw databases and reseeding. It uses
  separate real author/reviewer identities and existing submission commands.
- Enable legacy Token only in the new instance/organization. Release creator,
  snapshot creator and Host fetch account are different project Managers.
  The credential is not a read-only scope. JWT-only production targets require
  a separately confirmed authentication adapter; there is no automatic fallback.
- Host requires server-derived INGEST_RAW and current READ/PROCESS rights.
  The operator profile fixes workspace/resource/dataset/connection and source
  endpoint. It and all tokens/STS credentials stay outside Git and screenshots.
  Fork API/RQ mount only their own fixture directory, read-only; Host API/STS
  and MinIO bootstrap credentials are not mounted into those containers.
- MinIO is a new bucket with expiring scoped STS credentials allowing only
  GetObject/PutObject in `core/<workspace>/`. Runtime cannot list/create buckets,
  write a foreign prefix or delete. Admin is used only by isolated bootstrap
  and explicit synthetic corruption probes, never by production Build.
- Host fetches fixed authenticated snapshot endpoints over loopback. A
  transparent recorder forwards actual Fork responses; a separate transparent
  native-storage proxy injects lost replies/read outages. Neither creates valid
  fake payloads or substitutes successful provider receipts.
- Browser automation supplies only a synthetic session cookie. The loopback
  gateway strips caller identity headers before injecting trusted headers.
  This controlled gateway is not production SSO/IAM acceptance.

Fork revocation stops new downloads, including accepted-command revalidation;
it does not recall a Host copy. Authorized original-key metadata lookup can
still report historical acceptance. Host rights invalidation denies both lookup
and command replay. Historical accepted facts are never rewritten as failure.

## Run (Windows local worktree)

Use a new private report directory and verified free ports 25510–25517. Inspect
Docker subnets first; `HFRAW_SUBNET` must be a new non-overlapping /28. Never
prune existing networks to obtain capacity. Set:

```powershell
$env:FORK_HOST_LIVE_SYNTHETIC='1'
$env:HFRAW_OUTPUT='<new absolute private report directory>'
$env:HFRAW_FORK_WORKTREE='<independent Fork worktree>'
$env:HFRAW_SUBNET='<verified unused /28>'
node tests/fork-host-live/prepare.mjs
$env:POSTGRES_DSN=Get-Content "$env:HFRAW_OUTPUT/postgres-dsn" -Raw
# From apps/platform:
go run ./cmd/migrate -direction up -dir ../../migrations
# From repository root:
node tests/fork-host-live/fork-bundles.mjs
node tests/fork-host-live/run.mjs
```

Before the browser run, build `apps/web` and install the existing
`tests/source-admission` Playwright dependency/browser. Run the regular source
PostgreSQL regressions against this same dedicated Host database; their fixture
workspaces are separate. Fork regressions use its own test database, export/data
volumes and Redis DB 1, separate from the real trial's RQ DB 0.

`integrity.mjs` is a final, destructive-to-synthetic-fixtures-only probe: use a
fresh private output directory/new Host profile (copy only the new trial's source
fixture/bundle/token/bootstrap files), keep the same source namespace, and set
`HFRAW_LIVE_BINARY` to the compiled fixture. It tests mid-flight revocation,
wrong expected Manifest hash, then corrupts one explicitly selected synthetic
Fork private artifact and confirms quarantine. Run it after all normal/browser
cases and never repair or regenerate the corrupt source under its old identity.

## Required evidence and closure

- 3 exact immutable artifacts; native number lexemes and UTF-8 bytes preserved;
  one READY RAW version per capture, no Gold/publish/delivery/Core facts.
- Current principal/workspace/dataset/connection and injected actor negatives;
  live fetch/release/snapshot creator revocation and organization Token-off.
- Two held, genuinely overlapping source Manifest responses for the same key;
  one capture/version; all actual external calls charged independently.
- Successful native PUT followed by lost reply/read outage, PREPARED key lookup,
  actual Host process restart and original-key recovery with no extra version.
- Genuine valid package whose original Manifest plus both native declarations
  exceeds 32 MiB: exactly one Manifest invocation, no artifact/storage/version
  allocation. Terminal replay and lookup add no invocation. B-1/B/B+1,
  underdeclaration and late-concurrent-outcome cases remain PostgreSQL tests.
- Native corruption is integrity failure, not transport UNKNOWN; source
  corruption quarantines; neither silently repairs accepted bytes/history.
- Compare independent source/storage transport records to invocation start,
  observation, CostEvent quantity 1, ACTUAL/unknown monetary amount and typed
  CostAllocation. Provisioning/probe traffic is separate from admission traffic.
- en-US/zh-CN real gateway: acceptance/replay/refresh, lost response/original key,
  stale access removal and genuine over-budget terminal recovery (8 journeys).
- Preserve red/green logs, raw source bytes, IDs/digests, screenshots and all
  PostgreSQL/MinIO data. Compare #76/#307 resource and worktree baselines.
  `stop.mjs` stops only exact labeled trial containers; never remove volumes.

The live fixture's normal Go-test skip is intentional: only explicit live runs
are evidence. Regular PostgreSQL results, live service results, browser evidence,
GitHub CI and deployment authorization must be reported separately.

## Native corruption classification correction

The real trial reproduced an accepted MinIO object's wrong-length body being
reported as SOURCE_TRANSPORT_UNKNOWN/503. `ReadProtected` had already proven
ErrStorageContract before the producer hash check. Map that native contract
failure to SOURCE_INTEGRITY_FAILED at the evidenceStore boundary; preserve
actual transport uncertainty. PostgreSQL red/green regression confirms no
accepted history rewrite, repair version or silent success. No lifecycle/#307
implementation is changed.

## Local receipt (2026-10-22)

Pinned bases: Fork `8abaa18f54b296bf7b960a3a1e40c579227806c5`, Host
`499c769441852afb3db4639f99a38259a215efaf`. Independent branches are both named
`codex/fork-host-raw-live` in their respective repositories.

Final single-source-root Python 3.11.17/rfc8785 0.1.4 image: real Fork HTTP/RQ,
dedicated PostgreSQL/Redis/private export volume, real native MinIO with scoped
STS, production Host Build and a trusted synthetic browser gateway.

- Fork PostgreSQL: 126 passed, no failures (clean image, no import-path workaround).
- Host PostgreSQL: 36 source tests + 5 routing tests passed. The two explicit
  browser/live harness tests skip in the ordinary suite and ran separately.
- Real-service main acceptance: 24/24 PASS, including 8 en-US/zh-CN browser
  journeys; additional mid-flight authorization/source-integrity cases: 4/4 PASS.
- Independent runtime transport/cost comparison: 61 source HTTP invocations and
  141 native storage invocations; quantities and typed allocations agree.
- Three captures/versions and nine protected-copy intents in the main trial.
  Frozen member IDs/hashes/bytes remain intact. Annotation Result/Review/Snapshot,
  certification, ProductRelease and DeliveryOperation counts are zero in scope.
- Actual successful PUT response loss, native read outage, Host process restart,
  original-key recovery, live role/organization Token/current-rights revocation,
  real over-budget Manifest-only rejection, and both native/source corruption
  were exercised. Corrupt originals were retained, not repaired.
- Go vet/API build, frontend production build and diff checks pass.

Private evidence root:
`C:/Users/Henry/.codex/reports/fork-host-raw-live-20261022`.
Use `verified/live-results.json`, `verified/accounting.json`, `verified/closure.json`,
`verified-integrity/integrity-results.json`, `final/fork-postgres-clean.log`,
`final/fork-test-output/fork-postgres.xml`, `final/postgres-regressions.jsonl`,
native transport logs and locale screenshots. Earlier failed attempts, the
native-contract red test, corrected results and runtime image/import provenance
are retained separately. Never commit the private profiles/tokens from this root.

The protected baseline includes 1,549 existing #76/#307 containers and the
primary/candidate #76 plus #307 worktrees. Compare identity, image, start time,
mounts/ports, HEAD and status; do not infer container changes from JSON object
property order. Only the trial's exact labeled containers/processes are stopped;
all volumes/data/evidence remain. No push, PR, merge, deployment, Core integration,
provider disposal or production/customer-data acceptance was performed.
