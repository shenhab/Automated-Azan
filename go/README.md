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

## The Journal

Every prayer (and the pre-Fajr Quran, Friday Al-Kahf and manual plays) is recorded from about
90 seconds before it starts until the playback is over, so a silent or cut-short Athan can be
explained. Open **Journal** in the web app.

For each recording the Journal keeps one timeline of:

- what the **agent** did: connecting, asking the speaker to play, what the speaker confirmed,
  retries, stops, muting and releasing TVs;
- what the **devices** reported by themselves: the speaker, the display that hosts a Cast group,
  the TVs being muted (state, app, volume, mute), read-only, twice a second;
- the **audio file** being downloaded by the speaker (who, how much, whether all of it);
- the devices' own **status pages** (uptime, firmware, Wi-Fi) before and after, so a reboot or a
  firmware update between prayers shows up.

Changes a device made that the agent did not cause (someone stopping the speaker from a phone, a
volume change) are marked **not the agent** and make the result a warning. Each recording gets a
verdict (all good / warning / failed) and a one-line explanation, written to the log as well,
with an optional desktop notification when something looks wrong. After each Athan you can tell
the Journal whether you actually heard it — the one thing no device can report.

The raw message-by-message recording of each device can be downloaded, with a *shareable* version
that has addresses, ids and names removed. Recordings are kept in `<data dir>/journal`, 30 days
and 300 MB by default (both configurable). The Journal only ever asks devices for their status
(`internal/castrecord` enforces it), and a failure in it cannot affect a playback.

```toml
[journal]
enabled = true
retention_days = 30
max_mb = 300
watch_tvs = true            # also record the TVs the mute feature handles
watch_devices = []          # more devices, by name
notify_on = "problems"      # or "failures", or "off"
```

The Cast client library is vendored and patched in [`third_party/go-chromecast`](third_party/go-chromecast/PATCHES.md):
several connections in one process (the Athan, TVs, the Journal) used to lose each other's replies.

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
| `internal/journal` | Records what the agent and devices did around each prayer; verdicts, retention, export |
| `internal/trace` | How the rest of the agent reports events to the Journal without depending on it |
| `internal/castrecord` | Read-only recording of the Cast protocol (only status requests are ever sent) |
| `internal/web` | Dashboard, REST API, WebSocket live updates |
| `internal/tray` | System tray icon |
| `internal/timesync` | NTP time sync |
