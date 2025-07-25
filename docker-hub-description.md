# Automated Azan - Islamic Prayer Time Automation

🕌 **Automated prayer announcements via Chromecast with professional web interface**

## Quick Start (Recommended)

```bash
# 1. Create configuration directory
mkdir -p ~/azan-config

# 2. Create configuration file
cat > ~/azan-config/adahn.config << EOF
[Settings]
speakers-group-name = athan
location = naas
EOF

# 3. Deploy with default settings
docker run -d \
  --name athan \
  --network host \
  --restart unless-stopped \
  -v ~/azan-config:/app/config \
  -v azan_logs:/var/log \
  -v azan_data:/app/data \
  -e TZ=UTC \
  shenhab/athan:latest
```

**🌐 Access:** http://localhost:5000

## Easy Configuration

Edit `~/azan-config/adahn.config`:
```ini
[Settings]
# Your Chromecast/Google Home name (case-sensitive)
speakers-group-name = athan

# Prayer time location (naas, icci, or custom location)
location = naas
```

## Docker Compose (Advanced)

```yaml
version: '3.8'
services:
  automated-azan:
    image: shenhab/athan:latest
    container_name: athan
    restart: unless-stopped
    network_mode: host
    environment:
      - TZ=UTC  # Change to your timezone
    volumes:
      - azan_config:/app/config
      - azan_logs:/var/log
      - azan_data:/app/data
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:5000/"]
      interval: 30s
      timeout: 10s
      retries: 3

volumes:
  azan_logs:
  azan_data:
  azan_config:
```

## Features

✅ **Automated Prayer Announcements** - Scheduled based on your location  
✅ **Professional Web Interface** - Real-time monitoring at port 5000  
✅ **Device Testing** - Test audio on any discovered Chromecast  
✅ **Multiple Prayer Sources** - ICCI and Naas timetables  
✅ **Production Ready** - Health checks and automatic restarts  

## Web Interface

- **Device Management:** http://localhost:5000/chromecasts
- **Audio Testing:** http://localhost:5000/test  
- **Logs:** http://localhost:5000/logs
- **Settings:** http://localhost:5000/settings

## Quick Links

- 📖 **Full Documentation:** https://github.com/shenhab/Automated-Azan
- 🐛 **Issues:** https://github.com/shenhab/Automated-Azan/issues
- ⭐ **Star on GitHub:** https://github.com/shenhab/Automated-Azan

## Support

Network issues? Device not found? Use the web interface for troubleshooting and device discovery.

**🕌 Simple, reliable, automated prayer announcements.**
