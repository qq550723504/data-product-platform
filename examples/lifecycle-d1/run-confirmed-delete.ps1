param([switch]$HumanConfirmedExactTwoVersions)
$ErrorActionPreference='Stop'
if(-not $HumanConfirmedExactTwoVersions){throw 'Explicit human confirmation of the documented exact two versions is required'}
$repo=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$private='C:\Users\Henry\.codex\worktrees\issue307-lifecycle-d1\private-fixtures\dpp-lifecycle307-a6b30840'
$run='a6b30840-3e76-49ad-91d3-f0c399fcf170'
$project='dpp-lifecycle307-a6b30840'
$results=Join-Path $PSScriptRoot 'results'
$output=Join-Path $results $run
function Invoke-FixtureDocker([string[]]$DockerArguments){$result=& docker.exe @DockerArguments 2>&1;if($LASTEXITCODE -ne 0){throw ('Docker operation failed: '+($DockerArguments[0..1] -join ' '))};return ($result -join "`n").Trim()}
function Write-Utf8([string]$Path,[string]$Value){[IO.File]::WriteAllText($Path,$Value,[Text.UTF8Encoding]::new($false))}
function Hash([string]$Path){(Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()}
function Secret{$bytes=[byte[]]::new(32);[Security.Cryptography.RandomNumberGenerator]::Fill($bytes);[Convert]::ToHexString($bytes).ToLowerInvariant()}
if(Test-Path -LiteralPath (Join-Path $private 'delete-permission.json')){throw 'Execution already configured. Preserve starts/observations and reconcile; do not rerun writes'}
if(Test-Path -LiteralPath (Join-Path $output 'delete-acceptance.json')){throw 'Acceptance already finalized; no repeat deletion'}
$resources=Get-Content -LiteralPath (Join-Path $private 'resources.json') -Raw|ConvertFrom-Json
$config=Get-Content -LiteralPath (Join-Path $private 'config.json') -Raw|ConvertFrom-Json
if($resources.project -ne $project -or $config.RunID -ne $run -or $config.WorkspaceID -ne '39dd70e6-691a-4d69-b971-6c75533b72d6' -or $config.Enabled){throw 'Retained fixture identity/disabled preparation principal mismatch'}
$expectedIDs=@{postgres='2dac61950f7bec8b9cc03a5ca2c9acb279640daff161a428885f0eef5bfaa414';minio='4b7287adcc0c6b221f2516f0c4af8065d281c18a90322fbcdfcb01325e52f7c2';gateway='102dcc35b2d6c50f1e41207af9ba3a8dc202caeceb36620085d99f2a68ab73c9'}
$expectedImages=@{postgres='sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea';minio='sha256:a9eb76492485dd496bfd69b4114e47137f2da1a1a87e4a9b273cb21476cb1a98';gateway='sha256:d8542f48d34a9cf4e4c11a478865229840e87e4c96ea3f439101f31a5d35f75f'}
$topology=@()
foreach($kind in @('postgres','minio','gateway')){
 $item=(Invoke-FixtureDocker @('inspect',$resources.containers.$kind)|ConvertFrom-Json)[0]
 if($item.Id -ne $expectedIDs.$kind -or $item.Image -ne $expectedImages.$kind -or $item.State.Status -ne 'running' -or $item.Config.Labels.'lifecycle307.run' -ne $run -or @($item.NetworkSettings.Networks.PSObject.Properties).Count -ne 1){throw 'Retained service identity/state/network mismatch'}
 foreach($port in @($item.NetworkSettings.Ports.PSObject.Properties)){if(@($port.Value).Count -ne 0){throw 'Closed fixture unexpectedly published a host port'}}
 $topology+=@{name=$item.Name;id=$item.Id;image=$item.Image;networks=$item.NetworkSettings.Networks;ports=$item.NetworkSettings.Ports;mounts=@($item.Mounts|Select-Object Type,Name,Destination)}
}
$network=(Invoke-FixtureDocker @('network','inspect',$resources.network)|ConvertFrom-Json)[0]
if($network.Id -ne '6bec8f5e4e1164a0716740fade68c36b6fc995954186b876b20416357c2d9c63' -or -not $network.Internal -or @($network.Containers.PSObject.Properties).Count -ne 3){throw 'Closed network membership changed'}
if((Invoke-FixtureDocker @('exec',$resources.containers.minio,'sha256sum','/usr/local/bin/minio')).Split(' ')[0] -ne 'fb741388b404ec67c4f3777b812d6174ba5562532acb17fb1ddfba9a4e6249b8'){throw 'Provider binary changed'}
foreach($volume in @("${project}-postgres","${project}-objects")){$v=(Invoke-FixtureDocker @('volume','inspect',$volume)|ConvertFrom-Json)[0];if($v.Labels.'lifecycle307.run' -ne $run){throw 'Retained volume owner mismatch'}}
Write-Utf8 (Join-Path $output 'delete-environment-before.json') (@{as_of=[DateTimeOffset]::UtcNow;network_id=$network.Id;internal=$true;containers=$topology;object_cleanup=0;volume_cleanup=0}|ConvertTo-Json -Depth 12)
$extension=Join-Path $repo 'migrations/000060_lifecycle_scoped_completion.up.sql'
$reconciliationExtension=Join-Path $repo 'migrations/000061_lifecycle_denied_reconciliation.up.sql'
$baseVersion=Invoke-FixtureDocker @('exec',$resources.containers.postgres,'psql','-U','fixture','-d','lifecycle307','-Atc','SELECT max(version) FROM schema_migration')
if($baseVersion -ne '54'){throw 'Retained schema baseline changed; explicit schema preflight required'}
$oldOS=$env:GOOS;$oldArch=$env:GOARCH;$oldCGO=$env:CGO_ENABLED
Push-Location (Join-Path $repo 'apps/platform')
try{$env:GOOS='linux';$env:GOARCH='amd64';$env:CGO_ENABLED='0';& go build -o (Join-Path $private 'exact-delete') ./cmd/lifecycle-fixture-delete;if($LASTEXITCODE -ne 0){throw 'Linux exact deletion build failed'}}finally{$env:GOOS=$oldOS;$env:GOARCH=$oldArch;$env:CGO_ENABLED=$oldCGO;Pop-Location}
# Add only the reviewed new tables to the preserved old54 D1 schema snapshot.
# Do not install duplicate58, skip main55/57, forge schema_migration58 or run down.
Get-Content -LiteralPath $extension -Raw | & docker exec -i $resources.containers.postgres psql -U fixture -d lifecycle307 -v ON_ERROR_STOP=1 -1 | Out-Null
if($LASTEXITCODE -ne 0){throw 'Additive extension failed; transaction rolled back, no provider deletion'}
Get-Content -LiteralPath $reconciliationExtension -Raw | & docker exec -i $resources.containers.postgres psql -U fixture -d lifecycle307 -v ON_ERROR_STOP=1 -1 | Out-Null
if($LASTEXITCODE -ne 0){throw 'Additive reconciliation extension failed; no provider deletion'}
Write-Utf8 (Join-Path $output 'delete-schema-extension.json') (@{base_schema_version=54;extension_file='000060_lifecycle_scoped_completion.up.sql';extension_sha256=(Hash $extension);reconciliation_extension_file='000061_lifecycle_denied_reconciliation.up.sql';reconciliation_extension_sha256=(Hash $reconciliationExtension);schema_migration_unchanged=$true;binary_sha256=(Hash (Join-Path $private 'exact-delete'));applied_at=[DateTimeOffset]::UtcNow;note='Preserved fixture snapshot54 plus explicit additive60+61 tables. Fresh installation uses main55/57+D159/60/61; coordinated C1 owns unpublished58.'}|ConvertTo-Json)
$confirmed=[DateTimeOffset]::UtcNow
$permission=@{Enabled=$true;RunID=$run;WorkspaceID=$config.WorkspaceID;PlanHash='7c2847699997663ca453db54e0fab1565ad67fc9434c0c2c507c6adaa056a09f';DeleteAccess='delete'+$run.Replace('-','').Substring(0,12);DeleteSecret=(Secret);BasisRef='human-confirmed:2026-10-11:dpp-lifecycle307-a6b30840:exact-two-unprotected-versions';TopologySHA256=(Hash (Join-Path $output 'delete-environment-before.json'));ConfirmedAt=$confirmed.ToString('o');ExpiresAt=$confirmed.AddHours(1).ToString('o')}
Write-Utf8 (Join-Path $private 'delete-permission.json') ($permission|ConvertTo-Json)
function Run-Helper([string]$Mode){
 $name=$project+'-delete-'+$Mode
 Invoke-FixtureDocker @('create','--pull','never','--name',$name,'--label',"lifecycle307.run=$run",'--network',"container:$($resources.containers.gateway)",'--read-only','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data','--mount',"type=bind,source=$private,target=/run/private",'--mount',"type=bind,source=$results,target=/run/output",'--env',"LIFECYCLE_DELETE_MODE=$Mode",'--entrypoint','/run/private/exact-delete',$expectedImages.postgres)|Out-Null
 Invoke-FixtureDocker @('start',$name)|Out-Null
 $exit=Invoke-FixtureDocker @('wait',$name)
 $log=Invoke-FixtureDocker @('logs',$name);if($log){Write-Output $log}
 if($exit -ne '0'){throw "Controlled $Mode failed; preserve facts, no automatic retry"}
}
$template=[IO.File]::ReadAllText((Join-Path $PSScriptRoot 'confirmed-delete.Caddyfile'))
$active=$template.Replace('__DELETE_ACCESS__',$permission.DeleteAccess)
$sealed=$active.Replace('reverse_proxy @approvedDelete minio:9000','respond @approvedDelete 403')
function Set-Gateway([string]$Text){Write-Utf8 (Join-Path $private 'Caddyfile') $Text;Invoke-FixtureDocker @('exec',$resources.containers.gateway,'caddy','validate','--config','/etc/caddy/Caddyfile','--adapter','caddyfile')|Out-Null;Invoke-FixtureDocker @('restart',$resources.containers.gateway)|Out-Null;Invoke-FixtureDocker @('exec',$resources.containers.gateway,'wget','-T','10','-q','-O','-','http://127.0.0.1:9000/minio/health/ready')|Out-Null}
try{
 Run-Helper 'provision'
 Set-Gateway $active
 Write-Utf8 (Join-Path $output 'delete-execution-window.json') (@{gateway_config_sha256=(Hash (Join-Path $private 'Caddyfile'));authority_window_start=$permission.ConfirmedAt;authority_window_end=$permission.ExpiresAt;plan_hash=$permission.PlanHash;binary_sha256=(Hash (Join-Path $private 'exact-delete'));no_put_post_payload_get=$true;no_bypass=$true;protected_delete=$false}|ConvertTo-Json)
 Run-Helper 'execute'
 # Seal all object mutation before disabling the temporary native IAM identity.
 # This admin opening covers only SetUserStatus, with root authentication.
 $disableRoute="  @disableExecutor {`n   method PUT`n   path /minio/admin/v3/set-user-status`n   expression {http.request.header.Authorization}.contains('Credential=$($config.RootAccess)/') && {http.request.uri.query.accessKey} == '$($permission.DeleteAccess)' && {http.request.uri.query.status} == 'disabled'`n  }`n  reverse_proxy @disableExecutor minio:9000`n"
 Set-Gateway $sealed.Replace('  @mutate method',$disableRoute+'  @mutate method')
 Run-Helper 'disable'
}finally{
 Set-Gateway $sealed
 $permission.Enabled=$false
 Write-Utf8 (Join-Path $private 'delete-permission.json') ($permission|ConvertTo-Json)
 Write-Utf8 (Join-Path $output 'delete-window-closed.json') (@{as_of=[DateTimeOffset]::UtcNow;gateway_config_sha256=(Hash (Join-Path $private 'Caddyfile'));gateway_delete='DENY_ALL';fixture_execution_authority='DISABLED';object_cleanup=0;volume_cleanup=0}|ConvertTo-Json)
}
