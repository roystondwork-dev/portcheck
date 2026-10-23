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

$version = $env:PORTCHECK_VERSION
if (-not $version) {
    $version = (Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest").tag_name
}
$version = $version.TrimStart('v')
$archive = "portcheck_${version}_windows_${arch}.zip"
$base = "https://github.com/$repo/releases/download/v$version"

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("portcheck-" + [Guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    Write-Host "Downloading portcheck $version for windows/$arch..."
    Invoke-WebRequest "$base/$archive" -OutFile (Join-Path $tmp $archive) -UseBasicParsing
    $sums = (Invoke-WebRequest "$base/checksums.txt" -UseBasicParsing).Content
    $want = ($sums -split "`n" | Where-Object { $_ -match "\s$([regex]::Escape($archive))\s*$" } | ForEach-Object { ($_ -split '\s+')[0] }) | Select-Object -First 1
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
