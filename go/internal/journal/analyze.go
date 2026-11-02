package journal

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"azan-agent/internal/trace"
)

// Analyze turns a session's events into a verdict. It is a pure function so it
// can be tested against exact timelines (including real traces).
//
// The speaker's own reports (events from the watcher with role "speaker") are
// read only after the agent's first "load-sent": whatever was playing before
// (the pre-Fajr Quran stream, a leftover) is not the Athan. Playback "ended" at
// the first IDLE/app-closed after it started. healthIssues are device problems
// found by the health snapshots. ended says whether the session is over.
func Analyze(spec Spec, events []trace.Event, healthIssues []string, ended bool) Result {
	evs := append([]trace.Event(nil), events...)
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].At.Before(evs[j].At) })

	var f Facts
	f.Health = healthIssues
	var anchor time.Time
	var lastStop time.Time
	var level float64
	var levelKnown, muted bool
	var joined []string

	for _, e := range evs {
		if e.Source == trace.SourceAgent {
			switch e.Kind {
			case "group-members":
				f.Members, joined = fieldStrings(e, "configured"), fieldStrings(e, "joined")
			case "member-not-joined":
				f.MembersNotJoined = appendOnce(f.MembersNotJoined, e.Device)
			case "member-silent":
				f.MemberWarnings = appendOnce(f.MemberWarnings, e.Text)
			case "load-sent":
				if anchor.IsZero() {
					anchor = e.At
				}
			case "retry":
				f.Retries++
			case "stop-sent":
				lastStop = e.At
			case "play-failed":
				f.AgentError = fieldString(e, "error", e.Text)
			}
			continue
		}
		if e.Source == trace.SourceDevice && e.Kind == "member-played" {
			f.MembersPlayed = appendOnce(f.MembersPlayed, e.Device)
			continue
		}
		if e.Source != trace.SourceDevice || fieldString(e, "role", "") != "speaker" {
			continue
		}
		switch e.Kind {
		case "volume":
			if f.PlayedFrom.IsZero() {
				level, levelKnown = fieldFloat(e, "level"), true
				muted, _ = e.Fields["muted"].(bool)
			}
		case "player-state":
			if anchor.IsZero() || e.At.Before(anchor) {
				continue
			}
			state := fieldString(e, "state", "")
			switch {
			case (state == "PLAYING" || state == "BUFFERING") && f.PlayedFrom.IsZero():
				f.Started, f.PlayedFrom = true, e.At
			case state == "PAUSED" && !f.PlayedFrom.IsZero() && f.PlayedTo.IsZero():
				f.Paused = true
			case state == "IDLE" && !f.PlayedFrom.IsZero() && f.PlayedTo.IsZero():
				f.PlayedTo = e.At
				f.EndReason = fieldString(e, "idle_reason", "")
				if f.EndReason == "" {
					f.EndReason = "ended"
				}
			}
		case "app-closed":
			if !anchor.IsZero() && !e.At.Before(anchor) && !f.PlayedFrom.IsZero() && f.PlayedTo.IsZero() {
				f.PlayedTo, f.EndReason = e.At, "app closed"
			}
		}
	}

	if levelKnown {
		f.Volume, f.VolumeKnown, f.Muted = level, true, muted
	}
	if !f.PlayedTo.IsZero() {
		f.DurationSec = f.PlayedTo.Sub(f.PlayedFrom).Seconds()
		f.StoppedByAgent = !lastStop.IsZero() && f.PlayedTo.Sub(lastStop) >= -time.Second && f.PlayedTo.Sub(lastStop) <= 20*time.Second
	}
	// Outside changes between the request and the end of the playback.
	for _, e := range evs {
		if !e.External || anchor.IsZero() || e.At.Before(anchor) {
			continue
		}
		if !f.PlayedTo.IsZero() && e.At.After(f.PlayedTo.Add(time.Second)) {
			continue
		}
		f.External = append(f.External, fmt.Sprintf("%s %s (%s)", e.At.Format("15:04:05"), e.Text, e.Device))
	}

	sort.Strings(f.MembersPlayed)
	if f.Started {
		for _, name := range joined {
			if !contains(f.MembersPlayed, name) {
				f.MembersUnconfirmed = append(f.MembersUnconfirmed, name)
			}
		}
	}

	if !ended {
		return Result{Verdict: VerdictRunning, Headline: "In progress…", Facts: f}
	}
	if !spec.ExpectPlayback {
		return Result{Verdict: VerdictOK, Headline: "Nothing was expected to play", Facts: f}
	}
	if !f.Started {
		h := "Never started playing"
		switch {
		case f.AgentError != "":
			h += ": " + f.AgentError
		case anchor.IsZero():
			h += " — the agent never asked the speaker to play"
		default:
			h += " — the speaker did not report any playback"
		}
		return Result{Verdict: VerdictFailed, Headline: h, Facts: f}
	}

	var parts, warn []string
	played := fmt.Sprintf("Played %s", f.PlayedFrom.Format("15:04:05"))
	if !f.PlayedTo.IsZero() {
		played += fmt.Sprintf("–%s (%s)", f.PlayedTo.Format("15:04:05"), humanDuration(f.PlayedTo.Sub(f.PlayedFrom)))
	} else {
		played += " (still playing when the recording stopped)"
	}
	if f.VolumeKnown {
		played += fmt.Sprintf(" at volume %.2f", f.Volume)
	}
	parts = append(parts, played)

	switch {
	case f.PlayedTo.IsZero():
		if spec.Kind == "athan" {
			warn = append(warn, "no end was seen")
		}
	case f.EndReason == "FINISHED":
		parts = append(parts, "finished normally")
	case f.StoppedByAgent:
		parts = append(parts, "stopped by the agent")
	default:
		after := humanDuration(f.PlayedTo.Sub(f.PlayedFrom))
		why := f.EndReason
		if len(f.External) > 0 {
			why = "something else changed the speaker (" + f.EndReason + ")"
		}
		warn = append(warn, fmt.Sprintf("ended early after %s: %s", after, why))
	}
	if len(f.External) > 0 && !strings.Contains(strings.Join(warn, ";"), "something else") {
		warn = append(warn, fmt.Sprintf("something else changed a device during playback (%d change(s))", len(f.External)))
	}
	if f.Muted {
		warn = append(warn, "the speaker was muted")
	} else if f.VolumeKnown && f.Volume < 0.02 {
		warn = append(warn, fmt.Sprintf("the speaker's volume was %.2f", f.Volume))
	}
	if f.Paused {
		warn = append(warn, "the speaker was paused")
	}
	if f.Retries > 0 {
		warn = append(warn, fmt.Sprintf("needed %d retr%s", f.Retries, map[bool]string{true: "y", false: "ies"}[f.Retries == 1]))
	}
	for _, name := range f.MembersNotJoined {
		warn = append(warn, name+" did not join the group")
	}
	warn = append(warn, f.MemberWarnings...)
	warn = append(warn, healthIssues...)

	head := strings.Join(parts, " — ")
	if len(warn) == 0 {
		return Result{Verdict: VerdictOK, Headline: head, Facts: f}
	}
	return Result{Verdict: VerdictWarning, Headline: head + "; " + strings.Join(warn, "; "), Facts: f}
}

// fieldStrings reads a list of names: []string in memory, []any once a session
// has been through its JSON file.
func fieldStrings(e trace.Event, key string) []string {
	switch v := e.Fields[key].(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		var out []string
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func appendOnce(list []string, s string) []string {
	if contains(list, s) {
		return list
	}
	return append(list, s)
}

func fieldString(e trace.Event, key, def string) string {
	if v, ok := e.Fields[key].(string); ok && v != "" {
		return v
	}
	return def
}

func fieldFloat(e trace.Event, key string) float64 {
	switch v := e.Fields[key].(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	}
	return 0
}

// humanDuration renders 224s as "3m44s" and 6s as "6s".
func humanDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}
