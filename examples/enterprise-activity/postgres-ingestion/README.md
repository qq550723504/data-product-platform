# PostgreSQL → Hop runnable validation (#298)

B3's actual browser loop is implemented and locally exercised. See the [B3
execution record](B3-RUN.md) and [31 independent live Core checks](results/2026-10-11/b3-report.json).
Human explanation acceptance was explicitly deferred by the user. Healthy
OpenMetadata binding/reconnect and remote CI remain NOT_RUN. The earlier B1/B2
records below keep their original verification boundaries.

This is the bounded B1 engine trial defined by [the existing contract](../../../docs/architecture/postgres-ingestion-v1.md). It runs PostgreSQL and Hop, loads the existing synthetic fixtures, and validates JDBC extraction into local staging. It creates no Core DatasetVersion, certification, delivery or production deployment.

The [2026-10-11 recorded local run](results/2026-10-11/report.json) passed PGI-01–04 and the real reader privilege checks. Its implementation hashes use Git's LF source encoding; CSV hashes always cover the actual exported bytes. Its `base_sha` is the source checkout baseline, not a claim of remote CI approval.

From the repository root, with Python 3.11+ and Docker using Linux amd64 containers:

```powershell
python examples/enterprise-activity/postgres-ingestion/smoke.py --output .data/postgres-ingestion-b1
```

Use a **new** output directory on each run. No provider credentials or existing database are required. The script downloads the SHA-256 pinned pgJDBC 42.7.14 jar, builds a local derivative of the digest-pinned Hop 2.19.0 image (removing its bundled 42.7.12 driver), and starts an isolated synthetic PostgreSQL 16.15 source. Only Hop's control port is published, on `127.0.0.1`; PostgreSQL has no host port. Both containers, their anonymous source volume, the dedicated network and temporary secret files are removed in `finally`. Other projects' containers/volumes are untouched; the public image/build cache and non-secret output directory remain.

The script has no new Python dependencies, connector framework, queue or scheduler. PostgreSQL/JDBC and Hop Table Input / Text File Output own the commodity extraction. The Python code is the finite test oracle and fault harness. This reuses the B0 Hop choice; no SeaTunnel runtime is introduced. Hop retains Apache-2.0 and pgJDBC BSD-2-Clause licensing; the upstream image and dependency notices continue to apply.

| Check | Actual verification |
|---|---|
| PGI-01 | Three fixed SQL branches, 6/10/14 rows, ordered string-by-string equality with the frozen fixtures; repeat output byte hashes match |
| PGI-02 | Chinese, quotes, commas, embedded LF, surrounding whitespace and empty strings survive; original date strings and `-999` remain unchanged; nullable/NULL, missing/extra columns, changed type and renamed schema are rejected before registration |
| source-read-only | Real TCP/SCRAM reader login; INSERT/UPDATE/DELETE/TRUNCATE/ALTER/schema/table/TEMP/private-table/owner-role attempts fail even after disabling the session read-only setting |
| PGI-03 | Generated admin, reader and Hop credentials are distinct; the engine cannot mount the admin secret; scan URLs, XML, decoded compressed metadata/logs, container logs/configured env/commands and output artifacts for test credential values |
| PGI-04 | Proxy forwards to real Hop before dropping register/start responses; status outage; source database stop/start; Hop restart while registered, finished and actively extracting; at most one start per attempt |

`report.json` contains implementation/fixture/template hashes, image IDs/digests, Java/JDBC versions, append-only trial observations and PASS/FAIL. Successful manifests contain column structure/count/size/SHA-256; `requests.json` records URLs and submitted XML without Authorization headers. Local `staging/` contains synthetic RAW CSVs. CI archives only reports/manifests/requests, never secret files, diagnostic logs or row samples. These are **trial evidence**, not Core Audit/Evidence/Cost facts or batch acceptance receipts.

