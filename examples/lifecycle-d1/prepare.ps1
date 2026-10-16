param([Parameter(Mandatory=$true)][string]$PrivateRoot,[switch]$ArchiveAcceptance,[switch]$ProcessingDeliveryAcceptance,[string]$InternalSubnet='',[string]$MinioImage='',[ValidateSet('','business-races','late-hold','mixed')][string]$RemainingGroup='')
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if($RemainingGroup -ne '' -and (-not $ProcessingDeliveryAcceptance -or -not $ArchiveAcceptance)){throw 'Remaining profile requires exact-version processing fixture mode'}
if($ProcessingDeliveryAcceptance -and -not $ArchiveAcceptance){throw 'Processing/delivery slice requires exact-version native fixture mode'}
$repo = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$privateBase = [IO.Path]::GetFullPath($PrivateRoot)
if ($privateBase.StartsWith($repo + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'Private credentials must stay outside the checkout.' }
function Write-Utf8([string]$Path, [string]$Value) { [IO.File]::WriteAllText($Path, $Value.Replace("`r`n","`n"), [Text.UTF8Encoding]::new($false)) }
function New-Secret { $bytes = [byte[]]::new(32); [Security.Cryptography.RandomNumberGenerator]::Fill($bytes); [Convert]::ToHexString($bytes).ToLowerInvariant() }
function Docker([Parameter(ValueFromRemainingArguments=$true)][string[]]$Arguments) {
 $result = & docker.exe @Arguments 2>&1
 if ($LASTEXITCODE -ne 0) { throw "Docker operation failed: $($Arguments[0]); environment retained, no automatic cleanup" }
 return ($result -join "`n").Trim()
}
function Inspect([string]$Name) { return (Docker @('inspect', $Name) | ConvertFrom-Json)[0] }
$run = [guid]::NewGuid().ToString()
$project = 'dpp-lifecycle307-' + $run.Substring(0,8)
$private = Join-Path $privateBase $project
if (Test-Path -LiteralPath $private) { throw 'Fresh private directory required' }
New-Item -ItemType Directory -Path $private -Force | Out-Null
# Current user and SYSTEM only; credentials are never supplied in Docker argv/env.
$acl = [Security.AccessControl.DirectorySecurity]::new()
$acl.SetAccessRuleProtection($true,$false)
$identity = [Security.Principal.WindowsIdentity]::GetCurrent().User
foreach ($sid in @($identity, [Security.Principal.SecurityIdentifier]::new('S-1-5-18'))) {
 $rule = [Security.AccessControl.FileSystemAccessRule]::new($sid,'FullControl','ContainerInherit,ObjectInherit','None','Allow')
 $acl.AddAccessRule($rule)
}
Set-Acl -LiteralPath $private -AclObject $acl
$rootAccess = 'bootstrap' + $run.Replace('-','').Substring(0,12)
$rootSecret = New-Secret
$pgSecret = New-Secret
Write-Utf8 (Join-Path $private 'access') $rootAccess
Write-Utf8 (Join-Path $private 'secret') $rootSecret
Write-Utf8 (Join-Path $private 'pgpass') $pgSecret
Write-Utf8 (Join-Path $private 'start.sh') @'
#!/bin/sh
set -eu
export MINIO_ROOT_USER=$(cat /run/credentials/access)
export MINIO_ROOT_PASSWORD=$(cat /run/credentials/secret)
export MINIO_BROWSER=off
exec /usr/local/bin/minio server /data --address :9000 --console-address 127.0.0.1:9001
'@
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'storage.Caddyfile') -Destination (Join-Path $private 'Caddyfile')
$images = @{
 minio = 'sha256:a9eb76492485dd496bfd69b4114e47137f2da1a1a87e4a9b273cb21476cb1a98'
 postgres = 'sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea'
 gateway = 'sha256:d8542f48d34a9cf4e4c11a478865229840e87e4c96ea3f439101f31a5d35f75f'
}
if ($ArchiveAcceptance) { $images.minio = 'sha256:4d2b20cfce457652e6be8c5038d90704a99fdf0160b7e775d968f9163e96a741' }
if($MinioImage -ne ''){if(-not $ArchiveAcceptance -or $MinioImage -notmatch '^sha256:[a-f0-9]{64}$'){throw 'An explicit pinned native image requires archive-compatible fixture mode'};$images.minio=$MinioImage}
foreach ($image in $images.Values) { Docker @('image','inspect',$image,'--format','{{.Id}}') | Out-Null }
$network = $project + '-private'
$pgVolume = $project + '-postgres'
$objectVolume = $project + '-objects'
$names = @{ postgres=$project+'-postgres'; minio=$project+'-minio'; gateway=$project+'-gateway' }
$label = "lifecycle307.run=$run"
$networkArgs=@('network','create','--internal','--label',$label)
if($InternalSubnet -ne '') {
 if($InternalSubnet -notmatch '^10\.234\.(\d{1,3})\.0/24$' -or [int]$Matches[1] -gt 254){throw 'Explicit fixture subnet must be a bounded 10.234.x.0/24'}
 function Range([string]$CIDR){$parts=$CIDR.Split('/');if($parts.Count -ne 2 -or $parts[0].Contains(':')){return $null};$ip=[Net.IPAddress]::Parse($parts[0]).GetAddressBytes();$n=[uint64]$ip[0]*16777216+[uint64]$ip[1]*65536+[uint64]$ip[2]*256+$ip[3];$size=[uint64][math]::Pow(2,32-[int]$parts[1]);$start=[uint64]([math]::Floor($n/$size)*$size);@($start,($start+$size-1))}
 $requested=Range $InternalSubnet
 foreach($id in (Docker @('network','ls','-q')).Split("`n")) { $existing=(Docker @('network','inspect',$id)|ConvertFrom-Json)[0];foreach($subnet in $existing.IPAM.Config){if($null -ne $subnet.Subnet){$range=Range $subnet.Subnet;if($null -ne $range -and $requested[0] -le $range[1] -and $range[0] -le $requested[1]){throw 'Explicit fixture subnet overlaps an existing network'}}} }
 $networkArgs+=@('--subnet',$InternalSubnet)
}
$networkArgs+=$network
Docker @networkArgs | Out-Null
Docker @('volume','create','--label',$label,$pgVolume) | Out-Null
Docker @('volume','create','--label',$label,$objectVolume) | Out-Null
Docker @('create','--pull','never','--name',$names.postgres,'--label',$label,'--network',$network,
 '--mount',"type=volume,source=$pgVolume,target=/var/lib/postgresql/data",
 '--mount',"type=bind,source=$(Join-Path $private 'pgpass'),target=/run/credentials/pgpass,readonly",'-e','POSTGRES_USER=fixture',
 '-e','POSTGRES_DB=lifecycle307','-e','POSTGRES_PASSWORD_FILE=/run/credentials/pgpass',$images.postgres) | Out-Null
