param([Parameter(Mandatory=$true)][string]$PrivateDirectory,[Parameter(Mandatory=$true)][string]$Scenario)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if($Scenario -notmatch '^[a-z0-9-]{1,64}$'){throw 'Bounded evidence name required'}
$private=(Resolve-Path -LiteralPath $PrivateDirectory).Path
$r=Get-Content -LiteralPath (Join-Path $private 'resources.json') -Raw|ConvertFrom-Json
$run=([guid]$r.run_id).ToString();$project='dpp-lifecycle307-'+$run.Substring(0,8)
if($r.project -ne $project -or (Split-Path -Leaf $private) -ne $project){throw 'Owned environment required'}
$output=Join-Path $PSScriptRoot "results/$run/$Scenario.json"
if(Test-Path -LiteralPath $output){throw 'Append-only evidence; refuse overwrite'}
$sql=@'
SELECT json_build_object(
 'observed_at',clock_timestamp(),'schema_version',(SELECT max(version) FROM schema_migration),
 'originals',(SELECT json_agg(row_to_json(x)) FROM (SELECT v.id,v.status,v.storage_uri,v.checksum_value,v.byte_size FROM dataset_version v ORDER BY v.id)x),
 'copies',(SELECT json_agg(row_to_json(x)) FROM (SELECT c.id,c.dataset_version_id,n.namespace,c.object_key,c.provider_version,c.content_sha256,c.byte_size FROM lifecycle_fixture_copy c JOIN lifecycle_fixture_namespace n ON n.id=c.namespace_id ORDER BY n.namespace,c.object_key,c.provider_version)x),
 'operations',(SELECT json_agg(row_to_json(x)) FROM (SELECT o.id,o.action,o.source_copy_id,o.target_copy_id,o.target_key,c.provider_version,m.discovery_id,EXISTS(SELECT 1 FROM lifecycle_copy_verification v WHERE v.operation_id=o.id) verified,(SELECT count(*) FROM lifecycle_storage_call s WHERE s.copy_operation_id=o.id AND s.operation IN('ARCHIVE_COPY','RESTORE_COPY')) original_send_count FROM lifecycle_copy_operation o LEFT JOIN lifecycle_copy_materialization m ON m.operation_id=o.id LEFT JOIN lifecycle_fixture_copy c ON c.id=m.copy_id ORDER BY o.created_at)x),
 'original_calls',(SELECT json_agg(row_to_json(x)) FROM (SELECT c.id,c.copy_operation_id,c.operation,c.started_at,o.http_status,o.failure,o.copy_result_version,o.invoked_at,(SELECT count(*) FROM cost_allocation a WHERE a.lifecycle_storage_call_id=c.id) cost_allocations FROM lifecycle_storage_call c LEFT JOIN lifecycle_storage_observation o ON o.call_id=c.id WHERE c.operation IN('ARCHIVE_COPY','RESTORE_COPY') ORDER BY c.started_at)x),
 'work',(SELECT json_agg(row_to_json(x)) FROM (SELECT w.id,w.copy_call_id,w.basis_ref,(SELECT count(*) FROM lifecycle_reconciliation_run r WHERE r.work_id=w.id) runs,(SELECT status FROM outbox_event e WHERE e.aggregate_id=w.id AND e.event_type='LifecycleCopyRecoveryRequested') outbox_status,(SELECT count(*) FROM outbox_event_consumption c JOIN outbox_event e ON e.id=c.event_id WHERE e.aggregate_id=w.id AND e.event_type='LifecycleCopyRecoveryRequested') confirmations FROM lifecycle_reconciliation_work w ORDER BY w.requested_at)x),
 'results',(SELECT json_agg(row_to_json(x)) FROM (SELECT work_id,run_id,outcome,terminal,inventory_call_id,stat_call_id,copy_discovery_id,copy_decision_id,observed_at FROM lifecycle_reconciliation_result ORDER BY observed_at)x),
 'decisions',(SELECT json_agg(row_to_json(x)) FROM (SELECT id,operation_id,decision,reason,checkpoint_id,dependency_revision,occurred_at FROM lifecycle_copy_decision ORDER BY occurred_at)x),
 'physical_calls',(SELECT count(*) FROM lifecycle_storage_call),
 'observations',(SELECT count(*) FROM lifecycle_storage_observation),
 'known_invocations',(SELECT count(*) FROM lifecycle_storage_observation WHERE invoked_at IS NOT NULL),
 'physical_cost_allocations',(SELECT count(*) FROM cost_allocation WHERE lifecycle_storage_call_id IS NOT NULL),
 'unknown_without_cost',(SELECT count(*) FROM lifecycle_storage_call c WHERE NOT EXISTS(SELECT 1 FROM lifecycle_storage_observation o WHERE o.call_id=c.id AND invoked_at IS NOT NULL) AND NOT EXISTS(SELECT 1 FROM cost_allocation a WHERE a.lifecycle_storage_call_id=c.id)),
 'native_delete_attempts',(SELECT count(*) FROM lifecycle_provider_attempt),
 'absence_receipts',(SELECT count(*) FROM lifecycle_copy_absence_receipt),
 'copy_discoveries',(SELECT count(*) FROM lifecycle_copy_discovery),
 'native_lookup_calls',(SELECT count(*) FROM lifecycle_storage_call WHERE operation IN('RECOVER_COPY_VERSIONS','RECOVER_COPY_EXACT')),
 'verify_calls',(SELECT count(*) FROM lifecycle_storage_call WHERE operation='VERIFY_COPY_BYTES')
);
'@
$body=& docker.exe exec $r.containers.postgres psql -U fixture -d lifecycle307 -At -c $sql 2>&1
if($LASTEXITCODE -ne 0){throw 'Read-only PostgreSQL snapshot failed'}
$facts=($body -join "`n")|ConvertFrom-Json
if($facts.schema_version -ne 64){throw 'Current controlled schema64 required'}
[IO.File]::WriteAllText($output,($facts|ConvertTo-Json -Depth 20),[Text.UTF8Encoding]::new($false))
Write-Output "Read-only evidence $Scenario saved: calls=$($facts.physical_calls), costs=$($facts.physical_cost_allocations), DELETE attempts=$($facts.native_delete_attempts)."
