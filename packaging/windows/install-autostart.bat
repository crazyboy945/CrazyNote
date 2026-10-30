@echo off
rem ============================================================
rem  Register CrazyNote to start automatically on Windows boot.
rem  Right-click -> "Run as administrator".
rem
rem  Note: ASCII-only text so it works with any console code page.
rem ============================================================
setlocal

set "PORT=8899"
set "INSTALL_DIR=%ProgramData%\CrazyNote"
set "SERVICE_NAME=CrazyNote"

rem Check administrator privileges
net session >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Administrator privileges are required.
    echo Right-click this file and choose "Run as administrator".
    pause
    exit /b 1
)

netstat -ano | findstr /R /C:":%PORT% .*LISTENING" >nul 2>&1
if %errorlevel%==0 (
    echo [ERROR] Port %PORT% is already in use. Stop that program first.
    pause
    exit /b 1
)

echo Installing to %INSTALL_DIR% ...
mkdir "%INSTALL_DIR%" 2>nul
mkdir "%INSTALL_DIR%\notes" 2>nul
mkdir "%INSTALL_DIR%\appdata" 2>nul

copy /Y "%~dp0CrazyNote.exe" "%INSTALL_DIR%\CrazyNote.exe" >nul
copy /Y "%~dp0start.bat" "%INSTALL_DIR%\start.bat" >nul

echo Registering scheduled task for auto-start...
schtasks /Create /TN "%SERVICE_NAME%" /TR "\"%INSTALL_DIR%\CrazyNote.exe\" -data \"%INSTALL_DIR%\notes\" -appdata \"%INSTALL_DIR%\appdata\" -addr 0.0.0.0:%PORT%" /SC ONSTART /RU SYSTEM /RL HIGHEST /F >nul

if errorlevel 1 (
    echo [FAILED] Could not create the scheduled task.
    echo Please run this script as administrator.
    pause
    exit /b 1
)

rem Start it once now (detached via -daemon)
start "" "%INSTALL_DIR%\CrazyNote.exe" -daemon -log "%INSTALL_DIR%\CrazyNote.log" -data "%INSTALL_DIR%\notes" -appdata "%INSTALL_DIR%\appdata" -addr "0.0.0.0:%PORT%" > NUL 2>&1

echo.
echo [DONE] Auto-start registered.
echo   URL         : http://127.0.0.1:%PORT%
echo   Notes dir   : %INSTALL_DIR%\notes
echo   Log file    : %INSTALL_DIR%\CrazyNote.log
echo.
echo   To remove : schtasks /Delete /TN "%SERVICE_NAME%" /F
echo.
pause
endlocal
