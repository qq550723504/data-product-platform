# Docker is the only runtime; this adapter archives completed helpers privately.
$script:DppHelperTool = Join-Path $PSScriptRoot 'temporary_containers.py'
$script:DppHelperPython = (Get-Command python -ErrorAction Stop).Source
function New-DppTemporaryContainer {
 param([Parameter(Mandatory=$true)][string]$RunID,
       [Parameter(Mandatory=$true)][string[]]$DockerArguments)
 $result = & $script:DppHelperPython $script:DppHelperTool create --run-id $RunID -- @DockerArguments
 if($LASTEXITCODE -ne 0){throw 'Temporary helper creation incomplete; inspect private registry, no automatic retry'}
 return ($result | ConvertFrom-Json)
}
function Complete-DppTemporaryContainer {
 param([Parameter(Mandatory=$true)]$Handle,[int]$ExpectedExitCode=0,[switch]$Retain)
 $arguments=@($script:DppHelperTool,'complete','--ticket',$Handle.ticket,'--expected-exit',"$ExpectedExitCode")
 if($Retain){$arguments+='--retain'}
 $result = & $script:DppHelperPython @arguments
 if($LASTEXITCODE -ne 0){throw 'Helper cleanup incomplete; private diagnostics/resources retained. Do not rerun business work.'}
 return ($result | ConvertFrom-Json)
}
