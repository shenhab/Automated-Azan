#!/bin/bash

# Automated Azan - Development Environment Setup
# Streamlined setup for pipenv development

set -e

echo "🕌 Automated Azan - Development Setup"
echo "===================================="

# Install pipenv if not available
if ! command -v pipenv &> /dev/null; then
    echo "📦 Installing pipenv..."
    pip install --user pipenv || pip3 install --user pipenv
    export PATH="$HOME/.local/bin:$PATH"
fi

echo "✅ pipenv available"

# Install dependencies
echo "📦 Installing dependencies..."
pipenv install --dev

echo "✅ Development environment ready!"
echo ""
echo "🚀 Quick Start:"
echo "   make run    # Run prayer scheduler"
echo "   make web    # Run web interface" 
echo "   make test   # Test the system"
echo ""
echo "🔧 Configuration:"
echo "   cp adahn.config.example adahn.config"
echo "   nano adahn.config"
echo ""

# Check if in correct directory
if [[ ! -f "Pipfile" ]]; then
    echo "❌ Pipfile not found in current directory"
    echo "Please make sure you're in the project root directory"
    exit 1
fi

# Install dependencies
echo "📦 Installing dependencies..."
pipenv install --dev

# Verify installation
echo "🔍 Verifying installation..."
pipenv run python -c "import pychromecast" 2>/dev/null && echo "✅ pychromecast ready" || echo "⚠️  pychromecast issue"
pipenv run python -c "import flask" 2>/dev/null && echo "✅ flask ready" || echo "⚠️  flask issue"

# Create config if missing
if [[ ! -f "adahn.config" ]]; then
    echo ""
    echo "📋 Configuration:"
    if [[ -f "adahn.config.example" ]]; then
        cp adahn.config.example adahn.config
        echo "✅ Created adahn.config from example"
        echo "⚠️  Please edit adahn.config with your settings"
    else
        echo "⚠️  No example configuration found"
    fi
fi

echo ""
echo "🎉 Development environment ready!"
echo ""
echo "🚀 Next steps:"
echo "   1. Edit adahn.config with your speaker name and location"  
echo "   2. Run: make run (prayer scheduler) or make web (web interface)"
echo ""
echo "📱 Web interface will be at: http://localhost:5000"

echo ""
echo "🚀 Ready to develop! Your Automated Azan environment is set up."
