param([switch]$HumanConfirmedExactSource)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if(-not $HumanConfirmedExactSource){throw 'Human confirmation of c9f47cab fixture/a.csv exact version is required'}
$repo=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$private='C:\Users\Henry\.codex\worktrees\issue307-lifecycle-d1\private-fixtures\dpp-lifecycle307-c9f47cab'
$run='c9f47cab-23c6-4d16-8102-a4a34bac1501';$project='dpp-lifecycle307-c9f47cab'
$results=Join-Path $PSScriptRoot 'results';$output=Join-Path $results $run
function Docker([string[]]$Arguments){$value=& docker.exe @Arguments 2>&1;if($LASTEXITCODE -ne 0){throw "Docker $($Arguments[0]) failed; preserve all resources"};return ($value -join "`n").Trim()}
function Write-Utf8([string]$Path,[string]$Value){[IO.File]::WriteAllText($Path,$Value,[Text.UTF8Encoding]::new($false))}
function Hash([string]$Path){(Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()}
function Secret{$bytes=[byte[]]::new(32);[Security.Cryptography.RandomNumberGenerator]::Fill($bytes);[Convert]::ToHexString($bytes).ToLowerInvariant()}
if((Test-Path -LiteralPath (Join-Path $private 'delete-permission.json')) -or (Test-Path -LiteralPath (Join-Path $output 'delete-acceptance.json'))){throw 'One-shot execution already configured; no automatic repeat/send'}
$r=Get-Content -LiteralPath (Join-Path $private 'resources.json') -Raw|ConvertFrom-Json
$c=Get-Content -LiteralPath (Join-Path $private 'config.json') -Raw|ConvertFrom-Json
$archiveWindow=Get-Content -LiteralPath (Join-Path $private 'archive-window.json') -Raw|ConvertFrom-Json
if($r.project -ne $project -or $c.RunID -ne $run -or $c.WorkspaceID -ne '5fe971c0-7682-4b6b-b4c6-741883247365' -or $c.Enabled -or $archiveWindow.Enabled){throw 'Scope or closed archive window mismatch'}
$ids=@{postgres='08f55f18ce8520d4bd35a3314e208ce19c543737fa342db48214495d4facc786';minio='8877f5980344b5278ba86b2dcac0ce2efb783c2f89c7bd184c6e37eb62335138';gateway='78dbb57abd29a36113cc5d13624aacbe6888bcbff3694ccca24ce6fd500b61c5'}
$pins=@{postgres='sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea';minio='sha256:4d2b20cfce457652e6be8c5038d90704a99fdf0160b7e775d968f9163e96a741';gateway='sha256:d8542f48d34a9cf4e4c11a478865229840e87e4c96ea3f439101f31a5d35f75f'}
$topology=@()
foreach($role in @('postgres','minio','gateway')){
 $i=(Docker @('inspect',$r.containers.$role)|ConvertFrom-Json)[0]
 if($i.Id -ne $ids.$role -or $i.Image -ne $pins.$role -or $i.State.Status -ne 'running' -or $i.Config.Labels.'lifecycle307.run' -ne $run -or @($i.NetworkSettings.Networks.PSObject.Properties).Count -ne 1){throw 'Owned exact container identity/state/network mismatch'}
 foreach($port in @($i.NetworkSettings.Ports.PSObject.Properties)){if($null -ne $port.Value -and @($port.Value).Count -gt 0){throw 'Closed fixture published host ports'}}
 $topology+=@{role=$role;id=$i.Id;image=$i.Image;mounts=@($i.Mounts|Select-Object Type,Name,Destination);networks=$i.NetworkSettings.Networks;ports=$i.NetworkSettings.Ports}
}
$network=(Docker @('network','inspect',$r.network)|ConvertFrom-Json)[0]
if($network.Id -ne '6dea17e188c5b61449870f101f1caa1ff33485bf755e6126bd7f39f189af60ba' -or -not $network.Internal -or @($network.Containers.PSObject.Properties).Count -ne 3){throw 'Private exact network membership mismatch'}
if((Docker @('exec',$r.containers.minio,'sha256sum','/usr/local/bin/minio')).Split(' ')[0] -ne 'fb741388b404ec67c4f3777b812d6174ba5562532acb17fb1ddfba9a4e6249b8'){throw 'Provider binary changed'}
foreach($vname in @("${project}-postgres","${project}-objects")){$v=(Docker @('volume','inspect',$vname)|ConvertFrom-Json)[0];if($v.Labels.'lifecycle307.run' -ne $run){throw 'Volume owner mismatch'}}
if((Docker @('exec',$r.containers.postgres,'psql','-U','fixture','-d','lifecycle307','-Atc','SELECT max(version) FROM schema_migration')) -ne '62'){throw 'Retained schema mismatch; no migration changes allowed'}
Write-Utf8 (Join-Path $output 'delete-environment-before.json') (@{run_id=$run;as_of=[datetime]::UtcNow.ToString('o');network_id=$network.Id;internal=$true;containers=$topology;volumes=$r.volumes;cleanup=0}|ConvertTo-Json -Depth 12)
Write-Utf8 (Join-Path $private 'pre-delete.Caddyfile') ([IO.File]::ReadAllText((Join-Path $private 'Caddyfile')))
$priorOS=$env:GOOS;$priorArch=$env:GOARCH;$priorCGO=$env:CGO_ENABLED
Push-Location (Join-Path $repo 'apps/platform')
try{$env:GOOS='linux';$env:GOARCH='amd64';$env:CGO_ENABLED='0';& go build -o (Join-Path $private 'source-exact-delete') ./cmd/lifecycle-fixture-delete;if($LASTEXITCODE -ne 0){throw 'Exact deletion build failed'}}finally{$env:GOOS=$priorOS;$env:GOARCH=$priorArch;$env:CGO_ENABLED=$priorCGO;Pop-Location}
$confirmed=[DateTimeOffset]::UtcNow
$permission=@{Enabled=$true;RunID=$run;WorkspaceID=$c.WorkspaceID;PlanHash='7ccf841b84d919b3f9634983fe789e74fb82b0cbbf86017fd1ad95a841aed47e';DeleteAccess='delete'+$run.Replace('-','').Substring(0,12);DeleteSecret=(Secret);BasisRef='human-confirmed:2026-10-11:dpp-lifecycle307-c9f47cab:fixture/a.csv@3fe1cc44-b0f8-428c-9bb8-9f92dac5d15d';TopologySHA256=(Hash (Join-Path $output 'delete-environment-before.json'));ConfirmedAt=$confirmed.ToString('o');ExpiresAt=$confirmed.AddMinutes(30).ToString('o')}
Write-Utf8 (Join-Path $private 'delete-permission.json') ($permission|ConvertTo-Json)
$template=[IO.File]::ReadAllText((Join-Path $PSScriptRoot 'archive-source-delete.Caddyfile'))
function Gateway([string]$DeleteRule,[string]$AdminRule){
 $text=$template.Replace('__DELETE_ACCESS__',$permission.DeleteAccess).Replace('__DELETE_RULE__',$DeleteRule).Replace('__ADMIN_RULE__',$AdminRule)
 Write-Utf8 (Join-Path $private 'Caddyfile') $text
 Docker @('exec',$r.containers.gateway,'caddy','validate','--config','/etc/caddy/Caddyfile','--adapter','caddyfile')|Out-Null
 Docker @('restart',$r.containers.gateway)|Out-Null
 Docker @('exec',$r.containers.gateway,'wget','-T','10','-q','-O','-','http://127.0.0.1:9000/minio/health/ready')|Out-Null
}
function Run([string]$Mode){
 $name=$project+'-source-delete-'+$Mode
 Docker @('create','--pull','never','--name',$name,'--label',"lifecycle307.run=$run",'--network',"container:$($r.containers.gateway)",'--read-only','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data','--mount',"type=bind,source=$private,target=/run/private",'--mount',"type=bind,source=$results,target=/run/output",'--env',"LIFECYCLE_DELETE_MODE=$Mode",'--env','LIFECYCLE_DELETE_PROFILE=archive-source-c9f47cab','--entrypoint','/run/private/source-exact-delete',$pins.postgres)|Out-Null
 $hashes=@{}
 foreach($path in @('apps/platform/cmd/lifecycle-fixture-delete/main.go','apps/platform/cmd/lifecycle-fixture-delete/archive_source.go','apps/platform/internal/lifecycle/application/fixture_basis.go','apps/platform/internal/lifecycle/application/exact_execution.go','apps/platform/internal/platform/storage/lifecycle_delete.go')){$hashes[$path]=Hash (Join-Path $repo $path)}
 Write-Utf8 (Join-Path $output ($name+'-provenance.json')) (@{run_id=$run;helper_id=(Docker @('inspect',$name,'--format','{{.Id}}'));mode=$Mode;binary_sha256=(Hash (Join-Path $private 'source-exact-delete'));source_head=(& git -C $repo rev-parse HEAD).Trim();source_hashes=$hashes;gateway_sha256=(Hash (Join-Path $private 'Caddyfile'));topology_sha256=$permission.TopologySHA256;recorded_before_start=[datetime]::UtcNow.ToString('o')}|ConvertTo-Json -Depth 8)
 Docker @('start',$name)|Out-Null
 $exit=Docker @('wait',$name);$log=Docker @('logs',$name);if($log){Write-Output $log}
 if($exit -ne '0'){throw "Controlled $Mode failed; no automatic resend; preserve all receipts/objects/volumes"}
}
$admin="  @admin {`n   path /minio/admin/v3/*`n   method PUT POST`n   expression {http.request.header.Authorization}.contains('Credential=$($c.RootAccess)/')`n  }`n  reverse_proxy @admin minio:9000"
$disable="  @disableExecutor {`n   method PUT`n   path /minio/admin/v3/set-user-status`n   expression {http.request.header.Authorization}.contains('Credential=$($c.RootAccess)/') && {http.request.uri.query.accessKey} == '$($permission.DeleteAccess)' && {http.request.uri.query.status} == 'disabled'`n  }`n  reverse_proxy @disableExecutor minio:9000"
$provisioned=$false;$disabled=$false
try{
 Gateway 'respond @approvedDelete 403' $admin
 Run 'provision';$provisioned=$true
 Gateway 'reverse_proxy @approvedDelete minio:9000' ''
 Run 'execute'
}finally{
 try{Gateway 'respond @approvedDelete 403' $disable;if($provisioned){Run 'disable';$disabled=$true}}finally{
  try{Gateway 'respond @approvedDelete 403' ''}finally{
   $permission.Enabled=$false;Write-Utf8 (Join-Path $private 'delete-permission.json') ($permission|ConvertTo-Json)
   Write-Utf8 (Join-Path $output 'delete-window-closed.json') (@{as_of=[datetime]::UtcNow.ToString('o');gateway_sha256=(Hash (Join-Path $private 'Caddyfile'));gateway_delete='DENY_ALL';native_delete_user_disabled=$disabled;fixture_authority='DISABLED';object_cleanup=0;volume_cleanup=0}|ConvertTo-Json)
  }
 }
}
