@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..\..\..") do set "WORKSPACE=%%~fI"
set "PACKAGE_DIR=%WORKSPACE%\dist\LSYL Tunnel Client"
if not "%~1"=="" set "PACKAGE_DIR=%~1"
cd /d "%WORKSPACE%"
echo [INFO] Client package inputs:
echo   config: src\client\conf\client.yaml
echo   certs:  src\client\cert\*
echo [INFO] Client package directory will be recreated:
echo   %PACKAGE_DIR%
call "%SCRIPT_DIR%..\build.cmd" client || exit /b 1
if not exist ".\src\client\cert\server.crt" (
  echo [ERROR] Missing src\client\cert\server.crt. Put the server public certificate there before packaging.
  exit /b 1
)
if exist "%PACKAGE_DIR%" (
  rmdir /s /q "%PACKAGE_DIR%"
  if exist "%PACKAGE_DIR%" (
    echo [ERROR] Cannot clean package directory:
    echo   %PACKAGE_DIR%
    echo Close any running client from this directory, or choose another package path.
    exit /b 1
  )
)
mkdir "%PACKAGE_DIR%\bin" || exit /b 1
mkdir "%PACKAGE_DIR%\conf" || exit /b 1
mkdir "%PACKAGE_DIR%\assets" || exit /b 1
mkdir "%PACKAGE_DIR%\cert" || exit /b 1
mkdir "%PACKAGE_DIR%\secrets" || exit /b 1
mkdir "%PACKAGE_DIR%\tmp\gui" || exit /b 1
mkdir "%PACKAGE_DIR%\installer\Languages" || exit /b 1
copy /y ".\build\bin\client\lsyl-tunnel-client-gui.exe" "%PACKAGE_DIR%\bin\" >nul || exit /b 1
copy /y ".\build\bin\client\lsyl-tunnel-client-lite.exe" "%PACKAGE_DIR%\bin\" >nul || exit /b 1
copy /y ".\src\client\conf\client.yaml" "%PACKAGE_DIR%\conf\client.yaml" >nul || exit /b 1
copy /y ".\src\client\assets\client.ico" "%PACKAGE_DIR%\assets\client.ico" >nul || exit /b 1
copy /y ".\src\client\assets\client-connected.ico" "%PACKAGE_DIR%\assets\client-connected.ico" >nul || exit /b 1
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$src=Join-Path '%WORKSPACE%' 'src\client\cert';" ^
  "$dst='%PACKAGE_DIR%\cert';" ^
  "if(Test-Path $src){ Copy-Item -Path (Join-Path $src '*') -Destination $dst -Recurse -Force -ErrorAction SilentlyContinue }"
if not exist "%PACKAGE_DIR%\cert\server.crt" (
  echo [ERROR] Failed to package cert\server.crt.
  exit /b 1
)
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$p='%PACKAGE_DIR%\conf\client.yaml';" ^
  "$t=[IO.File]::ReadAllText($p);" ^
  "$t=$t.Replace('../../certs/server.crt','../cert/server.crt').Replace('..\..\certs\server.crt','..\cert\server.crt').Replace('../certs/server.crt','../cert/server.crt').Replace('..\certs\server.crt','..\cert\server.crt').Replace('../../secrets/client-password.txt','../secrets/client-password.txt').Replace('..\..\secrets\client-password.txt','..\secrets\client-password.txt');" ^
  "$t=[regex]::Replace($t,'(?m)^password:\s*.*$','password: \"\"');" ^
  "$t=[regex]::Replace($t,'(?m)^saved_credential:\s*\r?\n(?:[ \t]+[^\r\n]*(?:\r?\n|$))*','saved_credential: {}'+[Environment]::NewLine);" ^
  "$t=[regex]::Replace($t,'(?m)^saved_credential:\s*.*$','saved_credential: {}');" ^
  "$t=[regex]::Replace($t,'(?m)^client_id:\s*.*$','client_id: \"\"');" ^
  "[IO.File]::WriteAllText($p,$t,[Text.UTF8Encoding]::new($false))" || exit /b 1
copy /y "%SCRIPT_DIR%..\inno\make-client-installer.cmd" "%PACKAGE_DIR%\make-installer.cmd" >nul || exit /b 1
copy /y "%SCRIPT_DIR%..\inno\package-client.iss" "%PACKAGE_DIR%\installer\client.iss" >nul || exit /b 1
copy /y "%SCRIPT_DIR%..\inno\Languages\ChineseSimplified.isl" "%PACKAGE_DIR%\installer\Languages\ChineseSimplified.isl" >nul || exit /b 1
> "%PACKAGE_DIR%\README.txt" (
  echo LSYL Tunnel Client package
  echo.
  echo Build installer from this package:
  echo   make-installer.cmd
  echo.
  echo The package builder first uses ..\tools\inno\ISCC.exe when dist includes it.
  echo Otherwise install Inno Setup 6 or set INNO_SETUP_ISCC.
  echo.
  echo Generated installer:
  echo   ..\installers\LSYL-Tunnel-Client-Setup.exe
  echo.
  echo Default install path:
  echo   C:\Program Files\LSYL Tunnel Client
  echo.
  echo Build the installer, run it as Administrator, then start from Desktop or Start Menu.
  echo Package includes files from src\client\cert as cert\*.
  echo Client runs the tunnel engine inside the GUI and guards it from the tray by default.
  echo Win7 Lite client is bin\lsyl-tunnel-client-lite.exe, built with Go 1.20 for windows/386.
  echo Import a .lsylprofile, then connect or disconnect from the window.
  echo No extra client process or Windows client service is registered.
)
echo Client app package created from source config inputs:
echo   %PACKAGE_DIR%
echo Edit files under this package, then run make-installer.cmd for one-off installer customization.
endlocal
