# Automated Azan - UV Development Guide

This guide explains how to set up and work with the Automated Azan project using uv for dependency management.

## 🚀 UV Setup

### Why UV?
- **Fast dependency resolution** - 10-100x faster than pip
- **Deterministic builds** with lock files
- **Python version management** built-in
- **Virtual environment management** automatically handled
- **Better caching** and disk usage
- **Drop-in pip replacement**

### Installation

1. **Install uv** (if not already installed):
   ```bash
   pip install uv
   ```

2. **Clone and setup the project**:
   ```bash
   git clone https://github.com/shenhab/Automated-Azan.git
   cd Automated-Azan
   make setup
   ```

## 🛠️ Development Commands

### Project Setup
```bash
# Setup development environment
make setup

# Install dependencies only
make install

# Update dependencies
make update
```

### Running the Application
```bash
# Run prayer scheduler
make run
# or directly: uv run python main.py

# Run web interface
make web
# or directly: uv run python web_interface.py

# Run tests
make test
# or directly: uv run python test_suite.py
```

### Working with Dependencies

#### Adding New Dependencies
```bash
# Add to pyproject.toml dependencies array
# Then run:
uv pip install -e .
```

#### Platform-Specific Installation

**For GUI features (system tray):**
```bash
# Install with GUI dependencies
uv pip install -e ".[gui]"
```

**For headless servers:**
```bash
# Install core dependencies only
uv pip install -e .

# Run without system tray
uv run python main.py --no-tray
```

**For building executables:**
```bash
# Install build dependencies
uv pip install -e ".[build]"
```

#### Development Dependencies
Add to the `tool.uv.dev-dependencies` section in pyproject.toml:
```toml
[tool.uv]
dev-dependencies = [
    "pytest>=6.0",
    "black>=22.0",
]
```

### Virtual Environment Management

UV automatically manages virtual environments:
```bash
# Install and run in one command
uv run python main.py

# Or manually activate environment
uv venv
source .venv/bin/activate  # Linux/Mac
# .venv\Scripts\activate     # Windows
```

## 🏗️ Project Structure

```
Automated-Azan/
├── pyproject.toml          # Project configuration and dependencies
├── main.py                 # Prayer scheduler
├── web_interface.py        # Web interface
├── chromecast_manager.py   # Device management
├── prayer_times_fetcher.py # Prayer time API
└── Media/                  # Audio files
```

## 🔧 Configuration

1. **Copy configuration template**:
   ```bash
   cp adahn.config.example adahn.config
   ```

2. **Edit configuration**:
   ```bash
   nano adahn.config
   ```

3. **Basic settings**:
   ```ini
   [Settings]
   speakers-group-name = Living Room speakers
   location = Leeds, UK
   prayer_source = icci
   ```

## 🧪 Testing

```bash
# Run full test suite
make test

# Test Chromecast discovery
make test-chromecast

# Test specific component
uv run python -c "from chromecast_manager import ChromecastManager; print('✅ Import successful')"
```

## 🐳 Production Deployment

For production, use Docker (recommended):
```bash
make deploy
```

This builds a container with all dependencies and runs the service.

## 🔍 Troubleshooting

### Common Issues

1. **Dependencies not found**:
   ```bash
   uv pip install -e .
   ```

2. **Python version issues**:
   ```bash
   uv python install 3.11
   uv venv --python 3.11
   ```

3. **Virtual environment issues**:
   ```bash
   rm -rf .venv
   uv venv
   uv pip install -e .
   ```

### Getting Help
```bash
uv --help              # UV help
make help              # Project commands
uv run python --help  # Python help
```

## 📊 Performance Benefits

UV provides significant performance improvements:
- **10-100x faster** dependency resolution
- **Parallel downloads** and installs
- **Better caching** reduces repeated downloads
- **Smaller virtual environments**

---

🕌 **May this streamlined development environment help you maintain your prayers with ease.**