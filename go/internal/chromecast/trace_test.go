package chromecast

import (
	"strings"
	"sync"
	"testing"
	"time"

	"azan-agent/internal/trace"
)

// capture is a trace sink for tests.
type capture struct {
	mu  sync.Mutex
	evs []trace.Event
}

func (c *capture) Event(e trace.Event) { c.mu.Lock(); c.evs = append(c.evs, e); c.mu.Unlock() }

func (c *capture) kinds() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var k []string
	for _, e := range c.evs {
		k = append(k, e.Kind)
	}
	return strings.Join(k, " ")
}

func (c *capture) first(kind string) (trace.Event, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.evs {
		if e.Kind == kind {
			return e, true
		}
	}
	return trace.Event{}, false
}

func (c *capture) count(kind string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, e := range c.evs {
		if e.Kind == kind {
			n++
		}
	}
	return n
}

func sinkFor(t *testing.T) *capture {
	t.Helper()
	c := &capture{}
	trace.SetSink(c)
	t.Cleanup(func() { trace.SetSink(nil) })
	return c
}

func TestPlayingTheAthanTellsTheJournalWhatTheAgentDid(t *testing.T) {
	r := newTVRig(t)
	c := sinkFor(t)
	host, _ := r.athan.Addr()

	if err := r.m.PreConnect(); err != nil {
		t.Fatal(err)
	}
	if err := r.m.PlayAthan("Dhuhr", ""); err != nil {
		t.Fatal(err)
	}

	if _, ok := c.first("pre-connect"); !ok {
		t.Errorf("no pre-connect event: %s", c.kinds())
	}
	load, ok := c.first("load-sent")
	if !ok {
		t.Fatalf("no load-sent event: %s", c.kinds())
	}
	if load.Device != "Test Speaker" || load.Fields["host"] != host || !strings.Contains(load.Text, "media_Athan.mp3") {
		t.Errorf("load-sent = %+v", load)
	}
	v, ok := c.first("verified")
	if !ok {
		t.Fatalf("no verified event: %s", c.kinds())
	}
	if v.Fields["state"] != "PLAYING" || v.Fields["muted"] != false || v.Level != "" {
		t.Errorf("verified = %+v", v)
	}
	if c.count("play-failed") != 0 {
		t.Error("a successful play reported a failure")
	}
}

func TestStoppingPlaybackIsReportedWithTheSpeakersAddress(t *testing.T) {
	r := newTVRig(t)
	if err := r.m.PlayAthan("Dhuhr", ""); err != nil {
		t.Fatal(err)
	}
	c := sinkFor(t)
	host, _ := r.athan.Addr()
	if err := r.m.StopPlayback(); err != nil {
		t.Fatal(err)
	}
	e, ok := c.first("stop-sent")
	if !ok || e.Fields["host"] != host {
		t.Fatalf("stop-sent = %+v (%s)", e, c.kinds())
	}
}

func TestAPlayThatFailsIsReportedWithItsReasonAndItsRetries(t *testing.T) {
	r := newTVRig(t)
	c := sinkFor(t)
	r.athan.FailNextLoads(20)

	if err := r.m.PlayAthan("Dhuhr", ""); err == nil {
		t.Fatal("expected the play to fail")
	}
	e, ok := c.first("play-failed")
	if !ok {
		t.Fatalf("no play-failed event: %s", c.kinds())
	}
	if e.Level != "error" || e.Fields["error"] == nil || !strings.Contains(e.Fields["error"].(string), "did not start playing") {
		t.Errorf("play-failed = %+v", e)
	}
	if c.count("load-sent") < 2 || c.count("retry") < 1 {
		t.Errorf("the retries are not on the timeline: %s", c.kinds())
	}
}

func TestMutingATVIsOnTheTimelineWithTheAddressOfTheTV(t *testing.T) {
	r := newTVRig(t, "Living Room TV")
	c := sinkFor(t)
	tvHost, _ := r.tvs["Living Room TV"].Addr()

	r.tvp.PauseForAthan(nil, "Test Speaker")
	held, ok := c.first("tv-held")
	if !ok || held.Source != trace.SourceTV || held.Device != "Living Room TV" || !strings.Contains(held.Text, "muted") {
		t.Fatalf("tv-held = %+v (%s)", held, c.kinds())
	}
	mute, ok := c.first("mute-sent")
	if !ok || mute.Fields["host"] != tvHost {
		t.Errorf("mute-sent = %+v", mute)
	}

	r.tvp.ResumeAfterAthan()
	eventually(t, "TV released", func() bool { _, ok := c.first("tv-released"); return ok })
	if _, ok := c.first("volume-set"); !ok {
		t.Errorf("the release nudge is not on the timeline: %s", c.kinds())
	}
}

func TestATVTheOwnerMutedProducesNoMuteEvents(t *testing.T) {
	r := newTVRig(t, "Living Room TV")
	r.tvs["Living Room TV"].SetVolumeState(0.4, true)
	c := sinkFor(t)
	r.tvp.PauseForAthan(nil, "Test Speaker")
	if c.count("mute-sent") != 0 || c.count("tv-held") != 0 {
		t.Errorf("the agent claims to have changed a TV it left alone: %s", c.kinds())
	}
	if c.count("tv-skipped") != 1 {
		t.Errorf("the decision to leave it alone is not recorded: %s", c.kinds())
	}
	_ = time.Second
}
