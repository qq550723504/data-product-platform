param([switch]$Resume)
$ErrorActionPreference='Stop'
$taskExecutable=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../pgi-b3-independent-worker.exe'))
$taskRegistry=Join-Path $PSScriptRoot 'paused-worker.json'
Add-Type @'
using System;
using System.Runtime.InteropServices;
public static class Independent299WorkerPause {
 [DllImport("ntdll.dll")] public static extern int NtSuspendProcess(IntPtr handle);
 [DllImport("ntdll.dll")] public static extern int NtResumeProcess(IntPtr handle);
}
'@
if($Resume){
 $taskRecord=Get-Content -LiteralPath $taskRegistry -Raw | ConvertFrom-Json
 $taskProcess=Get-Process -Id $taskRecord.pid
 if($taskProcess.Path -ne $taskExecutable){throw 'registered process no longer matches this worktree worker'}
 $taskStatus=[Independent299WorkerPause]::NtResumeProcess($taskProcess.Handle)
}else{
 $taskMatches=@(Get-Process -Name pgi-b3-independent-worker | Where-Object {$_.Path -eq $taskExecutable})
 if($taskMatches.Count -ne 1){throw 'exactly one current worktree worker required'}
 $taskProcess=$taskMatches[0]
 $taskStatus=[Independent299WorkerPause]::NtSuspendProcess($taskProcess.Handle)
}
if($taskStatus -ne 0){throw "worker pause/resume failed: $taskStatus"}
[ordered]@{pid=$taskProcess.Id;executable=$taskExecutable;paused=(-not $Resume);utc=[DateTime]::UtcNow.ToString('o');runId='47203cb1-2b93-4946-8f09-72e29022ea4c'} | ConvertTo-Json | Set-Content -LiteralPath $taskRegistry -Encoding utf8
Get-Content -LiteralPath $taskRegistry
