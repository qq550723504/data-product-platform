[CmdletBinding(DefaultParameterSetName='File')]
param(
 [Parameter(Mandatory=$true,ParameterSetName='File')][string]$InputPath,
 [Parameter(Mandatory=$true,ParameterSetName='Stdin')][switch]$InputFromStdin,
 [Parameter(Mandatory=$true)][string]$AllowedStates
)
$ErrorActionPreference='Stop'
[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false)
. "$PSScriptRoot/../../examples/lifecycle-d1/durable-storage.ps1"
if($PSCmdlet.ParameterSetName -eq 'Stdin'){
 [Console]::InputEncoding=[Text.UTF8Encoding]::new($false,$true)
 $v=[Console]::In.ReadToEnd() | ConvertFrom-Json
}else{
 $v=Get-Content -LiteralPath $InputPath -Raw | ConvertFrom-Json
}
Assert-RetainedServiceStorage -RunID $v.run_id -LabelKey 'dpp.trial.run' -RegisteredVolumes @($v.volumes) -Services @($v.services) -AllowedStates @($AllowedStates.Split(',')) | ConvertTo-Json -Depth 8
