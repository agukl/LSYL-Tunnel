param(
    [string]$OutputDir = "dist",
    [string]$DistDir = "dist",
    [string[]]$Files
)

$ErrorActionPreference = "Stop"
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$workspace = Resolve-Path (Join-Path $scriptDir "..\..\..")
Set-Location $workspace

function Resolve-ExistingFile([string]$path) {
    if ([string]::IsNullOrWhiteSpace($path)) { return $null }
    if (Test-Path $path -PathType Leaf) { return (Resolve-Path $path).Path }
    return $null
}

function Get-DefaultReleaseFiles([string]$root) {
    @(
        (Join-Path $root "LSYL Tunnel Client\bin\lsyl-tunnel-client-gui.exe"),
        (Join-Path $root "LSYL Tunnel Client\bin\lsyl-tunnel-client-lite.exe"),
        (Join-Path $root "LSYL Tunnel Client\bin\WinDivert.dll"),
        (Join-Path $root "LSYL Tunnel Client\bin\WinDivert64.sys"),
        (Join-Path $root "installers\LSYL-Tunnel-Client-Setup.exe"),
        (Join-Path $root "installers\LSYL-Tunnel-Server-Setup.exe"),
        (Join-Path $root "installers\LSYL-Tunnel-Android.apk")
    ) | Where-Object { Test-Path $_ -PathType Leaf }
}

function Get-RelativeToRoot([string]$path, [string]$root) {
    $full = (Resolve-Path $path).Path
    $prefix = (Resolve-Path $root).Path.TrimEnd('\') + '\'
    if ($full.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)) {
        return $full.Substring($prefix.Length)
    }
    $relative = Resolve-Path -Relative $full
    return ($relative -replace '^[.][\\/]','')
}

if (-not $Files -or $Files.Count -eq 0) {
    $Files = Get-DefaultReleaseFiles $DistDir
}

$expandedFiles = @()
foreach ($file in $Files) {
    foreach ($part in ([string]$file -split ',')) {
        $part = $part.Trim()
        if ($part) { $expandedFiles += $part }
    }
}
$Files = $expandedFiles

$resolvedFiles = @()
foreach ($file in $Files) {
    $resolved = Resolve-ExistingFile $file
    if ($resolved) { $resolvedFiles += $resolved }
}

if ($resolvedFiles.Count -eq 0) {
    throw "No release files found. Build dist before writing the release manifest."
}

if (-not (Test-Path $OutputDir)) {
    New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
}

$buildTime = (Get-Date).ToString("yyyy-MM-ddTHH:mm:sszzz")
$entries = foreach ($file in $resolvedFiles) {
    $item = Get-Item $file
    $relative = Get-RelativeToRoot $file $DistDir
    $hash = Get-FileHash -Algorithm SHA256 -Path $file
    $isApk = $item.Extension -ieq ".apk"
    if ($isApk) {
        $sigStatus = "NotApplicable"
        $signerSubject = ""
    } else {
        $sig = Get-AuthenticodeSignature -FilePath $file
        $sigStatus = [string]$sig.Status
        $signerSubject = if ($sig.SignerCertificate) { $sig.SignerCertificate.Subject } else { "" }
    }
    $vi = $item.VersionInfo
    [ordered]@{
        path = $relative
        file_name = $item.Name
        size_bytes = $item.Length
        sha256 = $hash.Hash
        signature_status = $sigStatus
        signer_subject = $signerSubject
        product_name = $vi.ProductName
        company_name = $vi.CompanyName
        file_description = $vi.FileDescription
        file_version = $vi.FileVersion
        original_filename = $vi.OriginalFilename
    }
}

$manifest = [ordered]@{
    product = "LSYL Tunnel"
    build_time = $buildTime
    workspace = $workspace.Path
    dist_dir = (Resolve-Path $DistDir).Path
    binary_build = [ordered]@{
        stripped = $true
        go_build_flags = "-trimpath -ldflags '-s -w'"
        note = "Release binaries use Go official symbol stripping only. No obfuscation or executable packing is applied."
    }
    service = [ordered]@{
        name = "LSYLTunnelServer"
        display_name = "LSYL Tunnel Server"
        start_type = "manual"
        description = "LSYL Tunnel Server provides account-authenticated TLS tunnel and port forwarding. Manual start. Logs are written under logs."
    }
    notes = @(
        "Final dist is generated directly from source through build\tmp\dist-work and dist.stage.",
        "The server is delivered as installers\LSYL-Tunnel-Server-Setup.exe only.",
        "The client kit directory is included by default and omitted only when release.cmd /no-client-kit is used.",
        "Server uninstall keeps conf, certs, data, and logs by default.",
        "Formal releases should use a public code signing certificate. Self-signed certificates are for development or internal testing only."
    )
    files = @($entries)
}

$jsonPath = Join-Path $OutputDir "release-manifest.json"
$txtPath = Join-Path $OutputDir "release-manifest.txt"

$manifest | ConvertTo-Json -Depth 6 | Set-Content -Encoding UTF8 -Path $jsonPath

$lines = New-Object System.Collections.Generic.List[string]
$lines.Add("LSYL Tunnel Release Manifest")
$lines.Add("Build time: $buildTime")
$lines.Add("Dist: $((Resolve-Path $DistDir).Path)")
$lines.Add("Binary symbol stripping: True")
$lines.Add("  Go flags: -trimpath -ldflags '-s -w'")
$lines.Add("  Note: Go official symbol stripping only; no obfuscation or executable packing.")
$lines.Add("")
$lines.Add("Windows service:")
$lines.Add("  Name: LSYLTunnelServer")
$lines.Add("  Display name: LSYL Tunnel Server")
$lines.Add("  Start type: manual")
$lines.Add("")
$lines.Add("Files:")
foreach ($entry in $entries) {
    $lines.Add("  $($entry.path)")
    $lines.Add("    SHA256: $($entry.sha256)")
    $lines.Add("    Size: $($entry.size_bytes)")
    $lines.Add("    Signature: $($entry.signature_status)")
    if ($entry.signer_subject) { $lines.Add("    Signer: $($entry.signer_subject)") }
    $lines.Add("    Product: $($entry.product_name)")
    $lines.Add("    Description: $($entry.file_description)")
    $lines.Add("    Version: $($entry.file_version)")
}
$lines.Add("")
$lines.Add("Notes:")
foreach ($note in $manifest.notes) { $lines.Add("  $note") }
$lines | Set-Content -Encoding UTF8 -Path $txtPath

Write-Host "[INFO] Release manifest written:"
Write-Host "  $jsonPath"
Write-Host "  $txtPath"
