param([switch]$HumanConfirmedExactVersion)
$ErrorActionPreference='Stop';Set-StrictMode -Version Latest
if(-not $HumanConfirmedExactVersion){throw 'Explicit human confirmation of the 4abcfca5 quarantine/b exact version is required'}
$repo=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$private='C:\Users\Henry\.codex\worktrees\issue307-lifecycle-d1\private-fixtures\dpp-lifecycle307-4abcfca5'
$run='4abcfca5-583d-41ca-8548-10a0fc3270a1';$project='dpp-lifecycle307-4abcfca5'
$results=Join-Path $PSScriptRoot 'results';$output=Join-Path $results $run
function Docker([string[]]$Arguments){$value=& docker.exe @Arguments 2>&1;if($LASTEXITCODE -ne 0){throw "Docker $($Arguments[0]) failed; retain all resources"};return ($value -join "`n").Trim()}
function Write-Utf8([string]$Path,[string]$Value){[IO.File]::WriteAllText($Path,$Value.Replace("`r`n","`n"),[Text.UTF8Encoding]::new($false))}
function Hash([string]$Path){(Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()}
function Secret{$bytes=[byte[]]::new(32);[Security.Cryptography.RandomNumberGenerator]::Fill($bytes);[Convert]::ToHexString($bytes).ToLowerInvariant()}
if(Test-Path -LiteralPath (Join-Path $private 'delete-permission.json')){throw 'One-shot scope already configured; recovery only, never resend'}
$r=Get-Content -LiteralPath (Join-Path $private 'resources.json') -Raw|ConvertFrom-Json
$c=Get-Content -LiteralPath (Join-Path $private 'config.json') -Raw|ConvertFrom-Json
$recovery=Get-Content -LiteralPath (Join-Path $private 'worker/recovery.json') -Raw|ConvertFrom-Json
$prior=Get-Content -LiteralPath (Join-Path $output 'worker-environment.json') -Raw|ConvertFrom-Json
$m=Get-Content -LiteralPath (Join-Path $output 'manifest.json') -Raw|ConvertFrom-Json
$review=Get-Content -LiteralPath (Join-Path $output 'pending-delete-plan.json') -Raw|ConvertFrom-Json
if($r.project -ne $project -or $c.RunID -ne $run -or $c.WorkspaceID -ne 'd9deb37f-0e58-4f00-830e-c7aad21895f8' -or $c.Enabled -or $recovery.Enabled -or $c.ProviderInstanceID -ne $prior.provider_instance_id -or $c.IncarnationID -ne $prior.incarnation_id -or $review.plan_id -ne '872ca588-6856-420f-b450-e5cd4e8008f4' -or @($review.targets).Count -ne 1 -or $review.targets[0].ProviderVersion -ne 'ce87f789-a4f4-4ce7-bf1f-a044ac53d1d2' -or (Hash (Join-Path $output 'manifest.json')) -ne $prior.manifest_sha256){throw 'Pinned sealed scope/manifest mismatch'}
$ids=@{};$pins=@{}
foreach($item in $m.environment.containers){$role=$item.name.Substring($project.Length+1);$ids[$role]=$item.id;$pins[$role]=$item.image}
$ids.redis=$prior.redis.id;$pins.redis=$prior.redis.image
$topology=@()
foreach($role in @('postgres','minio','gateway','redis')){
 $name="$project-$role";$i=(Docker @('inspect',$name)|ConvertFrom-Json)[0]
 if($i.Id -ne $ids[$role] -or $i.Image -ne $pins[$role] -or $i.State.Status -ne 'running' -or $i.Config.Labels.'lifecycle307.run' -ne $run -or @($i.NetworkSettings.Networks.PSObject.Properties).Count -ne 1){throw 'Owned exact container mismatch'}
 foreach($port in @($i.NetworkSettings.Ports.PSObject.Properties)){if($null -ne $port.Value -and @($port.Value).Count -gt 0){throw 'Published fixture host port'}}
 $topology+=@{role=$role;id=$i.Id;image=$i.Image;mounts=@($i.Mounts|Select-Object Type,Name,Destination);networks=$i.NetworkSettings.Networks;ports=$i.NetworkSettings.Ports}
}
$members=@(Docker @('ps','-q','--filter',"label=lifecycle307.run=$run")) -join "`n"
if(@($members.Split("`n")|Where-Object {$_}).Count -ne 4){throw 'An acceptance worker/helper is running; no delete admission'}
$network=(Docker @('network','inspect',$r.network)|ConvertFrom-Json)[0]
if($network.Id -ne $prior.network.id -or -not $network.Internal -or @($network.Containers.PSObject.Properties).Count -ne 4){throw 'Closed network identity/membership mismatch'}
foreach($member in $network.Containers.PSObject.Properties){if($member.Name -notin @($ids.Values)){throw 'Unexpected network member'}}
if((Docker @('exec',$r.containers.minio,'sha256sum','/usr/local/bin/minio')).Split(' ')[0] -ne $m.environment.minio_binary_sha256){throw 'Provider binary changed'}
foreach($vname in $prior.retained_volumes){$v=(Docker @('volume','inspect',$vname)|ConvertFrom-Json)[0];if($v.Labels.'lifecycle307.run' -ne $run){throw 'Volume owner mismatch'}}
if((Docker @('exec',$r.containers.postgres,'psql','-U','fixture','-d','lifecycle307','-Atc','SELECT max(version) FROM schema_migration')) -ne '63'){throw 'Retained schema mismatch; never migrate this fixture'}
if((Hash (Join-Path $private 'Caddyfile')) -ne $prior.gateway_sha256){throw 'Sealed gateway changed'}
Write-Utf8 (Join-Path $output 'delete-environment-before.json') (@{run_id=$run;as_of=[datetime]::UtcNow.ToString('o');network_id=$network.Id;internal=$true;containers=$topology;volumes=$prior.retained_volumes;cleanup=0}|ConvertTo-Json -Depth 12)
$sealed=[IO.File]::ReadAllText((Join-Path $private 'Caddyfile'))
$priorOS=$env:GOOS;$priorArch=$env:GOARCH;$priorCGO=$env:CGO_ENABLED
Push-Location (Join-Path $repo 'apps/platform')
try{
 $env:GOOS='linux';$env:GOARCH='amd64';$env:CGO_ENABLED='0'
 & go build -o (Join-Path $private 'worker-exact-delete') ./cmd/lifecycle-fixture-delete
 if($LASTEXITCODE -ne 0){throw 'Exact deletion build failed'}
 & go build -o (Join-Path $private 'worker/recovery-survivors') ./cmd/lifecycle-fixture-recovery
 if($LASTEXITCODE -ne 0){throw 'Read-only survivor witness build failed'}
}finally{$env:GOOS=$priorOS;$env:GOARCH=$priorArch;$env:CGO_ENABLED=$priorCGO;Pop-Location}
$confirmed=[DateTimeOffset]::UtcNow
$permission=@{Enabled=$true;RunID=$run;WorkspaceID=$c.WorkspaceID;PlanHash=$review.scope_hash;DeleteAccess='delete'+$run.Replace('-','').Substring(0,12);DeleteSecret=(Secret);BasisRef='human-confirmed:2026-10-11:dpp-lifecycle307-4abcfca5:quarantine/fixture/b.csv@ce87f789-a4f4-4ce7-bf1f-a044ac53d1d2';TopologySHA256=(Hash (Join-Path $output 'delete-environment-before.json'));ConfirmedAt=$confirmed.ToString('o');ExpiresAt=$confirmed.AddMinutes(30).ToString('o')}
$executor=Join-Path $private 'executor';New-Item -ItemType Directory -Path $executor|Out-Null
# The native object executor cannot see bootstrap/root/admin credentials.
$minimal=@{RunID=$c.RunID;WorkspaceID=$c.WorkspaceID;ProviderInstanceID=$c.ProviderInstanceID;IncarnationID=$c.IncarnationID;ActorID=$c.ActorID;Endpoint=$c.Endpoint;PostgresDSN=$c.PostgresDSN;InspectAccess=$c.InspectAccess;InspectSecret=$c.InspectSecret;Token=$c.Token;OutputDirectory=$c.OutputDirectory}
Write-Utf8 (Join-Path $executor 'config.json') ($minimal|ConvertTo-Json)
Copy-Item -LiteralPath (Join-Path $private 'worker-exact-delete') -Destination (Join-Path $executor 'worker-exact-delete')
Write-Utf8 (Join-Path $private 'delete-permission.json') ($permission|ConvertTo-Json)
Write-Utf8 (Join-Path $executor 'delete-permission.json') ($permission|ConvertTo-Json)
function Gateway([string]$DeleteRule,[string]$AdminRule){
 $rules="  @unsigned header X-Amz-Content-Sha256 STREAMING-UNSIGNED-PAYLOAD-TRAILER`n  respond @unsigned 403`n  @bypass header X-Amz-Bypass-Governance-Retention *`n  respond @bypass 403`n  @copy header X-Amz-Copy-Source *`n  respond @copy 403`n$AdminRule`n  @approvedDelete {`n   method DELETE`n   expression {http.request.header.Authorization}.contains('Credential=$($permission.DeleteAccess)/') && {http.request.uri} == '/dpp-lifecycle307-quarantine/fixture/b.csv?versionId=ce87f789-a4f4-4ce7-bf1f-a044ac53d1d2'`n  }`n  $DeleteRule`n"
 Write-Utf8 (Join-Path $private 'Caddyfile') ($sealed.Replace('  @mutation method DELETE PUT POST PATCH',$rules+'  @mutation method DELETE PUT POST PATCH'))
 Docker @('exec',$r.containers.gateway,'caddy','validate','--config','/etc/caddy/Caddyfile','--adapter','caddyfile')|Out-Null
 Docker @('restart',$r.containers.gateway)|Out-Null
 Docker @('exec',$r.containers.gateway,'wget','-T','10','-q','-O','-','http://127.0.0.1:9000/minio/health/ready')|Out-Null
}
function Run([string]$Mode){
 $name=$project+'-worker-delete-'+$Mode;$mount=$private;$writable=''
 if($Mode -eq 'execute'){$mount=$executor;$writable=',readonly'}
 Docker @('create','--pull','never','--name',$name,'--label',"lifecycle307.run=$run",'--network',"container:$($r.containers.gateway)",'--read-only','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data','--mount',"type=bind,source=$mount,target=/run/private$writable",'--mount',"type=bind,source=$results,target=/run/output",'--env',"LIFECYCLE_DELETE_MODE=$Mode",'--env','LIFECYCLE_DELETE_PROFILE=worker-delete-4abcfca5','--entrypoint','/run/private/worker-exact-delete',$pins.postgres)|Out-Null
 $hashes=@{}
 foreach($path in @('apps/platform/cmd/lifecycle-fixture-delete/main.go','apps/platform/cmd/lifecycle-fixture-delete/worker_delete.go','apps/platform/internal/lifecycle/application/fixture_basis.go','apps/platform/internal/lifecycle/application/exact_execution.go','apps/platform/internal/platform/storage/lifecycle_delete.go')){$hashes[$path]=Hash (Join-Path $repo $path)}
 Write-Utf8 (Join-Path $output ($name+'-provenance.json')) (@{run_id=$run;helper_id=(Docker @('inspect',$name,'--format','{{.Id}}'));mode=$Mode;binary_sha256=(Hash (Join-Path $private 'worker-exact-delete'));source_head=(& git -C $repo rev-parse HEAD).Trim();source_hashes=$hashes;gateway_sha256=(Hash (Join-Path $private 'Caddyfile'));topology_sha256=$permission.TopologySHA256;bootstrap_credentials_mounted=($Mode -ne 'execute');recorded_before_start=[datetime]::UtcNow.ToString('o')}|ConvertTo-Json -Depth 8)
 Docker @('start',$name)|Out-Null
 $exit=Docker @('wait',$name);$log=Docker @('logs',$name);if($log){Write-Output $log}
 Write-Output "$Mode exit=$exit; no resend or cleanup"
 if($exit -ne '0' -and -not ($Mode -eq 'execute' -and $exit -eq '93')){throw "Controlled $Mode failed; read-only recovery only"}
}
$admin="  @admin {`n   path /minio/admin/v3/add-canned-policy /minio/admin/v3/add-user /minio/admin/v3/idp/builtin/policy/attach`n   method PUT POST`n   expression {http.request.header.Authorization}.contains('Credential=$($c.RootAccess)/')`n  }`n  reverse_proxy @admin minio:9000"
$disable="  @disableExecutor {`n   method PUT`n   path /minio/admin/v3/set-user-status`n   expression {http.request.header.Authorization}.contains('Credential=$($c.RootAccess)/') && {http.request.uri.query.accessKey} == '$($permission.DeleteAccess)' && {http.request.uri.query.status} == 'disabled'`n  }`n  reverse_proxy @disableExecutor minio:9000"
$disabled=$false
try{
 Gateway 'respond @approvedDelete 403' $admin
 Run 'provision'
 Gateway 'reverse_proxy @approvedDelete minio:9000' ''
 Run 'execute'
}finally{
 try{Gateway 'respond @approvedDelete 403' $disable;Run 'disable';$disabled=$true}finally{
  try{
   Write-Utf8 (Join-Path $private 'Caddyfile') $sealed
   Docker @('exec',$r.containers.gateway,'caddy','validate','--config','/etc/caddy/Caddyfile','--adapter','caddyfile')|Out-Null
   Docker @('restart',$r.containers.gateway)|Out-Null
  }finally{
   $permission.Enabled=$false
   Write-Utf8 (Join-Path $private 'delete-permission.json') ($permission|ConvertTo-Json)
   Write-Utf8 (Join-Path $executor 'delete-permission.json') ($permission|ConvertTo-Json)
   Write-Utf8 (Join-Path $output 'delete-window-closed.json') (@{as_of=[datetime]::UtcNow.ToString('o');gateway_sha256=(Hash (Join-Path $private 'Caddyfile'));gateway_delete='DENY_ALL';native_delete_user_disabled=$disabled;fixture_authority='DISABLED';object_cleanup=0;volume_cleanup=0}|ConvertTo-Json)
  }
 }
}
