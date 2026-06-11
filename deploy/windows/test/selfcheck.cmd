@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..\..\..") do set "WORKSPACE=%%~fI"
cd /d "%WORKSPACE%" || exit /b 1
set "SELF_TMP=build\tmp"
if not exist "%SELF_TMP%" mkdir "%SELF_TMP%"

echo [1/6] go test ./...
go test ./... || exit /b 1

echo [2/6] build binaries
call deploy\windows\build.cmd all || exit /b 1

echo [3/6] verify admin tools
build\bin\server\lsyl-tunnel-passwd.exe -password selfcheck-password >nul || exit /b 1
if exist %SELF_TMP%\selfcheck-certs rmdir /s /q %SELF_TMP%\selfcheck-certs
build\bin\server\lsyl-tunnel-cert.exe -out %SELF_TMP%\selfcheck-certs -hosts "localhost,127.0.0.1" || exit /b 1
if not exist %SELF_TMP%\selfcheck-certs\server.crt (echo [ERROR] Missing: %SELF_TMP%\selfcheck-certs\server.crt & exit /b 1)
if not exist %SELF_TMP%\selfcheck-certs\server.key (echo [ERROR] Missing: %SELF_TMP%\selfcheck-certs\server.key & exit /b 1)
rmdir /s /q %SELF_TMP%\selfcheck-certs

