# Installs portcheck and connects it to your browsers, in one step (Windows).
#
#   irm https://roystondwork-dev.github.io/portcheck/install.ps1 | iex
#
# Downloads the latest release for this CPU from GitHub, checks it against the
# release's checksums, installs portcheck.exe to
# %LOCALAPPDATA%\Programs\portcheck (or $env:PORTCHECK_INSTALL_DIR), adds that
# folder to your user PATH, and runs `portcheck browser install`, which
# registers portcheck with the browsers installed for this user.
#
# Options (environment variables):
#   PORTCHECK_INSTALL_DIR  install here instead
#   PORTCHECK_VERSION      a version such as 0.9.0 instead of the latest
#   PORTCHECK_NO_BROWSER=1 don't connect browsers
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$repo = 'roystondwork-dev/portcheck'

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { throw "portcheck installer: unsupported CPU $($env:PROCESSOR_ARCHITECTURE)" }
}

if ($env:PORTCHECK_VERSION) {
    $base = "https://github.com/$repo/releases/download/v$($env:PORTCHECK_VERSION.TrimStart('v'))"
} else {
    # No API call (rate-limited per IP): the latest release's checksums name
    # its archives, which carry the version.
    $base = "https://github.com/$repo/releases/latest/download"
}

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("portcheck-" + [Guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    # Saved to a file: GitHub serves it as binary, which Content would return as bytes.
    Invoke-WebRequest "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt') -UseBasicParsing
    $line = Get-Content (Join-Path $tmp 'checksums.txt') | Where-Object { $_ -match "\s\*?(portcheck_(.+)_windows_${arch}\.zip)\s*$" } | Select-Object -First 1
    if (-not $line) { throw "portcheck installer: no portcheck build for windows/$arch in this release" }
    $null = $line -match "\s\*?(portcheck_(.+)_windows_${arch}\.zip)\s*$"
    $archive = $Matches[1]
    $version = $Matches[2]
    $want = ($line -split '\s+')[0]
    Write-Host "Downloading portcheck $version for windows/$arch..."
    Invoke-WebRequest "$base/$archive" -OutFile (Join-Path $tmp $archive) -UseBasicParsing
    $got = (Get-FileHash (Join-Path $tmp $archive) -Algorithm SHA256).Hash.ToLower()
    if (-not $want -or $want.ToLower() -ne $got) { throw "portcheck installer: checksum mismatch for $archive" }
    Expand-Archive (Join-Path $tmp $archive) -DestinationPath (Join-Path $tmp 'x') -Force

    $dir = $env:PORTCHECK_INSTALL_DIR
    if (-not $dir) { $dir = Join-Path $env:LOCALAPPDATA 'Programs\portcheck' }
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $exe = Join-Path $dir 'portcheck.exe'
    Copy-Item (Join-Path $tmp 'x\portcheck.exe') $exe -Force
    Write-Host "Installed $exe"
} finally {
    Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

# Add the folder to the user PATH (new terminals pick it up).
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not (($userPath -split ';') -contains $dir)) {
    [Environment]::SetEnvironmentVariable('Path', ($(if ($userPath) { "$userPath;" } else { '' }) + $dir), 'User')
    Write-Host "Added $dir to your PATH (open a new terminal to use 'portcheck')."
}
$env:Path = "$env:Path;$dir"

& $exe version
if ($env:PORTCHECK_NO_BROWSER -ne '1') {
    Write-Host ''
    & $exe browser install | Out-Null
    if ($LASTEXITCODE -eq 0) {
        & $exe browser status -o plain
        Write-Host ''
        Write-Host 'Done. The portcheck extension connects by itself.'
    } else {
        Write-Host "No supported browser found to connect; run 'portcheck browser install' after installing one."
    }
}
