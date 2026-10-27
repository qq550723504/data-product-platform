# Real Core + standalone Next, against the dedicated disposable dependencies.
param([switch]$Stop)
$ErrorActionPreference='Stop'
$repoRoot=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$runRoot=Join-Path $repoRoot '.data/agent-q1-live'
$webRoot=Join-Path $repoRoot 'apps/web'
if ($Stop) {
    $record=Get-Content -LiteralPath (Join-Path $runRoot 'processes.json') -Raw | ConvertFrom-Json
    if ($record.repoRoot -ne $repoRoot) { throw 'Runtime owner mismatch' }
    $owned=@()
    foreach ($item in $record.processes) {
        $process=Get-Process -Id $item.processId -ErrorAction SilentlyContinue
        if (-not $process) { continue }
        $listener=Get-NetTCPConnection -LocalPort $item.port -State Listen -ErrorAction SilentlyContinue
        if ($process.ProcessName -ne $item.name -or [Math]::Abs(($process.StartTime.ToUniversalTime()-([DateTimeOffset]$item.startedAt).UtcDateTime).TotalSeconds) -gt 1 -or -not ($listener.OwningProcess -contains $item.processId)) { throw 'Ownership could not be verified. No process stopped.' }
        $owned+=$process
    }
    $owned | Stop-Process
    Write-Output 'Stopped only the recorded Q1 live Core/web processes; dependencies and test data retained.'
    return
}
foreach ($port in @(18496,4187)) {
    if (Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue) { throw "Port $port is occupied. No existing process stopped." }
}
$apiPath=Join-Path $runRoot 'api.exe'
$dist='.next-agent-q1-live'
$standalone=Join-Path $webRoot "$dist/standalone"
if (-not (Test-Path -LiteralPath $apiPath) -or -not (Test-Path -LiteralPath (Join-Path $standalone 'server.js'))) { throw 'Build Core and standalone Next before starting.' }
New-Item -ItemType Directory -Force -Path (Join-Path $runRoot 'evidence') | Out-Null
Copy-Item -LiteralPath (Join-Path $webRoot "$dist/static") -Destination (Join-Path $standalone $dist) -Recurse -Force
if (Test-Path -LiteralPath (Join-Path $webRoot 'public')) { Copy-Item -LiteralPath (Join-Path $webRoot 'public') -Destination $standalone -Recurse -Force }
$settings=@{
    APP_ENV='agent-quality-readonly-live'; APP_HTTP_ADDR='127.0.0.1:18496'
    POSTGRES_DSN='postgres://agent_q1_reader:agent-q1-reader-only@127.0.0.1:25493/agent_q1_live?sslmode=disable'
    REDIS_ADDR='127.0.0.1:26493'; REDIS_DB='0'; REDIS_PASSWORD=''
    OBJECT_STORAGE_ENDPOINT='127.0.0.1:29493'; OBJECT_STORAGE_ACCESS_KEY='agent-q1-only'
    OBJECT_STORAGE_SECRET_KEY='agent-q1-local-objects-only'; OBJECT_STORAGE_BUCKET='agent-q1-live'; OBJECT_STORAGE_USE_SSL='false'; OBJECT_STORAGE_WORKSPACE_PROFILE=''
    INDUSTRY_PACK_ROOT=(Join-Path $repoRoot 'industry-packs')
    QUALITY_REMEDIATION_PLAN_REF='park/remediation/energy-quarantine-v1.yaml'
    OPENMETADATA_ENABLED='false'; HOP_ENABLED='false'; SPLINK_ENABLED='false'; GX_ENABLED='false'; LABEL_STUDIO_ENABLED='false'; DELIVERY_API_ENABLED='false'
    RIGHTS_API_TOKEN=''; RIGHTS_API_ACTOR_ID=''; RIGHTS_API_WORKSPACE_IDS=''
    HUMAN_DECISION_API_ENABLED='true'; HUMAN_DECISION_API_TOKEN='agent-q1-live-secret'; HUMAN_DECISION_API_SUBJECT='agent-q1-live-reader'
    HUMAN_DECISION_API_ACTOR_ID='22222222-2222-4222-8222-222222222222'; HUMAN_DECISION_API_WORKSPACE_IDS='11111111-1111-4111-8111-111111111111'
    PLATFORM_API_BASE_URL='http://127.0.0.1:18496'; POC_WORKSPACE_ID='11111111-1111-4111-8111-111111111111'
    POC_ENABLE_REVIEW_ACTIONS='false'; POC_ENABLE_RELEASE_ACTIONS='false'; POC_ENABLE_EXECUTION_ACTIONS='false'; POC_ENABLE_DATASET_ACTIONS='false'
    POC_ENABLE_QUALITY_ACTIONS='false'; POC_ENABLE_COMPLIANCE_ACTIONS='false'; POC_ENABLE_DELIVERY_ACTIONS='false'; POC_ENABLE_INGEST_ACTIONS='false'; POC_ENABLE_INGESTION_ACTIONS='false'
    AGENT_Q1_LIVE_ACK='ISOLATED_READ_ONLY'; AGENT_Q1_READ_TRACE=(Join-Path $runRoot 'core-requests.jsonl')
    HOSTNAME='127.0.0.1'; PORT='4187'; NEXT_TELEMETRY_DISABLED='1'
}
$previous=@{}
try {
    foreach ($name in $settings.Keys) { $previous[$name]=[Environment]::GetEnvironmentVariable($name,'Process'); [Environment]::SetEnvironmentVariable($name,$settings[$name],'Process') }
    $api=Start-Process -FilePath $apiPath -WorkingDirectory $repoRoot -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $runRoot 'api.stdout.log') -RedirectStandardError (Join-Path $runRoot 'api.stderr.log')
    $nodePath=(Get-Command node -CommandType Application | Select-Object -First 1).Source
    $web=Start-Process -FilePath $nodePath -ArgumentList @('tests/agent-quality-live/start-web.mjs') -WorkingDirectory $repoRoot -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $runRoot 'web.stdout.log') -RedirectStandardError (Join-Path $runRoot 'web.stderr.log')
    @{ repoRoot=$repoRoot; url='http://127.0.0.1:4187/agent'; processes=@(
        @{processId=$api.Id;port=18496;name='api';startedAt=$api.StartTime.ToUniversalTime().ToString('o')},
        @{processId=$web.Id;port=4187;name='node';startedAt=$web.StartTime.ToUniversalTime().ToString('o')}
    ) } | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $runRoot 'processes.json') -Encoding utf8
} finally {
    foreach ($name in $previous.Keys) { [Environment]::SetEnvironmentVariable($name,$previous[$name],'Process') }
}
Write-Output 'Q1 real Core read-only preview: http://127.0.0.1:4187/agent'
