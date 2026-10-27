$ErrorActionPreference='Stop'
$repoRoot=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$webRoot=Join-Path $repoRoot 'apps/web'
$envFile=Join-Path $webRoot 'next-env.d.ts'
$originalEnv=[System.IO.File]::ReadAllBytes($envFile)
$configName='.next-agent-q2-tsconfig.json'
Copy-Item -LiteralPath (Join-Path $webRoot 'tsconfig.json') -Destination (Join-Path $webRoot $configName) -Force
$settings=@{NEXT_DIST_DIR='.next-agent-q2';NEXT_TSCONFIG_PATH=$configName;NEXT_TELEMETRY_DISABLED='1'}
$previous=@{}
Push-Location $webRoot
try {
 foreach ($name in $settings.Keys) { $previous[$name]=[Environment]::GetEnvironmentVariable($name,'Process'); [Environment]::SetEnvironmentVariable($name,$settings[$name],'Process') }
 & npm.cmd run build
 if ($LASTEXITCODE -ne 0) { throw 'Isolated Next build failed' }
} finally {
 [System.IO.File]::WriteAllBytes($envFile,$originalEnv)
 foreach ($name in $previous.Keys) { [Environment]::SetEnvironmentVariable($name,$previous[$name],'Process') }
 Pop-Location
}
