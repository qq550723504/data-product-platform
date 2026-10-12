# Read-only preflight for the two durable services. One-shot Go helpers may use
# tmpfs for scratch paths; they never run PostgreSQL or own retained object data.
function Assert-FixtureDurableStorage {
 param([Parameter(Mandatory=$true)]$Resources,[Parameter(Mandatory=$true)]$Postgres,[Parameter(Mandatory=$true)]$ObjectStore)
 $run=([guid]$Resources.run_id).ToString()
 $registered=@($Resources.volumes)
 if($registered.Count -ne 2 -or @($registered|Select-Object -Unique).Count -ne 2){throw 'Two distinct owned named data volumes required'}
 $result=@()
 foreach($service in @(@{role='postgres';container=$Postgres;name=$Resources.containers.postgres;path='/var/lib/postgresql/data';volume=$registered[0]},@{role='objects';container=$ObjectStore;name=$Resources.containers.minio;path='/data';volume=$registered[1]})){
  $item=$service.container
  if($item.Name.TrimStart('/') -ne $service.name -or $item.Config.Labels.'lifecycle307.run' -ne $run -or $item.State.Status -ne 'running'){throw 'Durable service ownership/state mismatch'}
  $mounts=@($item.Mounts|Where-Object {$_.Destination -eq $service.path})
  if($mounts.Count -ne 1 -or $mounts[0].Type -ne 'volume' -or -not $mounts[0].RW -or [string]::IsNullOrEmpty($mounts[0].Name) -or $mounts[0].Name -ne $service.volume){throw 'Retained service data must use its role-specific registered writable named volume'}
  # Any tmpfs or alternate mount covering the data path or a child (including
  # pg_wal) defeats retention even if a named volume exists at the parent.
  if($null -ne $item.HostConfig.Tmpfs){foreach($entry in $item.HostConfig.Tmpfs.PSObject.Properties){$path=$entry.Name.TrimEnd('/');if($path -eq '' -or $service.path -eq $path -or $service.path.StartsWith($path+'/') -or $path.StartsWith($service.path+'/')){throw 'Tmpfs cannot cover retained service data or WAL'}}}
  foreach($other in @($item.Mounts|Where-Object {$_.Destination -ne $service.path})){$path=$other.Destination.TrimEnd('/');if($path.StartsWith($service.path+'/')){throw 'Nested service data mount is outside the retained volume contract'}}
  $result+=@{role=$service.role;container=$service.name;container_id=$item.Id;destination=$service.path;volume=$mounts[0].Name;type='volume';writable=$true}
 }
 if($result[0].volume -eq $result[1].volume){throw 'PostgreSQL and native objects cannot share a data volume'}
 [pscustomobject]@{run_id=$run;status='PASS';services=$result;storage_io=0;mutation=0}
}
