FROM python:3.11-slim-bullseye

LABEL maintainer="Automated Azan Project"
LABEL description="Automated Islamic Prayer Time announcements via Chromecast with Web Interface"
LABEL version="2.0.0"

# Install minimal system dependencies (optimized for speed and size)
RUN apt-get update && apt-get install -y --no-install-recommends \
    # Chromecast discovery (required)
    avahi-daemon \
    # Required for avahi
    dbus \
    # Temporary: gcc for building Python packages (will be removed)
    gcc \
    && rm -rf /var/lib/apt/lists/* \
    && apt-get clean

WORKDIR /app

# Create necessary directories with proper permissions
RUN mkdir -p /var/log /app/data /app/logs /app/config && \
    chmod 755 /var/log /app/data /app/logs /app/config

# Install uv for fast Python package management (no-cache for smaller image)
RUN pip install --no-cache-dir --upgrade pip uv

# Copy package configuration files first for better layer caching
COPY pyproject.toml ./
# Copy uv.lock if exists for reproducible builds
COPY uv.lock* ./
# Copy requirements.txt as fallback for compatibility
COPY requirements.txt* ./

# Install dependencies using uv (system-wide for Docker)
RUN if [ -f "requirements.txt" ]; then \
        # Use requirements.txt for system-wide installation in Docker
        uv pip install --system -r requirements.txt; \
    elif [ -f "uv.lock" ]; then \
        # Fallback to uv sync if no requirements.txt
        uv sync --no-dev --frozen; \
    else \
        echo "ERROR: No dependency files found"; exit 1; \
    fi \
    # Remove build dependencies after Python packages are installed
    && apt-get remove -y gcc \
    && apt-get autoremove -y \
    && rm -rf /var/lib/apt/lists/*

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

# Health check using socket (no need for curl/requests)
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD python -c "import socket; s=socket.socket(); s.settimeout(5); s.connect(('localhost', 5000)); s.close()" || exit 1

# Default environment variables
ENV LOG_FILE=/var/log/azan_service.log
ENV PYTHONUNBUFFERED=1
ENV TZ=UTC


# Run with Python directly (packages installed system-wide in Docker)
CMD ["python", "main.py"]
