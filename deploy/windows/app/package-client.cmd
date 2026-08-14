@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..\..\..") do set "WORKSPACE=%%~fI"
set "PACKAGE_DIR=%WORKSPACE%\build\tmp\dist-work\LSYL Tunnel Client"
if not "%~1"=="" set "PACKAGE_DIR=%~1"
cd /d "%WORKSPACE%"
echo [INFO] Client package inputs:
echo   config: src\client\conf\client.yaml
echo   certs:  src\client\cert\*
echo   virtual redirect: third_party\windivert\2.2.2\x64
echo [INFO] Client package directory will be recreated:
echo   %PACKAGE_DIR%
if exist "%PACKAGE_DIR%" (
  rmdir /s /q "%PACKAGE_DIR%" 2>nul
  if exist "%PACKAGE_DIR%" (
    echo [ERROR] Cannot remove old client package directory:
    echo   %PACKAGE_DIR%
    exit /b 1
  )
)
call "%SCRIPT_DIR%..\build.cmd" client-package || exit /b 1
if not exist ".\src\client\cert\server.crt" (
  echo [ERROR] Missing src\client\cert\server.crt. Put the server public certificate there before packaging.
  exit /b 1
)
if not exist ".\third_party\windivert\2.2.2\x64\WinDivert.dll" (
  echo [ERROR] Missing third_party\windivert\2.2.2\x64\WinDivert.dll.
  exit /b 1
)
if not exist ".\third_party\windivert\2.2.2\x64\WinDivert64.sys" (
  echo [ERROR] Missing third_party\windivert\2.2.2\x64\WinDivert64.sys.
  exit /b 1
)
if not exist "%PACKAGE_DIR%" mkdir "%PACKAGE_DIR%" || exit /b 1
if not exist "%PACKAGE_DIR%\bin" mkdir "%PACKAGE_DIR%\bin" || exit /b 1
if not exist "%PACKAGE_DIR%\conf" mkdir "%PACKAGE_DIR%\conf" || exit /b 1
if not exist "%PACKAGE_DIR%\assets" mkdir "%PACKAGE_DIR%\assets" || exit /b 1
if not exist "%PACKAGE_DIR%\cert" mkdir "%PACKAGE_DIR%\cert" || exit /b 1
if not exist "%PACKAGE_DIR%\secrets" mkdir "%PACKAGE_DIR%\secrets" || exit /b 1
if not exist "%PACKAGE_DIR%\tmp\gui" mkdir "%PACKAGE_DIR%\tmp\gui" || exit /b 1
if not exist "%PACKAGE_DIR%\installer\Languages" mkdir "%PACKAGE_DIR%\installer\Languages" || exit /b 1
if not exist "%PACKAGE_DIR%\licenses\WinDivert\source" mkdir "%PACKAGE_DIR%\licenses\WinDivert\source" || exit /b 1
copy /y ".\build\bin\client\lsyl-tunnel-client-gui.exe" "%PACKAGE_DIR%\bin\" >nul || exit /b 1
copy /y ".\build\bin\client\lsyl-tunnel-client-lite.exe" "%PACKAGE_DIR%\bin\" >nul || exit /b 1
copy /y ".\third_party\windivert\2.2.2\x64\WinDivert.dll" "%PACKAGE_DIR%\bin\WinDivert.dll" >nul || exit /b 1
copy /y ".\third_party\windivert\2.2.2\x64\WinDivert64.sys" "%PACKAGE_DIR%\bin\WinDivert64.sys" >nul || exit /b 1
copy /y ".\third_party\windivert\2.2.2\LICENSE" "%PACKAGE_DIR%\licenses\WinDivert\LICENSE" >nul || exit /b 1
copy /y ".\third_party\windivert\2.2.2\README.md" "%PACKAGE_DIR%\licenses\WinDivert\README.md" >nul || exit /b 1
copy /y ".\third_party\windivert\2.2.2\source\WinDivert-2.2.2-source.zip" "%PACKAGE_DIR%\licenses\WinDivert\source\WinDivert-2.2.2-source.zip" >nul || exit /b 1
copy /y ".\src\client\conf\client.yaml" "%PACKAGE_DIR%\conf\client.yaml" >nul || exit /b 1
copy /y ".\src\client\assets\client.ico" "%PACKAGE_DIR%\assets\client.ico" >nul || exit /b 1
copy /y ".\src\client\assets\client-connected.ico" "%PACKAGE_DIR%\assets\client-connected.ico" >nul || exit /b 1
copy /y "%SCRIPT_DIR%..\inno\clear-client-id.ps1" "%PACKAGE_DIR%\clear-client-id.ps1" >nul || exit /b 1
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
  "[IO.File]::WriteAllText($p,$t,[Text.UTF8Encoding]::new($false))" || exit /b 1
powershell -NoProfile -ExecutionPolicy Bypass -File "%PACKAGE_DIR%\clear-client-id.ps1" -ConfigFile "%PACKAGE_DIR%\conf\client.yaml" || exit /b 1
copy /y "%SCRIPT_DIR%..\inno\make-client-installer.cmd" "%PACKAGE_DIR%\make-installer.cmd" >nul || exit /b 1
copy /y "%SCRIPT_DIR%..\inno\verify-installer-version.ps1" "%PACKAGE_DIR%\verify-installer-version.ps1" >nul || exit /b 1
copy /y "%SCRIPT_DIR%..\inno\package-client.iss" "%PACKAGE_DIR%\installer\client.iss" >nul || exit /b 1
copy /y "%SCRIPT_DIR%..\inno\Languages\ChineseSimplified.isl" "%PACKAGE_DIR%\installer\Languages\ChineseSimplified.isl" >nul || exit /b 1
> "%PACKAGE_DIR%\README.txt" (
  echo LSYL Tunnel Client package
  echo.
  echo Build installer from this package:
  echo   make-installer.cmd
  echo.
  echo The package builder first uses PATH, then project tool\inno, then dist tools when present.
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
  echo Standard 64-bit virtual forwarding includes WinDivert 2.2.2 runtime and license materials.
  echo Win7 Lite client is bin\lsyl-tunnel-client-lite.exe, built with Go 1.20 for windows/386.
  echo Import a .lsylprofile, then connect or disconnect from the window.
  echo No extra client process or Windows client service is registered.
)
echo Client package created from source config inputs:
echo   %PACKAGE_DIR%
echo release.cmd copies this package into dist by default unless /no-client-kit is used.
endlocal
