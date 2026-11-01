package main

import (
	"strings"
	"testing"
)

// The policy for merging Dependabot pull requests without a person, once CI is
// green. "Safe" means: a small, well-understood kind of update, touching only the
// files that kind of update should touch.
//
//	Go dependencies            patch and minor   (batched into one weekly PR)
//	Docker base image          patch only
//	GitHub Actions             patch only
//	anything else              a person decides: majors, a Go version / toolchain
//	                           change, unexpected files, missing metadata, other
//	                           ecosystems, or any PR not from Dependabot

// Real Dependabot commit messages from this repository.
const (
	msgGoMinor = `deps: bump golang.org/x/sys from 0.45.0 to 0.46.0 in /go

Bumps [golang.org/x/sys](https://github.com/golang/sys) from 0.45.0 to 0.46.0.

---
updated-dependencies:
- dependency-name: golang.org/x/sys
  dependency-version: 0.46.0
  dependency-type: direct:production
  update-type: version-update:semver-minor
...

Signed-off-by: dependabot[bot] <support@github.com>`

	msgActionMinor = `ci: bump docker/build-push-action from 7.2.0 to 7.4.0

---
updated-dependencies:
- dependency-name: docker/build-push-action
  dependency-version: 7.4.0
  dependency-type: direct:production
  update-type: version-update:semver-minor
...`

	msgDockerMinor = `deps: bump alpine from 3.20 to 3.24 in /go

---
updated-dependencies:
- dependency-name: alpine
  dependency-version: '3.24'
  dependency-type: direct:production
  update-type: version-update:semver-minor
...`

	msgGroup = `deps: bump the go-minor-patch group in /go with 3 updates

---
updated-dependencies:
- dependency-name: github.com/gorilla/websocket
  dependency-version: 1.5.3
  dependency-type: direct:production
  update-type: version-update:semver-patch
  dependency-group: go-minor-patch
- dependency-name: golang.org/x/sys
  dependency-version: 0.46.0
  dependency-type: direct:production
  update-type: version-update:semver-minor
  dependency-group: go-minor-patch
- dependency-name: github.com/BurntSushi/toml
  dependency-version: 1.6.0
  dependency-type: direct:production
  update-type: version-update:semver-minor
  dependency-group: go-minor-patch
...`
)

func upd(typ string) string {
	return "---\nupdated-dependencies:\n- dependency-name: x/y\n  dependency-version: 9.9.9\n  dependency-type: direct:production\n  update-type: version-update:semver-" + typ + "\n...\n"
}

var (
	goFiles     = []string{"go/go.mod", "go/go.sum"}
	dockerFiles = []string{"go/Dockerfile", "go/Dockerfile.headless"}
	actionFiles = []string{".github/workflows/ci.yml", ".github/workflows/release.yml"}
	bot         = "dependabot[bot]"
)

func TestGoDependenciesPatchAndMinorMerge(t *testing.T) {
	for name, msg := range map[string]string{"real minor": msgGoMinor, "patch": upd("patch"), "a batched group": msgGroup} {
		d := Decide(Input{Author: bot, Branch: "dependabot/go_modules/go/anything", Message: msg, Files: goFiles})
		if !d.Merge || !d.Release {
			t.Errorf("%s: %+v, want an automatic merge that also releases", name, d)
		}
	}
}

func TestDockerAndActionsMergeOnlyForPatch(t *testing.T) {
	cases := []struct {
		name   string
		branch string
		msg    string
		files  []string
		merge  bool
	}{
		{"docker patch", "dependabot/docker/go/alpine-3.24.3", upd("patch"), dockerFiles, true},
		{"docker minor (real alpine 3.20->3.24)", "dependabot/docker/go/alpine-3.24", msgDockerMinor, dockerFiles, false},
		{"action patch", "dependabot/github_actions/docker/login-action-4.7.1", upd("patch"), actionFiles, true},
		{"action minor (real build-push-action 7.2->7.4)", "dependabot/github_actions/docker/build-push-action-7.4.0", msgActionMinor, actionFiles, false},
	}
	for _, c := range cases {
		d := Decide(Input{Author: bot, Branch: c.branch, Message: c.msg, Files: c.files})
		if d.Merge != c.merge {
			t.Errorf("%s: merge=%v (%s), want %v", c.name, d.Merge, d.Reason, c.merge)
		}
	}
}

