#!/usr/bin/env bash
# deploy.sh — Build the headless Go binary, package it into a Docker image,
# and replace the running 'athan' container on this machine.
#
# Usage:  ./go/scripts/deploy.sh [--image IMAGE] [--tz TIMEZONE]
#
# Re-run this script any time you want to ship a new build.
set -euo pipefail

# ── Defaults ──────────────────────────────────────────────────────────────────
IMAGE="shenhab/athan:latest"
CONTAINER="athan"
TZ_VALUE="Europe/Dublin"
# --network host   → required for Chromecast mDNS discovery (no port mappings shown)
# --network bridge → proper isolation with explicit port mappings, but mDNS breaks
NETWORK_MODE="host"
AUTH_USER=""
AUTH_PASS=""

# ── Argument parsing ──────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --image)    IMAGE="$2"; shift 2 ;;
    --tz)       TZ_VALUE="$2"; shift 2 ;;
    --bridge)   NETWORK_MODE="bridge" ;;
    --host)     NETWORK_MODE="host" ;;
    --username) AUTH_USER="$2"; shift 2 ;;
    --password) AUTH_PASS="$2"; shift 2 ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

# ── Resolve paths ─────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# ── Colours ───────────────────────────────────────────────────────────────────
GREEN="\033[0;32m"; YELLOW="\033[1;33m"; RED="\033[0;31m"; NC="\033[0m"
step()  { echo -e "${GREEN}==>${NC} $*"; }
warn()  { echo -e "${YELLOW}[warn]${NC} $*"; }
error() { echo -e "${RED}[error]${NC} $*" >&2; exit 1; }

# ── 1. Compile ────────────────────────────────────────────────────────────────
step "Compiling headless Go binary (CGO_ENABLED=0, linux/amd64)..."
cd "$GO_DIR"

# Prefer the system Go that was used to build previously; fall back to PATH.
GO_BIN="${GO_BIN:-$(command -v go 2>/dev/null || echo /usr/local/go/bin/go)}"
[[ -x "$GO_BIN" ]] || error "Go not found. Set GO_BIN or install Go."

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  "$GO_BIN" build -ldflags="-s -w" -o azan-agent ./cmd/azan-agent

echo "  Binary: $(du -sh azan-agent | cut -f1)  $(pwd)/azan-agent"

# ── 2. Build Docker image ─────────────────────────────────────────────────────
step "Building Docker image  $IMAGE ..."
docker build -f Dockerfile.headless -t "$IMAGE" .
echo "  Image built."

# ── 3. Stop & remove the old container (only after a successful build) ────────
if docker inspect "$CONTAINER" >/dev/null 2>&1; then
  step "Stopping container '$CONTAINER'..."
  docker stop "$CONTAINER" || warn "Container was not running."
  docker rm   "$CONTAINER"
  echo "  Old container removed."
else
  warn "No existing container named '$CONTAINER' — starting fresh."
fi

# ── 4. Start the new container ────────────────────────────────────────────────
step "Starting new container '$CONTAINER'..."

# Build the auth env args (only passed on first run to seed credentials)
AUTH_ARGS=()
if [[ -n "$AUTH_USER" && -n "$AUTH_PASS" ]]; then
  AUTH_ARGS=(-e "AZAN_USERNAME=${AUTH_USER}" -e "AZAN_PASSWORD=${AUTH_PASS}")
  echo "  Auth credentials will be seeded for user: ${AUTH_USER}"
fi

if [[ "$NETWORK_MODE" == "host" ]]; then
  # Host networking: Chromecast mDNS discovery works, but no port mappings shown in docker ps.
  # The app binds directly to host ports 28426 (dashboard) and 28427 (media server).
  docker run -d \
    --name "$CONTAINER" \
    --network host \
    --restart unless-stopped \
    -e TZ="$TZ_VALUE" \
    "${AUTH_ARGS[@]+"${AUTH_ARGS[@]}"}" \
    -v azan_config:/config \
    -v azan_data:/root/.local/share/azan-agent \
    "$IMAGE"
else
  # Bridge networking: explicit port mappings, proper isolation.
  # WARNING: Chromecast mDNS discovery will not work in this mode.
  warn "Bridge networking selected — Chromecast mDNS discovery will not work."
  docker run -d \
    --name "$CONTAINER" \
    --restart unless-stopped \
    -e TZ="$TZ_VALUE" \
    "${AUTH_ARGS[@]+"${AUTH_ARGS[@]}"}" \
    -p 28426:28426 \
    -p 28427:28427 \
    -v azan_config:/config \
    -v azan_data:/root/.local/share/azan-agent \
    "$IMAGE"
fi

# ── 5. Summary ────────────────────────────────────────────────────────────────
HOST_IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
HOST_IP="${HOST_IP:-localhost}"

echo ""
echo -e "${GREEN}✓ Deployed successfully.${NC}"
echo ""
docker ps --filter "name=$CONTAINER" --format "  {{.Names}}  {{.Status}}  {{.Image}}"
echo ""
echo "  Dashboard → http://${HOST_IP}:28426"
echo "  Logs      → docker logs -f $CONTAINER"