echo [4/6] verify script and source layout
if not exist src\client\conf\client.yaml (echo [ERROR] Missing: src\client\conf\client.yaml & exit /b 1)
if not exist src\client\cert\server.crt (echo [ERROR] Missing: src\client\cert\server.crt & exit /b 1)
if not exist src\client\assets\client.ico (echo [ERROR] Missing: src\client\assets\client.ico & exit /b 1)
if not exist src\client\assets\client-connected.ico (echo [ERROR] Missing: src\client\assets\client-connected.ico & exit /b 1)
if not exist src\server\conf\server.yaml (echo [ERROR] Missing: src\server\conf\server.yaml & exit /b 1)
if not exist src\server\assets\server.ico (echo [ERROR] Missing: src\server\assets\server.ico & exit /b 1)
if not exist src\server\assets\server.svg (echo [ERROR] Missing: src\server\assets\server.svg & exit /b 1)
if not exist src\server\front\front.go (echo [ERROR] Missing: src\server\front\front.go & exit /b 1)
if not exist src\server\front\index.html (echo [ERROR] Missing: src\server\front\index.html & exit /b 1)
if not exist src\server\front\styles.css (echo [ERROR] Missing: src\server\front\styles.css & exit /b 1)
if not exist src\server\front\app.js (echo [ERROR] Missing: src\server\front\app.js & exit /b 1)
if not exist src\client\cmd\lsyl-tunnel-client\rsrc.syso (echo [ERROR] Missing: client CLI rsrc & exit /b 1)
if not exist src\client\cmd\lsyl-tunnel-client-gui\rsrc.syso (echo [ERROR] Missing: client GUI rsrc & exit /b 1)
if not exist src\client\cmd\lsyl-tunnel-client-lite\rsrc_windows_386.syso (echo [ERROR] Missing: client Lite 386 rsrc & exit /b 1)
if not exist src\client\cmd\lsyl-tunnel-client-lite\rsrc_windows_amd64.syso (echo [ERROR] Missing: client Lite amd64 rsrc & exit /b 1)
if not exist src\cmd\lsyl-tunnel-profile\rsrc.syso (echo [ERROR] Missing: profile tool rsrc & exit /b 1)
if not exist src\server\cmd\lsyl-tunnel-server\rsrc.syso (echo [ERROR] Missing: server CLI rsrc & exit /b 1)
if not exist src\server\cmd\lsyl-tunnel-server-gui\rsrc.syso (echo [ERROR] Missing: server GUI rsrc & exit /b 1)
if not exist src\server\cmd\lsyl-tunnel-server-svc\rsrc.syso (echo [ERROR] Missing: server service rsrc & exit /b 1)
if not exist src\cmd\lsyl-tunnel-cert\rsrc.syso (echo [ERROR] Missing: cert tool rsrc & exit /b 1)
if not exist src\cmd\lsyl-tunnel-passwd\rsrc.syso (echo [ERROR] Missing: password tool rsrc & exit /b 1)
if exist client\conf (echo [ERROR] Obsolete root source path exists: client\conf & exit /b 1)
if exist server\conf (echo [ERROR] Obsolete root source path exists: server\conf & exit /b 1)
if exist client\cmd (echo [ERROR] Obsolete root source path exists: client\cmd & exit /b 1)
if exist server\cmd (echo [ERROR] Obsolete root source path exists: server\cmd & exit /b 1)
if exist internal (echo [ERROR] Obsolete root source path exists: internal & exit /b 1)
if exist cmd (echo [ERROR] Obsolete root source path exists: cmd & exit /b 1)
if exist configs (echo [ERROR] Obsolete path exists: configs & exit /b 1)
if exist deploy\windows\tunnel (echo [ERROR] Obsolete path exists: deploy\windows\tunnel & exit /b 1)
if exist deploy\windows\build (echo [ERROR] Obsolete split build directory exists: deploy\windows\build & exit /b 1)
if exist deploy\windows\build\all.cmd (echo [ERROR] Obsolete split build script exists: deploy\windows\build\all.cmd & exit /b 1)
if exist deploy\windows\build\client.cmd (echo [ERROR] Obsolete split build script exists: deploy\windows\build\client.cmd & exit /b 1)
if exist deploy\windows\build\server.cmd (echo [ERROR] Obsolete split build script exists: deploy\windows\build\server.cmd & exit /b 1)
if exist deploy\windows\build\profile.cmd (echo [ERROR] Obsolete split build script exists: deploy\windows\build\profile.cmd & exit /b 1)
if exist deploy\windows\selfcheck.cmd (echo [ERROR] Obsolete path exists: deploy\windows\selfcheck.cmd & exit /b 1)
if exist deploy\windows\run-client.cmd (echo [ERROR] Obsolete path exists: deploy\windows\run-client.cmd & exit /b 1)
if exist deploy\windows\run-server.cmd (echo [ERROR] Obsolete path exists: deploy\windows\run-server.cmd & exit /b 1)
if exist deploy\windows\run (echo [ERROR] Obsolete split run directory exists: deploy\windows\run & exit /b 1)
if exist deploy\windows\run\client.cmd (echo [ERROR] Obsolete split run script exists: deploy\windows\run\client.cmd & exit /b 1)
if exist deploy\windows\run\client-gui.cmd (echo [ERROR] Obsolete split run script exists: deploy\windows\run\client-gui.cmd & exit /b 1)
if exist deploy\windows\run\server.cmd (echo [ERROR] Obsolete split run script exists: deploy\windows\run\server.cmd & exit /b 1)
if exist deploy\windows\run\server-gui.cmd (echo [ERROR] Obsolete split run script exists: deploy\windows\run\server-gui.cmd & exit /b 1)
if exist deploy\windows\cert\regen-server.cmd (echo [ERROR] Obsolete path exists: deploy\windows\cert\regen-server.cmd & exit /b 1)
if exist deploy\windows\service\client.cmd (echo [ERROR] Obsolete path exists: deploy\windows\service\client.cmd & exit /b 1)
if exist deploy\windows\app\install-client-app.cmd (echo [ERROR] Obsolete manual install script exists: deploy\windows\app\install-client-app.cmd & exit /b 1)
if exist deploy\windows\app\uninstall-client-app.cmd (echo [ERROR] Obsolete manual uninstall script exists: deploy\windows\app\uninstall-client-app.cmd & exit /b 1)
if exist deploy\windows\app\uninstall-client-app.ps1 (echo [ERROR] Obsolete manual uninstall script exists: deploy\windows\app\uninstall-client-app.ps1 & exit /b 1)
if exist deploy\windows\app\install-server-app.cmd (echo [ERROR] Obsolete manual install script exists: deploy\windows\app\install-server-app.cmd & exit /b 1)
if exist deploy\windows\app\uninstall-server-app.cmd (echo [ERROR] Obsolete manual uninstall script exists: deploy\windows\app\uninstall-server-app.cmd & exit /b 1)
if exist deploy\windows\app\uninstall-server-app.ps1 (echo [ERROR] Obsolete manual uninstall script exists: deploy\windows\app\uninstall-server-app.ps1 & exit /b 1)
if exist deploy\windows\release.cmd (echo [ERROR] Obsolete duplicated release entry exists: deploy\windows\release.cmd & exit /b 1)
if exist deploy\windows\inno\build-installers.cmd (echo [ERROR] Obsolete duplicated installer build entry exists: deploy\windows\inno\build-installers.cmd & exit /b 1)
if exist deploy\windows\inno\compile-client-package-installer.cmd (echo [ERROR] Obsolete compatibility entry exists: deploy\windows\inno\compile-client-package-installer.cmd & exit /b 1)
if not exist release.cmd (echo [ERROR] Missing: release.cmd & exit /b 1)
if not exist deploy\windows\build.cmd (echo [ERROR] Missing: deploy\windows\build.cmd & exit /b 1)
if not exist deploy\windows\run.cmd (echo [ERROR] Missing: deploy\windows\run.cmd & exit /b 1)
call deploy\windows\service\server.cmd status >nul || exit /b 1
if not exist deploy\windows\sign\sign-release.cmd (echo [ERROR] Missing: deploy\windows\sign\sign-release.cmd & exit /b 1)
if not exist deploy\windows\sign\sign-release.ps1 (echo [ERROR] Missing: deploy\windows\sign\sign-release.ps1 & exit /b 1)
if not exist deploy\windows\sign\write-release-manifest.ps1 (echo [ERROR] Missing: deploy\windows\sign\write-release-manifest.ps1 & exit /b 1)
if not exist deploy\windows\sign\init-selfsigned-codesign.cmd (echo [ERROR] Missing: deploy\windows\sign\init-selfsigned-codesign.cmd & exit /b 1)
if not exist deploy\windows\app\write-dist-tools.cmd (echo [ERROR] Missing: deploy\windows\app\write-dist-tools.cmd & exit /b 1)
if not exist deploy\windows\app\package-profile.cmd (echo [ERROR] Missing: deploy\windows\app\package-profile.cmd & exit /b 1)
if not exist deploy\windows\app\package-light-clients.cmd (echo [ERROR] Missing: deploy\windows\app\package-light-clients.cmd & exit /b 1)
if not exist deploy\windows\inno\make-installers.cmd (echo [ERROR] Missing: deploy\windows\inno\make-installers.cmd & exit /b 1)
if not exist deploy\windows\inno\bundle-inno.cmd (echo [ERROR] Missing: deploy\windows\inno\bundle-inno.cmd & exit /b 1)
if not exist deploy\windows\inno\make-client-installer.cmd (echo [ERROR] Missing: deploy\windows\inno\make-client-installer.cmd & exit /b 1)
if not exist deploy\windows\inno\make-server-installer.cmd (echo [ERROR] Missing: deploy\windows\inno\make-server-installer.cmd & exit /b 1)
if not exist deploy\windows\inno\package-client.iss (echo [ERROR] Missing: deploy\windows\inno\package-client.iss & exit /b 1)
if not exist deploy\windows\inno\package-server.iss (echo [ERROR] Missing: deploy\windows\inno\package-server.iss & exit /b 1)
findstr /c:"autostartservice" deploy\windows\inno\package-server.iss >nul || (echo [ERROR] Server installer is missing service auto-start confirmation task & exit /b 1)
findstr /c:"-start-type" deploy\windows\inno\package-server.iss >nul || (echo [ERROR] Server installer is missing service start-type registration parameter & exit /b 1)
if exist deploy\windows\inno\client.iss (echo [ERROR] Obsolete source-side Inno template remains: deploy\windows\inno\client.iss & exit /b 1)
if exist deploy\windows\inno\server.iss (echo [ERROR] Obsolete source-side Inno template remains: deploy\windows\inno\server.iss & exit /b 1)
powershell -NoProfile -ExecutionPolicy Bypass -Command "$hit=Select-String -Path 'deploy\windows\inno\package-client.iss' -SimpleMatch 'taskkill' -List; if($hit){ Write-Host ('[ERROR] Client installer must not force-kill processes: ' + $hit.Path + ':' + $hit.LineNumber); exit 1 }" || exit /b 1
findstr /c:"RollbackClientRuntimeFiles" deploy\windows\inno\package-client.iss >nul || (echo [ERROR] Client installer is missing rollback cleanup logic & exit /b 1)
call deploy\windows\sign\sign-release.cmd package >nul || exit /b 1
powershell -NoProfile -ExecutionPolicy Bypass -Command "$roots=@('src','deploy','docs'); $files=foreach($r in $roots){ if(Test-Path $r){ Get-ChildItem $r -Recurse -File } }; $files=$files | Where-Object { $_.FullName -notlike '*\deploy\windows\test\selfcheck.cmd' }; $hit=$files | Select-String -SimpleMatch 'LSYLTunnelClient','lsyl-tunnel-client-svc' -List; if($hit){ $hit | ForEach-Object { Write-Host ('[ERROR] Forbidden client service reference: ' + $_.Path + ':' + $_.LineNumber) }; exit 1 }" || exit /b 1
powershell -NoProfile -ExecutionPolicy Bypass -File deploy\windows\test\check-text.ps1 || exit /b 1

