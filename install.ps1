$ErrorActionPreference = "Stop"

$Repo = if ($env:WEECO_REPO) { $env:WEECO_REPO } else { "zhangyingwei/weeco" }
$Version = if ($env:WEECO_VERSION) { $env:WEECO_VERSION } else { "0.12.2" }
$Version = $Version -replace '^v', ''
$BinDir = if ($env:WEECO_BIN_DIR) { $env:WEECO_BIN_DIR } else { Join-Path $HOME ".local\bin" }
$BaseUrl = if ($env:WEECO_BASE_URL) { $env:WEECO_BASE_URL.TrimEnd("/") } else { "https://github.com/$Repo/releases/download" }

function Fail([string]$Message) {
    Write-Error "weeco install: $Message"
    exit 1
}

if ($Version -eq "latest") {
    try {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing
        $Version = $release.tag_name -replace '^v', ''
    } catch {
        Fail "could not resolve the latest release: $($_.Exception.Message)"
    }
}
if (-not $Version) {
    Fail "release version is empty"
}

$arch = $env:PROCESSOR_ARCHITECTURE
if ($env:PROCESSOR_ARCHITEW6432) {
    $arch = $env:PROCESSOR_ARCHITEW6432
}
if (-not $arch) {
    $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
}
switch ($arch.ToUpperInvariant()) {
    "AMD64" { $archName = "amd64" }
    "X64" { $archName = "amd64" }
    "ARM64" { $archName = "arm64" }
    default { Fail "unsupported architecture: $arch" }
}

$asset = "weeco-$Version-windows-$archName.zip"
$releaseUrl = "$BaseUrl/v$Version"
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("weeco-install-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tmp | Out-Null

try {
    $sumsPath = Join-Path $tmp "SHA256SUMS"
    $zipPath = Join-Path $tmp $asset
    Write-Host "weeco install: downloading $asset"
    Invoke-WebRequest -Uri "$releaseUrl/SHA256SUMS" -OutFile $sumsPath -UseBasicParsing
    Invoke-WebRequest -Uri "$releaseUrl/$asset" -OutFile $zipPath -UseBasicParsing

    $line = Get-Content $sumsPath | Where-Object { ($_ -split '\s+')[1] -eq $asset } | Select-Object -First 1
    if (-not $line) {
        Fail "SHA256SUMS has no entry for $asset"
    }
    $expected = ($line -split '\s+')[0].ToLowerInvariant()
    $actual = (Get-FileHash -Algorithm SHA256 -Path $zipPath).Hash.ToLowerInvariant()
    if ($actual -ne $expected) {
        Fail "checksum mismatch for $asset"
    }

    Expand-Archive -Path $zipPath -DestinationPath $tmp -Force
    $source = Join-Path $tmp "weeco.exe"
    if (-not (Test-Path $source)) {
        Fail "release archive did not contain weeco.exe"
    }
    New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
    $target = Join-Path $BinDir "weeco.exe"
    Copy-Item -Force $source $target

    & $target install --bin-dir $BinDir
    if ($LASTEXITCODE -ne 0) {
        Fail "weeco install --bin-dir failed"
    }

    Write-Host "weeco install: binary $target"
    Write-Host "weeco install: version $(& $target version)"

    $missing = $false
    foreach ($dep in @("herdr", "claude")) {
        $cmd = Get-Command $dep -ErrorAction SilentlyContinue
        if ($cmd) {
            Write-Host "weeco install: found $dep"
        } else {
            Write-Warning "weeco install: $dep is not on PATH"
            $missing = $true
        }
    }

    $pathParts = $env:Path -split ';'
    if ($pathParts -notcontains $BinDir) {
        Write-Host "weeco install: add $BinDir to PATH before invoking weeco"
    }

    if ($missing) {
        Write-Warning "install Herdr and Claude Code, complete their login, then run /weeco in a Herdr pane"
        if ($env:WEECO_REQUIRE_DEPS -eq "1") {
            exit 1
        }
    }

    Write-Host "weeco install: done"
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
