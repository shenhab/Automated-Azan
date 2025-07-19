#!/bin/bash

# Automated Azan Docker Helper Script

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if Docker and Docker Compose are installed
check_docker() {
    if ! command -v docker &> /dev/null; then
        print_error "Docker is not installed. Please install Docker first."
        exit 1
    fi
    
    if ! command -v docker-compose &> /dev/null; then
        print_error "Docker Compose is not installed. Please install Docker Compose first."
        exit 1
    fi
    
    print_status "Docker and Docker Compose are installed."
}

# Create necessary directories
setup_directories() {
    print_status "Setting up directories..."
    mkdir -p data logs
    chmod 755 data logs
}

# Setup environment file
setup_env() {
    if [ ! -f .env ]; then
        print_warning ".env file not found. Creating from template..."
        cp .env.example .env
        print_warning "Please edit .env file with your Twilio credentials before running the container."
        print_warning "You can skip Twilio setup if you don't want WhatsApp notifications."
    else
        print_status ".env file already exists."
    fi
}

# Check configuration
check_config() {
    if [ ! -f adahn.config ]; then
        print_error "adahn.config file not found. This file is required."
        exit 1
    fi
    print_status "Configuration file found."
}

# Main function
case "$1" in
    "setup")
        print_status "Setting up Automated Azan Docker environment..."
        check_docker
        setup_directories
        setup_env
        check_config
        print_status "Setup complete! You can now run: ./docker-helper.sh start"
        ;;
    "build")
        print_status "Building Docker image..."
        docker-compose build
        print_status "Build complete!"
        ;;
    "start")
        print_status "Starting Automated Azan container..."
        setup_directories
        docker-compose up -d
        print_status "Container started! Check logs with: ./docker-helper.sh logs"
        ;;
    "stop")
        print_status "Stopping Automated Azan container..."
        docker-compose down
        print_status "Container stopped!"
        ;;
    "restart")
        print_status "Restarting Automated Azan container..."
        docker-compose restart
        print_status "Container restarted!"
        ;;
    "logs")
        print_status "Showing container logs (Ctrl+C to exit)..."
        docker-compose logs -f
        ;;
    "status")
        print_status "Container status:"
        docker-compose ps
        ;;
    "shell")
        print_status "Opening shell in container..."
        docker-compose exec automated-azan /bin/bash
        ;;
    "clean")
        print_status "Cleaning up Docker resources..."
        docker-compose down -v
        docker image rm automated-azan_automated-azan 2>/dev/null || true
        print_status "Cleanup complete!"
        ;;
    *)
        echo "Automated Azan Docker Helper"
        echo ""
        echo "Usage: $0 {setup|build|start|stop|restart|logs|status|shell|clean}"
        echo ""
        echo "Commands:"
        echo "  setup   - Initial setup (create directories, check dependencies)"
        echo "  build   - Build Docker image"
        echo "  start   - Start the container"
        echo "  stop    - Stop the container"
        echo "  restart - Restart the container"
        echo "  logs    - Show container logs"
        echo "  status  - Show container status"
        echo "  shell   - Open shell in container"
        echo "  clean   - Remove container and image"
        echo ""
        echo "Quick start:"
        echo "  1. ./docker-helper.sh setup"
        echo "  2. Edit .env and adahn.config files"
        echo "  3. ./docker-helper.sh start"
        exit 1
        ;;
esac
