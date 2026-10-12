# Read-only Docker metadata preflight, shared by retained synthetic compositions.
# The caller supplies its real ownership label and role-specific registered
# volumes; this helper never queries Docker, changes a mount or touches bytes.
function Assert-RetainedServiceStorage {
 param([Parameter(Mandatory=$true)][string]$RunID,[Parameter(Mandatory=$true)][string]$LabelKey,[Parameter(Mandatory=$true)][string[]]$RegisteredVolumes,[Parameter(Mandatory=$true)][object[]]$Services)
 if([string]::IsNullOrWhiteSpace($RunID) -or [string]::IsNullOrWhiteSpace($LabelKey) -or $Services.Count -eq 0 -or $RegisteredVolumes.Count -ne $Services.Count -or @($RegisteredVolumes|Select-Object -Unique).Count -ne $Services.Count){throw 'Retained composition requires explicit ownership and one distinct registered named volume per role'}
 $roles=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
 $names=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
 $volumes=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
 $result=@()
 foreach($service in $Services){
  $item=$service.container
  if([string]::IsNullOrWhiteSpace($service.role) -or [string]::IsNullOrWhiteSpace($service.name) -or [string]::IsNullOrWhiteSpace($service.volume) -or $service.volume -notin $RegisteredVolumes -or -not $roles.Add($service.role) -or -not $names.Add($service.name) -or -not $volumes.Add($service.volume)){throw 'Retained roles, containers and registered volumes must be distinct'}
  if(-not $service.path.StartsWith('/') -or $service.path -eq '/' -or $service.path.EndsWith('/') -or $service.path.Contains('//') -or $service.path -match '/\.\.?(/|$)'){throw 'Explicit canonical absolute service data path required'}
  $label=$item.Config.Labels.PSObject.Properties[$LabelKey]
  if($null -eq $label -or $label.Value -ne $RunID -or $item.Name.TrimStart('/') -ne $service.name -or $item.State.Status -ne 'running'){throw 'Durable service ownership/state mismatch'}
  $mounts=@($item.Mounts|Where-Object {$_.Destination -eq $service.path})
  if($mounts.Count -ne 1 -or $mounts[0].Type -ne 'volume' -or -not $mounts[0].RW -or [string]::IsNullOrEmpty($mounts[0].Name) -or $mounts[0].Name -ne $service.volume){throw 'Retained service data must use its role-specific registered writable named volume'}
  # Any tmpfs or alternate mount covering the data path or a child (including
  # pg_wal) defeats retention even if a named volume exists at the parent.
  if($null -ne $item.HostConfig.Tmpfs){foreach($entry in $item.HostConfig.Tmpfs.PSObject.Properties){$path=$entry.Name.TrimEnd('/');if($path -eq '' -or $service.path -eq $path -or $service.path.StartsWith($path+'/') -or $path.StartsWith($service.path+'/')){throw 'Tmpfs cannot cover retained service data or WAL'}}}
  foreach($other in @($item.Mounts|Where-Object {$_.Destination -ne $service.path})){$path=$other.Destination.TrimEnd('/');if($path.StartsWith($service.path+'/')){throw 'Nested service data mount is outside the retained volume contract'}}
  $result+=@{role=$service.role;container=$service.name;container_id=$item.Id;destination=$service.path;volume=$mounts[0].Name;type='volume';writable=$true}
 }
 [pscustomobject]@{run_id=$RunID;label_key=$LabelKey;status='PASS';services=$result;storage_io=0;mutation=0}
}

# D1's two retained services use the existing lifecycle307 ownership contract.
# One-shot Go helpers may use tmpfs for scratch; they never own retained data.
function Assert-FixtureDurableStorage {
 param([Parameter(Mandatory=$true)]$Resources,[Parameter(Mandatory=$true)]$Postgres,[Parameter(Mandatory=$true)]$ObjectStore)
 $registered=@($Resources.volumes)
 if($registered.Count -ne 2){throw 'Two role-specific registered D1 volumes required'}
 Assert-RetainedServiceStorage -RunID ([guid]$Resources.run_id).ToString() -LabelKey 'lifecycle307.run' -RegisteredVolumes $registered -Services @(@{role='postgres';container=$Postgres;name=$Resources.containers.postgres;path='/var/lib/postgresql/data';volume=$registered[0]},@{role='objects';container=$ObjectStore;name=$Resources.containers.minio;path='/data';volume=$registered[1]})
}
