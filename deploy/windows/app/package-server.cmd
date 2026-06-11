@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..\..\..") do set "WORKSPACE=%%~fI"
set "PACKAGE_DIR=%WORKSPACE%\dist\LSYL Tunnel Server"
if not "%~1"=="" set "PACKAGE_DIR=%~1"
cd /d "%WORKSPACE%"
call "%SCRIPT_DIR%..\build.cmd" server || exit /b 1
if exist "%PACKAGE_DIR%" (
  rmdir /s /q "%PACKAGE_DIR%" 2>nul
  if exist "%PACKAGE_DIR%" (
    echo [WARN] Cannot remove package directory completely; refreshing files in place:
    echo   %PACKAGE_DIR%
    if exist "%PACKAGE_DIR%\install-server-app.cmd" del /f /q "%PACKAGE_DIR%\install-server-app.cmd" >nul 2>nul
    if exist "%PACKAGE_DIR%\uninstall-server-app.cmd" del /f /q "%PACKAGE_DIR%\uninstall-server-app.cmd" >nul 2>nul
    if exist "%PACKAGE_DIR%\uninstall-server-app.ps1" del /f /q "%PACKAGE_DIR%\uninstall-server-app.ps1" >nul 2>nul
    if exist "%PACKAGE_DIR%\install-server-app.cmd" (echo [ERROR] Cannot remove obsolete install-server-app.cmd & exit /b 1)
    if exist "%PACKAGE_DIR%\uninstall-server-app.cmd" (echo [ERROR] Cannot remove obsolete uninstall-server-app.cmd & exit /b 1)
    if exist "%PACKAGE_DIR%\uninstall-server-app.ps1" (echo [ERROR] Cannot remove obsolete uninstall-server-app.ps1 & exit /b 1)
  )
)
if not exist "%PACKAGE_DIR%" mkdir "%PACKAGE_DIR%" || exit /b 1
if not exist "%PACKAGE_DIR%\bin" mkdir "%PACKAGE_DIR%\bin" || exit /b 1
if not exist "%PACKAGE_DIR%\conf" mkdir "%PACKAGE_DIR%\conf" || exit /b 1
if not exist "%PACKAGE_DIR%\certs" mkdir "%PACKAGE_DIR%\certs" || exit /b 1
if not exist "%PACKAGE_DIR%\data" mkdir "%PACKAGE_DIR%\data" || exit /b 1
if not exist "%PACKAGE_DIR%\logs" mkdir "%PACKAGE_DIR%\logs" || exit /b 1
if not exist "%PACKAGE_DIR%\logs\request" mkdir "%PACKAGE_DIR%\logs\request" || exit /b 1
if not exist "%PACKAGE_DIR%\logs\business" mkdir "%PACKAGE_DIR%\logs\business" || exit /b 1
if not exist "%PACKAGE_DIR%\logs\entry-traffic" mkdir "%PACKAGE_DIR%\logs\entry-traffic" || exit /b 1
if not exist "%PACKAGE_DIR%\logs\flow-traffic" mkdir "%PACKAGE_DIR%\logs\flow-traffic" || exit /b 1
if not exist "%PACKAGE_DIR%\logs\service" mkdir "%PACKAGE_DIR%\logs\service" || exit /b 1
if not exist "%PACKAGE_DIR%\assets" mkdir "%PACKAGE_DIR%\assets" || exit /b 1
if not exist "%PACKAGE_DIR%\installer\Languages" mkdir "%PACKAGE_DIR%\installer\Languages" || exit /b 1
copy /y ".\build\bin\server\lsyl-tunnel-server.exe" "%PACKAGE_DIR%\bin\" >nul || exit /b 1
copy /y ".\build\bin\server\lsyl-tunnel-server-svc.exe" "%PACKAGE_DIR%\bin\" >nul || exit /b 1
copy /y ".\build\bin\server\lsyl-tunnel-server-gui.exe" "%PACKAGE_DIR%\bin\" >nul || exit /b 1
copy /y ".\build\bin\server\lsyl-tunnel-passwd.exe" "%PACKAGE_DIR%\bin\" >nul || exit /b 1
copy /y ".\build\bin\server\lsyl-tunnel-cert.exe" "%PACKAGE_DIR%\bin\" >nul || exit /b 1
copy /y ".\src\server\conf\server.yaml" "%PACKAGE_DIR%\conf\server.yaml" >nul || exit /b 1
if exist ".\src\server\assets\server.ico" copy /y ".\src\server\assets\server.ico" "%PACKAGE_DIR%\assets\server.ico" >nul || exit /b 1
if exist ".\src\server\assets\server.svg" copy /y ".\src\server\assets\server.svg" "%PACKAGE_DIR%\assets\server.svg" >nul
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$p='%PACKAGE_DIR%\conf\server.yaml';" ^
  "$t=[IO.File]::ReadAllText($p);" ^
  "$t=$t.Replace('../../../certs/','../certs/').Replace('..\..\..\certs\','..\certs\');" ^
  "$t=$t.Replace('../../certs/','../certs/').Replace('..\..\certs\','..\certs\');" ^
  "$t=$t.Replace('../../../runtime/data/','../data/').Replace('..\..\..\runtime\data\','..\data\');" ^
  "$t=$t.Replace('../../../runtime/logs/','../logs/').Replace('..\..\..\runtime\logs\','..\logs\');" ^
  "$t=$t.Replace('../../../data/server-state.json','../data/server-state.json').Replace('..\..\..\data\server-state.json','..\data\server-state.json');" ^
  "$t=$t.Replace('../../data/server-state.json','../data/server-state.json').Replace('..\..\data\server-state.json','..\data\server-state.json');" ^
  "$t=$t.Replace('../../../data/','../data/').Replace('..\..\..\data\','..\data\');" ^
  "$t=$t.Replace('../../data/','../data/').Replace('..\..\data\','..\data\');" ^
  "$t=$t.Replace('../../../logs/','../logs/').Replace('..\..\..\logs\','..\logs\');" ^
  "$t=$t.Replace('../../logs/','../logs/').Replace('..\..\logs\','..\logs\');" ^
  "[IO.File]::WriteAllText($p,$t,[Text.UTF8Encoding]::new($false))" || exit /b 1
