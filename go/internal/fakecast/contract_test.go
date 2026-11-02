package fakecast

// Contract tests: they hold the fake speaker to two outside authorities, so that
// "the agent works against the fake" means something about real devices.
//
//  1. Recorded traces (testdata/traces): the exact messages five real devices sent
//     (a speaker group, a display hub, a plain speaker, a TV box and a TV dongle),
//     captured read-only with tools/castrecord and scrubbed of addresses and names.
//  2. Google's published Cast message reference
//     (developers.google.com/cast/docs/reference/messages and
//     .../web_receiver/cast.framework.messages): the allowed player states, idle
//     reasons and error message types.
//
// When the fake and the real devices disagree, the fake is wrong.

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Documented vocabulary (Cast reference). Anything else is not a Cast value.
var (
	docPlayerStates = set("IDLE", "PLAYING", "PAUSED", "BUFFERING")
	docIdleReasons  = set("CANCELLED", "INTERRUPTED", "FINISHED", "ERROR")
	docErrorTypes   = set("LOAD_FAILED", "LOAD_CANCELLED", "INVALID_PLAYER_STATE", "INVALID_REQUEST")
	docInvalidWhy   = set("INVALID_COMMAND", "DUPLICATE_REQUESTID")
)

func set(v ...string) map[string]bool {
	m := map[string]bool{}
	for _, s := range v {
		m[s] = true
	}
	return m
}