Docker @('create','--pull','never','--name',$names.minio,'--label',$label,'--network',$network,'--network-alias','minio',
 '--mount',"type=volume,source=$objectVolume,target=/data",
 '--mount',"type=bind,source=$(Join-Path $private 'access'),target=/run/credentials/access,readonly",
 '--mount',"type=bind,source=$(Join-Path $private 'secret'),target=/run/credentials/secret,readonly",
 '--mount',"type=bind,source=$(Join-Path $private 'start.sh'),target=/run/credentials/start.sh,readonly",
 '--entrypoint','/bin/sh',$images.minio,'/run/credentials/start.sh') | Out-Null
Docker @('create','--pull','never','--name',$names.gateway,'--label',$label,'--network',$network,
 '--mount',"type=bind,source=$(Join-Path $private 'Caddyfile'),target=/etc/caddy/Caddyfile,readonly",
 '--tmpfs','/data','--tmpfs','/config',$images.gateway) | Out-Null
Write-Utf8 (Join-Path $private 'resources.json') (@{run_id=$run;project=$project;network=$network;volumes=@($pgVolume,$objectVolume);containers=$names} | ConvertTo-Json -Depth 8)
foreach ($name in $names.Values) { Docker @('start',$name) | Out-Null }
Docker @('exec',$names.gateway,'caddy','validate','--config','/etc/caddy/Caddyfile','--adapter','caddyfile') | Out-Null
& (Join-Path $PSScriptRoot 'finish-prepare.ps1') -PrivateDirectory $private -ArchiveAcceptance:$ArchiveAcceptance -ProcessingDeliveryAcceptance:$ProcessingDeliveryAcceptance -MinioImage $MinioImage -RemainingGroup $RemainingGroup -RetainHelper:($RemainingGroup -ne '')
