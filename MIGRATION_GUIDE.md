# 🔄 Pipenv to UV Migration Guide

This guide helps existing users migrate from pipenv to uv for the Automated Azan project.

## 🚨 Breaking Changes

### Files Removed
- `Pipfile` and `Pipfile.lock` → Replaced by `pyproject.toml`
- `requirements.txt` → Dependencies now in `pyproject.toml`
- `PIPENV_GUIDE.md` → Replaced by `UV_GUIDE.md`

### Commands Changed
| Old (pipenv) | New (uv) |
|-------------|----------|
| `pipenv install` | `uv pip install -e .` |
| `pipenv install --dev` | `uv pip install -e .` |
| `pipenv run python main.py` | `uv run python main.py` |
| `pipenv shell` | `uv shell` or `source .venv/bin/activate` |
| `pipenv update` | `uv pip install --upgrade -e .` |

## 🚀 Migration Steps

### For Development Users

1. **Clean up old environment:**
   ```bash
   # Remove pipenv virtual environment
   pipenv --rm

   # Or manually remove if needed
   rm -rf ~/.local/share/virtualenvs/Automated-Azan-*
   ```

2. **Install uv:**
   ```bash
   pip install uv
   ```

3. **Setup new environment:**
   ```bash
   # Create new virtual environment
   uv venv

   # Activate it
   source .venv/bin/activate  # Linux/Mac
   # .venv\Scripts\activate     # Windows

   # Install dependencies
   uv pip install -e .
   ```

4. **Update your workflows:**
   ```bash
   # Old way
   pipenv run python main.py

   # New way
   uv run python main.py
   ```

### For Docker Users

**No changes needed!** Docker containers will automatically use the new uv-based setup.

### For CI/CD Pipelines

Update your build scripts:

```yaml
# Before
- pip install pipenv
- pipenv install --dev
- pipenv run pytest

# After
- pip install uv
- uv pip install -e .
- uv run pytest
```

## 🎯 Why Migrate?

### Performance Benefits
- **10-100x faster** dependency resolution
- **Parallel downloads** and installs
- **Better caching** reduces repeated downloads
- **Smaller virtual environments**

### Developer Experience
- **Drop-in pip replacement**
- **Better error messages**
- **Cross-platform consistency**
- **Built-in Python version management**

## 🔧 Troubleshooting

### Virtual Environment Issues
```bash
# Remove and recreate if problems occur
rm -rf .venv
uv venv
uv pip install -e .
```

### Missing Dependencies
```bash
# Force reinstall all dependencies
uv pip install --force-reinstall -e .
```

### PATH Issues
```bash
# Add uv to PATH (if installed with --user)
export PATH="$HOME/.local/bin:$PATH"
```

### Permission Errors
```bash
# Install uv system-wide if needed
sudo pip install uv
```

## 📋 Verification

Test your migration is successful:

```bash
# Check uv is working
uv --version

# Test key imports
uv run python -c "import pychromecast; print('✅ pychromecast')"
uv run python -c "import flask; print('✅ flask')"

# Test project modules
uv run python -c "from prayer_times_fetcher import PrayerTimesFetcher; print('✅ Project modules')"

# Run the application
uv run python main.py
```

## 🆘 Need Help?

### Common Issues

1. **"uv not found"**
   - Install with: `pip install uv`
   - Add to PATH: `export PATH="$HOME/.local/bin:$PATH"`

2. **"Dependencies not found"**
   - Run: `uv pip install -e .`
   - Check you're in the project directory

3. **"Virtual environment issues"**
   - Remove: `rm -rf .venv`
   - Recreate: `uv venv && uv pip install -e .`

### Getting Support

- 📖 Read the [UV Guide](UV_GUIDE.md)
- 🐛 Report issues at: https://github.com/shenhab/Automated-Azan/issues
- 💬 Include your OS, Python version, and error messages

## 🎉 Benefits You'll Enjoy

After migration, you'll experience:

- ⚡ **Faster installs** - Dependencies install 10-100x faster
- 🎯 **Better reliability** - More consistent dependency resolution
- 🧹 **Cleaner workflows** - Simpler commands and better caching
- 🔄 **Future-proof** - Active development and modern tooling

---

🕌 **Welcome to the faster, more reliable UV ecosystem!**