[CmdletBinding()]
param(
    [Parameter(Mandatory = $false)]
    [ValidateSet("CurrentUser", "AllUsers")]
    [string]$InstallMode = "CurrentUser",

    [Parameter(Mandatory = $false)]
    [string]$ExpectedExePath
)

$ErrorActionPreference = "Stop"

function Assert-True {
    param(
        [bool]$Condition,
        [string]$Message
    )

    if (-not $Condition) {
        throw $Message
    }
}

# HKLM is the machine-wide fallback; the installer always writes here so the
# protocol works from a browser even before the user has launched the app once
# (and regardless of which install mode was chosen). The CurrentUser mode also
# writes the elevated user's HKCU, but that's secondary — HKLM is what matters
# for browser-click correctness in both modes.
$hivesToCheck = @("Registry::HKEY_LOCAL_MACHINE\Software\Classes\resultv")
if ($InstallMode -eq "CurrentUser") {
    $hivesToCheck += "Registry::HKEY_CURRENT_USER\Software\Classes\resultv"
}

foreach ($baseHivePath in $hivesToCheck) {
    $commandPath = Join-Path $baseHivePath "shell\open\command"

    Assert-True (Test-Path $baseHivePath) "Protocol key not found: $baseHivePath"
    Assert-True (Test-Path $commandPath) "Command key not found: $commandPath"

    $protocolProps = Get-ItemProperty -Path $baseHivePath
    $commandProps = Get-ItemProperty -Path $commandPath

    $command = [string]$commandProps.'(default)'
    Assert-True (-not [string]::IsNullOrWhiteSpace($command)) "Command value is empty at $commandPath"

    $quotedPattern = '^"[^"]+" "%1"$'
    Assert-True ($command -match $quotedPattern) "Protocol command is not fully quoted: $command"

    if (-not [string]::IsNullOrWhiteSpace($ExpectedExePath)) {
        $expectedCommand = '"' + $ExpectedExePath + '" "%1"'
        Assert-True ($command -eq $expectedCommand) "Unexpected command value. Expected: $expectedCommand ; Actual: $command"
    }

    $defaultValue = [string]$protocolProps.'(default)'
    Assert-True ($defaultValue -eq "URL:ResultV Protocol") "Default value should be 'URL:ResultV Protocol' (got: '$defaultValue') at $baseHivePath"

    $urlProtocol = [string]$protocolProps.'URL Protocol'
    Assert-True ($null -ne $urlProtocol) "Missing URL Protocol value in $baseHivePath"

    Write-Host "OK: resultv protocol registration is valid at $baseHivePath"
    Write-Host "    Default: $defaultValue"
    Write-Host "    Command: $command"
}

Write-Host "OK: resultv protocol registration is valid for mode '$InstallMode'."
