# Only fresh, network-isolated synthetic helpers. Never enumerate other containers.
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'temporary-containers.ps1')
$run=[guid]::NewGuid().ToString()
$output=Join-Path $env:LOCALAPPDATA "Codex/docker-maintenance/native-$run"
New-Item -ItemType Directory -Path $output|Out-Null
$handles=@()
function NativeDocker([string[]]$DockerArguments){
 $value=& docker.exe @DockerArguments
 if($LASTEXITCODE -ne 0){throw 'Synthetic native Docker operation failed'}
 return ($value -join "`n").Trim()
}
try {
 $handle=New-DppTemporaryContainer -RunID $run -DockerArguments @('--pull','never','--read-only','--network','none','--mount',"type=bind,source=$output,target=/run/output",'alpine:3.22','sh','-c','echo retained-data >/run/output/marker; echo synthetic-stdout; echo synthetic-stderr >&2')
 $handles+=$handle
 NativeDocker @('start',$handle.id)|Out-Null
 if((NativeDocker @('wait',$handle.id)) -ne '0'){throw 'Synthetic helper failed'}
 $result=Complete-DppTemporaryContainer -Handle $handle
 if($result.status -ne 'REMOVED' -or (Get-Content -LiteralPath (Join-Path $output 'marker')) -ne 'retained-data'){throw 'Data preservation failed'}
 $diagnostics=Join-Path (Split-Path -Parent $handle.ticket) 'diagnostics'
 if(-not (Get-Content -LiteralPath (Join-Path $diagnostics 'stdout.log') -Raw).Contains('synthetic-stdout')){throw 'stdout not archived'}
 if(-not (Get-Content -LiteralPath (Join-Path $diagnostics 'stderr.log') -Raw).Contains('synthetic-stderr')){throw 'stderr not archived'}
 $failed=New-DppTemporaryContainer -RunID $run -DockerArguments @('--pull','never','--read-only','--network','none','alpine:3.22','sh','-c','exit 7')
 $handles+=$failed
 NativeDocker @('start',$failed.id)|Out-Null
 NativeDocker @('wait',$failed.id)|Out-Null
 $refused=$false
 try {Complete-DppTemporaryContainer -Handle $failed|Out-Null}catch{$refused=$true}
 if(-not $refused -or (NativeDocker @('inspect',$failed.id,'--format','{{.State.Status}}')) -ne 'exited'){throw 'Unexpected failure was removed'}
 # This synthetic fixture intentionally exits 7; only its owner can now attest it.
 $intentional=Complete-DppTemporaryContainer -Handle $failed -ExpectedExitCode 7
 if($intentional.status -ne 'REMOVED'){throw 'Expected nonzero fixture cleanup failed'}
 [pscustomobject]@{status='PASS';run_id=$run;helpers_removed=2;unexpected_failure_retained_until_owner_attestation=$true;host_data_preserved=$true;private_diagnostics_archived=$true;volumes_removed=0;other_containers_touched=0}|ConvertTo-Json -Compress
} finally {
 foreach($known in $handles){
  $present=NativeDocker @('container','ls','--all','--quiet','--no-trunc','--filter',"id=$($known.id)")
  if($present){Write-Output "Synthetic helper retained for diagnosis: $($known.id)"}
 }
}
