# Automated Azan

Automated Islamic prayer (Athan) announcements on Chromecast / Google Home
speakers, with a web interface for managing devices, timetables and settings.

The application is written in Go and lives in [`go/`](go/). See
[`go/README.md`](go/README.md) for features, configuration, building and
packaging.

## Quick start (Docker)

```bash
docker compose up -d      # uses shenhab/athan:latest
```

Then open `http://<host>:28426`. Host networking is required so Chromecast
devices can be discovered on your LAN. Config and data persist in the
`azan_config` and `azan_data` volumes.

## Repository layout

| Path | Contents |
|---|---|
| `go/` | The Azan agent (source, tests, packaging, Dockerfile) |
| `docker-compose.yml` | Run the published image |
| `.github/` | Release workflow and Dependabot config |

## Legacy Python version

The original Python implementation is no longer maintained and has been
removed from `main`. It is preserved in git at the
[`python-legacy`](https://github.com/shenhab/Automated-Azan/tree/python-legacy) tag.
