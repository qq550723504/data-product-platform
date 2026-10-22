param([Parameter(Mandatory=$true)][string]$PrivateDirectory,[Parameter(Mandatory=$true)][ValidateSet('seed','review','witness','survivors','outbox','request','enqueue','scan','once','worker')][string]$Mode,[Parameter(Mandatory=$true)][string]$Scenario,[string]$WorkID='00000000-0000-0000-0000-000000000000',[string]$AttemptID='00000000-0000-0000-0000-000000000000',[switch]$RedisUnavailable,[switch]$StartOnly)
$ErrorActionPreference='Stop';Set-StrictMode -Version Latest
if($Scenario -notmatch '^[a-z][a-z0-9-]{0,63}$'){throw 'Finite step label required'}
$repo=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path;$private=(Resolve-Path -LiteralPath $PrivateDirectory).Path;$worker=Join-Path $private 'worker'
function Docker([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments){$result=& docker.exe @Arguments 2>&1;if($LASTEXITCODE -ne 0){throw "Docker $($Arguments[0]) failed; retain all resources"};($result -join "`n").Trim()}
function Write-Utf8([string]$Path,[string]$Value){[IO.File]::WriteAllText($Path,$Value.Replace("`r`n","`n"),[Text.UTF8Encoding]::new($false))}
$r=Get-Content -LiteralPath (Join-Path $private 'resources.json') -Raw|ConvertFrom-Json;$c=Get-Content -LiteralPath (Join-Path $private 'config.json') -Raw|ConvertFrom-Json;$p=Get-Content -LiteralPath (Join-Path $worker 'recovery.json') -Raw|ConvertFrom-Json
$run=([guid]$r.run_id).ToString();$project='dpp-lifecycle307-'+$run.Substring(0,8);$name="$project-recovery-$Scenario";$output=Join-Path $PSScriptRoot "results/$run"
if($r.project -ne $project -or $c.Enabled -or $p.RunID -ne $run -or $p.WorkspaceID -ne $c.WorkspaceID -or $p.ProviderInstanceID -ne $c.ProviderInstanceID -or $p.IncarnationID -ne $c.IncarnationID){throw 'Owned sealed window required'}
$m=Get-Content -LiteralPath (Join-Path $worker 'manifest.json') -Raw|ConvertFrom-Json;$a=@($m.objects|Where-Object {$_.namespace -eq 'dpp-lifecycle307-online' -and $_.key -eq 'fixture/a.csv'})[0]
if($Mode -in @('review','survivors')){$a=@($m.objects|Where-Object {$_.namespace -eq 'dpp-lifecycle307-quarantine' -and $_.key -eq 'fixture/b.csv'})[0]}
$redis="$project-redis:6379";if($RedisUnavailable){$redis='127.0.0.1:6399'}
$step=@{Mode=$Mode;Scenario=$Scenario;PostgresDSN=$c.PostgresDSN;RedisAddr=$redis;RecoveryConfig='/run/private/recovery.json';Manifest='/run/private/manifest.json';OutputDirectory="/run/output/$run";Barrier="/run/output/$run/barrier-$Scenario.json";SeedEnabled=($Mode -eq 'seed');WorkspaceID=$c.WorkspaceID;ActorID=$c.ActorID;CopyID=$a.copy_id;WorkID=([guid]$WorkID).ToString();AttemptID=([guid]$AttemptID).ToString();ExpiresAt=[DateTime]::UtcNow.AddMinutes(20).ToString('o')}
$configPath=Join-Path $worker "$Scenario.json";if(Test-Path -LiteralPath $configPath){throw 'One-shot step already configured; inspect retained facts'}
Write-Utf8 $configPath ($step|ConvertTo-Json)
$binary='recovery';if($Mode -eq 'review'){$binary='recovery-review'}
if($Mode -eq 'survivors'){$binary='recovery-survivors'}
Write-Utf8 (Join-Path $worker "$Scenario.sh") ("#!/bin/sh`nset -eu`nexport LIFECYCLE_RECOVERY_ACCEPTANCE_CONFIG=/run/private/$Scenario.json`nexec /run/private/$binary`n")
$gw=(Docker @('inspect',$r.containers.gateway)|ConvertFrom-Json)[0]
$pg=(Docker @('inspect',$r.containers.postgres)|ConvertFrom-Json)[0]
$binaryHash=(Get-FileHash -LiteralPath (Join-Path $worker $binary) -Algorithm SHA256).Hash.ToLowerInvariant()
$proof=@{name=$name;mode=$Mode;scenario=$Scenario;source_head=(& git -C $repo rev-parse HEAD).Trim();binary_sha256=$binaryHash;gateway_sha256=(Get-FileHash -LiteralPath (Join-Path $private 'Caddyfile') -Algorithm SHA256).Hash.ToLowerInvariant();workspace_id=$c.WorkspaceID;work_id=$WorkID;attempt_id=$AttemptID;native_delete='DENY_ALL';cleanup_requests=0}
Docker @('create','--pull','never','--name',$name,'--label',"lifecycle307.run=$run",'--network',"container:$($r.containers.gateway)",'--read-only','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data',
 '--mount',"type=bind,source=$worker,target=/run/private,readonly",'--mount',"type=bind,source=$(Join-Path $PSScriptRoot 'results'),target=/run/output",'--entrypoint','/bin/sh',$pg.Image,"/run/private/$Scenario.sh")|Out-Null
$proof.container_id=Docker @('inspect',$name,'--format','{{.Id}}')
Write-Utf8 (Join-Path $output "provenance-$Scenario.json") ($proof|ConvertTo-Json -Depth 8)
Docker @('start',$name)|Out-Null
Write-Output "Started $name (read-only native recovery, DELETE=0)."
if(-not $StartOnly){$exit=Docker @('wait',$name);if($Mode -eq 'seed'){$step.SeedEnabled=$false;Write-Utf8 $configPath ($step|ConvertTo-Json)};Write-Output "Exit=$exit; retain objects, containers and volumes.";if($exit -ne '0' -and $exit -ne '91' -and $exit -ne '92'){throw 'Step failed; no automatic retry or cleanup'}}
