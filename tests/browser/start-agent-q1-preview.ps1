# Starts only the isolated Next preview and a read-only synthetic Core fixture.
# No model, real Core, database, queue, engine or provider is started.
param([switch]$Stop)
$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$webRoot = Join-Path $repoRoot 'apps/web'
$runRoot = Join-Path $repoRoot '.data/agent-q1-preview'
if ($Stop) {
    $record = Get-Content -LiteralPath (Join-Path $runRoot 'processes.json') -Raw | ConvertFrom-Json
    if ($record.repoRoot -ne $repoRoot) { throw 'Preview owner mismatch. No process stopped.' }
    $startedAt = ([DateTimeOffset]$record.startedAt).UtcDateTime
    $owned = @()
    foreach ($entry in @(@{ processId = $record.webPid; port = 4186 }, @{ processId = $record.fixturePid; port = 4459 })) {
        $process = Get-Process -Id $entry.processId -ErrorAction SilentlyContinue
        if (-not $process) { continue }
        $listener = Get-NetTCPConnection -LocalPort $entry.port -State Listen -ErrorAction SilentlyContinue
        if ($process.ProcessName -ne 'node' -or [Math]::Abs(($process.StartTime.ToUniversalTime() - $startedAt).TotalSeconds) -gt 10 -or -not ($listener.OwningProcess -contains $entry.processId)) { throw 'Preview ownership could not be verified. No process stopped.' }
        $owned += $process
    }
    $owned | Stop-Process
    Write-Output 'Stopped only the recorded Agent Q1 preview processes.'
    return
}
$nodePath = (Get-Command node -CommandType Application | Select-Object -First 1).Source
if (-not (Test-Path -LiteralPath (Join-Path $webRoot '.next/BUILD_ID'))) { throw 'Build apps/web before starting the preview.' }
$standaloneRoot = Join-Path $webRoot '.next/standalone'
if (-not (Test-Path -LiteralPath (Join-Path $standaloneRoot 'server.js'))) { throw 'The standalone Next build is missing.' }
foreach ($port in @(4186, 4459)) {
    if (Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue) { throw "Port $port is occupied. No existing process was stopped." }
}
New-Item -ItemType Directory -Path $runRoot -Force | Out-Null
# Next standalone output intentionally excludes static/public assets.
Copy-Item -LiteralPath (Join-Path $webRoot '.next/static') -Destination (Join-Path $standaloneRoot '.next') -Recurse -Force
if (Test-Path -LiteralPath (Join-Path $webRoot 'public')) { Copy-Item -LiteralPath (Join-Path $webRoot 'public') -Destination $standaloneRoot -Recurse -Force }
$settings = @{
    PLATFORM_API_BASE_URL = 'http://127.0.0.1:4459'
    POC_WORKSPACE_ID = '11111111-1111-4111-8111-111111111111'
    HUMAN_DECISION_API_TOKEN = 'review-secret' # Public synthetic fixture credential only.
    NEXT_TELEMETRY_DISABLED = '1'
    HOSTNAME = '127.0.0.1'
    PORT = '4186'
    POC_ENABLE_REVIEW_ACTIONS = 'false'
    POC_ENABLE_RELEASE_ACTIONS = 'false'
    POC_ENABLE_EXECUTION_ACTIONS = 'false'
    POC_ENABLE_DATASET_ACTIONS = 'false'
    POC_ENABLE_QUALITY_ACTIONS = 'false'
    POC_ENABLE_COMPLIANCE_ACTIONS = 'false'
    POC_ENABLE_DELIVERY_ACTIONS = 'false'
    POC_ENABLE_INGEST_ACTIONS = 'false'
    POC_ENABLE_INGESTION_ACTIONS = 'false'
}
$previous = @{}
try {
    foreach ($name in $settings.Keys) {
        $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
        [Environment]::SetEnvironmentVariable($name, $settings[$name], 'Process')
    }
    $fixture = Start-Process -FilePath $nodePath -ArgumentList @('tests/browser/agent-q1-preview-fixture.mjs') -WorkingDirectory $repoRoot -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $runRoot 'fixture.stdout.log') -RedirectStandardError (Join-Path $runRoot 'fixture.stderr.log')
    $web = Start-Process -FilePath $nodePath -ArgumentList @('.next/standalone/server.js') -WorkingDirectory $webRoot -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $runRoot 'web.stdout.log') -RedirectStandardError (Join-Path $runRoot 'web.stderr.log')
    @{ repoRoot = $repoRoot; fixturePid = $fixture.Id; webPid = $web.Id; url = 'http://127.0.0.1:4186/agent'; startedAt = [DateTime]::UtcNow.ToString('o') } | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $runRoot 'processes.json') -Encoding utf8
} finally {
    foreach ($name in $previous.Keys) { [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process') }
}
Write-Output 'Read-only preview started: http://127.0.0.1:4186/agent'
Write-Output "Logs and exact process IDs: $runRoot"
