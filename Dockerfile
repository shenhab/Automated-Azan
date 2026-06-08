# Stage 1: install Python dependencies only (layer cached independently of source code)
FROM ghcr.io/astral-sh/uv:python3.11-bookworm-slim AS deps
WORKDIR /app
RUN apt-get update && apt-get upgrade -y && rm -rf /var/lib/apt/lists/*
COPY pyproject.toml uv.lock ./
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --no-dev --frozen --no-install-project && \
    uv pip install --upgrade pip wheel

# Stage 2: runtime image
FROM ghcr.io/astral-sh/uv:python3.11-bookworm-slim

LABEL maintainer="Automated Azan Project"
LABEL description="Automated Islamic Prayer Time announcements via Chromecast with Web Interface"
LABEL version="2.0.0"

RUN apt-get update && apt-get upgrade -y && apt-get install -y --no-install-recommends \
    avahi-daemon \
    dbus \
    iputils-ping \
    && rm -rf /var/lib/apt/lists/*

# Upgrade system Python pip/wheel/setuptools to clear scanner findings
RUN python3 -m pip install --upgrade pip wheel setuptools --break-system-packages

# Create user early so COPY --chown places files with correct ownership,
# avoiding an expensive chown -R over the entire /app tree at the end.
RUN useradd -m -u 1000 appuser

RUN install -d -o appuser -g appuser /var/log /app/data /app/logs /app/config

# Pull the pre-built venv from the deps stage
COPY --chown=appuser:appuser --from=deps /app/.venv /app/.venv
ENV PATH="/app/.venv/bin:$PATH"

WORKDIR /app
COPY --chown=appuser:appuser . .

RUN chmod +x docker-entrypoint.sh

RUN if [ ! -f "azan.toml" ]; then \
    printf '[speaker]\ngroup_name = "athan"\n\n[prayer]\nlocation = "naas"\npre_fajr_enabled = false\npre_fajr_minutes = 30\n\n[web]\nhost = "0.0.0.0"\nport = 5000\n\n[log]\nlevel = "INFO"\nfile_path = "logs/azan.log"\n' > azan.toml; \
    fi

RUN touch /app/data/.gitkeep /app/logs/.gitkeep

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
