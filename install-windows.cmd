@echo off
REM hi — Windows installer (CMD)
REM Usage: curl -fsSL https://raw.githubusercontent.com/mars-base/hi/main/install-windows.cmd -o install.cmd && install.cmd && del install.cmd

setlocal enabledelayedexpansion

set REPO=mars-base/hi
set BIN=hi.exe
set ARCH=amd64

REM Get latest release tag via GitHub API.
for /f "tokens=*" %%i in ('curl -sL "https://api.github.com/repos/%REPO%/releases/latest" 2^>nul ^| findstr /r "tag_name"') do set TAG_LINE=%%i
if "%TAG_LINE%"=="" set TAG=v1.0.0

REM Parse tag from JSON (minimal extraction).
for /f "tokens=2 delims=:" %%i in ('echo %TAG_LINE%') do set TAG=%%i
set TAG=%TAG:"=%
set TAG=%TAG: =%
set TAG=%TAG:,=%

set VERSION=%TAG:~1%
set ARCHIVE=hi-%VERSION%-windows-%ARCH%.zip
set URL=https://github.com/%REPO%/releases/download/%TAG%/%ARCHIVE%

echo Downloading hi %TAG% for windows/%ARCH%...
curl -fsSL "%URL%" -o "%ARCHIVE%"

REM Extract using PowerShell (available on all modern Windows).
powershell -Command "Expand-Archive -Path '%ARCHIVE%' -DestinationPath '.' -Force"
del "%ARCHIVE%"

REM Determine install directory.
set INSTALL_DIR=%USERPROFILE%\.local\bin
if not exist "%INSTALL_DIR%" mkdir "%INSTALL_DIR%"

REM Move binary.
move /Y "%BIN%" "%INSTALL_DIR%\%BIN%"

echo.
echo hi %TAG% installed to %INSTALL_DIR%\%BIN%

REM Add to user PATH if not already present.
set "USER_PATH="
for /f "tokens=3" %%i in ('reg query "HKCU\Environment" /v PATH 2^>nul') do set USER_PATH=%%i
echo !USER_PATH! | findstr /c:"%INSTALL_DIR%" >nul
if errorlevel 1 (
    echo.
    echo Adding %INSTALL_DIR% to your user PATH...
    setx PATH "!USER_PATH!;%INSTALL_DIR%" >nul
    echo PATH updated. Restart your terminal to use 'hi'.
)

echo.
echo Run: hi launch

endlocal
