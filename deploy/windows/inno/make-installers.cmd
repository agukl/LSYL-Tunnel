@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "DIST_DIR=%~dp0"

if exist "%DIST_DIR%LSYL Tunnel Client\make-installer.cmd" (
  call "%DIST_DIR%LSYL Tunnel Client\make-installer.cmd" || exit /b 1
) else (
  echo [WARN] Client package not found:
  echo   %DIST_DIR%LSYL Tunnel Client
)

if exist "%DIST_DIR%LSYL Tunnel Server\make-installer.cmd" (
  call "%DIST_DIR%LSYL Tunnel Server\make-installer.cmd" || exit /b 1
) else (
  echo [WARN] Server package not found:
  echo   %DIST_DIR%LSYL Tunnel Server
)

if exist "%DIST_DIR%installers\LSYL-Tunnel-Profile-Tool-Setup.exe" (
  del /f /q "%DIST_DIR%installers\LSYL-Tunnel-Profile-Tool-Setup.exe" >nul 2>nul
)

echo.
echo Installers are under:
echo   %DIST_DIR%installers
endlocal
