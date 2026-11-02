// Package trace is how the rest of the agent reports what it is doing to the
// Journal without depending on it. Anything can call Emit; with no sink set (the
// default, and in most tests) it does nothing. Emit never panics and never blocks
// for long, because tracing must not be able to disturb a playback.
package trace

import (
	"sync/atomic"
	"time"
)

// Where an event comes from.
const (
	SourceAgent  = "agent"  // something the agent decided or did
	SourceTV     = "tv"     // the TV-mute feature
	SourceMedia  = "media"  // the audio file being fetched by a speaker
	SourceSystem = "system" // the agent itself (start, stop, schedule)
	SourceHealth = "health" // a device's own status page
	SourceDevice = "device" // what a watched device reported (set by the Journal's watcher)
)

// Event is one thing that happened.
type Event struct {
	At       time.Time      `json:"at"`
	Source   string         `json:"source"`
	Kind     string         `json:"kind"`
	Device   string         `json:"device,omitempty"`
	Text     string         `json:"text"`
	Level    string         `json:"level,omitempty"`    // "", "warn" or "error"
	External bool           `json:"external,omitempty"` // a device changed and the agent did not cause it
	Fields   map[string]any `json:"fields,omitempty"`
}

// Sink receives events. Implementations must be quick and safe for concurrent use.
type Sink interface{ Event(Event) }

type box struct{ s Sink }

var current atomic.Pointer[box]

// SetSink installs the receiver of all events; nil removes it.
func SetSink(s Sink) {
	if s == nil {
		current.Store(nil)
		return
	}
	current.Store(&box{s})
}

// Emit delivers e to the sink, stamping the time if it is unset.
func Emit(e Event) {
	b := current.Load()
	if b == nil {
		return
	}
	if e.At.IsZero() {
		e.At = time.Now()
	}
	defer func() { _ = recover() }()
	b.s.Event(e)
}

// Agent emits an agent event. kv is alternating key, value pairs.
func Agent(kind, device, text string, kv ...any) {
	emit(SourceAgent, "", kind, device, text, kv)
}

// Warn emits an agent event that deserves attention.
func Warn(source, kind, device, text string, kv ...any) {
	emit(source, "warn", kind, device, text, kv)
}

// Error emits an event that records a failure.
func Error(source, kind, device, text string, kv ...any) {
	emit(source, "error", kind, device, text, kv)
}

// From emits an event from the given source.
func From(source, kind, device, text string, kv ...any) {
	emit(source, "", kind, device, text, kv)
}

func emit(source, level, kind, device, text string, kv []any) {
	if current.Load() == nil {
		return
	}
	var fields map[string]any
	for i := 0; i+1 < len(kv); i += 2 {
		k, ok := kv[i].(string)
		if !ok {
			continue
		}
		if fields == nil {
			fields = map[string]any{}
		}
		fields[k] = kv[i+1]
	}
	Emit(Event{Source: source, Level: level, Kind: kind, Device: device, Text: text, Fields: fields})
}
