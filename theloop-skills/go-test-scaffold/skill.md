# skill: go-test-scaffold
description: Scaffold table-driven Go test stubs for `go/internal/*` packages and document this repo's established Go unit-test conventions (table-driven cases, hand-written fakes, httptest mocks, `success`/`error` dict-style JSON assertions, `go test ./...`).
run: `python3 theloop-skills/go-test-scaffold/scaffold_go_test.py go/internal/<pkg>/<file>.go [FuncName ...]`

## When to use

Any work item shaped "add/write unit tests for `go/internal/X`" — a recurring
backlog pattern (10+ completed as of 2026-07: quran/controller.go,
timesync, GetHTTPTime, etc.). Run the scaffold script first to get a stub per
untested function, then read the conventions below before filling them in —
don't re-derive the test shape from scratch.

## The scaffold script

```
python3 theloop-skills/go-test-scaffold/scaffold_go_test.py go/internal/<pkg>/<file>.go [FuncName ...]
```

Parses the top-level func/method declarations in `<file>.go` and, for every
one that doesn't already have a `Test<Func>` in the sibling `<file>_test.go`,
appends a table-driven stub (with `TODO` markers) to that test file —
creating it with the correct `package` clause and `testing` import if it
doesn't exist yet. Pass one or more function names to scaffold only those;
omit them to scaffold every function in the file. It never overwrites or
duplicates an existing stub/test, and never touches source files, only the
`_test.go` sibling. Fill in each stub's real cases and remove the
`t.Fatalf("TODO...")` placeholder before committing — the stub is a starting
shape, not a passing test.

## Conventions this repo already follows

1. **Table-driven tests are the default shape** for anything with more than
   one case worth covering. Fields: `name`, input field(s), `want`
   field(s), optionally `wantErr bool`. Loop with
   `for _, tt := range tests { t.Run(tt.name, func(t *testing.T) { ... }) }`.
   See `internal/timesync/timesync_test.go`'s `TestParseWorldTimeDatetime`,
   `internal/prayer/scheduler_test.go`'s `TestParseHHMM`, and
   `internal/chromecast/manager_test.go`'s `TestEqualFold`.

2. **Hand-written fakes, not a mocking library** — none is used anywhere
   under `go/internal`. When the code under test depends on an interface
   (e.g. `quran.castPlayer`), define a small fake struct in the `_test.go`
   file implementing it, with a `var _ Interface = (*fake)(nil)`
   compile-time assertion and override-func fields for behavior that
   varies per test. See `internal/quran/controller_test.go`'s
   `fakeCastPlayer`. For pure in-memory state (no interface needed), just
   construct the real struct directly and poke its fields/maps, as in
   `internal/chromecast/manager_test.go`'s `TestDevicesReturnsAllCachedDevices`.

3. **`httptest.Server` for HTTP-fetching code**, not a `RoundTripper` mock:
   point the package's swappable base-URL variable at
   `httptest.NewServer(handler).URL` for the test's duration and restore it
   via `t.Cleanup`. See `internal/prayer/aladhan_test.go`'s
   `withAladhanTestServer`, which swaps `aladhanBase`.

4. **`success`/`error` dict-style JSON responses** — the same convention
   used on the Python side (see this repo's root `CLAUDE.md`, "JSON
   Response Convention") carries into the Go web layer via
   `writeJSON(w, map[string]interface{}{"success": ..., ...})`. Assert
   these by decoding the `httptest.ResponseRecorder` body into
   `map[string]interface{}` and checking `body["success"]`, alongside the
   HTTP status code. See `internal/web/auth_test.go`'s
   `TestRequireAPIAuth_AuthDisabled_Returns503` /
   `TestRequireAPIAuth_Unauthenticated_Returns401`.

5. **Goroutine/state-machine code** (chromecast playback, quran controller)
   is tested by driving the real exported method and polling with a small
   helper instead of a fixed `time.Sleep` — see `internal/quran/controller_test.go`'s
   `waitFor(t, timeout, cond)`.

6. **Error-path assertions** check `err != nil` (or `success:false` plus a
   non-2xx status), not a specific message — except where the production
   code documents that it owns a specific error string, in which case
   assert that substring too. See `scheduler_test.go`'s `TestParseHHMM`
   checking `strings.Contains(err.Error(), "parseHHMM")`.

## Running the suite

```
cd go && go test ./...
```

Run from the `go/` module root (it has its own `go.mod`), not the repo
root — the repo root is the Python app and an unqualified `./...` there
won't reach the Go module.
