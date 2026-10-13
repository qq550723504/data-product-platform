param([Parameter(Mandatory=$true)][string]$Helper,[Parameter(Mandatory=$true)][string]$InputPath,[string[]]$AllowedStates=@('running'))
$ErrorActionPreference='Stop'
. $Helper
$inputState=Get-Content -LiteralPath $InputPath -Raw | ConvertFrom-Json
Assert-RetainedServiceStorage -RunID $inputState.run_id -LabelKey 'dpp.trial.run' -RegisteredVolumes @($inputState.volumes) -Services @($inputState.services) -AllowedStates $AllowedStates | ConvertTo-Json -Depth 8
