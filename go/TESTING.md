# Testing

The tests are the specification. **If a test fails, fix the code, not the test.**
A test may only be changed when the test itself is wrong (bad fixture, race in the
harness, wrong oracle), and the commit should say why. New tests should be shown
to fail first: break the code on purpose (or run the test against the old code),
confirm red, restore.

**Tests must never reach real devices.** Anything that builds a `chromecast.Manager`
or starts the agent must set `AZAN_DISABLE_MDNS=1` so only the fake speakers it
creates are visible; `TestTestRigsNeverSeeRealDevices` guards this. (An early TV-mute
test without it found and acted on real speakers on the LAN.)

## Layers

| Layer | Where | What it proves | Runs |
|---|---|---|---|
| Unit | `internal/prayer/*_test.go`, `tools/nextversion` (the release rules) | The DST sanity guard corrects bad timetables (incl. after-midnight Isha); the daily refresh falls on every calendar day and the real refresh loop runs once on both DST nights; across **every day of 2026 and all 5 bundled timetables** the real scheduler yields ordered prayers at solar noon in UTC with no 1-hour jumps; Isha after midnight is scheduled on the right day | `go test ./...` |
| Component | `internal/chromecast/*_test.go` | Speaker control against a fake speaker (right file per prayer, warm-up reused, dropped session, speaker returns mid-retry, failure reported) and TV-mute (idle TV muted/unmuted, playing TV paused/resumed, what the owner muted/paused is left alone, overlapping calls, dropped connection, unreachable TV) | `go test ./...` |
| Journal | `internal/journal/*_test.go` | The verdict logic against exact timelines, including real traces (`fakecast/testdata/traces`); the read-only watcher against the fake speaker (an outside stop, an outside volume change, a dropped connection, an unreachable device, the watcher sends nothing but status requests); retention and size cap, crash recovery, scrubbed export, device health, notes | `go test ./...` |
| Cast library | `internal/chromecast/concurrent_requests_test.go` | Several connections in one process never lose each other's replies (fails on the unpatched library; see `third_party/go-chromecast/PATCHES.md`) | `go test ./...` |
| Fake speaker | `internal/fakecast` | A fake Google Cast receiver speaking the real CASTV2 protocol (checked with the same `go-chromecast` client the agent uses). Records messages, LOAD, mute/pause/play events with times, and downloads the media URL like a real speaker | used by the above |
| End to end | `e2e/` (tag `e2e`) | The **real agent** (binary or Docker image) with fake speakers: a scheduled prayer plays the right MP3 at the right time exactly once and a browser gets the `prayer_fired` event; TVs are quieted and restored; the whole web UI/API (auth, pages, status, discovery, play/stop, settings validation and re-planning, media, uploads, websocket); and, on clock-change days with a fake clock (binary mode only, tag `azanfaketime`), the Athan fires at the right instant across the spring-forward gap and the 00:05 refresh runs once and schedules correctly on both nights | `go test -tags e2e ./e2e/` |
| Live sources | `internal/prayer/live_test.go` (tag `live`) | The real Mawaqit (Naas, Newbridge, Cork, Galway), MAI and Aladhan data still downloads, parses and schedules correctly; reports how many days a site publishes with a wrong GMT/IST changeover. (ICCI is not listed: its online timetable was discontinued.) | weekly + on demand: `.github/workflows/live-sources.yml` |

CI (`.github/workflows/ci.yml`) runs everything except the live check; the Docker
image is only published (`release.yml`, see `../RELEASING.md`) if all of it, including both e2e modes,
passes. The live check never blocks a release: a red run means a website changed.

## Running the end-to-end tests

```bash
cd go
go test -tags e2e ./e2e/ -v -parallel 16          # builds and runs the agent binary (~3.5 min)

docker build -f Dockerfile -t azan-e2e:local .    # or test the image that ships
E2E_DOCKER_IMAGE=azan-e2e:local go test -tags e2e ./e2e/ -v -parallel 16
```

No real speaker, TV or timetable site is touched: timetables are fixtures written to
the agent's data directory, speakers are addressed with `AZAN_STATIC_DEVICES`
(`Name=host:port;Name2=host:port`) and `AZAN_DISABLE_MDNS=1` turns discovery off; `AZAN_MAI_URL` points the MAI source at a fake site. The
fake clock (`AZAN_FAKE_NOW`) exists only in builds with the `azanfaketime` tag, which
only the e2e harness uses; it is not called `faketime` because Go's runtime already
uses that tag. A few tests are skipped in Docker mode (fake clock; uploads, since the
harness runs the container as a non-root user).

```bash
go test -tags live ./internal/prayer/ -run Live -v   # live sources (needs internet)
```

## Not covered

Real Google devices/groups and firmware quirks, real mDNS on a LAN, audibility, and
the fall-back hour itself (a prayer scheduled inside the repeated 01:00–02:00 hour is
ambiguous by nature and no real prayer falls there). After deploying, press the UI's
*test* button on a real speaker. `go-chromecast` has an upstream data race, so do not
run `-race` on the Chromecast code.
