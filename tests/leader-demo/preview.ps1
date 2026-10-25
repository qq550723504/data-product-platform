# Isolated, loopback-only leader demo. Never starts a DB, model or business API.
param([switch]$Build,[switch]$Stop,[string]$DistName='.next-acceptance-identity-site-language-done-20261025')
$ErrorActionPreference='Stop'
$repoRoot=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$webRoot=Join-Path $repoRoot 'apps/web'
$runRoot=Join-Path $repoRoot '.data/leader-demo-20261025'
if($DistName -notmatch '^\.next-acceptance-identity-[a-z0-9-]+$'){throw 'Unexpected isolated build directory.'}
$recordPath=Join-Path $runRoot 'processes.json'
if($Stop){
 if(!(Test-Path -LiteralPath $recordPath)){throw 'No owned process record; nothing stopped.'}
 $record=Get-Content -LiteralPath $recordPath -Raw|ConvertFrom-Json
 if($record.repoRoot -ne $repoRoot){throw 'Owner mismatch; nothing stopped.'}
 $owned=@()
 foreach($entry in $record.processes){
  $p=Get-Process -Id $entry.processId -ErrorAction SilentlyContinue
  if(!$p){continue}
  $command=Get-CimInstance Win32_Process -Filter "ProcessId = $($entry.processId)"
  $listener=Get-NetTCPConnection -LocalPort $entry.port -State Listen -ErrorAction SilentlyContinue
  if($p.ProcessName -ne 'node' -or $p.StartTime.ToUniversalTime().Ticks -ne ([DateTimeOffset]$entry.startedAt).UtcDateTime.Ticks -or !$command.CommandLine.Contains($entry.commandMarker) -or !($listener.OwningProcess -contains $entry.processId)){throw 'Exact process identity not verified; nothing stopped.'}
  $owned+=$p
 }
 $owned|Stop-Process
 Write-Output 'Stopped only recorded leader demo processes. All databases and other previews retained.'
 return
}
# Refuse before touching build output used by a running preview.
foreach($port in @(4193,4493)){if(Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue){throw "Port $port occupied; existing processes and builds retained."}}
if($Build){
 $configName='.tsconfig.acceptance-identity-leader.json'
 $envFile=Join-Path $webRoot 'next-env.d.ts'
 $originalEnv=[IO.File]::ReadAllBytes($envFile)
 Copy-Item -LiteralPath (Join-Path $webRoot 'tsconfig.json') -Destination (Join-Path $webRoot $configName) -Force
 $previous=@{}
 try{
  foreach($pair in @{NEXT_DIST_DIR=$DistName;NEXT_TSCONFIG_PATH=$configName;NEXT_TELEMETRY_DISABLED='1'}.GetEnumerator()){$previous[$pair.Key]=[Environment]::GetEnvironmentVariable($pair.Key,'Process');[Environment]::SetEnvironmentVariable($pair.Key,$pair.Value,'Process')}
  Push-Location $webRoot
  try{& node node_modules/next/dist/bin/next build;if($LASTEXITCODE -ne 0){throw 'Isolated build failed; no preview started.'}}finally{Pop-Location}
 }finally{[IO.File]::WriteAllBytes($envFile,$originalEnv);foreach($name in $previous.Keys){[Environment]::SetEnvironmentVariable($name,$previous[$name],'Process')}}
}
if(!(Test-Path -LiteralPath (Join-Path $webRoot "$DistName/BUILD_ID"))){throw 'Build the isolated web first with -Build.'}
$standalone=Join-Path $webRoot "$DistName/standalone"
if(!(Test-Path -LiteralPath (Join-Path $standalone 'server.js'))){throw 'Isolated standalone output missing.'}
Copy-Item -LiteralPath (Join-Path $webRoot "$DistName/static") -Destination (Join-Path $standalone $DistName) -Recurse -Force
if(Test-Path -LiteralPath (Join-Path $webRoot 'public')){Copy-Item -LiteralPath (Join-Path $webRoot 'public') -Destination $standalone -Recurse -Force}
New-Item -ItemType Directory -Path $runRoot -Force|Out-Null
$nodePath=(Get-Command node -CommandType Application|Select-Object -First 1).Source
$settings=@{NEXT_DIST_DIR=$DistName;PLATFORM_API_BASE_URL='http://127.0.0.1:4493';POC_WORKSPACE_ID='11111111-1111-4111-8111-111111111111';HUMAN_DECISION_API_TOKEN='review-secret';NEXT_TELEMETRY_DISABLED='1';AGENT_TASKS_UI_WRITE_ENABLED='false';AGENT_TASKS_WRITE_ENABLED='false';GOVERNANCE_PREVIEW_MODE='synthetic-readonly';BROWSER_IAM_MODE='off';HOSTNAME='127.0.0.1';PORT='4193'}
foreach($name in @('REVIEW','RELEASE','EXECUTION','DATASET','QUALITY','COMPLIANCE','DELIVERY','INGEST','INGESTION')){$settings["POC_ENABLE_${name}_ACTIONS"]='false'}
$previous=@{};$entries=@()
try{
 # Do not inherit configured business/identity/provider sources into this demo.
 foreach($entry in Get-ChildItem Env:|Where-Object {$_.Name -match '^(POC_|BROWSER_|AGENT_|PLATFORM_|HUMAN_DECISION_|DELIVERY_|SOURCE_ADMISSION_|GOVERNANCE_|OPENAI_|ANTHROPIC_|DASHSCOPE_)'}){$previous[$entry.Name]=$entry.Value;[Environment]::SetEnvironmentVariable($entry.Name,$null,'Process')}
 foreach($name in $settings.Keys){if(!$previous.ContainsKey($name)){$previous[$name]=[Environment]::GetEnvironmentVariable($name,'Process')};[Environment]::SetEnvironmentVariable($name,$settings[$name],'Process')}
 foreach($spec in @(@{name='fixture';args=@('tests/leader-demo/readonly-fixture.mjs');cwd=$repoRoot;port=4493;marker='tests/leader-demo/readonly-fixture.mjs'},@{name='web';args=@((Join-Path $standalone 'server.js'));cwd=$standalone;port=4193;marker=(Join-Path $standalone 'server.js')})){
  $p=Start-Process -FilePath $nodePath -ArgumentList $spec.args -WorkingDirectory $spec.cwd -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $runRoot "$($spec.name).stdout.log") -RedirectStandardError (Join-Path $runRoot "$($spec.name).stderr.log")
  $entries+=@{processId=$p.Id;port=$spec.port;startedAt=$p.StartTime.ToUniversalTime().ToString('o');commandMarker=$spec.marker}
  @{repoRoot=$repoRoot;dist=$DistName;processes=$entries}|ConvertTo-Json -Depth 5|Set-Content -LiteralPath $recordPath
 }
}finally{foreach($name in $previous.Keys){[Environment]::SetEnvironmentVariable($name,$previous[$name],'Process')}}
$deadline=[DateTime]::UtcNow.AddSeconds(25)
do{try{$ready=(Invoke-WebRequest 'http://127.0.0.1:4193/' -TimeoutSec 2 -UseBasicParsing).StatusCode -eq 200}catch{$ready=$false};if(!$ready){Start-Sleep -Milliseconds 300}}while(!$ready -and [DateTime]::UtcNow -lt $deadline)
if(!$ready){throw 'Preview not ready; keep process record and logs. No automatic restart.'}
Write-Output 'Leader demo: http://127.0.0.1:4193/ (synthetic, API GET only, business writes/model/dispatch disabled)'
