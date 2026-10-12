param([Parameter(Mandatory=$true)][string]$PrivateDirectory)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$repo=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$private=(Resolve-Path -LiteralPath $PrivateDirectory).Path
function Docker([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments){$result=& docker.exe @Arguments 2>&1;if($LASTEXITCODE -ne 0){throw "Docker $($Arguments[0]) failed; retain all resources"};($result -join "`n").Trim()}
function Write-Utf8([string]$Path,[string]$Value){[IO.File]::WriteAllText($Path,$Value.Replace("`r`n","`n"),[Text.UTF8Encoding]::new($false))}
$r=Get-Content -LiteralPath (Join-Path $private 'resources.json') -Raw|ConvertFrom-Json
$c=Get-Content -LiteralPath (Join-Path $private 'config.json') -Raw|ConvertFrom-Json
$run=([guid]$r.run_id).ToString();$project='dpp-lifecycle307-'+$run.Substring(0,8)
if($r.project -ne $project -or (Split-Path -Leaf $private) -ne $project -or $c.Enabled -or $c.RunID -ne $run){throw 'Owned completed preparation required'}
$output=Join-Path $PSScriptRoot "results/$run"
$manifest=Join-Path $output 'manifest.json'
$m=Get-Content -LiteralPath $manifest -Raw|ConvertFrom-Json
if($m.workspace_id -ne $c.WorkspaceID -or @($m.objects).Count -ne 4 -or $m.destructive_requests -ne 0){throw 'Fresh unmodified four-object manifest required'}
$worker=Join-Path $private 'worker'
if(Test-Path -LiteralPath $worker){throw 'Recovery already prepared; refuse one-shot replay'}
New-Item -ItemType Directory -Path $worker|Out-Null
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'recovery.Caddyfile') -Destination (Join-Path $private 'Caddyfile')
Docker @('exec',$r.containers.gateway,'caddy','validate','--config','/etc/caddy/Caddyfile','--adapter','caddyfile')|Out-Null
Docker @('restart',$r.containers.gateway)|Out-Null
Docker @('exec',$r.containers.gateway,'wget','-q','-T','10','-O','-','http://127.0.0.1:9000/minio/health/ready')|Out-Null
$redisImage='sha256:858f009f9709ce576febc734aa78b8f6d624b82571f9ddb6bda4377c833b3499'
if((Docker @('image','inspect','redis:7-alpine','--format','{{.Id}}')) -ne $redisImage){throw 'Pinned Redis image mismatch'}
$redis=$project+'-redis';$redisVolume=$project+'-redis-data'
Docker @('volume','create','--label',"lifecycle307.run=$run",$redisVolume)|Out-Null
Docker @('create','--pull','never','--name',$redis,'--label',"lifecycle307.run=$run",'--label',"lifecycle307.project=$project",'--network',$r.network,
 '--read-only','--tmpfs','/tmp','--mount',"type=volume,source=$redisVolume,target=/data",$redisImage,'redis-server','--appendonly','yes','--save','')|Out-Null
Docker @('start',$redis)|Out-Null
if((Docker @('exec',$redis,'redis-cli','PING')) -ne 'PONG'){throw 'Redis unavailable'}
$recovery=@{Enabled=$true;SyntheticOnly=$true;RunID=$c.RunID;WorkspaceID=$c.WorkspaceID;ProviderInstanceID=$c.ProviderInstanceID;IncarnationID=$c.IncarnationID;ActorID=$c.ActorID;Endpoint=$c.Endpoint;InspectAccess=$c.InspectAccess;InspectSecret=$c.InspectSecret;Token=$c.Token;ManifestSHA256=(Get-FileHash -LiteralPath $manifest -Algorithm SHA256).Hash.ToLowerInvariant();ValidUntil=[DateTime]::UtcNow.AddHours(1).ToString('o')}
Write-Utf8 (Join-Path $worker 'recovery.json') ($recovery|ConvertTo-Json)
Write-Utf8 (Join-Path $worker 'dsn') $c.PostgresDSN
Copy-Item -LiteralPath $manifest -Destination (Join-Path $worker 'manifest.json')
$priorOS=$env:GOOS;$priorArch=$env:GOARCH;$priorCGO=$env:CGO_ENABLED
Push-Location (Join-Path $repo 'apps/platform')
try{$env:GOOS='linux';$env:GOARCH='amd64';$env:CGO_ENABLED='0';go build -o (Join-Path $worker 'recovery') ./cmd/lifecycle-fixture-recovery;if($LASTEXITCODE -ne 0){throw 'Linux recovery build failed'}}finally{$env:GOOS=$priorOS;$env:GOARCH=$priorArch;$env:CGO_ENABLED=$priorCGO;Pop-Location}
$redisInfo=(Docker @('inspect',$redis)|ConvertFrom-Json)[0]
$net=(Docker @('network','inspect',$r.network)|ConvertFrom-Json)[0]
if(-not $net.Internal -or @($net.Containers.PSObject.Properties).Count -ne 4){throw 'Closed four-member network required'}
$envProof=@{run_id=$run;workspace_id=$c.WorkspaceID;provider_instance_id=$c.ProviderInstanceID;incarnation_id=$c.IncarnationID;network=@{name=$r.network;id=$net.Id;internal=$true;members=@($net.Containers.PSObject.Properties.Name)};redis=@{name=$redis;id=$redisInfo.Id;image=$redisInfo.Image;volume=$redisVolume;host_ports=@()};retained_volumes=@($r.volumes)+@($redisVolume);source_head=(& git -C $repo rev-parse HEAD).Trim();binary_sha256=(Get-FileHash -LiteralPath (Join-Path $worker 'recovery') -Algorithm SHA256).Hash.ToLowerInvariant();manifest_sha256=$recovery.ManifestSHA256;gateway_sha256=(Get-FileHash -LiteralPath (Join-Path $private 'Caddyfile') -Algorithm SHA256).Hash.ToLowerInvariant();native_delete='DENY_ALL';provider_identity='INSPECT_ONLY';payload_get='DENY_ALL';bootstrap_enabled=$false;cleanup_requests=0}
Write-Utf8 (Join-Path $output 'worker-environment.json') ($envProof|ConvertTo-Json -Depth 12)
Write-Output "Read-only recovery prepared: $project; Redis $redis; all resources retained. DELETE=0."
