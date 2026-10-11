# PostgreSQL → Hop B1 runnable validation (#298)

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

PGI-05–09, real object storage/attempt access isolation, Core durable requests/attempts, three-slot atomic admission, platform Audit/Evidence/Cost, browser operations and human acceptance remain B2/B3 work. Local staging directories are not evidence of MinIO/S3 immutability or storage permission isolation. The CI `hop-smoke` job executes this script for changes to the trial, source fixtures, Hop adapter or CI definition; the older CSV aggregate step does not stand in for this JDBC/fault matrix.
