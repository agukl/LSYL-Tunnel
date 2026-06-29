@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..\..") do set "WORKSPACE=%%~fI"
if "%GOEXE%"=="" set "GOEXE=go"
set "TARGET=%~1"
if "%TARGET%"=="" set "TARGET=all"

if /i "%TARGET%"=="win7-lite" (
  call "%SCRIPT_DIR%build-win7-lite.cmd"
  exit /b %ERRORLEVEL%
)

cd /d "%WORKSPACE%" || exit /b 1

call "%SCRIPT_DIR%go-env.cmd" "%WORKSPACE%" "%GOEXE%" || exit /b 1
"%GOEXE%" version >nul 2>nul || exit /b 1

if /i "%TARGET%"=="all" goto build_all
if /i "%TARGET%"=="client" goto build_client
if /i "%TARGET%"=="client-package" goto build_client_package
if /i "%TARGET%"=="win7-lite" goto build_win7_lite
if /i "%TARGET%"=="server" goto build_server
if /i "%TARGET%"=="profile" goto build_profile

echo [ERROR] Unknown build target: %TARGET%
goto usage

:build_all
call :build_server || exit /b 1
call :build_client || exit /b 1
call :build_profile || exit /b 1
echo Build completed.
goto :eof

:build_client
if not exist ".\build\bin\client" mkdir ".\build\bin\client"

echo [1/3] build lsyl-tunnel-client.exe
"%GOEXE%" build -trimpath -ldflags "-s -w" -o ".\build\bin\client\lsyl-tunnel-client.exe" ".\src\client\cmd\lsyl-tunnel-client" || exit /b 1

echo [2/3] build lsyl-tunnel-client-gui.exe
"%GOEXE%" build -trimpath -ldflags "-H windowsgui -s -w" -o ".\build\bin\client\lsyl-tunnel-client-gui.exe" ".\src\client\cmd\lsyl-tunnel-client-gui" || exit /b 1

echo [3/3] build lsyl-tunnel-client-lite.exe
call "%SCRIPT_DIR%build-win7-lite.cmd" || exit /b 1

echo Client build completed: %WORKSPACE%\build\bin\client
goto :eof

:build_client_package
if not exist ".\build\bin\client" mkdir ".\build\bin\client"
if exist ".\build\bin\client\lsyl-tunnel-client.exe" del /f /q ".\build\bin\client\lsyl-tunnel-client.exe" >nul 2>nul

echo [1/2] build lsyl-tunnel-client-gui.exe
"%GOEXE%" build -trimpath -ldflags "-H windowsgui -s -w" -o ".\build\bin\client\lsyl-tunnel-client-gui.exe" ".\src\client\cmd\lsyl-tunnel-client-gui" || exit /b 1

echo [2/2] build lsyl-tunnel-client-lite.exe
call "%SCRIPT_DIR%build-win7-lite.cmd" || exit /b 1

echo Client package build completed: %WORKSPACE%\build\bin\client
goto :eof

:build_win7_lite
call "%SCRIPT_DIR%build-win7-lite.cmd" || exit /b 1
goto :eof

:build_server
if not exist ".\build\bin\server" mkdir ".\build\bin\server"

echo [1/5] build lsyl-tunnel-server.exe
"%GOEXE%" build -trimpath -ldflags "-s -w" -o ".\build\bin\server\lsyl-tunnel-server.exe" ".\src\server\cmd\lsyl-tunnel-server" || exit /b 1

echo [2/5] build lsyl-tunnel-server-svc.exe
"%GOEXE%" build -trimpath -ldflags "-s -w" -o ".\build\bin\server\lsyl-tunnel-server-svc.exe" ".\src\server\cmd\lsyl-tunnel-server-svc" || exit /b 1

echo [3/5] build lsyl-tunnel-server-gui.exe
"%GOEXE%" build -trimpath -ldflags "-H windowsgui -s -w" -o ".\build\bin\server\lsyl-tunnel-server-gui.exe" ".\src\server\cmd\lsyl-tunnel-server-gui" || exit /b 1

echo [4/5] build lsyl-tunnel-passwd.exe
"%GOEXE%" build -trimpath -ldflags "-s -w" -o ".\build\bin\server\lsyl-tunnel-passwd.exe" ".\src\cmd\lsyl-tunnel-passwd" || exit /b 1

echo [5/5] build lsyl-tunnel-cert.exe
"%GOEXE%" build -trimpath -ldflags "-s -w" -o ".\build\bin\server\lsyl-tunnel-cert.exe" ".\src\cmd\lsyl-tunnel-cert" || exit /b 1

echo Server build completed: %WORKSPACE%\build\bin\server
goto :eof

:build_profile
if not exist ".\build\bin\profile" mkdir ".\build\bin\profile"

echo [1/1] build lsyl-tunnel-profile.exe
"%GOEXE%" build -trimpath -ldflags "-s -w" -o ".\build\bin\profile\lsyl-tunnel-profile.exe" ".\src\cmd\lsyl-tunnel-profile" || exit /b 1

echo Profile tool build completed: %WORKSPACE%\build\bin\profile
goto :eof

:usage
echo Usage: deploy\windows\build.cmd [all^|server^|client^|client-package^|win7-lite^|profile]
exit /b 1
