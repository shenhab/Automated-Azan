# Automated Azan - Development Makefile
# Common tasks for development and deployment

.PHONY: help setup test clean docker-build docker-run web install update shell

# Default target
help:
	@echo "🕌 Automated Azan - Development Commands"
	@echo "======================================="
	@echo ""
	@echo "Development:"
	@echo "  setup     - Set up development environment with pipenv"
	@echo "  install   - Install dependencies"
	@echo "  update    - Update dependencies"
	@echo "  test      - Run test suite"
	@echo "  shell     - Activate pipenv shell"
	@echo "  run       - Run main application"
	@echo "  web       - Run web interface"
	@echo ""
	@echo "Docker:"
	@echo "  docker-build - Build Docker images"
	@echo "  docker-run   - Run with Docker Compose"
	@echo "  docker-logs  - View Docker logs"
	@echo "  docker-stop  - Stop Docker containers"
	@echo ""
	@echo "Cleanup:"
	@echo "  clean     - Clean up temporary files"

# Development setup
setup:
	@echo "🚀 Setting up development environment..."
	@./setup-dev.sh

install:
	@echo "📦 Installing dependencies..."
	@pipenv install --dev

update:
	@echo "🔄 Updating dependencies..."
	@pipenv update

shell:
	@echo "🐚 Activating pipenv shell..."
	@pipenv shell

# Testing
test:
	@echo "🧪 Running test suite..."
	@./test-pipenv.sh

# Running applications
run:
	@echo "🕌 Running Automated Azan main application..."
	@pipenv run python main.py

web:
	@echo "🌐 Running web interface..."
	@pipenv run python web_interface.py

# Docker commands
docker-build:
	@echo "🐳 Building Docker images..."
	@docker-compose build

docker-run:
	@echo "🐳 Starting Docker containers..."
	@docker-compose up -d

docker-logs:
	@echo "📋 Showing Docker logs..."
	@docker-compose logs -f

docker-stop:
	@echo "🛑 Stopping Docker containers..."
	@docker-compose down

# Cleanup
clean:
	@echo "🧹 Cleaning up..."
	@find . -type f -name "*.pyc" -delete
	@find . -type d -name "__pycache__" -delete
	@find . -type d -name ".pytest_cache" -delete
	@rm -rf build/ dist/ *.egg-info/

# Check system requirements
check:
	@echo "🔍 Checking system requirements..."
	@python --version
	@pipenv --version
	@docker --version 2>/dev/null || echo "⚠️  Docker not installed"
	@docker-compose --version 2>/dev/null || echo "⚠️  Docker Compose not installed"

# Development workflow
dev: setup test
	@echo "✅ Development environment ready!"
	@echo "Run 'make run' to start the application"
	@echo "Run 'make web' to start the web interface"
