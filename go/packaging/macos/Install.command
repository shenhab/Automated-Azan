#!/bin/bash
# Automated Azan __VERSION__ — Installer
# Double-click this file from the DMG to install without any terminal commands.

set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
APP_SRC="$SCRIPT_DIR/AzanAgent.app"
APP_DST="/Applications/AzanAgent.app"

echo "========================================"
echo "  Automated Azan __VERSION__ Installer"
echo "========================================"
echo ""

if [ ! -d "$APP_SRC" ]; then
    echo "Error: AzanAgent.app not found in the same folder as this script."
    echo "Please re-download the DMG and try again."
    exit 1
fi

echo "→ Removing macOS quarantine flag..."
xattr -rd com.apple.quarantine "$APP_SRC" 2>/dev/null || true

echo "→ Copying to /Applications..."
rm -rf "$APP_DST"
cp -r "$APP_SRC" "$APP_DST"
xattr -rd com.apple.quarantine "$APP_DST" 2>/dev/null || true

echo "→ Launching AzanAgent..."
open "$APP_DST"

echo ""
echo "✓ Done! AzanAgent is installed and running."
echo "  Look for its icon in your menu bar (top-right of screen)."
echo "  It will start automatically on every login."
echo ""
echo "  Dashboard: http://localhost:28426"
echo ""
read -p "Press Enter to close this window..."