type recorded struct {
	Dir     string          `json:"dir"`
	NS      string          `json:"ns"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func loadTraces(t *testing.T) map[string][]recorded {
	t.Helper()
	files, _ := filepath.Glob("testdata/traces/*.jsonl")
	if len(files) < 5 {
		t.Fatalf("expected the recorded device traces in testdata/traces, found %d files", len(files))
	}
	out := map[string][]recorded{}
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			var r recorded
			if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
				t.Fatalf("%s: bad fixture line: %v", f, err)
			}
			out[filepath.Base(f)] = append(out[filepath.Base(f)], r)
		}
		fh.Close()
	}
	return out
}

// statuses returns the "status" object of every RECEIVER_STATUS a device sent.
func receiverStatuses(frames []recorded) []map[string]any {
	var out []map[string]any
	for _, f := range frames {
		if f.Dir != "in" || f.Type != "RECEIVER_STATUS" {
			continue
		}
		var p struct {
			Status map[string]any `json:"status"`
		}
		if json.Unmarshal(f.Payload, &p) == nil && p.Status != nil {
			out = append(out, p.Status)
		}
	}
	return out
}

func keysOf(m map[string]any) []string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	sort.Strings(k)
	return k
}

// What every real device does, whatever its kind.
func TestRealDevicesAllFollowTheDocumentedBasics(t *testing.T) {
	for name, frames := range loadTraces(t) {
		sts := receiverStatuses(frames)
		if len(sts) < 10 {
			t.Errorf("%s: only %d receiver statuses recorded", name, len(sts))
		}
		for _, st := range sts {
			vol, ok := st["volume"].(map[string]any)
			if !ok {
				t.Fatalf("%s: a RECEIVER_STATUS without a volume object: %v", name, keysOf(st))
			}
			lvl, lok := vol["level"].(float64)
			_, mok := vol["muted"].(bool)
			if !lok || !mok || lvl < 0 || lvl > 1 {
				t.Errorf("%s: volume = %v, want level in [0,1] and boolean muted", name, vol)
			}
		}
	}
}

// The fake, asked the same question while idle, must answer like the real devices.
func TestFakeIdleReceiverStatusMatchesTheRealDevices(t *testing.T) {
	real := loadTraces(t)

	// Everything the real devices send, by key.
	topKeys, volKeys, appKeys := map[string]bool{}, map[string]bool{}, map[string]bool{}
	noAppsWhenIdle := 0
	for name, frames := range real {
		if !strings.HasPrefix(name, "idle-") {
			continue // the playback trace is a different scenario
		}
		for _, st := range receiverStatuses(frames) {
			for k := range st {
				topKeys[k] = true
			}
			for k := range st["volume"].(map[string]any) {
				volKeys[k] = true
			}
			if apps, ok := st["applications"].([]any); ok {
				for _, a := range apps {
					for k := range a.(map[string]any) {
						appKeys[k] = true
					}
				}
			}
		}
		if name != "idle-hub.jsonl" { // the hub runs its own "Backdrop" app
			if _, has := receiverStatuses(frames)[0]["applications"]; !has {
				noAppsWhenIdle++
			}
		}
	}
	if noAppsWhenIdle != 4 {
		t.Fatalf("fixture check: expected 4 non-hub devices that omit \"applications\" while idle, got %d", noAppsWhenIdle)
	}

	sp := New("Contract")
	st := sp.receiverStatus(1)["status"].(map[string]any)

	// 1. Real idle devices with no app running do not send an "applications" key at all.
	if _, has := st["applications"]; has {
		t.Errorf("the fake sends \"applications\" while idle; every real device without an app omits it")
	}
	// 2. Nothing the fake sends may be unknown to real devices.
	for k := range st {
		if !topKeys[k] {
			t.Errorf("the fake invents a status field %q that no real device sent", k)
		}
	}
	// 3. The volume object has the fields real devices report.
	vol := st["volume"].(map[string]any)
	for _, k := range []string{"level", "muted", "controlType", "stepInterval"} {
		if _, ok := vol[k]; !ok {
			t.Errorf("the fake's volume lacks %q, which every real device reports", k)
		}
	}
	for k := range vol {
		if !volKeys[k] {
			t.Errorf("the fake invents a volume field %q", k)
		}
	}
	// 4. After an app is launched, its entry carries what a real app entry carries.
	sp.appLaunched = true
	st = sp.receiverStatus(2)["status"].(map[string]any)
	apps, _ := st["applications"].([]any)
	if len(apps) != 1 {
		t.Fatalf("a launched app is missing from the status: %v", st)
	}
	app := apps[0].(map[string]any)
	for _, k := range []string{"appId", "displayName", "isIdleScreen", "sessionId", "statusText", "transportId"} {
		if _, ok := app[k]; !ok {
			t.Errorf("the fake's application lacks %q", k)
		}
		if !appKeys[k] {
			t.Errorf("fixture check: no real application entry carried %q", k)
		}
	}
	for k := range app {
		if !appKeys[k] {
			t.Errorf("the fake invents an application field %q", k)
		}
	}
}

// Whatever the fake does, it may only use the documented Cast vocabulary.
func TestFakeOnlySpeaksDocumentedCast(t *testing.T) {
	sp := New("Contract")
	for _, state := range []string{"IDLE", "PLAYING", "PAUSED"} {
		sp.mediaSession, sp.playerState = 1, state
		st := sp.mediaStatus(1)["status"].([]any)[0].(map[string]any)
		if !docPlayerStates[st["playerState"].(string)] {
			t.Errorf("playerState %q is not a Cast player state", st["playerState"])
		}
		if state == "IDLE" {
			// Optional per the reference (absent when IDLE just means "started");
			// when present it must be one of the documented reasons.
			if r, has := st["idleReason"].(string); has && !docIdleReasons[r] {
				t.Errorf("idleReason %q is not documented", r)
			}
		} else if _, has := st["idleReason"]; has {
			t.Errorf("idleReason is only meaningful when IDLE (docs), but sent with %s", state)
		}
		for _, k := range []string{"mediaSessionId", "playerState", "currentTime", "volume"} {
			if _, ok := st[k]; !ok {
				t.Errorf("MEDIA_STATUS lacks the documented field %q", k)
			}
		}
	}
	sp.idleReason = "ERROR"
	sp.playerState = "IDLE"
	if r := sp.mediaStatus(1)["status"].([]any)[0].(map[string]any)["idleReason"]; r != "ERROR" {
		t.Errorf("an error stop reports idleReason %v, want ERROR", r)
	}
	for _, e := range []string{"LOAD_FAILED"} { // the only error message the fake can send today
		if !docErrorTypes[e] {
			t.Errorf("%q is not a documented Cast error type", e)
		}
	}
	_ = docInvalidWhy
}

// ── a real Athan, as the speaker group reported it (athan-playback.jsonl) ──────

type mediaStatusRec struct {
	Dir     string
	Reply   bool // an answer to a request (requestId != 0), not a spontaneous update
	State   string
	Reason  string
	Session float64
	Keys    map[string]bool
}

func realMediaStatuses(t *testing.T) []mediaStatusRec {
	t.Helper()
	var out []mediaStatusRec
	for _, f := range loadTraces(t)["athan-playback.jsonl"] {
		if f.Dir != "in" || f.Type != "MEDIA_STATUS" {
			continue
		}
		var p struct {
			RequestID int              `json:"requestId"`
			Status    []map[string]any `json:"status"`
		}
		if err := json.Unmarshal(f.Payload, &p); err != nil {
			t.Fatal(err)
		}
		for _, s := range p.Status {
			r := mediaStatusRec{Dir: f.Dir, Reply: p.RequestID != 0, Keys: map[string]bool{}}
			r.State, _ = s["playerState"].(string)
			r.Reason, _ = s["idleReason"].(string)
			r.Session, _ = s["mediaSessionId"].(float64)
			for k := range s {
				r.Keys[k] = true
			}
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		t.Fatal("athan-playback.jsonl holds no media statuses")
	}
	return out
}

func TestRealAthanPlaybackFollowsTheDocumentedVocabulary(t *testing.T) {
	for _, s := range realMediaStatuses(t) {
		if !docPlayerStates[s.State] {
			t.Errorf("real device sent playerState %q, which the Cast reference does not list", s.State)
		}
		if s.Reason != "" && !docIdleReasons[s.Reason] {
			t.Errorf("real device sent idleReason %q, which the Cast reference does not list", s.Reason)
		}
		if s.Reason != "" && s.State != "IDLE" {
			t.Errorf("idleReason %q sent with %s; the reference says it applies when IDLE", s.Reason, s.State)
		}
		for _, k := range []string{"mediaSessionId", "playerState", "currentTime", "volume"} {
			if !s.Keys[k] {
				t.Errorf("real %s status lacks %q", s.State, k)
			}
		}
	}
}

// The order the real group reported things in, which the agent's playback check
// must be able to follow: IDLE without a reason is "just started" (the reference
// says idleReason is absent then) and is NOT a failure; BUFFERING and PLAYING
// follow; the end is IDLE with FINISHED.
func TestRealAthanPlaybackSequence(t *testing.T) {
	var seq []string
	for _, s := range realMediaStatuses(t) {
		step := s.State
		if s.Reason != "" {
			step += "/" + s.Reason
		}
		if len(seq) == 0 || seq[len(seq)-1] != step {
			seq = append(seq, step)
		}
	}
	want := []string{"IDLE", "BUFFERING", "PLAYING", "IDLE/FINISHED"}
	// PLAYING may interleave with repeats of IDLE/BUFFERING replies; compare as an ordered subsequence.
	i := 0
	for _, step := range seq {
		if i < len(want) && step == want[i] {
			i++
		}
	}
	if i != len(want) {
		t.Fatalf("real sequence %v does not contain %v in order", seq, want)
	}
	first := realMediaStatuses(t)[0]
	if first.State != "IDLE" || first.Reason != "" {
		t.Errorf("the first status for a new Athan was %s/%q, want IDLE with no idleReason", first.State, first.Reason)
	}
}
