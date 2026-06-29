@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..\..\..") do set "WORKSPACE=%%~fI"
set "TOOL_DIR=%WORKSPACE%\tool"
set "OUT_FILE=%WORKSPACE%\build\tmp\android\LSYL-Tunnel-Android.apk"
if not "%~1"=="" set "OUT_FILE=%~1"
for %%I in ("%OUT_FILE%") do set "OUT_FILE=%%~fI"

cd /d "%WORKSPACE%" || exit /b 1

set "MOBILE_APK_SOURCE=%MOBILE_APK%"
if not "%MOBILE_APK_SOURCE%"=="" goto use_explicit_apk

set "ANDROID_DIR=%WORKSPACE%\mobile\android"
set "ANDROID_OUTPUT_DIR=%ANDROID_DIR%\app\build\outputs\apk"
set "ANDROID_GRADLE=%GRADLE_EXE%"
if "%ANDROID_GRADLE%"=="" goto resolve_gradle
:gradle_resolved
set "ANDROID_TASK=%ANDROID_GRADLE_TASK%"
if "%ANDROID_TASK%"=="" set "ANDROID_TASK=:app:assembleDebug"
set "ANDROID_ARGS=%ANDROID_GRADLE_ARGS%"
if /i "%ANDROID_OFFLINE%"=="1" set "ANDROID_ARGS=%ANDROID_ARGS% --offline"

if not exist "%ANDROID_DIR%\settings.gradle.kts" (
  echo [ERROR] Missing Android project:
  echo   %ANDROID_DIR%
  exit /b 1
)

call :check_gradle || exit /b 1
call :clean_old_outputs || exit /b 1

echo [INFO] Build Android APK:
echo   %ANDROID_GRADLE% --no-daemon %ANDROID_ARGS% %ANDROID_TASK%
pushd "%ANDROID_DIR%" || exit /b 1
call "%ANDROID_GRADLE%" --no-daemon %ANDROID_ARGS% %ANDROID_TASK%
if errorlevel 1 (
  popd
  echo [ERROR] Android APK build failed.
  echo [ERROR] Fix Android SDK/Gradle dependencies, or set MOBILE_APK to an existing APK.
  exit /b 1
)
popd

if exist "%ANDROID_DIR%\app\build\outputs\apk\release\app-release.apk" set "MOBILE_APK_SOURCE=%ANDROID_DIR%\app\build\outputs\apk\release\app-release.apk"
if "%MOBILE_APK_SOURCE%"=="" if exist "%ANDROID_DIR%\app\build\outputs\apk\debug\app-debug.apk" set "MOBILE_APK_SOURCE=%ANDROID_DIR%\app\build\outputs\apk\debug\app-debug.apk"
if "%MOBILE_APK_SOURCE%"=="" if exist "%ANDROID_DIR%\app\build\outputs\apk\release\app-release-unsigned.apk" (
  echo [ERROR] Android release build only produced an unsigned APK:
  echo   %ANDROID_DIR%\app\build\outputs\apk\release\app-release-unsigned.apk
  echo Configure Android release signing, set ANDROID_GRADLE_TASK=:app:assembleDebug for internal testing, or set MOBILE_APK.
  exit /b 1
)
goto copy_selected_apk

:use_explicit_apk
for %%I in ("%MOBILE_APK_SOURCE%") do set "MOBILE_APK_SOURCE=%%~fI"
echo [INFO] Using explicit Android APK from MOBILE_APK:
echo   %MOBILE_APK_SOURCE%
goto copy_selected_apk

:copy_selected_apk
if "%MOBILE_APK_SOURCE%"=="" (
  echo [ERROR] Android APK was not created.
  echo [ERROR] Fix Android SDK/Gradle dependencies, or set MOBILE_APK to an existing APK.
  exit /b 1
)
if not exist "%MOBILE_APK_SOURCE%" (
  echo [ERROR] Android APK not found:
  echo   %MOBILE_APK_SOURCE%
  exit /b 1
)
for %%I in ("%OUT_FILE%") do set "OUT_DIR=%%~dpI"
if not exist "%OUT_DIR%" mkdir "%OUT_DIR%" || exit /b 1
copy /y "%MOBILE_APK_SOURCE%" "%OUT_FILE%" >nul || exit /b 1
echo Android APK ready:
echo   %OUT_FILE%
endlocal
exit /b 0

:check_gradle
if not exist "%ANDROID_GRADLE%" (
  echo [ERROR] Gradle executable not found:
  echo   %ANDROID_GRADLE%
  echo [ERROR] Set GRADLE_EXE to gradle.bat, add Gradle to PATH, place Gradle under tool\gradle-8.9, or set MOBILE_APK.
  exit /b 1
)
exit /b 0

:resolve_gradle
for /f "delims=" %%G in ('where gradle 2^>nul') do (
  set "ANDROID_GRADLE=%%G"
  goto gradle_resolved
)
if exist "%TOOL_DIR%\gradle-8.9\bin\gradle.bat" (
  set "ANDROID_GRADLE=%TOOL_DIR%\gradle-8.9\bin\gradle.bat"
  goto gradle_resolved
)
if exist "%TOOL_DIR%\gradle\bin\gradle.bat" (
  set "ANDROID_GRADLE=%TOOL_DIR%\gradle\bin\gradle.bat"
  goto gradle_resolved
)
if exist "%ANDROID_DIR%\gradlew.bat" (
  set "ANDROID_GRADLE=%ANDROID_DIR%\gradlew.bat"
  goto gradle_resolved
)
echo [ERROR] Gradle is required to build Android APK, but gradle was not found in PATH or project tool directory.
echo [ERROR] Install Gradle, set GRADLE_EXE, place Gradle under tool\gradle-8.9, or set MOBILE_APK.
exit /b 1

:clean_old_outputs
if not exist "%ANDROID_OUTPUT_DIR%" exit /b 0
rmdir /s /q "%ANDROID_OUTPUT_DIR%" 2>nul
if exist "%ANDROID_OUTPUT_DIR%" (
  echo [ERROR] Cannot remove old Android APK outputs:
  echo   %ANDROID_OUTPUT_DIR%
  exit /b 1
)
exit /b 0
