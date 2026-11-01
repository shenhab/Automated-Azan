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
| **minor** | the PR has the label `release:minor`, or its title starts with `feat:` |
| **major** | the PR has the label `release:major`, the title is `feat!:` / `fix!:`, or the commit body contains `BREAKING CHANGE` |

Opt out of releasing a merge with the label `release:skip` or `[skip release]` in the PR title/body.

> The rules are code with tests: `go/tools/nextversion`. Change them there, not in the workflow.

## By hand

* **Actions → Release → Run workflow** (from `main`) to release now: pick the bump, tick
  *force* to release even if nothing users run changed. Tick *dry run* to only see what would
  happen (works from any branch).
* Pushing a tag `vX.Y.Z` yourself releases exactly that version.

Releases are serialized (one at a time), so two quick merges can never get the same version.

## Dependabot

Dependabot opens the update PRs; safe ones **merge themselves** once CI is green
(`.github/workflows/dependabot-automerge.yml`, policy in `go/tools/automerge`):

| Update | Merges by itself? |
|---|---|
| Go dependencies, **patch and minor** (batched into one weekly PR) | yes — and also built for all six release targets first |
| Docker base image, **patch** | yes |
| GitHub Actions, **patch** | yes |
| any **major**; a change of the Go version or toolchain; a PR touching unexpected files; anything without readable metadata | no — waits for you |

It waits for the required checks on the exact commit and merges only that commit. A Go or
Docker update then starts a release automatically (a merge made by a workflow doesn't start
other workflows by itself, so it starts the Release workflow explicitly). Nothing redeploys
your running container: that still happens only when you pull the new image.

To see what it would do with any PR, including closed ones: *Actions → Dependabot auto-merge →
Run workflow* with the PR number (dry run is the default).

`main` requires a pull request and these passing checks: **Test**, **E2E (binary)**,
**E2E (docker)**, **Docker build (no push)**.