func TestOnlyCodeChangesTriggerARelease(t *testing.T) {
	g := Decide(Input{Author: bot, Branch: "dependabot/go_modules/go/x", Message: upd("patch"), Files: goFiles})
	d := Decide(Input{Author: bot, Branch: "dependabot/docker/go/x", Message: upd("patch"), Files: dockerFiles})
	a := Decide(Input{Author: bot, Branch: "dependabot/github_actions/x", Message: upd("patch"), Files: actionFiles})
	if !g.Release || !d.Release {
		t.Errorf("Go / Docker updates change what ships and must release: %+v %+v", g, d)
	}
	if !a.Merge || a.Release {
		t.Errorf("an Action patch should merge but not release: %+v", a)
	}
}

func TestMajorUpdatesAlwaysNeedAPerson(t *testing.T) {
	for _, b := range []struct{ branch, files string }{
		{"dependabot/go_modules/go/x", "go"}, {"dependabot/docker/go/x", "docker"}, {"dependabot/github_actions/x", "actions"},
	} {
		files := map[string][]string{"go": goFiles, "docker": dockerFiles, "actions": actionFiles}[b.files]
		if d := Decide(Input{Author: bot, Branch: b.branch, Message: upd("major"), Files: files}); d.Merge {
			t.Errorf("%s: a MAJOR update was merged automatically", b.branch)
		}
	}
}

func TestOneRiskyUpdateInAGroupHoldsTheWholePR(t *testing.T) {
	// The extra entry goes inside the metadata block, before its closing "...".
	risky := "- dependency-name: github.com/some/lib\n  dependency-version: 2.0.0\n  dependency-type: direct:production\n  update-type: version-update:semver-major\n...\n"
	msg := strings.Replace(msgGroup, "...", risky, 1)
	if d := Decide(Input{Author: bot, Branch: "dependabot/go_modules/go/g", Message: msg, Files: goFiles}); d.Merge {
		t.Errorf("a group containing a major update was merged: %+v", d)
	}
}

func TestAChangeOfGoVersionOrToolchainIsNeverAutomatic(t *testing.T) {
	// e.g. x/mod 0.42 required Go 1.26: the "dependency bump" moves the toolchain.
	d := Decide(Input{Author: bot, Branch: "dependabot/go_modules/go/x", Message: upd("minor"), Files: goFiles, GoDirectiveChanged: true})
	if d.Merge {
		t.Errorf("a PR that changes the Go version was merged: %+v", d)
	}
}

func TestUnexpectedFilesNeedAPerson(t *testing.T) {
	for name, files := range map[string][]string{
		"source code changed too": {"go/go.mod", "go/go.sum", "go/internal/web/server.go"},
		"a workflow changed":      {"go/go.mod", ".github/workflows/release.yml"},
		"no files at all":         nil,
	} {
		if d := Decide(Input{Author: bot, Branch: "dependabot/go_modules/go/x", Message: upd("patch"), Files: files}); d.Merge {
			t.Errorf("%s: merged automatically", name)
		}
	}
	if d := Decide(Input{Author: bot, Branch: "dependabot/github_actions/x", Message: upd("patch"), Files: []string{".github/workflows/ci.yml", "go/go.mod"}}); d.Merge {
		t.Error("an Action PR that also touches go.mod was merged automatically")
	}
}

func TestOnlyDependabotAndOnlyKnownEcosystems(t *testing.T) {
	if d := Decide(Input{Author: "some-person", Branch: "dependabot/go_modules/go/x", Message: upd("patch"), Files: goFiles}); d.Merge {
		t.Error("a PR not authored by Dependabot was merged automatically")
	}
	if d := Decide(Input{Author: bot, Branch: "dependabot/npm_and_yarn/x", Message: upd("patch"), Files: goFiles}); d.Merge {
		t.Error("an ecosystem outside the policy was merged automatically")
	}
	if d := Decide(Input{Author: bot, Branch: "feature/dependabot-lookalike", Message: upd("patch"), Files: goFiles}); d.Merge {
		t.Error("a non-Dependabot branch name was trusted")
	}
}

func TestMissingOrUnreadableMetadataNeedsAPerson(t *testing.T) {
	for name, msg := range map[string]string{
		"no metadata":          "deps: bump x from 1 to 2",
		"unknown update type":  "---\nupdated-dependencies:\n- dependency-name: a\n  update-type: version-update:semver-wobbly\n...",
		"entry without a type": "---\nupdated-dependencies:\n- dependency-name: a\n  dependency-version: 1\n...",
		"empty":                "",
	} {
		if d := Decide(Input{Author: bot, Branch: "dependabot/go_modules/go/x", Message: msg, Files: goFiles}); d.Merge {
			t.Errorf("%s: merged automatically", name)
		}
	}
}

func TestEveryDecisionExplainsItself(t *testing.T) {
	for _, in := range []Input{
		{Author: bot, Branch: "dependabot/go_modules/go/x", Message: upd("patch"), Files: goFiles},
		{Author: bot, Branch: "dependabot/go_modules/go/x", Message: upd("major"), Files: goFiles},
		{Author: "x"},
	} {
		if d := Decide(in); d.Reason == "" {
			t.Errorf("no reason for %+v", in)
		}
	}
}