Recovery results are intentionally explicit: lost register response without an ID stays `UNKNOWN` and is never started; lost start response is reconciled by the saved exact ID; lost status does not trigger another start; Hop restart loses its in-memory IDs and the old attempts stay `UNKNOWN`. Complete old CSVs alone do not authorize admission; incomplete running-restart output is rejected. A confirmed source failure uses an explicit new attempt and separate directory after the source returns. Automatic cross-restart recovery is not claimed.

Two integration gaps were exposed by the real server trial. Hop 2.19.0 requires `metastore_json` to contain gzip/base64 encoded JSON, including `{}` for an empty provider; the existing Bridge's empty value fails registration. This version also reads pipeline parameters from the **registration execution configuration**, not arbitrary `startPipeline` query parameters. B1 constructs the correct registration envelope. B2 must use this observed protocol when adapting the existing client; the old remote Bridge has not been accepted by this trial.

For secrets, plain shell environment variables do not automatically become Hop variables in this version. The launcher reads the mounted synthetic reader secret into a mode-0600 Hop variable configuration on executor-private `/tmp` tmpfs; submitted metadata retains only `${SOURCE_DB_PASSWORD}`. No password is passed in HTTP parameters, process arguments, workflow artifacts or diagnostics. The in-process configuration and explicitly mounted secret files are the authorized resolution boundary, not exportable artifacts.

The source is quiescent between explicit test mutations: the fixture owner is NOLOGIN and the only application account has CONNECT / schema USAGE / three-table SELECT. The provisioning superuser exists solely for this isolated harness. This proves the fixed text/NOT NULL contract, not arbitrary native PostgreSQL types or a cross-table snapshot under concurrent source updates.

PGI-05–09, Core durable requests/attempts, three-slot atomic admission, platform Audit/Evidence/Cost, browser operations and human acceptance remain B2/B3 work. B1 local staging directories are not evidence of MinIO/S3 immutability or storage permission isolation. The CI `hop-smoke` job executes this script for changes to the trial, source fixtures, Hop adapter, storage adapter or CI definition; the older CSV aggregate step does not stand in for this JDBC/fault matrix.

## B2 storage prerequisite: real provider trial

The [recorded storage run](results/2026-10-11/storage-report.json) tests actual MinIO and the repository's existing minio-go v7.0.83 SDK. It is a separate storage protocol trial, not a PostgreSQL-to-MinIO batch acceptance receipt. Run from the repository root with Git, Go 1.24.8+ (Linux amd64 cross-compilation), Python and Docker:

```powershell
python examples/enterprise-activity/postgres-ingestion/storage_smoke.py --output .data/postgres-ingestion-storage
```

The first run builds **unmodified official MinIO source**, release `RELEASE.2025-10-15T17-29-55Z`, commit `9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a`. Root Docker Hub/Quay images were unavailable in the local trial. The build is cached under ignored `.data/pgi-b2-storage-cache`; source, compiler, binary and image identities are recorded. MinIO is AGPL-3.0, minio-go is Apache-2.0. No new platform dependency, storage engine fork, catalog, queue or workflow runtime is introduced; no image is published.

