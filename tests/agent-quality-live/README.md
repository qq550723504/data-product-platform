# Agent Q1 real Core read-only acceptance

This test uses the actual Go API and repositories, production Next standalone, and persisted synthetic historical facts. It does not run engines, models, workers, delivery or quality Commands. It reuses the platform's existing PostgreSQL/Redis/MinIO dependencies; no new runtime or queue is introduced.

Use only this isolated checkout and the named disposable test project/database. The published passwords/tokens are synthetic local test values, not credentials for a retained environment. `seed.sql` is first-install only: do not rerun it against existing history or point it at another database. Check that the fixed ports are free before first startup; never stop another owner's listener.

Run from the repository root in PowerShell. Keep the named MinIO image from `compose.yml` available locally (source pin already used in `deploy/trial/runtime.py`). Postgres/MinIO data is tmpfs; this preview is disposable and does not survive dependency container removal. It shares Docker's existing bridge to avoid allocating another subnet, while all published ports bind `127.0.0.1`.

```powershell
# FIRST INSTALL ONLY: new dependencies and an empty test database.
docker compose -p dpp-agent-q1-live-20261023 -f tests/agent-quality-live/compose.yml up -d --wait --wait-timeout 60
New-Item -ItemType Directory -Force -Path '.data/agent-q1-live/evidence' | Out-Null
Push-Location apps/platform
go build -o ../../.data/agent-q1-live/api.exe ./cmd/api
go build -o ../../.data/agent-q1-live/migrate.exe ./cmd/migrate
Pop-Location
$oldDsn=$env:POSTGRES_DSN
try {
  $env:POSTGRES_DSN='postgres://postgres:agent-q1-bootstrap-only@127.0.0.1:25493/agent_q1_live?sslmode=disable'
  & '.data/agent-q1-live/migrate.exe' -direction up -dir migrations
  if ($LASTEXITCODE -ne 0) { throw 'Migration failed' }
} finally { $env:POSTGRES_DSN=$oldDsn }
Get-Content -LiteralPath tests/agent-quality-live/seed.sql -Raw | docker exec -i dpp-agent-q1-live-20261023-postgres-1 psql -X -v ON_ERROR_STOP=1 -U postgres -d agent_q1_live
if ($LASTEXITCODE -ne 0) { throw 'Seed failed' }

# The original .next build/fixture preview can remain running.
& './tests/agent-quality-live/build-web.ps1'
& './tests/agent-quality-live/start.ps1'
```

Core is on 18496, Next on 4187, PostgreSQL on 25493, Redis on 26493 and MinIO on 29493. Core runtime uses a separate SELECT-only non-superuser role with default read-only transactions. Its server-side synthetic principal can access only the test workspace. All web business actions and external engine/delivery flags are off. The API's existing startup creates an empty test bucket and queues a health ping; no worker consumes it. These bootstrap effects are outside the business-table no-change assertion.

Capture evidence **after bootstrap but before acceptance reads**:

```powershell
docker cp tests/agent-quality-live/fingerprint.sql dpp-agent-q1-live-20261023-postgres-1:/tmp/q1-fingerprint.sql
docker exec dpp-agent-q1-live-20261023-postgres-1 psql -X -qAt -v ON_ERROR_STOP=1 -U postgres -d agent_q1_live -f /tmp/q1-fingerprint.sql | Set-Content -LiteralPath '.data/agent-q1-live/evidence/database-before.json' -Encoding utf8
$oldAck=$env:AGENT_Q1_LIVE_ACK
try {
  $env:AGENT_Q1_LIVE_ACK='ISOLATED_READ_ONLY'
  node --test tests/agent-quality-live/api-readonly.test.mjs
  if ($LASTEXITCODE -ne 0) { throw 'API tests failed' }
} finally { $env:AGENT_Q1_LIVE_ACK=$oldAck }
```

Use the real browser at `http://127.0.0.1:4187/agent`: select the dataset/v1, navigate history to page two, open its old FAIL report, page findings, look up the existing attempt, and refresh. See the product acceptance record for wrong-version, missing report/attempt and foreign workspace scenarios. Never press an execution action or enable a worker. Browser interactions in this acceptance used CUA; no standalone Playwright runner was added.

After browser acceptance:

```powershell
docker exec dpp-agent-q1-live-20261023-postgres-1 psql -X -qAt -v ON_ERROR_STOP=1 -U postgres -d agent_q1_live -f /tmp/q1-fingerprint.sql | Set-Content -LiteralPath '.data/agent-q1-live/evidence/database-after.json' -Encoding utf8
docker cp tests/agent-quality-live/role-proof.sql dpp-agent-q1-live-20261023-postgres-1:/tmp/q1-role-proof.sql
docker exec -e PGPASSWORD=agent-q1-reader-only dpp-agent-q1-live-20261023-postgres-1 psql -X -qAt -v ON_ERROR_STOP=1 -U agent_q1_reader -d agent_q1_live -f /tmp/q1-role-proof.sql | Set-Content -LiteralPath '.data/agent-q1-live/evidence/database-role.json' -Encoding utf8
node tests/agent-quality-live/verify-readonly.mjs
```

`fingerprint.sql` creates only a transaction-local temp table, hashes all public row contents in deterministic order, and rolls back. `verify-readonly.mjs` reads recorded evidence only; it requires identical before/after arrays, SELECT on all public tables, zero table DML/CREATE privileges and all actual Next→Core requests to be GET. `start-web.mjs` uses Node's diagnostics channel to observe real network calls, logs only whitelisted fields, and does not mock responses. The fixed test identity has no provider credentials.

Stop/restart only these owned Core/web processes with:

```powershell
& './tests/agent-quality-live/start.ps1' -Stop
& './tests/agent-quality-live/start.ps1'
```

Stop verifies the exact checkout, PID, name, process start time and listener before stopping anything. It leaves containers/data and every other preview intact. Stopping or rebuilding this preview does not imply permission to stop retained containers, remove volumes, run business Commands or deploy elsewhere.
