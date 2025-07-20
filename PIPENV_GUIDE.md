# Automated Azan - Pipenv Development Guide

This guide explains how to set up and work with the Automated Azan project using pipenv for dependency management.

## 🐍 Pipenv Setup

### Why Pipenv?
- **Deterministic builds** with Pipfile.lock
- **Virtual environment management** automatically handled
- **Development and production dependencies** separated
- **Security scanning** built-in
- **Better dependency resolution** than pip

### Installation

1. **Install pipenv** (if not already installed):
   ```bash
   pip install pipenv
   ```

2. **Clone and setup the project**:
   ```bash
   git clone https://github.com/shenhab/Automated-Azan.git
   cd Automated-Azan
   
   # Install dependencies
   pipenv install
   
   # Or use the setup script
   ./setup-dev.sh
   ```

3. **Configure the application**:
   ```bash
   # Copy configuration template
   cp adahn.config.example adahn.config
   
   # Edit with your settings
   nano adahn.config
   ```

## 🚀 Development Workflow

### Activate Virtual Environment
```bash
# Activate the shell
pipenv shell

# Or run commands directly
pipenv run python main.py
```

### Common Commands
```bash
# Install new package
pipenv install requests

# Install development dependency
pipenv install pytest --dev

# Update all packages
pipenv update

# Show dependency graph
pipenv graph

# Check for security vulnerabilities
pipenv check

# Generate requirements.txt (for Docker)
pipenv requirements > requirements.txt
```

### Running the Application
```bash
# Main application
pipenv run python main.py

# Web interface
pipenv run python web_interface.py

# Run tests
pipenv run python -m pytest

# Or use the test script
./test-pipenv.sh
```

### Using Makefile (Recommended)
```bash
# See all available commands
make help

# Setup development environment
make setup

# Run tests
make test

# Run main application
make run

# Run web interface
make web

# Build Docker images
make docker-build
```

## 📦 Dependency Management

### Project Dependencies
The project uses these main dependencies:

- **pychromecast** - Chromecast communication
- **flask** - Web interface framework
- **flask-socketio** - Real-time web features
- **requests** - HTTP client for API calls
- **beautifulsoup4** - HTML parsing for prayer times
- **python-dateutil** - Date/time handling
- **schedule** - Task scheduling (alternative approach)
- **python-dotenv** - Environment variable loading

### Adding Dependencies
```bash
# Add runtime dependency
pipenv install package-name

# Add development dependency
pipenv install package-name --dev

# Install specific version
pipenv install "package-name==1.0.0"

# Install from git
pipenv install git+https://github.com/user/repo.git
```

### Removing Dependencies
```bash
# Remove package
pipenv uninstall package-name

# Remove all packages
pipenv uninstall --all
```

## 🐳 Docker Integration

The project includes Docker support that uses pipenv:

### Dockerfile Structure
```dockerfile
# Install pipenv
RUN pip install --upgrade pip pipenv

# Copy Pipfiles
COPY Pipfile Pipfile.lock ./

# Install dependencies using pipenv
RUN pipenv install --system --deploy
```

### Building Docker Images
```bash
# Build using pipenv
make docker-build

# Or manually
docker-compose build
```

## 🧪 Testing

### Test Suite
```bash
# Run comprehensive tests
./test-pipenv.sh

# Or use make
make test
```

### Testing Components
The test suite checks:
- Python version compatibility
- All critical imports
- Prayer times API functionality
- Chromecast discovery
- Web interface startup
- Configuration file validation

### Manual Testing
```bash
# Test prayer times fetching
pipenv run python -c "
from prayer_times_fetcher import PrayerTimesFetcher
fetcher = PrayerTimesFetcher()
print(fetcher.fetch_prayer_times('icci'))
"

# Test Chromecast discovery
pipenv run python -c "
from chromecast_manager import ChromecastManager
manager = ChromecastManager()
print('Found devices:', len(manager.chromecasts))
manager.cleanup()
"
```

## 🔧 Troubleshooting

### Common Issues

1. **Pipenv not found**
   ```bash
   # Install pipenv
   pip install --user pipenv
   
   # Add to PATH
   export PATH="$HOME/.local/bin:$PATH"
   ```

2. **Virtual environment issues**
   ```bash
   # Remove and recreate
   pipenv --rm
   pipenv install
   ```

3. **Dependency conflicts**
   ```bash
   # Clear cache and reinstall
   pipenv --clear
   pipenv install
   ```

4. **Lock file issues**
   ```bash
   # Regenerate lock file
   pipenv lock
   ```

### Environment Variables
```bash
# Ignore system virtual environment
export PIPENV_IGNORE_VIRTUALENVS=1

# Reduce verbosity
export PIPENV_VERBOSITY=-1

# Use specific Python version
export PIPENV_PYTHON=python3.11
```

## 📈 Best Practices

### Development
- Always use `pipenv install` instead of `pip install`
- Commit both `Pipfile` and `Pipfile.lock`
- Use `pipenv check` regularly for security updates
- Keep dependencies minimal and well-documented

### Production
- Use `pipenv install --deploy --system` in Docker
- Lock dependencies with `pipenv lock`
- Use `Pipfile.lock` for reproducible builds
- Separate development and production dependencies

### Project Structure
```
Automated-Azan/
├── Pipfile              # Dependency specification
├── Pipfile.lock         # Locked dependencies
├── main.py              # Main application
├── web_interface.py     # Web interface
├── chromecast_manager.py # Chromecast handling
├── prayer_times_fetcher.py # Prayer times API
├── setup-dev.sh         # Development setup
├── test-pipenv.sh       # Test suite
├── Makefile            # Common tasks
└── adahn.config        # Configuration
```

## 🚀 Deployment

### Local Development
```bash
# Setup and run
make dev
make run
```

### Docker Deployment
```bash
# Build and deploy
make docker-build
make docker-run
```

### Portainer Deployment
See `PORTAINER_DEPLOYMENT.md` for detailed instructions.

## 📚 Additional Resources

- [Pipenv Documentation](https://pipenv.pypa.io/en/latest/)
- [Python Virtual Environments Guide](https://docs.python.org/3/tutorial/venv.html)
- [Docker Best Practices](https://docs.docker.com/develop/best-practices/)
- [Project README](README.md) for general information
