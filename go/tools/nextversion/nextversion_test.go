package main

import (
	"reflect"
	"strings"
	"testing"
)

// The release rules, as a specification:
//
//   - Releasing is automatic when a change reaches main, but only if it changes
//     what users run (the agent and its image), not tests, docs or CI.
//   - The next version is the highest vMAJOR.MINOR.PATCH tag plus a bump. The
//     default bump is patch; a PR label, a feat: title or a breaking-change marker
//     raises it.
//   - A merge can opt out ("[skip release]" / label release:skip). A person
//     running the workflow by hand can force a release and choose the bump.

func in(files ...string) Input {
	return Input{Tags: []string{"v1.2.7", "v1.2.6"}, Message: "fix: something (#9)", Files: files}
}

func TestWhatCountsAsAShippedChange(t *testing.T) {
	shipped := []string{
		"go/internal/prayer/scheduler.go", "go/cmd/azan-agent/main.go", "go/go.mod", "go/go.sum",
		"go/Dockerfile", "go/Dockerfile.headless", "go/internal/prayer/embedded/naas_prayers_timetable.json",
		"go/internal/web/templates/settings.html", "go/internal/media/embedded/kahf.mp3", "go/packaging/macos/Info.plist",
	}
	notShipped := []string{
		"go/internal/prayer/scheduler_test.go", "go/e2e/e2e_test.go", "go/internal/prayer/testdata/x.json",
		"go/internal/fakecast/fakecast.go", "go/tools/nextversion/main.go", "go/TESTING.md", "go/README.md", "README.md",
		".github/workflows/ci.yml", "docker-compose.yml", ".gitignore", "go/scripts/deploy.sh", "go/Makefile",
		"go/Dockerfile.dockerignore",
	}
	for _, f := range shipped {
		if !isShipped(f) {
			t.Errorf("%s should count as a shipped change", f)
		}
	}
	for _, f := range notShipped {
		if isShipped(f) {
			t.Errorf("%s should NOT trigger a release", f)
		}
	}
}

