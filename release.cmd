@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%.") do set "WORKSPACE=%%~fI"
if "%GOEXE%"=="" set "GOEXE=go"
cd /d "%WORKSPACE%" || exit /b 1

set "HOSTS="
set "LOCAL_SIGN=0"
set "SKIP_TEST=0"
set "CLIENT_KIT=1"
set "BUNDLE_INNO=0"
set "VERIFY_ONLY=0"
set "KEEP_WORK=0"
set "HELP_REQUESTED=0"

:parse_args
if "%~1"=="" goto parsed
if /i "%~1"=="/?" (set "HELP_REQUESTED=1" & goto usage)
if /i "%~1"=="-?" (set "HELP_REQUESTED=1" & goto usage)
if /i "%~1"=="--help" (set "HELP_REQUESTED=1" & goto usage)
if /i "%~1"=="/hosts" (
  if "%~2"=="" goto missing_hosts
  set "HOSTS=%~2"
  shift
  shift
  goto parse_args
)
if /i "%~1"=="--hosts" (
  if "%~2"=="" goto missing_hosts
  set "HOSTS=%~2"
  shift
  shift
  goto parse_args
)
if /i "%~1"=="/local-sign" (set "LOCAL_SIGN=1" & shift & goto parse_args)
if /i "%~1"=="--local-sign" (set "LOCAL_SIGN=1" & shift & goto parse_args)
if /i "%~1"=="/skip-test" (set "SKIP_TEST=1" & shift & goto parse_args)
if /i "%~1"=="--skip-test" (set "SKIP_TEST=1" & shift & goto parse_args)
if /i "%~1"=="/client-kit" (set "CLIENT_KIT=1" & shift & goto parse_args)
if /i "%~1"=="--client-kit" (set "CLIENT_KIT=1" & shift & goto parse_args)
if /i "%~1"=="/no-client-kit" (set "CLIENT_KIT=0" & shift & goto parse_args)
if /i "%~1"=="--no-client-kit" (set "CLIENT_KIT=0" & shift & goto parse_args)
if /i "%~1"=="/bundle-inno" (set "BUNDLE_INNO=1" & shift & goto parse_args)
if /i "%~1"=="--bundle-inno" (set "BUNDLE_INNO=1" & shift & goto parse_args)
if /i "%~1"=="/keep-work" (set "KEEP_WORK=1" & shift & goto parse_args)
if /i "%~1"=="--keep-work" (set "KEEP_WORK=1" & shift & goto parse_args)
if /i "%~1"=="/verify-only" (set "VERIFY_ONLY=1" & shift & goto parse_args)
if /i "%~1"=="--verify-only" (set "VERIFY_ONLY=1" & shift & goto parse_args)
echo [ERROR] Unknown argument: %~1
echo.
goto usage

:missing_hosts
echo [ERROR] Missing value after /hosts.
echo.
goto usage

:parsed
set "WORK_DIR=%WORKSPACE%\build\tmp\dist-work"
set "STAGE_DIR=%WORKSPACE%\dist.stage"
set "FINAL_DIST_DIR=%WORKSPACE%\dist"
set "PREVIOUS_DIST_DIR=%WORKSPACE%\dist.previous"
set "CLIENT_PACKAGE_DIR=%WORK_DIR%\LSYL Tunnel Client"
set "SERVER_PACKAGE_DIR=%WORK_DIR%\LSYL Tunnel Server"
set "ANDROID_APK=%WORK_DIR%\android\LSYL-Tunnel-Android.apk"
set "INSTALLERS_DIR=%STAGE_DIR%\installers"

if "%VERIFY_ONLY%"=="1" goto verify_final_dist

call deploy\windows\go-env.cmd "%WORKSPACE%" "%GOEXE%" || exit /b 1

echo [INFO] LSYL Tunnel release workflow
echo [INFO] Workspace: %WORKSPACE%
echo [INFO] Final dist is generated directly from source through build\tmp\dist-work and dist.stage.
if "%CLIENT_KIT%"=="1" echo [INFO] Client kit will be kept in final dist for on-site client installer regeneration.
if "%BUNDLE_INNO%"=="1" if not "%CLIENT_KIT%"=="1" echo [WARN] /bundle-inno has no effect with /no-client-kit.

