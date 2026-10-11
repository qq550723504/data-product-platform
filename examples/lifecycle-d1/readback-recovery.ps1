param([Parameter(Mandatory=$true)][string]$PrivateDirectory,[Parameter(Mandatory=$true)][string]$Phase)
$ErrorActionPreference='Stop';Set-StrictMode -Version Latest
if($Phase -notmatch '^[a-z][a-z0-9-]{0,63}$'){throw 'Phase invalid'}
$private=(Resolve-Path -LiteralPath $PrivateDirectory).Path;$r=Get-Content -LiteralPath (Join-Path $private 'resources.json') -Raw|ConvertFrom-Json
$output=Join-Path $PSScriptRoot "results/$($r.run_id)/db-$Phase.json"
if(Test-Path -LiteralPath $output){throw 'Append-only readback file already exists'}
$sql=@'
SELECT json_build_object(
 'counts',json_build_object(
 'work',(SELECT count(*) FROM lifecycle_reconciliation_work),
 'runs',(SELECT count(*) FROM lifecycle_reconciliation_run),
 'results',(SELECT count(*) FROM lifecycle_reconciliation_result),
 'provider_attempts',(SELECT count(*) FROM lifecycle_provider_attempt),
 'provider_send_claims',(SELECT count(*) FROM lifecycle_provider_send_claim),
 'provider_observations',(SELECT count(*) FROM lifecycle_provider_observation),
 'released_reservations',(SELECT count(*) FROM lifecycle_dispatch_release),
 'unresolved_reservations',(SELECT count(*) FROM lifecycle_disposition_dispatch d WHERE NOT EXISTS(SELECT 1 FROM lifecycle_dispatch_release r WHERE r.dispatch_id=d.id) AND NOT EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt r WHERE r.dispatch_id=d.id)),
 'absence_receipts',(SELECT count(*) FROM lifecycle_copy_absence_receipt),
 'plan_completions',(SELECT count(*) FROM lifecycle_plan_completion),
 'storage_starts',(SELECT count(*) FROM lifecycle_storage_call),
 'storage_observations',(SELECT count(*) FROM lifecycle_storage_observation),
 'storage_cost_allocations',(SELECT count(*) FROM cost_allocation WHERE lifecycle_storage_call_id IS NOT NULL),
 'delete_cost_allocations',(SELECT count(*) FROM cost_allocation WHERE lifecycle_provider_attempt_id IS NOT NULL),
 'audit',(SELECT count(*) FROM audit_event),'evidence',(SELECT count(*) FROM evidence),'outbox',(SELECT count(*) FROM outbox_event)),
 'work',(SELECT json_agg(json_build_object('id',id,'attempt_id',attempt_id,'basis_ref',basis_ref,'requested_at',requested_at) ORDER BY requested_at,id) FROM lifecycle_reconciliation_work),
 'runs',(SELECT json_agg(json_build_object('id',id,'work_id',work_id,'started_at',started_at) ORDER BY started_at,id) FROM lifecycle_reconciliation_run),
 'results',(SELECT json_agg(json_build_object('id',id,'work_id',work_id,'run_id',run_id,'outcome',outcome,'terminal',terminal,'inventory_call_id',inventory_call_id,'stat_call_id',stat_call_id,'absence_receipt_id',absence_receipt_id,'completion_id',completion_id) ORDER BY observed_at,id) FROM lifecycle_reconciliation_result),
 'outbox_wakes',(SELECT json_agg(json_build_object('id',id,'work_id',aggregate_id,'status',status,'required_handlers',required_handlers,'routing_version',routing_version,'attempts',attempts,'confirmations',(SELECT count(*) FROM outbox_event_consumption c WHERE c.event_id=e.id)) ORDER BY created_at,id) FROM outbox_event e WHERE event_type='LifecycleDispositionRecoveryRequested')
);
'@
$result=& docker.exe exec $r.containers.postgres psql -U fixture -d lifecycle307 -v ON_ERROR_STOP=1 -At -c $sql
if($LASTEXITCODE -ne 0){throw 'Read-only DB query failed'}
$parsed=$result|ConvertFrom-Json
[IO.File]::WriteAllText($output,($parsed|ConvertTo-Json -Depth 20),[Text.UTF8Encoding]::new($false))
$parsed.counts|ConvertTo-Json
