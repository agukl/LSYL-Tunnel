@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%.") do set "WORKSPACE=%%~fI"
cd /d "%WORKSPACE%" || exit /b 1

set "HOSTS="
set "LOCAL_SIGN=0"
set "SKIP_TEST=0"
set "PACKAGE_ONLY=0"
set "VERIFY_ONLY=0"
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
if /i "%~1"=="/local-sign" (
  set "LOCAL_SIGN=1"
  shift
  goto parse_args
)
if /i "%~1"=="--local-sign" (
  set "LOCAL_SIGN=1"
  shift
  goto parse_args
)
if /i "%~1"=="/skip-test" (
  set "SKIP_TEST=1"
  shift
  goto parse_args
)
if /i "%~1"=="--skip-test" (
  set "SKIP_TEST=1"
  shift
  goto parse_args
)
if /i "%~1"=="/package-only" (
  set "PACKAGE_ONLY=1"
  shift
  goto parse_args
)
if /i "%~1"=="--package-only" (
  set "PACKAGE_ONLY=1"
  shift
  goto parse_args
)
if /i "%~1"=="/verify-only" (
  set "VERIFY_ONLY=1"
  shift
  goto parse_args
)
if /i "%~1"=="--verify-only" (
  set "VERIFY_ONLY=1"
  shift
  goto parse_args
)
echo [ERROR] Unknown argument: %~1
echo.
goto usage

:missing_hosts
echo [ERROR] Missing value after /hosts.
echo.
goto usage

:parsed
if "%VERIFY_ONLY%"=="1" goto verify

echo [INFO] LSYL Tunnel release workflow
echo [INFO] Workspace: %WORKSPACE%

if "%LOCAL_SIGN%"=="1" (
  echo [1/7] Ensure local code signing certificate
  call deploy\windows\sign\init-selfsigned-codesign.cmd || exit /b 1
) else (
  echo [1/7] Code signing setup
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
  echo [2/7] Generate server TLS certificate and sync client trust
  call deploy\windows\cert\init-server.cmd "%HOSTS%" || exit /b 1
  copy /y certs\server.crt src\client\cert\server.crt >nul || exit /b 1
  echo [INFO] Synced certs\server.crt to src\client\cert\server.crt
) else (
  echo [2/7] Server TLS certificate
  if not exist src\client\cert\server.crt if exist certs\server.crt copy /y certs\server.crt src\client\cert\server.crt >nul
  if exist src\client\cert\server.crt (
    echo [INFO] Client trust certificate ready: src\client\cert\server.crt
  ) else (
    echo [WARN] Missing src\client\cert\server.crt. Use /hosts "dns,ip" before building an installer for clients.
  )
)

if "%SKIP_TEST%"=="1" (
  echo [3/7] Tests skipped
) else (
  echo [3/7] Run Go tests
  go test ./... || exit /b 1
)

if "%PACKAGE_ONLY%"=="1" (
  echo [4/7] Build dist packages
  if exist dist\installers (
    rmdir /s /q dist\installers || exit /b 1
  )
  call deploy\windows\app\package-client.cmd || exit /b 1
  call deploy\windows\app\package-light-clients.cmd || exit /b 1
  call deploy\windows\app\package-server.cmd || exit /b 1
  call deploy\windows\app\package-profile.cmd || exit /b 1
  call deploy\windows\app\write-dist-tools.cmd || exit /b 1
  call deploy\windows\sign\sign-release.cmd package || exit /b 1
) else (
  echo [4/7] Build packages and Inno installers
  call deploy\windows\app\package-client.cmd || exit /b 1
  call deploy\windows\app\package-light-clients.cmd || exit /b 1
  call deploy\windows\app\package-server.cmd || exit /b 1
  call deploy\windows\app\package-profile.cmd || exit /b 1
  call deploy\windows\app\write-dist-tools.cmd || exit /b 1
  call deploy\windows\sign\sign-release.cmd package || exit /b 1
  if not exist "dist\installers" mkdir "dist\installers"
  call "dist\make-installers.cmd" || exit /b 1
  call deploy\windows\sign\sign-release.cmd installers || exit /b 1
)

goto verify

