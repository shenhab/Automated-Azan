#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────────────────────────────────────
# Automated Azan — build script
# Compiles the Go agent for every supported platform.
# Run from the go/ directory:  ./build.sh
# ─────────────────────────────────────────────────────────────────────────────

GO_VERSION="1.22.4"
BINARY="azan-agent"
CMD="./cmd/azan-agent"
DIST="dist"

# ── colours ──────────────────────────────────────────────────────────────────
GREEN="\033[0;32m"; YELLOW="\033[1;33m"; RED="\033[0;31m"; NC="\033[0m"
info()    { echo -e "${GREEN}[build]${NC} $*"; }
warn()    { echo -e "${YELLOW}[build]${NC} $*"; }
error()   { echo -e "${RED}[build]${NC} $*" >&2; exit 1; }

# ── ensure we're in the go/ directory ────────────────────────────────────────
cd "$(dirname "$0")"

# ── 1. install Go if missing ──────────────────────────────────────────────────
install_go() {
  info "Go not found — installing Go ${GO_VERSION}..."

  OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
  ARCH="$(uname -m)"
  case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    armv*)   ARCH="armv6l" ;;
    *)       error "Unsupported architecture: $ARCH" ;;
  esac

  TARBALL="go${GO_VERSION}.${OS}-${ARCH}.tar.gz"
  URL="https://go.dev/dl/${TARBALL}"
  TMP="$(mktemp -d)"

  info "Downloading ${URL}..."
  curl -fsSL "$URL" -o "${TMP}/${TARBALL}"

  info "Extracting to /usr/local/go..."
  if [ "$OS" = "darwin" ]; then
    sudo tar -C /usr/local -xzf "${TMP}/${TARBALL}"
  else
    tar -C /usr/local -xzf "${TMP}/${TARBALL}" 2>/dev/null || \
      sudo tar -C /usr/local -xzf "${TMP}/${TARBALL}"
  fi

  rm -rf "$TMP"
  export PATH="$PATH:/usr/local/go/bin"
  info "Go $(go version) installed."
}

if ! command -v go &>/dev/null; then
  install_go
else
  export PATH="$PATH:/usr/local/go/bin"
  info "Go found: $(go version)"
fi

# ── 2. install Linux tray dependencies (for native CGO build only) ───────────
if [ "$(uname -s)" = "Linux" ] && command -v apt-get &>/dev/null; then
  if ! pkg-config --exists gtk+-3.0 2>/dev/null; then
    info "Installing GTK3 / appindicator (required for system tray)..."
    sudo apt-get install -y pkg-config libgtk-3-dev libayatana-appindicator3-dev \
      >/dev/null 2>&1 || warn "GTK install failed — tray disabled in native build"
  fi
fi

# ── 3. download / tidy dependencies ──────────────────────────────────────────
info "Tidying dependencies..."
go mod tidy

# ── 4. prepare output directory ──────────────────────────────────────────────
mkdir -p "$DIST"

# ── 5. build targets ─────────────────────────────────────────────────────────
build() {
  local GOOS="$1" GOARCH="$2" OUT="$3" CGO="${4:-0}"
  printf "  %-40s" "${GOOS}/${GOARCH} → ${OUT}"
  if CGO_ENABLED="$CGO" GOOS="$GOOS" GOARCH="$GOARCH" \
      go build -ldflags="-s -w" -o "${DIST}/${OUT}" "$CMD" 2>/tmp/build_err; then
    SIZE="$(du -sh "${DIST}/${OUT}" | cut -f1)"
    echo -e "  ${GREEN}✓${NC}  ${SIZE}"
  else
    echo -e "  ${RED}✗${NC}"
    cat /tmp/build_err >&2
  fi
}

echo ""
info "Building all targets..."
echo ""

build linux   amd64  "${BINARY}-linux-amd64"
build linux   arm64  "${BINARY}-linux-arm64"
build linux   arm    "${BINARY}-linux-arm"
build windows amd64  "${BINARY}-windows-amd64.exe"
build darwin  amd64  "${BINARY}-darwin-amd64"
build darwin  arm64  "${BINARY}-darwin-arm64"

# ── 6. native build with CGO (tray support) ───────────────────────────────────
echo ""
info "Building native binary with CGO (system tray enabled)..."
NATIVE_OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
NATIVE_ARCH="$(uname -m)"
case "$NATIVE_ARCH" in x86_64) NATIVE_ARCH="amd64" ;; aarch64) NATIVE_ARCH="arm64" ;; esac
NATIVE_OUT="${BINARY}-${NATIVE_OS}-${NATIVE_ARCH}-tray"
build "$NATIVE_OS" "$NATIVE_ARCH" "$NATIVE_OUT" 1

# ── 7. summary ────────────────────────────────────────────────────────────────
echo ""
info "Output directory: ${DIST}/"
ls -lh "${DIST}/" | tail -n +2 | awk '{printf "  %-45s %s\n", $NF, $5}'
echo ""
info "Done. Dashboard will be available at http://localhost:28426"
