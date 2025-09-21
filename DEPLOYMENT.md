# Automated Azan - Deployment Guide

## Two Simple Deployment Methods

### 🐳 Production Deployment (Docker) - Recommended
*Single container with prayer scheduler + web interface*

```bash
# 1. Clone and configure
git clone https://github.com/shenhab/Automated-Azan.git
cd Automated-Azan

# 2. Create configuration
cp adahn.config.example adahn.config
nano adahn.config

# 3. Deploy
make deploy
```

**Access your system:**
- 🌐 Web Interface: http://localhost:5000
- 🔊 Device Management: http://localhost:5000/chromecasts  
- 🧪 Audio Testing: http://localhost:5000/test
- 📋 Logs: `make docker-logs`

---

### 🐍 Development Setup (uv)
*For development and local testing*

```bash
# 1. Setup development environment
git clone https://github.com/shenhab/Automated-Azan.git
cd Automated-Azan
make setup

# 2. Configure
cp adahn.config.example adahn.config
nano adahn.config

# 3. Run (choose one)
make run    # Prayer scheduler only
make web    # Web interface only
```

---
## Configuration

Edit `adahn.config`:

```ini
[Settings]
# Your Google Home speaker name (case-sensitive)
speakers-group-name = Living Room speakers

# Prayer time location  
location = Leeds, UK

# Prayer time source
prayer_source = icci

# Optional: Enable pre-Fajr Quran
pre_fajr_enabled = True
```

## Docker Commands

```bash
make deploy          # Complete deployment
make docker-logs     # View logs
make docker-stop     # Stop service
make docker-restart  # Restart service
make status          # Check status
```

## Development Commands  

```bash
make setup           # Setup environment
make run             # Run prayer scheduler
make web             # Run web interface
make test            # Test system
make test-chromecast # Test device discovery
```

## Requirements

**Docker Deployment:**
- Docker & Docker Compose
- Network access to Chromecast devices

**Development:**
- Python 3.9+
- uv
- Network access to Chromecast devices

## Troubleshooting

### Can't find Chromecast devices?
1. Ensure devices are on same network
2. Check speaker name matches exactly in config
3. Use web interface to test: http://localhost:5000/chromecasts

### Docker issues?
```bash
make deploy-check    # Validate setup
make docker-logs     # Check logs
docker-compose ps    # Container status
```

### Development issues?
```bash
make test-chromecast # Test discovery
make check           # Check requirements  
```

---

## What you get:

✅ **Automated Prayer Announcements** - Scheduled based on your location  
✅ **Professional Web Interface** - Real-time monitoring and control  
✅ **Device Testing** - Test audio on any discovered speaker  
✅ **Multiple Prayer Sources** - ICCI and Naas timetables  
✅ **Production Ready** - Docker deployment with health checks  
✅ **Real-time Updates** - Live status via web interface  

**🕌 Simple, reliable, automated prayer announcements.**
# Follow the displayed instructions
```

### Option 3: Manual Docker
```bash
docker build -t shenhab/athan .
docker run -d \
  --name athan \
  --network host \
  --restart unless-stopped \
  -v $(pwd)/adahn.config:/app/adahn.config:ro \
  -v azan_logs:/var/log \
  -v azan_data:/app/data \
  -e TZ=UTC \
  shenhab/athan
```

## Features

### 🕌 Automated Prayer Announcements
- **Automatic prayer time calculation** for Dublin (Naas) or ICCI
- **Smart Chromecast discovery** and reliable playback
- **Dual Adhan support**: Regular and Fajr-specific audio
- **Robust retry logic** with device-specific targeting

### 🌐 Web Management Interface
- **Real-time device discovery** with live status updates
- **Individual device testing** - test any Chromecast separately
- **Audio file management** with local serving
- **Comprehensive logging** with web-based log viewer
- **Responsive Bootstrap UI** that works on mobile and desktop

### 🐳 Production Ready
- **Single container deployment** with integrated web interface
- **Persistent data storage** with Docker volumes
- **Health monitoring** and automatic restarts
- **Network host mode** for optimal Chromecast discovery
- **Security hardened** with non-root user

## Troubleshooting

### Chromecast Discovery Issues
1. Ensure container uses `--network host`
2. Verify Chromecast devices are on same network
3. Check firewall allows multicast traffic
4. Use web interface `/chromecasts` to verify discovery

### Web Interface Not Loading
1. Check container health: `docker ps`
2. View logs: `make docker-logs`
3. Verify port 5000 is accessible
4. Check firewall allows port 5000

### Audio Playback Issues
1. Test individual devices via web interface `/test`
2. Verify Media files exist and are accessible
3. Check Chromecast speaker group name in config
4. Ensure selected device isn't already playing media

### Time Synchronization
The system includes modern NTP-based time sync to ensure accurate prayer times without requiring sudo privileges.

## Commands

```bash
# Management
make deploy              # Deploy complete system
make docker-logs         # View container logs
make docker-restart      # Restart container
make docker-stop         # Stop container
make docker-rebuild      # Rebuild and restart

# Development
make test               # Run tests
make clean              # Clean temporary files
```

## File Structure

```
Automated-Azan/
├── main.py                 # Main application with integrated web server
├── chromecast_manager.py   # Chromecast discovery and playback
├── web_interface.py        # Web management interface
├── time_sync.py           # Modern time synchronization
├── prayer_times_fetcher.py # Prayer time calculation
├── adahn.config           # Configuration file
├── Media/                 # Audio files (included in container)
│   ├── media_Athan.mp3
│   └── media_adhan_al_fajr.mp3
├── templates/             # Web UI templates
├── static/               # Web UI assets
├── Dockerfile            # Production container definition
├── docker-compose.yml    # Local deployment configuration
└── portainer-stack.yml   # Portainer deployment configuration
```

## Advanced Configuration

### Custom Speaker Groups
Create speaker groups in Google Home app, then update config:
```ini
speakers-group-name = My Custom Prayer Speakers
```

### Custom Prayer Times API
Extend `prayer_times_fetcher.py` to support additional APIs.

### Custom Web Interface
Mount custom templates:
```yaml
volumes:
  - ./custom-templates:/app/templates:ro
```

## Security Notes

- Container runs as non-root user (appuser:1000)
- Configuration file mounted read-only
- Health checks ensure service reliability
- Network host mode required for Chromecast (reduces network isolation)

## Support

- Web interface includes comprehensive device testing
- Built-in health checks and logging
- Automatic retry logic with device preservation
- Real-time status updates via WebSocket
