#!/bin/bash

# Streamlined setup for uv development
echo "🚀 Setting up Automated Azan development environment with uv..."

# Check if uv is available
if ! command -v uv &> /dev/null; then
    echo "📦 Installing uv package manager..."
    curl -LsSf https://astral.sh/uv/install.sh | sh

    # Add uv to PATH for current session
    export PATH="$HOME/.local/bin:$PATH"

    # Verify installation
    if ! command -v uv &> /dev/null; then
        echo "❌ Failed to install uv. Please install manually from https://github.com/astral-sh/uv"
        exit 1
    fi
fi

echo "✅ uv available ($(uv --version))"

# Check if pyproject.toml exists
if [[ ! -f "pyproject.toml" ]]; then
    echo "❌ pyproject.toml not found in current directory"
    echo "   Please run this script from the Automated-Azan directory"
    exit 1
fi

# Install dependencies
echo "📦 Installing dependencies..."
uv sync --dev

# Verify critical packages
echo ""
echo "🔍 Verifying installation..."
uv run python -c "import pychromecast" 2>/dev/null && echo "✅ pychromecast ready" || echo "⚠️  pychromecast issue"
uv run python -c "import flask" 2>/dev/null && echo "✅ flask ready" || echo "⚠️  flask issue"
uv run python -c "import schedule" 2>/dev/null && echo "✅ schedule ready" || echo "⚠️  schedule issue"
uv run python -c "import watchdog" 2>/dev/null && echo "✅ watchdog ready" || echo "⚠️  watchdog issue"

# Configure config file if not exists
if [[ ! -f "adahn.config" ]]; then
    if [[ -f "adahn.config.example" ]]; then
        echo "📝 Creating adahn.config from example..."
        cp adahn.config.example adahn.config
    else
        echo "⚠️  No adahn.config found. Creating default..."
        cat > adahn.config << EOF
[Settings]
speakers-group-name = athan
location = naas
pre_fajr_enabled = True
EOF
    fi
fi

echo ""
echo "✅ Development environment ready!"
echo ""
echo "Next steps:"
echo "1. Edit adahn.config to set your location and speaker group"
echo "2. Run the application: uv run python main.py"
echo "3. Or run the web interface: uv run python web_interface.py"
echo "4. Access web interface at: http://localhost:5000"