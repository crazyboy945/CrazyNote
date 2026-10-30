@echo off
rem ============================================================
rem  CrazyNote - Markdown Notes for Windows
rem
rem  Double-click to start. Notes and config are created next
rem  to this script on first run.
rem
rem  The service detaches from this window and keeps running in
rem  the background. Use stop.bat to stop it.
rem
rem  Note: ASCII-only text so it works with any console code page.
rem ============================================================
setlocal enabledelayedexpansion

set "PORT=8899"
set "NOTES_DIR=%~dp0notes"
set "APPDATA_DIR=%~dp0appdata"
set "BIN=%~dp0CrazyNote.exe"
set "LOG=%~dp0CrazyNote.log"

if not exist "%BIN%" (
    echo [ERROR] CrazyNote.exe not found next to this script.
    echo Please keep CrazyNote.exe and start.bat in the same folder.
    pause
    exit /b 1
)

if not exist "%NOTES_DIR%" mkdir "%NOTES_DIR%"
if not exist "%APPDATA_DIR%" mkdir "%APPDATA_DIR%"

rem Check if the port is already in use
netstat -ano | findstr /R /C:":%PORT% .*LISTENING" >nul 2>&1
if %errorlevel%==0 (
    echo [INFO] Port %PORT% is already in use - CrazyNote may be running.
    echo Opening browser...
    start "" "http://127.0.0.1:%PORT%"
    pause
    exit /b 0
)

echo.
echo   CrazyNote is starting...
echo   Notes dir : %NOTES_DIR%
echo   URL       : http://127.0.0.1:%PORT%
echo.
echo   The service runs in the background. This window can be closed.
echo   ----------------------------------------
echo.

rem Start detached (-daemon) so the service survives closing this window.
rem -log makes the service write its own log file (stdout is not available
rem once detached).
start "" "%BIN%" -daemon -log "%LOG%" -data "%NOTES_DIR%" -appdata "%APPDATA_DIR%" -addr "0.0.0.0:%PORT%" > NUL 2>&1

rem Wait until the service responds
set /a tries=0
:wait
timeout /t 1 /nobreak >nul 2>&1
set /a tries+=1
curl -s --noproxy "*" -o nul "http://127.0.0.1:%PORT%/api/health" 2>nul
if not errorlevel 1 goto ready
if %tries% lss 15 goto wait

echo [ERROR] Service did not start. Check CrazyNote.log in this folder.
pause
exit /b 1

:ready
echo [READY] Opening browser...
start "" "http://127.0.0.1:%PORT%"

rem Print the initial password (first run only). The server writes an
rem ASCII "INITIAL_PASSWORD=" line so this works with any code page.
if exist "%LOG%" (
    findstr /C:"INITIAL_PASSWORD=" "%LOG%" >nul 2>&1 && findstr /C:"INITIAL_PASSWORD=" "%LOG%"
)
echo.
echo Tip: you can change the password in the web UI settings.
echo Use stop.bat to stop the service.
echo.
pause
endlocal
