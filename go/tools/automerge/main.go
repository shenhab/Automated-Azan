// Command automerge decides whether a Dependabot pull request may be merged
// without a person once CI is green. The Dependabot auto-merge workflow runs it.
//
// "Safe" means a small, well-understood kind of update that touches only the files
// that kind of update should touch:
//
//	Go dependencies     patch and minor
//	Docker base image   patch only
//	GitHub Actions      patch only
//
// Everything else is left for a person: major updates, a change of Go version or
// toolchain, unexpected files, missing metadata, other ecosystems, and anything
// not authored by Dependabot. The rules are tested in automerge_test.go.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
)

const dependabot = "dependabot[bot]"

// Input is what the decision depends on.
type Input struct {
	Author             string   // login of the pull request author
	Branch             string   // head branch, e.g. dependabot/go_modules/go/...
	Message            string   // Dependabot's commit message (carries the update metadata)
	Files              []string // files changed by the pull request
	GoDirectiveChanged bool     // go/go.mod's "go" or "toolchain" line changed
}

// Decision is the outcome.
type Decision struct {
	Merge     bool   // safe to merge automatically
	Release   bool   // the merge changes what ships, so a release should be started
	Ecosystem string // go_modules, docker or github_actions (when recognised)
	Reason    string
}

type update struct{ name, kind string }

var (
	branchRE   = regexp.MustCompile(`^dependabot/([a-z_]+)/`)
	nameRE     = regexp.MustCompile(`^- dependency-name:\s*(.+?)\s*$`)
	typeRE     = regexp.MustCompile(`^\s+update-type:\s*version-update:semver-(\w+)\s*$`)
	dockerFile = regexp.MustCompile(`^go/Dockerfile[^/]*$`)
	workflow   = regexp.MustCompile(`^\.github/workflows/[^/]+\.ya?ml$`)
)

// policy: which update kinds may merge automatically, per ecosystem.
var allowed = map[string]map[string]bool{
	"go_modules":     {"patch": true, "minor": true},
	"docker":         {"patch": true},
	"github_actions": {"patch": true},
}

// parseUpdates reads the "updated-dependencies" block Dependabot puts in its
// commit message. An entry without a recognisable update type is returned with an
// empty kind, so it can never be treated as safe.
func parseUpdates(msg string) []update {
	i := strings.Index(msg, "updated-dependencies:")
	if i < 0 {
		return nil
	}
	var out []update
	cur := -1
	for _, line := range strings.Split(msg[i:], "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "..." {
			break
		}
		if m := nameRE.FindStringSubmatch(line); m != nil {
			out = append(out, update{name: m[1]})
			cur = len(out) - 1
			continue
		}
		if m := typeRE.FindStringSubmatch(line); m != nil && cur >= 0 {
			switch m[1] {
			case "patch", "minor", "major":
				out[cur].kind = m[1]
			}
		}
	}
	return out
}

func filesAllowed(eco string, files []string) (bad []string) {
	for _, f := range files {
		ok := false
		switch eco {
		case "go_modules":
			ok = f == "go/go.mod" || f == "go/go.sum"
		case "docker":
			ok = dockerFile.MatchString(f)
		case "github_actions":
			ok = workflow.MatchString(f)
		}
		if !ok {
			bad = append(bad, f)
		}
	}
	return bad
}

// Decide applies the policy.
func Decide(in Input) Decision {
	no := func(eco, format string, a ...any) Decision {
		return Decision{Ecosystem: eco, Reason: "needs a person: " + fmt.Sprintf(format, a...)}
	}
	if in.Author != dependabot {
		return no("", "not authored by Dependabot (%q)", in.Author)
	}
	m := branchRE.FindStringSubmatch(in.Branch)
	if m == nil {
		return no("", "branch %q is not a Dependabot branch", in.Branch)
	}
	eco := m[1]
	rules, known := allowed[eco]
	if !known {
		return no(eco, "the %s ecosystem is not covered by the policy", eco)
	}

	updates := parseUpdates(in.Message)
	if len(updates) == 0 {
		return no(eco, "no update metadata in the commit message")
	}
	for _, u := range updates {
		if u.kind == "" {
			return no(eco, "%s has no readable update type", u.name)
		}
		if !rules[u.kind] {
			return no(eco, "%s is a %s update (%s allows %s)", u.name, u.kind, eco, kinds(rules))
		}
	}
	if len(in.Files) == 0 {
		return no(eco, "the pull request changes no files")
	}
	if bad := filesAllowed(eco, in.Files); len(bad) > 0 {
		return no(eco, "it also changes %s", strings.Join(bad, ", "))
	}
	if eco == "go_modules" && in.GoDirectiveChanged {
		return no(eco, "it changes the Go version or toolchain")
	}

	var names []string
	for _, u := range updates {
		names = append(names, u.name+" ("+u.kind+")")
	}
	return Decision{
		Merge:     true,
		Release:   eco != "github_actions", // Go and Docker updates change what ships
		Ecosystem: eco,
		Reason:    fmt.Sprintf("%s: %s", eco, strings.Join(names, ", ")),
	}
}

func kinds(rules map[string]bool) string {
	var out []string
	for _, k := range []string{"patch", "minor", "major"} {
		if rules[k] {
			out = append(out, k)
		}
	}
	return strings.Join(out, " and ")
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
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
	author := flag.String("author", "", "pull request author login")
	branch := flag.String("branch", "", "pull request head branch")
	msgFile := flag.String("message-file", "", "file with Dependabot's commit message")
	filesFile := flag.String("files", "", "file listing changed files, one per line")
	goDir := flag.Bool("go-directive-changed", false, "go.mod's go/toolchain line changed")
	checksFile := flag.String("checks-file", "", "check runs JSON: evaluate the required checks instead of deciding")
	required := flag.String("required", "", "comma-separated required check names (with -checks-file)")
	flag.Parse()

	if *checksFile != "" {
		b, err := os.ReadFile(*checksFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "automerge:", err)
			os.Exit(2)
		}
		runs, err := ParseRuns(b)
		if err != nil {
			fmt.Fprintln(os.Stderr, "automerge:", err)
			os.Exit(2)
		}
		var names []string
		for _, n := range strings.Split(*required, ",") {
			if n = strings.TrimSpace(n); n != "" {
				names = append(names, n)
			}
		}
		v := EvaluateChecks(runs, names)
		fmt.Printf("state=%s\ndetail=%s\n", v.State, v.Detail)
		return
	}

	msg := ""
	if *msgFile != "" {
		b, err := os.ReadFile(*msgFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "automerge:", err)
			os.Exit(2)
		}
		msg = string(b)
	}
	var files []string
	if *filesFile != "" {
		var err error
		if files, err = readLines(*filesFile); err != nil {
			fmt.Fprintln(os.Stderr, "automerge:", err)
			os.Exit(2)
		}
	}
	d := Decide(Input{Author: *author, Branch: *branch, Message: msg, Files: files, GoDirectiveChanged: *goDir})
	// KEY=value lines, suitable for appending to $GITHUB_OUTPUT.
	fmt.Printf("merge=%t\nrelease=%t\necosystem=%s\nreason=%s\n", d.Merge, d.Release, d.Ecosystem, strings.ReplaceAll(d.Reason, "\n", " "))
}