func TestDefaultIsAPatchRelease(t *testing.T) {
	d, err := Decide(in("go/internal/web/server.go", "go/internal/web/server_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !d.Release || d.Version != "v1.2.8" || d.Bump != "patch" {
		t.Errorf("got %+v, want release v1.2.8 (patch)", d)
	}
}

func TestChangesThatDoNotAffectTheProductDoNotRelease(t *testing.T) {
	for name, files := range map[string][]string{
		"tests only":   {"go/internal/prayer/scheduler_test.go", "go/e2e/e2e_ui_test.go"},
		"docs only":    {"README.md", "go/TESTING.md"},
		"CI only":      {".github/workflows/ci.yml"},
		"test tooling": {"go/internal/fakecast/fakecast.go", "go/tools/nextversion/main.go"},
		"nothing":      nil,
	} {
		d, err := Decide(in(files...))
		if err != nil {
			t.Fatal(err)
		}
		if d.Release {
			t.Errorf("%s: released %s, but nothing users run changed", name, d.Version)
		}
		if d.Reason == "" {
			t.Errorf("%s: no reason given", name)
		}
	}
}

func TestDependencyAndBaseImageBumpsRelease(t *testing.T) {
	for name, files := range map[string][]string{
		"a Go dependency":   {"go/go.mod", "go/go.sum"},
		"a Docker base img": {"go/Dockerfile"},
		"refreshed data":    {"go/internal/prayer/embedded/cork_timetable.json"},
	} {
		d, _ := Decide(in(files...))
		if !d.Release || d.Bump != "patch" {
			t.Errorf("%s: got %+v, want a patch release", name, d)
		}
	}
}

func TestBumpIsRaisedByLabelTitleOrBreakingMarker(t *testing.T) {
	code := []string{"go/internal/web/server.go"}
	cases := []struct {
		name    string
		msg     string
		labels  []string
		want    string
		version string
	}{
		{"minor label", "Add a thing (#1)", []string{"release:minor"}, "minor", "v1.3.0"},
		{"major label", "Rework (#1)", []string{"release:major"}, "major", "v2.0.0"},
		{"feat title", "feat: new source (#1)", nil, "minor", "v1.3.0"},
		{"feat with scope, any case", "Feat(web): new page (#1)", nil, "minor", "v1.3.0"},
		{"breaking in body", "fix: x (#1)\n\nBREAKING CHANGE: config renamed", nil, "major", "v2.0.0"},
		{"bang title", "feat!: drop python (#1)", nil, "major", "v2.0.0"},
		{"label beats title", "feat: x (#1)", []string{"release:major"}, "major", "v2.0.0"},
		{"plain title stays patch", "MAI as the live source (#1)", nil, "patch", "v1.2.8"},
		{"a word merely starting with feat", "feature flags cleanup (#1)", nil, "patch", "v1.2.8"},
	}
	for _, c := range cases {
		d, err := Decide(Input{Tags: []string{"v1.2.7"}, Message: c.msg, Labels: c.labels, Files: code})
		if err != nil {
			t.Fatal(err)
		}
		if d.Bump != c.want || d.Version != c.version {
			t.Errorf("%s: got bump %s -> %s, want %s -> %s", c.name, d.Bump, d.Version, c.want, c.version)
		}
	}
}

func TestOptingOutOfARelease(t *testing.T) {
	code := []string{"go/internal/web/server.go"}
	for name, d := range map[string]Input{
		"marker in the message": {Tags: []string{"v1.2.7"}, Message: "fix: x [skip release] (#1)", Files: code},
		"marker in the body":    {Tags: []string{"v1.2.7"}, Message: "fix: x\n\n[Skip Release]", Files: code},
		"skip label":            {Tags: []string{"v1.2.7"}, Message: "fix: x", Labels: []string{"release:skip"}, Files: code},
	} {
		got, _ := Decide(d)
		if got.Release {
			t.Errorf("%s: released anyway: %+v", name, got)
		}
	}
}

func TestAPersonCanForceAReleaseAndChooseTheBump(t *testing.T) {
	// Only docs changed, but someone asked for a release by hand.
	d, err := Decide(Input{Tags: []string{"v1.2.7"}, Message: "docs", Files: []string{"README.md"}, Force: true, Bump: "minor"})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Release || d.Version != "v1.3.0" {
		t.Errorf("forced minor = %+v, want release v1.3.0", d)
	}
	// An explicit bump overrides what the commit says.
	d, _ = Decide(Input{Tags: []string{"v1.2.7"}, Message: "feat!: x", Files: []string{"go/go.mod"}, Bump: "patch"})
	if d.Version != "v1.2.8" {
		t.Errorf("explicit patch gave %s, want v1.2.8", d.Version)
	}
	// Force also overrides an opt-out marker.
	d, _ = Decide(Input{Tags: []string{"v1.2.7"}, Message: "fix [skip release]", Files: []string{"go/go.mod"}, Force: true})
	if !d.Release {
		t.Error("a forced release was blocked by [skip release]")
	}
	if _, err := Decide(Input{Tags: []string{"v1.2.7"}, Bump: "huge", Files: []string{"go/go.mod"}}); err == nil {
		t.Error("an invalid bump was accepted")
	}
}

func TestVersionsAreComparedNumericallyAndOddTagsIgnored(t *testing.T) {
	tags := []string{"v1.9.0", "v1.10.0", "v1.2.7", "python-legacy", "v1.2.6-go", "v2", "latest", "1.99.0", "v1.10.0-rc1"}
	d, _ := Decide(Input{Tags: tags, Message: "fix", Files: []string{"go/go.mod"}})
	if d.Version != "v1.10.1" {
		t.Errorf("next after %v = %s, want v1.10.1 (1.10 > 1.9; non-release tags ignored)", tags, d.Version)
	}
}

func TestFirstEverRelease(t *testing.T) {
	d, _ := Decide(Input{Tags: []string{"python-legacy"}, Message: "fix", Files: []string{"go/go.mod"}})
	if !d.Release || d.Version != "v0.0.1" {
		t.Errorf("first release = %+v, want v0.0.1", d)
	}
}

func TestParseTagList(t *testing.T) {
	got := splitList(" v1.0.0, v1.1.0 ,,x ")
	if want := []string{"v1.0.0", "v1.1.0", "x"}; !reflect.DeepEqual(got, want) {
		t.Errorf("splitList = %q, want %q", got, want)
	}
}

// ── explicit labels are explicit intent ──────────────────────────────────────

func TestAReleaseLabelForcesAReleaseEvenWhenNothingShippedChanged(t *testing.T) {
	docs := []string{"README.md", "go/TESTING.md", ".github/workflows/ci.yml"}
	for _, c := range []struct {
		label, version, bump string
	}{
		{"release:patch", "v1.2.8", "patch"},
		{"release:minor", "v1.3.0", "minor"},
		{"release:major", "v2.0.0", "major"},
	} {
		d, err := Decide(Input{Tags: []string{"v1.2.7"}, Message: "docs: tidy (#1)", Labels: []string{c.label}, Files: docs})
		if err != nil {
			t.Fatal(err)
		}
		if !d.Release || d.Version != c.version || d.Bump != c.bump {
			t.Errorf("%s on a docs-only change: %+v, want release %s (%s)", c.label, d, c.version, c.bump)
		}
		if !strings.Contains(d.Reason, c.label) {
			t.Errorf("reason %q does not say the label forced it", d.Reason)
		}
	}
}

func TestOnlyALabelForcesAReleaseNotATitle(t *testing.T) {
	// A feat: title still only chooses the bump; with nothing shipped there is no release.
	d, _ := Decide(Input{Tags: []string{"v1.2.7"}, Message: "feat: new docs (#1)", Files: []string{"README.md"}})
	if d.Release {
		t.Errorf("a feat: title alone forced a release: %+v", d)
	}
	// Unrelated labels do nothing.
	d, _ = Decide(Input{Tags: []string{"v1.2.7"}, Message: "docs", Labels: []string{"documentation", "dependencies"}, Files: []string{"README.md"}})
	if d.Release {
		t.Errorf("unrelated labels forced a release: %+v", d)
	}
}

func TestOptOutBeatsAReleaseLabel(t *testing.T) {
	for name, in := range map[string]Input{
		"skip label too": {Tags: []string{"v1.2.7"}, Message: "x", Labels: []string{"release:minor", "release:skip"}, Files: []string{"README.md"}},
		"skip marker":    {Tags: []string{"v1.2.7"}, Message: "x [skip release]", Labels: []string{"release:minor"}, Files: []string{"go/go.mod"}},
	} {
		if d, _ := Decide(in); d.Release {
			t.Errorf("%s: released although the change opted out: %+v", name, d)
		}
	}
}

func TestExplicitLabelsBeatWhatTheTitleSuggests(t *testing.T) {
	code := []string{"go/internal/web/server.go"}
	for _, c := range []struct {
		name, msg string
		labels    []string
		want      string
	}{
		{"patch label over a feat title", "feat: x (#1)", []string{"release:patch"}, "v1.2.8"},
		{"minor label over a breaking marker", "fix: x\n\nBREAKING CHANGE: y", []string{"release:minor"}, "v1.3.0"},
		{"the highest of several labels", "x", []string{"release:patch", "release:major", "release:minor"}, "v2.0.0"},
		{"labels are case-insensitive", "x", []string{"Release:Minor"}, "v1.3.0"},
	} {
		d, _ := Decide(Input{Tags: []string{"v1.2.7"}, Message: c.msg, Labels: c.labels, Files: code})
		if d.Version != c.want {
			t.Errorf("%s: got %s, want %s", c.name, d.Version, c.want)
		}
	}
}

// ── markers must be deliberate, not words that happen to appear ───────────────
//
// Found on the first real release: a PR whose description EXPLAINED the
// "[skip release]" marker was itself treated as opting out, so nothing was
// released. A marker counts only when it is on purpose: in the title, or on a line
// of its own. Likewise "BREAKING CHANGE" counts as the footer of a commit message,
// at the start of a line, not when it is mentioned in a sentence.

func TestProseThatMentionsAMarkerDoesNotTriggerIt(t *testing.T) {
	code := []string{"go/internal/web/server.go"}
	for name, msg := range map[string]string{
		"explains skip, in a sentence":     "feat: release automation (#63)\n\nUse the label release:skip or put [skip release] in the title to opt out.",
		"explains skip, in code quotes":    "fix: x (#1)\n\nThe marker `[skip release]` opts a merge out.",
		"explains skip, in a bullet":       "fix: x (#1)\n\n- `[skip release]` in the PR title/body opts out",
		"explains skip, inline after text": "fix: x\n\nTo avoid a release write [skip release] here.",
	} {
		d, _ := Decide(Input{Tags: []string{"v1.2.7"}, Message: msg, Files: code})
		if !d.Release {
			t.Errorf("%s: wrongly opted out: %+v", name, d)
		}
	}
	// "BREAKING CHANGE" mentioned in a sentence is not a breaking change.
	d, _ := Decide(Input{Tags: []string{"v1.2.7"}, Message: "docs: x (#1)\n\nA footer such as BREAKING CHANGE: y would make this a major release.", Files: code})
	if d.Bump == "major" {
		t.Errorf("a sentence mentioning BREAKING CHANGE raised a major bump: %+v", d)
	}
}

func TestDeliberateMarkersStillWork(t *testing.T) {
	code := []string{"go/internal/web/server.go"}
	for name, msg := range map[string]string{
		"in the title":             "fix: x [skip release] (#1)",
		"in the title, other case": "Fix: x [Skip Release]",
		"on a line of its own":     "fix: x (#1)\n\n[skip release]",
		"on its own line, padded":  "fix: x (#1)\n\nSome text.\n   [Skip Release]   \nMore.",
	} {
		if d, _ := Decide(Input{Tags: []string{"v1.2.7"}, Message: msg, Files: code}); d.Release {
			t.Errorf("%s: a deliberate opt-out was ignored: %+v", name, d)
		}
	}
	for name, msg := range map[string]string{
		"footer":                   "fix: x (#1)\n\nBREAKING CHANGE: config renamed",
		"footer with hyphen":       "fix: x (#1)\n\nBREAKING-CHANGE: config renamed",
		"footer after other lines": "fix: x (#1)\n\nDetails.\n\nBREAKING CHANGE: y",
	} {
		if d, _ := Decide(Input{Tags: []string{"v1.2.7"}, Message: msg, Files: code}); d.Bump != "major" {
			t.Errorf("%s: a breaking-change footer was ignored: %+v", name, d)
		}
	}
}
