# Portainer Deployment Guide for Automated Azan

This guide helps you deploy the Automated Azan application using Portainer with the `shenhab/athan:latest` image.

## Prerequisites

- Portainer installed and running
- Docker environment with network access to Chromecast devices
- Host networking capability (required for Chromecast discovery)

## Deployment Methods

### Method 1: Using App Template (Recommended)

1. **Add Custom Template**
   - Go to **Settings** → **App Templates**
   - Add custom template URL: `https://raw.githubusercontent.com/shenhab/Automated-Azan/main/portainer-template.json`
   - Click **App Templates** in the sidebar

2. **Deploy from Template**
   - Find "Automated Azan" in the templates
   - Fill in the environment variables:
     - **TZ**: Your timezone (e.g., `Europe/Dublin`, `America/New_York`)
     - **SPEAKERS_GROUP_NAME**: Your Chromecast device name (default: `Adahn`)
     - **LOCATION**: Prayer times location (`icci` or `naas`)
   - Click **Deploy the stack**

### Method 2: Using Stack File

1. **Create New Stack**
   - Go to **Stacks** → **Add Stack**
   - Name: `automated-azan`

2. **Use the Stack File**
   Copy and paste the following stack configuration:

```yaml
version: '3.8'

services:
  automated-azan:
    image: shenhab/athan:latest
    container_name: automated-azan-main
    restart: unless-stopped
    
    # REQUIRED: Use host network for Chromecast discovery
    network_mode: host
    
    environment:
      - LOG_FILE=/var/log/azan_service.log
      - TZ=Europe/Dublin  # Change to your timezone
      - PYTHONUNBUFFERED=1
    
    volumes:
      # Configuration - using volume for better security and persistence
      - azan_config:/app/config
      
      # Persistent data storage
      - azan_logs:/var/log
      - azan_data:/app/data
    
    # Health check for monitoring
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:5000/", "||", "exit", "1"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 30s
    
    command: python main.py
    
    # Container logging configuration
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"

volumes:
  azan_logs:
    driver: local
  azan_data:
    driver: local
  azan_config:
    driver: local
```

3. **Deploy the Stack**
   - Review the configuration
   - Update the `TZ` environment variable to your timezone
   - Click **Deploy the stack**

### Method 3: Manual Container Creation

1. **Create Container**
   - Go to **Containers** → **Add Container**
   - **Image**: `shenhab/athan:latest`
   - **Name**: `automated-azan`

2. **Network Configuration**
   - **Network**: Select "Host" (REQUIRED for Chromecast discovery)

3. **Environment Variables**
   Add the following environment variables:
   - `LOG_FILE` = `/var/log/azan_service.log`
   - `TZ` = `Your_Timezone` (e.g., `Europe/Dublin`)
   - `PYTHONUNBUFFERED` = `1`

4. **Volumes**
   Create the following volume mappings:
   - `azan_config` → `/app/config`
   - `azan_logs` → `/var/log`
   - `azan_data` → `/app/data`

5. **Restart Policy**
   - Set to "Unless Stopped"

6. **Deploy Container**

## Post-Deployment Configuration

### 1. Access Web Interface
- Open your browser to `http://your-server-ip:5000`
- The web interface provides:
  - Dashboard with prayer times
  - Chromecast device discovery and testing
  - Configuration management
  - Log viewing

### 2. Initial Setup
1. **Configure Chromecast**
   - Go to "Chromecasts" tab
   - Click "Discover Devices" to find your Chromecast devices
   - Test connection to ensure proper discovery

2. **Update Settings**
   - Go to "Settings" tab
   - Update "Speaker Group Name" to match your Chromecast device
   - Select appropriate location for prayer times
   - Save configuration

3. **Test Audio**
   - Use the "Test" tab to verify Adhan playback
   - Test both regular and Fajr Adhan

## Troubleshooting

### Common Issues

1. **No Chromecast Devices Found**
   - Ensure host networking is enabled
   - Verify Chromecast devices are on the same network
   - Check firewall settings

2. **Web Interface Not Accessible**
   - Verify port 5000 is not blocked
   - Check container logs in Portainer
   - Ensure container has started successfully

3. **Audio Playback Issues**
   - Test individual Chromecast devices
   - Verify media files are accessible
   - Check Chromecast device compatibility

### Viewing Logs
- In Portainer: Go to **Containers** → **automated-azan** → **Logs**
- In Web Interface: Use the "Logs" tab
- Docker command: `docker logs automated-azan-main`

## Environment Variables Reference

| Variable | Default | Description |
|----------|---------|-------------|
| `TZ` | `UTC` | Container timezone |
| `LOG_FILE` | `/var/log/azan_service.log` | Log file location |
| `PYTHONUNBUFFERED` | `1` | Python output buffering |

## Volume Mounts Reference

| Volume | Container Path | Purpose |
|--------|----------------|---------|
| `azan_config` | `/app/config` | Configuration files |
| `azan_logs` | `/var/log` | Application logs |
| `azan_data` | `/app/data` | Prayer times data |

## Network Requirements

- **Host Network Mode**: Required for Chromecast discovery
- **Port 5000**: Web interface access
- **Multicast DNS**: Required for Chromecast discovery
- **Ports 8008/8009**: Chromecast communication (handled by host network)

## Security Considerations

- The application runs as non-root user (appuser, UID 1000)
- Sensitive configuration stored in Docker volumes
- Web interface runs on localhost only by default
- No external dependencies required after deployment

## Updating the Application

To update to the latest version:
1. Stop the current container/stack
2. Pull the latest image: `docker pull shenhab/athan:latest`
3. Restart the container/stack
4. Configuration and data will be preserved in volumes
