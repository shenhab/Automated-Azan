#!/bin/sh
# Docker Entrypoint Script

# Wait for network to be ready by pinging a reliable host (Google's DNS)
echo "-------------------------------------"
echo "--- Checking Network Connectivity ---"
echo "-------------------------------------"
while ! ping -c 1 -W 1 8.8.8.8; do
    echo "Network not ready, waiting 5 seconds to retry..."
    sleep 5
done
echo "✅ Network is up."
echo "-------------------------------------"
echo "--- Starting Application ---"
echo "-------------------------------------"

# Execute the main container command (CMD in Dockerfile)
exec "$@"
