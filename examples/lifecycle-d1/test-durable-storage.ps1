# Metadata contract checks only. No Docker mutation, object or database I/O.
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'durable-storage.ps1')
$run='00000000-0000-4000-8000-000000000307'
$resources=[pscustomobject]@{run_id=$run;volumes=@('fixture-pg','fixture-objects');containers=[pscustomobject]@{postgres='fixture-pg-service';minio='fixture-object-service'}}
function Service([string]$Name,[string]$Path,[string]$Volume){[pscustomobject]@{Name='/'+$Name;Id=$Name+'-id';Config=[pscustomobject]@{Labels=[pscustomobject]@{'lifecycle307.run'=$run}};State=[pscustomobject]@{Status='running'};HostConfig=[pscustomobject]@{Tmpfs=$null};Mounts=@([pscustomobject]@{Type='volume';Destination=$Path;Name=$Volume;RW=$true})}}
$pg=Service 'fixture-pg-service' '/var/lib/postgresql/data' 'fixture-pg'
$objects=Service 'fixture-object-service' '/data' 'fixture-objects'
Assert-FixtureDurableStorage $resources $pg $objects|Out-Null
$rejected=0
foreach($case in @('core-tmpfs-256m','object-tmpfs','tmpfs-parent','tmpfs-wal','anonymous-volume','unregistered-volume','swapped-service-volumes','readonly-volume','nested-data-mount','stopped-service')){
 $p=Service 'fixture-pg-service' '/var/lib/postgresql/data' 'fixture-pg';$o=Service 'fixture-object-service' '/data' 'fixture-objects'
 switch($case){
  'core-tmpfs-256m' {$p.Mounts=@();$p.HostConfig.Tmpfs=[pscustomobject]@{'/var/lib/postgresql/data'='rw,size=256m'}}
  'object-tmpfs' {$o.Mounts=@();$o.HostConfig.Tmpfs=[pscustomobject]@{'/data'='rw,size=256m'}}
  'tmpfs-parent' {$p.HostConfig.Tmpfs=[pscustomobject]@{'/var/lib/postgresql'='rw,size=256m'}}
  'tmpfs-wal' {$p.HostConfig.Tmpfs=[pscustomobject]@{'/var/lib/postgresql/data/pg_wal'='rw,size=256m'}}
  'anonymous-volume' {$p.Mounts[0].Name=''}
  'unregistered-volume' {$p.Mounts[0].Name='other-run'}
  'swapped-service-volumes' {$p.Mounts[0].Name='fixture-objects';$o.Mounts[0].Name='fixture-pg'}
  'readonly-volume' {$p.Mounts[0].RW=$false}
  'nested-data-mount' {$p.Mounts+=@([pscustomobject]@{Type='bind';Destination='/var/lib/postgresql/data/pg_wal';Name='';RW=$true})}
  'stopped-service' {$p.State.Status='exited'}
 }
 $failed=$false;try{Assert-FixtureDurableStorage $resources $p $o|Out-Null}catch{$failed=$true};if(-not $failed){throw ('Unsafe retained-service metadata accepted: '+$case)};$rejected++
}
$generic=@()
foreach($role in @('core','source','minio','redis')){
 $path='/data';if($role -in @('core','source')){$path='/var/lib/postgresql/data'}
 $c=Service ($role+'-service') $path ($role+'-volume');$c.Config.Labels=[pscustomobject]@{'pgi.run'='synthetic-c2-retained'}
 $generic+=@{role=$role;name=$role+'-service';path=$path;volume=$role+'-volume';container=$c}
}
$registered=@($generic|ForEach-Object {$_.volume})
Assert-RetainedServiceStorage -RunID 'synthetic-c2-retained' -LabelKey 'pgi.run' -RegisteredVolumes $registered -Services $generic|Out-Null
foreach($state in @('created','exited')){
 $spec=@($generic|ConvertTo-Json -Depth 8|ConvertFrom-Json);foreach($service in $spec){$service.container.State.Status=$state}
 Assert-RetainedServiceStorage -RunID 'synthetic-c2-retained' -LabelKey 'pgi.run' -RegisteredVolumes $registered -Services $spec -AllowedStates @('created','exited')|Out-Null
}
foreach($case in @('wrong-label-key','wrong-run','duplicate-volume','duplicate-role','duplicate-container','relative-data-path','default-rejects-created','prestart-tmpfs','unsupported-state')){
 $spec=@($generic|ConvertTo-Json -Depth 8|ConvertFrom-Json)
 $label='pgi.run';$owner='synthetic-c2-retained'
 $states=@('running')
 switch($case){
  'wrong-label-key' {$label='lifecycle307.run'}
  'wrong-run' {$owner='different-run'}
  'duplicate-volume' {$spec[1].volume=$spec[0].volume}
  'duplicate-role' {$spec[1].role=$spec[0].role}
  'duplicate-container' {$spec[1].name=$spec[0].name}
  'relative-data-path' {$spec[0].path='var/lib/postgresql/data'}
  'default-rejects-created' {$spec[0].container.State.Status='created'}
  'prestart-tmpfs' {$states=@('created','exited');foreach($service in $spec){$service.container.State.Status='created'};$spec[0].container.HostConfig.Tmpfs=[pscustomobject]@{'/var/lib/postgresql/data'='rw,size=256m'}}
  'unsupported-state' {$states=@('paused')}
 }
 $failed=$false;try{Assert-RetainedServiceStorage -RunID $owner -LabelKey $label -RegisteredVolumes $registered -Services $spec -AllowedStates $states|Out-Null}catch{$failed=$true};if(-not $failed){throw ('Unsafe four-role metadata accepted: '+$case)};$rejected++
}
[pscustomobject]@{status='PASS';positive=4;rejected=$rejected;docker_calls=0;storage_io=0;mutation=0}|ConvertTo-Json
