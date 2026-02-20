param(
    [string]$Repo = $env:ISS_REPO,
    [string]$Version = $env:ISS_VERSION,
    [string]$InstallDir = $env:ISS_INSTALL_DIR
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

if ([string]::IsNullOrWhiteSpace($Repo)) {
    $Repo = "kivayan/iss"
}

if ([string]::IsNullOrWhiteSpace($Version)) {
    $Version = "latest"
}

if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    $InstallDir = Join-Path $env:LOCALAPPDATA "Programs\iss"
}

if (-not [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Windows)) {
    throw "This installer supports Windows only."
}

$archValue = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
switch ($archValue) {
    "X64" { $arch = "amd64" }
    "Arm64" { $arch = "arm64" }
    default { throw "Unsupported architecture: $archValue" }
}

if ($Version -ne "latest" -and -not $Version.StartsWith("v")) {
    $Version = "v$Version"
}

$assetName = "iss_windows_$arch.zip"
$checksumsName = "checksums.txt"

if ($Version -eq "latest") {
    $baseUrl = "https://github.com/$Repo/releases/latest/download"
}
else {
    $baseUrl = "https://github.com/$Repo/releases/download/$Version"
}

$tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ("iss-install-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tmpDir | Out-Null

try {
    $archivePath = Join-Path $tmpDir $assetName
    $checksumsPath = Join-Path $tmpDir $checksumsName

    Write-Host "Downloading $assetName from $Repo"
    Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/$assetName" -OutFile $archivePath
    Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/$checksumsName" -OutFile $checksumsPath

    $checksumLine = Get-Content $checksumsPath | Where-Object { $_ -match ("\s" + [Regex]::Escape($assetName) + "$") } | Select-Object -First 1
    if (-not $checksumLine) {
        throw "Checksum for $assetName was not found in $checksumsName"
    }

    $expectedChecksum = (($checksumLine -split "\s+")[0]).ToLowerInvariant()
    $actualChecksum = (Get-FileHash -Path $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($expectedChecksum -ne $actualChecksum) {
        throw "Checksum mismatch for $assetName"
    }

    $extractDir = Join-Path $tmpDir "extract"
    Expand-Archive -Path $archivePath -DestinationPath $extractDir -Force

    $binaryPath = Join-Path $extractDir "iss.exe"
    if (-not (Test-Path $binaryPath)) {
        throw "Expected binary iss.exe was not found in release archive"
    }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $targetPath = Join-Path $InstallDir "iss.exe"
    Copy-Item -Path $binaryPath -Destination $targetPath -Force

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($null -eq $userPath) {
        $userPath = ""
    }

    $normalizedInstallDir = $InstallDir.TrimEnd('\\')
    $pathEntries = $userPath -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
    $alreadyInPath = $false

    foreach ($entry in $pathEntries) {
        if ([string]::Equals($entry.TrimEnd('\\'), $normalizedInstallDir, [StringComparison]::OrdinalIgnoreCase)) {
            $alreadyInPath = $true
            break
        }
    }

    if (-not $alreadyInPath) {
        $newUserPath = if ([string]::IsNullOrWhiteSpace($userPath)) { $InstallDir } else { "$userPath;$InstallDir" }
        [Environment]::SetEnvironmentVariable("Path", $newUserPath, "User")
        $env:Path = "$env:Path;$InstallDir"
        Write-Host "Added $InstallDir to your user PATH."
        Write-Host "Open a new PowerShell window before using iss."
    }

    Write-Host "Installed iss to $targetPath"
    Write-Host "Done. Run 'iss --help' to verify."
}
finally {
    Remove-Item -Path $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
}