// ── waiting for CI ───────────────────────────────────────────────────────────
//
// A pull request may be merged only when every required check has passed on the
// commit being merged. A skipped run of the same name (another workflow's job, a
// dry run) must never stand in for a real result.

func run(id int64, name, status, conclusion string) Run {
	return Run{ID: id, Name: name, Status: status, Conclusion: conclusion}
}

func TestChecksReadyOnlyWhenEveryRequiredOneSucceeded(t *testing.T) {
	runs := []Run{run(1, "Test", "completed", "success"), run(2, "E2E", "completed", "success"), run(3, "GitGuardian", "completed", "success")}
	if v := EvaluateChecks(runs, []string{"Test", "E2E"}); v.State != "ready" {
		t.Errorf("all required passed: %+v", v)
	}
}

func TestChecksPendingWhileSomethingIsMissingOrRunning(t *testing.T) {
	if v := EvaluateChecks([]Run{run(1, "Test", "completed", "success")}, []string{"Test", "E2E"}); v.State != "pending" {
		t.Errorf("a required check that has not appeared yet: %+v", v)
	}
	if v := EvaluateChecks([]Run{run(1, "Test", "completed", "success"), run(2, "E2E", "in_progress", "")}, []string{"Test", "E2E"}); v.State != "pending" {
		t.Errorf("a required check still running: %+v", v)
	}
	if v := EvaluateChecks(nil, []string{"Test"}); v.State != "pending" {
		t.Errorf("no runs at all: %+v", v)
	}
}

func TestAFailedCheckStopsEverythingEvenIfOthersAreStillRunning(t *testing.T) {
	runs := []Run{run(1, "Test", "completed", "failure"), run(2, "E2E", "in_progress", "")}
	v := EvaluateChecks(runs, []string{"Test", "E2E"})
	if v.State != "failed" || v.Detail == "" {
		t.Errorf("got %+v, want failed with a reason", v)
	}
	for _, bad := range []string{"cancelled", "timed_out", "action_required", "stale"} {
		if v := EvaluateChecks([]Run{run(1, "Test", "completed", bad)}, []string{"Test"}); v.State != "failed" {
			t.Errorf("conclusion %q should fail, got %+v", bad, v)
		}
	}
}

func TestASuccessfulRerunReplacesAnEarlierFailure(t *testing.T) {
	runs := []Run{run(1, "Test", "completed", "failure"), run(7, "Test", "completed", "success")}
	if v := EvaluateChecks(runs, []string{"Test"}); v.State != "ready" {
		t.Errorf("a later success should win: %+v", v)
	}
	runs = []Run{run(1, "Test", "completed", "success"), run(7, "Test", "completed", "failure")}
	if v := EvaluateChecks(runs, []string{"Test"}); v.State != "failed" {
		t.Errorf("a later failure should win: %+v", v)
	}
}

func TestASkippedRunNeverStandsInForARealResult(t *testing.T) {
	// The real incident: a dry run of another workflow left a skipped "Test" on the
	// same commit, with a higher id than CI's real one.
	for name, runs := range map[string][]Run{
		"real success, later skipped": {run(1, "Test", "completed", "success"), run(9, "Test", "completed", "skipped")},
		"real FAILURE, later skipped": {run(1, "Test", "completed", "failure"), run(9, "Test", "completed", "skipped")},
	} {
		v := EvaluateChecks(runs, []string{"Test"})
		want := map[string]string{"real success, later skipped": "ready", "real FAILURE, later skipped": "failed"}[name]
		if v.State != want {
			t.Errorf("%s: got %+v, want %s", name, v, want)
		}
	}
	// A required check that only ever shows as skipped has not really run.
	if v := EvaluateChecks([]Run{run(9, "Test", "completed", "skipped")}, []string{"Test"}); v.State != "pending" {
		t.Errorf("only a skipped run: %+v, want pending (it never really ran)", v)
	}
}

func TestParseRunsJSON(t *testing.T) {
	runs, err := ParseRuns([]byte(`[{"id":5,"name":"Test","status":"completed","conclusion":"success"},{"id":6,"name":"E2E","status":"in_progress","conclusion":null}]`))
	if err != nil || len(runs) != 2 || runs[1].Status != "in_progress" || runs[1].Conclusion != "" {
		t.Errorf("ParseRuns = %+v, %v", runs, err)
	}
	if _, err := ParseRuns([]byte("not json")); err == nil {
		t.Error("garbage was accepted")
	}
}
