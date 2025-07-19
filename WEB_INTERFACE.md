# 🕌 Automated Azan Web Interface

A modern, intuitive web interface for managing your Islamic prayer automation system.

## ✨ Features

### 📡 **Smart Chromecast Discovery**
- **Auto-discover** Chromecast devices on your network
- **Test connectivity** before configuration  
- **One-click setup** for speaker selection
- **Real-time device status** monitoring

### 🏠 **Comprehensive Dashboard**
- **Live prayer times** with countdown to next prayer
- **System status** at a glance
- **Real-time updates** via WebSocket
- **Quick access** to all features

### ⚙️ **Easy Configuration**
- **Visual settings** for prayer location and speakers
- **Instant validation** and feedback
- **Test configuration** with detailed results
- **No command-line editing** required

### 📋 **Advanced Logging**
- **Real-time log streaming**
- **Search and filter** capabilities
- **Syntax highlighting** for easy reading
- **Export and analysis** tools

## 🚀 Quick Start

1. **Start the services:**
   ```bash
   docker-compose up -d
   ```

2. **Open your browser:**
   ```
   http://localhost:5000
   ```

3. **Configure your system:**
   - Go to **Chromecast Devices** → **Discover Devices**
   - **Select** your preferred speaker/group
   - **Test** the connection
   - **Save** your configuration

4. **Monitor your system:**
   - View **Dashboard** for prayer times and status
   - Check **Logs** for detailed activity
   - Adjust **Settings** as needed

## 🖥️ Interface Overview

### Dashboard
- **Prayer Times**: Today's schedule with next prayer highlighted
- **Status Cards**: Configuration, scheduler, and device status
- **Recent Activity**: Latest log entries
- **Quick Stats**: System health at a glance

### Chromecast Management
- **Device Discovery**: Automatic network scanning
- **Device Cards**: Visual representation of found devices
- **Connection Testing**: Verify device connectivity
- **Selection Interface**: Easy speaker configuration

### Settings Panel
- **Prayer Configuration**: Location and speaker settings
- **Quick Actions**: Common tasks and shortcuts
- **Status Overview**: Current system configuration
- **Help & Tips**: Usage guidance

### Logs Viewer
- **Real-time Streaming**: Live log updates
- **Filtering**: By log level (Error, Warning, Info)
- **Search**: Full-text search with regex support
- **Statistics**: Error counts and system metrics

## 🔧 Configuration Options

The web interface stores configuration in the same `adahn.config` file used by the main application:

```ini
[Settings]
# Automatically updated via web interface
speakers-group-name = Your Chromecast Device Name
location = naas  # or icci
```

## 🌐 Network Requirements

- **Port 5000**: Web interface access
- **Host Networking**: Required for Chromecast discovery
- **Same Network**: Device and Chromecasts must be on same subnet

## 🛠️ Advanced Usage

### Keyboard Shortcuts
- `Ctrl+R`: Refresh current page/data
- `Ctrl+F`: Open search (in logs viewer)

### API Endpoints
The web interface exposes REST APIs for automation:
- `GET /api/prayer-times`: Current prayer schedule
- `POST /api/discover-chromecasts`: Trigger device discovery
- `POST /api/save-config`: Update configuration
- `POST /api/test-chromecast`: Test device connectivity

### Real-time Updates
WebSocket connection provides live updates for:
- Prayer time changes
- Device discovery results
- Configuration updates
- System status changes

## 🔒 Security Considerations

- **Local Network Only**: Interface binds to all interfaces but should be firewall protected
- **No Authentication**: Designed for trusted local network use
- **Environment Variables**: Sensitive Twilio credentials stored in `.env` file
- **Read-only Logs**: Log viewer provides read-only access to system logs

## 🐛 Troubleshooting

### Web Interface Not Loading
- Check if port 5000 is available: `netstat -tulpn | grep 5000`
- Verify Docker container is running: `docker-compose ps`
- Check container logs: `docker-compose logs web-interface`

### Chromecast Discovery Issues
- Ensure host networking is enabled in Docker
- Verify devices are on same network subnet
- Check firewall settings for mDNS/Bonjour traffic
- Try manual discovery from Google Home app

### Configuration Not Saving
- Check file permissions on `adahn.config`
- Verify volume mounts in docker-compose.yml
- Review web interface logs for errors
- Ensure container has write access to config file

## 📱 Mobile Compatibility

The interface is fully responsive and works great on:
- **Desktop browsers** (Chrome, Firefox, Safari, Edge)
- **Tablet devices** (iPad, Android tablets)
- **Mobile phones** (iOS Safari, Chrome Mobile)

Touch-friendly interface with:
- **Large buttons** for easy tapping
- **Responsive layout** that adapts to screen size
- **Mobile-optimized** navigation and menus
- **Zoom-friendly** text and controls

---

**🎯 The web interface makes managing your Islamic prayer automation system as simple as a few clicks!**