echo [5/6] verify build outputs and package layouts
if not exist build\bin\client\lsyl-tunnel-client.exe (echo [ERROR] Missing: build\bin\client\lsyl-tunnel-client.exe & exit /b 1)
if not exist build\bin\client\lsyl-tunnel-client-gui.exe (echo [ERROR] Missing: build\bin\client\lsyl-tunnel-client-gui.exe & exit /b 1)
if not exist build\bin\client\lsyl-tunnel-client-lite.exe (echo [ERROR] Missing: build\bin\client\lsyl-tunnel-client-lite.exe & exit /b 1)
if exist build\bin\client\lsyl-tunnel-client-svc.exe (echo [ERROR] Obsolete client service exe remains & exit /b 1)
if not exist build\bin\profile\lsyl-tunnel-profile.exe (echo [ERROR] Missing: build\bin\profile\lsyl-tunnel-profile.exe & exit /b 1)
if not exist build\bin\server\lsyl-tunnel-server.exe (echo [ERROR] Missing: build\bin\server\lsyl-tunnel-server.exe & exit /b 1)
if not exist build\bin\server\lsyl-tunnel-server-gui.exe (echo [ERROR] Missing: build\bin\server\lsyl-tunnel-server-gui.exe & exit /b 1)
if not exist build\bin\server\lsyl-tunnel-server-svc.exe (echo [ERROR] Missing: build\bin\server\lsyl-tunnel-server-svc.exe & exit /b 1)
if not exist build\bin\server\lsyl-tunnel-cert.exe (echo [ERROR] Missing: build\bin\server\lsyl-tunnel-cert.exe & exit /b 1)
if not exist build\bin\server\lsyl-tunnel-passwd.exe (echo [ERROR] Missing: build\bin\server\lsyl-tunnel-passwd.exe & exit /b 1)
if not exist mobile\android\app\build\outputs\apk\release\app-release.apk if not exist mobile\android\app\build\outputs\apk\debug\app-debug.apk (echo [ERROR] Missing Android APK for lightweight clients package & exit /b 1)
powershell -NoProfile -ExecutionPolicy Bypass -Command "$targets=@('build\bin\client\lsyl-tunnel-client.exe','build\bin\client\lsyl-tunnel-client-gui.exe','build\bin\client\lsyl-tunnel-client-lite.exe','build\bin\profile\lsyl-tunnel-profile.exe','build\bin\server\lsyl-tunnel-server.exe','build\bin\server\lsyl-tunnel-server-gui.exe','build\bin\server\lsyl-tunnel-server-svc.exe','build\bin\server\lsyl-tunnel-cert.exe','build\bin\server\lsyl-tunnel-passwd.exe'); $bad=@(); foreach($t in $targets){ $vi=(Get-Item $t).VersionInfo; if([string]::IsNullOrWhiteSpace($vi.ProductName) -or [string]::IsNullOrWhiteSpace($vi.FileDescription) -or [string]::IsNullOrWhiteSpace($vi.CompanyName) -or $vi.FileVersion -ne '1.1.0.0'){ $bad += ($t + ' product=' + $vi.ProductName + ' desc=' + $vi.FileDescription + ' company=' + $vi.CompanyName + ' version=' + $vi.FileVersion) } }; if($bad){ Write-Host '[ERROR] Missing or incomplete Windows version resource:'; $bad | ForEach-Object { Write-Host ('  ' + $_) }; exit 1 }" || exit /b 1
powershell -NoProfile -ExecutionPolicy Bypass -Command "$p='build\bin\client\lsyl-tunnel-client-lite.exe'; $b=[IO.File]::ReadAllBytes($p); $pe=[BitConverter]::ToInt32($b,0x3c); $machine=[BitConverter]::ToUInt16($b,$pe+4); if($machine -ne 0x014c){ Write-Host ('[ERROR] Win7 Lite client must be PE32/i386, got machine 0x{0:X4}' -f $machine); exit 1 }" || exit /b 1
powershell -NoProfile -ExecutionPolicy Bypass -File deploy\windows\sign\write-release-manifest.ps1 -OutputDir %SELF_TMP%\selfcheck-release-manifest -Files build\bin\client\lsyl-tunnel-client.exe,build\bin\client\lsyl-tunnel-client-gui.exe,build\bin\client\lsyl-tunnel-client-lite.exe,build\bin\profile\lsyl-tunnel-profile.exe,build\bin\server\lsyl-tunnel-server.exe,build\bin\server\lsyl-tunnel-server-gui.exe,build\bin\server\lsyl-tunnel-server-svc.exe,build\bin\server\lsyl-tunnel-cert.exe,build\bin\server\lsyl-tunnel-passwd.exe >nul || exit /b 1
if not exist %SELF_TMP%\selfcheck-release-manifest\release-manifest.json (echo [ERROR] Missing generated release manifest json & exit /b 1)
if not exist %SELF_TMP%\selfcheck-release-manifest\release-manifest.txt (echo [ERROR] Missing generated release manifest txt & exit /b 1)
rmdir /s /q %SELF_TMP%\selfcheck-release-manifest
if exist %SELF_TMP%\selfcheck-client-package rmdir /s /q %SELF_TMP%\selfcheck-client-package
call deploy\windows\app\package-client.cmd %SELF_TMP%\selfcheck-client-package >nul || exit /b 1
if not exist %SELF_TMP%\selfcheck-client-package\cert\server.crt (echo [ERROR] Missing: client package cert & exit /b 1)
if not exist %SELF_TMP%\selfcheck-client-package\bin\lsyl-tunnel-client-lite.exe (echo [ERROR] Missing: client package Lite exe & exit /b 1)
if exist %SELF_TMP%\selfcheck-client-package\bin\lsyl-tunnel-profile.exe (echo [ERROR] Client package must not include independent profile tool & exit /b 1)
if not exist %SELF_TMP%\selfcheck-client-package\make-installer.cmd (echo [ERROR] Missing: client package installer builder & exit /b 1)
if not exist %SELF_TMP%\selfcheck-client-package\installer\client.iss (echo [ERROR] Missing: client package Inno script & exit /b 1)
if not exist %SELF_TMP%\selfcheck-client-package\installer\Languages\ChineseSimplified.isl (echo [ERROR] Missing: client package installer language & exit /b 1)
powershell -NoProfile -ExecutionPolicy Bypass -Command "$hit=Select-String -Path '%SELF_TMP%\selfcheck-client-package\installer\client.iss' -SimpleMatch 'taskkill' -List; if($hit){ Write-Host ('[ERROR] Client package installer must not force-kill processes: ' + $hit.Path + ':' + $hit.LineNumber); exit 1 }" || exit /b 1
findstr /c:"RollbackClientRuntimeFiles" %SELF_TMP%\selfcheck-client-package\installer\client.iss >nul || (echo [ERROR] Client package installer is missing rollback cleanup logic & exit /b 1)
if exist %SELF_TMP%\selfcheck-client-package\install-client-app.cmd (echo [ERROR] Client package should not include manual install script & exit /b 1)
if exist %SELF_TMP%\selfcheck-client-package\uninstall-client-app.cmd (echo [ERROR] Client package should not include manual uninstall script & exit /b 1)
if exist %SELF_TMP%\selfcheck-client-package\uninstall-client-app.ps1 (echo [ERROR] Client package should not include manual uninstall script & exit /b 1)
if exist %SELF_TMP%\selfcheck-client-package\bin\lsyl-tunnel-client-svc.exe (echo [ERROR] Client package should not include service exe & exit /b 1)
findstr /c:"../cert/server.crt" %SELF_TMP%\selfcheck-client-package\conf\client.yaml >nul || exit /b 1
powershell -NoProfile -ExecutionPolicy Bypass -Command "$p='%SELF_TMP%\selfcheck-client-package\conf\client.yaml'; $t=Get-Content -Raw -Encoding UTF8 $p; if($t -match 'ciphertext:|key_id:|client_id:\s*demo-client'){ Write-Host '[ERROR] Client package config contains runtime-only credential data or demo client id.'; exit 1 }; if($t -notmatch '(?m)^saved_credential:\s*\{\}' -or $t -notmatch '(?m)^forwards:'){ Write-Host '[ERROR] Client package config was not sanitized correctly or lost forwards.'; exit 1 }" || exit /b 1
rmdir /s /q %SELF_TMP%\selfcheck-client-package
if exist %SELF_TMP%\selfcheck-light-clients-package rmdir /s /q %SELF_TMP%\selfcheck-light-clients-package
call deploy\windows\app\package-light-clients.cmd %SELF_TMP%\selfcheck-light-clients-package >nul || exit /b 1
if not exist %SELF_TMP%\selfcheck-light-clients-package\android\lsyl-tunnel-mobile.apk (echo [ERROR] Missing: lightweight package Android APK & exit /b 1)
if not exist %SELF_TMP%\selfcheck-light-clients-package\windows-win7\lsyl-tunnel-client-lite.exe (echo [ERROR] Missing: lightweight package Win7 Lite exe & exit /b 1)
if not exist %SELF_TMP%\selfcheck-light-clients-package\profiles\README.txt (echo [ERROR] Missing: lightweight package profiles README & exit /b 1)
powershell -NoProfile -ExecutionPolicy Bypass -Command "$p='%SELF_TMP%\selfcheck-light-clients-package\windows-win7\lsyl-tunnel-client-lite.exe'; $b=[IO.File]::ReadAllBytes($p); $pe=[BitConverter]::ToInt32($b,0x3c); $machine=[BitConverter]::ToUInt16($b,$pe+4); if($machine -ne 0x014c){ Write-Host ('[ERROR] Lightweight package Win7 Lite client must be PE32/i386, got machine 0x{0:X4}' -f $machine); exit 1 }" || exit /b 1
rmdir /s /q %SELF_TMP%\selfcheck-light-clients-package
if exist %SELF_TMP%\selfcheck-profile-package rmdir /s /q %SELF_TMP%\selfcheck-profile-package
call deploy\windows\app\package-profile.cmd %SELF_TMP%\selfcheck-profile-package >nul || exit /b 1
if not exist %SELF_TMP%\selfcheck-profile-package\bin\lsyl-tunnel-profile.exe (echo [ERROR] Missing: profile package tool & exit /b 1)
if not exist %SELF_TMP%\selfcheck-profile-package\README.txt (echo [ERROR] Missing: profile package README & exit /b 1)
if exist %SELF_TMP%\selfcheck-profile-package\bin\lsyl-tunnel-client-gui.exe (echo [ERROR] Profile package must not include client GUI & exit /b 1)
if exist %SELF_TMP%\selfcheck-profile-package\bin\lsyl-tunnel-client-lite.exe (echo [ERROR] Profile package must not include client Lite & exit /b 1)
if exist %SELF_TMP%\selfcheck-profile-package\installer (echo [ERROR] Profile package must not include installer files & exit /b 1)
rmdir /s /q %SELF_TMP%\selfcheck-profile-package
if exist %SELF_TMP%\selfcheck-server-package rmdir /s /q %SELF_TMP%\selfcheck-server-package
call deploy\windows\app\package-server.cmd %SELF_TMP%\selfcheck-server-package >nul || exit /b 1
if not exist %SELF_TMP%\selfcheck-server-package\bin\lsyl-tunnel-server.exe (echo [ERROR] Missing: server package cli & exit /b 1)
if not exist %SELF_TMP%\selfcheck-server-package\bin\lsyl-tunnel-server-svc.exe (echo [ERROR] Missing: server package service & exit /b 1)
if not exist %SELF_TMP%\selfcheck-server-package\bin\lsyl-tunnel-server-gui.exe (echo [ERROR] Missing: server package gui & exit /b 1)
if not exist %SELF_TMP%\selfcheck-server-package\bin\lsyl-tunnel-cert.exe (echo [ERROR] Missing: server package cert tool & exit /b 1)
if not exist %SELF_TMP%\selfcheck-server-package\bin\lsyl-tunnel-passwd.exe (echo [ERROR] Missing: server package password tool & exit /b 1)
if not exist %SELF_TMP%\selfcheck-server-package\conf\server.yaml (echo [ERROR] Missing: server package config & exit /b 1)
if not exist %SELF_TMP%\selfcheck-server-package\certs (echo [ERROR] Missing: server package certs dir & exit /b 1)
if not exist %SELF_TMP%\selfcheck-server-package\data (echo [ERROR] Missing: server package data dir & exit /b 1)
if not exist %SELF_TMP%\selfcheck-server-package\logs (echo [ERROR] Missing: server package logs dir & exit /b 1)
if not exist %SELF_TMP%\selfcheck-server-package\logs\request (echo [ERROR] Missing: server package request logs dir & exit /b 1)
if not exist %SELF_TMP%\selfcheck-server-package\logs\business (echo [ERROR] Missing: server package business logs dir & exit /b 1)
if not exist %SELF_TMP%\selfcheck-server-package\logs\entry-traffic (echo [ERROR] Missing: server package entry traffic logs dir & exit /b 1)
if not exist %SELF_TMP%\selfcheck-server-package\logs\flow-traffic (echo [ERROR] Missing: server package flow traffic logs dir & exit /b 1)
if not exist %SELF_TMP%\selfcheck-server-package\logs\service (echo [ERROR] Missing: server package service logs dir & exit /b 1)
if not exist %SELF_TMP%\selfcheck-server-package\assets\server.ico (echo [ERROR] Missing: server package icon & exit /b 1)
if not exist %SELF_TMP%\selfcheck-server-package\make-installer.cmd (echo [ERROR] Missing: server package installer builder & exit /b 1)
if not exist %SELF_TMP%\selfcheck-server-package\installer\server.iss (echo [ERROR] Missing: server package Inno script & exit /b 1)
if not exist %SELF_TMP%\selfcheck-server-package\installer\Languages\ChineseSimplified.isl (echo [ERROR] Missing: server package installer language & exit /b 1)
findstr /c:"autostartservice" %SELF_TMP%\selfcheck-server-package\installer\server.iss >nul || (echo [ERROR] Server package installer is missing service auto-start confirmation task & exit /b 1)
findstr /c:"-start-type" %SELF_TMP%\selfcheck-server-package\installer\server.iss >nul || (echo [ERROR] Server package installer is missing service start-type registration parameter & exit /b 1)
if exist %SELF_TMP%\selfcheck-server-package\install-server-app.cmd (echo [ERROR] Server package should not include manual install script & exit /b 1)
if exist %SELF_TMP%\selfcheck-server-package\uninstall-server-app.cmd (echo [ERROR] Server package should not include manual uninstall script & exit /b 1)
if exist %SELF_TMP%\selfcheck-server-package\uninstall-server-app.ps1 (echo [ERROR] Server package should not include manual uninstall script & exit /b 1)
findstr /c:"../certs/server.crt" %SELF_TMP%\selfcheck-server-package\conf\server.yaml >nul || exit /b 1
findstr /c:"../data/server-state.json" %SELF_TMP%\selfcheck-server-package\conf\server.yaml >nul || exit /b 1
findstr /c:"../data/server-permanent-block.txt" %SELF_TMP%\selfcheck-server-package\conf\server.yaml >nul || exit /b 1
findstr /c:"../logs/request/request.jsonl" %SELF_TMP%\selfcheck-server-package\conf\server.yaml >nul || exit /b 1
findstr /c:"../logs/business/business.jsonl" %SELF_TMP%\selfcheck-server-package\conf\server.yaml >nul || exit /b 1
findstr /c:"../logs/entry-traffic/entry-traffic.jsonl" %SELF_TMP%\selfcheck-server-package\conf\server.yaml >nul || exit /b 1
findstr /c:"../logs/flow-traffic/flow-traffic.jsonl" %SELF_TMP%\selfcheck-server-package\conf\server.yaml >nul || exit /b 1
findstr /c:"logs\service\server-service.log" %SELF_TMP%\selfcheck-server-package\installer\server.iss >nul || exit /b 1
powershell -NoProfile -ExecutionPolicy Bypass -Command "$p='%SELF_TMP%\selfcheck-server-package\conf\server.yaml'; $t=Get-Content -Raw -Encoding UTF8 $p; if($t -match '(?m)username:\s*(alice|bob)\b' -or $t -match '(?m)^\s*-\s*name:\s*(web|sql|rev)\b'){ Write-Host '[ERROR] Server package config contains demo users or demo forwards.'; exit 1 }" || exit /b 1
rmdir /s /q %SELF_TMP%\selfcheck-server-package

