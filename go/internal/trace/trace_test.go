package trace

import (
	"sync"
	"testing"
	"time"
)

type recorder struct {
	mu  sync.Mutex
	evs []Event
}

func (r *recorder) Event(e Event) { r.mu.Lock(); r.evs = append(r.evs, e); r.mu.Unlock() }

func TestEmitWithoutASinkIsHarmless(t *testing.T) {
	SetSink(nil)
	Emit(Event{Kind: "x"}) // must not panic
	Agent("load-sent", "athan", "sent")
}

func TestEmitDeliversAndStampsTheTime(t *testing.T) {
	r := &recorder{}
	SetSink(r)
	t.Cleanup(func() { SetSink(nil) })

	before := time.Now()
	Agent("load-sent", "athan", "Load sent", "host", "10.0.0.5", "attempt", 2)
	if len(r.evs) != 1 {
		t.Fatalf("got %d events", len(r.evs))
	}
	e := r.evs[0]
	if e.Source != SourceAgent || e.Kind != "load-sent" || e.Device != "athan" || e.Text != "Load sent" {
		t.Errorf("unexpected event: %+v", e)
	}
	if e.At.Before(before) || e.At.After(time.Now()) {
		t.Errorf("event time %v not stamped", e.At)
	}
	if e.Fields["host"] != "10.0.0.5" || e.Fields["attempt"] != 2 {
		t.Errorf("fields = %v", e.Fields)
	}
}

func TestASinkThatPanicsCannotBreakTheCaller(t *testing.T) {
	SetSink(panicSink{})
	t.Cleanup(func() { SetSink(nil) })
	Agent("x", "", "boom") // must return normally: tracing may never disturb playback
}

type panicSink struct{}

func (panicSink) Event(Event) { panic("sink bug") }

func TestOddKeyValueArgumentsAreIgnoredNotFatal(t *testing.T) {
	r := &recorder{}
	SetSink(r)
	t.Cleanup(func() { SetSink(nil) })
	Agent("x", "", "t", "dangling")
	if len(r.evs) != 1 {
		t.Fatal("event lost")
	}
}
