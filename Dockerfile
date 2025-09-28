FROM python:3.11-slim-bullseye

LABEL maintainer="Automated Azan Project"
LABEL description="Automated Islamic Prayer Time announcements via Chromecast with Web Interface"
LABEL version="2.0.0"

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
    # Time synchronization
    ntpdate

WORKDIR /app

# Create necessary directories with proper permissions
RUN mkdir -p /var/log /app/data /app/logs /app/config && \
    chmod 755 /var/log /app/data /app/logs /app/config

# Install pipenv
RUN pip install --upgrade pip pipenv

# Copy Pipfiles first for better layer caching
COPY Pipfile Pipfile.lock ./

# Install dependencies using pipenv
RUN pipenv install --system --deploy

# Copy application files
COPY . .

# Create default configuration if none exists
RUN if [ ! -f "adahn.config" ]; then \
    echo "[Settings]" > adahn.config && \
    echo "# Your Chromecast/Google Home speaker name (case-sensitive)" >> adahn.config && \
    echo "speakers-group-name = athan" >> adahn.config && \
    echo "" >> adahn.config && \
    echo "# Prayer time location (naas, icci, or custom location)" >> adahn.config && \
    echo "location = naas" >> adahn.config && \
    echo "" >> adahn.config && \
    echo "# Optional: Enable pre-Fajr Quran" >> adahn.config && \
    echo "# pre_fajr_enabled = True" >> adahn.config; \
    fi

# Create placeholder files for data directory
RUN touch /app/data/.gitkeep /app/logs/.gitkeep

# Create a non-root user for security
RUN useradd -m -u 1000 appuser && \
    chown -R appuser:appuser /app && \
    chown -R appuser:appuser /var/log && \
    mkdir -p /app/config && \
    chown -R appuser:appuser /app/config

USER appuser

# Expose ports
EXPOSE 5000
# Chromecast discovery ports (informational - requires host networking)
EXPOSE 8008 8009

# Health check to verify both main app and web interface
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD python -c "import requests; requests.get('http://localhost:5000', timeout=5)" || exit 1

# Default environment variables
ENV LOG_FILE=/var/log/azan_service.log
ENV PYTHONUNBUFFERED=1
ENV TZ=UTC


CMD ["python", "main.py"]
