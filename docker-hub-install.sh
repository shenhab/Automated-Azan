#!/bin/bash

# Automated Azan - Quick Docker Hub Installation Script
# This script provides an easy way to install from Docker Hub with defaults

set -e

echo "🕌 Automated Azan - Quick Installation from Docker Hub"
echo "===================================================="

# Check if Docker is installed
if ! command -v docker &> /dev/null; then
    echo "❌ Docker is not installed. Please install Docker first:"
    echo "   https://docs.docker.com/get-docker/"
    exit 1
fi

# Create configuration directory
CONFIG_DIR="$HOME/azan-config"
echo "📁 Creating configuration directory: $CONFIG_DIR"
mkdir -p "$CONFIG_DIR"

# Get user preferences
echo ""
echo "🔧 Configuration Setup:"
read -p "Enter your Chromecast/Google Home name (default: athan): " SPEAKER_NAME
read -p "Enter your location (naas/icci, default: naas): " LOCATION
read -p "Enter your timezone (default: UTC): " TIMEZONE

# Set defaults
SPEAKER_NAME=${SPEAKER_NAME:-"athan"}
LOCATION=${LOCATION:-"naas"}
TIMEZONE=${TIMEZONE:-"UTC"}

# Create configuration file
echo "📝 Creating configuration file..."
cat > "$CONFIG_DIR/adahn.config" << EOF
[Settings]
# Your Chromecast/Google Home speaker name (case-sensitive)
speakers-group-name = ${SPEAKER_NAME}

# Prayer time location (naas, icci, or custom location)
location = ${LOCATION}

# Optional: Enable pre-Fajr Quran
# pre_fajr_enabled = True
EOF

echo "✅ Configuration created at: $CONFIG_DIR/adahn.config"

# Pull and run the container
echo ""
echo "🐳 Deploying Automated Azan from Docker Hub..."

# Stop existing container if it exists
if docker ps -a --format "table {{.Names}}" | grep -q "^athan$"; then
    echo "🛑 Stopping existing container..."
    docker stop athan || true
    docker rm athan || true
fi

# Run the container
docker run -d \
  --name athan \
  --network host \
  --restart unless-stopped \
  -v "$CONFIG_DIR:/app/config" \
  -v azan_logs:/var/log \
  -v azan_data:/app/data \
  -e TZ="$TIMEZONE" \
  shenhab/athan:latest

echo ""
echo "🎉 Deployment Complete!"
echo "======================================"
echo "🕌 Automated Azan is now running!"
echo ""
echo "🌐 Web Interface: http://localhost:5000"
echo "🔊 Device Management: http://localhost:5000/chromecasts"
echo "🧪 Test Audio: http://localhost:5000/test"
echo "⚙️  Settings: http://localhost:5000/settings"
echo ""
echo "📋 Container Management:"
echo "   View logs: docker logs athan"
echo "   Stop: docker stop athan"
echo "   Restart: docker restart athan"
echo ""
echo "📁 Configuration file: $CONFIG_DIR/adahn.config"
echo "💡 Edit the config file and restart the container to apply changes"
echo ""
echo "🔍 Next Steps:"
echo "1. Visit http://localhost:5000/chromecasts to discover your devices"
echo "2. Test audio playback at http://localhost:5000/test"
echo "3. Verify prayer times are correct for your location"
echo ""
echo "🆘 Need help? Visit: https://github.com/shenhab/Automated-Azan"
