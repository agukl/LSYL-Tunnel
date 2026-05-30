@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..\..\..") do set "WORKSPACE=%%~fI"
cd /d "%WORKSPACE%" || exit /b 1
if not exist certs mkdir certs

set "SUBJECT=%LSYL_CODESIGN_SUBJECT%"
if not defined SUBJECT set "SUBJECT=CN=LSYL Tunnel Local Code Signing"
set "THUMB_FILE=%WORKSPACE%\certs\codesign-thumbprint.txt"
set "CER_FILE=%WORKSPACE%\certs\codesign-local.cer"
set "FORCE=0"
if /i "%~1"=="/force" set "FORCE=1"

powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$subject=$env:SUBJECT; $thumbFile=$env:THUMB_FILE; $cerFile=$env:CER_FILE; $force=$env:FORCE -eq '1';" ^
  "$cert=$null;" ^
  "if(-not $force){ $cert=Get-ChildItem Cert:\CurrentUser\My -CodeSigningCert -ErrorAction SilentlyContinue | Where-Object { $_.Subject -eq $subject -and $_.NotAfter -gt (Get-Date).AddDays(30) } | Sort-Object NotAfter -Descending | Select-Object -First 1 }" ^
  "if(-not $cert){ $cert=New-SelfSignedCertificate -Type CodeSigningCert -Subject $subject -CertStoreLocation Cert:\CurrentUser\My -KeyExportPolicy Exportable -KeyUsage DigitalSignature -HashAlgorithm SHA256 -NotAfter (Get-Date).AddYears(5) }" ^
  "Export-Certificate -Cert $cert -FilePath $cerFile -Force | Out-Null;" ^
  "Import-Certificate -FilePath $cerFile -CertStoreLocation Cert:\CurrentUser\TrustedPublisher | Out-Null;" ^
  "Import-Certificate -FilePath $cerFile -CertStoreLocation Cert:\CurrentUser\Root | Out-Null;" ^
  "Set-Content -Path $thumbFile -Value (($cert.Thumbprint -replace '\s','').ToUpperInvariant()) -Encoding ASCII;" ^
  "Write-Host ('[INFO] Code signing certificate ready: '+$cert.Subject);" ^
  "Write-Host ('[INFO] Thumbprint: '+(($cert.Thumbprint -replace '\s','').ToUpperInvariant()));" ^
  "Write-Host ('[INFO] Thumbprint file: '+$thumbFile);" ^
  "Write-Host ('[INFO] Public cert: '+$cerFile);" || exit /b 1

echo.
echo Local code signing is configured for this workspace.
echo sign-release.cmd will auto-read certs\codesign-thumbprint.txt.
echo To recreate the certificate, run: deploy\windows\sign\init-selfsigned-codesign.cmd /force
endlocal
