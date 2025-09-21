
# **Automated Azan ⏰🕌**
*Automated Islamic prayer announcements on Chromecast/Google Home speakers with professional web management interface*

## **🌟 Features**
✅ **Automated Prayer Scheduling** - Plays Azan automatically based on location and prayer times  
✅ **Chromecast Integration** - Works with Google Home speakers and speaker groups  
✅ **Professional Web Interface** - Complete management dashboard with real-time updates  
✅ **Device Testing** - Test Adhan playbook on any discovered speaker  
✅ **Multiple Prayer Sources** - ICCI and Naas mosque timetables  
✅ **Special Fajr Handling** - Different Adhan for Fajr with optional pre-prayer Quran  
✅ **Production Ready** - Docker deployment with health checks and monitoring  
✅ **Real-time Updates** - Socket.io integration for live status updates  

## **🚀 Quick Start**

### **� Direct from Docker Hub (Easiest)**
*Install directly from Docker Hub without cloning the repository*

```bash
# Quick install script (automatically configures everything)
curl -sSL https://raw.githubusercontent.com/shenhab/Automated-Azan/main/docker-hub-install.sh | bash
```

**Or manual installation:**
```bash
# Create configuration
mkdir -p ~/azan-config
cat > ~/azan-config/adahn.config << EOF
[Settings]
speakers-group-name = athan
location = naas
EOF

# Deploy container
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

**Access:** 🌐 http://localhost:5000

### **�🐳 Production Deployment (Docker)**
*Recommended for production use - single container with everything included*


```bash
# Clone and configure
git clone https://github.com/shenhab/Automated-Azan.git
cd Automated-Azan
cp adahn.config.example adahn.config
nano adahn.config  # Configure your location and speaker name

# Deploy with Docker
make deploy
```

**That's it!** Your Automated Azan system is now running:
- 🕌 **Prayer announcements**: Automatically scheduled based on your location
- 🌐 **Web interface**: http://localhost:5000
- 🔍 **Device management**: http://localhost:5000/chromecasts
- 🧪 **Audio testing**: http://localhost:5000/test

### **🔨 Native Executable (Nuitka)**
*High-performance single-file executable - 10x faster than Python!*

```bash
# 1. Clone and install build tools
git clone https://github.com/shenhab/Automated-Azan.git
cd Automated-Azan

# 2. Install build tools (platform-specific)
# Windows: Visual Studio Build Tools
# Linux: sudo apt install build-essential
# macOS: xcode-select --install

# 3. Build executable (5-15 minutes)
make build-nuitka

# 4. Configure and run
cp adahn.config.example adahn.config
nano adahn.config
./dist/AutomatedAzan  # Single 40MB executable!
```

### **🐍 Development Setup (uv)**
*For developers and local testing*

```bash
# Setup development environment
git clone https://github.com/shenhab/Automated-Azan.git
cd Automated-Azan
make setup     # Installs uv and dependencies
make run       # Run prayer scheduler
# OR
make web       # Run web interface only
```

## **⚙️ Configuration**

Edit `adahn.config` with your settings:

```ini
[Settings]
# Your Google Home speaker or speaker group name (case-sensitive)
speakers-group-name = athan

# Prayer time location
location = Leeds, UK

# Prayer time source (icci or naas)
prayer_source = icci

