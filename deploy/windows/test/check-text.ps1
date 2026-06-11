param(
    [string[]]$Roots = @("src", "deploy", "docs", "README.md", "release.cmd")
)

$ErrorActionPreference = "Stop"

$workspace = Resolve-Path (Join-Path $PSScriptRoot "..\..\..")
$utf8 = New-Object System.Text.UTF8Encoding($false, $true)
$skipExtensions = @(
    ".exe", ".ico", ".crt", ".key", ".syso", ".png", ".jpg", ".jpeg", ".gif", ".pfx", ".cer", ".dll", ".zip"
)
$badCodePoints = @(
    0xfffd,
    0x93c8,
    0x9422,
    0x7035,
    0x5bb8,
    0x95ab,
    0x95c5
)
$badPatterns = @()
foreach ($codePoint in $badCodePoints) {
    $badPatterns += [string][char]$codePoint
}
$badPatterns += @(
    "?/" + "span",
    "?/" + "label",
    "?/" + "button",
    "?/" + "div"
)

function Get-TextFiles([string]$Root) {
    $path = Join-Path $workspace $Root
    if (-not (Test-Path $path)) {
        return
    }
    $item = Get-Item $path
    if ($item.PSIsContainer) {
        Get-ChildItem $item.FullName -Recurse -File
    } else {
        $item
    }
}

$failed = $false
$self = $PSCommandPath
$files = foreach ($root in $Roots) {
    Get-TextFiles $root
}

foreach ($file in $files) {
    if ($null -eq $file) {
        continue
    }
    if ($file.FullName -eq $self) {
        continue
    }
    if ($file.FullName -like "*\build\*" -or $file.FullName -like "*\runtime\*" -or $file.FullName -like "*\tmp\*" -or $file.FullName -like "*\.git\*") {
        continue
    }
    if ($skipExtensions -contains $file.Extension.ToLowerInvariant()) {
        continue
    }

    try {
        $text = [System.IO.File]::ReadAllText($file.FullName, $utf8)
    } catch {
        Write-Host ("[ERROR] Text file is not valid UTF-8: " + $file.FullName)
        $failed = $true
        continue
    }

    foreach ($pattern in $badPatterns) {
        if ($text.Contains($pattern)) {
            Write-Host ("[ERROR] Possible mojibake or broken HTML: " + $file.FullName)
            $failed = $true
            break
        }
    }
}

if ($failed) {
    exit 1
}

Write-Host "text encoding check PASS"
