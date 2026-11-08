$ErrorActionPreference = 'Stop'
$workspace = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..\..'))
$fixture = Join-Path ([IO.Path]::GetTempPath()) ('lsyl-release-test-' + [Guid]::NewGuid().ToString('N'))

function Assert-True([bool]$value, [string]$message) {
    if (-not $value) { throw $message }
}

function Write-FixtureFile([string]$relative, [string]$content) {
    $path = Join-Path $fixture $relative
    [IO.Directory]::CreateDirectory((Split-Path $path -Parent)) | Out-Null
    [IO.File]::WriteAllText($path, $content, [Text.Encoding]::ASCII)
}

try {
    $buildScript = Get-Content -LiteralPath (Join-Path $workspace 'deploy\windows\build.cmd') -Raw
    $packageTarget = [regex]::Match($buildScript, '(?ms)^:build_client_package\r?\n(.*?)(?=^:build_win7_lite\r?$)')
    Assert-True $packageTarget.Success 'The client-package build target is missing.'
    Assert-True ($packageTarget.Value -notmatch 'WinDivert(?:64\.sys|\.dll)') 'Release builds must not overwrite the development WinDivert runtime.'
    $packageScript = Get-Content -LiteralPath (Join-Path $workspace 'deploy\windows\app\package-client.cmd') -Raw
    foreach ($name in @('WinDivert.dll', 'WinDivert64.sys')) {
        $copy = 'copy /y ".\third_party\windivert\2.2.2\x64\' + $name + '" "%PACKAGE_DIR%\bin\' + $name + '"'
        Assert-True ($packageScript.Contains($copy)) "The client package must obtain $name directly from third_party."
    }

    [IO.Directory]::CreateDirectory($fixture) | Out-Null
    Copy-Item -LiteralPath (Join-Path $workspace 'release.cmd') -Destination (Join-Path $fixture 'release.cmd')
    Write-FixtureFile 'LICENSE' 'test license'
    Write-FixtureFile 'dist\previous-release.txt' 'keep the previous successful release'
    Write-FixtureFile 'build\tmp\release-result.txt' 'Status: SUCCESS'
    Write-FixtureFile 'deploy\windows\go-env.cmd' "@echo off`r`nexit /b 7`r`n"
    Push-Location $fixture
    try {
        $output = & $env:ComSpec /d /c 'release.cmd /skip-test' 2>&1
        $code = $LASTEXITCODE
        Assert-True ($code -ne 0) 'Toolchain failure must return a nonzero exit code.'
        $resultPath = Join-Path $fixture 'build\tmp\release-result.txt'
        $result = Get-Content -LiteralPath $resultPath -Raw
        Assert-True ($result.Contains('Status: FAILED')) 'A previous SUCCESS must not survive a failed release.'
        Assert-True ($result.Contains('Step: Resolve Go toolchain')) 'Toolchain failure must name the failed step.'

        Write-FixtureFile 'deploy\windows\go-env.cmd' "@echo off`r`nexit /b 0`r`n"
        Write-FixtureFile 'deploy\windows\app\package-client.cmd' "@echo off`r`necho Simulated locked build dependency`r`nexit /b 9`r`n"
        $output = & $env:ComSpec /d /c 'release.cmd /skip-test' 2>&1
        $code = $LASTEXITCODE
        Assert-True ($code -eq 9) 'Package failure must preserve the command exit code.'
        Assert-True (($output -join "`n").Contains('Existing dist files belong to an earlier release')) 'Failure must clearly distinguish old dist files from this run.'
        Assert-True (-not (($output -join "`n").Contains('Release ready'))) 'A failed release must not report success.'
        $result = Get-Content -LiteralPath $resultPath -Raw
        Assert-True ($result.Contains('Step: Build client package')) 'Package failure must name the failed step.'
        $oldDist = Get-Content -LiteralPath (Join-Path $fixture 'dist\previous-release.txt') -Raw
        Assert-True ($oldDist -eq 'keep the previous successful release') 'An early failure must preserve the previous dist.'
    }
    finally { Pop-Location }
    Write-Host '[PASS] Release runtime isolation and failure reporting.'
}
finally {
    $tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') + '\'
    $resolved = [IO.Path]::GetFullPath($fixture)
    if (-not $resolved.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase) -or
        (Split-Path $resolved -Leaf) -notmatch '^lsyl-release-test-[0-9a-f]{32}$') {
        throw 'Refusing to remove a test directory outside the temporary fixture root.'
    }
    if (Test-Path -LiteralPath $resolved) { Remove-Item -LiteralPath $resolved -Recurse -Force }
}
