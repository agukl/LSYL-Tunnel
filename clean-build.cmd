@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%.") do set "WORKSPACE=%%~fI"
cd /d "%WORKSPACE%" || exit /b 1

:parse_args
if "%~1"=="" goto run
if /i "%~1"=="/dry-run" (set "PS_ARGS=%PS_ARGS% -DryRun" & shift & goto parse_args)
if /i "%~1"=="--dry-run" (set "PS_ARGS=%PS_ARGS% -DryRun" & shift & goto parse_args)
if /i "%~1"=="-dry-run" (set "PS_ARGS=%PS_ARGS% -DryRun" & shift & goto parse_args)
if /i "%~1"=="/all" (set "PS_ARGS=%PS_ARGS% -All" & shift & goto parse_args)
if /i "%~1"=="--all" (set "PS_ARGS=%PS_ARGS% -All" & shift & goto parse_args)
if /i "%~1"=="-all" (set "PS_ARGS=%PS_ARGS% -All" & shift & goto parse_args)
if /i "%~1"=="/?" (set "PS_ARGS=%PS_ARGS% -Help" & shift & goto parse_args)
if /i "%~1"=="-?" (set "PS_ARGS=%PS_ARGS% -Help" & shift & goto parse_args)
if /i "%~1"=="--help" (set "PS_ARGS=%PS_ARGS% -Help" & shift & goto parse_args)
echo [ERROR] Unknown argument: %~1
echo Usage: clean-build.cmd [/dry-run] [/all]
exit /b 1

:run
powershell -NoProfile -ExecutionPolicy Bypass -File deploy\windows\clean-build.ps1 %PS_ARGS%
exit /b %ERRORLEVEL%
