<#
.SYNOPSIS
  Builds and starts the Lumina demo: the dashboard plus stand-ins for openHAB,
  Vikunja, Grafana and the calendars (no secrets, no real systems are touched).

.DESCRIPTION
  1. builds the frontend and the two Go programs (into ./bin)
  2. starts demo-services on :18080
  3. starts the dashboard on the given port with config/config.demo.yaml
  4. opens the browser; Ctrl+C stops everything

  Weather is live data from Open-Meteo, so it needs an internet connection.

.PARAMETER Port
  Port of the dashboard (default 8080).

.PARAMETER NoBrowser
  Do not open the browser.

.PARAMETER SkipBuild
  Reuse the binaries from the last run.
#>
param(
  [int]$Port = 8080,
  [switch]$NoBrowser,
  [switch]$SkipBuild
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$bin = Join-Path $root 'bin'
New-Item -ItemType Directory -Force $bin | Out-Null

function Require($name) {
  if (-not (Get-Command $name -ErrorAction SilentlyContinue)) { throw "'$name' is required but was not found in PATH." }
}

if (-not $SkipBuild) {
  Require go
  Require npm
  Write-Host '== building frontend' -ForegroundColor Cyan
  Push-Location (Join-Path $root 'frontend')
  try {
    if (-not (Test-Path node_modules)) { npm ci; if ($LASTEXITCODE) { throw 'npm ci failed' } }
    npm run build
    if ($LASTEXITCODE) { throw 'frontend build failed' }
  } finally { Pop-Location }

  Write-Host '== building backend' -ForegroundColor Cyan
  Push-Location (Join-Path $root 'backend')
  try {
    go build -o (Join-Path $bin 'lumina.exe') ./cmd/lumina
    if ($LASTEXITCODE) { throw 'lumina build failed' }
    go build -o (Join-Path $bin 'demo-services.exe') ./mocks/demo-services
    if ($LASTEXITCODE) { throw 'demo-services build failed' }
  } finally { Pop-Location }
}

foreach ($p in 18080, $Port) {
  if (Get-NetTCPConnection -LocalPort $p -State Listen -ErrorAction SilentlyContinue) {
    throw "Port $p is already in use. Stop the other program (or use -Port) and try again."
  }
}

$services = $null
try {
  Write-Host '== starting demo services on :18080' -ForegroundColor Cyan
  $services = Start-Process -FilePath (Join-Path $bin 'demo-services.exe') -ArgumentList '-addr', ':18080' -PassThru -WindowStyle Hidden

  $env:LUMINA_ADDR = ":$Port"
  $config = Join-Path $root 'config\config.demo.yaml'
  Write-Host "== starting Lumina on http://localhost:$Port (Ctrl+C to stop)" -ForegroundColor Cyan

  if (-not $NoBrowser) {
    # open the browser once the server answers
    Start-Job -ArgumentList $Port -ScriptBlock {
      param($port)
      for ($i = 0; $i -lt 50; $i++) {
        try { Invoke-WebRequest "http://localhost:$port/readyz" -UseBasicParsing -TimeoutSec 1 | Out-Null; break } catch { Start-Sleep -Milliseconds 200 }
      }
      Start-Process "http://localhost:$port"
    } | Out-Null
  }

  & (Join-Path $bin 'lumina.exe') -config $config
} finally {
  if ($services -and -not $services.HasExited) { Stop-Process -Id $services.Id -Force }
  Get-Job | Remove-Job -Force -ErrorAction SilentlyContinue
  Write-Host '== stopped' -ForegroundColor Cyan
}
