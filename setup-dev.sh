#!/bin/bash

# Automated Azan - Development Environment Setup with Pipenv
# This script sets up a development environment using pipenv

set -e

echo "🕌 Automated Azan - Development Environment Setup"
echo "================================================"

# Check if pipenv is installed
if ! command -v pipenv &> /dev/null; then
    echo "❌ pipenv is not installed"
    echo "Installing pipenv..."
    pip install --user pipenv
    
    # Add pipenv to PATH if not already there
    export PATH="$HOME/.local/bin:$PATH"
    
    if ! command -v pipenv &> /dev/null; then
        echo "❌ Failed to install pipenv. Please install it manually:"
        echo "   pip install pipenv"
        echo "   or follow instructions at: https://pipenv.pypa.io/en/latest/install/"
        exit 1
    fi
fi

echo "✅ pipenv is available"

# Check if Pipfile exists
if [[ ! -f "Pipfile" ]]; then
    echo "❌ Pipfile not found in current directory"
    echo "Please make sure you're in the project root directory"
    exit 1
fi

echo "📦 Installing dependencies with pipenv..."
pipenv install --dev

echo "🔍 Verifying installation..."
pipenv run python -c "import pychromecast; print('✅ pychromecast imported successfully')"
pipenv run python -c "import flask; print('✅ flask imported successfully')"
pipenv run python -c "import requests; print('✅ requests imported successfully')"

echo ""
echo "🎉 Development environment setup completed!"
echo "=========================================="
echo ""
echo "To activate the virtual environment:"
echo "  pipenv shell"
echo ""
echo "To run the application:"
echo "  pipenv run python main.py"
echo ""
echo "To run the web interface:"
echo "  pipenv run python web_interface.py"
echo ""
echo "To run commands in the virtual environment:"
echo "  pipenv run <command>"
echo ""
echo "To check dependencies:"
echo "  pipenv graph"
echo ""
echo "To update dependencies:"
echo "  pipenv update"

# Check configuration file
echo ""
echo "📋 Configuration Check:"
if [[ -f "adahn.config" ]]; then
    echo "✅ Configuration file found"
else
    echo "⚠️  Configuration file not found"
    echo "Creating sample configuration..."
    
    read -p "Enter your Chromecast device name (default: Adahn): " DEVICE_NAME
    read -p "Enter your location (icci or naas, default: icci): " LOCATION
    
    DEVICE_NAME=${DEVICE_NAME:-Adahn}
    LOCATION=${LOCATION:-icci}
    
    cat > adahn.config << EOF
[Settings]
speakers-group-name = ${DEVICE_NAME}
location = ${LOCATION}
EOF
    
    echo "✅ Configuration file created with your settings"
fi

echo ""
echo "🚀 Ready to develop! Your Automated Azan environment is set up."
