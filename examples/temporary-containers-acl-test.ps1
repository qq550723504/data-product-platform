# Windows-only privacy regression using synthetic directories/files, no Docker.
param([string]$OutputDirectory='')
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if($OutputDirectory -eq ''){
 $OutputDirectory=Join-Path $env:LOCALAPPDATA ('Codex/docker-maintenance/acl-'+[guid]::NewGuid().ToString('N'))
 New-Item -ItemType Directory -Path $OutputDirectory|Out-Null
}
$aclRoot=Join-Path $OutputDirectory 'archive-acl'
New-Item -ItemType Directory -Path $aclRoot|Out-Null
& icacls.exe $aclRoot /grant '*S-1-1-0:(OI)(CI)RX'|Out-Null
if($LASTEXITCODE -ne 0){throw 'Synthetic archive ACL setup failed'}
$aclProbe=Join-Path $aclRoot 'synthetic-diagnostic.txt'
[IO.File]::WriteAllText($aclProbe,'synthetic-only')
$python=(Get-Command python -ErrorAction Stop).Source
& $python -c 'import sys; from pathlib import Path; sys.path.insert(0,sys.argv[1]); import temporary_containers as tc; tc.private_root(Path(sys.argv[2]))' $PSScriptRoot $aclRoot
if($LASTEXITCODE -ne 0){throw 'Private archive ACL replacement failed'}
$allowed=@([Security.Principal.WindowsIdentity]::GetCurrent().User.Value,'S-1-5-18')
foreach($aclPath in @($aclRoot,$aclProbe)){
 $foreign=@((Get-Acl -LiteralPath $aclPath).Access|Where-Object {$_.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value -notin $allowed})
 if($foreign.Count){throw 'Unrelated explicit archive access survived'}
}
[pscustomobject]@{status='PASS';foreign_explicit_archive_access_removed=$true;existing_diagnostic_file_protected=$true;docker_operations=0}|ConvertTo-Json -Compress
