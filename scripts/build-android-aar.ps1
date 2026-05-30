<#
.SYNOPSIS
    Rebuild the gomobile AAR for the ResultV Android app.

.DESCRIPTION
    Invokes `gomobile bind` with the correct tags and output path.
    Run from the repo root or any subdirectory — the script resolves
    paths relative to its own location.

.PARAMETER WithNaive
    Include the `with_naive_outbound` build tag. Currently blocked by
    NDK 27.x ld.lld incompatibility with sagernet's precompiled
    libcronet.a — see Known Issues in android-migration-plan.md.

.EXAMPLE
    .\scripts\build-android-aar.ps1
    .\scripts\build-android-aar.ps1 -WithNaive
#>
[CmdletBinding()]
param(
    [switch]$WithNaive
)

$ErrorActionPreference = 'Stop'

$RepoRoot = "C:\ResultV"
if (-not (Test-Path "$RepoRoot\go.mod")) {
    $RepoRoot = $PWD.Path
}

$Output = Join-Path $RepoRoot "android\libs\libbox.aar"
$OutputDir = Split-Path -Parent $Output
if (-not (Test-Path $OutputDir)) { New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null }

# Base tags
$Tags = "mobile,with_gvisor,with_utls,with_clash_api,with_quic,with_wireguard"

if ($WithNaive) {
    $Tags += ",with_naive_outbound"
}

Write-Host "Building AAR with tags: $Tags"
Write-Host "Output: $Output"

Push-Location $RepoRoot
try {
    $env:GOFLAGS = "-tags=$Tags"
    $Splat = @(
        "bind",
        "-target=android",
        "-androidapi=26",
        "-ldflags=-checklinkname=0",
        "-o",
        $Output,
        "./mobile",
        "github.com/sagernet/sing-box/experimental/libbox"
    )
    & gomobile @Splat

    if ($LASTEXITCODE -ne 0) {
        Write-Error "gomobile bind failed with exit code $LASTEXITCODE"
        exit $LASTEXITCODE
    }
} finally {
    Pop-Location
}

Write-Host "✅ AAR built successfully: $Output" -ForegroundColor Green
Get-Item $Output | Select-Object Name, Length, LastWriteTime | Format-Table -AutoSize
