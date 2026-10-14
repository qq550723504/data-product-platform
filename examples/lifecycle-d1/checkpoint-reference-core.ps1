# Retained PostgreSQL checkpoint for the fixed synthetic 5d2274ee fixture.
# Source operations are pg_dump/SELECT only; no provider or lifecycle mutation.
param([Parameter(Mandatory=$true)][string]$PrivateDirectory,[Parameter(Mandatory=$true)][ValidatePattern('^[0-9]{2}$')][string]$CheckpointAttempt,[switch]$VerifyExistingAuth,[switch]$VerifyExistingFacts,[ValidateSet('01','02')][string]$Generation='01')
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'durable-storage.ps1')
$repoDirectory=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$private=(Resolve-Path -LiteralPath $PrivateDirectory).Path
$run='5d2274ee-a671-4bdc-b406-22b8dad8fd61'
$project='dpp-lifecycle307-5d2274ee'
$sourceName=$project+'-postgres'
$replicaName=$project+'-predelete-postgres'
$networkName=$project+'-predelete-private'
$image='sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea'
$backup=Join-Path $private ('predelete-core-checkpoint-'+$CheckpointAttempt)
$public=Join-Path $repoDirectory 'docs/acceptance/evidence/issue307-d1/delete-preflight-5d2274ee'
if($Generation -eq '02'){$replicaName+='-02';$networkName+='-02';$public+='-generation02'}
function Require($Condition,[string]$Message){if(-not $Condition){throw $Message}}
function Docker([string[]]$Arguments,[string]$InputText=''){
 $info=[Diagnostics.ProcessStartInfo]::new('docker');$info.UseShellExecute=$false;$info.CreateNoWindow=$true;$info.RedirectStandardOutput=$true;$info.RedirectStandardError=$true
 foreach($argument in $Arguments){$info.ArgumentList.Add($argument)}
 if($InputText -ne ''){$info.RedirectStandardInput=$true}
 $process=[Diagnostics.Process]::Start($info);$stdout=$process.StandardOutput.ReadToEndAsync();$stderr=$process.StandardError.ReadToEndAsync();if($InputText -ne ''){$process.StandardInput.Write($InputText);$process.StandardInput.Close()};$process.WaitForExit();Require ($process.ExitCode -eq 0) 'Checkpoint Docker command failed; resources retained';$stdout.GetAwaiter().GetResult().Trim()
}
function Write-Utf8([string]$Path,[string]$Text){[IO.File]::WriteAllText($Path,$Text.Replace("`r`n","`n"),[Text.UTF8Encoding]::new($false))}
function Export-Private([string[]]$Arguments,[string]$Path){
 $info=[Diagnostics.ProcessStartInfo]::new('docker');$info.UseShellExecute=$false;$info.CreateNoWindow=$true;$info.RedirectStandardOutput=$true;$info.RedirectStandardError=$true
 foreach($argument in $Arguments){$info.ArgumentList.Add($argument)}
 $process=[Diagnostics.Process]::Start($info);$errorTask=$process.StandardError.ReadToEndAsync();$stream=[IO.File]::Open($Path,[IO.FileMode]::CreateNew)
 try{$process.StandardOutput.BaseStream.CopyTo($stream)}finally{$stream.Dispose()}
 $process.WaitForExit();Write-Utf8 ($Path+'.private-log') $errorTask.GetAwaiter().GetResult();Require ($process.ExitCode -eq 0) 'Checkpoint private export failed; no source mutation'
}
function SQL([string]$Container,[string]$Query,[string]$Database='lifecycle307',[string]$User='fixture'){
 Docker -Arguments @('exec','-i',$Container,'psql','-X','-qAt','-v','ON_ERROR_STOP=1','-U',$User,'-d',$Database) -InputText $Query
}
function Fingerprint([string]$Container){
 $tables=SQL $Container "BEGIN READ ONLY; SELECT json_agg(table_name ORDER BY table_name) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE'; ROLLBACK;"|ConvertFrom-Json
 $parts=@();foreach($table in $tables){$identifier='"'+$table.Replace('"','""')+'"';$literal="'"+$table.Replace("'","''")+"'";$parts+="SELECT json_build_object('relation',$literal,'rows',COALESCE((SELECT jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text) FROM public.$identifier t),'[]'::jsonb))::text"}
 $raw=SQL $Container ('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY; SET LOCAL TIME ZONE ''UTC''; '+($parts -join ' UNION ALL ')+ '; ROLLBACK;')
 $fingerprints=foreach($line in $raw.Split("`n")){if(-not $line){continue};$record=$line|ConvertFrom-Json;$canonical=ConvertTo-Json -InputObject @($record.rows) -Depth 100 -Compress;$hash=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($canonical))).ToLowerInvariant();[pscustomobject]@{relation=$record.relation;rows=@($record.rows).Count;sha256=$hash}}
 @($fingerprints|Sort-Object relation)
}
function Verify-CheckpointAuth {
 $receipt=Get-Content -LiteralPath (Join-Path $public 'core-checkpoint.json') -Raw|ConvertFrom-Json
 Require ($receipt.status -eq 'PASS_CORE_CHECKPOINT_ONLY' -and $receipt.source_container_id -eq $source.Id -and $receipt.core_dump_sha256 -eq (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $backup 'core.dump')).Hash.ToLowerInvariant()) 'Only the exact completed checkpoint can be authenticated'
 $replica=(Docker @('inspect',$replicaName)|ConvertFrom-Json)[0]
 Require ($replica.Id -eq $receipt.replica_container_id -and $replica.Image -eq $image -and $replica.State.Status -eq 'exited') 'Stopped checkpoint identity changed'
 Assert-RetainedServiceStorage -RunID $run -LabelKey 'lifecycle307.run' -RegisteredVolumes @($replicaName) -Services @(@{role='checkpoint-core';container=$replica;name=$replicaName;path='/var/lib/postgresql/data';volume=$replicaName}) -AllowedStates @('exited')|Out-Null
 $dsn=[uri]$cfg.PostgresDSN;Require ($dsn.Scheme -in @('postgres','postgresql')) 'Explicit fixture DSN required';$identity=$dsn.UserInfo.Split(':',2);Require ($identity.Count -eq 2 -and $identity[0] -eq 'fixture') 'Fixture role only'
 $passwordFile=Join-Path $backup 'fixture-auth-password';Require (-not (Test-Path -LiteralPath $passwordFile)) 'Auth preparation already exists; preserve, no automatic resume';Write-Utf8 $passwordFile ([uri]::UnescapeDataString($identity[1]))
 $results=@()
 try{
  Docker @('start',$replicaName)|Out-Null
  $ready=$false;for($index=0;$index -lt 60;$index++){try{Docker @('exec',$replicaName,'pg_isready','-U','checkpoint_admin','-d','postgres')|Out-Null;$ready=$true;break}catch{Start-Sleep -Seconds 1}};Require $ready 'Checkpoint unavailable for remote auth'
  foreach($mode in @('positive','negative')){
   $name=$project+'-predelete-core-auth-'+$mode
   if($Generation -eq '02'){$name+='-02'}
   Require ((Docker @('ps','-aq','--filter',"name=^/$name$")) -eq '') 'Auth helper already exists; retained replay refused'
   $command='export PGPASSWORD="$(cat /run/auth/fixture-auth-password)"; exec psql -h '+$replicaName+' -U fixture -d lifecycle307 -v ON_ERROR_STOP=1 -qAt -c ''SELECT current_user'''
   if($mode -eq 'negative'){$command='export PGPASSWORD=invalid-checkpoint-password; exec psql -h '+$replicaName+' -U fixture -d lifecycle307 -v ON_ERROR_STOP=1 -qAt -c ''SELECT current_user'''}
   Docker @('create','--pull','never','--name',$name,'--label',"lifecycle307.run=$run",'--label','lifecycle307.role=checkpoint-auth-witness','--network',$networkName,'--read-only','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data','--mount',"type=bind,source=$backup,target=/run/auth,readonly",'--entrypoint','/bin/sh',$image,'-c',$command)|Out-Null
   Docker @('start',$name)|Out-Null;$exit=[int](Docker @('wait',$name))
   # Docker logs include stderr; retain privately, never echo credential-related diagnostics.
   Export-Private @('logs',$name) (Join-Path $backup ($mode+'-remote-auth.private-log'))
   $logText=Get-Content -LiteralPath (Join-Path $backup ($mode+'-remote-auth.private-log')) -Raw
   $errText=Get-Content -LiteralPath (Join-Path $backup ($mode+'-remote-auth.private-log.private-log')) -Raw
   if($mode -eq 'positive'){Require ($exit -eq 0 -and $logText.Trim() -eq 'fixture') 'Remote SCRAM positive authentication failed'}else{Require ($exit -eq 2 -and $errText -match 'password authentication failed') 'Invalid password was not rejected by remote authentication'}
   $helper=(Docker @('inspect',$name)|ConvertFrom-Json)[0];$results+=@{mode=$mode;container=$name;id=$helper.Id;image_id=$helper.Image;exit=$exit;status='PASS';host_ports=@();credential_values_disclosed=0}
  }
 }finally{Docker @('stop','--time','30',$replicaName)|Out-Null}
 $output=Join-Path $public 'core-checkpoint-auth.json';Require (-not (Test-Path -LiteralPath $output)) 'Append-only auth receipt exists'
 Write-Utf8 $output (@{status='PASS_REMOTE_SCRAM_POSITIVE_AND_NEGATIVE';as_of=[DateTimeOffset]::UtcNow.ToString('o');environment_run_id=$run;replica_id=$replica.Id;network_id=$receipt.network_id;checks=$results;replica_state='STOPPED_RETAINED_NO_BUSINESS_ADMISSION';source_core_mutation=0;source_restart=0;provider_io=0;delete=0;cleanup=0;credential_values_disclosed=0}|ConvertTo-Json -Depth 8)
 Write-Output 'PASS remote SCRAM positive/negative; checkpoint stopped/retained; source/provider mutation=0'
}
function Verify-CheckpointFacts {
 $sourceHash=(Get-FileHash -Algorithm SHA256 -LiteralPath $PSCommandPath).Hash.ToLowerInvariant()
 $receipt=Get-Content -LiteralPath (Join-Path $public 'core-checkpoint.json') -Raw|ConvertFrom-Json
 $replica=(Docker @('inspect',$replicaName)|ConvertFrom-Json)[0]
 Require ($receipt.source_container_id -eq $source.Id -and $replica.Id -eq $receipt.replica_container_id -and $replica.Image -eq $image -and $replica.State.Status -eq 'exited') 'Exact stopped checkpoint required'
 Require ($receipt.core_dump_sha256 -eq (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $backup 'core.dump')).Hash.ToLowerInvariant()) 'Original private dump changed'
 Assert-RetainedServiceStorage -RunID $run -LabelKey 'lifecycle307.run' -RegisteredVolumes @($replicaName) -Services @(@{role='checkpoint-core';container=$replica;name=$replicaName;path='/var/lib/postgresql/data';volume=$replicaName}) -AllowedStates @('exited')|Out-Null
 try{
  Docker @('start',$replicaName)|Out-Null
  $ready=$false;for($index=0;$index -lt 60;$index++){try{Docker @('exec',$replicaName,'pg_isready','-U','checkpoint_admin','-d','postgres')|Out-Null;$ready=$true;break}catch{Start-Sleep -Seconds 1}};Require $ready 'Checkpoint unavailable'
  $currentSource=Fingerprint $sourceName;$currentReplica=Fingerprint $replicaName
  $expected=$receipt.relation_fingerprints|ConvertTo-Json -Depth 8 -Compress
  Require (($currentSource|ConvertTo-Json -Depth 8 -Compress) -ceq $expected -and ($currentReplica|ConvertTo-Json -Depth 8 -Compress) -ceq $expected) 'Current source/replica differ from original checkpoint'
 }finally{Docker @('stop','--time','30',$replicaName)|Out-Null}
 Require ((Get-FileHash -Algorithm SHA256 -LiteralPath $PSCommandPath).Hash.ToLowerInvariant() -eq $sourceHash) 'Verification source changed during run'
 $output=Join-Path $public 'core-checkpoint-final-readback.json';Require (-not (Test-Path -LiteralPath $output)) 'Append-only verification exists'
 $dumpSource='NOT_CAPTURED; final readback source is separately bound, not a claim of final-source re-dump/re-restore'
 if($Generation -eq '02'){Require ($receipt.generation -eq '02' -and $receipt.script_checkout_sha256 -eq $sourceHash -and $receipt.script_unchanged_during_run) 'Generation02 dump/restore and final readback source must match';$dumpSource=$receipt.script_checkout_sha256}
 Write-Utf8 $output (@{status='PASS_FINAL_READ_ONLY_SOURCE_AND_REPLICA';as_of=[DateTimeOffset]::UtcNow.ToString('o');script_checkout_sha256=$sourceHash;script_unchanged_during_run=$true;dump_sha256=$receipt.core_dump_sha256;relations=$currentSource.Count;rows=($currentSource|Measure-Object rows -Sum).Sum;all_fingerprints_match_original_checkpoint=$true;replica_state='STOPPED_RETAINED_NO_BUSINESS_ADMISSION';original_dump_restore_script_hash=$dumpSource;source_core_mutation=0;source_restart=0;provider_io=0;delete=0;cleanup=0}|ConvertTo-Json -Depth 8)
 Write-Output 'PASS final read-only source/replica fingerprints; final verification source bound separately'
}
Require ($private -eq 'C:\Users\Henry\.codex\fixture-private\dpp-lifecycle307-5d2274ee') 'Only the exact proposal private root is supported'
Require (-not ($VerifyExistingAuth -and $VerifyExistingFacts)) 'One explicit verification stage only'
if(-not $VerifyExistingAuth -and -not $VerifyExistingFacts){Require (-not (Test-Path -LiteralPath $backup)) 'Append-only checkpoint already exists; no automatic resume/overwrite'}
$noSendReceipt='trial-no-send-candidate.json';if($Generation -eq '02'){$noSendReceipt='trial-no-send-applied.json'}
Require (Test-Path -LiteralPath (Join-Path $public $noSendReceipt)) 'Current scoped no-send receipt required'
$source=(Docker @('inspect',$sourceName)|ConvertFrom-Json)[0]
Require ($source.Id -eq 'c85b9ff408ed110f531ac79d27a3b99f66242d12766e1674e3de78067d140bd0' -and $source.Image -eq $image) 'Source identity changed'
Assert-RetainedServiceStorage -RunID $run -LabelKey 'lifecycle307.run' -RegisteredVolumes @($sourceName) -Services @(@{role='source-core';container=$source;name=$sourceName;path='/var/lib/postgresql/data';volume=$sourceName})|Out-Null
$cfg=Get-Content -LiteralPath (Join-Path $private 'config.json') -Raw|ConvertFrom-Json
Require (-not $cfg.Enabled -and $cfg.RunID -eq $run) 'Original window must remain sealed'
if($VerifyExistingAuth){Verify-CheckpointAuth;return}
if($VerifyExistingFacts){Verify-CheckpointFacts;return}
Require ((Docker @('ps','-aq','--filter',"name=^/$replicaName$")) -eq '' -and (Docker @('volume','ls','-q','--filter',"name=^$replicaName$")) -eq '') 'Checkpoint resource name already owned'
Require ((Docker @('network','ls','-q','--filter',"name=^$networkName$")) -eq '') 'Checkpoint network already exists'
New-Item -ItemType Directory -Path $backup|Out-Null
$checkpointSourceHash=(Get-FileHash -Algorithm SHA256 -LiteralPath $PSCommandPath).Hash.ToLowerInvariant()
if($Generation -eq '02'){$nativeClosure=Get-Content -LiteralPath (Join-Path $repoDirectory 'docs/acceptance/evidence/issue307-d1/reference-disposal-preparation01/native-restore-closure.json') -Raw|ConvertFrom-Json;Require ($nativeClosure.status -eq 'PASS_EXACT_SIX_NATIVE_RESTORE' -and $nativeClosure.versions -eq 6) 'Generation02 requires completed exact native restore'}
$sourceBefore=Fingerprint $sourceName
Write-Utf8 (Join-Path $backup 'source-before.private.json') ($sourceBefore|ConvertTo-Json -Depth 8)
Export-Private @('exec',$sourceName,'pg_dump','-U','fixture','-d','lifecycle307','--format=custom','--create') (Join-Path $backup 'core.dump')
Export-Private @('exec',$sourceName,'pg_dumpall','-U','fixture','--globals-only') (Join-Path $backup 'globals.sql')
$passwordBytes=[byte[]]::new(32);[Security.Cryptography.RandomNumberGenerator]::Fill($passwordBytes)
Write-Utf8 (Join-Path $backup 'checkpoint-password') ([Convert]::ToHexString($passwordBytes).ToLowerInvariant())
$subnet='10.234.152.0/24';if($Generation -eq '02'){$subnet='10.234.153.0/24'}
$checkpointNetwork=Docker @('network','create','--internal','--subnet',$subnet,'--label',"lifecycle307.run=$run",'--label','lifecycle307.role=predelete-checkpoint',$networkName)
Docker @('volume','create','--label',"lifecycle307.run=$run",'--label','lifecycle307.role=predelete-core',$replicaName)|Out-Null
Docker @('create','--pull','never','--name',$replicaName,'--label',"lifecycle307.run=$run",'--label','lifecycle307.role=predelete-core','--network',$networkName,'--mount',"type=volume,source=$replicaName,target=/var/lib/postgresql/data",'--mount',"type=bind,source=$backup,target=/run/backup,readonly",'--env','POSTGRES_USER=checkpoint_admin','--env','POSTGRES_DB=postgres','--env','POSTGRES_PASSWORD_FILE=/run/backup/checkpoint-password','--env','POSTGRES_HOST_AUTH_METHOD=scram-sha-256',$image)|Out-Null
$replica=(Docker @('inspect',$replicaName)|ConvertFrom-Json)[0]
Assert-RetainedServiceStorage -RunID $run -LabelKey 'lifecycle307.run' -RegisteredVolumes @($replicaName) -Services @(@{role='checkpoint-core';container=$replica;name=$replicaName;path='/var/lib/postgresql/data';volume=$replicaName}) -AllowedStates @('created')|Out-Null
Docker @('start',$replicaName)|Out-Null
$ready=$false;for($index=0;$index -lt 60;$index++){try{Docker @('exec',$replicaName,'pg_isready','-U','checkpoint_admin','-d','postgres')|Out-Null;$ready=$true;break}catch{Start-Sleep -Seconds 1}}
Require $ready 'Retained checkpoint not ready; no source restart or cleanup'
Export-Private @('exec',$replicaName,'psql','-X','-U','checkpoint_admin','-d','postgres','-v','ON_ERROR_STOP=1','-f','/run/backup/globals.sql') (Join-Path $backup 'globals-restore.private-log')
Export-Private @('exec',$replicaName,'pg_restore','--exit-on-error','--create','-U','checkpoint_admin','-d','postgres','/run/backup/core.dump') (Join-Path $backup 'core-restore.private-log')
$restored=Fingerprint $replicaName
$sourceAfter=Fingerprint $sourceName
$beforeJSON=$sourceBefore|ConvertTo-Json -Depth 8 -Compress
Require (($restored|ConvertTo-Json -Depth 8 -Compress) -ceq $beforeJSON) 'Independent restored Core facts differ'
Require (($sourceAfter|ConvertTo-Json -Depth 8 -Compress) -ceq $beforeJSON) 'Source Core changed during checkpoint'
$roleQuery="BEGIN READ ONLY; SELECT json_build_object('name',rolname,'super',rolsuper,'inherit',rolinherit,'create_role',rolcreaterole,'create_db',rolcreatedb,'login',rolcanlogin,'replication',rolreplication,'connections',rolconnlimit,'password',rolpassword,'valid_until',rolvaliduntil,'bypass_rls',rolbypassrls)::text FROM pg_authid WHERE rolname='fixture'; ROLLBACK;"
$sourceRole=SQL $sourceName $roleQuery
$restoredRole=SQL $replicaName $roleQuery
Require ($sourceRole -ceq $restoredRole) 'Fixture role/SCRAM verifier differs'
$aclQuery="BEGIN READ ONLY; SELECT json_agg(json_build_object('name',c.relname,'kind',c.relkind,'owner',r.rolname,'acl',c.relacl::text,'rls',c.relrowsecurity) ORDER BY c.relname,c.relkind) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_roles r ON r.oid=c.relowner WHERE n.nspname='public'; ROLLBACK;"
Require ((SQL $sourceName $aclQuery) -ceq (SQL $replicaName $aclQuery)) 'Object owner/ACL differs'
$replica=(Docker @('inspect',$replicaName)|ConvertFrom-Json)[0]
$durability=Assert-RetainedServiceStorage -RunID $run -LabelKey 'lifecycle307.run' -RegisteredVolumes @($replicaName) -Services @(@{role='checkpoint-core';container=$replica;name=$replicaName;path='/var/lib/postgresql/data';volume=$replicaName})
Require (@($replica.HostConfig.PortBindings.PSObject.Properties).Count -eq 0) 'Checkpoint host port published'
Docker @('stop','--time','30',$replicaName)|Out-Null
$result=[ordered]@{status='PASS_CORE_CHECKPOINT_ONLY';as_of=[DateTimeOffset]::UtcNow.ToString('o');environment_run_id=$run;source_container_id=$source.Id;replica_container_id=$replica.Id;replica=$replicaName;named_volume=$replicaName;image_id=$image;network_id=$checkpointNetwork;network=$networkName;internal=$true;host_ports=@();relations=$sourceBefore.Count;rows=($sourceBefore|Measure-Object rows -Sum).Sum;relation_fingerprints=$sourceBefore;independent_restore_equal=$true;source_facts_unchanged=$true;fixture_role_scram_equal=$true;object_owner_acl_equal=$true;durability=$durability;replica_state='STOPPED_RETAINED_NO_BUSINESS_ADMISSION';core_dump_sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $backup 'core.dump')).Hash.ToLowerInvariant();globals_dump_sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $backup 'globals.sql')).Hash.ToLowerInvariant();source_core_mutation=0;source_restart=0;provider_io=0;delete=0;cleanup=0;native_six_version_backup_restore='NOT_RUN; separate required pre-delete condition';remote_scram_positive_negative='NOT_RUN; verifier equality is not a remote authentication test'}
if($Generation -eq '02'){Require ((Get-FileHash -Algorithm SHA256 -LiteralPath $PSCommandPath).Hash.ToLowerInvariant() -eq $checkpointSourceHash) 'Checkpoint source changed during run';$result['script_checkout_sha256']=$checkpointSourceHash;$result['script_unchanged_during_run']=$true;$result['native_six_version_backup_restore']='PASS_EXACT_SIX_NATIVE_RESTORE; receipt bound separately';$result['generation']='02'}
$output=Join-Path $public 'core-checkpoint.json';Require (-not (Test-Path -LiteralPath $output)) 'Checkpoint receipt already exists';Write-Utf8 $output ($result|ConvertTo-Json -Depth 12)
Write-Output ([pscustomobject]@{status=$result.status;relations=$result.relations;rows=$result.rows;replica_state=$result.replica_state;source_mutation=0;provider_io=0;delete=0}|ConvertTo-Json -Compress)
