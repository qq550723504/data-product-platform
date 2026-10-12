param([Parameter(Mandatory=$true)][string]$PrivateDirectory)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$repo=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$private=(Resolve-Path -LiteralPath $PrivateDirectory).Path
function Docker([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments){$v=& docker.exe @Arguments 2>&1;if($LASTEXITCODE -ne 0){throw "Docker $($Arguments[0]) failed; all resources retained"};($v -join "`n").Trim()}
function Write-Utf8([string]$Path,[string]$Value){[IO.File]::WriteAllText($Path,$Value.Replace("`r`n","`n"),[Text.UTF8Encoding]::new($false))}
function New-Secret{$b=[byte[]]::new(32);[Security.Cryptography.RandomNumberGenerator]::Fill($b);[Convert]::ToHexString($b).ToLowerInvariant()}
$r=Get-Content -LiteralPath (Join-Path $private 'resources.json') -Raw|ConvertFrom-Json
$c=Get-Content -LiteralPath (Join-Path $private 'config.json') -Raw|ConvertFrom-Json
$run=([guid]$r.run_id).ToString();$project='dpp-lifecycle307-'+$run.Substring(0,8)
$output=Join-Path $PSScriptRoot "results/$run";$manifest=Join-Path $output 'manifest.json';$m=Get-Content -LiteralPath $manifest -Raw|ConvertFrom-Json
if($private.StartsWith($repo+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase) -or $r.project -ne $project -or (Split-Path -Leaf $private) -ne $project -or $c.Enabled -or -not $c.ArchiveAcceptance -or $m.environment.migrations_through -ne 64 -or $m.environment.base_main -ne '1892cd76c4ee578896ddfb3c6f4a7252bfacc51b' -or @($m.objects).Count -ne 4 -or $m.destructive_requests -ne 0){throw 'Fresh completed isolated schema64 fixture required'}
$net=(Docker @('network','inspect',$r.network)|ConvertFrom-Json)[0]
if(-not $net.Internal -or @($net.Containers.PSObject.Properties).Count -ne 3 -or $net.Labels.'lifecycle307.run' -ne $run){throw 'Closed three-member network required'}
foreach($name in $r.containers.PSObject.Properties.Value){$i=(Docker @('inspect',$name)|ConvertFrom-Json)[0];if($i.State.Status -ne 'running' -or $i.Config.Labels.'lifecycle307.run' -ne $run){throw 'Service owner/state mismatch'};foreach($p in @($i.NetworkSettings.Ports.PSObject.Properties)){if($null -ne $p.Value){throw 'Reader native fixture cannot publish ports'}}}
$admin=Join-Path $private 'reader-admin';$owner=Join-Path $private 'reader-owner'
. (Join-Path $PSScriptRoot 'durable-storage.ps1')
Assert-FixtureDurableStorage $r ((Docker @('inspect',$r.containers.postgres)|ConvertFrom-Json)[0]) ((Docker @('inspect',$r.containers.minio)|ConvertFrom-Json)[0])|Out-Null
foreach($dir in @($admin,$owner)){if(Test-Path -LiteralPath $dir){throw 'Reader already prepared; do not repeat provisioning'};New-Item -ItemType Directory -Path $dir|Out-Null}
$priorOS=$env:GOOS;$priorArch=$env:GOARCH;$priorCGO=$env:CGO_ENABLED
Push-Location (Join-Path $repo 'apps/platform')
try{$env:GOOS='linux';$env:GOARCH='amd64';$env:CGO_ENABLED='0';go build -o (Join-Path $owner 'reader') ./cmd/lifecycle-fixture-reader;if($LASTEXITCODE -ne 0){throw 'Reader Linux build failed'}}finally{$env:GOOS=$priorOS;$env:GOARCH=$priorArch;$env:CGO_ENABLED=$priorCGO;Pop-Location}
Copy-Item -LiteralPath (Join-Path $owner 'reader') -Destination (Join-Path $admin 'reader')
$reader=@{RunID=$run;WorkspaceID=$c.WorkspaceID;ProviderInstanceID=$c.ProviderInstanceID;IncarnationID=$c.IncarnationID;ActorID=$c.ActorID;Epoch=[guid]::NewGuid().ToString();Enabled=$true;StartedAt=[DateTime]::UtcNow.ToString('o');ExpiresAt=[DateTime]::UtcNow.AddMinutes(45).ToString('o');Endpoint=$c.Endpoint;PostgresDSN=$c.PostgresDSN;Token=$c.Token;Access='reader'+$run.Replace('-','').Substring(0,12);Secret=New-Secret;InspectAccess=$c.InspectAccess;InspectSecret=$c.InspectSecret;OutputDirectory=$c.OutputDirectory;ManifestSHA256=(Get-FileHash -Algorithm SHA256 -LiteralPath $manifest).Hash.ToLowerInvariant();RootAccess='';RootSecret=''}
Write-Utf8 (Join-Path $owner 'reader.json') ($reader|ConvertTo-Json -Depth 8)
$root=$reader.Clone();$root.RootAccess=$c.RootAccess;$root.RootSecret=$c.RootSecret;Write-Utf8 (Join-Path $admin 'reader.json') ($root|ConvertTo-Json -Depth 8)
foreach($dir in @($admin,$owner)){Copy-Item -LiteralPath $manifest -Destination (Join-Path $dir 'manifest.json')}
$gateway=[IO.File]::ReadAllText((Join-Path $PSScriptRoot 'reader.Caddyfile')).Replace('__ROOT_ACCESS__',$c.RootAccess).Replace('__READER_ACCESS__',$reader.Access).Replace('__INSPECT_ACCESS__',$c.InspectAccess).Replace('__A_VERSION__',$m.objects[0].version_id)
Write-Utf8 (Join-Path $private 'Caddyfile') $gateway
Docker @('exec',$r.containers.gateway,'caddy','validate','--config','/etc/caddy/Caddyfile','--adapter','caddyfile')|Out-Null
Docker @('restart',$r.containers.gateway)|Out-Null
$sources=@{};foreach($path in @('migrations/000064_lifecycle_reconciliation_work.up.sql','apps/platform/internal/lifecycle/application/reader_service.go','apps/platform/internal/lifecycle/application/occupancy.go','apps/platform/internal/lifecycle/transport/http/reader.go','apps/platform/internal/platform/storage/lifecycle_copy.go','apps/platform/internal/platform/storage/lifecycle_reader.go','apps/platform/cmd/lifecycle-fixture-reader/main.go')){$sources[$path]=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $repo $path)).Hash.ToLowerInvariant()}
Write-Utf8 (Join-Path $output 'reader-environment.json') (@{run_id=$run;project=$project;workspace_id=$c.WorkspaceID;provider_instance_id=$c.ProviderInstanceID;incarnation_id=$c.IncarnationID;source_head=(& git -C $repo rev-parse HEAD).Trim();source_hashes=$sources;binary_sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $owner 'reader')).Hash.ToLowerInvariant();network_id=$net.Id;internal=$true;host_ports=@();http='ephemeral loopback only inside one-shot acceptance helper';objects=@($m.objects);retained_volumes=@($r.volumes);root_in_reader=$false;window_expires=$reader.ExpiresAt;delete=0;copy=0;cleanup=0;production_delivery='NOT_RUN'}|ConvertTo-Json -Depth 12)
& (Join-Path $PSScriptRoot 'run-reader-step.ps1') -PrivateDirectory $private -Mode provision
Write-Output "Prepared $project closed reader acceptance; no deletion authority or cleanup."
