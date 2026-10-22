# Real wrapper completion code, synthetic files, in-memory adapters; no Docker/provider/DB.
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$taskTempBase=[IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$fixtureRoot=Join-Path $taskTempBase ('dpp-helper-wiring-'+[guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory((Join-Path $fixtureRoot 'copy-worker'))|Out-Null
[IO.File]::WriteAllText((Join-Path $fixtureRoot 'copy-worker/dsn'),'postgres://synthetic')
[IO.File]::WriteAllText((Join-Path $fixtureRoot 'copy-worker/redis'),'synthetic:6379')
$script:events=[Collections.Generic.List[object]]::new();$script:fakeExit=0;$script:heldHelperStarted=$false;$script:passed=0
function Check([bool]$Value,[string]$Message){if(-not $Value){throw $Message}}
function Docker {
 param([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments)
 $script:events.Add(@{kind=$Arguments[0]})
 switch($Arguments[0]){'inspect'{return ('a'*64)};'wait'{return "$script:fakeExit"};'start'{return ''};default{throw 'Unexpected Docker command in offline test'}}
}
function New-DppTemporaryContainer {
 param([string]$RunID,[string[]]$DockerArguments)
 $script:events.Add(@{kind='new'});return @{id=('a'*64);ticket='synthetic'}
}
function Complete-DppTemporaryContainer {
 param($Handle,[int]$ExpectedExitCode=0,[switch]$Retain)
 $script:events.Add(@{kind='complete';retain=$Retain.IsPresent;expected=$ExpectedExitCode})
 return @{removed=(-not $Retain.IsPresent);retained=$Retain.IsPresent}
}
function Write-Utf8([string]$Path,[string]$Value){$script:events.Add(@{kind='write';path=$Path})}
function Join-Path {
 param([AllowEmptyString()][string]$Path,[string]$ChildPath)
 if([string]::IsNullOrEmpty($Path)){$Path=$fixtureRoot}
 return [IO.Path]::Combine($Path,$ChildPath)
}
function Get-FileHash {param([string]$Algorithm,[string]$LiteralPath);return @{Hash=('a'*64)}}
function git {return ('a'*40)}
$blocks=@{}
foreach($name in @('finish-prepare','run-reader-step','run-plane-step','run-copy-recovery-step')){
 $path=Join-Path $PSScriptRoot ($name+'.ps1');$tokens=$null;$parseErrors=$null
 $ast=[Management.Automation.Language.Parser]::ParseFile($path,[ref]$tokens,[ref]$parseErrors)
 Check ($parseErrors.Count -eq 0) "Wrapper parse failed: $name"
 $text=[IO.File]::ReadAllText($path)
 if($name -eq 'run-copy-recovery-step'){
  $fn=$ast.Find({param($node)$node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq 'Run'},$true)
  . ([scriptblock]::Create($fn.Extent.Text))
 }else{
  $offset=$text.IndexOf('$temporaryHelper=New-DppTemporaryContainer')
  Check ($offset -ge 0) "Missing helper API: $name"
  $blocks[$name]=[scriptblock]::Create($text.Substring($offset))
 }
}
function Case {
 param([string]$Wrapper,[string]$Mode='accept',[int]$Actual=0,[int]$Expected=0,
       [bool]$Retain=$false,[bool]$Retained=$false,[bool]$Start=$false,[bool]$Failure=$false)
 $script:events.Clear();$script:fakeExit=$Actual;$script:heldHelperStarted=$false
 $RetainHelper=$Retain;$StartOnly=$Start;$Scenario='copy-hold-owner'
 $repo=$fixtureRoot;$private=$fixtureRoot;$dir=$fixtureRoot;$results=$fixtureRoot;$output=$fixtureRoot
 $run='d98080ae-a114-4a9b-9972-e8bc9c896cfe';$project='dpp-lifecycle307-d98080ae'
 $name='synthetic';$helper='synthetic';$pg=@{Image=('sha256:'+('a'*64))};$gw=@{Id=('b'*64)}
 $r=@{containers=@{gateway='synthetic'}};$c=@{WorkspaceID=$run;ActorID=$run;OutputDirectory=('/run/output/'+$run)}
 $config=@{Enabled=$true};$environment=@{};$window=@{ExpiresAt='2099-01-01T00:00:00Z'}
 $RoleDirectory='';$adminMode=$false;$ProducerBinary='archive';$WorkID='';$CopyCallID='';$Target='restore/online-a.csv';$caught=$null
 try {
  if($Wrapper -eq 'run-copy-recovery-step'){$role=if($Start){'copy-worker'}else{'copy-producer'};Run $role 'synthetic' $Expected|Out-Null}
  else{& $blocks[$Wrapper]|Out-Null}
 }catch{$caught=$_.Exception.Message}
 Check (($null -ne $caught) -eq $Failure) "Unexpected outcome for $Wrapper/$Mode : $caught"
 if($Failure){Check ($caught -match 'failed') "Failure did not reach business exit validation: $caught"}
 Check (@($script:events|Where-Object kind -eq 'new').Count -eq 1) 'Helper create count changed'
 Check (@($script:events|Where-Object kind -eq 'start').Count -eq 1) 'Helper start count changed'
 $completed=@($script:events|Where-Object kind -eq 'complete')
 if($Failure -or $Start){
  Check ($completed.Count -eq 0) 'Failed or asynchronous helper was completed'
  if($Start){Check $script:heldHelperStarted 'StartOnly barrier marker lost'}
 }else{
  Check ($completed.Count -eq 1) 'Completed helper did not use adapter exactly once'
  Check ($completed[0].retain -eq $Retained) 'Default retirement or explicit retention changed'
  Check ($completed[0].expected -eq $Expected) 'Exact expected exit not forwarded'
  $writeBefore=$false
  foreach($event in $script:events){if($event.kind -eq 'complete'){break};if($event.kind -eq 'write'){$writeBefore=$true}}
  Check $writeBefore 'Completion preceded persisted business/provenance receipts'
 }
 $script:passed++
}
try {
 Case 'finish-prepare'
 Case 'finish-prepare' -Retain $true -Retained $true
 Case 'finish-prepare' -Actual 1 -Failure $true
 Case 'run-reader-step'
 Case 'run-reader-step' -Retain $true -Retained $true
 Case 'run-reader-step' -Mode 'crash' -Actual 95 -Expected 95 -Retained $true
 Case 'run-reader-step' -Actual 1 -Failure $true
 Case 'run-plane-step' -Mode 'setup'
 Case 'run-plane-step' -Mode 'setup' -Retain $true -Retained $true
 Case 'run-plane-step' -Mode 'lc13-init' -Retained $true
 Case 'run-plane-step' -Mode 'pre-issued-crash' -Actual 94 -Expected 94 -Retained $true
 Case 'run-plane-step' -Mode 'setup' -Actual 1 -Failure $true
 Case 'run-copy-recovery-step'
 Case 'run-copy-recovery-step' -Retain $true -Retained $true
 Case 'run-copy-recovery-step' -Actual 93 -Expected 93 -Retained $true
 Case 'run-copy-recovery-step' -Actual 1 -Failure $true
 Case 'run-copy-recovery-step' -Start $true
 @{status='PASS_OFFLINE_WRAPPER_WIRING';cases=$script:passed;docker_executable_calls=0;business_execution=0}|ConvertTo-Json -Compress
}finally{
 $resolved=[IO.Path]::GetFullPath($fixtureRoot)
 $prefix=$taskTempBase.TrimEnd([IO.Path]::DirectorySeparatorChar)+[IO.Path]::DirectorySeparatorChar+'dpp-helper-wiring-'
 if(-not $resolved.StartsWith($prefix,[StringComparison]::OrdinalIgnoreCase)){throw 'Synthetic fixture cleanup escaped its named temporary directory'}
 Remove-Item -LiteralPath $resolved -Recurse -Force
}
