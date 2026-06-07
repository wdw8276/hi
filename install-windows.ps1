# hi — Windows installer (PowerShell)
# Usage: irm https://raw.githubusercontent.com/mars-base/hi/main/install-windows.ps1 | iex
#
# Supports fresh install and upgrade to latest release.

param(
    [string]$InstallDir = ""
)

$ErrorActionPreference = "Stop"

$Repo = "mars-base/hi"
$Bin = "hi.exe"
$Arch = "amd64"

# Get latest release tag.
try {
    $Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
    $Tag = $Release.tag_name
} catch {
    Write-Error "Failed to fetch latest release from GitHub. Check your internet connection."
    exit 1
}

$Version = $Tag -replace '^v', ''

# Determine install directory.
if ($InstallDir -eq "") {
    $InstallDir = "$env:USERPROFILE\.local\bin"
}
$Dest = Join-Path $InstallDir $Bin

# Check current version — skip if already up to date.
$currentVersion = ""
try {
    $currentVersion = (& $Dest --version 2>$null) -replace '^hi ', ''
} catch {}
if ($currentVersion -eq $Version) {
    Write-Host "hi $Tag is already installed and up to date."
    exit 0
}

if ($currentVersion) {
    Write-Host "Upgrading hi: $currentVersion -> $Version"
} else {
    Write-Host "Installing hi $Tag for windows/$Arch..."
}

# Download to temp name to avoid file-lock issues.
$Archive = "hi-${Version}-windows-${Arch}.zip"
$Url = "https://github.com/$Repo/releases/download/$Tag/$Archive"
$TmpBin = Join-Path $InstallDir "hi_new.exe"

Invoke-WebRequest -Uri $Url -OutFile $Archive
Expand-Archive -Path $Archive -DestinationPath . -Force
Remove-Item $Archive

# Ensure install directory exists.
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

# Stop running hi processes so we can replace the binary.
$running = Get-Process hi -ErrorAction SilentlyContinue
if ($running) {
    Write-Host "Stopping running hi processes..."
    $running | Stop-Process -Force
    Start-Sleep -Seconds 2
}

# Replace binary.
Move-Item -Force $Bin $TmpBin
try {
    Remove-Item $Dest -Force -ErrorAction SilentlyContinue
    Move-Item -Force $TmpBin $Dest
} catch {
    Write-Warning "Could not replace $Dest — file may be in use."
    Write-Warning "Close all Claude Code / hi sessions and run this script again."
    Remove-Item $TmpBin -Force -ErrorAction SilentlyContinue
    Remove-Item $Bin -Force -ErrorAction SilentlyContinue
    exit 1
}

Write-Host ""
Write-Host "hi $Tag installed to $Dest"

# Add to user PATH if not already present.
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -notlike "*$InstallDir*") {
    Write-Host ""
    Write-Host "Adding $InstallDir to your user PATH..."
    [Environment]::SetEnvironmentVariable("Path", "$UserPath;$InstallDir", "User")
    $env:Path = "$env:Path;$InstallDir"
    Write-Host "PATH updated. Restart your terminal or run: `$env:Path += ';$InstallDir'"
}

Write-Host ""
Write-Host "Run: hi launch"
