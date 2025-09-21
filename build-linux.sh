#!/bin/bash
# Automated Azan - Linux Build Script with Nuitka
# Optimized for Linux x64 (Ubuntu, Debian, CentOS, etc.)

set -e

echo "🚀 Automated Azan - Linux Build"
echo "================================"

# Color codes for better output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Check if Python is available
if ! command -v python3 &> /dev/null; then
    echo -e "${RED}❌ Python3 not found. Please install Python 3.11 or later.${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Python $(python3 --version) found${NC}"

# Check if UV is available
if ! command -v uv &> /dev/null; then
    echo -e "${YELLOW}📦 Installing UV...${NC}"
    pip3 install --user uv
    export PATH="$HOME/.local/bin:$PATH"
fi

echo -e "${GREEN}✅ UV $(uv --version) found${NC}"

# Install build dependencies
echo -e "${BLUE}📦 Installing build dependencies...${NC}"
uv pip install -e ".[build,gui]"

# Check for C compiler
if command -v gcc &> /dev/null; then
    echo -e "${GREEN}✅ GCC compiler found: $(gcc --version | head -n1)${NC}"
elif command -v clang &> /dev/null; then
    echo -e "${GREEN}✅ Clang compiler found: $(clang --version | head -n1)${NC}"
else
    echo -e "${RED}❌ No C compiler found!${NC}"
    echo
    echo "Please install build tools:"
    if command -v apt &> /dev/null; then
        echo "  sudo apt update && sudo apt install build-essential"
    elif command -v yum &> /dev/null; then
        echo "  sudo yum groupinstall 'Development Tools'"
    elif command -v dnf &> /dev/null; then
        echo "  sudo dnf groupinstall 'Development Tools'"
    elif command -v pacman &> /dev/null; then
        echo "  sudo pacman -S base-devel"
    else
        echo "  Install GCC or Clang for your distribution"
    fi
    exit 1
fi

# Create output directory
mkdir -p dist

# Check for GUI environment
if [ -n "$DISPLAY" ] || [ -n "$WAYLAND_DISPLAY" ]; then
    echo -e "${GREEN}✅ GUI environment detected${NC}"
    GUI_AVAILABLE=true
else
    echo -e "${YELLOW}⚠️  No GUI environment detected (headless mode)${NC}"
    GUI_AVAILABLE=false
fi

# Run Nuitka build
echo -e "${BLUE}🔨 Compiling with Nuitka...${NC}"
echo "This will take 5-15 minutes..."

uv run python -m nuitka \
    --standalone \
    --onefile \
    --assume-yes-for-downloads \
    --lto=yes \
    --enable-plugin=anti-bloat \
    --include-data-dir=Media=Media \
    --include-data-file=adahn.config.example=adahn.config.example \
    --include-package=flask \
    --include-package=flask_socketio \
    --include-package=pychromecast \
    --include-package=schedule \
    --include-package=requests \
    --include-package=beautifulsoup4 \
    --include-package=python_dateutil \
    --include-package=dotenv \
    --include-package-data=pystray \
    --include-package-data=PIL \
    --output-filename=AutomatedAzan \
    --output-dir=dist \
    main.py

# Check if build succeeded
if [ -f "dist/AutomatedAzan" ]; then
    echo
    echo -e "${GREEN}🎉 Build successful!${NC}"
    echo -e "${BLUE}📦 Executable: dist/AutomatedAzan${NC}"

    # Get file size
    size=$(du -h "dist/AutomatedAzan" | cut -f1)
    echo -e "${BLUE}💾 Size: $size${NC}"

    # Make executable
    chmod +x "dist/AutomatedAzan"

    echo
    echo -e "${BLUE}🧪 Testing executable...${NC}"
    if ./dist/AutomatedAzan --help > /dev/null 2>&1; then
        echo -e "${GREEN}✅ Executable test passed${NC}"
    else
        echo -e "${YELLOW}⚠️  Executable test failed (may be normal)${NC}"
    fi

    echo
    echo -e "${GREEN}🎯 Next steps:${NC}"
    echo "   1. Test: ./dist/AutomatedAzan --no-tray --debug"
    echo "   2. Copy adahn.config.example to adahn.config and configure"
    echo "   3. Distribute dist/AutomatedAzan to users"

    if [ "$GUI_AVAILABLE" = true ]; then
        echo "   4. Test GUI: ./dist/AutomatedAzan"
    else
        echo "   4. For GUI systems, rebuild on a machine with DISPLAY"
    fi
    echo
else
    echo -e "${RED}❌ Build failed - executable not found${NC}"
    exit 1
fi