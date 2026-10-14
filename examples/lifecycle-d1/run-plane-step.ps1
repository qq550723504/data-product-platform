param([Parameter(Mandatory=$true)][string]$PrivateDirectory,[Parameter(Mandatory=$true)][ValidateSet('provision','seal','setup','accept','final','reference-races','pre-issued','pre-issued-crash','verify-pre-issued-crash')][string]$Mode)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$repo=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$private=(Resolve-Path -LiteralPath $PrivateDirectory).Path
function Docker([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments){$v=& docker.exe @Arguments 2>&1;if($LASTEXITCODE -ne 0){throw "Docker $($Arguments[0]) failed; all resources retained"};($v -join "`n").Trim()}
function Write-Utf8([string]$Path,[string]$Value){[IO.File]::WriteAllText($Path,$Value.Replace("`r`n","`n"),[Text.UTF8Encoding]::new($false))}
$r=Get-Content -LiteralPath (Join-Path $private 'resources.json') -Raw|ConvertFrom-Json
$dir=Join-Path $private 'plane-owner'
if($Mode -in @('provision','seal')){$dir=Join-Path $private 'plane-admin'}
$c=Get-Content -LiteralPath (Join-Path $dir 'reader.json') -Raw|ConvertFrom-Json
$run=([guid]$r.run_id).ToString();$project='dpp-lifecycle307-'+$run.Substring(0,8)
if($private.StartsWith($repo+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase) -or $r.project -ne $project -or (Split-Path -Leaf $private) -ne $project -or $c.RunID -ne $run -or -not $c.Enabled){throw 'Owned fresh reader window required'}
if($Mode -notin @('provision','seal') -and ($c.RootAccess -ne '' -or $c.RootSecret -ne '')){throw 'Reader must not mount root credentials'}
$gw=(Docker @('inspect',$r.containers.gateway)|ConvertFrom-Json)[0]
if($gw.Config.Labels.'lifecycle307.run' -ne $run -or $gw.State.Status -ne 'running'){throw 'Gateway owner/state mismatch'}
$name=$project+'-reader-'+$Mode+'-'+([guid]::NewGuid().ToString('N').Substring(0,6))
$pg=(Docker @('inspect',$r.containers.postgres)|ConvertFrom-Json)[0]
. (Join-Path $PSScriptRoot 'durable-storage.ps1')
Assert-FixtureDurableStorage $r $pg ((Docker @('inspect',$r.containers.minio)|ConvertFrom-Json)[0])|Out-Null
Write-Utf8 (Join-Path $dir ($name+'.sh')) ("#!/bin/sh`nset -eu`nexec /run/private/reader -mode plane-$Mode`n")
Docker @('create','--pull','never','--name',$name,'--label',"lifecycle307.run=$run",'--network',"container:$($r.containers.gateway)",'--read-only','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data','--mount',"type=bind,source=$dir,target=/run/private,readonly",'--mount',"type=bind,source=$(Join-Path $PSScriptRoot 'results'),target=/run/output",'--mount',"type=bind,source=$repo,target=/run/repo,readonly",'--mount',"type=bind,source=$(Join-Path $dir 'rules'),target=/run/rules,readonly",'--entrypoint','/bin/sh',$pg.Image,('/run/private/'+$name+'.sh'))|Out-Null
Docker @('start',$name)|Out-Null
$exit=Docker @('wait',$name)
$expected='0';if($Mode -eq 'pre-issued-crash'){$expected='94'}
$receipt=@{run_id=$run;mode=$Mode;container=$name;id=(Docker @('inspect',$name,'--format','{{.Id}}'));exit_code=[int]$exit;expected_exit=[int]$expected;binary_sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $dir 'reader')).Hash.ToLowerInvariant();root_mounted=($Mode -in @('provision','seal'));delete=0;cleanup=0}
Write-Utf8 (Join-Path $PSScriptRoot "results/$run/reader-helper-$name.json") ($receipt|ConvertTo-Json)
if($exit -ne $expected){throw "Reader $Mode failed; helper and facts retained. Inspect its log; no automatic resend"}
Write-Output "Reader $Mode complete ($exit); DELETE=0, cleanup=0."
