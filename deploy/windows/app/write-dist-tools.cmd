@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..\..\..") do set "WORKSPACE=%%~fI"
set "DIST_DIR=%WORKSPACE%\dist.stage"
if not "%~1"=="" set "DIST_DIR=%~1"

if not exist "%DIST_DIR%" mkdir "%DIST_DIR%" || exit /b 1
copy /y "%SCRIPT_DIR%..\inno\make-installers.cmd" "%DIST_DIR%\make-installers.cmd" >nul || exit /b 1
if /i "%LSYL_BUNDLE_INNO%"=="1" call "%SCRIPT_DIR%..\inno\bundle-inno.cmd" "%DIST_DIR%\tools\inno" || exit /b 1

> "%DIST_DIR%\README.txt" (
  echo LSYL Tunnel release dist
  echo.
  echo Installers:
  echo   installers\LSYL-Tunnel-Client-Setup.exe
  echo   installers\LSYL-Tunnel-Server-Setup.exe
  echo   installers\LSYL-Tunnel-Android.apk
  echo.
  echo Client kit:
  echo   LSYL Tunnel Client\
  echo.
  echo The client kit is present in default release.cmd output.
  echo Use release.cmd /no-client-kit only for a compact installer-only dist.
  echo To regenerate only the Windows client installer after editing client config and cert:
  echo   make-installers.cmd
  echo.
  echo Server and Android installers are fixed release outputs under installers\.
  echo Optional Inno compiler is under tools\inno only when release.cmd /bundle-inno is used.
)

echo Dist client kit tools written:
echo   %DIST_DIR%\make-installers.cmd
echo   %DIST_DIR%\README.txt
if exist "%DIST_DIR%\tools\inno\ISCC.exe" echo   %DIST_DIR%\tools\inno\ISCC.exe
endlocal
