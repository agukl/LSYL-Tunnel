param(
    [switch]$All,
    [switch]$DryRun,
    [switch]$Help
)

$ErrorActionPreference = "Stop"
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$workspace = (Resolve-Path (Join-Path $scriptDir "..\..")).Path
$buildRoot = Join-Path $workspace "build"
$workspacePrefix = $workspace.TrimEnd('\') + '\'

if ($Help) {
    Write-Host "Usage: powershell -NoProfile -ExecutionPolicy Bypass -File deploy\windows\clean-build.ps1 [-DryRun] [-All]"
    Write-Host ""
    Write-Host "  -DryRun  Show generated items that would be removed."
    Write-Host "  -All     Also remove legacy build\_toolchains if present."
    exit 0
}

$build = $null
$buildPrefix = $null
if (Test-Path -LiteralPath $buildRoot) {
    $build = (Resolve-Path -LiteralPath $buildRoot).Path
    $buildPrefix = $build.TrimEnd('\') + '\'
}
$removed = 0

function Assert-CleanTarget([string]$path) {
    $resolved = (Resolve-Path -LiteralPath $path).Path
    if ($resolved -eq $workspace) {
        throw "Refusing to remove workspace root directly: $resolved"
    }
    if (-not $resolved.StartsWith($workspacePrefix, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to remove unexpected path: $resolved"
    }
    if ($build -and $resolved -eq $build) {
        throw "Refusing to remove build root directly: $resolved"
    }
    return $resolved
}

function Remove-CleanItem([string]$path) {
    if (-not (Test-Path -LiteralPath $path)) {
        return
    }

    $resolved = Assert-CleanTarget $path
    if ($DryRun) {
        Write-Host "[DRY-RUN] Would remove: $resolved"
        return
    }

    $lastError = $null
    for ($attempt = 1; $attempt -le 8; $attempt++) {
        try {
            Remove-Item -LiteralPath $resolved -Recurse -Force -ErrorAction Stop
            if (-not (Test-Path -LiteralPath $resolved)) {
                Write-Host "[INFO] Removed: $resolved"
                $script:removed++
                return
            }
        } catch {
            $lastError = $_
        }
        Start-Sleep -Milliseconds (200 * $attempt)
    }

    if (Test-Path -LiteralPath $resolved) {
        if ($lastError) {
            throw "Failed to remove after retries: $resolved. $($lastError.Exception.Message)"
        }
        throw "Failed to remove after retries: $resolved"
    }
}

$targets = New-Object System.Collections.Generic.List[string]

function Add-CleanTarget([string]$path) {
    if (-not [string]::IsNullOrWhiteSpace($path)) {
        $targets.Add($path)
    }
}

if ($build) {
    foreach ($name in @(
        "bin",
        "tmp",
        "tools",
        "release",
        "release.stage",
        "gocache",
        "gomodcache",
        "trim.txt"
    )) {
        Add-CleanTarget (Join-Path $build $name)
    }

    Get-ChildItem -LiteralPath $build -Directory -Force |
        Where-Object {
            $_.Name -match '^[0-9a-f]{2}$' -or
            $_.Name -like '*.stage' -or
            $_.Name -match '^(go-build|gocache|gomodcache|codex-gomodcache)'
        } |
        ForEach-Object { Add-CleanTarget $_.FullName }

    $toolchains = Join-Path $build "_toolchains"
    if (Test-Path -LiteralPath $toolchains) {
        Get-ChildItem -LiteralPath $toolchains -File -Force -ErrorAction SilentlyContinue |
            Where-Object { $_.Extension -in @(".zip", ".tmp", ".download") } |
            ForEach-Object { Add-CleanTarget $_.FullName }
    }
}

foreach ($path in @(
    (Join-Path $workspace "tmp"),
    (Join-Path $workspace "dist.stage"),
    (Join-Path $workspace "dist.previous"),
    (Join-Path $workspace "mobile\android\.gradle"),
    (Join-Path $workspace "mobile\android\.kotlin"),
    (Join-Path $workspace "mobile\android\build"),
    (Join-Path $workspace "mobile\android\app\build"),
    (Join-Path $workspace "src\client\tmp"),
    (Join-Path $workspace "src\server\tmp")
)) {
    Add-CleanTarget $path
}

if ($All) {
    if ($build) {
        Add-CleanTarget (Join-Path $build "_toolchains")
    }
}

Write-Host "[INFO] Cleaning generated outputs under:"
Write-Host "  $workspace"
if (-not $build) {
    Write-Host "[INFO] Build directory does not exist:"
    Write-Host "  $buildRoot"
}
if (-not $All) {
    Write-Host "[INFO] Keeping project-local tool directory. Use it for reusable Go/Gradle/Inno tools."
    Write-Host "[INFO] Legacy build\_toolchains is kept unless /all is used."
}

foreach ($target in $targets | Select-Object -Unique) {
    Remove-CleanItem $target
}

if ($DryRun) {
    Write-Host "[INFO] Dry run complete."
} else {
    Write-Host "[INFO] Clean complete. Removed item count: $removed"
}