echo [6/6] verify docs and installer config
if not exist README.md (echo [ERROR] Missing: README.md & exit /b 1)
if not exist docs\README-zh.md (echo [ERROR] Missing: docs\README-zh.md & exit /b 1)
if not exist docs\system-flow-zh.md (echo [ERROR] Missing: docs\system-flow-zh.md & exit /b 1)
if not exist docs\deployment-zh.md (echo [ERROR] Missing: docs\deployment-zh.md & exit /b 1)
if not exist docs\client-user-zh.md (echo [ERROR] Missing: docs\client-user-zh.md & exit /b 1)
if not exist docs\server-admin-zh.md (echo [ERROR] Missing: docs\server-admin-zh.md & exit /b 1)
if not exist docs\config-reference-zh.md (echo [ERROR] Missing: docs\config-reference-zh.md & exit /b 1)
if not exist docs\security-model-zh.md (echo [ERROR] Missing: docs\security-model-zh.md & exit /b 1)
if not exist docs\customization-zh.md (echo [ERROR] Missing: docs\customization-zh.md & exit /b 1)
if not exist docs\release-notes-zh.md (echo [ERROR] Missing: docs\release-notes-zh.md & exit /b 1)
if not exist docs\internal\README-zh.md (echo [ERROR] Missing: docs\internal\README-zh.md & exit /b 1)
if not exist docs\internal\architecture-zh.md (echo [ERROR] Missing: docs\internal\architecture-zh.md & exit /b 1)
if not exist docs\internal\quickstart-zh.md (echo [ERROR] Missing: docs\internal\quickstart-zh.md & exit /b 1)
if not exist docs\internal\network-flow-zh.md (echo [ERROR] Missing: docs\internal\network-flow-zh.md & exit /b 1)
if not exist docs\internal\windows-service-zh.md (echo [ERROR] Missing: docs\internal\windows-service-zh.md & exit /b 1)
if not exist docs\internal\release-signing-zh.md (echo [ERROR] Missing: docs\internal\release-signing-zh.md & exit /b 1)
if not exist docs\internal\release-readiness-checklist-zh.md (echo [ERROR] Missing: docs\internal\release-readiness-checklist-zh.md & exit /b 1)
if not exist deploy\windows\README.md (echo [ERROR] Missing: deploy\windows\README.md & exit /b 1)
if not exist deploy\windows\inno\assets\client.ico (echo [ERROR] Missing: installer client icon & exit /b 1)
if not exist deploy\windows\inno\assets\server.ico (echo [ERROR] Missing: installer server icon & exit /b 1)

echo selfcheck PASS
endlocal
