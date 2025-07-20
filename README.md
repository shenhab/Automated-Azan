# **Automated Azan ⏰🕌**  
*A Python-based tool to automate Azan (Islamic call to prayer) notifications using Google Home devices.*

## **📌 Features**
✅ Plays Azan automatically based on your configured location and prayer times  
✅ Supports Google Home speakers and speaker groups via `pychromecast`  
✅ Multiple prayer time sources (ICCI and Naas mosque timetables)  
✅ Special handling for Fajr prayer with optional pre-Fajr Quran radio  
✅ Runs continuously as a reliable background service  
✅ Automatic time synchronization via NTP  
✅ Comprehensive logging for debugging and monitoring  
✅ **Docker support for easy deployment**

## **🧩 Components**
The application consists of several components working together:

1. **`main.py`**: Main application entry point with the AthanScheduler class
2. **`prayer_times_fetcher.py`**: Fetches prayer times from different sources
3. **`chromecast_manager.py`**: Manages Google Home integration and audio playback
4. **`adahn.config`**: Configuration file for specifying location and device settings
5. **`azan.service`**: SystemD service file for running as a background service
6. **`.env`**: Optional environment file for Twilio API credentials
7. **`Dockerfile`** & **`docker-compose.yml`**: Docker deployment configuration

## **🚀 Installation & Setup**

### **� Prerequisites**
- **Python 3.9+** (recommended: Python 3.11)
- **pipenv** for dependency management
- **Chromecast/Google Home device** on the same network
- **Docker** (for containerized deployment)

### **🐍 Option A: Python Development Setup (pipenv)**

For development or local testing:

```bash
# Clone the repository
git clone https://github.com/shenhab/Automated-Azan.git
cd Automated-Azan

# Install pipenv if not already installed
pip install pipenv

# Install dependencies
pipenv install

# Configure your settings
cp adahn.config.example adahn.config
nano adahn.config

# Run the application
pipenv run python main.py

# Or activate the virtual environment
pipenv shell
python main.py
```

#### **Quick Setup Script**
```bash
# Run the automated setup script
./setup-dev.sh
```

### **�🐳 Option B: Docker Installation (Recommended for Production)**

Docker provides an isolated environment with all dependencies pre-configured.

#### **Prerequisites**
- **Docker** and **Docker Compose** installed ([Installation Guide](https://docs.docker.com/get-docker/))
- **Google Home/Chromecast device** on the same network

#### **Quick Start**
```bash
# Clone the repository
git clone https://github.com/shenhab/Automated-Azan.git
cd Automated-Azan

# Configure your location and speaker in adahn.config
nano adahn.config

# Build and run with Docker Compose
docker-compose up -d
```

#### **Docker Configuration**
The Docker setup includes:
- All system dependencies (Chrome, network tools)
- Proper network configuration for Chromecast discovery
- Persistent volumes for logs and data
- Environment variable support

**Important Docker Notes:**
- The container runs with `network_mode: host` to enable Chromecast discovery
- Logs are mounted to `./logs/` directory on your host
- Prayer timetable data is stored in `./data/` directory

#### **Docker Commands**
```bash
# View logs
docker-compose logs -f

# Stop the service
docker-compose down

# Rebuild after changes
docker-compose up --build -d

# Check container status
docker-compose ps
```

### **📦 Option B: Manual Installation**
### **1️⃣ Prerequisites**
Before you begin, ensure you have the following installed:

- **Python 3.8+** ([Installation Guide](https://www.python.org/downloads/))
- **Pipenv** for dependency management ([Installation Guide](https://pipenv.pypa.io/en/latest/))
- **Git** for cloning the repository ([Download Git](https://git-scm.com/))
- **Google Home/Chromecast device** on the same network
- **Twilio account** (optional, for WhatsApp notifications)

### **2️⃣ Installation Steps**
Run the following commands in your terminal:

```bash
# Clone the repository
git clone https://github.com/shenhab/Automated-Azan.git

# Navigate to the project directory
cd Automated-Azan

# Install dependencies using Pipenv
pipenv install
```

### **3️⃣ Configure the Application**

#### Basic Configuration
Edit the `adahn.config` file to set your speaker group name and preferred prayer time source:

```ini
[Settings]
speakers-group-name = YourGoogleSpeakerName
location = naas  # Options: naas, icci
```

### **4️⃣ Run the Application**
You can run the application in two ways:

#### a) Direct Execution
Start the application directly:

```bash
pipenv run python main.py
```

#### b) As a System Service (Recommended)
For continuous operation, install as a systemd service:

```bash
# Copy service file (may need sudo)
sudo cp azan.service /etc/systemd/system/

# Enable and start the service
sudo systemctl enable azan.service
sudo systemctl start azan.service

# Check service status
sudo systemctl status azan.service
```

## **🔍 Prayer Time Sources**
The application supports multiple prayer time sources:

### **ICCI (Islamic Cultural Centre of Ireland)**
Uses the official ICCI API for Dublin prayer times.

### **Naas Masjid**
Fetches prayer times from Naas Masjid's Mawaqit page.

## **⚙️ Advanced Configuration**

### **Prayer Time Handling**
- Fajr prayers play a special Fajr adhan sound
- 45 minutes before Fajr, Quran radio starts playing (can be modified in code)

### **Logs**
Logs are stored in `/var/log/azan_service.log` and include:
- Prayer time fetching status
- Scheduled prayer announcements
- Connection issues with Google Home devices
- WhatsApp notification status

## **🔧 Troubleshooting**

### **Google Home Connection Issues**
- Ensure your Google Home device is on the same network as the server
- Verify the device name in `adahn.config` matches exactly
- Check firewall settings to allow mDNS discovery

### **Missing Prayer Times**
- Check internet connectivity to prayer time sources
- Verify that the timetable JSON files exist and are valid
- Force a refresh by setting `force_download=True` in the code

### **NTP Synchronization Problems**
- Ensure the system has internet access to reach NTP servers
- Check the system's time zone configuration
- Run `timedatectl status` to verify NTP synchronization

### **Service Not Starting**
- Check systemd logs with `journalctl -u azan.service`
- Verify Python and dependency installation
- Ensure proper file permissions

## **💡 Future Improvements**
- Web interface for easy configuration
- Support for more prayer time sources
- Customizable adhan sounds
- Mobile app integration
- Multiple language support
- Prayer time adjustments and calibration

## **📜 License**
This project is licensed under the **MIT License**. Feel free to use, modify, and contribute!

## **🤝 Contributing**
Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request
