param([Parameter(Mandatory=$true)][string]$InputPath,[Parameter(Mandatory=$true)][string]$AllowedStates)
$ErrorActionPreference='Stop'
. "$PSScriptRoot/../../examples/lifecycle-d1/durable-storage.ps1"
$v=Get-Content -LiteralPath $InputPath -Raw | ConvertFrom-Json
Assert-RetainedServiceStorage -RunID $v.run_id -LabelKey 'dpp.trial.run' -RegisteredVolumes @($v.volumes) -Services @($v.services) -AllowedStates @($AllowedStates.Split(',')) | ConvertTo-Json -Depth 8