if "%LOCAL_SIGN%"=="1" (
  echo [1/8] Ensure local code signing certificate
  call deploy\windows\sign\init-selfsigned-codesign.cmd || exit /b 1
) else (
  echo [1/8] Code signing setup
  if exist certs\codesign-thumbprint.txt (
    echo [INFO] Using existing local code signing thumbprint: certs\codesign-thumbprint.txt
  ) else if defined LSYL_SIGN_CERT_PFX (
    echo [INFO] Using PFX from LSYL_SIGN_CERT_PFX.
  ) else if defined LSYL_SIGN_CERT_SHA1 (
    echo [INFO] Using certificate thumbprint from LSYL_SIGN_CERT_SHA1.
  ) else (
    echo [INFO] No signing certificate configured. Add /local-sign for local test signing, or configure a formal certificate.
  )
)

if defined HOSTS (
  echo [2/8] Generate server TLS certificate and sync client trust
  call deploy\windows\cert\init-server.cmd "%HOSTS%" || exit /b 1
  copy /y certs\server.crt src\client\cert\server.crt >nul || exit /b 1
  echo [INFO] Synced certs\server.crt to src\client\cert\server.crt
) else (
  echo [2/8] Server TLS certificate
  if not exist src\client\cert\server.crt if exist certs\server.crt copy /y certs\server.crt src\client\cert\server.crt >nul
  if exist src\client\cert\server.crt (
    echo [INFO] Client trust certificate ready: src\client\cert\server.crt
  ) else (
    echo [WARN] Missing src\client\cert\server.crt. Use /hosts "dns,ip" before building a client installer.
  )
)

if "%SKIP_TEST%"=="1" (
  echo [3/8] Tests skipped
) else (
  echo [3/8] Run Go tests
  "%GOEXE%" test ./src/... || exit /b 1
)

echo [4/8] Recreate temporary release work directories
call :remove_dir "%WORK_DIR%" "old release work directory" || exit /b 1
call :remove_dir "%STAGE_DIR%" "old staged dist directory" || exit /b 1
mkdir "%WORK_DIR%" || exit /b 1
mkdir "%INSTALLERS_DIR%" || exit /b 1

echo [5/8] Build temporary packages from source
call deploy\windows\app\package-client.cmd "%CLIENT_PACKAGE_DIR%" || goto fail
call deploy\windows\app\package-server.cmd "%SERVER_PACKAGE_DIR%" || goto fail
call deploy\windows\app\build-android-apk.cmd "%ANDROID_APK%" || goto fail

set "LSYL_CLIENT_PACKAGE_DIR=%CLIENT_PACKAGE_DIR%"
set "LSYL_SERVER_PACKAGE_DIR=%SERVER_PACKAGE_DIR%"
call deploy\windows\sign\sign-release.cmd package || goto fail
set "LSYL_CLIENT_PACKAGE_DIR="
set "LSYL_SERVER_PACKAGE_DIR="

echo [6/8] Build installers into staged dist
call "%CLIENT_PACKAGE_DIR%\make-installer.cmd" "%INSTALLERS_DIR%" || goto fail
call "%SERVER_PACKAGE_DIR%\make-installer.cmd" "%INSTALLERS_DIR%" || goto fail
copy /y "%ANDROID_APK%" "%INSTALLERS_DIR%\LSYL-Tunnel-Android.apk" >nul || goto fail

if "%CLIENT_KIT%"=="1" (
  robocopy "%CLIENT_PACKAGE_DIR%" "%STAGE_DIR%\LSYL Tunnel Client" /E /NFL /NDL /NJH /NJS /NP >nul
  if errorlevel 8 goto fail
  if "%BUNDLE_INNO%"=="1" set "LSYL_BUNDLE_INNO=1"
  call deploy\windows\app\write-dist-tools.cmd "%STAGE_DIR%" || goto fail
  set "LSYL_BUNDLE_INNO="
)

set "LSYL_INSTALLERS_DIR=%INSTALLERS_DIR%"
call deploy\windows\sign\sign-release.cmd installers || goto fail
set "LSYL_INSTALLERS_DIR="

call :verify_dist "%STAGE_DIR%" || goto fail
call :commit_dist || goto fail_keep_stage
powershell -NoProfile -ExecutionPolicy Bypass -File deploy\windows\sign\write-release-manifest.ps1 -OutputDir "%FINAL_DIST_DIR%" -DistDir "%FINAL_DIST_DIR%" || goto fail_after_commit

