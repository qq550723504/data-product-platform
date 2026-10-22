param([Parameter(Mandatory=$true)][string]$PrivateDirectory,[switch]$Readback,[switch]$PrepareSourceDeletePlan)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$repo=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$private=(Resolve-Path -LiteralPath $PrivateDirectory).Path
if($private.StartsWith($repo+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'Private credentials must be outside checkout'}
function Docker([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments){$result=& docker.exe @Arguments 2>&1;if($LASTEXITCODE -ne 0){throw "Docker $($Arguments[0]) failed; resources retained"};return ($result -join "`n").Trim()}
function Write-Utf8([string]$Path,[string]$Value){[IO.File]::WriteAllText($Path,$Value,[Text.UTF8Encoding]::new($false))}
function New-Secret {$b=[byte[]]::new(32);[Security.Cryptography.RandomNumberGenerator]::Fill($b);[Convert]::ToHexString($b).ToLowerInvariant()}
$r=Get-Content -LiteralPath (Join-Path $private 'resources.json') -Raw|ConvertFrom-Json
$c=Get-Content -LiteralPath (Join-Path $private 'config.json') -Raw|ConvertFrom-Json
$run=([guid]$r.run_id).ToString();$project='dpp-lifecycle307-'+$run.Substring(0,8)
if($run -eq 'a6b30840-3e76-49ad-91d3-f0c399fcf170' -or $r.project -ne $project -or $c.RunID -ne $run -or -not $c.ArchiveAcceptance -or $c.Enabled -or (Split-Path -Leaf $private) -ne $project){throw 'Fresh archive-only scope required'}
if($Readback -and $PrepareSourceDeletePlan){throw 'Choose exactly one continuation mode'}
if((Test-Path -LiteralPath (Join-Path $private 'archive-window.json')) -and -not ($Readback -or $PrepareSourceDeletePlan)){throw 'One-shot copy window already exists; inspect retained facts instead of repeating COPY'}
$net=(Docker @('network','inspect',$r.network)|ConvertFrom-Json)[0]
if(-not $net.Internal -or $net.Labels.'lifecycle307.run' -ne $run -or @($net.Containers.PSObject.Properties).Count -ne 3){throw 'Closed network owner/member mismatch'}
$pins=@{postgres='sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea';minio='sha256:72476ee7f52a538e561627f7d8746e20c15d50c13e8bcdc58b22fa65cff94df6';gateway='sha256:d8542f48d34a9cf4e4c11a478865229840e87e4c96ea3f439101f31a5d35f75f'}
$topology=@()
foreach($role in @('postgres','minio','gateway')){$name=$r.containers.$role;$i=(Docker @('inspect',$name)|ConvertFrom-Json)[0];if($i.Image -ne $pins.$role -or $i.Config.Labels.'lifecycle307.run' -ne $run -or $i.State.Status -ne 'running' -or @($i.NetworkSettings.Networks.PSObject.Properties).Count -ne 1){throw 'Pinned owned container mismatch'};foreach($binding in @($i.NetworkSettings.Ports.PSObject.Properties)){if($null -ne $binding.Value -and @($binding.Value).Count -gt 0){throw 'Host port must not be exposed'}};$topology+=@{role=$role;name=$name;id=$i.Id;image=$i.Image}}
foreach($volume in $r.volumes){$v=(Docker @('volume','inspect',$volume)|ConvertFrom-Json)[0];if($v.Labels.'lifecycle307.run' -ne $run){throw 'Volume owner mismatch'}}
$results=Join-Path $PSScriptRoot 'results';$output=Join-Path $results $run
$manifestPath=Join-Path $output 'manifest.json'
$manifest=Get-Content -LiteralPath $manifestPath -Raw|ConvertFrom-Json
if($manifest.run_id -ne $run -or $manifest.workspace_id -ne $c.WorkspaceID -or @($manifest.objects).Count -ne 4 -or $manifest.destructive_requests -ne 0 -or $manifest.environment.migrations_through -ne 68){throw 'Fresh finalized schema68 preparation manifest required; retained earlier environments cannot run this current helper'}
$witness=Join-Path $private 'independent-witness';if(-not ($Readback -or $PrepareSourceDeletePlan)){New-Item -ItemType Directory -Path $witness|Out-Null}
Write-Utf8 (Join-Path $private 'archive-topology.json') (@{run_id=$run;network_id=$net.Id;containers=$topology;volumes=$r.volumes;host_ports=@();witness='Private host directory outside Core/provider volumes'}|ConvertTo-Json -Depth 8)
$window=@{Enabled=$true;RunID=$run;WorkspaceID=$c.WorkspaceID;Epoch=[guid]::NewGuid().ToString();Access='copy'+$run.Replace('-','').Substring(0,12);Secret=New-Secret;TopologySHA256=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $private 'archive-topology.json')).Hash.ToLowerInvariant();StartedAt=[datetime]::UtcNow.ToString('o');ExpiresAt=[datetime]::UtcNow.AddMinutes(30).ToString('o')}
if($Readback -or $PrepareSourceDeletePlan){$window=Get-Content -LiteralPath (Join-Path $private 'archive-window.json') -Raw|ConvertFrom-Json;if($window.Enabled -or $window.RunID -ne $run -or [datetime]::UtcNow -ge [datetime]$window.ExpiresAt -or -not (Test-Path -LiteralPath (Join-Path $output 'archive-start.json'))){throw 'Continuation requires a sealed same-run window'};if($Readback -and (Test-Path -LiteralPath (Join-Path $output 'archive-acceptance.json'))){throw 'Completed acceptance cannot repeat readback'};if($PrepareSourceDeletePlan -and ((Test-Path -LiteralPath (Join-Path $output 'archive-source-delete-plan.json')) -or -not (Test-Path -LiteralPath (Join-Path $output 'archive-acceptance.json')))){throw 'Source proposal requires completed acceptance and no existing proposal'};$window.Enabled=$true}
$priorOS=$env:GOOS;$priorArch=$env:GOARCH;$priorCGO=$env:CGO_ENABLED
Push-Location (Join-Path $repo 'apps/platform')
try{$env:GOOS='linux';$env:GOARCH='amd64';$env:CGO_ENABLED='0';& go build -o (Join-Path $private 'archive') ./cmd/lifecycle-fixture-archive;if($LASTEXITCODE -ne 0){throw 'Archive binary build failed'}}finally{$env:GOOS=$priorOS;$env:GOARCH=$priorArch;$env:CGO_ENABLED=$priorCGO;Pop-Location}
function Gateway([bool]$Enabled,[bool]$Admin=$false){
 $template=[IO.File]::ReadAllText((Join-Path $PSScriptRoot 'archive.Caddyfile'))
 if(-not $Admin){$template=[regex]::Replace($template,'(?ms)  @admin \{.*?  reverse_proxy @admin minio:9000\r?\n','')}
 $copy='';$payload=''
 if($Enabled){$copy=@'
  @scopedCopy {
   path /dpp-lifecycle307-quarantine/archive/a.csv /dpp-lifecycle307-quarantine/restore/online-a.csv /dpp-lifecycle307-quarantine/restore/old-cut-a.csv /dpp-lifecycle307-quarantine/restore/missing-a.csv /dpp-lifecycle307-quarantine/restore/gap-a.csv
   method PUT
   header X-Amz-Copy-Source *versionId=*
   expression {http.request.header.Authorization}.contains('Credential=__ACCESS__/')
  }
  reverse_proxy @scopedCopy minio:9000
'@
 $payload=@'
  @scopedPayload {
   path /dpp-lifecycle307-quarantine/archive/a.csv /dpp-lifecycle307-quarantine/restore/online-a.csv /dpp-lifecycle307-quarantine/restore/old-cut-a.csv /dpp-lifecycle307-quarantine/restore/missing-a.csv /dpp-lifecycle307-quarantine/restore/gap-a.csv
   method GET
   expression {http.request.header.Authorization}.contains('Credential=__ACCESS__/')
  }
  reverse_proxy @scopedPayload minio:9000
'@
 }
 $template=$template.Replace('__COPY_RULE__',$copy).Replace('__PAYLOAD_RULE__',$payload).Replace('__ACCESS__',$window.Access)
 Write-Utf8 (Join-Path $private 'Caddyfile') $template
 Docker @('exec',$r.containers.gateway,'caddy','validate','--config','/etc/caddy/Caddyfile','--adapter','caddyfile')|Out-Null
 Docker @('restart',$r.containers.gateway)|Out-Null
}
function Run([string]$Mode){
 $role=if($Mode -in @('provision','enable','disable')){'archive-admin'}else{'archive-owner'}
 $roleDirectory=Join-Path $private $role
 if(-not (Test-Path -LiteralPath $roleDirectory)){New-Item -ItemType Directory -Path $roleDirectory|Out-Null}
 $roleConfig=$c.PSObject.Copy()
 if($role -eq 'archive-owner'){$roleConfig.RootAccess='';$roleConfig.RootSecret=''}
 Write-Utf8 (Join-Path $roleDirectory 'config.json') ($roleConfig|ConvertTo-Json -Depth 16)
 Write-Utf8 (Join-Path $roleDirectory 'archive-window.json') ($window|ConvertTo-Json -Depth 6)
 if(-not (Test-Path -LiteralPath (Join-Path $roleDirectory 'archive'))){Copy-Item -LiteralPath (Join-Path $private 'archive') -Destination (Join-Path $roleDirectory 'archive')}
 if(-not (Test-Path -LiteralPath (Join-Path $roleDirectory 'manifest.json'))){Copy-Item -LiteralPath $manifestPath -Destination (Join-Path $roleDirectory 'manifest.json')}
 $helper=$project+'-archive-'+$Mode+'-'+[guid]::NewGuid().ToString().Substring(0,6)
 $arguments=@('create','--pull','never','--name',$helper,'--label',"lifecycle307.run=$run",'--network',"container:$($r.containers.gateway)",'--read-only','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data','--mount',"type=bind,source=$roleDirectory,target=/run/private,readonly",'--mount',"type=bind,source=$results,target=/run/output",'--mount',"type=bind,source=$witness,target=/run/witness")
 if($role -eq 'archive-admin'){
  $receipt=Join-Path $roleDirectory 'archive-admin-receipts.jsonl'
  if(-not (Test-Path -LiteralPath $receipt)){Write-Utf8 $receipt ''}
  $arguments+=@('--mount',"type=bind,source=$receipt,target=/run/private/archive-admin-receipts.jsonl")
 }
 $arguments+=@('-e',"LIFECYCLE_ARCHIVE_MODE=$Mode",'--entrypoint','/run/private/archive',$pins.postgres)
 Docker @arguments|Out-Null
 $sourceHashes=@{}
 foreach($path in @('apps/platform/cmd/lifecycle-fixture-archive/main.go','apps/platform/internal/lifecycle/application/copy_service.go','apps/platform/internal/platform/storage/lifecycle_copy.go','apps/platform/internal/lifecycle/infrastructure/file_restore_ledger.go','migrations/000063_lifecycle_archive_restore.up.sql')){$sourceHashes[$path]=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $repo $path)).Hash.ToLowerInvariant()}
 Write-Utf8 (Join-Path $output ($helper+'-provenance.json')) (@{run_id=$run;mode=$Mode;helper_id=(Docker @('inspect',$helper,'--format','{{.Id}}'));binary_sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $private 'archive')).Hash.ToLowerInvariant();source_head=(& git -C $repo rev-parse HEAD).Trim();source_hashes=$sourceHashes;topology_sha256=$window.TopologySHA256;gateway_sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $private 'Caddyfile')).Hash.ToLowerInvariant();recorded_before_start=[datetime]::UtcNow.ToString('o')}|ConvertTo-Json -Depth 8)
 Docker @('start',$helper)|Out-Null
 $exit=Docker @('wait',$helper)
 if($exit -ne '0'){throw "Archive $Mode failed; helper $helper and all facts retained, no automatic provider retry"}
 Write-Output "Archive $Mode completed in $project."
}
Write-Utf8 (Join-Path $private 'archive-window.json') ($window|ConvertTo-Json -Depth 6)
$provisioned=$false
try{if($PrepareSourceDeletePlan){Gateway $false;Run 'plan-source-delete'}elseif($Readback){Gateway $false $true;Run 'enable';$provisioned=$true;Gateway $false;Run 'readback'}else{Gateway $false $true;Run 'provision';$provisioned=$true;Gateway $true;Run 'execute'}}finally{
 try{Gateway $false $true}finally{
  try{if($provisioned){Run 'disable'}}finally{
   try{Gateway $false}finally{
    $window.Enabled=$false;Write-Utf8 (Join-Path $private 'archive-window.json') ($window|ConvertTo-Json -Depth 6)
    foreach($role in @('archive-admin','archive-owner')){$path=Join-Path $private "$role/archive-window.json";if(Test-Path -LiteralPath $path){$value=Get-Content -LiteralPath $path -Raw|ConvertFrom-Json;$value.Enabled=$false;Write-Utf8 $path ($value|ConvertTo-Json -Depth 6)}}
   }
  }
 }
}
$sealFile=if($PrepareSourceDeletePlan){'archive-source-plan-seal.json'}else{'archive-seal.json'}
Write-Utf8 (Join-Path $output $sealFile) (@{run_id=$run;sealed_at=[datetime]::UtcNow.ToString('o');native_copy_user='DISABLED';principal_enabled=$false;gateway='Object DELETE/PUT/POST/payload GET denied';native_delete=0;cleanup=0;volumes=$r.volumes;witness='Retained private directory outside volumes'}|ConvertTo-Json -Depth 6)
Write-Output "Acceptance evidence: $(Join-Path $output 'archive-acceptance.json'). All versions/volumes retained; new DELETE awaits exact manifest confirmation."
