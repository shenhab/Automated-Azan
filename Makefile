# Automated Azan - Development Makefile
# Common tasks for development and deployment

.PHONY: help setup test clean docker-build docker-run web install update shell

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

test-chromecast:
	@echo "📡 Running comprehensive Chromecast discovery tests..."
	@./test-chromecast-discovery.sh

test-quick:
	@echo "⚡ Running quick Chromecast method comparison..."
	@pipenv run python chromecast_comparison.py

test-all: test test-chromecast test-quick
	@echo "✅ All tests completed!"

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

# Help
help:
	@echo "🕌 Automated Azan - Available Commands"
	@echo "========================================="
	@echo ""
	@echo "🏗️  Setup & Environment:"
	@echo "  make setup         Setup pipenv environment"
	@echo "  make install       Install dependencies"
	@echo "  make update        Update dependencies"
	@echo "  make shell         Activate pipenv shell"
	@echo "  make clean         Clean up temporary files"
	@echo ""
	@echo "🧪 Testing:"
	@echo "  make test          Run application tests"
	@echo "  make test-chromecast    Run comprehensive Chromecast tests"
	@echo "  make test-quick    Quick Chromecast method comparison"
	@echo "  make test-all      Run all tests"
	@echo ""
	@echo "🚀 Running:"
	@echo "  make run           Run main application"
	@echo "  make web           Run web interface"
	@echo ""
	@echo "🐳 Docker:"
	@echo "  make docker-build  Build Docker images"
	@echo "  make docker-run    Start containers"
	@echo "  make docker-logs   Show container logs"
	@echo "  make docker-stop   Stop containers"
	@echo ""
	@echo "🔍 Utilities:"
	@echo "  make check         Check system requirements"
	@echo "  make dev           Complete development setup"
	@echo "  make help          Show this help message"
	@echo ""

.DEFAULT_GOAL := help
.PHONY: setup install update shell test test-chromecast test-quick test-all run web docker-build docker-run docker-logs docker-stop clean check dev help
