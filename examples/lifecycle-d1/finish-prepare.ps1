# Completes the owned empty environment; never retries provider writes or cleans.
param([Parameter(Mandatory=$true)][string]$PrivateDirectory,[switch]$ArchiveAcceptance,[switch]$ProcessingDeliveryAcceptance,[string]$MinioImage='')
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if($ProcessingDeliveryAcceptance -and -not $ArchiveAcceptance){throw 'Processing/delivery slice requires exact-version native fixture mode'}
$repo=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$private=(Resolve-Path -LiteralPath $PrivateDirectory).Path
function Docker([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments) {
 $result=& docker.exe @Arguments 2>&1
 if ($LASTEXITCODE -ne 0) { throw "Docker $($Arguments[0]) failed; all resources retained" }
 return ($result -join "`n").Trim()
}
function Write-Utf8([string]$Path,[string]$Value){[IO.File]::WriteAllText($Path,$Value.Replace("`r`n","`n"),[Text.UTF8Encoding]::new($false))}
function New-Secret {$b=[byte[]]::new(32);[Security.Cryptography.RandomNumberGenerator]::Fill($b);[Convert]::ToHexString($b).ToLowerInvariant()}
function Inspect([string]$Name){(Docker @('inspect',$Name)|ConvertFrom-Json)[0]}
$r=Get-Content -LiteralPath (Join-Path $private 'resources.json') -Raw|ConvertFrom-Json
$run=([guid]$r.run_id).ToString()
$project='dpp-lifecycle307-'+$run.Substring(0,8)
if ($r.project -ne $project -or (Split-Path -Leaf $private) -ne $project) {throw 'Owned run identity mismatch'}
if (Test-Path -LiteralPath (Join-Path $private 'config.json')) {throw 'Preparation already configured; do not repeat writes. Inspect retained facts instead.'}
$pg=Inspect $r.containers.postgres
$mi=Inspect $r.containers.minio
$gw=Inspect $r.containers.gateway
foreach ($item in @($pg,$mi,$gw)){
 if ($item.Config.Labels.'lifecycle307.run' -ne $run -or $item.State.Status -ne 'running' -or @($item.NetworkSettings.Networks.PSObject.Properties).Count -ne 1) {throw 'Container ownership/state/network mismatch'}
 foreach($binding in @($item.NetworkSettings.Ports.PSObject.Properties)){if($null -ne $binding.Value -and @($binding.Value).Count -ne 0){throw 'Closed fixture must have no active host port'}}
}
$net=(Docker @('network','inspect',$r.network)|ConvertFrom-Json)[0]
if (-not $net.Internal -or @($net.Containers.PSObject.Properties).Count -ne 3 -or $net.Labels.'lifecycle307.run' -ne $run){throw 'Closed network scope mismatch'}
foreach($volume in $r.volumes){$v=(Docker @('volume','inspect',$volume)|ConvertFrom-Json)[0];if($v.Labels.'lifecycle307.run' -ne $run){throw 'Volume owner mismatch'}}
$expectedMinio = 'sha256:a9eb76492485dd496bfd69b4114e47137f2da1a1a87e4a9b273cb21476cb1a98'
if ($ArchiveAcceptance) { $expectedMinio = 'sha256:4d2b20cfce457652e6be8c5038d90704a99fdf0160b7e775d968f9163e96a741' }
if($MinioImage -ne ''){if(-not $ArchiveAcceptance -or $MinioImage -notmatch '^sha256:[a-f0-9]{64}$'){throw 'Explicit pinned fixture image required'};$expectedMinio=$MinioImage}
if ($mi.Image -ne $expectedMinio -or $pg.Image -ne 'sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea' -or $gw.Image -ne 'sha256:d8542f48d34a9cf4e4c11a478865229840e87e4c96ea3f439101f31a5d35f75f'){throw 'Pinned image mismatch'}
$binaryHash=(Docker @('exec',$r.containers.minio,'sha256sum','/usr/local/bin/minio')).Split(' ')[0]
if($binaryHash -ne 'fb741388b404ec67c4f3777b812d6174ba5562532acb17fb1ddfba9a4e6249b8'){throw 'Pinned binary mismatch'}
$nativeVersion=Docker @('exec',$r.containers.minio,'/usr/local/bin/minio','--version')
$rootAccess=[IO.File]::ReadAllText((Join-Path $private 'access'))
$caddy=[IO.File]::ReadAllText((Join-Path $PSScriptRoot 'storage.Caddyfile')).Replace('__BOOTSTRAP_ACCESS__',$rootAccess)
Write-Utf8 (Join-Path $private 'Caddyfile') $caddy
Docker @('exec',$r.containers.gateway,'caddy','validate','--config','/etc/caddy/Caddyfile','--adapter','caddyfile')|Out-Null
# Stop/start changes only this gateway process; volumes and native objects remain.
Docker @('restart',$r.containers.gateway)|Out-Null
Docker @('exec',$r.containers.gateway,'wget','-T','10','-q','-O','-','http://127.0.0.1:9000/minio/health/ready')|Out-Null
$results=Join-Path $PSScriptRoot 'results'
New-Item -ItemType Directory -Path $results -Force|Out-Null
$output=Join-Path $results $run
if(Test-Path -LiteralPath $output){throw 'Public output already exists; refuse one-shot replay'}
$helper=$project+'-prepare'
$environment=@{
 project=$project;network=@{name=$r.network;id=$net.Id;internal=$true;members=@($r.containers.PSObject.Properties.Value)}
 volumes=@($r.volumes);host_ports=@();gateway='127.0.0.1:9000 within gateway network namespace only';postgres_endpoint=$r.containers.postgres+':5432 within private network only'
 sdk='minio-go/v7 v7.0.83';admin_sdk='madmin-go/v3 v3.0.94';minio_binary_sha256=$binaryHash;minio_version_output=$nativeVersion
 source_commit='9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a';license='AGPL-3.0';gateway_delete='DENY_ALL'
 gateway_config_sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $private 'Caddyfile')).Hash.ToLowerInvariant()
 preservation='Named PostgreSQL/object volumes retained; no cleanup on failure/success';preparation_helper=$helper
 containers=@(@{name=$r.containers.postgres;id=$pg.Id;image=$pg.Image;requested_port_bindings=$pg.HostConfig.PortBindings},@{name=$r.containers.minio;id=$mi.Id;image=$mi.Image;requested_port_bindings=$mi.HostConfig.PortBindings},@{name=$r.containers.gateway;id=$gw.Id;image=$gw.Image;requested_port_bindings=$gw.HostConfig.PortBindings})
 base_main=(& git -C $repo merge-base HEAD origin/main).Trim()
 source_head=(& git -C $repo rev-parse HEAD).Trim()
 preparation_source_sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $repo 'apps/platform/cmd/lifecycle-fixture-prepare/main.go')).Hash.ToLowerInvariant()
 storage_inspector_source_sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $repo 'apps/platform/internal/platform/storage/lifecycle_inspection.go')).Hash.ToLowerInvariant()
}
$pgSecret=[IO.File]::ReadAllText((Join-Path $private 'pgpass'))
$config=@{
 RunID=$run;WorkspaceID=[guid]::NewGuid().ToString();ProviderInstanceID=[guid]::NewGuid().ToString();IncarnationID=[guid]::NewGuid().ToString();ActorID=[guid]::NewGuid().ToString()
 Enabled=$true;ArchiveAcceptance=[bool]$ArchiveAcceptance;ProcessingDeliveryAcceptance=[bool]$ProcessingDeliveryAcceptance;Endpoint='127.0.0.1:9000';PostgresDSN="postgres://fixture:${pgSecret}@$($r.containers.postgres):5432/lifecycle307?sslmode=disable"
 RootAccess=$rootAccess;RootSecret=[IO.File]::ReadAllText((Join-Path $private 'secret'));InspectAccess='inspect'+$run.Replace('-','').Substring(0,12);InspectSecret=(New-Secret);Token=(New-Secret)
 OutputDirectory="/run/output/$run";Environment=$environment
}
# Build before marking this run configured; binaries and credentials remain private.
$priorOS=$env:GOOS;$priorArch=$env:GOARCH;$priorCGO=$env:CGO_ENABLED
Push-Location (Join-Path $repo 'apps/platform')
try{
 $env:GOOS='linux';$env:GOARCH='amd64';$env:CGO_ENABLED='0'
 & go build -o (Join-Path $private 'migrate') ./cmd/migrate
 if($LASTEXITCODE -ne 0){throw 'Linux migration build failed'}
 & go build -o (Join-Path $private 'prepare') ./cmd/lifecycle-fixture-prepare
 if($LASTEXITCODE -ne 0){throw 'Linux fixture build failed'}
}finally{$env:GOOS=$priorOS;$env:GOARCH=$priorArch;$env:CGO_ENABLED=$priorCGO;Pop-Location}
Copy-Item -LiteralPath (Join-Path $repo 'migrations') -Destination (Join-Path $private 'migrations') -Recurse
$environment.migrations_through=([int]((Get-ChildItem -LiteralPath (Join-Path $private 'migrations') -Filter '*.up.sql'|Sort-Object Name|Select-Object -Last 1).Name.Split('_')[0]))
Write-Utf8 (Join-Path $private 'config.json') ($config|ConvertTo-Json -Depth 16)
Write-Utf8 (Join-Path $private 'dsn') $config.PostgresDSN
Write-Utf8 (Join-Path $private 'prepare.sh') @'
#!/bin/sh
set -eu
if [ -f /run/output/__RUN__/manifest.json ]; then
  echo 'Fixture manifest finalized; explicit new action required. No I/O performed.'
  exit 1
