# hi — Windows installer (PowerShell)
# Usage: irm https://raw.githubusercontent.com/mars-base/hi/main/install-windows.ps1 | iex
#
# Supports fresh install and upgrade to latest release.

# Ensure TLS 1.2 for older Windows / PowerShell versions.
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

# Wrap in try/finally so the shell never exits unexpectedly.
$scriptBlock = {
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
        return
    }

    $Version = $Tag -replace '^v', ''
    $InstallDir = "$env:USERPROFILE\.local\bin"
    $Dest = Join-Path $InstallDir $Bin

    # Check current version.
    $currentVersion = ""
    try { $currentVersion = (& $Dest --version 2>$null) -replace '^hi ', '' } catch {}

    if ($currentVersion -eq $Version) {
        Write-Host "hi $Tag is already installed and up to date."
        return
    }

    if ($currentVersion) {
        Write-Host "Upgrading hi: $currentVersion -> $Version"
    } else {
        Write-Host "Installing hi $Tag for windows/$Arch..."
    }

    # Download.
    $Archive = "hi-${Version}-windows-${Arch}.zip"
    $Url = "https://github.com/$Repo/releases/download/$Tag/$Archive"
    $TmpDir = "$env:TEMP\hi-install-$Version"

    New-Item -ItemType Directory -Path $TmpDir -Force | Out-Null
    try {
        Invoke-WebRequest -Uri $Url -OutFile (Join-Path $TmpDir $Archive)
        Expand-Archive -Path (Join-Path $TmpDir $Archive) -DestinationPath $TmpDir -Force

        # Check for running hi processes.
        $running = Get-Process hi -ErrorAction SilentlyContinue
        if ($running) {
            Write-Host ""
            Write-Host "WARNING: hi is currently running (PID: $($running.Id))."
            Write-Host "Stop it first (Ctrl+C in Claude Code), then run this script again."
            return
        }

        # Ensure install directory exists.
        if (-not (Test-Path $InstallDir)) {
            New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        }

        # Replace binary.
        $TmpBin = Join-Path $TmpDir $Bin
        Remove-Item $Dest -Force -ErrorAction SilentlyContinue
        Move-Item -Force $TmpBin $Dest

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
    } catch {
        Write-Host ""
        Write-Host "ERROR: Install failed — $($_.Exception.Message)"
    } finally {
        Remove-Item $TmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

& $scriptBlock
