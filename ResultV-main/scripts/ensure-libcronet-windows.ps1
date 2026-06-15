# Copies libcronet.dll from the Go module cache (version pinned in go.sum) into
# build/windows/ for Wails NSIS and portable exe packaging. Required for
# sing-box naive on Windows with with_purego (see docs/build-naive.md).
$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
$dest = Join-Path $repoRoot "build/windows/libcronet.dll"
$null = New-Item -ItemType Directory -Force -Path (Split-Path $dest)

# Download the module so Dir is populated in the module cache on CI runners.
& go mod download "github.com/sagernet/cronet-go/lib/windows_amd64"
if ($LASTEXITCODE -ne 0) { throw "go mod download failed for windows_amd64 lib" }

$json = & go list -m -json "github.com/sagernet/cronet-go/lib/windows_amd64" 2>&1
if ($LASTEXITCODE -ne 0) { throw "go list failed: $json" }
$mod = $json | ConvertFrom-Json
if (-not $mod.Dir) { throw "go list returned no Dir for windows_amd64 lib" }
$src = Join-Path $mod.Dir "libcronet.dll"
if (-not (Test-Path $src)) { throw "libcronet.dll not found in module: $src" }
Copy-Item -LiteralPath $src -Destination $dest -Force
Write-Host "Copied libcronet.dll -> $dest ($((Get-Item $dest).Length) bytes)"