:verify
echo [5/7] Verify release outputs and signatures
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$packageOnly='%PACKAGE_ONLY%' -eq '1';" ^
  "$expectSigned=$false;" ^
  "if($env:LSYL_SIGN_CERT_PFX -or $env:LSYL_SIGN_CERT_SHA1 -or (Test-Path 'certs\codesign-thumbprint.txt')){ $expectSigned=$true }" ^
  "$targets=@('dist\LSYL Tunnel Client\bin\lsyl-tunnel-client-gui.exe','dist\LSYL Tunnel Client\bin\lsyl-tunnel-client-lite.exe','dist\LSYL Tunnel Lightweight Clients\windows-win7\lsyl-tunnel-client-lite.exe','dist\LSYL Tunnel Profile Tool\bin\lsyl-tunnel-profile.exe','dist\LSYL Tunnel Server\bin\lsyl-tunnel-server.exe','dist\LSYL Tunnel Server\bin\lsyl-tunnel-server-svc.exe','dist\LSYL Tunnel Server\bin\lsyl-tunnel-server-gui.exe','dist\LSYL Tunnel Server\bin\lsyl-tunnel-passwd.exe','dist\LSYL Tunnel Server\bin\lsyl-tunnel-cert.exe');" ^
  "$artifacts=@('dist\LSYL Tunnel Lightweight Clients\android\lsyl-tunnel-mobile.apk');" ^
  "if(-not $packageOnly){ $targets += @('dist\installers\LSYL-Tunnel-Client-Setup.exe','dist\installers\LSYL-Tunnel-Server-Setup.exe') }" ^
  "$missing=@(); $bad=@(); foreach($t in $targets){ if(-not (Test-Path $t)){ $missing += $t; continue }; $sig=Get-AuthenticodeSignature $t; $name=Split-Path $t -Leaf; if($sig.SignerCertificate){ Write-Host ('[INFO] '+$name+' signature: '+$sig.Status+' / '+$sig.SignerCertificate.Subject) } else { Write-Host ('[INFO] '+$name+' signature: '+$sig.Status) }; if($expectSigned -and $sig.Status -ne 'Valid'){ $bad += $t } }" ^
  "foreach($t in $artifacts){ if(-not (Test-Path $t)){ $missing += $t } else { Write-Host ('[INFO] release artifact: '+$t) } }" ^
  "if($missing){ $missing | ForEach-Object { Write-Host ('[ERROR] Missing release output: '+$_) }; exit 1 }" ^
  "if($bad){ $bad | ForEach-Object { Write-Host ('[ERROR] Invalid or missing signature: '+$_) }; exit 1 }" ^
  "exit 0" || exit /b 1

echo [6/7] Write release manifest
if "%PACKAGE_ONLY%"=="1" (
  powershell -NoProfile -ExecutionPolicy Bypass -File deploy\windows\sign\write-release-manifest.ps1 -PackageOnly || exit /b 1
) else (
  powershell -NoProfile -ExecutionPolicy Bypass -File deploy\windows\sign\write-release-manifest.ps1 || exit /b 1
)

echo [7/7] Release ready
if "%PACKAGE_ONLY%"=="1" (
  echo Packages:
  echo   %WORKSPACE%\dist\LSYL Tunnel Client
  echo   %WORKSPACE%\dist\LSYL Tunnel Lightweight Clients
  echo   %WORKSPACE%\dist\LSYL Tunnel Server
  echo   %WORKSPACE%\dist\LSYL Tunnel Profile Tool
  echo Manifest:
  echo   %WORKSPACE%\dist\release-manifest.txt
) else (
  echo Installers:
  echo   %WORKSPACE%\dist\installers\LSYL-Tunnel-Client-Setup.exe
  echo   %WORKSPACE%\dist\installers\LSYL-Tunnel-Server-Setup.exe
  echo Manifest:
  echo   %WORKSPACE%\dist\release-manifest.txt
)
endlocal
exit /b 0

:usage
echo Usage:
echo   release.cmd [/hosts "dns,ip"] [/local-sign] [/skip-test] [/package-only] [/verify-only]
echo.
echo Common:
echo   release.cmd /hosts "vpn.example.com,203.0.113.10" /local-sign
echo   release.cmd
echo   release.cmd /skip-test
echo   release.cmd /verify-only
echo.
echo Options:
echo   /hosts        Regenerate server TLS certificate and copy server.crt into client trust input.
echo   /local-sign   Create/reuse a local self-signed code signing certificate for development signing.
echo   /skip-test    Skip go test ./... for faster local iteration.
echo   /package-only Build signed dist packages but do not compile Inno installers.
echo   /verify-only  Only verify current release outputs and signatures.
if "%HELP_REQUESTED%"=="1" exit /b 0
exit /b 1
