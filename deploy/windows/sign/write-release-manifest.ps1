param(
    [string]$OutputDir = "dist",
    [switch]$PackageOnly,
    [string[]]$Files
)

$ErrorActionPreference = "Stop"
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$workspace = Resolve-Path (Join-Path $scriptDir "..\..\..")
Set-Location $workspace

function Resolve-ExistingFile([string]$path) {
    if ([string]::IsNullOrWhiteSpace($path)) { return $null }
    if (Test-Path $path -PathType Leaf) {
        return (Resolve-Path $path).Path
    }
    return $null
}

function Get-DefaultReleaseFiles {
    $targets = @(
        "dist\LSYL Tunnel Client\bin\lsyl-tunnel-client-gui.exe",
        "dist\LSYL Tunnel Client\bin\lsyl-tunnel-client-lite.exe",
        "dist\LSYL Tunnel Lightweight Clients\android\lsyl-tunnel-mobile.apk",
        "dist\LSYL Tunnel Lightweight Clients\windows-win7\lsyl-tunnel-client-lite.exe",
        "dist\LSYL Tunnel Profile Tool\bin\lsyl-tunnel-profile.exe",
        "dist\LSYL Tunnel Server\bin\lsyl-tunnel-server.exe",
        "dist\LSYL Tunnel Server\bin\lsyl-tunnel-server-svc.exe",
        "dist\LSYL Tunnel Server\bin\lsyl-tunnel-server-gui.exe",
        "dist\LSYL Tunnel Server\bin\lsyl-tunnel-passwd.exe",
        "dist\LSYL Tunnel Server\bin\lsyl-tunnel-cert.exe"
    )
    if (-not $PackageOnly) {
        $targets += @(
            "dist\installers\LSYL-Tunnel-Client-Setup.exe",
            "dist\installers\LSYL-Tunnel-Server-Setup.exe"
        )
    }
    return $targets
}

if (-not $Files -or $Files.Count -eq 0) {
    $Files = Get-DefaultReleaseFiles
}

$expandedFiles = @()
foreach ($file in $Files) {
    foreach ($part in ([string]$file -split ',')) {
        $part = $part.Trim()
        if ($part) {
            $expandedFiles += $part
        }
    }
}
$Files = $expandedFiles

$resolvedFiles = @()
foreach ($file in $Files) {
    $resolved = Resolve-ExistingFile $file
    if ($resolved) {
        $resolvedFiles += $resolved
    }
}

if ($resolvedFiles.Count -eq 0) {
    throw "No release files found. Build dist packages or installers before writing the release manifest."
}

if (-not (Test-Path $OutputDir)) {
    New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
}

$buildTime = (Get-Date).ToString("yyyy-MM-ddTHH:mm:sszzz")
$entries = foreach ($file in $resolvedFiles) {
    $item = Get-Item $file
    $relative = Resolve-Path -Relative $file
    $relative = $relative -replace '^[.][\\/]',''
    $hash = Get-FileHash -Algorithm SHA256 -Path $file
    $sig = Get-AuthenticodeSignature -FilePath $file
    $vi = $item.VersionInfo
    [ordered]@{
        path = $relative
        file_name = $item.Name
        size_bytes = $item.Length
        sha256 = $hash.Hash
        signature_status = [string]$sig.Status
        signer_subject = if ($sig.SignerCertificate) { $sig.SignerCertificate.Subject } else { "" }
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
    package_only = [bool]$PackageOnly
    service = [ordered]@{
        name = "LSYLTunnelServer"
        display_name = "LSYL Tunnel Server"
        start_type = "manual"
        description = "LSYL Tunnel Server provides account-authenticated TLS tunnel and port forwarding. Manual start. Logs are written under logs."
    }
    notes = @(
        "The client does not register a Windows service.",
        "The server registers the fixed LSYLTunnelServer service with manual start.",
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
$lines.Add("Package only: $([bool]$PackageOnly)")
$lines.Add("")
$lines.Add("Windows service:")
$lines.Add("  Name: LSYLTunnelServer")
$lines.Add("  Display name: LSYL Tunnel Server")
$lines.Add("  Start type: manual")
$lines.Add("  Description: LSYL Tunnel Server provides account-authenticated TLS tunnel and port forwarding. Manual start. Logs are written under logs.")
$lines.Add("")
$lines.Add("Files:")
foreach ($entry in $entries) {
    $lines.Add("  $($entry.path)")
    $lines.Add("    SHA256: $($entry.sha256)")
    $lines.Add("    Size: $($entry.size_bytes)")
    $lines.Add("    Signature: $($entry.signature_status)")
    if ($entry.signer_subject) {
        $lines.Add("    Signer: $($entry.signer_subject)")
    }
    $lines.Add("    Product: $($entry.product_name)")
    $lines.Add("    Description: $($entry.file_description)")
    $lines.Add("    Version: $($entry.file_version)")
}
$lines.Add("")
$lines.Add("Notes:")
$lines.Add("  The client does not register a Windows service.")
$lines.Add("  The server registers the fixed LSYLTunnelServer service with manual start.")
$lines.Add("  Server uninstall keeps conf, certs, data, and logs by default.")
$lines.Add("  Formal releases should use a public code signing certificate. Self-signed certificates are for development or internal testing only.")
$lines | Set-Content -Encoding UTF8 -Path $txtPath

Write-Host "[INFO] Release manifest written:"
Write-Host "  $jsonPath"
Write-Host "  $txtPath"
