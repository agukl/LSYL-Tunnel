[CmdletBinding()]
param(
    [switch]$Full,
    [switch]$Offline
)

$ErrorActionPreference = 'Stop'
$Workspace = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..\..'))

function Resolve-CheckTool {
    param([string]$Override, [string[]]$Names, [string[]]$Fallbacks, [string]$Hint)
    if ($Override) {
        $command = Get-Command $Override -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($command) { return $command.Source }
        throw "Configured tool not found: $Override. $Hint"
    }
    foreach ($name in $Names) {
        $command = Get-Command $name -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($command) { return $command.Source }
    }
    foreach ($path in $Fallbacks) {
        $candidate = Join-Path $Workspace $path
        if (Test-Path -LiteralPath $candidate -PathType Leaf) { return $candidate }
    }
    throw "Required tool not found. $Hint"
}

function Invoke-Check {
    param([string]$Label, [string]$Executable, [string[]]$Arguments)
    Write-Host "[CHECK] $Label"
    Write-Host "  $Executable $($Arguments -join ' ')"
    & $Executable @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$Label failed (exit $LASTEXITCODE)." }
}

# This entry deliberately does not invoke release, installers, or config generators.
$environmentNames = @('GOOS', 'GOARCH', 'CGO_ENABLED', 'GOTOOLCHAIN', 'JAVA_HOME', 'ANDROID_HOME')
$savedEnvironment = @{}
foreach ($name in $environmentNames) {
    $savedEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}

$exitCode = 0
Push-Location $Workspace
try {
    $go = Resolve-CheckTool -Override $env:GOEXE -Names @('go.exe') -Fallbacks @('tool\go\bin\go.exe') -Hint 'Set GOEXE or install Go in PATH or tool\go.'

    # Resolve the full suite before running tests, so missing prerequisites fail early.
    if ($Full) {
        $node = Resolve-CheckTool -Override $env:NODE_EXE -Names @('node.exe') -Fallbacks @('tool\node\node.exe') -Hint 'Set NODE_EXE or install Node.js in PATH or tool\node.'
        $nodeVersion = & $node --version
        if ($LASTEXITCODE -ne 0) {
            throw 'Node.js could not start.'
        }
        Write-Host "[TOOL] Node.js $nodeVersion"
        $go120 = Resolve-CheckTool -Override $env:GO120EXE -Names @('go1.20.14.exe', 'go1.20.exe', 'go120.exe') -Fallbacks @('tool\go1.20.14\go\bin\go.exe', 'tool\go1.20\go\bin\go.exe') -Hint 'Set GO120EXE or install Go 1.20 under tool\go1.20.14.'
        $go120Version = & $go120 version
        if ($LASTEXITCODE -ne 0 -or "$go120Version" -notmatch '\bgo1\.20(?:\.|\s)') {
            throw 'Win7 checks require the Go 1.20 toolchain.'
        }
        $gradle = Resolve-CheckTool -Override $env:GRADLE_EXE -Names @('gradle.bat') -Fallbacks @('tool\gradle-8.9\bin\gradle.bat', 'tool\gradle\bin\gradle.bat') -Hint 'Set GRADLE_EXE or install Gradle in PATH or tool\gradle-8.9.'
        $javaOverride = ''
        if ($env:JAVA_HOME) { $javaOverride = Join-Path $env:JAVA_HOME 'bin\java.exe' }
        $java = Resolve-CheckTool -Override $javaOverride -Names @('java.exe') -Fallbacks @('tool\jdk17\bin\java.exe') -Hint 'Set JAVA_HOME or install JDK 17+ in PATH or tool\jdk17.'
        $env:JAVA_HOME = Split-Path (Split-Path $java -Parent) -Parent
        if (-not $env:ANDROID_HOME -and -not $env:ANDROID_SDK_ROOT -and (Test-Path -LiteralPath (Join-Path $Workspace 'tool\android-sdk') -PathType Container)) {
            $env:ANDROID_HOME = Join-Path $Workspace 'tool\android-sdk'
        }
    }

    Invoke-Check 'Go tests' $go @('test', '-count=1', '-timeout=90s', './src/...')
    Invoke-Check 'Go vet' $go @('vet', './src/...')
    if ($Full) {
        Invoke-Check 'Frontend module tests' $node @('--test', 'src/server/front/tests/api.test.mjs', 'src/server/front/tests/format.test.mjs')
        $env:GOOS = 'windows'
        $env:GOARCH = '386'
        $env:CGO_ENABLED = '0'
        $env:GOTOOLCHAIN = 'local'
        try {
            Invoke-Check 'Win7 Go 1.20 baseline' $go120 @('test', '-modfile=go.win7.mod', '-mod=readonly', '-count=1', '-timeout=90s', './src/internal/protocol', './src/client/tunnel', './src/client/mobileprofile', './src/client/lite')
        }
        finally {
            foreach ($name in @('GOOS', 'GOARCH', 'CGO_ENABLED', 'GOTOOLCHAIN')) {
                [Environment]::SetEnvironmentVariable($name, $savedEnvironment[$name], 'Process')
            }
        }
        Push-Location (Join-Path $Workspace 'mobile\android')
        try {
            $gradleArgs = @('--no-daemon', '--console=plain')
            if ($Offline -or $env:ANDROID_OFFLINE -eq '1') { $gradleArgs += '--offline' }
            $gradleArgs += @(':app:testDebugUnitTest', '--rerun')
            Invoke-Check 'Android JVM tests' $gradle $gradleArgs
        }
        finally { Pop-Location }
        Write-Host '[PASS] Full source checks completed. No distribution package was generated.'
    }
    else {
        Write-Host '[PASS] Quick source checks completed (Go test + vet). Use check.cmd -Full for all platforms.'
    }
}
catch {
    Write-Host "[FAIL] $($_.Exception.Message)" -ForegroundColor Red
    $exitCode = 1
}
finally {
    foreach ($name in $environmentNames) {
        [Environment]::SetEnvironmentVariable($name, $savedEnvironment[$name], 'Process')
    }
    Pop-Location
}
exit $exitCode
