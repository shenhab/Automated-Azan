# Releasing

**Releases are automatic.** Merging to `main` runs `.github/workflows/release.yml`:

```
test  →  plan  →  tag  →  Docker image ┐
                          binaries     ├→  GitHub release
                                       ┘
```

1. **Test** — the full CI (unit, component, end-to-end in both binary and Docker mode).
2. **Plan** — decide whether this change is a release, and which version (below).
3. **Tag** — `vX.Y.Z` is created *before* anything is published.
4. **Build** — the Docker image (`shenhab/athan:vX.Y.Z`, `:latest`, `:sha-<short>`) and the
   binaries for Linux / Windows / macOS, all stamped with that version.
5. **Release** — the GitHub release, with notes generated from the merged pull requests, is
   published last. If anything fails after the tag was created, the tag is removed again, so no
   half-released version is left behind.

## What counts as a release

Only a change to **what users run**: the agent and its bundled data and web assets, its
dependencies, or how it is built and packaged (`go/**`). These do **not** release: tests
(`*_test.go`, `go/e2e`, `testdata`), the fake speaker, tooling (`go/tools`, `go/scripts`),
documentation, CI (`.github/**`) and `docker-compose.yml`. A Dependabot bump of an Action
therefore doesn't release; a bump of a Go dependency or the Docker base image does.

## Which version

The next version is the highest `vMAJOR.MINOR.PATCH` tag plus a bump:

| Bump | When |
|---|---|
| **patch** (default) | any other releasing change |
| **minor** | the title starts with `feat:` |
| **major** | the title is `feat!:` / `fix!:`, or the commit body contains `BREAKING CHANGE` |

### PR labels — explicit intent

A `release:*` label on the PR **forces a release** (even if only docs or CI changed) and sets the bump.
Explicit labels win over what the title suggests; if several are present the highest wins.

| Label | Effect |
|---|---|
| `release:patch` | release, bump patch |
| `release:minor` | release, bump minor |
| `release:major` | release, bump major |
| `release:skip` (or `[skip release]` in the PR title/body) | do **not** release; beats the labels above |

> The rules are code with tests: `go/tools/nextversion`. Change them there, not in the workflow.

## By hand

* **Actions → Release → Run workflow** (from `main`) to release now: pick the bump, tick
  *force* to release even if nothing users run changed. Tick *dry run* to only see what would
  happen (works from any branch).
* Pushing a tag `vX.Y.Z` yourself releases exactly that version.

Releases are serialized (one at a time), so two quick merges can never get the same version.
