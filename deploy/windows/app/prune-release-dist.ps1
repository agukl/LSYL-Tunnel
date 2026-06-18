param(
    [string]$DistDir = "dist"
)

$ErrorActionPreference = "Stop"
trap {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}

if (-not (Test-Path -LiteralPath $DistDir)) {
    exit 0
}

$dist = (Resolve-Path -LiteralPath $DistDir).Path
$distPrefix = $dist.TrimEnd('\') + '\'

function Remove-DistDirectory([string]$name) {
    $target = Join-Path $dist $name
    if (-not (Test-Path -LiteralPath $target)) {
        return
    }

    $resolved = (Resolve-Path -LiteralPath $target).Path
    if (-not $resolved.StartsWith($distPrefix, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to remove unexpected path: $resolved"
    }

    $lastError = $null
    for ($attempt = 1; $attempt -le 12; $attempt++) {
        try {
            Remove-Item -LiteralPath $resolved -Recurse -Force -ErrorAction Stop
            if (-not (Test-Path -LiteralPath $resolved)) {
                Write-Host "[INFO] Removed release-only intermediate: $resolved"
                return
            }
        } catch {
            $lastError = $_
        }
        Start-Sleep -Milliseconds (250 * $attempt)
    }

    if (Test-Path -LiteralPath $resolved) {
        if ($lastError) {
            throw "Failed to remove release-only intermediate after retries: $resolved. $($lastError.Exception.Message)"
        }
        throw "Failed to remove release-only intermediate after retries: $resolved"
    }
}

Remove-DistDirectory "LSYL Tunnel Server"
Remove-DistDirectory "LSYL Tunnel Lightweight Clients"
