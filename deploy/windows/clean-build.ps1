param(
    [switch]$All,
    [switch]$DryRun,
    [switch]$Help
)

$ErrorActionPreference = "Stop"
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$workspace = (Resolve-Path (Join-Path $scriptDir "..\..")).Path
$buildRoot = Join-Path $workspace "build"

if ($Help) {
    Write-Host "Usage: clean-build.cmd [/dry-run] [/all]"
    Write-Host ""
    Write-Host "  /dry-run  Show build items that would be removed."
    Write-Host "  /all      Also remove build\_toolchains."
    exit 0
}

if (-not (Test-Path -LiteralPath $buildRoot)) {
    Write-Host "[INFO] Build directory does not exist:"
    Write-Host "  $buildRoot"
    exit 0
}

$build = (Resolve-Path -LiteralPath $buildRoot).Path
$buildPrefix = $build.TrimEnd('\') + '\'
$removed = 0

function Assert-InBuild([string]$path) {
    $resolved = (Resolve-Path -LiteralPath $path).Path
    if ($resolved -eq $build) {
        throw "Refusing to remove build root directly: $resolved"
    }
    if (-not $resolved.StartsWith($buildPrefix, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to remove unexpected path: $resolved"
    }
    return $resolved
}

function Remove-BuildItem([string]$path) {
    if (-not (Test-Path -LiteralPath $path)) {
        return
    }

    $resolved = Assert-InBuild $path
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

$targets = @(
    (Join-Path $build "bin"),
    (Join-Path $build "tmp"),
    (Join-Path $build "tools"),
    (Join-Path $build "trim.txt")
)

$targets += Get-ChildItem -LiteralPath $build -Directory -Force |
    Where-Object { $_.Name -match '^[0-9a-f]{2}$' } |
    ForEach-Object { $_.FullName }

if ($All) {
    $targets += Join-Path $build "_toolchains"
}

Write-Host "[INFO] Cleaning build outputs under:"
Write-Host "  $build"
if (-not $All) {
    Write-Host "[INFO] Keeping build\_toolchains. Use /all to remove downloaded toolchains too."
}

foreach ($target in $targets | Select-Object -Unique) {
    Remove-BuildItem $target
}

if ($DryRun) {
    Write-Host "[INFO] Dry run complete."
} else {
    Write-Host "[INFO] Clean complete. Removed item count: $removed"
}
