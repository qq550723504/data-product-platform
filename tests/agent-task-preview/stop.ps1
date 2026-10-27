$ErrorActionPreference='Stop'
$repoRoot=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$record=Get-Content -LiteralPath (Join-Path $repoRoot '.data/agent-q2-preview/processes.json') -Raw|ConvertFrom-Json
if ($record.repoRoot -ne $repoRoot) {throw 'Runtime owner mismatch'}
$owned=@()
foreach($item in $record.processes){
 $process=Get-Process -Id $item.processId -ErrorAction SilentlyContinue
 if(-not $process){continue}
 $listener=Get-NetTCPConnection -LocalPort $item.port -State Listen -ErrorAction SilentlyContinue
 if($process.ProcessName -ne $item.name -or [Math]::Abs(($process.StartTime.ToUniversalTime()-([DateTimeOffset]$item.startedAt).UtcDateTime).TotalSeconds)-gt 1 -or -not($listener.OwningProcess -contains $item.processId)){throw 'Process identity/port mismatch; nothing stopped.'}
 $owned+=$process
}
$gateway=(& docker inspect dpp-agent-q2-gateway-20261023|ConvertFrom-Json)[0]
$configMount=$gateway.Mounts|Where-Object Destination -eq '/etc/caddy/Caddyfile'
if($gateway.Config.Labels.'codex.scope' -ne 'agent-q2-test' -or -not $configMount -or [IO.Path]::GetFullPath($configMount.Source) -ne [IO.Path]::GetFullPath((Join-Path $PSScriptRoot 'Caddyfile'))){throw 'Gateway owner mismatch; nothing stopped.'}
$owned|Stop-Process
& docker stop $gateway.Id
Write-Output 'Stopped only recorded Q2 preview processes and gateway; database retained.'