if not "%KEEP_WORK%"=="1" call :remove_dir "%WORK_DIR%" "release work directory" || exit /b 1

echo Release ready
echo Installers:
echo   %FINAL_DIST_DIR%\installers\LSYL-Tunnel-Client-Setup.exe
echo   %FINAL_DIST_DIR%\installers\LSYL-Tunnel-Server-Setup.exe
echo   %FINAL_DIST_DIR%\installers\LSYL-Tunnel-Android.apk
if "%CLIENT_KIT%"=="1" echo Client kit: %FINAL_DIST_DIR%\LSYL Tunnel Client
echo Manifest:
echo   %FINAL_DIST_DIR%\release-manifest.txt
endlocal
exit /b 0

:verify_final_dist
call :verify_dist "%FINAL_DIST_DIR%" || exit /b 1
echo [INFO] Current dist verified:
echo   %FINAL_DIST_DIR%
endlocal
exit /b 0

:verify_dist
set "VERIFY_DIR=%~1"
if not exist "%VERIFY_DIR%\installers\LSYL-Tunnel-Client-Setup.exe" (
  echo [ERROR] Missing client installer: %VERIFY_DIR%\installers\LSYL-Tunnel-Client-Setup.exe
  exit /b 1
)
if not exist "%VERIFY_DIR%\installers\LSYL-Tunnel-Server-Setup.exe" (
  echo [ERROR] Missing server installer: %VERIFY_DIR%\installers\LSYL-Tunnel-Server-Setup.exe
  exit /b 1
)
if not exist "%VERIFY_DIR%\installers\LSYL-Tunnel-Android.apk" (
  echo [ERROR] Missing Android APK: %VERIFY_DIR%\installers\LSYL-Tunnel-Android.apk
  exit /b 1
)
if exist "%VERIFY_DIR%\LSYL Tunnel Server" (
  echo [ERROR] Final dist must not contain server package directory: %VERIFY_DIR%\LSYL Tunnel Server
  exit /b 1
)
if exist "%VERIFY_DIR%\LSYL Tunnel Lightweight Clients" (
  echo [ERROR] Final dist must not contain lightweight handoff directory: %VERIFY_DIR%\LSYL Tunnel Lightweight Clients
  exit /b 1
)
if exist "%VERIFY_DIR%\LSYL Tunnel Client" (
  if not exist "%VERIFY_DIR%\LSYL Tunnel Client\make-installer.cmd" (
    echo [ERROR] Client kit is incomplete: missing make-installer.cmd
    exit /b 1
  )
  if not exist "%VERIFY_DIR%\make-installers.cmd" (
    echo [ERROR] Client kit dist is missing make-installers.cmd
    exit /b 1
  )
) else (
  if exist "%VERIFY_DIR%\make-installers.cmd" (
    echo [ERROR] make-installers.cmd exists but client kit is not present.
    exit /b 1
  )
)
powershell -NoProfile -ExecutionPolicy Bypass -File deploy\windows\inno\verify-installer-version.ps1 -ScriptFile deploy\windows\inno\package-client.iss -InstallerFile "%VERIFY_DIR%\installers\LSYL-Tunnel-Client-Setup.exe"
if errorlevel 1 exit /b 1
powershell -NoProfile -ExecutionPolicy Bypass -File deploy\windows\inno\verify-installer-version.ps1 -ScriptFile deploy\windows\inno\package-server.iss -InstallerFile "%VERIFY_DIR%\installers\LSYL-Tunnel-Server-Setup.exe"
if errorlevel 1 exit /b 1
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$expectSigned=$false;" ^
  "if($env:LSYL_SIGN_CERT_PFX -or $env:LSYL_SIGN_CERT_SHA1 -or (Test-Path 'certs\codesign-thumbprint.txt')){ $expectSigned=$true }" ^
  "$targets=@('%VERIFY_DIR%\installers\LSYL-Tunnel-Client-Setup.exe','%VERIFY_DIR%\installers\LSYL-Tunnel-Server-Setup.exe');" ^
  "if(Test-Path '%VERIFY_DIR%\LSYL Tunnel Client\bin\lsyl-tunnel-client-gui.exe'){ $targets += @('%VERIFY_DIR%\LSYL Tunnel Client\bin\lsyl-tunnel-client-gui.exe','%VERIFY_DIR%\LSYL Tunnel Client\bin\lsyl-tunnel-client-lite.exe') }" ^
  "$bad=@(); foreach($t in $targets){ $sig=Get-AuthenticodeSignature $t; $name=Split-Path $t -Leaf; if($sig.SignerCertificate){ Write-Host ('[INFO] '+$name+' signature: '+$sig.Status+' / '+$sig.SignerCertificate.Subject) } else { Write-Host ('[INFO] '+$name+' signature: '+$sig.Status) }; if($expectSigned -and $sig.Status -ne 'Valid'){ $bad += $t } }" ^
  "if($bad){ $bad | ForEach-Object { Write-Host ('[ERROR] Invalid or missing signature: '+$_) }; exit 1 }" ^
  "exit 0" || exit /b 1
