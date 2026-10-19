@echo off
setlocal
cd /d "%~dp0server"
if errorlevel 1 (
    echo ERROR: Cannot find the server folder.
    pause
    exit /b 1
)
go run .
if errorlevel 1 (
    echo.
    echo ERROR: ASFWMS failed to run. Verify Go is installed and check the message above.
    pause
    exit /b 1
)
endlocal
