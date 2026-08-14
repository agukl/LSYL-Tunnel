param(
    [Parameter(Mandatory = $true)]
    [string]$ConfigFile
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$temporaryFile = $null
$backupFile = $null
try {
    $resolvedPath = (Resolve-Path -LiteralPath $ConfigFile -ErrorAction Stop).ProviderPath
    $bytes = [IO.File]::ReadAllBytes($resolvedPath)
    $hasUtf8Bom = $bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF
    $textOffset = if ($hasUtf8Bom) { 3 } else { 0 }
    $utf8 = New-Object Text.UTF8Encoding($false, $true)
    $text = $utf8.GetString($bytes, $textOffset, $bytes.Length - $textOffset)

    $pattern = [regex]::new('(?m)^(client_id[ \t]*:[ \t]*)[^\r\n]*(\r?)$')
    $matches = $pattern.Matches($text)
    if ($matches.Count -ne 1) {
        throw "client config must contain exactly one top-level client_id field; found $($matches.Count)"
    }

    $replaceIdentity = [Text.RegularExpressions.MatchEvaluator]{
        param($match)
        return $match.Groups[1].Value + '""' + $match.Groups[2].Value
    }
    $updated = $pattern.Replace($text, $replaceIdentity, 1)
    if ($updated -ceq $text) {
        exit 0
    }

    $updatedBody = $utf8.GetBytes($updated)
    if ($hasUtf8Bom) {
        $updatedBytes = New-Object byte[] ($updatedBody.Length + 3)
        $updatedBytes[0] = 0xEF
        $updatedBytes[1] = 0xBB
        $updatedBytes[2] = 0xBF
        [Array]::Copy($updatedBody, 0, $updatedBytes, 3, $updatedBody.Length)
    }
    else {
        $updatedBytes = $updatedBody
    }

    $directory = [IO.Path]::GetDirectoryName($resolvedPath)
    $leafName = [IO.Path]::GetFileName($resolvedPath)
    $operationID = [Guid]::NewGuid().ToString('N')
    $temporaryFile = Join-Path $directory ($leafName + '.' + $operationID + '.tmp')
    $backupFile = Join-Path $directory ($leafName + '.' + $operationID + '.bak')
    [IO.File]::WriteAllBytes($temporaryFile, $updatedBytes)
    [IO.File]::Replace($temporaryFile, $resolvedPath, $backupFile)
    $temporaryFile = $null
    [IO.File]::Delete($backupFile)
    $backupFile = $null
}
catch {
    Write-Error ("Failed to clear client_id from {0}: {1}" -f $ConfigFile, $_.Exception.Message)
    exit 1
}
finally {
    if ($null -ne $temporaryFile -and [IO.File]::Exists($temporaryFile)) {
        [IO.File]::Delete($temporaryFile)
    }
    if ($null -ne $backupFile -and [IO.File]::Exists($backupFile)) {
        [IO.File]::Delete($backupFile)
    }
}
