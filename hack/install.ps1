# maek installer for Windows (PowerShell)
$ErrorActionPreference = 'Stop'

$TemplateServer = "__SERVER_URL__"
$TemplateVersion = "__VERSION__"

$BaseUrl = $env:SERVER_URL
if ([string]::IsNullOrEmpty($BaseUrl) -and ($TemplateServer -like "http://*" -or $TemplateServer -like "https://*")) {
    $BaseUrl = $TemplateServer
}

$Ver = $env:VERSION
if ([string]::IsNullOrEmpty($Ver) -and ($TemplateVersion -match '^v?[0-9]')) {
    $Ver = $TemplateVersion
}
if ([string]::IsNullOrEmpty($Ver)) {
    $Ver = "0.1.0"
}
$Ver = $Ver.TrimStart('v')

$TmpDir = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), [System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $TmpDir -Force | Out-Null
$ZipPath = Join-Path $TmpDir "maek_Windows_x86_64.zip"

Write-Host "==> Downloading maek for Windows (x86_64)..." -ForegroundColor Cyan

$DownloadSuccess = $false
if (-not [string]::IsNullOrEmpty($BaseUrl)) {
    $ServerUrl = "$BaseUrl/_maek/download?os=Windows&arch=x86_64"
    try {
        Invoke-WebRequest -Uri $ServerUrl -OutFile $ZipPath -UseBasicParsing -ErrorAction Stop
        $DownloadSuccess = $true
    } catch {
        # Fallback to GitHub
    }
}

if (-not $DownloadSuccess) {
    $GithubUrl = "https://github.com/gosuda/maek/releases/download/v${Ver}/maek_${Ver}_Windows_x86_64.zip"
    Write-Host "==> Fetching from GitHub Releases (v${Ver})..." -ForegroundColor Cyan
    Invoke-WebRequest -Uri $GithubUrl -OutFile $ZipPath -UseBasicParsing
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
