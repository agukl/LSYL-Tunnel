param(
    [string]$Scope = "all"
)

$ErrorActionPreference = "Stop"
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$workspace = Resolve-Path (Join-Path $scriptDir "..\..\..")
Set-Location $workspace

if (-not $env:LSYL_SIGN_CERT_PFX -and -not $env:LSYL_SIGN_CERT_SHA1) {
    $thumbFile = Join-Path $workspace "certs\codesign-thumbprint.txt"
    if (Test-Path $thumbFile) {
        $env:LSYL_SIGN_CERT_SHA1 = ((Get-Content -Raw -Encoding ASCII $thumbFile) -replace '\s','').ToUpperInvariant()
    }
}

if (-not $env:LSYL_SIGN_CERT_PFX -and -not $env:LSYL_SIGN_CERT_SHA1) {
    Write-Host "[INFO] Code signing skipped. Run deploy\windows\sign\init-selfsigned-codesign.cmd, or set LSYL_SIGN_CERT_PFX / LSYL_SIGN_CERT_SHA1."
    exit 0
}

if (-not $env:LSYL_SIGN_TIMESTAMP_URL) {
    $env:LSYL_SIGN_TIMESTAMP_URL = "http://timestamp.digicert.com"
}

function Get-SignToolPath {
    if ($env:LSYL_SIGNTOOL) {
        $cmd = Get-Command $env:LSYL_SIGNTOOL -ErrorAction SilentlyContinue
        if ($cmd) { return $cmd.Source }
        if (Test-Path $env:LSYL_SIGNTOOL) { return (Resolve-Path $env:LSYL_SIGNTOOL).Path }
    }
    $cmd = Get-Command signtool.exe -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    $kits = Join-Path ${env:ProgramFiles(x86)} "Windows Kits\10\bin"
    if (Test-Path $kits) {
        $found = Get-ChildItem $kits -Recurse -Filter signtool.exe -ErrorAction SilentlyContinue | Sort-Object FullName -Descending | Select-Object -First 1
        if ($found) { return $found.FullName }
    }
    return $null
}

function Get-Targets([string]$scope) {
    $clientPackage = @(
        "dist\LSYL Tunnel Client\bin\lsyl-tunnel-client-gui.exe",
        "dist\LSYL Tunnel Client\bin\lsyl-tunnel-client-lite.exe"
    )
    $lightweightPackage = @(
        "dist\LSYL Tunnel Lightweight Clients\windows-win7\lsyl-tunnel-client-lite.exe"
    )
    $profilePackage = @("dist\LSYL Tunnel Profile Tool\bin\lsyl-tunnel-profile.exe")
    $serverPackage = @(
        "dist\LSYL Tunnel Server\bin\lsyl-tunnel-server.exe",
        "dist\LSYL Tunnel Server\bin\lsyl-tunnel-server-svc.exe",
        "dist\LSYL Tunnel Server\bin\lsyl-tunnel-server-gui.exe",
        "dist\LSYL Tunnel Server\bin\lsyl-tunnel-passwd.exe",
        "dist\LSYL Tunnel Server\bin\lsyl-tunnel-cert.exe"
    )
    $clientInstaller = @("dist\installers\LSYL-Tunnel-Client-Setup.exe")
    $serverInstaller = @("dist\installers\LSYL-Tunnel-Server-Setup.exe")

    switch ($scope.ToLowerInvariant()) {
        "all" { return @($clientPackage + $lightweightPackage + $profilePackage + $serverPackage + $clientInstaller + $serverInstaller) }
        "package" { return @($clientPackage + $lightweightPackage + $profilePackage + $serverPackage) }
        "client-package" { return $clientPackage }
        "lightweight-package" { return $lightweightPackage }
        "profile-package" { return $profilePackage }
        "server-package" { return $serverPackage }
        "installers" { return @($clientInstaller + $serverInstaller) }
        "client-installer" { return $clientInstaller }
        "server-installer" { return $serverInstaller }
        default { throw "Unknown signing scope: $scope" }
    }
}

function Get-StoreCertificate([string]$thumbprint) {
    $needle = ($thumbprint -replace '\s','').ToUpperInvariant()
    foreach ($store in @("Cert:\CurrentUser\My", "Cert:\LocalMachine\My")) {
        $cert = Get-ChildItem $store -ErrorAction SilentlyContinue | Where-Object {
            (($_.Thumbprint -replace '\s','').ToUpperInvariant()) -eq $needle
        } | Select-Object -First 1
        if ($cert) { return $cert }
    }
    return $null
}

