@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "SCRIPT_DIR=%~dp0"
for %%I in ("%SCRIPT_DIR%..\..\..") do set "WORKSPACE=%%~fI"
set "PACKAGE_DIR=%WORKSPACE%\dist\LSYL Tunnel Profile Tool"
if not "%~1"=="" set "PACKAGE_DIR=%~1"
cd /d "%WORKSPACE%"
echo [INFO] Profile tool package directory will be refreshed:
echo   %PACKAGE_DIR%
call "%SCRIPT_DIR%..\build.cmd" profile || exit /b 1
if exist "%PACKAGE_DIR%" (
  rmdir /s /q "%PACKAGE_DIR%" 2>nul
  if exist "%PACKAGE_DIR%" (
    echo [WARN] Cannot remove package directory completely; refreshing files in place:
    echo   %PACKAGE_DIR%
    if exist "%PACKAGE_DIR%\installer" rmdir /s /q "%PACKAGE_DIR%\installer" 2>nul
    if exist "%PACKAGE_DIR%\make-installer.cmd" del /f /q "%PACKAGE_DIR%\make-installer.cmd" >nul 2>nul
    if exist "%PACKAGE_DIR%\installer" (
      echo [ERROR] Cannot remove obsolete profile installer files:
      echo   %PACKAGE_DIR%\installer
      echo Close any process using this directory, or choose another package path.
      exit /b 1
    )
    if exist "%PACKAGE_DIR%\make-installer.cmd" (
      echo [ERROR] Cannot remove obsolete profile installer script:
      echo   %PACKAGE_DIR%\make-installer.cmd
      exit /b 1
    )
  )
)
if not exist "%PACKAGE_DIR%" mkdir "%PACKAGE_DIR%" || exit /b 1
if not exist "%PACKAGE_DIR%\bin" mkdir "%PACKAGE_DIR%\bin" || exit /b 1
if not exist "%PACKAGE_DIR%\profiles" mkdir "%PACKAGE_DIR%\profiles" || exit /b 1
copy /y ".\build\bin\profile\lsyl-tunnel-profile.exe" "%PACKAGE_DIR%\bin\" >nul || exit /b 1
> "%PACKAGE_DIR%\README.txt" (
  echo LSYL Tunnel Profile Tool
  echo.
  echo This is an independent development helper for switching LSYL Tunnel Client
  echo connection profiles. It is not part of the client installer and does not
  echo register a Windows service.
  echo.
  echo Run without arguments to open the CMD interactive menu:
  echo   bin\lsyl-tunnel-profile.exe
  echo.
  echo Common commands:
  echo   bin\lsyl-tunnel-profile.exe current
  echo   bin\lsyl-tunnel-profile.exe list
  echo   bin\lsyl-tunnel-profile.exe import-current local
  echo   bin\lsyl-tunnel-profile.exe import dev-a -conf D:\profiles\dev-a\client.yaml -cert D:\profiles\dev-a\server.crt
  echo   bin\lsyl-tunnel-profile.exe use dev-a
  echo   bin\lsyl-tunnel-profile.exe delete dev-a -yes
  echo.
  echo Defaults:
  echo   install:  C:\Program Files\LSYL Tunnel Client
  echo   profiles: C:\ProgramData\LSYL Tunnel Profiles
  echo.
  echo Use -install and -profiles to override these paths.
)
echo Profile tool package created:
echo   %PACKAGE_DIR%
endlocal
