# hi — Windows installer (PowerShell)
# Usage: irm https://raw.githubusercontent.com/mars-base/hi/main/install-windows.ps1 | iex

param(
    [string]$InstallDir = ""
)

$ErrorActionPreference = "Stop"

$Repo = "mars-base/hi"
$Bin = "hi.exe"

# Detect architecture (only amd64 is supported on Windows).
$Arch = "amd64"

# Get latest release tag.
try {
    $Tag = (Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest").tag_name
} catch {
    Write-Warning "Could not fetch latest release, using fallback v1.0.0"
    $Tag = "v1.0.0"
}

$Version = $Tag -replace '^v', ''
$Archive = "hi-${Version}-windows-${Arch}.zip"
$Url = "https://github.com/$Repo/releases/download/$Tag/$Archive"

Write-Host "Downloading hi $Tag for windows/$Arch..."
Invoke-WebRequest -Uri $Url -OutFile $Archive

# Extract.
Expand-Archive -Path $Archive -DestinationPath . -Force
Remove-Item $Archive

# Determine install directory.
if ($InstallDir -eq "") {
    $LocalBin = "$env:USERPROFILE\.local\bin"
    if (Test-Path "$env:SystemRoot\System32\where.exe") {
        # Prefer a directory already in PATH if possible.
        $InPath = $false
        try {
            $null = Get-Command hi.exe -ErrorAction Stop
            $InPath = $true
        } catch {}
        if (-not $InPath) {
            # Check if LocalAppData is in PATH.
            $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
            if ($UserPath -split ";" | Where-Object { $_ -eq $LocalBin }) {
                $InPath = $true
            }
        }
    }
    $InstallDir = $LocalBin
}

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

# Move binary.
$Dest = Join-Path $InstallDir $Bin
Move-Item -Force $Bin $Dest

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