The upstream OSS project is archived and its final release has [GHSA-hv4r-mvr4-25vw](https://github.com/minio/minio/security/advisories/GHSA-hv4r-mvr4-25vw). This trial uses the vendor's unsigned-trailer rejection mitigation at a digest-pinned Caddy gateway. The private MinIO port is unpublished on a separate network; engine-network probes must fail through **both DNS and direct IP**. The gateway is the only permitted storage entrance. This is a condition of the result, not a security fix to MinIO or approval of the existing `minio:latest` Compose setup. Production provider maintenance/admission remains unresolved; a direct MinIO endpoint does not inherit this PASS.

The existing `Store.Put` permits overwrites, so it cannot establish this ingestion invariant. Temporary STS engine credentials allow Get/Put only in one staging attempt prefix. Core credentials read that staging prefix and Get/Put their own Core prefix. The gateway requires literal `If-None-Match: *` for every Core PUT and rejects Core DELETE, multipart POST and copy. `Store.PutImmutable` uses the existing SDK's native atomic create-only operation, disables multipart for the bounded 128 MiB contract, and reports `ErrObjectExists`; an acceptance retry must independently read/hash an existing object before using it.

The trial verifies byte fidelity, nine real HTTP-403 mutation/access denials plus one conditional HTTP-412 conflict, two concurrent writes with exactly one winner and one intact payload, the unsigned-trailer rejection, and absence of root credentials in Docker metadata/logs/test output. Containers/networks/private credentials are removed; only local build cache and non-secret report remain. CI archives only `storage-report.json`. Remote CI is not proven by this local receipt.

At the time of this storage prerequisite, the gate was PASS and Core requests/physical attempts, three-RAW atomic finalize, receipt replay and PGI-05–07 were **NOT_RUN**. That historical report is retained. The subsequent B2 execution below supplies the Core result; B3 remains separate.

## B2: atomic Core acceptance

The probe DSN and native Hop metadata must name the same host/port/database/user;
custom JDBC URL overrides and alternate connection modes are rejected. The
local harness uses one Docker host endpoint reachable by Core and Hop (host
endpoint on Windows, isolated bridge gateway on Linux). Windows uses the pgx
lookup hook to resolve host.docker.internal to loopback on the host, while Hop
uses Docker resolution. InstanceRef is an operator reference, not an observed
cluster system identifier. This local-staging pilot requires a loopback Hop
control endpoint. Cached engine, injected source credentials, staging and Core
storage settings require runtime restart after changes; commands refuse stale
clients.

A claim records an UNKNOWN intent with zero confirmed invocation quantity. It
never proves zero eventual cost or an actual call. Actual HTTP-call attempts
append their separate ACTUAL cost with the response observation, including
failures/unknown responses. A crash before observation leaves an explicit
unobserved intent, rather than a phantom actual charge or a silent zero-cost
claim. Existing attempt/CostAllocation facts are reused; no runtime or billing
infrastructure is added.

The [recorded B2 run](results/2026-10-11/b2-report.json) executes the actual ingestion HTTP Handler and application services against fresh Core PostgreSQL, the B1 PostgreSQL/Hop pair and the mandatory storage gateway. It validates PGI-05–07; it is not browser or human acceptance. Reproduce from the repository root using the same prerequisites as the storage trial:

```powershell
python examples/enterprise-activity/postgres-ingestion/b2_smoke.py --output .data/postgres-ingestion-b2
```

Each run uses a new directory. The source probe, Hop control API, Core PostgreSQL and gateway are loopback-published. Source/Core databases are separate instances. Containers, networks and temporary credential files are removed and their absence checked; RAW samples remain only in ignored local staging. CI archives the sanitized `b2-report.json`, which contains implementation hashes, manifest structure/hashes, stable receipt/version IDs and executed results. It excludes secrets and source rows. The runner rejects code changes during execution. `base_sha` describes the checkout baseline; implementation hashes bind the actual working source. Remote CI remains independently unverified.

`internal/ingestion/domain` owns the provider-neutral request, manifest and three-slot receipt contract. `application` owns explicit Prepare / Dispatch / Recover / RetryFailed / Admit Commands. The Hop Adapter reuses the existing client and B1's registration envelope. Missing/mismatched identity in a Hop status response is rejected rather than filled from the requested ID. Every register/start/status has a durable physical invocation identity and typed CostAllocation before the call; outcomes, Audit, Evidence and Outbox are appended. A lost register reply cannot authorize start, a lost start is recovered only through the persisted ID, and UNKNOWN cannot create a replacement attempt. Only confirmed FAILED permits an explicit new attempt, with independent cost/history.

Admit checks all manifest identities, exact three-slot membership, headers/order, rows, byte sizes, actual SHA-256 and the attempt-owned URI. It preserves CSV strings, including blanks, whitespace, original dates and `-999`. It completes every immutable Core PUT and reads/hashes each object before allocating a version. It then locks the workspace fence, operation, sorted resources and sorted datasets, rechecks authority/source/config, and reuses the existing DatasetVersion allocation/publication primitives in one PostgreSQL transaction. Three READY versions, receipt bindings, manifest Evidence, Audit and Outbox commit together. Object/transaction failures expose zero partial versions; safe orphan objects can be verified and reused. Deferred database guards reject incomplete receipts and freeze request/attempt/observation/receipt history. Same-key concurrent/repeated commands return the same operation/attempt/receipt; conflicting request or manifest identities are rejected.

The matrix includes invalid membership/source/workspace/template/schema/config/path/dataset/header/count/hash/size, actual staging corruption, six simultaneous admissions with an independent reader seeing only zero or three versions, six simultaneous Prepare replays, a failed object slot, lost successful PUT response, a real mid-transaction slot failure and a real PostgreSQL COMMIT response drop **after backend commit**. It also executes source SELECT revocation, Core resource blocking, trusted principal disable, connection instance replacement, revocation between object writes and Core commit, lost register/start/status replies, malformed status identity, confirmed engine failure/new attempt and Hop restart/identity loss. Physical Hop requests must equal durable invocation/cost records. Native attempt credentials are denied Core read/write and sibling-attempt writes. Core facts, wire requests, logs and artifacts are scanned for generated secret values.

Reuse check: DatasetVersion repositories, transaction/fence, storage, principal Resolver, CostAllocation, Evidence/Audit and Outbox already supply the infrastructure; they are reused. Hop/JDBC own extraction and MinIO's native IAM/STS/conditional PUT own storage permissions and atomic object creation. A second connector runtime such as SeaTunnel, a new workflow scheduler or custom IAM/signature implementation would duplicate these capabilities. The added madmin-go v3.0.94 (Apache-2.0, compatible with the existing minio-go v7.0.83) is used only by the isolated test to provision native identities/policy. Runtime publishing uses the existing storage SDK. MinIO forbids STS-to-STS chaining, so the publisher parent is a native regular user with staging-only Get/Put; its per-attempt child has a narrower prefix. Core receives a distinct scoped credential. Hop receives neither storage identity nor Core credentials. The parent credential must be provisioned by the storage operator; the application does not implement IAM.

For an existing controlled Core database, `cmd/ingestion-pilot` runs the same Handler with server-owned private profile and credential-file references. Apply migrations through the existing migration runner, including 000051; 000050 is reserved for #295. Set `INGESTION_PROFILE` and `INGESTION_CORE_DSN_FILE` to private files, then run from `apps/platform`:

```powershell
go run ./cmd/ingestion-pilot
```

The default address is `127.0.0.1:8098`. The authenticated API is `POST /api/v1/ingestions`, `POST /api/v1/ingestions/{id}/dispatch|recover|retry-failed|admit`, and `GET /api/v1/ingestions/{id}`. Prepare accepts workspace/request/batch keys and consistency assertions for the three targets. An operator first provisions assets through existing Commands and explicitly binds their IDs in the private profile's `Targets` (`workspaceId`, `code`, `resourceId`, `datasetId`). Asset codes/names from generic CRUD cannot establish authority; no code-based adoption or missing-binding fallback exists. Source queries expose only the authenticated workspace's bindings. Swapped, repeated, unrelated and same-name replacement targets are denied. Every Command verifies the explicit IDs and typed source relation under the existing resource/Dataset locks, and binding changes alter the frozen configuration hash. Identity, capability, SQL, connection, template, metadata and secret references come from server configuration. Admit accepts the observed manifest. There is no generic status PATCH or raw engine-payload endpoint. Current actor/workspace/capability, source privileges/schema and public configuration are rechecked at Commands; denied accepted-receipt replay returns no receipt. The source is quiescent and revocation checks are point-in-time checks, not a distributed transaction with the source or an arbitrary concurrent-source snapshot guarantee.

B2 PASS is conditional on this controlled profile and mandatory gateway; it does not admit the archived provider for production. PGI-08–09, existing processing/quality/rights/certification/delivery, browser flow and non-developer explanation remain B3 work. No certification, delivery credential or user acceptance is inferred from RAW READY.
