param(
    [Parameter(Mandatory = $true)][string]$PackageDir,
    [string]$InstallerPath = $env:WEBVIEW2_RUNTIME_INSTALLER
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
# A caller using PowerShell 7 can pass module paths to Windows PowerShell 5.1.
Import-Module (Join-Path $PSHOME 'Modules\Microsoft.PowerShell.Security\Microsoft.PowerShell.Security.psd1') -ErrorAction Stop

function Assert-MicrosoftInstaller([string]$Path) {
    $signature = Get-AuthenticodeSignature -LiteralPath $Path
    if ($signature.Status -ne 'Valid' -or $null -eq $signature.SignerCertificate -or
        $signature.SignerCertificate.GetNameInfo([Security.Cryptography.X509Certificates.X509NameType]::SimpleName, $false) -ne 'Microsoft Corporation') {
        throw "WebView2 installer must have a valid Microsoft Corporation signature: $Path"
    }
}

$workspace = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..\..\..')).Path
$cacheDir = Join-Path $workspace 'tool\webview2'
if ([string]::IsNullOrWhiteSpace($InstallerPath)) {
    $offlineInstaller = Join-Path $cacheDir 'MicrosoftEdgeWebView2RuntimeInstallerX64.exe'
    if (Test-Path -LiteralPath $offlineInstaller -PathType Leaf) {
        $InstallerPath = $offlineInstaller
    } else {
        $InstallerPath = Join-Path $cacheDir 'MicrosoftEdgeWebview2Setup.exe'
        if (-not (Test-Path -LiteralPath $InstallerPath -PathType Leaf)) {
            New-Item -ItemType Directory -Path $cacheDir -Force | Out-Null
            $downloadPath = Join-Path $cacheDir (([guid]::NewGuid().ToString('N')) + '.download.exe')
            try {
                [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
                Write-Host '[INFO] Downloading Microsoft WebView2 Evergreen bootstrapper...'
                Invoke-WebRequest -UseBasicParsing -Uri 'https://go.microsoft.com/fwlink/p/?LinkId=2124703' -OutFile $downloadPath -TimeoutSec 120
                Assert-MicrosoftInstaller $downloadPath
                Move-Item -LiteralPath $downloadPath -Destination $InstallerPath -Force
            } catch {
                throw "Cannot prepare WebView2 installer. Place the Microsoft installer under tool\webview2 or set WEBVIEW2_RUNTIME_INSTALLER. $($_.Exception.Message)"
            } finally {
                if (Test-Path -LiteralPath $downloadPath) { Remove-Item -LiteralPath $downloadPath -Force }
            }
        }
    }
}

$InstallerPath = (Resolve-Path -LiteralPath $InstallerPath).Path
Assert-MicrosoftInstaller $InstallerPath
$destination = Join-Path $PackageDir 'prerequisites'
New-Item -ItemType Directory -Path $destination -Force | Out-Null
Copy-Item -LiteralPath $InstallerPath -Destination (Join-Path $destination 'MicrosoftEdgeWebview2Setup.exe') -Force
Write-Host "[INFO] Packaged WebView2 prerequisite: $InstallerPath"
