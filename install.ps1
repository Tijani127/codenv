# codenv installer for Windows.
#
# One-liner (copy/paste into PowerShell):
#   irm https://github.com/Tijani127/codenv/releases/latest/download/install.ps1 | iex
#
# What it does:
#   1. Downloads the right codenv-windows-<arch>.zip from the latest release.
#   2. Expands it into %LOCALAPPDATA%\Programs\codenv.
#   3. Adds that directory to your user PATH.
#
# Usage:
#   .\install.ps1                      install the latest release for this machine's architecture
#   .\install.ps1 -Arch x64            force an architecture (x64 or arm64)
#   .\install.ps1 -NoPath              do not touch PATH; print the install directory instead
#
# Works on Windows PowerShell 5.1 and PowerShell 7+.

[CmdletBinding()]
param(
    [ValidateSet('x64', 'arm64')]
    [string]$Arch = '',
    [switch]$NoPath
)

$ErrorActionPreference = 'Stop'

# Windows PowerShell 5.1 defaults to TLS 1.0/1.2 negotiation tables that GitHub
# does not always accept; force TLS 1.2 up front.
try { [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12 } catch { }

# Map the machine's architecture to the release asset. PROCESSOR_ARCHITECTURE
# reports the actual OS architecture even in 32-bit shells.
$machine = "$env:PROCESSOR_ARCHITECTURE"
if ($machine -eq '') { $machine = 'AMD64' }
$machine = $machine.ToUpper()
if (-not $Arch) {
    switch ($machine) {
        'ARM64'    { $Arch = 'arm64' }
        'AMD64'    { $Arch = 'x64' }
        'X86'      { Write-Host 'Note: 32-bit shell on a 64-bit OS; installing the x64 build.'; $Arch = 'x64' }
        default    { throw "Unsupported processor architecture: $env:PROCESSOR_ARCHITECTURE" }
    }
}

$repo    = 'https://github.com/Tijani127/codenv'
$release = "$repo/releases/latest/download/codenv-windows-$Arch.zip"
$dest    = Join-Path $env:LOCALAPPDATA 'Programs\codenv'

Write-Host "Installing codenv ($Arch) to $dest ..."
New-Item -ItemType Directory -Force -Path $dest | Out-Null

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("codenv-install-{0:N}.zip" -f [guid]::NewGuid())
try {
    # -UseBasicParsing keeps the download working where IE first-run setup is
    # missing (PS 5.1 without it can fail parsing the response).
    try {
        Invoke-WebRequest -Uri $release -OutFile $tmp -UseBasicParsing -ErrorAction Stop
    } catch {
        # irm (iwr) prefers TLS-correct HTTP via Invoke-WebRequest; fall back so
        # corporate proxies that break web responses still get through.
        [Net.WebClient]::new().DownloadFile($release, $tmp)
    }
    Expand-Archive -Path $tmp -DestinationPath $dest -Force
} finally {
    Remove-Item $tmp -Force -ErrorAction SilentlyContinue
}

$exe = Join-Path $dest 'codenv.exe'
if (-not (Test-Path $exe)) {
    throw "The archive did not contain codenv.exe; the release may have changed."
}

if (-not $NoPath) {
    # Edit the user PATH without fiddling with machine state: SetEnvironmentVariable
    # with 'User' scope persists for this account only, and the registry write is
    # picked up by new shells.
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ($null -eq $userPath) { $userPath = '' }
    $entries = $userPath -split ';' | Where-Object { $_ -ne '' }
    if ($entries -notcontains $dest) {
        $newPath = ($entries + $dest) -join ';'
        [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
        Write-Host "Added $dest to your user PATH."
        # Correct this process too, so the version check below works in the
        # same window.
        $env:Path = "$env:Path;$dest"
    }
}

& $exe version
Write-Host ''
Write-Host 'codenv is installed. Open a NEW PowerShell window, then:'
Write-Host ''
Write-Host '    cd to your project and run:  codenv install && codenv shell'
Write-Host ''
Write-Host 'Or start immediately:  ' -NoNewline
Write-Host "$dest\codenv.exe shell" -ForegroundColor Cyan
