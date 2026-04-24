# Maintainer: Gede Cahya <cahya@smara.dev>
pkgname=smara
pkgver=1.8.4
pkgrel=1
pkgdesc="Autonomous Multi-Agent Terminal — Terminal pintar yang mengorkestrasi agen AI otonom dengan memori tim tersinkronisasi"
arch=('x86_64' 'aarch64')
url="https://github.com/gede-cahya/Smara-CLI"
license=('MIT')
depends=('sqlite')
makedepends=('go>=1.21' 'git' 'gcc')
optdepends=(
    'ollama: Local LLM provider (default)'
    'nodejs: Required for some MCP servers'
    'python: Required for some MCP servers (uvx)'
)
source=("${pkgname}-${pkgver}.tar.gz::https://github.com/gede-cahya/Smara-CLI/archive/refs/tags/v${pkgver}.tar.gz")
sha256sums=('SKIP')

build() {
    cd "${pkgname}-${pkgver}"
    
    export CGO_ENABLED=1
    export GOFLAGS="-buildmode=pie -trimpath -mod=readonly -modcacherw"
    
    go build -o "${pkgname}" -ldflags "-s -w -X main.version=${pkgver}" ./cmd/smara/
}

check() {
    cd "${pkgname}-${pkgver}"
    go test ./... || true
}

package() {
    cd "${pkgname}-${pkgver}"
    
    # Binary
    install -Dm755 "${pkgname}" "${pkgdir}/usr/bin/${pkgname}"
    
    # License
    if [ -f LICENSE ]; then
        install -Dm644 LICENSE "${pkgdir}/usr/share/licenses/${pkgname}/LICENSE"
    fi
    
    # Shell completions
    mkdir -p "${pkgdir}/usr/share/bash-completion/completions"
    mkdir -p "${pkgdir}/usr/share/zsh/site-functions"
    mkdir -p "${pkgdir}/usr/share/fish/vendor_completions.d"
    
    # Generate completions if binary works
    if ./"${pkgname}" completion bash > /dev/null 2>&1; then
        ./"${pkgname}" completion bash > "${pkgdir}/usr/share/bash-completion/completions/${pkgname}"
        ./"${pkgname}" completion zsh > "${pkgdir}/usr/share/zsh/site-functions/_${pkgname}"
        ./"${pkgname}" completion fish > "${pkgdir}/usr/share/fish/vendor_completions.d/${pkgname}.fish"
    fi
    
    # Documentation
    if [ -f README.md ]; then
        install -Dm644 README.md "${pkgdir}/usr/share/doc/${pkgname}/README.md"
    fi
}
