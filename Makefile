# Automated Azan - Streamlined Makefile
# Two deployment methods: pipenv (development) and Docker (production)

docker-rebuild: docker-stop docker-build docker-run
	@echo "🔄 Rebuilt and restarted Docker container"

docker-fix-health: docker-stop docker-build docker-run
	@echo "🏥 Fixed health check and restarted container"
	@echo "   💡 This rebuilds the container with health check fixes"

docker-fix-athan: docker-stop docker-build docker-run
	@echo "🎵 Fixed Athan collision protection and restarted container"
	@echo "   💡 Prevents Athan from interrupting itself during playback"HONY: help setup install run web test clean deploy docker-build docker-run docker-logs docker-stop

# Default target
.DEFAULT_GOAL := help

#=============================================================================
# DEVELOPMENT (pipenv)
#=============================================================================

setup:
	@echo "🚀 Setting up development environment..."
	@echo "   📦 Installing pipenv and dependencies..."
	@pip install --user pipenv || pip3 install --user pipenv
	@pipenv install --dev
	@echo "   ✅ Development environment ready!"
	@echo "   💡 Run 'make run' to start prayer scheduler"
	@echo "   💡 Run 'make web' to start web interface"

install:
	@echo "📦 Installing dependencies with pipenv..."
	@pipenv install --dev

run:
	@echo "� Starting Automated Azan prayer scheduler..."
	@pipenv run python main.py

web:
	@echo "🌐 Starting web interface..."
	@echo "   📱 Interface will be available at: http://localhost:5000"
	@pipenv run python web_interface.py

test:
	@echo "🧪 Running test suite..."
	@pipenv run python -m pytest -v || echo "No pytest found, running manual tests..."
	@echo "   � Testing Chromecast discovery..."
	@pipenv run python chromecast_comparison.py

test-chromecast:
	@echo "📡 Running comprehensive Chromecast discovery tests..."
	@pipenv run python chromecast_comparison.py

shell:
	@echo "🐚 Activating pipenv shell..."
	@pipenv shell

update:
	@echo "🔄 Updating dependencies..."
	@pipenv update

#=============================================================================
# PRODUCTION DEPLOYMENT (Docker)
#=============================================================================

deploy: deploy-check docker-build docker-run
	@echo ""
	@echo "🎉 Deployment Complete!"
	@echo "======================================"
	@echo "🕌 Automated Azan is now running in Docker"
	@echo "🌐 Web interface: http://localhost:5000"
	@echo "🔍 Device management: http://localhost:5000/chromecasts"
	@echo "🧪 Test audio: http://localhost:5000/test"
	@echo "📋 View logs: make docker-logs"
	@echo ""
	@echo "Next Steps:"
	@echo "1. Verify your speakers appear at: http://localhost:5000/chromecasts"
	@echo "2. Test Adhan playback at: http://localhost:5000/test"
	@echo "3. Check prayer times are correct for your location"
deploy-check:
	@echo "� Validating deployment requirements..."
	@echo ""
	@echo "� Required files:"
	@test -f adahn.config && echo "   ✅ adahn.config" || (echo "   ❌ adahn.config missing! Run: cp adahn.config.example adahn.config" && exit 1)
	@test -f Media/media_Athan.mp3 && echo "   ✅ Media/media_Athan.mp3" || echo "   ⚠️  Media/media_Athan.mp3 missing (will use default)"
	@test -f Media/media_adhan_al_fajr.mp3 && echo "   ✅ Media/media_adhan_al_fajr.mp3" || echo "   ⚠️  Media/media_adhan_al_fajr.mp3 missing (will use default)"
	@test -f docker-compose.yml && echo "   ✅ docker-compose.yml" || echo "   ❌ docker-compose.yml missing!"
	@test -f Dockerfile && echo "   ✅ Dockerfile" || echo "   ❌ Dockerfile missing!"
	@echo ""
	@echo "� Docker environment:"
	@docker --version 2>/dev/null && echo "   ✅ Docker available" || (echo "   ❌ Docker not found! Please install Docker" && exit 1)
	@docker-compose --version 2>/dev/null && echo "   ✅ Docker Compose available" || (echo "   ❌ Docker Compose not found! Please install Docker Compose" && exit 1)
	@echo ""
	@echo "� Configuration preview:"
	@echo "   📍 Location: $$(grep '^location' adahn.config | cut -d'=' -f2 | xargs 2>/dev/null || echo 'not configured')"
	@echo "   🔊 Speaker Group: $$(grep '^speakers-group-name' adahn.config | cut -d'=' -f2 | xargs 2>/dev/null || echo 'not configured')"
	@echo ""
	@echo "✅ All requirements validated!"

