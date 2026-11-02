package journal

import (
	"testing"
	"time"

	"azan-agent/internal/trace"
)

func action(sec float64, kind, host string) trace.Event {
	return trace.Event{At: at(sec), Source: trace.SourceAgent, Kind: kind, Fields: map[string]any{"host": host}}
}

func chg(sec float64, kind string, kv ...any) change {
	return change{At: at(sec), Kind: kind, Fields: kvs(kv)}
}

func TestChangesTheAgentCausedAreNotOutsideInterference(t *testing.T) {
	cases := []struct {
		name    string
		actions []trace.Event
		host    string
		c       change
		want    bool // caused by the agent
	}{
		{"app launched after our Load", []trace.Event{action(0, "load-sent", "10.0.0.5")}, "10.0.0.5", chg(2, "app-launched"), true},
		{"playing after our Load", []trace.Event{action(0, "load-sent", "10.0.0.5")}, "10.0.0.5", chg(3, "player-state", "state", "PLAYING"), true},
		{"app closed after our stop", []trace.Event{action(100, "stop-sent", "10.0.0.5")}, "10.0.0.5", chg(101, "app-closed"), true},
		{"cancelled after our stop", []trace.Event{action(100, "stop-sent", "10.0.0.5")}, "10.0.0.5", chg(101, "player-state", "state", "IDLE", "idle_reason", "CANCELLED"), true},
		{"volume change after our volume command", []trace.Event{action(200, "volume-set", "10.0.0.9")}, "10.0.0.9", chg(201, "volume"), true},
		{"app closed with no stop from us", []trace.Event{action(0, "load-sent", "10.0.0.5")}, "10.0.0.5", chg(50, "app-closed"), false},
		{"app closed long after our stop", []trace.Event{action(100, "stop-sent", "10.0.0.5")}, "10.0.0.5", chg(160, "app-closed"), false},
		{"volume changed on a device we never touched", []trace.Event{action(0, "load-sent", "10.0.0.5")}, "10.0.0.5", chg(60, "volume"), false},
		{"someone interrupted our media", []trace.Event{action(0, "load-sent", "10.0.0.5")}, "10.0.0.5", chg(60, "player-state", "state", "IDLE", "idle_reason", "INTERRUPTED"), false},
		{"our command went to a different device", []trace.Event{action(0, "load-sent", "10.0.0.5")}, "10.0.0.7", chg(2, "app-launched"), false},
		{"the change came before our command", []trace.Event{action(0, "load-sent", "10.0.0.5")}, "10.0.0.5", chg(-5, "app-launched"), false},
	}
	for _, tc := range cases {
		a := newAttribution()
		for _, e := range tc.actions {
			a.Add(e)
		}
		if got := a.Caused(tc.host, tc.c); got != tc.want {
			t.Errorf("%s: caused = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestNaturalEndsAndFaultsAreNeverBlamedOnSomeoneElse(t *testing.T) {
	a := newAttribution()
	if !a.Caused("h", chg(5, "player-state", "state", "IDLE", "idle_reason", "FINISHED")) {
		t.Error("a file that played to its end is not interference")
	}
	if !a.Caused("h", chg(5, "player-state", "state", "IDLE", "idle_reason", "ERROR")) {
		t.Error("a device error is a fault, not interference")
	}
	if !a.Caused("h", chg(5, "initial-state")) {
		t.Error("the first look at a device is not a change")
	}
}

func TestAnActionWithoutAHostCoversEveryDevice(t *testing.T) {
	a := newAttribution()
	a.Add(trace.Event{At: at(0), Source: trace.SourceAgent, Kind: "load-sent"})
	if !a.Caused("any-host", chg(2, "app-launched")) {
		t.Error("an action with no host should cover any device")
	}
}

func TestActionsAreForgottenAfterTheirWindow(t *testing.T) {
	a := newAttribution()
	a.Add(action(0, "load-sent", "h"))
	a.Add(action(1000, "load-sent", "h")) // trimming happens while adding
	if len(a.actions) > 1 {
		t.Errorf("old actions kept: %d", len(a.actions))
	}
	_ = time.Second
}
