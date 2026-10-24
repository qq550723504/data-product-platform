# Starts only the owned plan-only acceptance host and isolated Next build.
$ErrorActionPreference='Stop'
$repoRoot=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$runRoot=Join-Path $repoRoot '.data/agent-q2-preview'
$webRoot=Join-Path $repoRoot 'apps/web'
foreach ($port in @(18497,4188,4189)) {
 if (Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue) { throw "Port $port occupied; nothing stopped." }
}
$standalone=Join-Path $webRoot '.next-agent-q2/standalone'
if (-not (Test-Path (Join-Path $standalone 'server.js'))) { throw 'Build the isolated web first.' }
Copy-Item -LiteralPath (Join-Path $webRoot '.next-agent-q2/static') -Destination (Join-Path $standalone '.next-agent-q2') -Recurse -Force
if (Test-Path (Join-Path $webRoot 'public')) { Copy-Item -LiteralPath (Join-Path $webRoot 'public') -Destination $standalone -Recurse -Force }
$profilePath=Join-Path $runRoot 'readonly-profile.json'
if(-not(Test-Path -LiteralPath $profilePath)){throw 'Prepare the independent readonly database profile first.'}
$profile=Get-Content -LiteralPath $profilePath -Raw|ConvertFrom-Json
if($profile.database -notlike 'agent_q2_acceptance_*'){throw 'Independent acceptance database required.'}
$settings=@{
 AGENT_Q2_PREVIEW_ACK='ISOLATED_PLAN_ONLY';PLATFORM_API_BASE_URL='http://127.0.0.1:18497'
 POC_WORKSPACE_ID='11111111-1111-4111-8111-111111111111'; HUMAN_DECISION_API_TOKEN='agent-q2-core-only'
 AGENT_Q2_PREVIEW_DSN=$profile.dsn;AGENT_TASKS_WRITE_ENABLED='false';AGENT_TASKS_UI_WRITE_ENABLED='false';HOSTNAME='127.0.0.1';PORT='4188';NEXT_TELEMETRY_DISABLED='1'
 POC_ENABLE_REVIEW_ACTIONS='false';POC_ENABLE_RELEASE_ACTIONS='false';POC_ENABLE_EXECUTION_ACTIONS='false';POC_ENABLE_DATASET_ACTIONS='false'
 POC_ENABLE_QUALITY_ACTIONS='false';POC_ENABLE_COMPLIANCE_ACTIONS='false';POC_ENABLE_DELIVERY_ACTIONS='false';POC_ENABLE_INGEST_ACTIONS='false';POC_ENABLE_INGESTION_ACTIONS='false'
}
$previous=@{}
try {
 foreach ($name in $settings.Keys) { $previous[$name]=[Environment]::GetEnvironmentVariable($name,'Process');[Environment]::SetEnvironmentVariable($name,$settings[$name],'Process') }
 $api=Start-Process -FilePath (Join-Path $runRoot 'agent-preview.exe') -WorkingDirectory $repoRoot -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $runRoot 'api.stdout.log') -RedirectStandardError (Join-Path $runRoot 'api.stderr.log')
 $nodePath=(Get-Command node -CommandType Application|Select-Object -First 1).Source
 $web=Start-Process -FilePath $nodePath -ArgumentList @((Join-Path $standalone 'server.js')) -WorkingDirectory $standalone -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $runRoot 'web.stdout.log') -RedirectStandardError (Join-Path $runRoot 'web.stderr.log')
 @{repoRoot=$repoRoot;processes=@(@{processId=$api.Id;name=$api.ProcessName;port=18497;startedAt=$api.StartTime.ToUniversalTime().ToString('o')},@{processId=$web.Id;name=$web.ProcessName;port=4188;startedAt=$web.StartTime.ToUniversalTime().ToString('o')})}|ConvertTo-Json -Depth 5|Set-Content (Join-Path $runRoot 'processes.json')
} finally { foreach ($name in $previous.Keys){[Environment]::SetEnvironmentVariable($name,$previous[$name],'Process')} }
$gatewayId=& docker ps -aq --filter name=^dpp-agent-q2-gateway-20261023$
if ($gatewayId) {
 $gateway=(& docker inspect $gatewayId|ConvertFrom-Json)[0]
 $configMount=$gateway.Mounts|Where-Object Destination -eq '/etc/caddy/Caddyfile'
 if ($gateway.Config.Labels.'codex.scope' -ne 'agent-q2-test' -or $gateway.State.Running -or -not $configMount -or [IO.Path]::GetFullPath($configMount.Source) -ne [IO.Path]::GetFullPath((Join-Path $PSScriptRoot 'Caddyfile'))) {throw 'Existing gateway does not match stopped owned preview.'}
 & docker start $gatewayId
} else {
 & docker run -d --name dpp-agent-q2-gateway-20261023 --label codex.scope=agent-q2-test --network bridge -p 127.0.0.1:4189:80 --mount "type=bind,source=$PSScriptRoot/Caddyfile,target=/etc/caddy/Caddyfile,readonly" caddy:2.11.7-alpine
}
if ($LASTEXITCODE -ne 0) {throw 'Gateway startup failed; retain process record for recovery.'}
$ready=$false
$deadline=[DateTime]::UtcNow.AddSeconds(20)
$previewAuth='Basic '+[Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes('operator:agent-q2-preview'))
do {
 try {
  $response=Invoke-WebRequest -Uri 'http://127.0.0.1:4189/agent' -Headers @{Authorization=$previewAuth} -TimeoutSec 2 -UseBasicParsing
  $ready=$response.StatusCode -eq 200
 } catch { $ready=$false }
 if (-not $ready) {Start-Sleep -Milliseconds 250}
} while (-not $ready -and [DateTime]::UtcNow -lt $deadline)
if (-not $ready) {throw 'Preview is not HTTP-ready; preserve recorded processes and inspect logs. No automatic restart.'}
Write-Output 'UI preview ready: http://127.0.0.1:4189/ (Agent and Core asset queries use Core; remaining screens use local synthetic state)'
