@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..\..\..") do set "WORKSPACE=%%~fI"
set "OUTPUT_DIR=%WORKSPACE%\dist\installers"
if not "%~1"=="" set "OUTPUT_DIR=%~1"

cd /d "%WORKSPACE%" || exit /b 1

set "MOBILE_APK_SOURCE=%MOBILE_APK%"
if "%MOBILE_APK_SOURCE%"=="" if exist ".\mobile\android\app\build\outputs\apk\release\app-release.apk" set "MOBILE_APK_SOURCE=.\mobile\android\app\build\outputs\apk\release\app-release.apk"
if "%MOBILE_APK_SOURCE%"=="" if exist ".\mobile\android\app\build\outputs\apk\debug\app-debug.apk" set "MOBILE_APK_SOURCE=.\mobile\android\app\build\outputs\apk\debug\app-debug.apk"

echo [INFO] Android installer package input:
echo   APK: %MOBILE_APK_SOURCE%
echo [INFO] Android installer output directory:
echo   %OUTPUT_DIR%

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

if not exist "%OUTPUT_DIR%" mkdir "%OUTPUT_DIR%" || exit /b 1
copy /y "%MOBILE_APK_SOURCE%" "%OUTPUT_DIR%\LSYL-Tunnel-Android.apk" >nul || exit /b 1

echo Android installer package copied:
echo   %OUTPUT_DIR%\LSYL-Tunnel-Android.apk
endlocal
