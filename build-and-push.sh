#!/bin/bash

# Build and Push Script for Automated Azan Docker Image
# This script ensures Portainer compatibility

set -e

# Configuration
IMAGE_NAME="shenhab/athan"
TAG="latest"
FULL_IMAGE_NAME="${IMAGE_NAME}:${TAG}"

echo "🔨 Building Docker image: ${FULL_IMAGE_NAME}"

# Build the image with proper tags for multi-platform support
docker build \
  --platform linux/amd64,linux/arm64 \
  -t "${FULL_IMAGE_NAME}" \
  --label "org.opencontainers.image.source=https://github.com/shenhab/Automated-Azan" \
  --label "org.opencontainers.image.description=Automated Islamic Prayer Time announcements via Chromecast" \
  --label "org.opencontainers.image.licenses=MIT" \
  .

echo "✅ Docker image built successfully"

# Test the image locally before pushing
echo "🧪 Testing the image locally..."
docker run --rm --name test-azan -d -p 5001:5000 "${FULL_IMAGE_NAME}"

# Wait for container to start
sleep 10

# Check if the web interface is accessible
if curl -f http://localhost:5001/ > /dev/null 2>&1; then
    echo "✅ Image test passed - web interface is accessible"
    docker stop test-azan
else
    echo "❌ Image test failed - web interface not accessible"
    docker stop test-azan
    exit 1
fi

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
echo "docker run -d --name automated-azan --network host --restart unless-stopped ${FULL_IMAGE_NAME}"