copy /y "%SCRIPT_DIR%..\inno\make-server-installer.cmd" "%PACKAGE_DIR%\make-installer.cmd" >nul || exit /b 1
copy /y "%SCRIPT_DIR%..\inno\package-server.iss" "%PACKAGE_DIR%\installer\server.iss" >nul || exit /b 1
copy /y "%SCRIPT_DIR%..\inno\Languages\ChineseSimplified.isl" "%PACKAGE_DIR%\installer\Languages\ChineseSimplified.isl" >nul || exit /b 1
> "%PACKAGE_DIR%\README.txt" (
  echo LSYL Tunnel Server package
  echo.
  echo Build installer from this package:
  echo   make-installer.cmd
  echo.
  echo The package builder first uses ..\tools\inno\ISCC.exe when dist includes it.
  echo Otherwise install Inno Setup 6 or set INNO_SETUP_ISCC.
  echo.
  echo Generated installer:
  echo   ..\installers\LSYL-Tunnel-Server-Setup.exe
  echo.
  echo Default install path:
  echo   C:\Program Files\LSYL Tunnel Server
  echo.
  echo Build the installer and run it as Administrator.
  echo The installer generates certs\server.crt and certs\server.key if missing.
  echo Give certs\server.crt to client administrators and put it into the client cert directory.
  echo Keep server.key only on the server.
  echo.
  echo The installer registers the LSYLTunnelServer Windows service as manual start by default.
  echo Select the installer auto-start task only when the server should start automatically with Windows.
  echo Existing conf, certs, data and logs are preserved during install and default uninstall.
  echo Server logs are grouped under logs\request, logs\business, logs\entry-traffic, logs\flow-traffic and logs\service.
)
echo Server app package created:
echo   %PACKAGE_DIR%
endlocal
