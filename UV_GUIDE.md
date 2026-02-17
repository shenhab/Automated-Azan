# UV Package Manager Guide

## What is UV?

UV is a fast Python package manager written in Rust that aims to replace pip, pip-tools, pipenv, poetry, and other Python workflow tools. It's significantly faster than traditional Python package managers.

## Installation

### Install UV (if not already installed)
```bash
curl -LsSf https://astral.sh/uv/install.sh | sh
```

Or using pip:
```bash
pip install uv
```

## Basic Commands

### Install dependencies
```bash
# Install all dependencies from pyproject.toml
uv sync

# Install including dev dependencies
uv sync --dev
```

### Run commands in the virtual environment
```bash
# Run the main application
uv run python main.py

# Run tests
uv run pytest

# Run any command
uv run <command>
```

### Add new dependencies
```bash
# Add a production dependency
uv add requests

# Add a development dependency
uv add --dev pytest

# Add with version constraint
uv add "flask>=3.0"
```

### Remove dependencies
```bash
# Remove a dependency
uv remove requests
```

### Update dependencies
```bash
# Update all dependencies
uv sync --upgrade

# Update a specific package
uv add --upgrade flask
```

### Virtual Environment Management
```bash
# UV automatically creates and manages a .venv directory
# Activate the virtual environment manually (optional)
source .venv/bin/activate

# Deactivate
deactivate
```

## Project Structure

The project uses `pyproject.toml` for all configuration:
- Dependencies are listed under `[project] dependencies`
- Dev dependencies are under `[dependency-groups] dev`
- Build configuration uses setuptools
- All Python modules are specified in `[tool.setuptools]`

## Common Workflows

### Fresh Installation
```bash
# Clone the repository
git clone <repo-url>
cd Automated-Azan

# Install uv (if needed)
curl -LsSf https://astral.sh/uv/install.sh | sh

# Install all dependencies
uv sync --dev

# Run the application
uv run python main.py
```

### Running Tests
```bash
# Run all tests
uv run pytest

# Run with coverage
uv run pytest --cov

# Run specific test file
uv run pytest tests/test_config_manager.py
```

### Development Workflow
```bash
# Start development
uv sync --dev

# Make changes to code...

# Run tests
uv run pytest

# Run the application
uv run python main.py
```

### Deployment
```bash
# Install production dependencies only
uv sync --no-dev

# Run the application
uv run python main.py
```

## Advantages of UV

1. **Speed**: 10-100x faster than pip
2. **Reproducible**: Uses lock file for exact dependency versions
3. **Simple**: Single tool replaces multiple Python tools
4. **Compatible**: Works with pyproject.toml standard
5. **Automatic venv**: Manages virtual environments automatically

## Troubleshooting

### Clear cache if having issues
```bash
uv cache clean
```

### Recreate virtual environment
```bash
rm -rf .venv
uv sync
```

### Check UV version
```bash
uv --version
```

### Update UV itself
```bash
uv self update
```

## Migration from Pipenv

If you were using Pipenv before:
- `pipenv install` → `uv sync`
- `pipenv install --dev` → `uv sync --dev`
- `pipenv run python main.py` → `uv run python main.py`
- `pipenv shell` → `source .venv/bin/activate`
- `Pipfile` → `pyproject.toml`

## Environment Variables

UV respects standard Python environment variables:
- `VIRTUAL_ENV`: Path to virtual environment
- `UV_CACHE_DIR`: Cache directory location
- `UV_NO_CACHE`: Disable cache

## For CI/CD

```yaml
# GitHub Actions example
- name: Install uv
  uses: astral-sh/setup-uv@v1

- name: Install dependencies
  run: uv sync

- name: Run tests
  run: uv run pytest
```