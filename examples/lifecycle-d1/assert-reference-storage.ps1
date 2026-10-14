param([Parameter(Mandatory=$true)][ValidateSet('source-core','source-objects','restored-core','restored-objects','generation02-core')][string]$Role,[ValidateSet('running','created','exited')][string]$State='running')
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'durable-storage.ps1')
$project='dpp-lifecycle307-5d2274ee'
$map=@{'source-core'=@(($project+'-postgres'),'/var/lib/postgresql/data');'source-objects'=@(($project+'-minio'),'/data');'restored-core'=@(($project+'-predelete-postgres'),'/var/lib/postgresql/data');'restored-objects'=@(($project+'-predelete-objects'),'/data');'generation02-core'=@(($project+'-predelete-postgres-02'),'/var/lib/postgresql/data')}
$name=$map[$Role][0];$volume=$name;if($Role -eq 'source-objects'){$volume=$project+'-objects'}
$raw=& docker inspect $name;if($LASTEXITCODE -ne 0){throw 'Exact retained service missing'};$item=($raw|ConvertFrom-Json)[0]
Assert-RetainedServiceStorage -RunID '5d2274ee-a671-4bdc-b406-22b8dad8fd61' -LabelKey 'lifecycle307.run' -RegisteredVolumes @($volume) -AllowedStates @($State) -Services @(@{role=$Role;name=$name;container=$item;path=$map[$Role][1];volume=$volume}) | ConvertTo-Json -Depth 10
