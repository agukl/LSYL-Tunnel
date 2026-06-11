@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..\..\..") do set "WORKSPACE=%%~fI"
set "PACKAGE_DIR=%WORKSPACE%\dist\LSYL Tunnel Lightweight Clients"
if not "%~1"=="" set "PACKAGE_DIR=%~1"

cd /d "%WORKSPACE%" || exit /b 1

set "MOBILE_APK_SOURCE=%MOBILE_APK%"
if "%MOBILE_APK_SOURCE%"=="" if exist ".\mobile\android\app\build\outputs\apk\release\app-release.apk" set "MOBILE_APK_SOURCE=.\mobile\android\app\build\outputs\apk\release\app-release.apk"
if "%MOBILE_APK_SOURCE%"=="" if exist ".\mobile\android\app\build\outputs\apk\debug\app-debug.apk" set "MOBILE_APK_SOURCE=.\mobile\android\app\build\outputs\apk\debug\app-debug.apk"

echo [INFO] Lightweight client package inputs:
echo   Android APK: %MOBILE_APK_SOURCE%
echo   Win7 Lite:   build\bin\client\lsyl-tunnel-client-lite.exe
echo [INFO] Lightweight client package directory will be recreated:
echo   %PACKAGE_DIR%

if "%MOBILE_APK_SOURCE%"=="" (
  echo [ERROR] Missing Android APK.
  echo Build the Android app first, or set MOBILE_APK to an APK path.
  echo Expected one of:
  echo   mobile\android\app\build\outputs\apk\release\app-release.apk
  echo   mobile\android\app\build\outputs\apk\debug\app-debug.apk
  exit /b 1
)
if not exist "%MOBILE_APK_SOURCE%" (
  echo [ERROR] Android APK not found:
  echo   %MOBILE_APK_SOURCE%
  exit /b 1
)
if not exist ".\build\bin\client\lsyl-tunnel-client-lite.exe" (
  echo [ERROR] Missing Win7 Lite client:
  echo   build\bin\client\lsyl-tunnel-client-lite.exe
  echo Run deploy\windows\build.cmd win7-lite first.
  exit /b 1
)

if exist "%PACKAGE_DIR%" (
  rmdir /s /q "%PACKAGE_DIR%" 2>nul
  if exist "%PACKAGE_DIR%" (
    echo [WARN] Cannot remove package directory completely; refreshing files in place:
    echo   %PACKAGE_DIR%
  )
)

if not exist "%PACKAGE_DIR%" mkdir "%PACKAGE_DIR%" || exit /b 1
if not exist "%PACKAGE_DIR%\android" mkdir "%PACKAGE_DIR%\android" || exit /b 1
if not exist "%PACKAGE_DIR%\windows-win7" mkdir "%PACKAGE_DIR%\windows-win7" || exit /b 1
if not exist "%PACKAGE_DIR%\profiles" mkdir "%PACKAGE_DIR%\profiles" || exit /b 1

copy /y "%MOBILE_APK_SOURCE%" "%PACKAGE_DIR%\android\lsyl-tunnel-mobile.apk" >nul || exit /b 1
copy /y ".\build\bin\client\lsyl-tunnel-client-lite.exe" "%PACKAGE_DIR%\windows-win7\lsyl-tunnel-client-lite.exe" >nul || exit /b 1

> "%PACKAGE_DIR%\profiles\README.txt" (
  echo Put exported .lsylprofile files here when handing a user-specific package to an implementation site.
  echo.
  echo The same .lsylprofile can be imported by the Android app and the Win7 Lite client.
)

> "%PACKAGE_DIR%\README.txt" (
  echo LSYL Tunnel Lightweight Clients package
  echo.
  echo Contents:
  echo   android\lsyl-tunnel-mobile.apk
  echo     Android mobile client. Requires Android 10 / API 29 or newer.
  echo.
  echo   windows-win7\lsyl-tunnel-client-lite.exe
  echo     Win7-friendly 32-bit lightweight client. No external runtime is required on the target PC.
  echo.
  echo   profiles\
  echo     Optional handoff directory for exported .lsylprofile files.
  echo.
  echo Import flow:
  echo   1. Use the Windows client GUI after login to export a .lsylprofile.
  echo   2. Copy the .lsylprofile to the phone or Win7 machine.
  echo   3. Import the same .lsylprofile in either client, then connect.
)

echo Lightweight clients package created:
echo   %PACKAGE_DIR%
endlocal
