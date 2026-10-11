# After metadata readback, disable the native reader identity, then seal every
# local window and gateway payload/admin route. Never cleans objects or volumes.
param([Parameter(Mandatory=$true)][string]$PrivateDirectory)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$private=(Resolve-Path -LiteralPath $PrivateDirectory).Path
function Docker([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments){$v=& docker.exe @Arguments 2>&1;if($LASTEXITCODE -ne 0){throw "Docker $($Arguments[0]) failed; all resources retained"};($v -join "`n").Trim()}
function Write-Utf8([string]$Path,[string]$Value){[IO.File]::WriteAllText($Path,$Value.Replace("`r`n","`n"),[Text.UTF8Encoding]::new($false))}
$r=Get-Content -LiteralPath (Join-Path $private 'resources.json') -Raw|ConvertFrom-Json
$c=Get-Content -LiteralPath (Join-Path $private 'reader-owner/reader.json') -Raw|ConvertFrom-Json
$run=([guid]$c.RunID).ToString();$output=Join-Path $PSScriptRoot "results/$run"
if(Test-Path -LiteralPath (Join-Path $output 'reader-seal.json')){throw 'Already sealed; no native replay required'}
& (Join-Path $PSScriptRoot 'run-reader-step.ps1') -PrivateDirectory $private -Mode seal
$gateway=[IO.File]::ReadAllText((Join-Path $private 'Caddyfile'))
foreach($pair in @(@('  @admin {','  @write method'),@('  @payload {','  @inspection {'))){$begin=$gateway.IndexOf($pair[0]);$end=$gateway.IndexOf($pair[1],$begin);if($begin -lt 0 -or $end -le $begin){throw 'Reader seal template mismatch; retain resources'};$gateway=$gateway.Remove($begin,$end-$begin)}
Write-Utf8 (Join-Path $private 'Caddyfile') $gateway
Docker @('exec',$r.containers.gateway,'caddy','validate','--config','/etc/caddy/Caddyfile','--adapter','caddyfile')|Out-Null
Docker @('restart',$r.containers.gateway)|Out-Null
foreach($dir in @('reader-owner','reader-admin')){$path=Join-Path $private "$dir/reader.json";$w=Get-Content -LiteralPath $path -Raw|ConvertFrom-Json;$w.Enabled=$false;Write-Utf8 $path ($w|ConvertTo-Json -Depth 8)}
Write-Utf8 (Join-Path $output 'reader-seal.json') (@{run_id=$c.RunID;native_reader_identity='DISABLED';windows_enabled=$false;gateway_payload='DENY_ALL';gateway_admin='DENY_ALL';gateway_delete='DENY_ALL';gateway_copy='DENY_ALL';gateway_config_sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $private 'Caddyfile')).Hash.ToLowerInvariant();retained_original_versions=4;retained_named_volumes=2;lost_reader_pin='RETAINED_UNCONFIRMED';delete=0;copy=0;cleanup=0}|ConvertTo-Json -Depth 6)
Write-Output 'Reader sealed; all original versions, pins and volumes retained.'
