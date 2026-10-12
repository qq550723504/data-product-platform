param([Parameter(Mandatory=$true)][string]$RunDirectory)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$dir=(Resolve-Path -LiteralPath $RunDirectory).Path
$repo=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
function Read-JSON([string]$Name){Get-Content -LiteralPath (Join-Path $dir $Name) -Raw|ConvertFrom-Json}
function Require([bool]$Condition,[string]$Message){if(-not $Condition){throw $Message}}
$m=Read-JSON 'manifest.json';$summary=Read-JSON 'plane-acceptance-summary.json';$sources=Read-JSON 'plane-candidate-sources.json';$inventory=@(Read-JSON 'plane-final-inventory.json');$calls=@(Read-JSON 'plane-final-calls.json');$observations=@(Read-JSON 'plane-final-observations.json');$pins=@(Read-JSON 'plane-final-pins.json');$stops=@(Read-JSON 'plane-final-terminations.json');$transfers=@(Read-JSON 'plane-final-transfers.json');$native=Read-JSON 'plane-final-native-inspection.json';$account=Read-JSON 'plane-independent-accounting.json';$seal=Read-JSON 'plane-seal.json'
Require ($sources.run_id -eq $m.run_id -and (Split-Path -Leaf $dir) -eq $m.run_id) 'Run binding mismatch'
foreach($f in $sources.source_hashes.PSObject.Properties){Require ((Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $repo $f.Name)).Hash.ToLowerInvariant() -eq $f.Value) ('Source changed: '+$f.Name)}
foreach($key in @('native_processing','direct_delivery','same_key_replay_zero_io','queued_reverse_restriction_zero_io','natural_expiry_actual_tcp_slow_client','expired_new_delivery_zero_io')){Require ($summary.$key -eq 'PASS') ('Acceptance failed: '+$key)}
Require ($inventory.Count -eq 6 -and @($native.objects).Count -eq 6 -and @($native.inventories).Count -eq 2) 'Incomplete exact native inventory'
foreach($original in $m.objects){Require (@($inventory|Where-Object {$_.namespace -eq $original.namespace -and $_.object_key -eq $original.key -and $_.provider_version -eq $original.version_id -and $_.content_sha256 -eq $original.sha256 -and $_.byte_size -eq $original.bytes}).Count -eq 1) 'Original identity changed'}
Require ($calls.Count -eq 40 -and $observations.Count -eq 40 -and $account.calls -eq 40 -and $account.observations -eq 40 -and $account.invocations -eq 40 -and $account.storage_cost_allocations -eq 40) 'Storage/cost mismatch'
foreach($call in $calls){Require (@($observations|Where-Object {$_.call_id -eq $call.id}).Count -eq 1) 'Missing or repeated physical observation';Require (@($account.costs|Where-Object {$_.lifecycle_storage_call_id -eq $call.id -and $_.activity_id -eq $call.id -and $_.quantity -eq 1 -and $_.unit -eq 'invocation' -and $_.pricing_mode -eq 'ACTUAL'}).Count -eq 1) 'Missing physical-attempt allocation'}
Require ($pins.Count -eq 5 -and $stops.Count -eq 5 -and $account.active_pins -eq 0) 'Pin owner stop not closed'
foreach($pin in $pins){Require (@($stops|Where-Object {$_.pin_id -eq $pin.id -and $_.reader_id -eq $pin.reader_id -and $_.generation -eq $pin.generation}).Count -eq 1) 'Unbacked pin termination'}
Require ($transfers.Count -eq 2 -and @($transfers|Where-Object {$_.completed -and $_.bytes_written -eq 129328}).Count -eq 1 -and @($transfers|Where-Object {-not $_.completed -and $_.bytes_written -eq 36714}).Count -eq 1) 'Transfer observation mismatch'
Require (@($account.delivery_operations|Where-Object {$_.status -ne 'ISSUED'}).Count -eq 0) 'Transfer altered terminal delivery history'
$slow=Read-JSON 'plane-slow-client-natural-expiry.json';$active=Read-JSON 'plane-slow-client-active.json'
Require ($active.active_transfer_pins -eq 1 -and $active.client_payload_reads -eq 0 -and -not $active.writer_barrier -and ([DateTime]$active.observed_at).ToUniversalTime() -lt ([DateTime]$summary.use_until).ToUniversalTime()) 'No positive real slow-client occupancy proof'
Require ($slow.Result.StopConfirmed -and $slow.Result.BytesWritten -eq 36714 -and ([DateTime]$slow.At).ToUniversalTime() -ge ([DateTime]$summary.use_until).ToUniversalTime() -and $slow.Error -match 'i/o timeout') 'No real naturally expired write stop'
$held=@($native.objects|Where-Object {$_.copy.Namespace -eq 'dpp-lifecycle307-online' -and $_.copy.Key -eq 'fixture/b.csv'})[0]
$governance=@($native.objects|Where-Object {$_.copy.Namespace -eq 'dpp-lifecycle307-quarantine' -and $_.copy.Key -eq 'fixture/a.csv'})[0]
Require ($held.protection.LegalHold -and $governance.protection.RetentionMode -eq 'GOVERNANCE') 'Original native protection changed'
Require (-not $seal.windows_enabled -and $seal.reader_identity -eq 'DISABLED' -and $seal.producer_identity -eq 'DISABLED' -and $seal.gateway_all_requests -eq 'DENY_ALL') 'Plane not sealed'
Require ($summary.delete -eq 0 -and $summary.cleanup -eq 0 -and $seal.delete -eq 0 -and $seal.cleanup -eq 0) 'Unauthorized destructive action'
$record=@{run_id=$m.run_id;status='PASS';verification='offline independent JSON/source/identity/stop/accounting checks; no native I/O';source_files=@($sources.source_hashes.PSObject.Properties).Count;retained_versions=6;physical_calls=40;active_pins=0;delete=0;cleanup=0;production_acceptance='NOT_RUN'}
$path=Join-Path $dir 'plane-offline-verification.json';if(Test-Path -LiteralPath $path){throw 'Verification already recorded; retain prior evidence'};[IO.File]::WriteAllText($path,($record|ConvertTo-Json -Depth 5),[Text.UTF8Encoding]::new($false))
Write-Output 'Offline plane verification PASS; no provider call or cleanup.'
