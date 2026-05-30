@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..\..") do set "WORKSPACE=%%~fI"
set "GO120_VERSION=1.20.14"
set "GO120_ARCHIVE=go%GO120_VERSION%.windows-amd64.zip"
set "GO120_TOOLCHAIN=%WORKSPACE%\build\_toolchains\go%GO120_VERSION%"
set "OUT=%WORKSPACE%\build\bin\client\lsyl-tunnel-client-lite.exe"

cd /d "%WORKSPACE%" || exit /b 1

if not exist ".\go.win7.mod" (
  echo [ERROR] Missing go.win7.mod.
  exit /b 1
)

call :resolve_go120 || exit /b 1
call :verify_go120 || exit /b 1

if "%GOPROXY%"=="" set "GOPROXY=https://goproxy.cn,direct"
set "GOOS=windows"
set "GOARCH=386"
set "CGO_ENABLED=0"
set "GOTOOLCHAIN=local"

if not exist ".\build\bin\client" mkdir ".\build\bin\client"

echo build Win7 Lite client with %GO120_VERSION_TEXT%
echo output: %OUT%
"%GO120EXE%" build -modfile=go.win7.mod -mod=readonly -trimpath -ldflags "-H windowsgui -s -w" -o "%OUT%" ".\src\client\cmd\lsyl-tunnel-client-lite" || exit /b 1
echo Win7 Lite client build completed: %OUT%
goto :eof

:resolve_go120
if not "%GO120EXE%"=="" goto :eof
if exist "%GO120_TOOLCHAIN%\go\bin\go.exe" (
  set "GO120EXE=%GO120_TOOLCHAIN%\go\bin\go.exe"
  goto :eof
)
for %%G in (go1.20.14.exe go1.20.exe go120.exe) do (
  for /f "delims=" %%P in ('where %%G 2^>nul') do (
    set "GO120EXE=%%P"
    goto :eof
  )
)
if /i "%NO_GO120_DOWNLOAD%"=="1" (
  echo [ERROR] Go 1.20 toolchain not found. Set GO120EXE, install go1.20.14, or unset NO_GO120_DOWNLOAD to allow a local download.
  exit /b 1
)
call :download_go120 || exit /b 1
set "GO120EXE=%GO120_TOOLCHAIN%\go\bin\go.exe"
goto :eof

:download_go120
echo [INFO] Go 1.20 toolchain not found. Downloading local toolchain for Win7 build...
if not exist "%WORKSPACE%\build\_toolchains" mkdir "%WORKSPACE%\build\_toolchains"
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$ErrorActionPreference='Stop';" ^
  "[Net.ServicePointManager]::SecurityProtocol=[Net.SecurityProtocolType]::Tls12;" ^
  "$archive='%GO120_ARCHIVE%';" ^
  "$dst='%GO120_TOOLCHAIN%';" ^
  "$zip=Join-Path '%WORKSPACE%\build\_toolchains' $archive;" ^
  "$urls=@(('https://go.dev/dl/{0}' -f $archive),('https://golang.google.cn/dl/{0}' -f $archive),('https://dl.google.com/go/{0}' -f $archive));" ^
  "if(Test-Path $dst){ Remove-Item -LiteralPath $dst -Recurse -Force };" ^
  "$ok=$false;" ^
  "foreach($u in $urls){ try { Write-Host ('[INFO] Downloading '+$u); Invoke-WebRequest -UseBasicParsing -Uri $u -OutFile $zip; $ok=$true; break } catch { Write-Host ('[WARN] '+$_.Exception.Message) } };" ^
  "if(-not $ok){ throw 'failed to download Go 1.20 toolchain' };" ^
  "New-Item -ItemType Directory -Force -Path $dst | Out-Null;" ^
  "Add-Type -AssemblyName System.IO.Compression.FileSystem;" ^
  "[IO.Compression.ZipFile]::ExtractToDirectory($zip,$dst);" ^
  "Remove-Item -LiteralPath $zip -Force" || exit /b 1
if not exist "%GO120_TOOLCHAIN%\go\bin\go.exe" (
  echo [ERROR] Go 1.20 download did not produce go.exe.
  exit /b 1
)
goto :eof

:verify_go120
if "%GO120EXE%"=="" (
  echo [ERROR] GO120EXE is empty.
  exit /b 1
)
for /f "delims=" %%V in ('"%GO120EXE%" version 2^>nul') do set "GO120_VERSION_TEXT=%%V"
if "%GO120_VERSION_TEXT%"=="" (
  echo [ERROR] Cannot run Go 1.20 executable: %GO120EXE%
  exit /b 1
)
echo %GO120_VERSION_TEXT% | findstr /c:"go1.20." >nul
if errorlevel 1 (
  echo [ERROR] Win7 Lite build requires Go 1.20.x, got: %GO120_VERSION_TEXT%
  echo Set GO120EXE to a Go 1.20 go.exe.
  exit /b 1
)
goto :eof
