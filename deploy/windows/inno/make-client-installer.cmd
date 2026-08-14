@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "PACKAGE_DIR=%~dp0"
set "SCRIPT_FILE=%PACKAGE_DIR%installer\client.iss"
set "OUT_DIR=%PACKAGE_DIR%..\installers"
set "DIST_DIR=%PACKAGE_DIR%.."
if not "%~1"=="" set "OUT_DIR=%~1"
for %%I in ("%OUT_DIR%") do set "OUT_DIR=%%~fI"
for %%I in ("%DIST_DIR%") do set "DIST_DIR=%%~fI"
set "PROJECT_TOOL_DIR="
call :find_project_tool_dir
set "OUT_FILE=%OUT_DIR%\LSYL-Tunnel-Client-Setup.exe"

if exist "%OUT_FILE%" (
  del /f /q "%OUT_FILE%" >nul 2>nul
  if exist "%OUT_FILE%" (
    echo [ERROR] Cannot remove old client installer:
    echo   %OUT_FILE%
    exit /b 1
  )
)

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
if not exist "%PACKAGE_DIR%clear-client-id.ps1" (
  echo [ERROR] Missing client identity sanitizer:
  echo   %PACKAGE_DIR%clear-client-id.ps1
  exit /b 1
)
if not exist "%PACKAGE_DIR%cert\server.crt" (
  echo [ERROR] Missing client trust certificate:
  echo   %PACKAGE_DIR%cert\server.crt
  exit /b 1
)

powershell -NoProfile -ExecutionPolicy Bypass -File "%PACKAGE_DIR%clear-client-id.ps1" -ConfigFile "%PACKAGE_DIR%conf\client.yaml" || exit /b 1
call :resolve_iscc || exit /b 1
goto found_iscc

:missing_iscc
echo [ERROR] Inno Setup Compiler not found.
echo Add ISCC.exe to PATH, place it under tool\inno, add dist\tools\inno, install Inno Setup 6, or set INNO_SETUP_ISCC.
exit /b 1

:find_project_tool_dir
for %%I in ("%PACKAGE_DIR%..\tool" "%PACKAGE_DIR%..\..\tool" "%PACKAGE_DIR%..\..\..\tool" "%PACKAGE_DIR%..\..\..\..\tool" "%PACKAGE_DIR%..\..\..\..\..\tool") do (
  if not defined PROJECT_TOOL_DIR if exist "%%~fI\" set "PROJECT_TOOL_DIR=%%~fI"
)
exit /b 0

:resolve_iscc
if defined INNO_SETUP_ISCC (
  if exist "%INNO_SETUP_ISCC%" (
    set "ISCC=%INNO_SETUP_ISCC%"
    exit /b 0
  )
  echo [ERROR] INNO_SETUP_ISCC points to a missing file:
  echo   %INNO_SETUP_ISCC%
  exit /b 1
)
for /f "delims=" %%I in ('where iscc.exe 2^>nul') do (
  set "ISCC=%%I"
  exit /b 0
)
if defined PROJECT_TOOL_DIR if exist "%PROJECT_TOOL_DIR%\inno\ISCC.exe" (
  set "ISCC=%PROJECT_TOOL_DIR%\inno\ISCC.exe"
  exit /b 0
)
if defined PROJECT_TOOL_DIR if exist "%PROJECT_TOOL_DIR%\Inno Setup 6\ISCC.exe" (
  set "ISCC=%PROJECT_TOOL_DIR%\Inno Setup 6\ISCC.exe"
  exit /b 0
)
if exist "%DIST_DIR%\tools\inno\ISCC.exe" (
  set "ISCC=%DIST_DIR%\tools\inno\ISCC.exe"
  exit /b 0
)
if exist "%LocalAppData%\Programs\Inno Setup 6\ISCC.exe" (
  set "ISCC=%LocalAppData%\Programs\Inno Setup 6\ISCC.exe"
  exit /b 0
)
if exist "%ProgramFiles(x86)%\Inno Setup 6\ISCC.exe" (
  set "ISCC=%ProgramFiles(x86)%\Inno Setup 6\ISCC.exe"
  exit /b 0
)
if exist "%ProgramFiles%\Inno Setup 6\ISCC.exe" (
  set "ISCC=%ProgramFiles%\Inno Setup 6\ISCC.exe"
  exit /b 0
)
goto missing_iscc

:found_iscc
if not exist "%OUT_DIR%" mkdir "%OUT_DIR%" || exit /b 1
echo [INFO] Building client installer from package:
echo   %PACKAGE_DIR%
echo [INFO] Inno compiler:
echo   %ISCC%
"%ISCC%" "/O%OUT_DIR%" "%SCRIPT_FILE%" || exit /b 1
if not exist "%OUT_FILE%" (
  echo [ERROR] Client installer was not created:
  echo   %OUT_FILE%
  exit /b 1
)
powershell -NoProfile -ExecutionPolicy Bypass -File "%PACKAGE_DIR%verify-installer-version.ps1" -ScriptFile "%SCRIPT_FILE%" -InstallerFile "%OUT_FILE%"
if errorlevel 1 exit /b 1
echo Client installer created:
echo   %OUT_FILE%
endlocal
