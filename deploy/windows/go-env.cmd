@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "WORKSPACE=%~1"
set "GOEXE=%~2"
if "%WORKSPACE%"=="" set "WORKSPACE=%CD%"
if "%GOEXE%"=="" set "GOEXE=go"
set "TOOL_DIR=%WORKSPACE%\tool"

call :resolve_go || exit /b 1

if "%GOCACHE%"=="" set "GOCACHE=%WORKSPACE%\build\gocache"
if "%GOPROXY%"=="" set "GOPROXY=https://goproxy.cn,direct"
if not exist "%GOCACHE%" mkdir "%GOCACHE%" >nul 2>nul

if not "%GOMODCACHE%"=="" goto done

set "DEFAULT_GOMODCACHE="
for /f "delims=" %%M in ('"%GOEXE%" env GOMODCACHE 2^>nul') do (
  if not defined DEFAULT_GOMODCACHE set "DEFAULT_GOMODCACHE=%%M"
)
if "%DEFAULT_GOMODCACHE%"=="" goto done

if not exist "%DEFAULT_GOMODCACHE%" mkdir "%DEFAULT_GOMODCACHE%" >nul 2>nul
set "GOMODCACHE_PROBE=%DEFAULT_GOMODCACHE%\.lsyl-write-test-%RANDOM%-%RANDOM%.tmp"
copy /y nul "%GOMODCACHE_PROBE%" >nul 2>nul
if exist "%GOMODCACHE_PROBE%" (
  del /f /q "%GOMODCACHE_PROBE%" >nul 2>nul
  goto done
)

echo [ERROR] Default Go module cache is not writable: %DEFAULT_GOMODCACHE%
echo [ERROR] Packaging stopped. Fix the Go module cache permissions before packaging.
exit /b 1

:done
endlocal & set "GOEXE=%GOEXE%" & set "GOCACHE=%GOCACHE%" & set "GOPROXY=%GOPROXY%"
exit /b 0

:resolve_go
echo "%GOEXE%" | findstr /c:"\\" /c:":" >nul
if not errorlevel 1 (
  if exist "%GOEXE%" exit /b 0
  echo [ERROR] Go executable not found:
  echo   %GOEXE%
  exit /b 1
)

for /f "delims=" %%G in ('where "%GOEXE%" 2^>nul') do (
  set "GOEXE=%%G"
  exit /b 0
)

if /i "%GOEXE%"=="go" (
  if exist "%TOOL_DIR%\go\bin\go.exe" (
    set "GOEXE=%TOOL_DIR%\go\bin\go.exe"
    exit /b 0
  )
)

echo [ERROR] Go executable not found from PATH or project tool directory.
echo [ERROR] Install Go and add it to PATH, set GOEXE, or place go.exe at:
echo   %TOOL_DIR%\go\bin\go.exe
exit /b 1
