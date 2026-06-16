@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..\..") do set "WORKSPACE=%%~fI"
if "%GOEXE%"=="" set "GOEXE=go"
set "TARGET=%~1"
if "%TARGET%"=="" set "TARGET=all"
if "%LSYL_RELEASE_PROTECT%"=="" set "LSYL_RELEASE_PROTECT=0"
if "%LSYL_GARBLE_VERSION%"=="" set "LSYL_GARBLE_VERSION=v0.15.0"

if /i "%TARGET%"=="win7-lite" (
  call "%SCRIPT_DIR%build-win7-lite.cmd"
  exit /b %ERRORLEVEL%
)

"%GOEXE%" version >nul 2>nul
if errorlevel 1 (
  echo [ERROR] Go executable not found. Install Go and add it to PATH, or set GOEXE to the full go.exe path.
  exit /b 1
)

cd /d "%WORKSPACE%" || exit /b 1

if /i "%LSYL_RELEASE_PROTECT%"=="1" (
  call :resolve_garble || exit /b 1
  echo [INFO] Protected release build enabled: garble %LSYL_GARBLE_VERSION% + -trimpath + -s -w
)

if /i "%TARGET%"=="all" goto build_all
if /i "%TARGET%"=="client" goto build_client
if /i "%TARGET%"=="win7-lite" goto build_win7_lite
if /i "%TARGET%"=="server" goto build_server
if /i "%TARGET%"=="profile" goto build_profile

echo [ERROR] Unknown build target: %TARGET%
goto usage

:build_all
call :build_server || exit /b 1
call :build_client || exit /b 1
call :build_profile || exit /b 1
echo Build completed.
goto :eof

:build_client
if not exist ".\build\bin\client" mkdir ".\build\bin\client"

echo [1/3] build lsyl-tunnel-client.exe
call :build_go ".\build\bin\client\lsyl-tunnel-client.exe" ".\src\client\cmd\lsyl-tunnel-client" "" || exit /b 1

echo [2/3] build lsyl-tunnel-client-gui.exe
call :build_go ".\build\bin\client\lsyl-tunnel-client-gui.exe" ".\src\client\cmd\lsyl-tunnel-client-gui" "-H windowsgui" || exit /b 1

echo [3/3] build lsyl-tunnel-client-lite.exe
call "%SCRIPT_DIR%build-win7-lite.cmd" || exit /b 1

echo Client build completed: %WORKSPACE%\build\bin\client
goto :eof

:build_win7_lite
call "%SCRIPT_DIR%build-win7-lite.cmd" || exit /b 1
goto :eof

:build_server
if not exist ".\build\bin\server" mkdir ".\build\bin\server"

echo [1/5] build lsyl-tunnel-server.exe
call :build_go ".\build\bin\server\lsyl-tunnel-server.exe" ".\src\server\cmd\lsyl-tunnel-server" "" || exit /b 1

echo [2/5] build lsyl-tunnel-server-svc.exe
call :build_go ".\build\bin\server\lsyl-tunnel-server-svc.exe" ".\src\server\cmd\lsyl-tunnel-server-svc" "" || exit /b 1

echo [3/5] build lsyl-tunnel-server-gui.exe
call :build_go ".\build\bin\server\lsyl-tunnel-server-gui.exe" ".\src\server\cmd\lsyl-tunnel-server-gui" "-H windowsgui" || exit /b 1

echo [4/5] build lsyl-tunnel-passwd.exe
call :build_go ".\build\bin\server\lsyl-tunnel-passwd.exe" ".\src\cmd\lsyl-tunnel-passwd" "" || exit /b 1

echo [5/5] build lsyl-tunnel-cert.exe
call :build_go ".\build\bin\server\lsyl-tunnel-cert.exe" ".\src\cmd\lsyl-tunnel-cert" "" || exit /b 1

echo Server build completed: %WORKSPACE%\build\bin\server
goto :eof

:build_profile
if not exist ".\build\bin\profile" mkdir ".\build\bin\profile"

echo [1/1] build lsyl-tunnel-profile.exe
call :build_go ".\build\bin\profile\lsyl-tunnel-profile.exe" ".\src\cmd\lsyl-tunnel-profile" "" || exit /b 1

echo Profile tool build completed: %WORKSPACE%\build\bin\profile
goto :eof

:build_go
set "BUILD_OUT=%~1"
set "BUILD_PKG=%~2"
set "BUILD_LDFLAGS=%~3"
if /i "%LSYL_RELEASE_PROTECT%"=="1" goto build_go_protected
if "%BUILD_LDFLAGS%"=="" (
  "%GOEXE%" build -trimpath -o "%BUILD_OUT%" "%BUILD_PKG%"
) else (
  "%GOEXE%" build -trimpath -ldflags "%BUILD_LDFLAGS%" -o "%BUILD_OUT%" "%BUILD_PKG%"
)
exit /b %ERRORLEVEL%

:build_go_protected
if "%BUILD_LDFLAGS%"=="" (
  "%GARBLEEXE%" -literals -tiny build -trimpath -ldflags "-s -w" -o "%BUILD_OUT%" "%BUILD_PKG%"
) else (
  "%GARBLEEXE%" -literals -tiny build -trimpath -ldflags "%BUILD_LDFLAGS% -s -w" -o "%BUILD_OUT%" "%BUILD_PKG%"
)
exit /b %ERRORLEVEL%

:resolve_garble
if not "%GARBLEEXE%"=="" goto verify_garble
set "GARBLE_TOOL_DIR=%WORKSPACE%\build\tools\bin"
if exist "%GARBLE_TOOL_DIR%\garble.exe" (
  set "GARBLEEXE=%GARBLE_TOOL_DIR%\garble.exe"
  goto verify_garble
)
for /f "delims=" %%P in ('where garble 2^>nul') do (
  set "GARBLEEXE=%%P"
  goto verify_garble
)
if /i "%LSYL_NO_GARBLE_INSTALL%"=="1" (
  echo [ERROR] garble.exe not found. Install garble, set GARBLEEXE, or unset LSYL_NO_GARBLE_INSTALL.
  exit /b 1
)
echo [INFO] garble.exe not found. Installing mvdan.cc/garble@%LSYL_GARBLE_VERSION% to build\tools\bin...
if not exist "%GARBLE_TOOL_DIR%" mkdir "%GARBLE_TOOL_DIR%" || exit /b 1
if "%GOPROXY%"=="" set "GOPROXY=https://goproxy.cn,direct"
set "OLD_GOBIN=%GOBIN%"
set "GOBIN=%GARBLE_TOOL_DIR%"
"%GOEXE%" install mvdan.cc/garble@%LSYL_GARBLE_VERSION% || exit /b 1
set "GOBIN=%OLD_GOBIN%"
set "GARBLEEXE=%GARBLE_TOOL_DIR%\garble.exe"
goto verify_garble

:verify_garble
if "%GARBLEEXE%"=="" (
  echo [ERROR] GARBLEEXE is empty.
  exit /b 1
)
"%GARBLEEXE%" version >nul 2>nul
if errorlevel 1 (
  echo [ERROR] Cannot run garble: %GARBLEEXE%
  exit /b 1
)
goto :eof

:usage
echo Usage: deploy\windows\build.cmd [all^|server^|client^|win7-lite^|profile]
echo.
echo Set LSYL_RELEASE_PROTECT=1 to build package binaries with garble and -ldflags "-s -w".
echo The release.cmd entry enables this automatically for dist packages and installers.
exit /b 1
