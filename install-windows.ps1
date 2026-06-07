# hi — Windows installer (PowerShell)
# Usage: irm https://raw.githubusercontent.com/mars-base/hi/main/install-windows.ps1 | iex
#
# Supports fresh install and upgrade to latest release.

param(
    [string]$InstallDir = ""
)

# Ensure TLS 1.2 for older Windows / PowerShell versions.
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$Repo = "mars-base/hi"
$Bin = "hi.exe"
$Arch = "amd64"

# Get latest release tag.
$Tag = $null
try {
    $Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
    $Tag = $Release.tag_name
} catch {
    Write-Host "ERROR: Failed to fetch latest release from GitHub."
    Write-Host "Check your internet connection and try again."
}

if ($Tag) {
    $Version = $Tag -replace '^v', ''

    # Determine install directory.
    if ($InstallDir -eq "") {
        $InstallDir = "$env:USERPROFILE\.local\bin"
    }
    $Dest = Join-Path $InstallDir $Bin

    # Check current version.
    $currentVersion = ""
    try {
        $currentVersion = (& $Dest --version 2>$null) -replace '^hi ', ''
    } catch {}

    if ($currentVersion -eq $Version) {
        Write-Host "hi $Tag is already installed and up to date."
    } else {
        if ($currentVersion) {
            Write-Host "Upgrading hi: $currentVersion -> $Version"
        } else {
            Write-Host "Installing hi $Tag for windows/$Arch..."
        }

        # Download.
        $Archive = "hi-${Version}-windows-${Arch}.zip"
        $Url = "https://github.com/$Repo/releases/download/$Tag/$Archive"
        $TmpDir = "$env:TEMP\hi-install-$Version"
        $TmpBin = Join-Path $TmpDir $Bin

        New-Item -ItemType Directory -Path $TmpDir -Force | Out-Null
        Invoke-WebRequest -Uri $Url -OutFile (Join-Path $TmpDir $Archive)
        Expand-Archive -Path (Join-Path $TmpDir $Archive) -DestinationPath $TmpDir -Force

        # Check for running hi processes.
        $running = Get-Process hi -ErrorAction SilentlyContinue
        if ($running) {
            Write-Host ""
            Write-Host "WARNING: hi is currently running (PID: $($running.Id))."
            Write-Host "Stop it first (Ctrl+C in Claude Code), then run this script again."
            Remove-Item $TmpDir -Recurse -Force -ErrorAction SilentlyContinue
        } else {
            # Ensure install directory exists.
            if (-not (Test-Path $InstallDir)) {
                New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
            }

            # Replace binary.
            $installed = $false
            Move-Item -Force (Join-Path $TmpDir $Bin) $TmpBin
            try {
                Remove-Item $Dest -Force -ErrorAction SilentlyContinue
                Move-Item -Force $TmpBin $Dest
                $installed = $true
            } catch {
                Write-Host ""
                Write-Host "WARNING: Could not replace $Dest — file may be in use."
                Write-Host "Close all Claude Code / hi sessions and run this script again."
                Remove-Item $TmpBin -Force -ErrorAction SilentlyContinue
            }
            Remove-Item $TmpDir -Recurse -Force -ErrorAction SilentlyContinue

            if ($installed) {
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
            }
        }
    }
}
