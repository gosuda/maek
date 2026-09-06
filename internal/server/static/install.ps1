# maek installer for Windows (PowerShell)
$ErrorActionPreference = 'Stop'

$TemplateServer = "__SERVER_URL__"

$BaseUrl = $env:SERVER_URL
if ([string]::IsNullOrEmpty($BaseUrl) -and ($TemplateServer -like "http://*" -or $TemplateServer -like "https://*")) {
    $BaseUrl = $TemplateServer
}

if ([string]::IsNullOrEmpty($BaseUrl)) {
    Write-Error "Server URL not determined. Please set `$env:SERVER_URL (e.g. `$env:SERVER_URL='http://localhost:8080') before running this script."
    exit 1
}

$TmpDir = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), [System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $TmpDir -Force | Out-Null
$ZipPath = Join-Path $TmpDir "maek_Windows_x86_64.zip"

Write-Host "==> Downloading maek for Windows (x86_64) from $BaseUrl..." -ForegroundColor Cyan

$DownloadUrl = "$BaseUrl/_maek/download?os=Windows&arch=x86_64"
try {
    Invoke-WebRequest -Uri $DownloadUrl -OutFile $ZipPath -UseBasicParsing -ErrorAction Stop
} catch {
    Write-Error "Failed to download maek from $DownloadUrl: $_"
    Remove-Item -Recurse -Force $TmpDir
    exit 1
}

$InstallDir = "$env:LOCALAPPDATA\Programs\maek"
if (!(Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

Write-Host "==> Extracting to $InstallDir..." -ForegroundColor Cyan
Expand-Archive -Path $ZipPath -DestinationPath $InstallDir -Force
Remove-Item -Recurse -Force $TmpDir

# Add to user environment PATH if not already present
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -notlike "*$InstallDir*") {
    $NewPath = if ([string]::IsNullOrEmpty($UserPath)) { $InstallDir } else { "$UserPath;$InstallDir" }
    [Environment]::SetEnvironmentVariable("Path", $NewPath, "User")
    $env:Path = "$env:Path;$InstallDir"
}

Write-Host ""
Write-Host "✓ maek installed successfully!" -ForegroundColor Green
Write-Host "Location: $InstallDir\maek.exe" -ForegroundColor Green
& "$InstallDir\maek.exe" version
Write-Host ""
Write-Host "You can now run 'maek agent' in your terminal." -ForegroundColor Green