# Enable pre-Fajr Quran (45 minutes before Fajr)
pre_fajr_enabled = True
```
## **🎛️ Web Interface**

The integrated web dashboard provides:

- **📊 Dashboard**: Live status, next prayer info, and system overview
- **� Device Management**: Discover and manage Chromecast speakers  
- **🧪 Audio Testing**: Test Adhan playbook on any speaker
- **📋 Live Logs**: Real-time application logs with filtering
- **⚙️ Settings**: Configuration management via web interface
- **📱 Responsive Design**: Mobile-friendly Bootstrap interface

## **📦 Deployment Options**

### **Option 1: Docker Deployment (Recommended)**

**Single Command Production Deployment:**
```bash
make deploy
```

**Advanced Docker Commands:**
```bash
make docker-build     # Build container
make docker-run       # Start container
make docker-logs      # View logs
make docker-stop      # Stop container
make deploy-check     # Validate configuration
```

**Container Features:**
- ✅ Single unified container (Azan + Web interface)
- ✅ Host network access for Chromecast discovery
- ✅ Persistent volumes for configuration and logs
- ✅ Health checks and automatic restart
- ✅ Security hardening with non-root user
- ✅ Timezone support (defaults to Europe/Dublin)

### **Option 2: Development with uv**

**Quick Development Setup:**
```bash
make setup    # Setup environment
make install  # Install dependencies  
make test     # Run tests
make run      # Start main application
make deploy   # deploy the docker
```

**Development Commands:**
```bash
make shell           # Activate uv shell
make update          # Update dependencies
make test-chromecast # Test device discovery
make clean           # Clean temporary files
```

## **🔍 System Requirements**

**For Docker Deployment:**
- Docker & Docker Compose
- Network access to Chromecast devices

**For Development:**
- Python 3.9+ (recommended: Python 3.11)
- uv
- Network access to Chromecast devices

## **📖 How It Works**

1. **Prayer Time Fetching**: Automatically downloads prayer schedules from ICCI or Naas sources
2. **Scheduling**: Creates cron-like schedule for each prayer throughout the day
3. **Device Discovery**: Finds your Chromecast/Google Home speakers via network scan
4. **Audio Playback**: Streams appropriate Adhan audio files to selected speakers
5. **Web Management**: Provides real-time monitoring and manual testing capabilities
6. **Logging**: Comprehensive logging for debugging and monitoring

## **🛠️ Available Commands**

### **Production Deployment**
```bash
make deploy           # Full Docker deployment
make deploy-check     # Validate configuration
make docker-logs      # View container logs
make docker-stop      # Stop deployment
```

### **Development**
```bash
make setup           # Complete development setup
make run             # Run prayer scheduler
make web             # Run web interface
make test            # Run all tests
make test-chromecast # Test device discovery
```

### **Utilities**
```bash
make help            # Show all available commands
make clean           # Clean temporary files
make check           # Check system requirements
```

## **🔧 Troubleshooting**

### **Chromecast Discovery Issues**
- Ensure devices are on same network
- Verify speaker names match exactly (case-sensitive)
- Use web interface to test device discovery: http://localhost:5000/chromecasts

### **Docker Deployment Issues**
```bash
make deploy-check    # Validate all requirements
make docker-logs     # Check container logs
docker-compose ps    # Check container status
```

### **Development Issues**
```bash
make test-chromecast # Test device discovery
make test           # Run full test suite
uv run python web_interface.py  # Test web interface directly
```

## **📁 Project Structure**

```
Automated-Azan/
├── main.py                 # Main prayer scheduler
├── web_interface.py        # Web dashboard
├── chromecast_manager.py   # Device management
├── prayer_times_fetcher.py # Prayer time sources
├── adahn.config           # Configuration file
├── docker-compose.yml     # Production deployment
├── Makefile              # Build and deployment commands
├── templates/            # Web interface templates
├── Media/               # Adhan audio files
└── logs/                # Application logs
```

## **🔐 Security**

- Docker containers run as non-root user
- Configuration files mounted read-only where possible
- Persistent volumes for logs and data
- Health checks for monitoring
- No external network requirements (except for prayer time updates)

## **🎯 Production Ready**

This system is designed for reliable 24/7 operation:
- ✅ Automatic restart on failure
- ✅ Persistent configuration and logs
- ✅ Health monitoring
- ✅ Comprehensive error handling
- ✅ Time synchronization
- ✅ Professional web interface

## **📜 License**
MIT License - Feel free to use and modify

## **🤝 Contributing**
Contributions welcome! Please submit pull requests for improvements.

---

**🕌 May this tool help you maintain your prayers with ease and consistency.**
