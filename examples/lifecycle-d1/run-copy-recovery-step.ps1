param([Parameter(Mandatory=$true)][string]$PrivateDirectory,[Parameter(Mandatory=$true)][ValidateSet('provision','provision-recovery','produce-archive','produce-restore','witness','restrict','readback','outbox','once','scan','enqueue','request','worker','seal')][string]$Mode,[Parameter(Mandatory=$true)][string]$Scenario,[string]$WorkID,[string]$CopyCallID,[ValidateSet('restore/online-a.csv','restore/old-cut-a.csv','restore/missing-a.csv','restore/gap-a.csv')][string]$Target='restore/online-a.csv',[ValidateSet('archive','archive-v2')][string]$ProducerBinary='archive')
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot '../temporary-containers.ps1')
$repo=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$private=(Resolve-Path -LiteralPath $PrivateDirectory).Path
if($private.StartsWith($repo+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase) -or $Scenario -notmatch '^[a-z0-9-]{1,64}$'){throw 'Private directory/scenario scope invalid'}
function Docker([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments){$v=& docker.exe @Arguments 2>&1;if($LASTEXITCODE -ne 0){throw "Docker $($Arguments[0]) failed; retain every resource. $($v -join ' ')"};($v -join "`n").Trim()}
function Write-Utf8([string]$Path,[string]$Value){[IO.File]::WriteAllText($Path,$Value.Replace("`r`n","`n"),[Text.UTF8Encoding]::new($false))}
$r=Get-Content -LiteralPath (Join-Path $private 'resources.json') -Raw|ConvertFrom-Json
$c=Get-Content -LiteralPath (Join-Path $private 'config.json') -Raw|ConvertFrom-Json
$window=Get-Content -LiteralPath (Join-Path $private 'copy-admin/archive-window.json') -Raw|ConvertFrom-Json
$run=([guid]$r.run_id).ToString();$project='dpp-lifecycle307-'+$run.Substring(0,8)
$output=Join-Path $PSScriptRoot "results/$run"
$m=Get-Content -LiteralPath (Join-Path $output 'manifest.json') -Raw|ConvertFrom-Json
if($r.project -ne $project -or (Split-Path -Leaf $private) -ne $project -or $window.RunID -ne $run -or -not $window.Enabled -or [DateTime]::UtcNow -ge [DateTime]$window.ExpiresAt -or $c.Enabled -or $m.environment.migrations_through -ne 66 -or $m.destructive_requests -ne 0){throw 'Finite fresh schema66 window required; no retained-old environment mutation'}
$net=(Docker @('network','inspect',$r.network)|ConvertFrom-Json)[0]
$members=@($net.Containers.PSObject.Properties).Count
$brokerFault=$false
if($Mode -eq 'outbox' -and $Scenario -eq 'copy-outbox-broker-unavailable' -and $members -eq 3){
 $redis=(Docker @('inspect',($project+'-redis'))|ConvertFrom-Json)[0]
 $brokerFault=$redis.State.Status -eq 'exited' -and $redis.Config.Labels.'lifecycle307.run' -eq $run -and $redis.Image -eq 'sha256:858f009f9709ce576febc734aa78b8f6d624b82571f9ddb6bda4377c833b3499'
}
if(-not $net.Internal -or ($members -ne 4 -and -not $brokerFault) -or $net.Labels.'lifecycle307.run' -ne $run){throw 'Closed four-member owner mismatch'}
$gw=(Docker @('inspect',$r.containers.gateway)|ConvertFrom-Json)[0]
if($gw.State.Status -ne 'running' -or $gw.Config.Labels.'lifecycle307.run' -ne $run){throw 'Gateway owner/state mismatch'}
function Gateway([bool]$Copy,[bool]$Admin,[bool]$Payload){
 $template=[IO.File]::ReadAllText((Join-Path $PSScriptRoot 'archive.Caddyfile'))
 if(-not $Admin){$template=[regex]::Replace($template,'(?ms)  @admin \{.*?  reverse_proxy @admin minio:9000\r?\n','')}
 $copyRule='';$payloadRule=''
 if($Copy){$copyRule=@'
  @scopedCopy {
   path /dpp-lifecycle307-quarantine/archive/a.csv /dpp-lifecycle307-quarantine/restore/online-a.csv /dpp-lifecycle307-quarantine/restore/old-cut-a.csv /dpp-lifecycle307-quarantine/restore/missing-a.csv /dpp-lifecycle307-quarantine/restore/gap-a.csv
   method PUT
   header X-Amz-Copy-Source *versionId=*
   expression {http.request.header.Authorization}.contains('Credential=__COPY_ACCESS__/')
  }
  reverse_proxy @scopedCopy minio:9000
'@}
 if($Payload){$payloadRule=@'
  @verifiedTargetBytes {
   path /dpp-lifecycle307-quarantine/archive/a.csv /dpp-lifecycle307-quarantine/restore/online-a.csv /dpp-lifecycle307-quarantine/restore/old-cut-a.csv /dpp-lifecycle307-quarantine/restore/missing-a.csv /dpp-lifecycle307-quarantine/restore/gap-a.csv
   method GET
   expression {http.request.header.Authorization}.contains('Credential=__READ_ACCESS__/')
  }
  reverse_proxy @verifiedTargetBytes minio:9000
'@}
 $template=$template.Replace('__COPY_RULE__',$copyRule).Replace('__PAYLOAD_RULE__',$payloadRule).Replace('__COPY_ACCESS__',$window.Access).Replace('__READ_ACCESS__',$window.RecoveryAccess)
 Write-Utf8 (Join-Path $private 'Caddyfile') $template
 Docker @('exec',$r.containers.gateway,'caddy','validate','--config','/etc/caddy/Caddyfile','--adapter','caddyfile')|Out-Null
 Docker @('restart',$r.containers.gateway)|Out-Null
}
function Run([string]$Role,[string]$Action,[int]$ExpectedExit){
 $dir=Join-Path $private $Role;$binary=if($Role -eq 'copy-worker'){'recovery'}else{'archive'}
 if($Role -eq 'copy-producer'){$binary=$ProducerBinary}
 $helper=$project+'-'+$Scenario+'-'+[guid]::NewGuid().ToString().Substring(0,6)
 $arguments=@('create','--pull','never','--name',$helper,'--label',"lifecycle307.run=$run",'--network',"container:$($r.containers.gateway)",'--read-only','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data','--mount',"type=bind,source=$dir,target=/run/private,readonly",'--mount',"type=bind,source=$(Join-Path $PSScriptRoot 'results'),target=/run/output")
 if($Role -eq 'copy-admin'){
  $receipt=Join-Path $dir 'archive-admin-receipts.jsonl'
  if(-not (Test-Path -LiteralPath $receipt)){[IO.File]::WriteAllText($receipt,'')}
  $arguments+=@('--mount',"type=bind,source=$receipt,target=/run/private/archive-admin-receipts.jsonl")
 }
 $witness=Join-Path $private 'copy-independent-witness'
 $arguments+=@('--mount',("type=bind,source=$witness,target=/run/witness"+$(if($Role -eq 'copy-worker'){',readonly'}else{''})))
 if($Role -eq 'copy-worker'){
  $cfg=@{Mode=$Action;Scenario=$Scenario;PostgresDSN=[IO.File]::ReadAllText((Join-Path $dir 'dsn'));RedisAddr=[IO.File]::ReadAllText((Join-Path $dir 'redis'));RecoveryConfig='/run/private/recovery.json';Manifest='';OutputDirectory="/run/output/$run";Barrier="/run/output/$run/$Scenario-barrier.json";SeedEnabled=$false;CopyRecovery=$true;WorkspaceID=$c.WorkspaceID;ActorID=$c.ActorID;ExpiresAt=$window.ExpiresAt;WorkID=if($WorkID){$WorkID}else{[guid]::Empty.ToString()};CopyCallID=if($CopyCallID){$CopyCallID}else{[guid]::Empty.ToString()}}
  Write-Utf8 (Join-Path $dir ($Scenario+'.json')) ($cfg|ConvertTo-Json)
  $arguments+=@('-e',"LIFECYCLE_RECOVERY_ACCEPTANCE_CONFIG=/run/private/$Scenario.json")
 } else {$arguments+=@('-e',"LIFECYCLE_ARCHIVE_MODE=$Action",'-e',"LIFECYCLE_FIXTURE_CHECKPOINT_KEY=$Scenario",'-e',"LIFECYCLE_ARCHIVE_TARGET=$Target")}
 $arguments+=@('--entrypoint',"/run/private/$binary",'sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea')
 $temporaryHelper=New-DppTemporaryContainer -RunID $run -DockerArguments $arguments[1..($arguments.Count-1)]
 $proof=@{run_id=$run;scenario=$Scenario;role=$Role;action=$Action;helper_id=(Docker @('inspect',$helper,'--format','{{.Id}}'));source_head=(& git -C $repo rev-parse HEAD).Trim();binary_sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $dir $binary)).Hash.ToLowerInvariant();gateway_sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $private 'Caddyfile')).Hash.ToLowerInvariant();recorded_before_start=[DateTime]::UtcNow.ToString('o');expected_exit=$ExpectedExit;native_delete=0;cleanup=0;credentials=if($Role -eq 'copy-worker'){'READ_ONLY_NO_ROOT_NO_COPY_NO_DELETE'}elseif($Role -eq 'copy-producer'){'COPY_ONLY_NO_ROOT_NO_DELETE'}else{'IAM_PROVISIONING_ONLY'}}
 $proof.source_hashes=@{}
 foreach($path in @('apps/platform/cmd/lifecycle-fixture-archive/main.go','apps/platform/cmd/lifecycle-fixture-archive/copy_recovery.go','apps/platform/cmd/lifecycle-fixture-recovery/main.go','apps/platform/cmd/lifecycle-fixture-recovery/copy_crash.go','apps/platform/internal/lifecycle/application/copy_recovery.go','apps/platform/internal/lifecycle/application/copy_service.go','apps/platform/internal/platform/storage/lifecycle_copy.go','apps/platform/internal/platform/storage/lifecycle_copy_recovery.go','apps/platform/internal/platform/storage/lifecycle_copy_fixture_fault.go','apps/platform/internal/lifecycle/pilot/copy_recovery.go','migrations/000064_lifecycle_reconciliation_work.up.sql')){$proof.source_hashes[$path]=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $repo $path)).Hash.ToLowerInvariant()}
 Write-Utf8 (Join-Path $output ($helper+'-provenance.json')) ($proof|ConvertTo-Json -Depth 7)
 Docker @('start',$helper)|Out-Null
 $exit=[int](Docker @('wait',$helper))
 Write-Utf8 (Join-Path $output ($helper+'-exit.json')) (@{helper_id=$proof.helper_id;exit_code=$exit;expected=$ExpectedExit}|ConvertTo-Json)
 if($exit -ne $ExpectedExit){throw "Controlled step failed; retained helper $helper exit=$exit; inspect before any continuation"}
 Write-Utf8 (Join-Path $output ($helper+'-cleanup.json')) ((Complete-DppTemporaryContainer -Handle $temporaryHelper -ExpectedExitCode $ExpectedExit)|ConvertTo-Json)
 Write-Output "Controlled $Scenario exit=$exit in $project; all objects/volumes retained."
}
try {
 switch($Mode){
  provision {Gateway $false $true $false;Run 'copy-admin' 'provision' 0}
  provision-recovery {Gateway $false $true $false;Run 'copy-admin' 'recovery-provision' 0}
  produce-archive {Gateway $true $false $false;Run 'copy-producer' 'response-loss-archive' 93}
  produce-restore {Gateway $true $false $false;Run 'copy-producer' 'response-loss-restore' 93}
  witness {Gateway $false $false $false;Run 'copy-producer' 'copy-witness' 0}
  restrict {Gateway $false $false $false;Run 'copy-producer' 'copy-restrict' 0}
  readback {Gateway $false $false $false;Run 'copy-producer' 'copy-final-readback' 0}
  seal {
   Gateway $false $true $false
   Run 'copy-admin' 'disable' 0
   Run 'copy-admin' 'recovery-disable' 0
   foreach($role in @('copy-admin','copy-producer')){$p=Join-Path $private "$role/archive-window.json";$v=Get-Content -LiteralPath $p -Raw|ConvertFrom-Json;$v.Enabled=$false;Write-Utf8 $p ($v|ConvertTo-Json)}
   $p=Join-Path $private 'copy-worker/recovery.json';$v=Get-Content -LiteralPath $p -Raw|ConvertFrom-Json;$v.Enabled=$false;Write-Utf8 $p ($v|ConvertTo-Json)
  }
  default {Gateway $false $false $true;$expected=if($Scenario -eq 'copy-crash-after-lookup'){92}else{0};Run 'copy-worker' $Mode $expected}
 }
}finally {Gateway $false $false $false}
