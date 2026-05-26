# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

### Development (uv)
```bash
uv sync --dev          # Install all dependencies
uv run python main.py  # Run prayer scheduler + web interface
uv run python web_interface.py  # Run web interface only
uv run python -m pytest tests/ -v  # Run full test suite
uv run python -m pytest tests/test_basic_functionality.py -v  # Run single test file
```

### Production (Docker)
```bash
make deploy            # Full deployment: validate + build + run
make docker-logs       # Follow container logs
make docker-stop       # Stop container
make docker-rebuild    # Rebuild and restart
```

### Utilities
```bash
make deploy-check      # Validate config and Docker requirements
make status            # Show container status and config summary
make clean             # Remove .pyc, __pycache__, build artifacts
```

## Architecture

### Entry Point and Threading Model
`main.py` starts two concurrent components:
1. `AthanScheduler.run_scheduler()` — blocking main thread loop that sleeps until each prayer, then calls `ChromecastManager.start_adahn()` or `start_adahn_alfajr()`
2. `web_interface.py` — Flask+SocketIO server in a background daemon thread

Both share the same `ChromecastManager` and `AthanScheduler` instances, passed into the web thread at startup via `start_web_interface(scheduler.chromecast_manager, scheduler, config_watcher)`.

### Configuration
Two coexisting config systems:
- **`adahn.config`** (INI format) — primary user config with `[Settings]` section; read by `config_manager.py` (legacy) and `app_config.py` (new dataclass approach)
- **`app_config.py`** — `AppConfig` dataclass singleton (`get_app_config()`) that loads from env vars first, then overlays `adahn.config`. Subsections: `LocationConfig`, `PrayerConfig`, `AudioConfig`, `WebConfig`, `LoggingConfig`.

`config_watcher.py` uses `watchdog` to hot-reload config changes without restart.

### Prayer Time Sources
`PrayerTimesFetcher` in `prayer_times_fetcher.py` supports two sources configured via `location` in `adahn.config`:
- `naas` — reads `data/naas_prayers_timetable.json` (local JSON timetable)
- `icci` — reads `data/icci_timetable.json` (local JSON timetable)

Prayer times are cached in-memory keyed by `{location}_{date}`. DST changes are detected daily at 1 AM and trigger a re-download.

### Chromecast Layer
Two parallel implementations exist:
- **`chromecast_manager.py`** — monolithic original manager used by `AthanScheduler` and `web_interface.py`
- **`chromecast/`** package — refactored modular version with `discovery.py`, `connection.py`, `playback.py`, `circuit_breaker.py`, and `manager.py`

The `chromecast_manager.py` (root) uses class-level `_shared_chromecasts` cache and a `playback_lock` to prevent concurrent Athan playback. It serves media via HTTP URLs to the Chromecast device.

### JSON Response Convention
All module methods return `{"success": bool, ...}` dicts rather than raising exceptions. Check `result.get('success', False)` before using returned data. The only exception is `run_scheduler()` which runs indefinitely.

### Web Interface
`web_interface.py` serves Flask routes and emits Socket.IO events for live updates. `web_interface_api.py` contains the REST API endpoints. Audio files in `Media/` are served statically. The web server binds to `0.0.0.0:5000`.

### Docker Deployment
`Dockerfile` builds a single container running both scheduler and web interface. Requires `--network host` for mDNS-based Chromecast discovery. Config is mounted at `/app/config/adahn.config`. Timezone is set via `TZ` env var (default: `Europe/Dublin`).
