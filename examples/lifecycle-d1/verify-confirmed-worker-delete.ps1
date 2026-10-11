# Read-only closeout of the already-consumed human confirmation. Never calls S3.
param()
$ErrorActionPreference='Stop';Set-StrictMode -Version Latest
$run='4abcfca5-583d-41ca-8548-10a0fc3270a1';$project='dpp-lifecycle307-4abcfca5'
$private='C:\Users\Henry\.codex\worktrees\issue307-lifecycle-d1\private-fixtures\dpp-lifecycle307-4abcfca5'
$output=Join-Path $PSScriptRoot "results/$run"
function Write-Json([string]$Name,$Value){$path=Join-Path $output $Name;if(Test-Path -LiteralPath $path){throw 'Append-only evidence exists'};[IO.File]::WriteAllText($path,($Value|ConvertTo-Json -Depth 30),[Text.UTF8Encoding]::new($false))}
function Docker([string[]]$Arguments){$result=& docker.exe @Arguments 2>&1;if($LASTEXITCODE -ne 0){throw "Read-only Docker $($Arguments[0]) failed"};return ($result -join "`n").Trim()}
$sql=@'
SELECT json_build_object(
 'attempts',(SELECT json_agg(row_to_json(a)) FROM lifecycle_provider_attempt a),
 'send_claims',(SELECT json_agg(row_to_json(s)) FROM lifecycle_provider_send_claim s),
 'provider_observations',(SELECT json_agg(row_to_json(o)) FROM lifecycle_provider_observation o),
 'receipts',(SELECT json_agg(row_to_json(r)) FROM lifecycle_copy_absence_receipt r),
 'completions',(SELECT json_agg(row_to_json(c)) FROM lifecycle_plan_completion c),
 'core_versions',(SELECT json_agg(json_build_object('id',v.id,'status',v.status,'storage_uri',v.storage_uri,'checksum_algorithm',v.checksum_algorithm,'checksum_value',v.checksum_value,'byte_size',v.byte_size)) FROM dataset_version v WHERE v.id IN (SELECT dataset_version_id FROM lifecycle_fixture_copy)),
 'storage_cost_matches',(SELECT bool_and((SELECT count(*) FROM cost_allocation a WHERE a.lifecycle_storage_call_id=c.id)=1 AND (SELECT count(*) FROM lifecycle_storage_observation o WHERE o.call_id=c.id)=1) FROM lifecycle_storage_call c),
 'native_delete_costs',(SELECT json_agg(json_build_object('id',a.id,'attempt_id',a.lifecycle_provider_attempt_id,'cost_event_id',a.cost_event_id)) FROM cost_allocation a WHERE a.lifecycle_provider_attempt_id IS NOT NULL),
 'unresolved_attempts',(SELECT json_agg(row_to_json(a)) FROM lifecycle_provider_attempt a WHERE NOT EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt r WHERE r.dispatch_id=a.dispatch_id) AND NOT EXISTS(SELECT 1 FROM lifecycle_dispatch_release r WHERE r.dispatch_id=a.dispatch_id)),
 'fact_events',(SELECT json_agg(json_build_object('type',e.event_type,'aggregate_id',e.aggregate_id,'status',e.status,'routing_version',e.routing_version)) FROM outbox_event e WHERE e.aggregate_id IN (SELECT id FROM lifecycle_copy_absence_receipt UNION SELECT id FROM lifecycle_plan_completion UNION SELECT id FROM lifecycle_reconciliation_result WHERE work_id='67eaf55d-a4a3-4e05-8f0f-12d1e57001a5'))
);
'@
$facts=(Docker @('exec',"$project-postgres",'psql','-U','fixture','-d','lifecycle307','-v','ON_ERROR_STOP=1','-At','-c',$sql))|ConvertFrom-Json
if(@($facts.send_claims).Count -ne 1 -or @($facts.provider_observations).Count -ne 1 -or $facts.provider_observations[0].attempt_id -ne '13323038-1024-49f8-b5cd-a8ffe4c4ea4e' -or $facts.provider_observations[0].certainty -ne 'CONFIRMED' -or $facts.provider_observations[0].outcome -ne 'ACCEPTED' -or @($facts.native_delete_costs).Count -ne 1 -or -not $facts.storage_cost_matches -or @($facts.receipts).Count -ne 1 -or $facts.receipts[0].copy_id -ne '171d1d1b-03dd-4fc9-a801-47044992cb89' -or @($facts.completions).Count -ne 1 -or @($facts.unresolved_attempts).Count -ne 1 -or $facts.unresolved_attempts[0].copy_id -ne '490e8615-11b7-4042-9183-181c1b8d5938'){throw 'Exact delete/recovery facts or original UNKNOWN reservation changed'}
$manifest=Get-Content (Join-Path $output 'manifest.json') -Raw|ConvertFrom-Json
if(@($facts.core_versions).Count -ne 2){throw 'Historical Core membership changed'}
foreach($v in $facts.core_versions){$copy=@($manifest.objects|Where-Object {$_.dataset_version_id -eq $v.id -and $_.namespace -eq 'dpp-lifecycle307-online'})[0];if($v.status -ne 'READY' -or $v.storage_uri -ne "s3://$($copy.namespace)/$($copy.key)" -or $v.checksum_algorithm -ne 'SHA256' -or $v.checksum_value -ne $copy.sha256 -or $v.byte_size -ne $copy.bytes){throw 'Historical Core content facts changed'}}
Write-Json 'real-delete-exact-facts.json' $facts
$before=Get-Content (Join-Path $output 'real-delete-witness-before.json') -Raw|ConvertFrom-Json
$after=Get-Content (Join-Path $output 'real-delete-survivors-after.json') -Raw|ConvertFrom-Json
if(@($after.objects).Count -ne 3 -or @($after.receipt_backed_absent).Count -ne 1){throw 'Survivor scope changed'}
foreach($item in $after.objects){$old=@($before.objects|Where-Object {$_.copy.ID -eq $item.copy.ID});if($old.Count -ne 1 -or ($old[0].copy|ConvertTo-Json -Compress) -ne ($item.copy|ConvertTo-Json -Compress) -or $old[0].protection.LegalHold -ne $item.protection.LegalHold -or $old[0].protection.RetentionMode -ne $item.protection.RetentionMode -or $old[0].protection.RetainUntil -ne $item.protection.RetainUntil){throw 'Survivor identity/protection changed'}}
$bootstrap=Get-Content (Join-Path $private 'config.json') -Raw|ConvertFrom-Json
$permission=Get-Content (Join-Path $private 'delete-permission.json') -Raw|ConvertFrom-Json
$recovery=Get-Content (Join-Path $private 'worker/recovery.json') -Raw|ConvertFrom-Json
$sealed=(Get-FileHash -LiteralPath (Join-Path $private 'Caddyfile') -Algorithm SHA256).Hash.ToLowerInvariant()
if($bootstrap.Enabled -or $permission.Enabled -or $recovery.Enabled -or $sealed -ne '81b26869b3ee1074035798a4bb6ba8c455ec71d69b57b7a7434d0da4d6011662'){throw 'A native mutation/authority window remains open'}
$policy=Get-Content (Join-Path $output 'delete-executor-policy.json') -Raw|ConvertFrom-Json
if(@($policy.Statement).Count -ne 1 -or $policy.Statement[0].Resource[0] -ne 'arn:aws:s3:::dpp-lifecycle307-quarantine/fixture/b.csv' -or $policy.Statement[0].Condition.StringEquals.'s3:versionid' -ne 'ce87f789-a4f4-4ce7-bf1f-a044ac53d1d2' -or (@($policy.Statement[0].Action)-join ',') -ne 's3:DeleteObject,s3:DeleteObjectVersion'){throw 'Native policy exceeds approved scope'}
$baseline=Get-Content 'C:\Users\Henry\.codex\worktrees\issue307-lifecycle-d1\worker-delete-preservation-before.json' -Raw|ConvertFrom-Json
$currentIDs=@((Docker @('ps','-aq','--no-trunc','--filter','label=lifecycle307.run')).Split("`n")|Where-Object {$_})
$services=@($baseline.containers|Where-Object {$_.status -eq 'running'})
$inspected=@((Docker (@('inspect')+@($services.Id)))|ConvertFrom-Json)
foreach($c in $services){$a=@($inspected|Where-Object {$_.Id -eq $c.Id});if($a.Count -ne 1 -or $a[0].State.Status -ne 'running' -or $a[0].Image -ne $c.Image -or ($a[0].Mounts|Select-Object Type,Name,Destination|ConvertTo-Json -Compress) -ne ($c.mounts|ConvertTo-Json -Compress)){throw 'Retained fixture service identity/state/mount changed'}}
$volumes=@((Docker @('volume','ls','-q')).Split("`n"))
$owned=@()
foreach($prefix in @('a6b30840','a1647373','bb44901c','c9f47cab','4abcfca5')){$owned+="dpp-lifecycle307-$prefix-postgres";$owned+="dpp-lifecycle307-$prefix-objects"};$owned+="$project-redis-data"
foreach($name in $owned){if($name -notin $volumes){throw 'Owned fixture volume missing'}}
$missing=@($baseline.volumes|Where-Object {$_ -notin $volumes})
$missingHelpers=@($baseline.containers|Where-Object {$_.Id -notin $currentIDs}|Select-Object Id,Name,status)
Write-Json 'real-delete-preservation.json' @{as_of=[datetime]::UtcNow.ToString('o');retained_running_service_count=$services.Count;owned_fixture_volumes=$owned;owned_fixture_volume_retention='PASS';survivor_identity_and_protection='PASS';gateway_sha256=$sealed;all_authority_windows='DISABLED';all_native_mutations='DENY_ALL';object_cleanup_requests=0;volume_cleanup_requests=0;all_prior_global_volume_count=$baseline.volumes.Count;current_global_volume_count=$volumes.Count;missing_nonfixture_global_volumes=$missing;missing_preexisting_exited_helpers=$missingHelpers;global_docker_inventory='DRIFT_OBSERVED_CALLER_UNKNOWN';helper_retention='FAIL';note='No cleanup command was issued by this run; fixture services/objects and all 11 owned named volumes are retained. Global inventory and exited-helper retention must not be reported unchanged.'}
$admin=@(Get-Content (Join-Path $private 'executor-admin-receipts.jsonl')|ForEach-Object {$_|ConvertFrom-Json})
Write-Json 'real-delete-admin-receipts.json' $admin
Write-Output 'PASS: one consumed exact deletion; receipt/completion and costs matched; three survivor identities/protections and 11 fixture volumes retained. Global Docker/helper drift recorded separately.'
