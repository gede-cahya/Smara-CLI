# Smara CLI Installer for Windows (PowerShell)
# Usage: irm https://raw.githubusercontent.com/gede-cahya/Smara-CLI/main/install.ps1 | iex

$ErrorActionPreference = "Stop"

# Configuration
$REPO = "gede-cahya/Smara-CLI"
$BINARY_NAME = "smara"
$VERSION = "1.4.0"
$INSTALL_DIR = "$env:LOCALAPPDATA\Programs\smara"
$GITHUB_BASE = "https://github.com/$REPO"

function Write-Info($msg) { Write-Host "  ▸ $msg" -ForegroundColor Cyan }
function Write-Ok($msg) { Write-Host "  ✓ $msg" -ForegroundColor Green }
function Write-Warn($msg) { Write-Host "  ⚠ $msg" -ForegroundColor Yellow }
function Write-Err($msg) { Write-Host "  ✗ $msg" -ForegroundColor Red; exit 1 }

# Banner
Write-Host ""
Write-Host "  ███████╗███╗   ███╗ █████╗ ██████╗  █████╗ " -ForegroundColor Cyan
Write-Host "  ██╔════╝████╗ ████║██╔══██╗██╔══██╗██╔══██╗" -ForegroundColor Cyan
Write-Host "  ███████╗██╔████╔██║███████║██████╔╝███████║" -ForegroundColor Cyan
Write-Host "  ╚════██║██║╚██╔╝██║██╔══██║██╔══██╗██╔══██║" -ForegroundColor Cyan
Write-Host "  ███████║██║ ╚═╝ ██║██║  ██║██║  ██║██║  ██║" -ForegroundColor Cyan
Write-Host "  ╚══════╝╚═╝     ╚═╝╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═╝" -ForegroundColor Cyan
Write-Host ""
Write-Info "Smara CLI Installer v$VERSION"
Write-Host ""

# Detect architecture
$ARCH = if ([System.Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }
if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { $ARCH = "arm64" }

Write-Info "Platform: windows/$ARCH"

# Create install directory
if (-not (Test-Path $INSTALL_DIR)) {
    New-Item -ItemType Directory -Path $INSTALL_DIR -Force | Out-Null
}

# Download URL
$FILENAME = "$BINARY_NAME-$VERSION-windows-$ARCH.zip"
$DOWNLOAD_URL = "$GITHUB_BASE/releases/download/v$VERSION/$FILENAME"
$TMP_DIR = Join-Path $env:TEMP "smara-install"

if (Test-Path $TMP_DIR) { Remove-Item -Recurse -Force $TMP_DIR }
New-Item -ItemType Directory -Path $TMP_DIR -Force | Out-Null

Write-Info "Mengunduh $BINARY_NAME v$VERSION..."

try {
    Invoke-WebRequest -Uri $DOWNLOAD_URL -OutFile "$TMP_DIR\$FILENAME" -UseBasicParsing
} catch {
    Write-Warn "Release binary tidak ditemukan. Mencoba build dari source..."
    
    # Check for Go
    $goCmd = Get-Command go -ErrorAction SilentlyContinue
    if (-not $goCmd) {
        Write-Err "Go tidak ditemukan. Install Go 1.21+ dari https://go.dev/dl/"
    }
    
    Write-Info "Mengkloning repository..."
    git clone --depth 1 "$GITHUB_BASE.git" "$TMP_DIR\smara" 2>&1 | Out-Null
    
    Set-Location "$TMP_DIR\smara"
    Write-Info "Mengkompilasi..."
    $env:CGO_ENABLED = "1"
    go build -o "$INSTALL_DIR\smara.exe" ./cmd/smara/
    
    # Add to PATH
    $currentPath = [System.Environment]::GetEnvironmentVariable("Path", "User")
    if ($currentPath -notlike "*$INSTALL_DIR*") {
        [System.Environment]::SetEnvironmentVariable("Path", "$currentPath;$INSTALL_DIR", "User")
        $env:Path = "$env:Path;$INSTALL_DIR"
        Write-Ok "Ditambahkan ke PATH: $INSTALL_DIR"
    }
    
    Write-Ok "Smara v$VERSION berhasil diinstall dari source!"
    Write-Host ""
    Write-Info "Buka terminal baru, lalu jalankan: smara start"
    Write-Host ""
    
    # Cleanup
    Set-Location $env:USERPROFILE
    Remove-Item -Recurse -Force $TMP_DIR -ErrorAction SilentlyContinue
    return
}

# Extract
Write-Info "Mengekstrak..."
Expand-Archive -Path "$TMP_DIR\$FILENAME" -DestinationPath $TMP_DIR -Force

# Install
Write-Info "Memasang ke $INSTALL_DIR..."
Copy-Item "$TMP_DIR\$BINARY_NAME.exe" "$INSTALL_DIR\$BINARY_NAME.exe" -Force

# Add to PATH if not already there
$currentPath = [System.Environment]::GetEnvironmentVariable("Path", "User")
if ($currentPath -notlike "*$INSTALL_DIR*") {
    [System.Environment]::SetEnvironmentVariable("Path", "$currentPath;$INSTALL_DIR", "User")
    $env:Path = "$env:Path;$INSTALL_DIR"
    Write-Ok "Ditambahkan ke PATH: $INSTALL_DIR"
}

# Cleanup
Remove-Item -Recurse -Force $TMP_DIR -ErrorAction SilentlyContinue

Write-Ok "Smara v$VERSION berhasil diinstall!"
Write-Host ""
Write-Info "Buka terminal baru, lalu jalankan: smara start"
Write-Host ""
