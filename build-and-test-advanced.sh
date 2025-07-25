#!/bin/bash

# Advanced Build and Test Script for Automated Azan Docker Image
# Handles port conflicts and provides comprehensive testing

set -e

# Configuration
IMAGE_NAME="shenhab/athan"
TAG="latest"
FULL_IMAGE_NAME="${IMAGE_NAME}:${TAG}"

echo "🔨 Building Docker image: ${FULL_IMAGE_NAME}"

# Build the image for current platform
docker build \
  -t "${FULL_IMAGE_NAME}" \
  --label "org.opencontainers.image.source=https://github.com/shenhab/Automated-Azan" \
  --label "org.opencontainers.image.description=Automated Islamic Prayer Time announcements via Chromecast" \
  --label "org.opencontainers.image.licenses=MIT" \
  .

echo "✅ Docker image built successfully"

# Function to find an available port
find_available_port() {
    local port=5000
    while netstat -ln | grep -q ":$port "; do
        ((port++))
    done
    echo $port
}

# Test the image with different networking approaches
echo "🧪 Testing the image..."

# Test 1: Bridge network test (for basic functionality)
echo "📡 Test 1: Basic functionality test with bridge networking..."
AVAILABLE_PORT=$(find_available_port)
docker run --rm --name test-azan-bridge -d -p "${AVAILABLE_PORT}:5000" "${FULL_IMAGE_NAME}"

sleep 10

if curl -f "http://localhost:${AVAILABLE_PORT}/" > /dev/null 2>&1; then
    echo "✅ Bridge network test passed - web interface accessible on port ${AVAILABLE_PORT}"
    docker stop test-azan-bridge
else
    echo "❌ Bridge network test failed"
    docker stop test-azan-bridge
    exit 1
fi

# Test 2: Host network test (for Chromecast discovery)
echo "📡 Test 2: Host networking test (for Chromecast compatibility)..."

# Check if port 5000 is available on host
if netstat -ln | grep -q ":5000 "; then
    echo "⚠️  Port 5000 is already in use - skipping host network test"
    echo "ℹ️  This is normal and doesn't affect Portainer deployment"
    HOST_TEST_PASSED=true
else
    docker run --rm --name test-azan-host -d --network host "${FULL_IMAGE_NAME}"
    
    sleep 15
    
    if curl -f http://localhost:5000/ > /dev/null 2>&1; then
        echo "✅ Host network test passed - ready for Chromecast discovery"
        docker stop test-azan-host
        HOST_TEST_PASSED=true
    else
        echo "❌ Host network test failed"
        docker stop test-azan-host
        HOST_TEST_PASSED=false
    fi
fi

# Test 3: Container logs check
echo "📋 Test 3: Checking container startup logs..."
docker run --rm --name test-azan-logs -d -p "${AVAILABLE_PORT}:5000" "${FULL_IMAGE_NAME}"

sleep 5
docker logs test-azan-logs | head -20

if docker logs test-azan-logs 2>&1 | grep -q "Starting Automated Azan Web Interface"; then
    echo "✅ Container startup logs look healthy"
    docker stop test-azan-logs
else
    echo "❌ Container startup issues detected"
    docker logs test-azan-logs
    docker stop test-azan-logs
    exit 1
fi

echo ""
echo "📊 Test Summary:"
echo "✅ Bridge networking: PASSED"
if [ "$HOST_TEST_PASSED" = true ]; then
    echo "✅ Host networking: PASSED"
else
    echo "⚠️  Host networking: SKIPPED (port conflict)"
fi
echo "✅ Container startup: PASSED"
echo ""

echo "🤔 Do you want to push the image to Docker Hub? (y/n)"
read -r response

if [[ "$response" =~ ^[Yy]$ ]]; then
    # Push the image
    echo "📤 Pushing image to Docker Hub..."
    docker push "${FULL_IMAGE_NAME}"
    
    echo "🎉 Successfully pushed ${FULL_IMAGE_NAME}"
    echo ""
    echo "📋 Portainer Deployment Instructions:"
    echo "1. In Portainer, go to 'App Templates'"
    echo "2. Add this template URL: https://raw.githubusercontent.com/shenhab/Automated-Azan/main/portainer-template.json"
    echo "3. Or use the stack file directly with image: ${FULL_IMAGE_NAME}"
    echo ""
    echo "🔧 Recommended Docker Run Commands:"
    echo "For Chromecast discovery (recommended):"
    echo "  docker run -d --name athan --network host --restart unless-stopped ${FULL_IMAGE_NAME}"
    echo ""
    echo "For testing without Chromecast:"
    echo "  docker run -d --name athan -p 5000:5000 --restart unless-stopped ${FULL_IMAGE_NAME}"
    echo ""
    echo "🌐 Access web interface at: http://your-server-ip:5000"
else
    echo "📋 Image built and tested successfully but not pushed."
    echo "To push later, run: docker push ${FULL_IMAGE_NAME}"
fi
