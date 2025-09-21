# 🌍 Platform Compatibility Guide

This guide covers cross-platform compatibility for the Automated Azan project using UV.

## ✅ Supported Platforms

### **Operating Systems**
- ✅ **Windows** 10/11 (GUI + Headless)
- ✅ **Linux** (GUI + Headless)
- ✅ **macOS** (GUI + Headless)
- ✅ **Docker** (All platforms)

### **Python Versions**
- ✅ **Python 3.11+** (Required)
- ⚠️ **Python 3.10** (May work but not tested)
- ❌ **Python 3.9 and below** (Not supported)

## 🚀 Installation by Platform

### **Windows**

#### GUI Installation (with system tray):
```powershell
# Install UV
pip install uv

# Clone project
git clone https://github.com/shenhab/Automated-Azan.git
cd Automated-Azan

# Install with GUI support
uv pip install -e ".[gui]"

# Run with system tray
uv run python main.py
```

#### Headless Installation (Windows Server):
```powershell
# Install UV
pip install uv

# Install core dependencies only
uv pip install -e .

# Run without system tray
uv run python main.py --no-tray
```

### **Linux**

#### GUI Installation (Desktop):
```bash
# Install UV
pip install uv

# Clone project
git clone https://github.com/shenhab/Automated-Azan.git
cd Automated-Azan

# Install with GUI support
uv pip install -e ".[gui]"

# Run with system tray
uv run python main.py
```

#### Headless Installation (Server):
```bash
# Install UV
pip install uv

# Install core dependencies only
uv pip install -e .

# Run without system tray (recommended for servers)
uv run python main.py --no-tray
```

#### Docker Installation (Recommended for servers):
```bash
# Use Docker for production deployment
make deploy
```

### **macOS**

#### GUI Installation:
```bash
# Install UV
pip install uv

# Clone project
git clone https://github.com/shenhab/Automated-Azan.git
cd Automated-Azan

# Install with GUI support
uv pip install -e ".[gui]"

# Run with system tray
uv run python main.py
```

## 🔧 Platform-Specific Notes

### **Windows Considerations**
- **PowerShell Execution Policy**: You may need to run:
  ```powershell
  Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
  ```
- **Virtual Environment**: Uses `Scripts\activate.ps1`
- **Antivirus**: Some antivirus software may flag PyInstaller builds

### **Linux Considerations**
- **Display Server**: System tray requires X11 or Wayland
- **Headless Servers**: Always use `--no-tray` flag
- **Permissions**: May need to install UV system-wide:
  ```bash
  sudo pip install uv
  ```

### **macOS Considerations**
- **System Integrity Protection**: May affect system tray functionality
- **Homebrew Python**: Recommended over system Python
- **Permissions**: May need to allow network access for Chromecast discovery

## 🐳 Docker (Universal Solution)

**Recommended for production deployments on any platform:**

```bash
# Works on Windows, Linux, macOS
docker run -d \
  --name athan \
  --network host \
  --restart unless-stopped \
  -v $(pwd)/adahn.config:/app/config/adahn.config \
  shenhab/athan:latest
```

## 🧪 Testing Installation

### **Basic Functionality Test**
```bash
# Test UV is working
uv --version

# Test core imports
uv run python -c "import pychromecast; print('✅ Core dependencies')"

# Test GUI imports (if installed)
uv run python -c "import pystray; print('✅ GUI dependencies')" 2>/dev/null || echo "ℹ️ GUI not available"

# Test project modules
uv run python -c "from prayer_times_fetcher import PrayerTimesFetcher; print('✅ Project modules')"
```

### **Platform-Specific Tests**

**Windows:**
```powershell
# Test PowerShell script execution
.\init_env.ps1

# Test UV in PowerShell
uv run python --version
```

**Linux/macOS:**
```bash
# Test shell script execution
./init_env.sh

# Test UV in bash
uv run python --version
```

## 🚨 Common Issues & Solutions

### **1. "uv command not found"**
```bash
# Solution: Add to PATH
export PATH="$HOME/.local/bin:$PATH"  # Linux/macOS
# or
$env:PATH += ";$HOME\.local\bin"      # Windows PowerShell
```

### **2. "pystray import error" (Headless systems)**
```bash
# Solution: Run with --no-tray flag
uv run python main.py --no-tray
```

### **3. "Permission denied" on virtual environment**
```bash
# Solution: Remove and recreate
rm -rf .venv
uv venv
uv pip install -e .
```

### **4. "SSL certificate error"**
```bash
# Solution: Update certificates or use --trusted-host
uv pip install --trusted-host pypi.org -e .
```

### **5. "Display/GUI errors" on headless Linux**
```bash
# Solution: Use headless mode
export DISPLAY=""
uv run python main.py --no-tray
```

## 📊 Performance by Platform

| Platform | Install Speed | Runtime Performance | System Tray |
|----------|---------------|---------------------|-------------|
| Windows GUI | ⚡ Fast | 🟢 Excellent | ✅ Full Support |
| Windows Headless | ⚡ Very Fast | 🟢 Excellent | ❌ Disabled |
| Linux GUI | ⚡ Fast | 🟢 Excellent | ✅ Full Support |
| Linux Headless | ⚡ Very Fast | 🟢 Excellent | ❌ Disabled |
| macOS GUI | ⚡ Fast | 🟢 Excellent | ⚠️ Limited |
| Docker | ⚡ Fast | 🟢 Excellent | ❌ N/A |

## 🎯 Recommendations

### **For Development**
- **Windows/macOS**: Use GUI installation for best experience
- **Linux Desktop**: Use GUI installation
- **Linux Server**: Use headless or Docker

### **For Production**
- **All Platforms**: Use Docker (most reliable)
- **Windows Server**: Use headless installation
- **Linux Server**: Use headless or Docker
- **Cloud/VPS**: Use Docker

### **For CI/CD**
```yaml
# GitHub Actions example
- name: Setup UV
  run: pip install uv

- name: Install dependencies
  run: uv pip install -e .

- name: Test headless mode
  run: uv run python main.py --no-tray --debug
```

---

🕌 **The application now works flawlessly across all platforms and environments!**