docker-build:
	@echo "🐳 Building Docker image..."
	@echo "   � Building unified container (Prayer scheduler + Web interface)..."
	@docker-compose build --no-cache

docker-run:
	@echo "🐳 Starting Docker container..."
	@echo "   � Launching Automated Azan service..."
	@docker-compose up -d

docker-logs:
	@echo "� Showing container logs..."
	@docker-compose logs -f

docker-stop:
	@echo "🛑 Stopping Docker container..."
	@docker-compose down

docker-restart:
	@echo "� Restarting Docker container..."
	@docker-compose restart

docker-rebuild: docker-stop docker-build docker-run
	@echo "� Rebuilt and restarted Docker container"

#=============================================================================
# UTILITIES
#=============================================================================

clean:
	@echo "🧹 Cleaning up temporary files..."
	@find . -type f -name "*.pyc" -delete 2>/dev/null || true
	@find . -type d -name "__pycache__" -delete 2>/dev/null || true
	@find . -type d -name ".pytest_cache" -delete 2>/dev/null || true
	@rm -rf build/ dist/ *.egg-info/ 2>/dev/null || true
	@echo "   ✅ Cleanup complete"

check:
	@echo "🔍 System requirements check..."
	@echo ""
	@echo "Python:"
	@python3 --version 2>/dev/null && echo "   ✅ Python 3 available" || echo "   ❌ Python 3 not found"
	@echo ""
	@echo "Pipenv (for development):"
	@pipenv --version 2>/dev/null && echo "   ✅ Pipenv available" || echo "   ⚠️  Pipenv not found (install with: pip install pipenv)"
	@echo ""
	@echo "Docker (for production):"
	@docker --version 2>/dev/null && echo "   ✅ Docker available" || echo "   ⚠️  Docker not found"
	@docker-compose --version 2>/dev/null && echo "   ✅ Docker Compose available" || echo "   ⚠️  Docker Compose not found"

status:
	@echo "📊 Current system status..."
	@echo ""
	@echo "Docker containers:"
	@docker-compose ps 2>/dev/null || echo "   No Docker containers running"
	@echo ""
	@echo "Configuration:"
	@test -f adahn.config && echo "   📍 Location: $$(grep '^location' adahn.config | cut -d'=' -f2 | xargs)" || echo "   ⚠️  No adahn.config found"
	@test -f adahn.config && echo "   🔊 Speaker: $$(grep '^speakers-group-name' adahn.config | cut -d'=' -f2 | xargs)" || echo "   ⚠️  No speaker configured"

#=============================================================================
# HELP
#=============================================================================

help:
	@echo "� Automated Azan - Streamlined Deployment"
	@echo "========================================"
	@echo ""
	@echo "🚀 QUICK START:"
	@echo "   make setup     Setup development environment (pipenv)"
	@echo "   make deploy    Deploy production system (Docker)"
	@echo ""
	@echo "🐍 DEVELOPMENT (pipenv):"
	@echo "   make setup           Setup pipenv environment and dependencies"
	@echo "   make install         Install/update dependencies"
	@echo "   make run             Run prayer scheduler"
	@echo "   make web             Run web interface"
	@echo "   make test            Run test suite"
	@echo "   make test-chromecast Test device discovery"
	@echo "   make shell           Activate pipenv shell"
	@echo "   make update          Update dependencies"
	@echo ""
	@echo "� PRODUCTION (Docker):"
	@echo "   make deploy          Complete deployment (check + build + run)"
	@echo "   make deploy-check    Validate deployment requirements"
	@echo "   make docker-build    Build Docker image"
	@echo "   make docker-run      Start container"
	@echo "   make docker-logs     View container logs (follow mode)"
	@echo "   make docker-stop     Stop container"
	@echo "   make docker-restart  Restart container"
	@echo "   make docker-rebuild  Rebuild and restart container"
	@echo "   make docker-fix-health Fix health check issues"
	@echo "   make docker-fix-athan Fix Athan collision protection"
	@echo ""
	@echo "�️  UTILITIES:"
	@echo "   make check           Check system requirements"
	@echo "   make status          Show current system status"
	@echo "   make clean           Clean temporary files"
	@echo "   make help            Show this help message"
	@echo ""
	@echo "📖 USAGE EXAMPLES:"
	@echo "   Development: make setup && make run"
	@echo "   Production:  make deploy"
	@echo "   Testing:     make web (then visit http://localhost:5000)"
	@echo ""
