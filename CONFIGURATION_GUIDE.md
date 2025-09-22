# 🔧 Configuration Guide - Automated Azan Application

## 📋 Configuration Architecture Overview

The Automated Azan application uses a **modular configuration system** with separate files for different concerns:

```
📁 Configuration Files:
├── app_config.py           → 🏠 Main application settings (NEW)
├── chromecast_config.py    → 🎵 Audio system constants
├── prayer_times_config.py  → 🕐 Prayer-specific settings
├── config_manager.py       → 📜 Legacy configuration (DEPRECATED)
└── adahn.config           → 📄 User configuration file (INI format)
```

---

## 🆕 New Modern Configuration System

### **app_config.py** - Main Application Configuration

Uses **dataclass pattern** with **type safety** and **environment variable support**.

#### **🌍 Environment Variable Patterns**

All environment variables follow the pattern: `AZAN_[SECTION]_[SETTING]`

```bash
# Location Settings
AZAN_LATITUDE=51.5074
AZAN_LONGITUDE=-0.1278
AZAN_CITY="London"
AZAN_COUNTRY="UK"
AZAN_TIMEZONE="Europe/London"

# Prayer Settings
AZAN_PRAYER_SOURCE="NAAS"
AZAN_CALCULATION_METHOD="ISNA"
AZAN_MADHAB="Shafi"
AZAN_PRE_FAJR_ENABLED=true
AZAN_PRE_FAJR_MINUTES=30

# Audio Settings
AZAN_SPEAKERS_GROUP="Living Room"
AZAN_VOLUME_LEVEL=0.7
AZAN_FADE_IN_DURATION=2.0
AZAN_FADE_OUT_DURATION=3.0

# Web Interface
AZAN_WEB_HOST="0.0.0.0"
AZAN_WEB_PORT=5000
AZAN_WEB_DEBUG=false
AZAN_WEB_SECRET_KEY="your-secret-key"

# Logging
AZAN_LOG_LEVEL="INFO"
AZAN_LOG_FILE_ENABLED=true
AZAN_LOG_FILE_PATH="logs/azan.log"
AZAN_LOG_CONSOLE_ENABLED=true

# Security
AZAN_API_KEY_REQUIRED=false
AZAN_API_KEY="your-api-key"
AZAN_RATE_LIMIT_ENABLED=true
AZAN_RATE_LIMIT_REQUESTS=100

# Notifications
AZAN_NOTIFICATIONS_ENABLED=false
AZAN_EMAIL_ENABLED=false
AZAN_EMAIL_SMTP_SERVER="smtp.gmail.com"
AZAN_EMAIL_USERNAME="your-email@gmail.com"
AZAN_EMAIL_RECIPIENTS="admin@domain.com,user@domain.com"
```

---

## 🎵 Chromecast Configuration System

### **chromecast_config.py** - Audio System Constants

**Technical settings** for developers and system administrators.

#### **🌍 Environment Variable Patterns**

All environment variables follow the pattern: `CHROMECAST_[CATEGORY]_[SETTING]`

```bash
# Device Discovery
CHROMECAST_DISCOVERY_COOLDOWN=30
CHROMECAST_DISCOVERY_TIMEOUT=8
CHROMECAST_DISCOVERY_MAX_ATTEMPTS=15
CHROMECAST_DISCOVERY_CALLBACK_TIMEOUT=3

# Connection Management
CHROMECAST_CONNECTION_TIMEOUT=10
CHROMECAST_CONNECTION_MAX_RETRIES=3
CHROMECAST_CONNECTION_RETRY_DELAY=2.0
CHROMECAST_DEFAULT_PORT=8009
CHROMECAST_SOCKET_TIMEOUT=3

# Media Playback
CHROMECAST_PLAYBACK_MAX_RETRIES=2
CHROMECAST_MEDIA_LOAD_MAX_ATTEMPTS=15
CHROMECAST_MEDIA_LOAD_INITIAL_WAIT=0.5
CHROMECAST_MEDIA_LOAD_SHORT_WAIT=1.0
CHROMECAST_MEDIA_LOAD_MEDIUM_WAIT=2.0
CHROMECAST_MEDIA_LOAD_LONG_WAIT=3.0

# Athan Settings
ATHAN_TIMEOUT_SECONDS=480
ATHAN_REGULAR_FILENAME="media_Athan.mp3"
ATHAN_FAJR_FILENAME="media_adhan_al_fajr.mp3"

# Device Priority
CHROMECAST_PRIMARY_DEVICE="Adahn"

# Circuit Breaker
CIRCUIT_BREAKER_FAILURE_THRESHOLD=5
CIRCUIT_BREAKER_RECOVERY_TIMEOUT=60

# Health Monitoring
HEALTH_CHECK_INTERVAL_SECONDS=30
HEALTH_CHECK_TIMEOUT_SECONDS=5
```

---

## 📄 Configuration File Formats

### **1. Environment Variables (.env file)**
```bash
# Create .env file in project root
AZAN_LATITUDE=51.5074
AZAN_LONGITUDE=-0.1278
AZAN_CITY="London"
AZAN_PRAYER_SOURCE="NAAS"
AZAN_WEB_PORT=5000
```

### **2. Legacy INI Format (adahn.config)**
```ini
[Location]
latitude = 51.5074
longitude = -0.1278
city = London
country = UK

[Prayer]
source = NAAS
calculation_method = ISNA
madhab = Shafi
pre_fajr_enabled = true

[Audio]
speakers_group = Living Room
volume = 0.7

[Web]
host = 0.0.0.0
port = 5000
debug = false
```

---

