@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..\..\..") do set "WORKSPACE=%%~fI"
set "DIST_ROOT=%WORKSPACE%\dist"
set "DEST_DIR=%DIST_ROOT%\tools\inno"
if not "%~1"=="" (
  set "DEST_DIR=%~1"
  for %%I in ("%~1\..\..") do set "DIST_ROOT=%%~fI"
)
for %%I in ("%DIST_ROOT%") do set "DIST_ROOT=%%~fI"
for %%I in ("%DEST_DIR%") do set "DEST_DIR=%%~fI"

call :find_inno
if not defined INNO_DIR (
  echo [WARN] Inno Setup 6 was not found on this build machine.
  echo [WARN] dist will still work on machines with Inno installed or INNO_SETUP_ISCC configured.
  exit /b 0
)
for %%I in ("%INNO_DIR%") do set "INNO_DIR=%%~fI"
if not exist "%INNO_DIR%\ISCC.exe" (
  echo [WARN] Inno Setup compiler not found:
  echo   %INNO_DIR%\ISCC.exe
  exit /b 0
)

echo [INFO] Bundling Inno Setup compiler:
echo   from %INNO_DIR%
echo   to   %DEST_DIR%

powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$ErrorActionPreference='Stop';" ^
  "$src=[IO.Path]::GetFullPath($env:INNO_DIR);" ^
  "$dst=[IO.Path]::GetFullPath($env:DEST_DIR);" ^
  "$root=[IO.Path]::GetFullPath($env:DIST_ROOT);" ^
  "$rootPrefix=$root.TrimEnd([IO.Path]::DirectorySeparatorChar,[IO.Path]::AltDirectorySeparatorChar)+[IO.Path]::DirectorySeparatorChar;" ^
  "if(-not $dst.StartsWith($rootPrefix,[StringComparison]::OrdinalIgnoreCase)){ throw ('Refusing to write outside dist: '+$dst) }" ^
  "if(Test-Path -LiteralPath $dst){ Remove-Item -LiteralPath $dst -Recurse -Force }" ^
  "New-Item -ItemType Directory -Force -Path $dst | Out-Null;" ^
  "$skip=@('Compil32.exe','ISetup-dark.chm','ISetup.chm','isfaq.url','whatsnew.htm');" ^
  "Get-ChildItem -LiteralPath $src -File | Where-Object { $_.Name -notmatch '^unins' -and $skip -notcontains $_.Name -and $_.Name -notlike 'Wiz*.bmp' } | Copy-Item -Destination $dst -Force;" ^
  "$langSrc=Join-Path $src 'Languages'; $langDst=Join-Path $dst 'Languages';" ^
  "if(Test-Path -LiteralPath $langSrc){ New-Item -ItemType Directory -Force -Path $langDst | Out-Null; Copy-Item -Path (Join-Path $langSrc '*') -Destination $langDst -Recurse -Force }" ^
  "$readme=Join-Path $dst 'README.txt';" ^
  "$lines=@('Bundled Inno Setup compiler for LSYL Tunnel installer generation.','','This directory is copied from the local Inno Setup installation so implementation staff can build installers from dist without installing Inno separately.','Use ..\..\make-installers.cmd from the dist root, or package-local make-installer.cmd.','','Inno Setup license and copyright notices are retained in license.txt.','Original project: https://jrsoftware.org/');" ^
  "Set-Content -LiteralPath $readme -Encoding ASCII -Value $lines;" ^
  "foreach($required in @('ISCC.exe','ISCmplr.dll','ISPP.dll','ISPPBuiltins.iss','Setup.e32','SetupLdr.e32','SetupLdr.e64','Default.isl','license.txt')){ if(-not (Test-Path -LiteralPath (Join-Path $dst $required))){ throw ('Bundled Inno file missing: '+$required) } }"
if errorlevel 1 exit /b 1

echo [INFO] Bundled Inno Setup compiler ready.
exit /b 0

:find_inno
if defined INNO_SETUP_HOME (
  if exist "%INNO_SETUP_HOME%\ISCC.exe" (
    set "INNO_DIR=%INNO_SETUP_HOME%"
    exit /b 0
  )
)
if defined INNO_SETUP_ISCC (
  if exist "%INNO_SETUP_ISCC%" (
    for %%I in ("%INNO_SETUP_ISCC%") do set "INNO_DIR=%%~dpI"
    exit /b 0
  )
)
if exist "%LocalAppData%\Programs\Inno Setup 6\ISCC.exe" (
  set "INNO_DIR=%LocalAppData%\Programs\Inno Setup 6"
  exit /b 0
)
if exist "%ProgramFiles(x86)%\Inno Setup 6\ISCC.exe" (
  set "INNO_DIR=%ProgramFiles(x86)%\Inno Setup 6"
  exit /b 0
)
if exist "%ProgramFiles%\Inno Setup 6\ISCC.exe" (
  set "INNO_DIR=%ProgramFiles%\Inno Setup 6"
  exit /b 0
)
exit /b 0
