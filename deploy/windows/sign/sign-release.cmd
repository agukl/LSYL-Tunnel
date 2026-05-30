@echo off
setlocal
set "SCRIPT_DIR=%~dp0"
set "SCOPE=%~1"
if "%SCOPE%"=="" set "SCOPE=all"
powershell -NoProfile -ExecutionPolicy Bypass -File "%SCRIPT_DIR%sign-release.ps1" -Scope "%SCOPE%"
exit /b %ERRORLEVEL%