## 🔄 Configuration Loading Priority

Settings are loaded in this order (later overrides earlier):

1. **Default values** in dataclass definitions
2. **Legacy INI file** (adahn.config)
3. **Environment variables** (from .env or system)
4. **Runtime updates** (via API or web interface)

---

## 🎯 Usage Examples

### **Python Code Usage**

```python
# New modern approach
from app_config import get_app_config

config = get_app_config()

# Access typed configuration
latitude = config.location.latitude      # float
port = config.web.port                   # int
debug_mode = config.web.debug            # bool
speakers = config.audio.speakers_group_name  # str

# Get as dictionary for JSON API
config_dict = config.to_dict()

# Chromecast technical settings
from chromecast_config import get_config as get_chromecast_config

chromecast_config = get_chromecast_config()
timeout = chromecast_config.CONNECTION_TIMEOUT_SECONDS
retries = chromecast_config.CONNECTION_MAX_RETRIES
```

### **Legacy Compatibility**

```python
# Old code still works (backward compatibility)
from app_config import get_setting, get_location, get_prayer_source

setting = get_setting('Location', 'latitude', 51.5074)
location = get_location()
source = get_prayer_source()
```

---

## 🔧 Configuration Validation

### **Automatic Validation**

The new system automatically validates:

- ✅ **Latitude**: -90 to 90 degrees
- ✅ **Longitude**: -180 to 180 degrees
- ✅ **Volume**: 0.0 to 1.0
- ✅ **Port numbers**: 1 to 65535
- ✅ **Directory creation**: Auto-creates missing directories
- ✅ **Type conversion**: Automatic string to int/float/bool conversion

### **Error Handling**

```python
try:
    config = AppConfig()
except ValueError as e:
    print(f"Configuration error: {e}")
    # Handle invalid configuration
```

---

## 🐳 Docker Configuration

### **Environment Variables in Docker**

```dockerfile
# Dockerfile
ENV AZAN_LATITUDE=51.5074
ENV AZAN_LONGITUDE=-0.1278
ENV AZAN_WEB_PORT=5000
ENV CHROMECAST_CONNECTION_TIMEOUT=15
```

### **Docker Compose**

```yaml
# docker-compose.yml
version: '3.8'
services:
  azan:
    image: automated-azan
    environment:
      - AZAN_LATITUDE=51.5074
      - AZAN_LONGITUDE=-0.1278
      - AZAN_PRAYER_SOURCE=NAAS
      - AZAN_WEB_PORT=5000
      - CHROMECAST_DISCOVERY_COOLDOWN=30
    volumes:
      - ./config:/app/config
      - ./logs:/app/logs
```

---

## 🚀 Best Practices

### **1. ✅ Environment-Specific Settings**

```bash
# Development
AZAN_ENVIRONMENT=development
AZAN_WEB_DEBUG=true
AZAN_LOG_LEVEL=DEBUG
CHROMECAST_CONNECTION_TIMEOUT=5

# Production
AZAN_ENVIRONMENT=production
AZAN_WEB_DEBUG=false
AZAN_LOG_LEVEL=INFO
CHROMECAST_CONNECTION_TIMEOUT=10
```

### **2. 🔐 Secrets Management**

```bash
# Use environment variables for secrets
AZAN_WEB_SECRET_KEY="${SECRET_KEY}"
AZAN_API_KEY="${API_KEY}"
AZAN_EMAIL_PASSWORD="${EMAIL_PASSWORD}"

# Don't put secrets in config files!
```

### **3. 📊 Configuration Monitoring**

```python
# Check configuration via API
GET /api/config/status

# Response:
{
  "success": true,
  "config": {
    "app": {"name": "Automated Azan", "version": "2.0.0"},
    "location": {"city": "London", "latitude": 51.5074},
    "prayer": {"source": "NAAS", "calculation_method": "ISNA"}
  }
}
```

---

## 🔄 Migration from Legacy System

### **Step 1: Install New System**
```bash
# New configuration system is backward compatible
# Existing adahn.config files continue to work
```

### **Step 2: Gradual Migration**
```python
# Replace old imports gradually
# Old: from config_manager import ConfigManager
# New: from app_config import get_app_config

# Old code still works during transition
```

### **Step 3: Environment Variables**
```bash
# Convert INI settings to environment variables
# INI:  [Location] latitude = 51.5074
# ENV:  AZAN_LATITUDE=51.5074
```

---

## ❓ Troubleshooting

### **Common Issues**

| Issue | Cause | Solution |
|-------|-------|----------|
| **Invalid latitude** | Outside -90 to 90 range | Check `AZAN_LATITUDE` value |
| **Port in use** | Web port already taken | Change `AZAN_WEB_PORT` |
| **Config not loading** | Wrong file path | Check `AZAN_CONFIG_FILE` path |
| **Chromecast timeout** | Network issues | Increase `CHROMECAST_CONNECTION_TIMEOUT` |
| **Log file errors** | Permission issues | Check `AZAN_LOG_FILE_PATH` permissions |

### **Debug Configuration**

```python
# Print current configuration
from app_config import get_app_config
import json

config = get_app_config()
print(json.dumps(config.to_dict(), indent=2))
```

---

## 📚 Related Documentation

- 🕌 [Main README](README.md)
- 🐳 [Docker Guide](DOCKER_GUIDE.md)
- 🧪 [Testing Guide](TESTING.md)
- 🏗️ [Architecture Mind Map](ARCHITECTURE_MINDMAP.md)

---

The new configuration system provides **type safety**, **environment variable support**, and **backward compatibility** while maintaining clean separation between user settings and technical constants! 🎯✨