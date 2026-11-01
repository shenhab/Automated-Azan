# Testing

The tests are the specification. **If a test fails, fix the code, not the test.**
A test may only be changed when the test itself is wrong (bad fixture, race in the
harness, wrong oracle), and the commit should say why. New tests should be shown
to fail first: break the code on purpose, confirm red, revert.

## Layers

| Layer | Where | What it proves | Runs |
|---|---|---|---|
| Unit | `internal/prayer/*_test.go` | DST handling: the sanity guard corrects bad timetables; the daily refresh fires on every calendar day (incl. 29 Mar / 25 Oct); across **every day of 2026 and all 5 timetable sources** the real scheduler yields prayers in order, at solar noon in UTC, with no 1-hour jumps | `go test ./...` |
| Component | `internal/chromecast/manager_test.go` | Speaker control against a fake speaker: right audio file per prayer, pre-connected session reused, recovers from a dropped session, delivers when the speaker returns mid-retry, reports failure when it never does | `go test ./...` |
| Fake speaker | `internal/fakecast` | A fake Google Cast receiver speaking the real CASTV2 protocol (verified with the same `go-chromecast` client the agent uses). Records every message, `LOAD`, volume/mute, and downloads the media URL like a real speaker | used by the above |
| End to end | `e2e/` (build tag `e2e`) | The **real agent** (binary, or the Docker image) with a timetable whose prayer is due in ~2 min: the right MP3 reaches the speaker at the right time, exactly once, and the speaker can download it | `go test -tags e2e ./e2e/` |

CI (`.github/workflows/ci.yml`) runs all of it; the Docker image is only published
(`docker.yml`) if everything, including both e2e modes, passes.

## Running the end-to-end test

```bash
cd go
go test -tags e2e ./e2e/ -v                      # builds and runs the agent binary (~2.5 min)

docker build -f Dockerfile -t azan-e2e:local .   # or test the image that ships
E2E_DOCKER_IMAGE=azan-e2e:local go test -tags e2e ./e2e/ -v
```

No real speaker or network source is touched: timetables are fixtures written to
the agent's data directory and the speaker is addressed with `AZAN_STATIC_DEVICES`
(`Name=host:port;Name2=host:port`) instead of mDNS.

## What is not covered

Real Google devices/groups and firmware quirks, real mDNS on a LAN, audibility, and
changes to the layout of the live timetable websites. After deploying, press the
UI's *test* button on a real speaker. Note: `go-chromecast` has an upstream data
race, so do not run `-race` on the Chromecast code.