fi
export POSTGRES_DSN=$(cat /run/private/dsn)
timeout 90 /run/private/migrate -direction up -dir /run/private/migrations
export LIFECYCLE_FIXTURE_CONFIG=/run/private/config.json
exec /run/private/prepare
'@.Replace('__RUN__',$run)
Docker @('create','--pull','never','--name',$helper,'--label',"lifecycle307.run=$run",'--network',"container:$($r.containers.gateway)",
 '--read-only','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data',
 '--mount',"type=bind,source=$private,target=/run/private,readonly",'--mount',"type=bind,source=$results,target=/run/output",
 '--mount',"type=bind,source=$(Join-Path $repo 'migrations'),target=/run/migrations,readonly",'--entrypoint','/bin/sh',$pg.Image,'/run/private/prepare.sh')|Out-Null
$helperID=Docker @('inspect',$helper,'--format','{{.Id}}')
Write-Utf8 (Join-Path $private 'helper.json') (@{name=$helper;id=$helperID;image=$pg.Image;network_mode='container:'+$gw.Id;binary_sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $private 'prepare')).Hash.ToLowerInvariant()}|ConvertTo-Json)
$environment.preparation_container_id=$helperID
$environment.preparation_binary_sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $private 'prepare')).Hash.ToLowerInvariant()
Write-Utf8 (Join-Path $private 'config.json') ($config|ConvertTo-Json -Depth 16)
Docker @('start',$helper)|Out-Null
Write-Output "Preparation started inside closed run $run. Helper: $helper. No automatic cleanup."
Write-Output "After successful completion, public manifest: $(Join-Path $output 'manifest.json')"
# Wait is in the caller's resumable exec session; no provider operation is retried.
$exit=Docker @('wait',$helper)
if($exit -ne '0'){throw 'Preparation failed. Preserve facts/objects and inspect receipts; no automatic retry or cleanup.'}
$config.Enabled=$false
Write-Utf8 (Join-Path $private 'config.json') ($config|ConvertTo-Json -Depth 16)
Write-Output 'Manifest complete; fixture principal disabled pending user confirmation. DELETE=0; cleanup=0.'
