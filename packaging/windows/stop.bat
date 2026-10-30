@echo off
rem ============================================================
rem  Stop the CrazyNote service running on this machine.
rem  Double-click to run.
rem ============================================================
setlocal enabledelayedexpansion

set "PORT=8899"

rem Find the PID listening on the port
set "FOUND="
for /f "tokens=5" %%p in ('netstat -ano ^| findstr /R /C:":%PORT% .*LISTENING"') do (
    set "FOUND=%%p"
)

if not defined FOUND (
    echo [INFO] No CrazyNote service is listening on port %PORT%.
    echo Nothing to stop.
    pause
    exit /b 0
)

rem Verify the process is actually CrazyNote before killing it
set "PROCNAME="
for /f "tokens=1" %%n in ('tasklist /FI "PID eq %FOUND%" /NH') do set "PROCNAME=%%n"

echo Stopping CrazyNote (PID %FOUND%, %PROCNAME%)...
taskkill /F /PID %FOUND% >nul 2>&1

if errorlevel 1 (
    echo [WARN] Could not stop the process. Try running as administrator.
) else (
    echo [DONE] CrazyNote stopped.
)

pause
endlocal
