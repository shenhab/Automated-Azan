# Stage 1: install Python dependencies only (layer cached independently of source code)
FROM ghcr.io/astral-sh/uv:python3.11-bookworm-slim AS deps
WORKDIR /app
COPY pyproject.toml uv.lock ./
RUN uv sync --no-dev --frozen --no-install-project

# Stage 2: runtime image
FROM ghcr.io/astral-sh/uv:python3.11-bookworm-slim

LABEL maintainer="Automated Azan Project"
LABEL description="Automated Islamic Prayer Time announcements via Chromecast with Web Interface"
LABEL version="2.0.0"

RUN apt-get update && apt-get install -y --no-install-recommends \
    avahi-daemon \
    dbus \
    iputils-ping \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /var/log /app/data /app/logs /app/config && \
    chmod 755 /var/log /app/data /app/logs /app/config

# Pull the pre-built venv from the deps stage
COPY --from=deps /app/.venv /app/.venv
ENV PATH="/app/.venv/bin:$PATH"

WORKDIR /app
COPY . .

RUN chmod +x docker-entrypoint.sh

RUN if [ ! -f "azan.toml" ]; then \
    printf '[speaker]\ngroup_name = "athan"\n\n[prayer]\nlocation = "naas"\npre_fajr_enabled = false\npre_fajr_minutes = 30\n\n[web]\nhost = "0.0.0.0"\nport = 5000\n\n[log]\nlevel = "INFO"\nfile_path = "logs/azan.log"\n' > azan.toml; \
    fi

RUN touch /app/data/.gitkeep /app/logs/.gitkeep

RUN useradd -m -u 1000 appuser && \
    chown -R appuser:appuser /app && \
    chown -R appuser:appuser /var/log && \
    chown -R appuser:appuser /app/config

USER appuser

EXPOSE 5000
EXPOSE 8008 8009

HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD python -c "import socket; s=socket.socket(); s.settimeout(5); s.connect(('localhost', 5000)); s.close()" || exit 1

ENV LOG_FILE=/var/log/azan_service.log
ENV PYTHONUNBUFFERED=1
ENV TZ=Europe/Dublin

ENTRYPOINT ["./docker-entrypoint.sh"]
CMD ["python", "main.py"]
