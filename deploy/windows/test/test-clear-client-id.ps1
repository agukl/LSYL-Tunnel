Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$workspace = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..\..'))
$sanitizer = Join-Path $workspace 'deploy\windows\inno\clear-client-id.ps1'
$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('lsyl-clear-client-id-' + [Guid]::NewGuid().ToString('N'))
$utf8Bom = New-Object Text.UTF8Encoding($true)
$utf8NoBom = New-Object Text.UTF8Encoding($false)

function Assert-True {
    param(
        [bool]$Condition,
        [string]$Message
    )
    if (-not $Condition) {
        throw $Message
    }
}

function Invoke-Sanitizer {
    param([string]$ConfigFile)

    $previousErrorAction = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $output = & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $sanitizer -ConfigFile $ConfigFile 2>&1
        $exitCode = $LASTEXITCODE
    }
    finally {
        $ErrorActionPreference = $previousErrorAction
    }
    return [pscustomobject]@{
        ExitCode = $exitCode
        Output = ($output -join [Environment]::NewLine)
    }
}

function Get-FileBase64 {
    param([string]$Path)
    return [Convert]::ToBase64String([IO.File]::ReadAllBytes($Path))
}

try {
    [IO.Directory]::CreateDirectory($testRoot) | Out-Null

    $validPath = Join-Path $testRoot 'valid.yaml'
    $validText = "config_version: 3`r`nserver_addr: example.test:443`r`nclient_id: site-machine`r`nmetadata:`r`n  client_id: nested-value`r`nforwards:`r`n  - name: rdp`r`n"
    [IO.File]::WriteAllText($validPath, $validText, $utf8Bom)
    $validResult = Invoke-Sanitizer -ConfigFile $validPath
    Assert-True ($validResult.ExitCode -eq 0) ("valid config failed: " + $validResult.Output)
    $validActual = [IO.File]::ReadAllText($validPath)
    $validExpected = $validText.Replace('client_id: site-machine', 'client_id: ""')
    Assert-True ($validActual -ceq $validExpected) 'sanitizer changed content outside the top-level client_id value'
    $validBytes = [IO.File]::ReadAllBytes($validPath)
    Assert-True ($validBytes.Length -ge 3 -and $validBytes[0] -eq 0xEF -and $validBytes[1] -eq 0xBB -and $validBytes[2] -eq 0xBF) 'sanitizer did not preserve the UTF-8 BOM'

    $missingPath = Join-Path $testRoot 'missing.yaml'
    [IO.File]::WriteAllText($missingPath, "server_addr: example.test:443`nforwards: []`n", $utf8NoBom)
    $missingBefore = Get-FileBase64 -Path $missingPath
    $missingResult = Invoke-Sanitizer -ConfigFile $missingPath
    Assert-True ($missingResult.ExitCode -ne 0) 'config without client_id was accepted'
    Assert-True ((Get-FileBase64 -Path $missingPath) -ceq $missingBefore) 'config without client_id was modified'

    $duplicatePath = Join-Path $testRoot 'duplicate.yaml'
    [IO.File]::WriteAllText($duplicatePath, "client_id: first`r`nserver_addr: example.test:443`r`nclient_id: second`r`n", $utf8NoBom)
    $duplicateBefore = Get-FileBase64 -Path $duplicatePath
    $duplicateResult = Invoke-Sanitizer -ConfigFile $duplicatePath
    Assert-True ($duplicateResult.ExitCode -ne 0) 'config with duplicate top-level client_id fields was accepted'
    Assert-True ((Get-FileBase64 -Path $duplicatePath) -ceq $duplicateBefore) 'config with duplicate client_id fields was modified'

    Write-Host '[PASS] client_id sanitizer behavior'
}
finally {
    if (Test-Path -LiteralPath $testRoot) {
        Remove-Item -LiteralPath $testRoot -Recurse -Force
    }
}
