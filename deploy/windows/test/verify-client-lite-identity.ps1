param(
    [Parameter(Mandatory = $true)]
    [string]$BinaryFile
)

$ErrorActionPreference = "Stop"
trap {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}

if (-not (Test-Path -LiteralPath $BinaryFile -PathType Leaf)) {
    throw "Lite executable not found: $BinaryFile"
}

$versionInfo = (Get-Item -LiteralPath $BinaryFile).VersionInfo
$expected = [ordered]@{
    InternalName = "lsyl-tunnel-client-lite"
    OriginalFilename = "lsyl-tunnel-client-lite.exe"
}

foreach ($field in $expected.Keys) {
    $actual = [string]$versionInfo.$field
    if ($actual -cne $expected[$field]) {
        throw "Lite identity mismatch: $BinaryFile ${field}='$actual' expected='$($expected[$field])'"
    }
}

Write-Host "[INFO] Lite file identity verified: $BinaryFile"
exit 0
