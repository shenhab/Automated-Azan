# Automated Azan — Go Agent

A standalone Go binary that replicates all features of the Python app.
Single file, no runtime, no dependencies — just double-click and run.

## Requirements

- [Go 1.22+](https://go.dev/dl/)
- On Linux with system tray: `libgtk-3-dev libappindicator3-dev`

## Quick start

```bash
cd go/
go mod tidy       # download dependencies
make build        # builds ./azan-agent (or azan-agent.exe on Windows)
./azan-agent      # run with system tray
```

## Cross-compile for all platforms

```bash
make build-all
# outputs to dist/
```

| File | Platform |
|---|---|
| `azan-agent-windows-amd64.exe` | Windows |
| `azan-agent-darwin-amd64` | macOS Intel |
| `azan-agent-darwin-arm64` | macOS Apple Silicon |
| `azan-agent-linux-amd64` | Linux |
| `azan-agent-linux-arm64` | Raspberry Pi 4/5 |

## Install as background service

```bash
# After building on the target machine:
./azan-agent install    # registers as Windows Service / macOS LaunchAgent / systemd
./azan-agent start      # starts immediately
./azan-agent stop       # stops
./azan-agent uninstall  # removes
```

## Configuration

On first run, place `azan.toml` next to the binary (or set `AZAN_CONFIG_FILE`):

```toml
[speaker]
group_name = "athan"

[prayer]
location = "naas"          # naas or icci
pre_fajr_enabled = true
pre_fajr_minutes = 30
friday_kahf_enabled = false

[web]
host = "0.0.0.0"
port = 5000

[log]
level = "INFO"
file_path = "logs/azan.log"
```

The web dashboard is at `http://localhost:5000`.

## Directory layout expected at runtime

```
azan-agent(.exe)
azan.toml
Media/
  media_Athan.mp3
  media_adhan_al_fajr.mp3
  (other MP3s)
data/
  (timetable JSON files — auto-downloaded on first run)
logs/
```

## Architecture

| Package | Responsibility |
|---|---|
| `cmd/azan-agent` | Entry point, wires all packages, service/tray mode |
| `internal/config` | TOML config load/save/hot-reload |
| `internal/prayer` | Fetch prayer times (ICCI/Naas), daily scheduler |
| `internal/chromecast` | mDNS discovery, CASTV2 playback |
| `internal/media` | HTTP file server for MP3s (Chromecast fetches from here) |
| `internal/web` | Dashboard, REST API, WebSocket live updates |
| `internal/tray` | System tray icon |
| `internal/timesync` | NTP time sync |
