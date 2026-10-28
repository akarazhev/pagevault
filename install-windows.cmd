@echo off
setlocal DisableDelayedExpansion
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0install-windows.ps1" %*
set "PAGEVAULT_EXIT_CODE=%ERRORLEVEL%"
if not "%PAGEVAULT_EXIT_CODE%"=="0" echo Installation failed. See README.md.
pause
exit /b %PAGEVAULT_EXIT_CODE%
