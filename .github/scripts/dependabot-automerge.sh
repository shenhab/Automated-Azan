#!/usr/bin/env bash
# Decide whether a Dependabot pull request is safe to merge without a person and,
# if so, wait for CI and merge exactly the commit that CI tested.
# The policy itself is code with tests: go/tools/automerge.
#
# Environment: REPO (owner/name), PR (number), GH_TOKEN,
#   DRY_RUN=true      only report the decision (default false)
#   BASE_CHECKS       comma-separated checks every PR must pass
#   BUILD_CHECKS      comma-separated extra checks for Go / Docker updates
#   WAIT_MINUTES      how long to wait for the checks (default 40)
#   POLL_SECONDS      polling interval (default 30)
#   TOOL_DIR          where go/ lives (default go)
set -euo pipefail

: "${REPO:?}" "${PR:?}"
DRY_RUN="${DRY_RUN:-false}"
WAIT_MINUTES="${WAIT_MINUTES:-40}"
POLL_SECONDS="${POLL_SECONDS:-30}"
TOOL_DIR="${TOOL_DIR:-go}"
BASE_CHECKS="${BASE_CHECKS:-Test,E2E (binary),E2E (docker),Docker build (no push)}"
BUILD_CHECKS="${BUILD_CHECKS:-Cross-build (linux/amd64),Cross-build (linux/arm64),Cross-build (linux/arm),Cross-build (windows/amd64),Cross-build (darwin/amd64),Cross-build (darwin/arm64)}"
WORK="$(mktemp -d)"
SUMMARY="${GITHUB_STEP_SUMMARY:-/dev/null}"

say() { echo "$*"; echo "$*" >> "$SUMMARY"; }

# ── 1. Facts about the pull request ──────────────────────────────────────────
pr_json=$(gh api "repos/$REPO/pulls/$PR")
AUTHOR=$(jq -r '.user.login' <<<"$pr_json")
BRANCH=$(jq -r '.head.ref' <<<"$pr_json")
HEAD_SHA=$(jq -r '.head.sha' <<<"$pr_json")
HEAD_REPO=$(jq -r '.head.repo.full_name // ""' <<<"$pr_json")
STATE=$(jq -r '.state' <<<"$pr_json")

gh api "repos/$REPO/pulls/$PR/commits" --paginate --jq '.[].commit.message' > "$WORK/message.txt"
gh api "repos/$REPO/pulls/$PR/files" --paginate --jq '.[].filename' > "$WORK/files.txt"
GO_DIRECTIVE=false
if gh api "repos/$REPO/pulls/$PR/files" --paginate --jq '.[] | select(.filename=="go/go.mod") | .patch // ""' \
     | grep -Eq '^[+-](go|toolchain) '; then GO_DIRECTIVE=true; fi

echo "PR #$PR by $AUTHOR on $BRANCH ($HEAD_SHA), state $STATE"

# ── 2. Decide ────────────────────────────────────────────────────────────────
# (A dry run may evaluate a closed PR, to show what the policy would have decided.)
if [ "$STATE" != "open" ] && [ "$DRY_RUN" != "true" ]; then say "PR #$PR is $STATE — nothing to do."; exit 0; fi
if [ "$HEAD_REPO" != "$REPO" ]; then say "**PR #$PR is from a fork ($HEAD_REPO) — left for a person.**"; exit 0; fi

(cd "$TOOL_DIR" && go run ./tools/automerge -author "$AUTHOR" -branch "$BRANCH" \
    -message-file "$WORK/message.txt" -files "$WORK/files.txt" -go-directive-changed="$GO_DIRECTIVE") > "$WORK/decision.txt"
MERGE=$(sed -n 's/^merge=//p' "$WORK/decision.txt")
RELEASE=$(sed -n 's/^release=//p' "$WORK/decision.txt")
ECOSYSTEM=$(sed -n 's/^ecosystem=//p' "$WORK/decision.txt")
REASON=$(sed -n 's/^reason=//p' "$WORK/decision.txt")

if [ "$MERGE" != "true" ]; then
  say "**PR #$PR is left for a person** — $REASON"
  exit 0
fi
say "**PR #$PR is safe to merge automatically** — $REASON"
if [ "$DRY_RUN" = "true" ]; then say "(dry run: not waiting, not merging)"; exit 0; fi

# ── 3. Wait for every required check on THIS commit ──────────────────────────
required="$BASE_CHECKS"
case "$ECOSYSTEM" in go_modules|docker) required="$required,$BUILD_CHECKS" ;; esac
deadline=$((SECONDS + WAIT_MINUTES * 60))
while :; do
  cur=$(gh api "repos/$REPO/pulls/$PR" --jq '[.state, .head.sha] | @tsv')
  if [ "$(cut -f1 <<<"$cur")" != "open" ]; then say "PR #$PR was closed meanwhile — nothing to do."; exit 0; fi
  if [ "$(cut -f2 <<<"$cur")" != "$HEAD_SHA" ]; then say "PR #$PR was updated while waiting — a new run will handle it."; exit 0; fi

  gh api "repos/$REPO/commits/$HEAD_SHA/check-runs?per_page=100" --jq '[.check_runs[] | {id, name, status, conclusion}]' > "$WORK/runs.json"
  (cd "$TOOL_DIR" && go run ./tools/automerge -checks-file "$WORK/runs.json" -required "$required") > "$WORK/verdict.txt"
  STATE_NOW=$(sed -n 's/^state=//p' "$WORK/verdict.txt")
  DETAIL=$(sed -n 's/^detail=//p' "$WORK/verdict.txt")
  case "$STATE_NOW" in
    ready) break ;;
    failed) say "A required check failed ($DETAIL) — PR #$PR is left for a person."; exit 0 ;;
  esac
  if [ "$SECONDS" -ge "$deadline" ]; then say "Timed out after $WAIT_MINUTES min waiting for: $DETAIL (PR #$PR)."; exit 1; fi
  echo "waiting: $DETAIL"
  sleep "$POLL_SECONDS"
done
say "All required checks passed on $HEAD_SHA."

# ── 4. Merge exactly what was tested ─────────────────────────────────────────
gh api -X PUT "repos/$REPO/pulls/$PR/merge" -f merge_method=squash -f sha="$HEAD_SHA" --jq '"merged: \(.sha)"'
say "Merged PR #$PR (squash, commit $HEAD_SHA tested)."

# ── 5. Start a release if this changes what ships ────────────────────────────
# (A merge made with the workflow token does not start other workflows by itself.)
if [ "$RELEASE" = "true" ]; then
  gh workflow run release.yml -R "$REPO" --ref main
  say "Started the Release workflow."
else
  say "No release needed (nothing users run changed)."
fi