exit /b 0

:remove_dir
set "TARGET_DIR=%~1"
set "TARGET_LABEL=%~2"
if not exist "%TARGET_DIR%" exit /b 0
rmdir /s /q "%TARGET_DIR%" 2>nul
if exist "%TARGET_DIR%" (
  echo [ERROR] Cannot remove %TARGET_LABEL%:
  echo   %TARGET_DIR%
  exit /b 1
)
exit /b 0

:commit_dist
call :remove_dir "%PREVIOUS_DIST_DIR%" "previous dist directory" || exit /b 1
if exist "%FINAL_DIST_DIR%" move "%FINAL_DIST_DIR%" "%PREVIOUS_DIST_DIR%" >nul || exit /b 1
move "%STAGE_DIR%" "%FINAL_DIST_DIR%" >nul
if errorlevel 1 (
  echo [ERROR] Failed to promote staged dist: %STAGE_DIR%
  if exist "%PREVIOUS_DIST_DIR%" if not exist "%FINAL_DIST_DIR%" move "%PREVIOUS_DIST_DIR%" "%FINAL_DIST_DIR%" >nul
  exit /b 1
)
call :remove_dir "%PREVIOUS_DIST_DIR%" "previous dist directory" || exit /b 1
exit /b 0

:fail
set "ERR=%ERRORLEVEL%"
if "%ERR%"=="0" set "ERR=1"
echo [ERROR] Release failed. Final dist was not replaced.
if not "%STAGE_DIR%"=="" if exist "%STAGE_DIR%" echo [INFO] Staged dist left for inspection: %STAGE_DIR%
endlocal
exit /b %ERR%

:fail_keep_stage
set "ERR=%ERRORLEVEL%"
if "%ERR%"=="0" set "ERR=1"
echo [ERROR] Release failed while replacing dist.
echo [INFO] Staged dist left for inspection: %STAGE_DIR%
endlocal
exit /b %ERR%

:fail_after_commit
set "ERR=%ERRORLEVEL%"
if "%ERR%"=="0" set "ERR=1"
echo [ERROR] Release replaced final dist but failed while writing release manifest.
echo [INFO] Final dist is available for inspection: %FINAL_DIST_DIR%
endlocal
exit /b %ERR%

:usage
echo Usage:
echo   release.cmd [/hosts "dns,ip"] [/local-sign] [/skip-test] [/no-client-kit] [/bundle-inno] [/verify-only] [/keep-work]
echo.
echo Common:
echo   release.cmd /hosts "vpn.example.com,203.0.113.10" /local-sign
echo   release.cmd
echo   release.cmd /no-client-kit
echo   release.cmd /verify-only
echo.
echo Options:
echo   /hosts        Regenerate server TLS certificate and copy server.crt into client trust input.
echo   /local-sign   Create/reuse a local self-signed code signing certificate for development signing.
echo   /skip-test    Skip go test ./src/... for faster local iteration.
echo   /client-kit   Keep dist\LSYL Tunnel Client for on-site client installer rebuilds. This is the default.
echo   /no-client-kit Build a compact dist without dist\LSYL Tunnel Client or dist\make-installers.cmd.
echo   /bundle-inno  With client kit, copy Inno Setup compiler into dist\tools\inno when available.
echo   /verify-only  Only verify current dist outputs.
echo   /keep-work    Keep build\tmp\dist-work after a successful release for inspection.
if "%HELP_REQUESTED%"=="1" exit /b 0
exit /b 1
