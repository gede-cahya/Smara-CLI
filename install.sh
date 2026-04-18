#!/bin/sh
# Smara CLI Installer
# Usage: curl -fsSL https://raw.githubusercontent.com/gede-cahya/Smara-CLI/main/install.sh | sh
#
# Supports: Linux (amd64, arm64), macOS (amd64, arm64)

set -e

# Configuration
REPO="gede-cahya/Smara-CLI"
BINARY_NAME="smara"
INSTALL_DIR="/usr/local/bin"
VERSION="1.1.0"
GITHUB_BASE="https://github.com/${REPO}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

info() { printf "  ${CYAN}▸${NC} %s\n" "$1"; }
success() { printf "  ${GREEN}✓${NC} %s\n" "$1"; }
warn() { printf "  ${YELLOW}⚠${NC} %s\n" "$1"; }
error() { printf "  ${RED}✗${NC} %s\n" "$1"; exit 1; }

# Banner
printf "\n${CYAN}${BOLD}"
cat << 'EOF'
  ███████╗███╗   ███╗ █████╗ ██████╗  █████╗ 
  ██╔════╝████╗ ████║██╔══██╗██╔══██╗██╔══██╗
  ███████╗██╔████╔██║███████║██████╔╝███████║
  ╚════██║██║╚██╔╝██║██╔══██║██╔══██╗██╔══██║
  ███████║██║ ╚═╝ ██║██║  ██║██║  ██║██║  ██║
  ╚══════╝╚═╝     ╚═╝╚═╝  ╚═╝╚═╝  ╚═╝╚═╝  ╚═╝
EOF
printf "${NC}\n"
info "Smara CLI Installer v${VERSION}"
printf "\n"

# Detect OS
detect_os() {
    OS="$(uname -s)"
    case "${OS}" in
        Linux*)     OS="linux";;
        Darwin*)    OS="darwin";;
        CYGWIN*|MINGW*|MSYS*) OS="windows";;
        *)          error "OS tidak didukung: ${OS}";;
    esac
    echo "${OS}"
}

# Detect architecture
detect_arch() {
    ARCH="$(uname -m)"
    case "${ARCH}" in
        x86_64|amd64)   ARCH="amd64";;
        aarch64|arm64)  ARCH="arm64";;
        armv7l)         ARCH="arm";;
        *)              error "Arsitektur tidak didukung: ${ARCH}";;
    esac
    echo "${ARCH}"
}

# Check for required tools
check_deps() {
    for cmd in curl tar; do
        if ! command -v "$cmd" > /dev/null 2>&1; then
            error "Dibutuhkan: ${cmd}. Install dulu lalu coba lagi."
        fi
    done
}

# Download and install
install() {
    OS=$(detect_os)
    ARCH=$(detect_arch)
    
    info "Platform: ${OS}/${ARCH}"
    
    check_deps

    # Construct download URL
    FILENAME="${BINARY_NAME}-${VERSION}-${OS}-${ARCH}"
    if [ "${OS}" = "windows" ]; then
        FILENAME="${FILENAME}.zip"
    else
        FILENAME="${FILENAME}.tar.gz"
    fi
    DOWNLOAD_URL="${GITHUB_BASE}/releases/download/v${VERSION}/${FILENAME}"

    # Create temp directory
    TMP_DIR=$(mktemp -d)
    trap 'rm -rf "${TMP_DIR}"' EXIT

    # Download
    info "Mengunduh ${BINARY_NAME} v${VERSION}..."
    if ! curl -fsSL "${DOWNLOAD_URL}" -o "${TMP_DIR}/${FILENAME}" 2>/dev/null; then
        # If release not found, try building from source
        warn "Release binary tidak ditemukan. Mencoba build dari source..."
        install_from_source
        return
    fi

    # Extract
    info "Mengekstrak..."
    cd "${TMP_DIR}"
    if [ "${OS}" = "windows" ]; then
        unzip -q "${FILENAME}"
    else
        tar -xzf "${FILENAME}"
    fi

    # Install
    info "Memasang ke ${INSTALL_DIR}..."
    if [ -w "${INSTALL_DIR}" ]; then
        cp "${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
        chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    else
        sudo cp "${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
        sudo chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    fi

    success "Smara v${VERSION} berhasil diinstall!"
    printf "\n"
    info "Jalankan: ${BOLD}smara start${NC}"
    printf "\n"
}

# Build from source as fallback
install_from_source() {
    # Check for Go
    if ! command -v go > /dev/null 2>&1; then
        error "Go tidak ditemukan. Install Go 1.21+ terlebih dahulu: https://go.dev/dl/"
    fi

    GO_VERSION=$(go version | grep -oP 'go\K[0-9]+\.[0-9]+')
    info "Go ${GO_VERSION} ditemukan"

    # Clone and build
    info "Mengkloning repository..."
    git clone --depth 1 "${GITHUB_BASE}.git" "${TMP_DIR}/smara" 2>/dev/null || \
    git clone --depth 1 "https://github.com/${REPO}.git" "${TMP_DIR}/smara" 2>/dev/null

    cd "${TMP_DIR}/smara"
    
    info "Mengkompilasi..."
    CGO_ENABLED=1 go build -o "${BINARY_NAME}" ./cmd/smara/

    # Install
    info "Memasang ke ${INSTALL_DIR}..."
    if [ -w "${INSTALL_DIR}" ]; then
        cp "${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
        chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    else
        sudo cp "${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
        sudo chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    fi

    success "Smara v${VERSION} berhasil diinstall dari source!"
    printf "\n"
    info "Jalankan: ${BOLD}smara start${NC}"
    printf "\n"
}

# Check if already installed
if command -v smara > /dev/null 2>&1; then
    CURRENT=$(smara version 2>/dev/null | grep -oP 'v\K[0-9.]+' || echo "unknown")
    warn "Smara v${CURRENT} sudah terinstall. Mengupdate ke v${VERSION}..."
fi

install
