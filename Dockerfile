FROM python:3.11-slim-bullseye

# Install system dependencies for network tools and basic utilities
RUN apt-get update && apt-get install -y \
    # Basic utilities
    wget \
    curl \
    # Network discovery dependencies for Chromecast
    avahi-daemon \
    avahi-utils \
    # Build tools for Python packages
    build-essential \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Create log directory with proper permissions
RUN mkdir -p /var/log && chmod 755 /var/log

# Install pipenv
RUN pip install --upgrade pip pipenv

# Copy Pipfiles
COPY Pipfile Pipfile.lock ./

# Install dependencies using pipenv
RUN pipenv install --system --deploy

COPY . .

# Expose port for web interface
EXPOSE 5000

# Create a non-root user for security
RUN useradd -m -u 1000 appuser && \
    chown -R appuser:appuser /app && \
    chown -R appuser:appuser /var/log

USER appuser

# Expose the network interface for Chromecast discovery
# Note: The container will need to run with --network host for Chromecast discovery
EXPOSE 8008 8009

# Add health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD python -c "import sys; sys.exit(0)" || exit 1

CMD ["python", "main.py"]
