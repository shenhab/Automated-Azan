package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Run is one GitHub check run on a commit.
type Run struct {
	ID         int64
	Name       string
	Status     string // queued, in_progress, completed
	Conclusion string // success, failure, cancelled, skipped, ... (empty until completed)
}

// Verdict summarises the required checks: "ready" (all passed), "pending"
// (something is missing or still running) or "failed".
type Verdict struct {
	State  string
	Detail string
}

// ParseRuns decodes the check runs as listed by the GitHub API.
func ParseRuns(data []byte) ([]Run, error) {
	var raw []struct {
		ID         int64   `json:"id"`
		Name       string  `json:"name"`
		Status     string  `json:"status"`
		Conclusion *string `json:"conclusion"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("check runs: %w", err)
	}
	out := make([]Run, 0, len(raw))
	for _, r := range raw {
		run := Run{ID: r.ID, Name: r.Name, Status: r.Status}
		if r.Conclusion != nil {
			run.Conclusion = *r.Conclusion
		}
		out = append(out, run)
	}
	return out, nil
}

// EvaluateChecks decides whether every required check has passed.
//
// For each required name the newest run counts (so a successful re-run replaces an
// earlier failure), but skipped runs are ignored entirely: a skipped job of the
// same name from another workflow or a dry run must never stand in for a real
// result, and a check that only ever shows as skipped has not really run.
// A failure anywhere wins over "still running".
func EvaluateChecks(runs []Run, required []string) Verdict {
	pending := []string{}
	for _, name := range required {
		var mine []Run
		for _, r := range runs {
			if r.Name == name && r.Conclusion != "skipped" {
				mine = append(mine, r)
			}
		}
		if len(mine) == 0 {
			pending = append(pending, name+" (not started)")
			continue
		}
		sort.Slice(mine, func(i, j int) bool { return mine[i].ID < mine[j].ID })
		latest := mine[len(mine)-1]
		switch {
		case latest.Status != "completed":
			pending = append(pending, name+" (running)")
		case latest.Conclusion == "success" || latest.Conclusion == "neutral":
		default:
			return Verdict{State: "failed", Detail: fmt.Sprintf("%s finished with %q", name, latest.Conclusion)}
		}
	}
	if len(pending) > 0 {
		return Verdict{State: "pending", Detail: strings.Join(pending, ", ")}
	}
	return Verdict{State: "ready", Detail: "all required checks passed"}
}
