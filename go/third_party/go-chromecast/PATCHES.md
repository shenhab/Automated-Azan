# go-chromecast: local patches

This is `github.com/vishen/go-chromecast` v0.3.4 (Apache-2.0, see `LICENSE`), reduced to the
packages the agent uses and patched. `go/go.mod` points at it with a `replace` directive.

**Why.** The library says "not concurrent safe" in several places. That was harmless while the
agent used one Cast connection at a time, but the agent runs several in one process (the Athan
session, TVs being muted at once, the Journal watching devices every 0.5 s). Concurrent requests
lost their replies; every loss cost a 5-second timeout and a retry, and the unguarded reply map
could crash the program ("concurrent map read and map write"). A lost reply while an Athan is
being loaded delays the Athan.

**What changed** (each marked `PATCHED` in the source):

1. `application/application.go`
   - The request counter was a plain package-level `int` shared by all connections, incremented
     without a lock. It is now an atomic.
   - `resultChanMap` (reply table) was read by the receiver goroutine while the sender wrote to
     it. It is now guarded by a mutex.
   - `sendAndWait` registered its reply handler *after* sending, so a quick reply could arrive
     before anyone was listening. It now registers first.
   - Requests are no longer stamped with their id by mutating the payload (see 2).
2. `cast/connection.go`
   - Many payloads are shared package-level objects (`cast.GetStatusHeader`, `cast.ConnectHeader`,
     ...). Stamping a request id into them let two goroutines overwrite each other's id before
     serialising, so the reply carried someone else's id and was dropped. The id is now written
     into the serialised message.
   - The length prefix and the body were two separate writes; the receive goroutine's reply to a
     device PING could land between them and corrupt the stream. They are now one write under a
     lock.

**Test.** `go/internal/chromecast/concurrent_requests_test.go` fails on the unpatched library
(four concurrent connections lose replies in the first round) and passes with these patches.

**Not fixed** (still true of this copy): other fields of `Application` (the cached app, media and
volume state) are read and written from two goroutines without locks, so `-race` still reports
the library. The agent never relies on those caches for anything the Journal or the Athan
depend on.

**Upgrading.** Re-apply the `PATCHED` changes to the new version, or send them upstream and drop
this directory.
