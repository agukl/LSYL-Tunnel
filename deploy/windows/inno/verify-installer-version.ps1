param(
    [Parameter(Mandatory = $true)]
    [string]$ScriptFile,

    [Parameter(Mandatory = $true)]
    [string]$InstallerFile
)

$ErrorActionPreference = "Stop"
trap {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}

if (-not (Test-Path -LiteralPath $ScriptFile)) {
    throw "Installer script not found: $ScriptFile"
}
if (-not (Test-Path -LiteralPath $InstallerFile)) {
    throw "Installer output not found: $InstallerFile"
}

$line = Select-String -LiteralPath $ScriptFile -Pattern '^\s*#define\s+AppVersion\s+"([^"]+)"' | Select-Object -First 1
if (-not $line) {
    throw "Cannot read AppVersion from installer script: $ScriptFile"
}

$expected = $line.Matches[0].Groups[1].Value.Trim()
$actual = ((Get-Item -LiteralPath $InstallerFile).VersionInfo.ProductVersion + "").Trim()

if ($actual -ne $expected) {
    throw "Installer version mismatch: $InstallerFile product_version=$actual expected=$expected"
}

Write-Host "[INFO] Installer version verified: $InstallerFile product_version=$actual"
exit 0
