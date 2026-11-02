package journal

import (
	"strings"
	"testing"
	"time"

	"azan-agent/internal/trace"
)

var t0 = time.Date(2026, 11, 2, 5, 39, 0, 0, time.UTC)

func at(sec float64) time.Time { return t0.Add(time.Duration(sec * float64(time.Second))) }

func ag(sec float64, kind string, kv ...any) trace.Event {
	e := trace.Event{At: at(sec), Source: trace.SourceAgent, Kind: kind, Device: "athan", Text: kind}
	e.Fields = kvs(kv)
	return e
}

func dev(sec float64, kind string, kv ...any) trace.Event {
	e := trace.Event{At: at(sec), Source: trace.SourceDevice, Kind: kind, Device: "athan", Text: kind}
	e.Fields = kvs(kv)
	if e.Fields == nil {
		e.Fields = map[string]any{}
	}
	e.Fields["role"] = "speaker"
	return e
}

func kvs(kv []any) map[string]any {
	var m map[string]any
	for i := 0; i+1 < len(kv); i += 2 {
		if m == nil {
			m = map[string]any{}
		}
		m[kv[i].(string)] = kv[i+1]
	}
	return m
}

func ps(sec float64, state, reason string) trace.Event {
	return dev(sec, "player-state", "state", state, "idle_reason", reason)
}

var athanSpec = Spec{Kind: "athan", Prayer: "Fajr", Label: "Fajr Athan", ExpectPlayback: true}

func TestAnAthanThatPlayedToTheEndIsOK(t *testing.T) {
	r := Analyze(athanSpec, []trace.Event{
		dev(-5, "volume", "level", 0.5, "muted", false),
		ag(0, "load-sent"),
		ps(3, "IDLE", ""), ps(3.2, "BUFFERING", ""), ps(3.5, "PLAYING", ""),
		ps(227, "IDLE", "FINISHED"),
	}, nil, true)
	if r.Verdict != VerdictOK {
		t.Fatalf("verdict = %s (%s)", r.Verdict, r.Headline)
	}
	if !r.Facts.Started || r.Facts.EndReason != "FINISHED" {
		t.Errorf("facts = %+v", r.Facts)
	}
	if !strings.Contains(r.Headline, "05:39:03") || !strings.Contains(r.Headline, "3m44s") {
		t.Errorf("headline lacks times/duration: %q", r.Headline)
	}
	if r.Facts.Volume != 0.5 {
		t.Errorf("volume = %v", r.Facts.Volume)
	}
}

func TestAnAthanThatNeverStartedFails(t *testing.T) {
	r := Analyze(athanSpec, []trace.Event{
		ag(0, "load-sent"),
		ag(10, "play-failed", "error", "the speaker accepted the request but did not start playing"),
		ps(1, "IDLE", ""),
	}, nil, true)
	if r.Verdict != VerdictFailed {
		t.Fatalf("verdict = %s (%s)", r.Verdict, r.Headline)
	}
	if !strings.Contains(r.Headline, "did not start playing") {
		t.Errorf("the agent's error is missing: %q", r.Headline)
	}
}

func TestSilenceWithNoAgentActionAtAllIsAFailure(t *testing.T) {
	r := Analyze(athanSpec, nil, nil, true)
	if r.Verdict != VerdictFailed {
		t.Fatalf("verdict = %s", r.Verdict)
	}
}

func TestTheQuranStreamStillPlayingBeforeTheAthanIsNotTheAthan(t *testing.T) {
	// Real Fajr 2026-11-02: the pre-Fajr stream was BUFFERING when the recording
	// started, then the Athan replaced it (IDLE -> BUFFERING -> PLAYING).
	r := Analyze(athanSpec, []trace.Event{
		ps(-40, "BUFFERING", ""),
		ag(0, "load-sent"),
		ps(3, "IDLE", ""), ps(3, "BUFFERING", ""), ps(3.1, "PLAYING", ""),
		ps(227, "IDLE", "FINISHED"),
	}, nil, true)
	if r.Verdict != VerdictOK || r.Facts.PlayedFrom.Sub(t0) != 3*time.Second {
		t.Fatalf("verdict %s from %v (%s)", r.Verdict, r.Facts.PlayedFrom, r.Headline)
	}
}

