@echo off
setlocal EnableExtensions
REM Build a portable SQLite-based ASFWMS release. No shell is shown by the
REM resulting Windows applications; logs are written separately per run.
set "ROOT=%~dp0"
set "OUTPUT=%ROOT%dist\asfwms"
set "BACKEND=%ROOT%server"
where go >nul 2>&1
if errorlevel 1 (
  echo ERROR: Go is missing. Install Go 1.23 or newer from https://go.dev/dl/
  exit /b 1
)
if not exist "%ROOT%database\schema.sql" (
  echo ERROR: Missing database\schema.sql
  exit /b 1
)
if not exist "%ROOT%client\structures\index.html" (
  echo ERROR: Missing frontend files.
  exit /b 1
)
echo.
echo == ASFWMS compilation and release ==
echo Root: "%ROOT%"
echo Release: "%OUTPUT%"
REM Avoid accidentally retaining old binaries in the release directory.
if exist "%OUTPUT%" rmdir /s /q "%OUTPUT%"
mkdir "%OUTPUT%\database" 2>nul
mkdir "%OUTPUT%\systems\windows" 2>nul
mkdir "%OUTPUT%\systems\macos" 2>nul
mkdir "%OUTPUT%\systems\linux" 2>nul
mkdir "%OUTPUT%\logs" 2>nul
robocopy "%ROOT%client" "%OUTPUT%\client" /E /NFL /NDL /NJH /NJS /NP >nul
if errorlevel 8 goto failed
copy /Y "%ROOT%database\schema.sql" "%OUTPUT%\database\schema.sql" >nul
if errorlevel 1 goto failed
REM SQLite stays a local file. Copy it only if this is the user's current DB.
if exist "%ROOT%database\database.db" (
  copy /Y "%ROOT%database\database.db" "%OUTPUT%\database\database.db" >nul
  if errorlevel 1 goto failed
)
REM Never create or share credentials from a template silently.
copy /Y "%ROOT%.env.example" "%OUTPUT%\.env.example" >nul
if errorlevel 1 goto failed
if exist "%ROOT%.env" (
  copy /Y "%ROOT%.env" "%OUTPUT%\.env" >nul
) else (
  copy /Y "%ROOT%.env.example" "%OUTPUT%\.env" >nul
)
if errorlevel 1 goto failed
copy /Y "%ROOT%README.md" "%OUTPUT%\README.md" >nul
pushd "%BACKEND%"
if errorlevel 1 goto failed
set "CGO_ENABLED=0"
echo [1/4] Go tests...
go test ./...
if errorlevel 1 goto failed_server
echo [2/4] Go vet...
go vet ./...
if errorlevel 1 goto failed_server
echo [3/4] Windows amd64 GUI binaries (no console)...
set "GOOS=windows"
set "GOARCH=amd64"
go build -trimpath -buildvcs=false -ldflags "-s -w -H=windowsgui" -o "%OUTPUT%\systems\windows\asfwms_start.exe" .
if errorlevel 1 goto failed_server
go build -trimpath -buildvcs=false -ldflags "-s -w -H=windowsgui" -o "%OUTPUT%\systems\windows\asfwms_stop.exe" ./cmd/stop
if errorlevel 1 goto failed_server
echo [4/4] macOS and Linux amd64/arm64 binaries...
set "GOOS=darwin"
set "GOARCH=amd64"
go build -trimpath -buildvcs=false -ldflags "-s -w" -o "%OUTPUT%\systems\macos\asfwms_start_amd64" .
if errorlevel 1 goto failed_server
go build -trimpath -buildvcs=false -ldflags "-s -w" -o "%OUTPUT%\systems\macos\asfwms_stop_amd64" ./cmd/stop
if errorlevel 1 goto failed_server
set "GOARCH=arm64"
go build -trimpath -buildvcs=false -ldflags "-s -w" -o "%OUTPUT%\systems\macos\asfwms_start_arm64" .
if errorlevel 1 goto failed_server
go build -trimpath -buildvcs=false -ldflags "-s -w" -o "%OUTPUT%\systems\macos\asfwms_stop_arm64" ./cmd/stop
if errorlevel 1 goto failed_server
set "GOOS=linux"
set "GOARCH=amd64"
go build -trimpath -buildvcs=false -ldflags "-s -w" -o "%OUTPUT%\systems\linux\asfwms_start_amd64" .
if errorlevel 1 goto failed_server
go build -trimpath -buildvcs=false -ldflags "-s -w" -o "%OUTPUT%\systems\linux\asfwms_stop_amd64" ./cmd/stop
if errorlevel 1 goto failed_server
set "GOARCH=arm64"
go build -trimpath -buildvcs=false -ldflags "-s -w" -o "%OUTPUT%\systems\linux\asfwms_start_arm64" .
if errorlevel 1 goto failed_server
go build -trimpath -buildvcs=false -ldflags "-s -w" -o "%OUTPUT%\systems\linux\asfwms_stop_arm64" ./cmd/stop
if errorlevel 1 goto failed_server
popd
echo.
echo SUCCESS: compiled release is in "%OUTPUT%"
echo Start Windows with systems\windows\asfwms_start.exe; stop with asfwms_stop.exe.
echo DO NOT publish dist\asfwms; it can include database.db and .env.
exit /b 0
:failed_server
popd
:failed
echo ERROR: Compilation or file copy failed. Do not deploy this incomplete release.
exit /b 1
