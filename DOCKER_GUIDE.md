# Docker Deployment Guide for Automated Azan

## Quick Start

1. **Clone and navigate to the repository:**
   ```bash
   git clone https://github.com/shenhab/Automated-Azan.git
   cd Automated-Azan
   ```

2. **Set up environment variables:**
   ```bash
   cp .env.example .env
   # Edit .env with your Twilio credentials (optional)
   nano .env
   ```

3. **Configure prayer location:**
   ```bash
   # Edit adahn.config to set your location (speaker will be configured via web interface)
   nano adahn.config
   ```

4. **Start the application with web interface:**
   ```bash
   # Using Docker Compose (recommended)
   docker-compose up -d
   
   # Or using the helper script
   ./docker-helper.sh setup
   ./docker-helper.sh start
   ```

5. **Access the web interface:**
   ```bash
   # Open your browser and go to:
   http://localhost:5000
   
   # Use the web interface to:
   # - Discover and select Chromecast devices
   # - Configure prayer settings  
   # - Monitor logs and status
   ```

6. **Monitor logs:**
   ```bash
   docker-compose logs -f
   # Or use the web interface logs page
   ```

## Web Interface Features

The included web interface provides an intuitive way to manage your Automated Azan system:

### 🏠 Dashboard
- **Real-time prayer times** with next prayer countdown
- **System status** monitoring (configuration, scheduler, speaker)  
- **Quick stats** and recent activity logs
- **Live updates** via WebSocket connection

### 📡 Chromecast Management
- **Auto-discovery** of Chromecast devices on your network
- **Device testing** to verify connectivity
- **One-click selection** of speaker/group for prayers
- **Real-time device status** and information

### ⚙️ Settings
- **Prayer configuration** (location, speaker selection)
- **Form validation** and instant feedback
- **Configuration testing** with detailed results
- **Quick actions** for common tasks

### 📋 Logs Viewer
- **Real-time log streaming** with auto-refresh
- **Log level filtering** (Error, Warning, Info, Debug)
- **Search functionality** with regex support
- **Syntax highlighting** and formatted display
- **Log statistics** and connection status

### 🔧 Advanced Features
- **WebSocket connectivity** for real-time updates
- **Responsive design** works on desktop and mobile
- **Keyboard shortcuts** (Ctrl+R to refresh, Ctrl+F to search)
- **Auto-scroll logs** with manual override
- **Background device discovery** every 30 seconds

## Configuration

### Environment Variables (.env)

Create a `.env` file with your Twilio credentials:

```bash
# Twilio Configuration (optional for WhatsApp notifications)
TWILIO_ACCOUNT_SID=your_account_sid_here
TWILIO_AUTH_TOKEN=your_auth_token_here
TWILIO_CONTENT_SID=your_content_sid_here
TWILIO_WHATSAPP_NUMBER=whatsapp:+14155238886
RECIPIENT_NUMBER=whatsapp:+your_phone_number_here

# Optional: Override log file location
LOG_FILE=/var/log/azan_service.log
```

### Prayer Configuration (adahn.config)

```ini
[Settings]
# Set the name of your Google Home speaker or speaker group
speakers-group-name = Adahn

# Choose your location (uncomment one):
# location = icci  # For ICCI mosque timetable
location = naas    # For Naas mosque timetable
```

## Docker Setup Details

### What the Docker Configuration Provides

1. **Isolated Environment**: All dependencies are pre-installed and configured
2. **Network Access**: Proper network configuration for Chromecast discovery
3. **Persistent Data**: Prayer timetables and logs are stored in mounted volumes
4. **Security**: Runs as non-root user
5. **Health Monitoring**: Built-in health checks

### Directory Structure

```
Automated-Azan/
├── data/           # Prayer timetable storage (mounted volume)
├── logs/           # Application logs (mounted volume)
├── .env            # Environment variables
├── adahn.config    # Prayer and speaker configuration
├── Dockerfile      # Docker image configuration
├── docker-compose.yml  # Docker Compose configuration
└── docker-helper.sh    # Helper script for easy management
```

### Network Requirements

The application needs to discover and communicate with Chromecast devices on your network:

- Container runs with `network_mode: host` for device discovery
- Requires ports 8008 and 8009 for Chromecast communication
- Avahi daemon is included for mDNS/Bonjour service discovery

## Troubleshooting

### Common Issues

1. **Chromecast Not Found**
   - Ensure your Docker host and Chromecast are on the same network
   - Verify the speaker name in `adahn.config` matches your device
   - Check that the container is running with host networking

2. **Permission Denied for Logs**
   ```bash
   sudo chown -R 1000:1000 logs/
   ```

3. **Prayer Times Not Updating**
   - Check network connectivity
   - Verify the location setting in `adahn.config`
   - Look at logs for download errors

4. **WhatsApp Notifications Not Working**
   - Verify Twilio credentials in `.env`
   - Check that the phone number is in correct format
   - Ensure Twilio WhatsApp sandbox is configured

### Viewing Logs

```bash
# All logs
docker-compose logs -f

# Just application logs
tail -f logs/azan_service.log

# Using helper script
./docker-helper.sh logs
```

### Debugging

```bash
# Enter container shell
./docker-helper.sh shell

# Check container status
./docker-helper.sh status

# Restart container
./docker-helper.sh restart
```

## Manual Docker Commands

If you prefer not to use Docker Compose:

```bash
# Build image
docker build -t automated-azan .

# Run container
docker run -d \
  --name automated-azan \
  --network host \
  --restart unless-stopped \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/logs:/var/log \
  -v $(pwd)/adahn.config:/app/adahn.config:ro \
  --env-file .env \
  automated-azan
```

## Security Considerations

1. **Environment Variables**: Store sensitive Twilio credentials in `.env` file
2. **File Permissions**: Ensure proper ownership of mounted volumes
3. **Network Access**: Container needs host network access for Chromecast discovery
4. **Non-root User**: Application runs as user `appuser` (UID 1000)

## Performance and Resources

- **Memory**: ~200MB RAM usage
- **CPU**: Minimal, mostly idle except during prayer times
- **Storage**: <100MB for application, logs grow over time
- **Network**: Periodic HTTP requests for prayer time updates

## Maintenance

### Updating Prayer Times
Prayer times are automatically updated monthly. You can force an update by restarting the container:

```bash
./docker-helper.sh restart
```

### Log Rotation
Consider setting up log rotation for the application logs:

```bash
# Add to crontab
0 0 * * * /usr/sbin/logrotate /path/to/logrotate.conf
```

### Backup
Important files to backup:
- `adahn.config` - Your configuration
- `.env` - Your credentials
- `data/` - Prayer timetable cache