func TestAnAthanThatWasCutShortByAnOutsideChangeIsAWarning(t *testing.T) {
	// The 23:38 button run: the app closed ~6 s in with no stop from the agent.
	closed := dev(9, "app-closed")
	closed.External = true
	r := Analyze(athanSpec, []trace.Event{
		ag(0, "load-sent"),
		ps(3, "BUFFERING", ""), ps(3.2, "PLAYING", ""),
		closed,
	}, nil, true)
	if r.Verdict != VerdictWarning {
		t.Fatalf("verdict = %s (%s)", r.Verdict, r.Headline)
	}
	if !strings.Contains(r.Headline, "something else") || len(r.Facts.External) != 1 {
		t.Errorf("headline/facts do not blame an outside change: %q %+v", r.Headline, r.Facts.External)
	}
}

func TestStoppingTheQuranOnPurposeIsNotAProblem(t *testing.T) {
	spec := Spec{Kind: "quran", Label: "Pre-Fajr Quran", ExpectPlayback: true}
	r := Analyze(spec, []trace.Event{
		ag(0, "load-sent"), ps(2, "PLAYING", ""),
		ag(1790, "stop-sent"), ps(1791, "IDLE", "CANCELLED"),
	}, nil, true)
	if r.Verdict != VerdictOK || !r.Facts.StoppedByAgent {
		t.Fatalf("verdict %s, stopped by agent %v (%s)", r.Verdict, r.Facts.StoppedByAgent, r.Headline)
	}
}

func TestMutedSpeakerOrRetriesAreWarnings(t *testing.T) {
	muted := Analyze(athanSpec, []trace.Event{
		dev(-1, "volume", "level", 0.5, "muted", true),
		ag(0, "load-sent"), ps(2, "PLAYING", ""), ps(100, "IDLE", "FINISHED"),
	}, nil, true)
	if muted.Verdict != VerdictWarning || !strings.Contains(muted.Headline, "muted") {
		t.Errorf("muted: %s %q", muted.Verdict, muted.Headline)
	}
	retried := Analyze(athanSpec, []trace.Event{
		ag(0, "load-sent"), ag(5, "retry"), ag(6, "load-sent"),
		ps(8, "PLAYING", ""), ps(100, "IDLE", "FINISHED"),
	}, nil, true)
	if retried.Verdict != VerdictWarning || retried.Facts.Retries != 1 {
		t.Errorf("retried: %s retries=%d", retried.Verdict, retried.Facts.Retries)
	}
}

func TestADeviceHealthProblemIsAWarning(t *testing.T) {
	r := Analyze(athanSpec, []trace.Event{
		ag(0, "load-sent"), ps(2, "PLAYING", ""), ps(100, "IDLE", "FINISHED"),
	}, []string{"Nest display restarted since the last prayer"}, true)
	if r.Verdict != VerdictWarning || !strings.Contains(r.Headline, "restarted") {
		t.Errorf("%s %q", r.Verdict, r.Headline)
	}
}

func TestASessionStillRunningIsReportedAsRunning(t *testing.T) {
	r := Analyze(athanSpec, []trace.Event{ag(0, "load-sent"), ps(2, "PLAYING", "")}, nil, false)
	if r.Verdict != VerdictRunning {
		t.Fatalf("verdict = %s", r.Verdict)
	}
}

func TestSessionsThatExpectNothingAreOK(t *testing.T) {
	r := Analyze(Spec{Kind: "manual", Label: "Stop"}, nil, nil, true)
	if r.Verdict != VerdictOK {
		t.Fatalf("verdict = %s", r.Verdict)
	}
}
