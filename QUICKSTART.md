# 🕌 Automated Azan - Quick Start

## 30-Second Setup

### Production (Docker) - Recommended
```bash
git clone https://github.com/shenhab/Automated-Azan.git
cd Automated-Azan
cp adahn.config.example adahn.config
nano adahn.config  # Configure your speaker name and location
make deploy
```
**Done!** Access at http://localhost:5000

### Development (uv)
```bash
git clone https://github.com/shenhab/Automated-Azan.git
cd Automated-Azan
make setup
cp adahn.config.example adahn.config
nano adahn.config
make run  # or 'make web' for web interface
```

## Configuration
Edit `adahn.config`:
```ini
[Settings]
speakers-group-name = Living Room speakers  # Your Google Home name
location = Leeds, UK                        # Your city
prayer_source = icci                        # icci or naas
```

## What You Get
✅ Automatic prayer announcements on your Google Home  
✅ Professional web interface at http://localhost:5000  
✅ Device testing and management  
✅ Real-time prayer time updates  
✅ Production-ready Docker deployment  

## Commands
```bash
make deploy      # Deploy with Docker
make setup       # Setup development
make help        # Show all commands
```

**🕌 May this help you maintain your prayers with ease.**
