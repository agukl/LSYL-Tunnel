@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..\..\..") do set "WORKSPACE=%%~fI"
set "DIST_DIR=%WORKSPACE%\dist"
if not "%~1"=="" set "DIST_DIR=%~1"

if not exist "%DIST_DIR%" mkdir "%DIST_DIR%" || exit /b 1
copy /y "%SCRIPT_DIR%..\inno\make-installers.cmd" "%DIST_DIR%\make-installers.cmd" >nul || exit /b 1
call "%SCRIPT_DIR%..\inno\bundle-inno.cmd" "%DIST_DIR%\tools\inno" || exit /b 1

> "%DIST_DIR%\README.txt" (
  echo LSYL Tunnel distributable directory
  echo.
  echo This directory is self-contained for implementation delivery.
  echo.
  echo Packages:
  echo   LSYL Tunnel Client\
  echo   LSYL Tunnel Lightweight Clients\
  echo   LSYL Tunnel Server\
  echo   LSYL Tunnel Profile Tool\
  echo.
  echo Lightweight Clients contains the Android APK and the Win7 32-bit Lite client for direct handoff.
  echo.
  echo Tools:
  echo   tools\inno\              Bundled Inno Setup compiler when available on the build machine
  echo.
  echo To build installers on an implementation machine:
  echo   make-installers.cmd
  echo.
  echo The scripts first use tools\inno\ISCC.exe when present. If it is not bundled,
  echo install Inno Setup 6 or set INNO_SETUP_ISCC to ISCC.exe.
  echo.
  echo Or build only one side:
  echo   "LSYL Tunnel Client\make-installer.cmd"
  echo   "LSYL Tunnel Server\make-installer.cmd"
  echo.
  echo Generated installers are written to:
  echo   installers\
  echo.
  echo Edit package-local conf and cert files before running make-installer.cmd when a one-off package is needed.
)

echo Dist helper scripts created:
echo   %DIST_DIR%\make-installers.cmd
echo   %DIST_DIR%\README.txt
if exist "%DIST_DIR%\tools\inno\ISCC.exe" echo   %DIST_DIR%\tools\inno\ISCC.exe
endlocal
