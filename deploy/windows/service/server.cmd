@echo off
setlocal
set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..\..\..") do set "WORKSPACE=%%~fI"
cd /d "%WORKSPACE%"
set "SVC=LSYLTunnelServer"
set "ACTION=%~1"
if "%ACTION%"=="" goto usage
if /i "%ACTION%"=="install" goto install
if /i "%ACTION%"=="register" goto install
if /i "%ACTION%"=="start" goto start
if /i "%ACTION%"=="stop" goto stop
if /i "%ACTION%"=="status" goto status
if /i "%ACTION%"=="uninstall" goto uninstall
if /i "%ACTION%"=="remove" goto uninstall
if /i "%ACTION%"=="delete" goto uninstall
goto usage

:require_admin
powershell -NoProfile -ExecutionPolicy Bypass -Command "$p=[Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent(); if($p.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)){exit 0}; exit 1" >nul 2>nul
if errorlevel 1 (
  echo [ERROR] Please run this command as Administrator.
  exit /b 1
)
exit /b 0

:service_exists
sc query "%SVC%" >nul 2>nul
if errorlevel 1 (
  echo Service is not installed: %SVC%
  exit /b 1
)
exit /b 0

:install
call :require_admin || exit /b 1
if not exist ".\build\bin\server\lsyl-tunnel-server-svc.exe" call "%SCRIPT_DIR%..\build.cmd" server || exit /b 1
if not exist ".\build\bin\server\lsyl-tunnel-server-gui.exe" call "%SCRIPT_DIR%..\build.cmd" server || exit /b 1
if not exist ".\src\server\conf\server.yaml" (
  echo [ERROR] Missing src\server\conf\server.yaml.
  exit /b 1
)
if not exist ".\runtime\logs\service" mkdir ".\runtime\logs\service"
echo Registering server service: %SVC%
echo Display name: LSYL Tunnel Server
echo Description: LSYL Tunnel server for account-authenticated TLS tunnel and port forwarding.
echo Binary: %WORKSPACE%\build\bin\server\lsyl-tunnel-server-svc.exe
echo Config: %WORKSPACE%\src\server\conf\server.yaml
echo Log: %WORKSPACE%\runtime\logs\service\server-service-YYYY-MM-DD.log
set "SERVICE_RESULT=%WORKSPACE%\runtime\logs\service\service-register-error.txt"
del /f /q "%SERVICE_RESULT%" >nul 2>nul
"%WORKSPACE%\build\bin\server\lsyl-tunnel-server-gui.exe" -service-action install -service-exe "%WORKSPACE%\build\bin\server\lsyl-tunnel-server-svc.exe" -service-name "%SVC%" -start-type manual -config "%WORKSPACE%\src\server\conf\server.yaml" -log "%WORKSPACE%\runtime\logs\service\server-service.log" -result-file "%SERVICE_RESULT%"
if errorlevel 1 (
  echo [ERROR] Server Windows service registration failed.
  if exist "%SERVICE_RESULT%" (
    type "%SERVICE_RESULT%"
  ) else (
    echo Please run this command as Administrator and check whether %SVC% is being deleted.
  )
  exit /b 1
)
echo Server service installed or updated: %SVC%
echo Start type: manual. Use server.cmd start when you want to run it.
exit /b 0

:start
call :require_admin || exit /b 1
call :service_exists || exit /b 1
sc start "%SVC%"
exit /b %ERRORLEVEL%

:stop
call :require_admin || exit /b 1
call :service_exists || exit /b 1
sc stop "%SVC%"
exit /b %ERRORLEVEL%

:status
sc query "%SVC%" >nul 2>nul
if errorlevel 1 (
  echo Service is not installed: %SVC%
  exit /b 0
)
sc query "%SVC%"
echo.
sc qc "%SVC%"
echo.
sc qdescription "%SVC%"
exit /b 0

:uninstall
call :require_admin || exit /b 1
sc query "%SVC%" >nul 2>nul
if errorlevel 1 (
  echo Service already not installed: %SVC%
  exit /b 0
)
sc stop "%SVC%" >nul 2>nul
sc delete "%SVC%"
exit /b %ERRORLEVEL%

:usage
echo Usage: deploy\windows\service\server.cmd install^|start^|stop^|status^|uninstall
echo Alias: register = install, remove/delete = uninstall
exit /b 1