function Invoke-SignToolCommand([string]$signTool, [string[]]$signArgs) {
    $oldErrorActionPreference = $ErrorActionPreference
    try {
        $ErrorActionPreference = "Continue"
        $output = & $signTool @signArgs 2>&1
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $oldErrorActionPreference
    }
    [pscustomobject][ordered]@{
        ExitCode = $exitCode
        Output = @($output)
    }
}

function Write-SignToolOutput($result) {
    foreach ($line in $result.Output) {
        if ($line) { Write-Host $line }
    }
}

function Sign-WithSignTool([string]$target, [string]$signTool) {
    $args = @("sign")
    if ($env:LSYL_SIGN_CERT_PFX) {
        if (-not (Test-Path $env:LSYL_SIGN_CERT_PFX)) {
            throw "PFX file not found: $env:LSYL_SIGN_CERT_PFX"
        }
        $args += @("/f", $env:LSYL_SIGN_CERT_PFX)
        if ($env:LSYL_SIGN_CERT_PASSWORD) {
            $args += @("/p", $env:LSYL_SIGN_CERT_PASSWORD)
        }
    } else {
        $args += @("/sha1", $env:LSYL_SIGN_CERT_SHA1)
    }
    $baseArgs = @($args + @("/fd", "SHA256"))
    if ($env:LSYL_SIGN_TIMESTAMP_URL) {
        $result = Invoke-SignToolCommand -signTool $signTool -signArgs @($baseArgs + @("/tr", $env:LSYL_SIGN_TIMESTAMP_URL, "/td", "SHA256", $target))
        if ($result.ExitCode -ne 0) {
            Write-Host "[WARN] Timestamp signing failed. Retrying without timestamp: $target"
            $result = Invoke-SignToolCommand -signTool $signTool -signArgs @($baseArgs + @($target))
            if ($result.ExitCode -ne 0) {
                Write-SignToolOutput $result
                throw "signtool sign failed: $target"
            }
        }
    } else {
        $result = Invoke-SignToolCommand -signTool $signTool -signArgs @($baseArgs + @($target))
        if ($result.ExitCode -ne 0) {
            Write-SignToolOutput $result
            throw "signtool sign failed: $target"
        }
    }
    $verify = Invoke-SignToolCommand -signTool $signTool -signArgs @("verify", "/pa", "/v", $target)
    if ($verify.ExitCode -ne 0) {
        Write-SignToolOutput $verify
        throw "signtool verify failed: $target"
    }
}

function Sign-WithPowerShell([string]$target, [System.Security.Cryptography.X509Certificates.X509Certificate2]$cert) {
    $params = @{ FilePath = $target; Certificate = $cert; HashAlgorithm = "SHA256" }
    if ($env:LSYL_SIGN_TIMESTAMP_URL) { $params.TimestampServer = $env:LSYL_SIGN_TIMESTAMP_URL }
    $sig = Set-AuthenticodeSignature @params
    if ($sig.Status -ne "Valid" -and $params.ContainsKey("TimestampServer")) {
        Write-Host "[WARN] Timestamp signing status: $($sig.Status). Retrying without timestamp."
        $params.Remove("TimestampServer")
        $sig = Set-AuthenticodeSignature @params
    }
    $check = Get-AuthenticodeSignature -FilePath $target
    if (-not $check.SignerCertificate) {
        throw "No Authenticode signature was written: $target"
    }
    Write-Host "[INFO] Signed by: $($check.SignerCertificate.Subject)"
}

$signTool = Get-SignToolPath
$usePowerShell = $false
$storeCert = $null

if ($signTool) {
    Write-Host "[INFO] Using signtool: $signTool"
} elseif ($env:LSYL_SIGN_CERT_SHA1) {
    $storeCert = Get-StoreCertificate $env:LSYL_SIGN_CERT_SHA1
    if (-not $storeCert) {
        throw "Signing certificate not found in certificate store: $env:LSYL_SIGN_CERT_SHA1"
    }
    $usePowerShell = $true
    Write-Host "[INFO] signtool.exe not found. Using PowerShell Authenticode signing with certificate thumbprint."
} elseif ($env:LSYL_SIGN_CERT_PFX) {
    throw "signtool.exe not found. PFX signing requires Windows SDK signtool, or import the cert and set LSYL_SIGN_CERT_SHA1."
}

$failed = $false
foreach ($target in Get-Targets $Scope) {
    if (-not (Test-Path $target)) {
        Write-Host "[INFO] Sign target not found, skipped: $target"
        continue
    }
    Write-Host "[INFO] Signing: $target"
    try {
        if ($usePowerShell) {
            Sign-WithPowerShell -target $target -cert $storeCert
        } else {
            Sign-WithSignTool -target $target -signTool $signTool
        }
    } catch {
        $failed = $true
        Write-Host "[ERROR] $($_.Exception.Message)"
    }
}

if ($failed) { exit 1 }
exit 0
