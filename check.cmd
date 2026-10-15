@echo off
setlocal EnableExtensions DisableDelayedExpansion
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0deploy\windows\test\check.ps1" %*
exit /b %errorlevel%
