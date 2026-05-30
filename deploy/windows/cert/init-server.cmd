@echo off
setlocal
set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..\..\..") do set "WORKSPACE=%%~fI"
cd /d "%WORKSPACE%"
if not exist ".\build\bin\server\lsyl-tunnel-cert.exe" call "%SCRIPT_DIR%..\build.cmd" server || exit /b 1
set "HOSTS=%~1"
if "%HOSTS%"=="" set "HOSTS=localhost,127.0.0.1"
".\build\bin\server\lsyl-tunnel-cert.exe" -out certs -hosts "%HOSTS%"
endlocal
