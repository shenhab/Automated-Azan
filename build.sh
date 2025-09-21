#!/bin/bash
# Automated Azan - Universal Build Script
# Automatically detects platform and runs appropriate build

set -e

echo "🚀 Automated Azan - Universal Build Script"
echo "=========================================="

# Detect platform
case "$(uname -s)" in
    Darwin*)
        echo "🍎 macOS detected"
        exec ./build-macos.sh
        ;;
    Linux*)
        echo "🐧 Linux detected"
        exec ./build-linux.sh
        ;;
    CYGWIN*|MINGW*|MSYS*)
        echo "🪟 Windows (Git Bash/MSYS) detected"
        echo "Please run build-windows.bat instead"
        exit 1
        ;;
    *)
        echo "❌ Unsupported platform: $(uname -s)"
        echo "Please use platform-specific build scripts:"
        echo "  - Windows: build-windows.bat"
        echo "  - Linux: build-linux.sh"
        echo "  - macOS: build-macos.sh"
        exit 1
        ;;
esac