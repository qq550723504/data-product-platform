# Refresh only the recorded UI process. Retain API, gateway and database.
$ErrorActionPreference='Stop'
$repoRoot=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$runRoot=Join-Path $repoRoot '.data/agent-q2-preview'
$recordPath=Join-Path $runRoot 'processes.json'
$record=Get-Content -LiteralPath $recordPath -Raw|ConvertFrom-Json
if($record.repoRoot -ne $repoRoot){throw 'Runtime owner mismatch'}
$item=@($record.processes|Where-Object port -eq 4188)
if($item.Count -ne 1){throw 'Exactly one owned UI process required'}
$process=Get-Process -Id $item[0].processId -ErrorAction Stop
$listener=Get-NetTCPConnection -LocalPort 4188 -State Listen -ErrorAction Stop
if($process.ProcessName -ne $item[0].name -or [Math]::Abs(($process.StartTime.ToUniversalTime()-([DateTimeOffset]$item[0].startedAt).UtcDateTime).TotalSeconds)-gt 1 -or -not($listener.OwningProcess -contains $process.Id)){throw 'UI identity/port mismatch; nothing stopped'}
Stop-Process -Id $process.Id
& (Join-Path $PSScriptRoot 'build-web.ps1')
$standalone=Join-Path $repoRoot 'apps/web/.next-agent-q2/standalone'
Copy-Item -LiteralPath (Join-Path $repoRoot 'apps/web/.next-agent-q2/static') -Destination (Join-Path $standalone '.next-agent-q2') -Recurse -Force
Copy-Item -LiteralPath (Join-Path $repoRoot 'apps/web/public') -Destination $standalone -Recurse -Force
$settings=@{PLATFORM_API_BASE_URL='http://127.0.0.1:18497';POC_WORKSPACE_ID='11111111-1111-4111-8111-111111111111';HUMAN_DECISION_API_TOKEN='agent-q2-core-only';AGENT_TASKS_UI_WRITE_ENABLED='false';HOSTNAME='127.0.0.1';PORT='4188';NEXT_TELEMETRY_DISABLED='1'}
foreach($flag in @('REVIEW','RELEASE','EXECUTION','DATASET','QUALITY','COMPLIANCE','DELIVERY','INGEST','INGESTION')){$settings['POC_ENABLE_'+$flag+'_ACTIONS']='false'}
$previous=@{}
try{
 foreach($name in $settings.Keys){$previous[$name]=[Environment]::GetEnvironmentVariable($name,'Process');[Environment]::SetEnvironmentVariable($name,$settings[$name],'Process')}
 $nodePath=(Get-Command node -CommandType Application|Select-Object -First 1).Source
 $web=Start-Process -FilePath $nodePath -ArgumentList @((Join-Path $standalone 'server.js')) -WorkingDirectory $standalone -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $runRoot 'web.stdout.log') -RedirectStandardError (Join-Path $runRoot 'web.stderr.log')
 $record.processes=@($record.processes|Where-Object port -ne 4188)+@(@{processId=$web.Id;name=$web.ProcessName;port=4188;startedAt=$web.StartTime.ToUniversalTime().ToString('o')})
 $record|ConvertTo-Json -Depth 5|Set-Content -LiteralPath $recordPath
}finally{foreach($name in $previous.Keys){[Environment]::SetEnvironmentVariable($name,$previous[$name],'Process')}}
Write-Output 'Owned UI refreshed. Existing API, gateway and database retained.'
