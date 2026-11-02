package journal

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"azan-agent/internal/castrecord"
	"azan-agent/internal/trace"
)

func loadTrace(t *testing.T, name string) []castrecord.Frame {
	t.Helper()
	f, err := os.Open("../fakecast/testdata/traces/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var frames []castrecord.Frame
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var fr castrecord.Frame
		if err := json.Unmarshal(sc.Bytes(), &fr); err != nil {
			t.Fatal(err)
		}
		frames = append(frames, fr)
	}
	return frames
}

func feedAll(tr *tracker, frames []castrecord.Frame) []change {
	var out []change
	for _, f := range frames {
		out = append(out, tr.Feed(f)...)
	}
	return out
}

func kinds(cs []change) string {
	var k []string
	for _, c := range cs {
		s := c.Kind
		if st, ok := c.Fields["state"].(string); ok {
			s += ":" + st
			if r, _ := c.Fields["idle_reason"].(string); r != "" {
				s += "/" + r
			}
		}
		k = append(k, s)
	}
	return strings.Join(k, " ")
}

func TestARealAthanTraceBecomesTheExpectedChanges(t *testing.T) {
	// testdata/traces/athan-playback: a real scheduled Isha on the real group.
	got := kinds(feedAll(newTracker(), loadTrace(t, "athan-playback.jsonl")))
	want := "initial-state app-launched player-state:IDLE player-state:BUFFERING player-state:PLAYING player-state:IDLE/FINISHED"
	if got != want {
		t.Errorf("changes:\n got  %s\n want %s", got, want)
	}
}

func TestAnIdleDeviceProducesOnlyTheInitialState(t *testing.T) {
	for _, name := range []string{"idle-group.jsonl", "idle-hub.jsonl", "idle-speaker.jsonl", "idle-tvbox.jsonl", "idle-tvdongle.jsonl"} {
		got := kinds(feedAll(newTracker(), loadTrace(t, name)))
		if got != "initial-state" {
			t.Errorf("%s: %s", name, got)
		}
	}
}

func TestARealAthanTraceIsJudgedOK(t *testing.T) {
	frames := loadTrace(t, "athan-playback.jsonl")
	tr := newTracker()
	var evs []trace.Event
	for _, c := range feedAll(tr, frames) {
		evs = append(evs, c.event("athan", "speaker"))
	}
	evs = append(evs, ag(0, "load-sent"))
	// the agent's request came just before the speaker reacted
	for i := range evs {
		if evs[i].Source == "agent" {
			evs[i].At = frames[6].At.Add(-2 * 1e9)
		}
	}
	r := Analyze(athanSpec, evs, nil, true)
	if r.Verdict != VerdictOK || !strings.Contains(r.Headline, "finished normally") {
		t.Fatalf("%s: %s", r.Verdict, r.Headline)
	}
}

func TestVolumeAndMuteChangesAreReported(t *testing.T) {
	tr := newTracker()
	recv := func(level float64, muted bool) castrecord.Frame {
		p, _ := json.Marshal(map[string]any{"type": "RECEIVER_STATUS", "status": map[string]any{
			"volume": map[string]any{"level": level, "muted": muted, "controlType": "attenuation"}}})
		return castrecord.Frame{Dir: castrecord.In, Type: "RECEIVER_STATUS", Payload: p}
	}
	got := feedAll(tr, []castrecord.Frame{recv(0.13, false), recv(0.13, false), recv(0.13, true), recv(0.2, true), recv(0.2, false)})
	if kinds(got) != "initial-state volume volume volume" {
		t.Fatalf("changes: %s", kinds(got))
	}
	if got[1].Fields["muted"] != true || got[2].Fields["level"] != 0.2 {
		t.Errorf("fields wrong: %+v / %+v", got[1].Fields, got[2].Fields)
	}
}

func TestAnAppThatClosesIsReportedAndTheStaleMediaStateIsForgotten(t *testing.T) {
	tr := newTracker()
	app := func(withApp bool) castrecord.Frame {
		st := map[string]any{"volume": map[string]any{"level": 0.5, "muted": false}}
		if withApp {
			st["applications"] = []any{map[string]any{"appId": "CC1AD845", "displayName": "Default Media Receiver", "transportId": "x"}}
		}
		p, _ := json.Marshal(map[string]any{"type": "RECEIVER_STATUS", "status": st})
		return castrecord.Frame{Dir: castrecord.In, Type: "RECEIVER_STATUS", Payload: p}
	}
	media := func(state string) castrecord.Frame {
		p, _ := json.Marshal(map[string]any{"type": "MEDIA_STATUS", "status": []any{map[string]any{"mediaSessionId": 1, "playerState": state}}})
		return castrecord.Frame{Dir: castrecord.In, Type: "MEDIA_STATUS", Payload: p}
	}
	got := feedAll(tr, []castrecord.Frame{app(true), media("PLAYING"), app(false), app(true), media("PLAYING")})
	// the second PLAYING is a NEW app instance's report, not a repeat of the old one
	if kinds(got) != "initial-state player-state:PLAYING app-closed app-launched player-state:PLAYING" {
		t.Errorf("changes: %s", kinds(got))
	}
}

func TestFramesWeSentAreIgnored(t *testing.T) {
	tr := newTracker()
	if cs := tr.Feed(castrecord.Frame{Dir: castrecord.Out, Type: "GET_STATUS"}); len(cs) != 0 {
		t.Errorf("outbound frame produced %v", cs)
	}
}
