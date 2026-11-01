// Command nextversion decides whether a change that reached main should be
// released and, if so, which version it gets. The release workflow runs it.
//
// The rules (see RELEASING.md and nextversion_test.go):
//   - only a change to what users run (the agent and its image) releases; tests,
//     docs and CI do not;
//   - the version is the highest vMAJOR.MINOR.PATCH tag plus a bump: patch by
//     default, raised by a "feat:" title or a breaking-change marker;
//   - a PR label release:patch / release:minor / release:major is explicit intent:
//     it forces a release even if nothing users run changed, and sets the bump
//     (explicit labels win over what the title suggests; the highest label wins);
//   - "[skip release]" or the label release:skip opts out, and beats a release
//     label; a person running the workflow by hand can force a release and choose
//     the bump.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Input is everything the decision depends on.
type Input struct {
	Tags    []string // all repository tags
	Message string   // commit message (title and body)
	Labels  []string // labels on the merged pull request
	Files   []string // files changed by the push
	Bump    string   // "", "auto", "patch", "minor" or "major"
	Force   bool     // release even if nothing shipped changed or an opt-out is present
}

// Decision is the outcome.
type Decision struct {
	Release bool
	Version string // vMAJOR.MINOR.PATCH (set when Release)
	Bump    string
	Reason  string
}

var (
	semverTag     = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)
	breakingTitle = regexp.MustCompile(`^\w+(\([^)]*\))?!:`)
	featTitle     = regexp.MustCompile(`(?i)^feat(\([^)]*\))?:`)
)

type version struct{ major, minor, patch int }

func (v version) String() string { return fmt.Sprintf("v%d.%d.%d", v.major, v.minor, v.patch) }

func (v version) less(o version) bool {
	if v.major != o.major {
		return v.major < o.major
	}
	if v.minor != o.minor {
		return v.minor < o.minor
	}
	return v.patch < o.patch
}

// latest returns the highest plain release tag (vX.Y.Z); pre-release and other
// tags are ignored. The zero version means there is none yet.
func latest(tags []string) version {
	var all []version
	for _, t := range tags {
		if m := semverTag.FindStringSubmatch(strings.TrimSpace(t)); m != nil {
			a, _ := strconv.Atoi(m[1])
			b, _ := strconv.Atoi(m[2])
			c, _ := strconv.Atoi(m[3])
			all = append(all, version{a, b, c})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].less(all[j]) })
	if len(all) == 0 {
		return version{}
	}
	return all[len(all)-1]
}

// isShipped reports whether a changed file alters what users run: the agent, its
// bundled data and web assets, its dependencies, or how it is built and
// packaged. Tests, test support, docs, CI and local tooling do not.
func isShipped(path string) bool {
	if !strings.HasPrefix(path, "go/") {
		return false
	}
	switch {
	case strings.HasSuffix(path, "_test.go"),
		strings.HasSuffix(path, ".md"),
		strings.HasPrefix(path, "go/e2e/"),
		strings.HasPrefix(path, "go/tools/"),
		strings.HasPrefix(path, "go/scripts/"),
		strings.HasPrefix(path, "go/internal/fakecast/"),
		strings.Contains(path, "/testdata/"),
		path == "go/Makefile", path == "go/build.sh", path == "go/Dockerfile.dockerignore",
		strings.HasSuffix(path, ".gitignore"):
		return false
	}
	return true
}

func hasLabel(labels []string, want string) bool {
	for _, l := range labels {
		if strings.EqualFold(strings.TrimSpace(l), want) {
			return true
		}
	}
	return false
}

// Decide applies the release rules.
func Decide(in Input) (Decision, error) {
	bump := strings.ToLower(strings.TrimSpace(in.Bump))
	switch bump {
	case "", "auto":
		bump = ""
	case "patch", "minor", "major":
	default:
		return Decision{}, fmt.Errorf("invalid bump %q (use auto, patch, minor or major)", in.Bump)
	}

	// An explicit release label is explicit intent: it sets the bump and forces the
	// release. The highest label wins.
	labelBump := ""
	for _, l := range []string{"patch", "minor", "major"} {
		if hasLabel(in.Labels, "release:"+l) {
			labelBump = l
		}
	}

	lowerMsg := strings.ToLower(in.Message)
	reason := ""
	if !in.Force {
		if strings.Contains(lowerMsg, "[skip release]") || hasLabel(in.Labels, "release:skip") {
			return Decision{Reason: "opted out of releasing ([skip release] or label release:skip)"}, nil
		}
		shipped := false
		for _, f := range in.Files {
			if isShipped(strings.TrimSpace(f)) {
				shipped = true
				break
			}
		}
		switch {
		case shipped:
		case labelBump != "":
			reason = fmt.Sprintf("forced by the label release:%s (nothing users run changed)", labelBump)
		default:
			return Decision{Reason: "nothing users run changed (only tests, docs, CI or tooling)"}, nil
		}
	}

	title := strings.TrimSpace(strings.SplitN(in.Message, "\n", 2)[0])
	switch {
	case bump != "": // chosen explicitly by a person running the workflow
	case labelBump != "":
		bump = labelBump
	case strings.Contains(in.Message, "BREAKING CHANGE"), breakingTitle.MatchString(title):
		bump = "major"
	case featTitle.MatchString(title):
		bump = "minor"
	default:
		bump = "patch"
	}

	next := latest(in.Tags)
	switch bump {
	case "major":
		next = version{next.major + 1, 0, 0}
	case "minor":
		next = version{next.major, next.minor + 1, 0}
	default:
		next = version{next.major, next.minor, next.patch + 1}
	}
	if reason == "" {
		reason = fmt.Sprintf("%s release", bump)
	}
	return Decision{Release: true, Version: next.String(), Bump: bump, Reason: reason}, nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func readLines(path string) ([]string, error) {
	if path == "" {
		return nil, nil
	}
	f := os.Stdin
	if path != "-" {
		var err error
		if f, err = os.Open(path); err != nil {
			return nil, err
		}
		defer f.Close()
	}
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if l := strings.TrimSpace(sc.Text()); l != "" {
			out = append(out, l)
		}
	}
	return out, sc.Err()
}

func main() {
	tags := flag.String("tags", "", "comma-separated list of all tags")
	msgFile := flag.String("message-file", "", "file with the commit message")
	labels := flag.String("labels", "", "comma-separated pull request labels")
	filesFile := flag.String("files", "", "file listing changed files, one per line ('-' for stdin)")
	bump := flag.String("bump", "auto", "auto, patch, minor or major")
	force := flag.Bool("force", false, "release even if nothing shipped changed")
	flag.Parse()

	files, err := readLines(*filesFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "nextversion:", err)
		os.Exit(2)
	}
	msg := ""
	if *msgFile != "" {
		b, err := os.ReadFile(*msgFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "nextversion:", err)
			os.Exit(2)
		}
		msg = string(b)
	}
	d, err := Decide(Input{Tags: splitList(*tags), Message: msg, Labels: splitList(*labels), Files: files, Bump: *bump, Force: *force})
	if err != nil {
		fmt.Fprintln(os.Stderr, "nextversion:", err)
		os.Exit(2)
	}
	// KEY=value lines, suitable for appending to $GITHUB_OUTPUT.
	fmt.Printf("release=%t\nversion=%s\nbump=%s\nreason=%s\n", d.Release, d.Version, d.Bump, strings.ReplaceAll(d.Reason, "\n", " "))
}
