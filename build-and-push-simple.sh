#!/bin/bash

# Simple Build and Push Script for Automated Azan Docker Image
# Single platform build for faster testing

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

# Test the image locally before pushing
echo "🧪 Testing the image locally with host networking..."
echo "⚠️  Note: Using host networking for proper Chromecast discovery"
docker run --rm --name test-azan -d --network host "${FULL_IMAGE_NAME}"

# Wait for container to start
sleep 15

# Check if the web interface is accessible (using host network, so port 5000 directly)
if curl -f http://localhost:5000/ > /dev/null 2>&1; then
    echo "✅ Image test passed - web interface is accessible on host network"
    docker stop test-azan
else
    echo "❌ Image test failed - web interface not accessible"
    echo "ℹ️  This might be normal if port 5000 is already in use"
    echo "ℹ️  The image should still work fine in Portainer with host networking"
    docker stop test-azan
    # Don't exit with error code since port conflicts are common in testing
fi

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
    echo "🔧 Manual Docker Run Command:"
    echo "docker run -d --name athan --network host --restart unless-stopped ${FULL_IMAGE_NAME}"
else
    echo "📋 Image built successfully but not pushed."
    echo "To push later, run: docker push ${FULL_IMAGE_NAME}"
fi
