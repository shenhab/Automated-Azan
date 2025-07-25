#!/bin/bash

# Automated Azan - Portainer Deployment Helper Script
# This script helps prepare your environment for Portainer deployment

set -e

echo "🕌 Automated Azan - Portainer Deployment Helper"
echo "=============================================="

# Check if running as root (needed for Docker operations)
if [[ $EUID -eq 0 ]]; then
   echo "⚠️  This script should not be run as root"
   echo "Please run as a regular user with Docker access"
   exit 1
fi

# Check if Docker is installed and running
echo "� Checking dependencies..."
if [[ -f "Pipfile" ]]; then
    echo "✅ Found Pipfile - using pipenv for dependency management"
    if command -v pipenv &> /dev/null; then
        echo "✅ pipenv is installed"
    else
        echo "⚠️  pipenv not found. Installing pipenv..."
        pip install --user pipenv
        export PATH="$HOME/.local/bin:$PATH"
    fi
else
    echo "❌ Pipfile not found"
    exit 1
fi
if ! command -v docker &> /dev/null; then
    echo "❌ Docker is not installed"
    echo "Please install Docker first: https://docs.docker.com/get-docker/"
    exit 1
fi

if ! docker info &> /dev/null; then
    echo "❌ Docker is not running or you don't have access"
    echo "Please start Docker or add your user to the docker group"
    exit 1
fi

echo "✅ Docker is installed and running"

# Check if Portainer is accessible
echo "🔍 Checking Portainer configuration..."
read -p "Enter your Portainer URL (e.g., http://localhost:9000): " PORTAINER_URL

if [[ -z "$PORTAINER_URL" ]]; then
    echo "⚠️  No Portainer URL provided. Please make sure Portainer is installed."
    echo "Install Portainer: docker run -d -p 8000:8000 -p 9000:9000 --name=portainer --restart=always -v /var/run/docker.sock:/var/run/docker.sock -v portainer_data:/data portainer/portainer-ce"
fi

# Check configuration file
echo "🔍 Checking configuration file..."
if [[ ! -f "adahn.config" ]]; then
    echo "❌ Configuration file 'adahn.config' not found"
    echo "Creating a sample configuration file..."
    
    cat > adahn.config << EOF
[Settings]
speakers-group-name = athan
location = icci
EOF
    
    echo "✅ Created sample 'adahn.config' file"
    echo "Please edit it with your settings before deployment"
    
    read -p "Enter your Chromecast device name (default: athan): " DEVICE_NAME
    read -p "Enter your location (icci or naas, default: icci): " LOCATION
    
    DEVICE_NAME=${DEVICE_NAME:-athan}
    LOCATION=${LOCATION:-icci}
    
    cat > adahn.config << EOF
[Settings]
speakers-group-name = ${DEVICE_NAME}
location = ${LOCATION}
EOF
    
    echo "✅ Updated configuration file with your settings"
else
    echo "✅ Configuration file found"
fi

# Build Docker images
echo "🔨 Building Docker images..."
echo "This may take a few minutes..."

if docker build -t shenhab/athan:latest .; then
    echo "✅ Main application image built successfully"
else
    echo "❌ Failed to build main application image"
    exit 1
fi

if docker build -f Dockerfile.web -t shenhab/athan-web:latest .; then
    echo "✅ Web interface image built successfully"
else
    echo "❌ Failed to build web interface image"
    exit 1
fi

# Create volumes
echo "📁 Creating Docker volumes..."
docker volume create azan_logs || true
docker volume create azan_config || true
echo "✅ Volumes created"

# Optional: Test deployment locally
read -p "Would you like to test the deployment locally first? (y/N): " TEST_LOCAL

if [[ $TEST_LOCAL =~ ^[Yy]$ ]]; then
    echo "🧪 Testing local deployment..."
    
    # Stop any existing containers
    docker stop athan athan-web 2>/dev/null || true
    docker rm athan athan-web 2>/dev/null || true
    
    # Start containers
    echo "Starting main application..."
    docker run -d \
        --name athan \
        --network host \
        --restart unless-stopped \
        -v azan_logs:/var/log \
        -v azan_config:/app/config \
        -v $(pwd)/adahn.config:/app/adahn.config:ro \
        -e TZ=Europe/Dublin \
        shenhab/athan:latest
    
    echo "Starting web interface..."
    docker run -d \
        --name athan-web \
        --restart unless-stopped \
        -p 5000:5000 \
        -v azan_logs:/var/log \
        -v azan_config:/app/config \
        -v $(pwd)/adahn.config:/app/adahn.config:ro \
        -e TZ=Europe/Dublin \
        shenhab/athan-web:latest
    
    echo "⏳ Waiting for containers to start..."
    sleep 5
    
    # Check container status
    if docker ps | grep -q "automated-azan-main"; then
        echo "✅ Main application container is running"
    else
        echo "❌ Main application container failed to start"
        docker logs automated-azan-main
    fi
    
    if docker ps | grep -q "automated-azan-web"; then
        echo "✅ Web interface container is running"
        echo "🌐 Web interface should be accessible at: http://localhost:5000"
    else
        echo "❌ Web interface container failed to start"
        docker logs automated-azan-web
    fi
    
    echo ""
    echo "Test containers are running. You can:"
    echo "1. Check the web interface at http://localhost:5000"
    echo "2. View logs: docker logs automated-azan-main"
    echo "3. Stop test containers: docker stop automated-azan-main automated-azan-web"
    echo ""
    read -p "Press Enter when you're ready to proceed with Portainer deployment..."
    
    # Clean up test containers
    docker stop automated-azan-main automated-azan-web 2>/dev/null || true
    docker rm automated-azan-main automated-azan-web 2>/dev/null || true
fi

# Generate deployment instructions
echo "📋 Generating deployment files..."

# Create environment file for easy copying to Portainer
cat > .env.portainer << EOF
# Environment variables for Portainer deployment
TZ=Europe/Dublin
LOG_FILE=/var/log/azan_service.log
EOF

echo "✅ Environment file created: .env.portainer"

# Final instructions
echo ""
echo "🎉 Preparation completed successfully!"
echo "================================================"
echo ""
echo "Next steps for Portainer deployment:"
echo ""
echo "1. Open your Portainer web interface: ${PORTAINER_URL}"
echo "2. Go to 'Stacks' → 'Add stack'"
echo "3. Name your stack: 'automated-azan'"
echo "4. Upload or copy-paste the contents of: portainer-stack.yml"
echo "5. Add environment variables from: .env.portainer"
echo "6. Deploy the stack"
echo ""
echo "Files created:"
echo "- portainer-stack.yml (Portainer stack configuration)"
echo "- Dockerfile.web (Web interface Docker image)"
echo "- .env.portainer (Environment variables template)"
echo "- PORTAINER_DEPLOYMENT.md (Detailed deployment guide)"
echo ""
echo "Docker images built:"
echo "- shenhab/athan:latest (Main application)"
echo "- shenhab/athan-web:latest (Web interface)"
echo ""
echo "📖 For detailed instructions, see: PORTAINER_DEPLOYMENT.md"
echo ""
echo "🕌 Happy praying! May your automated azan serve you well."
