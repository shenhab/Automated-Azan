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
    # iputils-ping for network checks in entrypoint
    iputils-ping \
    # curl needed for uv installer
    curl \
    && rm -rf /var/lib/apt/lists/* \
    && apt-get clean

WORKDIR /app

# Create necessary directories with proper permissions
RUN mkdir -p /var/log /app/data /app/logs /app/config && \
    chmod 755 /var/log /app/data /app/logs /app/config

# Install uv using the official installer, place it globally
RUN curl -LsSf https://astral.sh/uv/install.sh | sh && \
    mv /root/.local/bin/uv /usr/local/bin/uv

# Copy package configuration files first for better layer caching
COPY pyproject.toml uv.lock ./

# Install dependencies using uv sync with the lockfile for reproducible builds
RUN uv sync --no-dev --frozen

# Add the venv to PATH so python/uvicorn etc. resolve without 'uv run'
ENV PATH="/app/.venv/bin:$PATH"

# Copy application files
COPY . .

# Ensure entrypoint is executable
RUN chmod +x docker-entrypoint.sh

# Create default configuration if none exists
RUN if [ ! -f "azan.toml" ]; then \
    printf '[speaker]\ngroup_name = "athan"\n\n[prayer]\nlocation = "naas"\npre_fajr_enabled = false\npre_fajr_minutes = 30\n\n[web]\nhost = "0.0.0.0"\nport = 5000\n\n[log]\nlevel = "INFO"\nfile_path = "logs/azan.log"\n' > azan.toml; \
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
ENV TZ=Europe/Dublin

# Set the entrypoint
ENTRYPOINT ["./docker-entrypoint.sh"]

# Run with Python directly (packages installed system-wide in Docker)
CMD ["python", "main.py"]
