#!/bin/bash
# Automated Azan - macOS Build Script with Nuitka
# Optimized for macOS (Intel and Apple Silicon)

set -e

echo "🚀 Automated Azan - macOS Build"
echo "================================"

# Color codes for better output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Detect architecture
ARCH=$(uname -m)
if [ "$ARCH" = "arm64" ]; then
    echo -e "${BLUE}🍎 Apple Silicon (M1/M2) detected${NC}"
else
    echo -e "${BLUE}💻 Intel Mac detected${NC}"
fi

# Check if Python is available
if ! command -v python3 &> /dev/null; then
    echo -e "${RED}❌ Python3 not found.${NC}"
    echo "Please install Python 3.11 or later:"
    echo "  - Download from python.org"
    echo "  - Or use Homebrew: brew install python@3.11"
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

# Check for Xcode Command Line Tools
if ! command -v gcc &> /dev/null && ! command -v clang &> /dev/null; then
    echo -e "${RED}❌ No C compiler found!${NC}"
    echo
    echo "Please install Xcode Command Line Tools:"
    echo "  xcode-select --install"
    echo
    echo "Or install Xcode from the App Store"
    exit 1
fi

if command -v clang &> /dev/null; then
    echo -e "${GREEN}✅ Clang compiler found: $(clang --version | head -n1)${NC}"
else
    echo -e "${GREEN}✅ GCC compiler found: $(gcc --version | head -n1)${NC}"
fi

# Create output directory
mkdir -p dist

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
    --macos-app-icon=Media/azan.ico \
    --macos-app-name="Automated Azan" \
    --macos-app-version=1.0.0 \
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
    echo -e "${BLUE}🏗️  Architecture: $ARCH${NC}"

    # Make executable
    chmod +x "dist/AutomatedAzan"

    echo
    echo -e "${BLUE}🧪 Testing executable...${NC}"
    if ./dist/AutomatedAzan --help > /dev/null 2>&1; then
        echo -e "${GREEN}✅ Executable test passed${NC}"
    else
        echo -e "${YELLOW}⚠️  Executable test failed (may be normal)${NC}"
    fi

    # Check if we can create an app bundle
    echo
    echo -e "${BLUE}📦 Creating App Bundle...${NC}"

    # Create .app structure
    APP_NAME="Automated Azan.app"
    mkdir -p "dist/$APP_NAME/Contents/MacOS"
    mkdir -p "dist/$APP_NAME/Contents/Resources"

    # Copy executable
    cp "dist/AutomatedAzan" "dist/$APP_NAME/Contents/MacOS/"

    # Create Info.plist
    cat > "dist/$APP_NAME/Contents/Info.plist" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key>
    <string>Automated Azan</string>
    <key>CFBundleDisplayName</key>
    <string>Automated Azan</string>
    <key>CFBundleIdentifier</key>
    <string>com.automatedazan.app</string>
    <key>CFBundleVersion</key>
    <string>1.0.0</string>
    <key>CFBundleShortVersionString</key>
    <string>1.0.0</string>
    <key>CFBundleExecutable</key>
    <string>AutomatedAzan</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>LSMinimumSystemVersion</key>
    <string>10.15</string>
</dict>
</plist>
EOF

    echo -e "${GREEN}✅ App bundle created: dist/$APP_NAME${NC}"

    echo
    echo -e "${GREEN}🎯 Next steps:${NC}"
    echo "   1. Test: ./dist/AutomatedAzan --no-tray --debug"
    echo "   2. Copy adahn.config.example to adahn.config and configure"
    echo "   3. Test App: open \"dist/$APP_NAME\""
    echo "   4. Distribute both dist/AutomatedAzan and \"dist/$APP_NAME\""

    if [ "$ARCH" = "arm64" ]; then
        echo "   5. Note: Built for Apple Silicon (won't run on Intel Macs)"
    else
        echo "   5. Note: Built for Intel (should run on Apple Silicon via Rosetta)"
    fi
    echo
else
    echo -e "${RED}❌ Build failed - executable not found${NC}"
    exit 1
fi