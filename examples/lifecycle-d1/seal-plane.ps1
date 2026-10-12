param([Parameter(Mandatory=$true)][string]$PrivateDirectory)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$private=(Resolve-Path -LiteralPath $PrivateDirectory).Path
function Docker([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments){$v=& docker.exe @Arguments 2>&1;if($LASTEXITCODE -ne 0){throw 'Owned plane seal failed; resources retained'};($v -join "`n").Trim()}
function Write-Utf8([string]$Path,[string]$Value){[IO.File]::WriteAllText($Path,$Value.Replace("`r`n","`n"),[Text.UTF8Encoding]::new($false))}
$r=Get-Content -LiteralPath (Join-Path $private 'resources.json') -Raw|ConvertFrom-Json
$c=Get-Content -LiteralPath (Join-Path $private 'plane-owner/reader.json') -Raw|ConvertFrom-Json
$run=([guid]$c.RunID).ToString();$output=Join-Path $PSScriptRoot "results/$run"
if(Test-Path -LiteralPath (Join-Path $output 'plane-seal.json')){throw 'Already sealed; native replay refused'}
& (Join-Path $PSScriptRoot 'run-plane-step.ps1') -PrivateDirectory $private -Mode seal
Write-Utf8 (Join-Path $private 'Caddyfile') @'
{
 admin off
 auto_https off
}
:9000 {
 respond 403
}
'@
Docker @('exec',$r.containers.gateway,'caddy','validate','--config','/etc/caddy/Caddyfile','--adapter','caddyfile')|Out-Null
Docker @('restart',$r.containers.gateway)|Out-Null
foreach($dir in @('plane-owner','plane-admin')){$path=Join-Path $private "$dir/reader.json";$w=Get-Content -LiteralPath $path -Raw|ConvertFrom-Json;$w.Enabled=$false;Write-Utf8 $path ($w|ConvertTo-Json -Depth 8)}
Write-Utf8 (Join-Path $output 'plane-seal.json') (@{run_id=$run;reader_identity='DISABLED';producer_identity='DISABLED';windows_enabled=$false;gateway_all_requests='DENY_ALL';retained_named_volumes=@($r.volumes);delete=0;cleanup=0}|ConvertTo-Json -Depth 6)
Write-Output 'Plane sealed; objects, protection, facts and volumes retained.'
