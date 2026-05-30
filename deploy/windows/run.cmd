@echo off
setlocal
set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..\..") do set "WORKSPACE=%%~fI"
set "TARGET=%~1"
if "%TARGET%"=="" goto usage

cd /d "%WORKSPACE%" || exit /b 1

if /i "%TARGET%"=="client-gui" goto run_client_gui
if /i "%TARGET%"=="client-lite" goto run_client_lite
if /i "%TARGET%"=="client" goto run_client
if /i "%TARGET%"=="server-gui" goto run_server_gui
if /i "%TARGET%"=="server" goto run_server

echo [ERROR] Unknown run target: %TARGET%
goto usage

:run_client_gui
if not exist ".\build\bin\client\lsyl-tunnel-client-gui.exe" call "%SCRIPT_DIR%build.cmd" client || exit /b 1
start "LSYL Tunnel Client" ".\build\bin\client\lsyl-tunnel-client-gui.exe"
echo lsyl-tunnel-client-gui started
goto :eof

:run_client_lite
if not exist ".\build\bin\client\lsyl-tunnel-client-lite.exe" call "%SCRIPT_DIR%build.cmd" client || exit /b 1
start "LSYL Tunnel Lite" ".\build\bin\client\lsyl-tunnel-client-lite.exe"
echo lsyl-tunnel-client-lite started
goto :eof

:run_client
if not exist ".\build\bin\client\lsyl-tunnel-client.exe" call "%SCRIPT_DIR%build.cmd" client || exit /b 1
if not exist ".\src\client\conf\client.yaml" (
  echo [ERROR] Missing src\client\conf\client.yaml.
  exit /b 1
)
".\build\bin\client\lsyl-tunnel-client.exe" -config ".\src\client\conf\client.yaml"
goto :eof

:run_server_gui
if not exist ".\build\bin\server\lsyl-tunnel-server-gui.exe" call "%SCRIPT_DIR%build.cmd" server || exit /b 1
start "LSYL Tunnel Server" ".\build\bin\server\lsyl-tunnel-server-gui.exe"
echo lsyl-tunnel-server-gui started
goto :eof

:run_server
if not exist ".\build\bin\server\lsyl-tunnel-server.exe" call "%SCRIPT_DIR%build.cmd" server || exit /b 1
if not exist ".\src\server\conf\server.yaml" (
  echo [ERROR] Missing src\server\conf\server.yaml.
  exit /b 1
)
if not exist ".\certs\server.crt" (
  echo [ERROR] Missing certs\server.crt. Run: deploy\windows\cert\init-server.cmd
  exit /b 1
)
if not exist ".\certs\server.key" (
  echo [ERROR] Missing certs\server.key. Run: deploy\windows\cert\init-server.cmd
  exit /b 1
)
".\build\bin\server\lsyl-tunnel-server.exe" -config ".\src\server\conf\server.yaml"
goto :eof

:usage
echo Usage: deploy\windows\run.cmd [server^|server-gui^|client^|client-gui^|client-lite]
exit /b 1
