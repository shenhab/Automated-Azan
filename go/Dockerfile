# syntax=docker/dockerfile:1
#
# Self-contained build for the headless Azan agent: compiles the binary inside
# Docker, so CI (or anyone) only needs `docker build`. No pre-built binary.
#
#   docker build -t shenhab/athan:dev go/
#
# (Dockerfile.headless is the older variant that packages a binary compiled on
# the host; scripts/deploy.sh still uses it for local deploys.)

# ── Build stage ──────────────────────────────────────────────────────────────
# Runs on the build host's own architecture and cross-compiles (pure Go, no cgo).
FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine AS build
WORKDIR /src

# Dependencies first so this layer is cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/azan-agent ./cmd/azan-agent

# ── Runtime stage ────────────────────────────────────────────────────────────
FROM alpine:3.20

# ca-certificates — HTTPS requests to prayer-time APIs
# tzdata          — timezone database (needed for Europe/Dublin DST rules)
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=build /out/azan-agent .

# Config is mounted at /config/azan.toml via a Docker volume.
# The binary creates a default azan.toml on first run if none exists.
ENV AZAN_CONFIG_FILE=/config/azan.toml
ENV TZ=Europe/Dublin

EXPOSE 28426

HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD wget -qO- http://localhost:28426/ >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/app/azan-agent"]
