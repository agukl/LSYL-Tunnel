@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "PACKAGE_DIR=%~dp0"
set "SCRIPT_FILE=%PACKAGE_DIR%installer\client.iss"
set "OUT_DIR=%PACKAGE_DIR%..\installers"
set "DIST_DIR=%PACKAGE_DIR%.."
for %%I in ("%OUT_DIR%") do set "OUT_DIR=%%~fI"
for %%I in ("%DIST_DIR%") do set "DIST_DIR=%%~fI"

if not exist "%SCRIPT_FILE%" (
  echo [ERROR] Missing installer script:
  echo   %SCRIPT_FILE%
  exit /b 1
)
if not exist "%PACKAGE_DIR%bin\lsyl-tunnel-client-gui.exe" (
  echo [ERROR] Missing client GUI:
  echo   %PACKAGE_DIR%bin\lsyl-tunnel-client-gui.exe
  exit /b 1
)
if not exist "%PACKAGE_DIR%bin\lsyl-tunnel-client-lite.exe" (
  echo [ERROR] Missing Win7 Lite client:
  echo   %PACKAGE_DIR%bin\lsyl-tunnel-client-lite.exe
  exit /b 1
)
if not exist "%PACKAGE_DIR%conf\client.yaml" (
  echo [ERROR] Missing client config:
  echo   %PACKAGE_DIR%conf\client.yaml
  exit /b 1
)
if not exist "%PACKAGE_DIR%cert\server.crt" (
  echo [ERROR] Missing client trust certificate:
  echo   %PACKAGE_DIR%cert\server.crt
  exit /b 1
)

set "ISCC=%INNO_SETUP_ISCC%"
if not defined ISCC if exist "%DIST_DIR%\tools\inno\ISCC.exe" set "ISCC=%DIST_DIR%\tools\inno\ISCC.exe"
if not defined ISCC if exist "%LocalAppData%\Programs\Inno Setup 6\ISCC.exe" set "ISCC=%LocalAppData%\Programs\Inno Setup 6\ISCC.exe"
if not defined ISCC if exist "%ProgramFiles(x86)%\Inno Setup 6\ISCC.exe" set "ISCC=%ProgramFiles(x86)%\Inno Setup 6\ISCC.exe"
if not defined ISCC if exist "%ProgramFiles%\Inno Setup 6\ISCC.exe" set "ISCC=%ProgramFiles%\Inno Setup 6\ISCC.exe"
if not defined ISCC set "ISCC=iscc.exe"
echo "%ISCC%" | findstr /c:"\\" >nul
if not errorlevel 1 (
  if not exist "%ISCC%" goto missing_iscc
) else (
  where "%ISCC%" >nul 2>nul
  if errorlevel 1 goto missing_iscc
)
goto found_iscc

:missing_iscc
echo [ERROR] Inno Setup Compiler not found.
echo Add dist\tools\inno\ISCC.exe, install Inno Setup 6, add ISCC.exe to PATH, or set INNO_SETUP_ISCC.
exit /b 1

:found_iscc
if not exist "%OUT_DIR%" mkdir "%OUT_DIR%" || exit /b 1
echo [INFO] Building client installer from package:
echo   %PACKAGE_DIR%
echo [INFO] Inno compiler:
echo   %ISCC%
"%ISCC%" "%SCRIPT_FILE%" || exit /b 1
echo Client installer created:
echo   %OUT_DIR%\LSYL-Tunnel-Client-Setup.exe
endlocal
