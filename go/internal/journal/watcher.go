package journal

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/vishen/go-chromecast/application"
	"github.com/vishen/go-chromecast/cast"

	"azan-agent/internal/castrecord"
	"azan-agent/internal/trace"
)

var (
	// watcherRetry is the pause before a device that stopped answering is tried again.
	watcherRetry = 2 * time.Second
	// watcherQuickRetry is the pause after one of the first failures to connect:
	// the Cast client library occasionally loses the very first reply, and trying
	// again at once is the cure.
	watcherQuickRetry = 300 * time.Millisecond
)

// failuresBeforeUnreachable is how many connection attempts in a row may fail,
// before the first contact, until the device is reported as unreachable.
const failuresBeforeUnreachable = 3

const (
	defaultCap      = 15 * time.Minute
	defaultQuranCap = 2 * time.Hour
	connectTimeout  = 3 * time.Second
	pollTimeout     = 4 * time.Second
)

// startWatchers begins watching every device of the session, and the monitor
// that decides when the session is over.
func (r *Run) startWatchers() {
	for _, t := range r.targets {
		t := t
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			r.watch(t)
		}()
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.monitor()
	}()
}

// watch follows one device until the session stops, reconnecting after drops.
// It only ever asks for status (see internal/castrecord), so it cannot disturb
// what the device is playing.
func (r *Run) watch(t Target) {
	tr := newTracker()
	down := false
	fails := 0
	everContact := false
	var lastErr error
	// A device that never answered before the session ended must still be reported.
	defer func() {
		if !everContact && !down && lastErr != nil {
			r.mu.Lock()
			r.updateLive(t, "unreachable", nil)
			r.mu.Unlock()
			r.add(trace.Event{At: time.Now(), Source: trace.SourceDevice, Kind: "unreachable", Device: t.Name, Level: "warn",
				Text:   "never made contact with the device: " + lastErr.Error(),
				Fields: map[string]any{"role": t.Role, "host": t.Host}})
		}
	}()
	for {
		contact := false
		err := r.watchOnce(t, tr, &down, &contact)
		everContact = everContact || contact
		if err != nil {
			lastErr = err
		}
		select {
		case <-r.stop:
			return
		default:
		}
		if err != nil {
			fails++
			if contact {
				fails = 1 // it had been working; count from the loss
			}
			if !down && (contact || fails >= failuresBeforeUnreachable) {
				down = true
				r.mu.Lock()
				r.updateLive(t, "unreachable", nil)
				r.mu.Unlock()
				r.add(trace.Event{At: time.Now(), Source: trace.SourceDevice, Kind: "unreachable", Device: t.Name, Level: "warn",
					Text:   "lost contact with the device: " + err.Error(),
					Fields: map[string]any{"role": t.Role, "host": t.Host}})
			}
		} else {
			fails = 0
		}
		wait := watcherRetry
		if !down && fails < failuresBeforeUnreachable {
			wait = watcherQuickRetry
		}
		select {
		case <-r.stop:
			return
		case <-time.After(wait):
		}
	}
}

func (r *Run) watchOnce(t Target, tr *tracker, down, contact *bool) error {
	rec := castrecord.New(cast.NewConnection())
	app := application.NewApplication(application.WithConnection(rec), application.WithCacheDisabled(true))
	if err := r.startWithin(app, t); err != nil {
		return err
	}
	defer app.Close(false)
	*contact = true
	if *down {
		*down = false
		r.mu.Lock()
		r.updateLive(t, "reconnected", nil)
		r.mu.Unlock()
		r.add(trace.Event{At: time.Now(), Source: trace.SourceDevice, Kind: "reconnected", Device: t.Name,
			Text: "contact with the device restored", Fields: map[string]any{"role": t.Role, "host": t.Host}})
	}

	sent := 0
	drain := func() {
		frames := rec.FramesSince(sent)
		sent += len(frames)
		for _, f := range frames {
			if line, err := json.Marshal(f); err == nil {
				r.writeRaw(t, string(line))
			}
			for _, c := range tr.Feed(f) {
				r.deviceChange(t, c)
			}
		}
	}
	tick := time.NewTicker(r.j.o.PollEvery)
	defer tick.Stop()
	for {
		drain()
		select {
		case <-r.stop:
			return nil
		case <-tick.C:
		}
		if err := updateWithin(app, pollTimeout); err != nil {
			drain()
			return err
		}
	}
}

// startWithin connects, giving up after connectTimeout or when the session stops.
func (r *Run) startWithin(app *application.Application, t Target) error {
	done := make(chan error, 1)
	go func() { done <- app.Start(t.Host, t.Port) }()
	abandon := func() {
		go func() {
			if <-done == nil {
				app.Close(false)
			}
		}()
	}
	select {
	case err := <-done:
		return err
	case <-time.After(connectTimeout):
		abandon()
		return fmt.Errorf("no answer from %s:%d within %v", t.Host, t.Port, connectTimeout)
	case <-r.stop:
		abandon()
		return fmt.Errorf("stopped")
	}
}

// updateWithin is app.Update() with a time limit (the library otherwise retries
// a dead connection for ~10 s). On timeout the connection is closed.
func updateWithin(app *application.Application, limit time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- app.Update() }()
	select {
	case err := <-done:
		return err
	case <-time.After(limit):
		app.Close(false)
		return fmt.Errorf("no answer within %v", limit)
	}
}

// deviceChange records a change a device reported, marking it as outside
// interference when the agent did not cause it while it was playing or holding
// the device.
func (r *Run) deviceChange(t Target, c change) {
	ev := c.event(t.Name, t.Role)
	ev.Fields["host"] = t.Host

	r.mu.Lock()
	external := false
	// The agent's commands go to the speaker (the group); a member is a device
	// of its own, possibly at another address, that only reacts to them.
	actionHost := t.Host
	if t.Role == "member" {
		for _, x := range r.targets {
			if x.Role == "speaker" {
				actionHost = x.Host
				break
			}
		}
	}
	if c.Kind != "initial-state" && !r.attr.Caused(actionHost, c) {
		switch t.Role {
		case "speaker", "member":
			external = !r.loadAt.IsZero() && (r.playEndAt.IsZero() || !c.At.After(r.playEndAt))
		case "tv":
			external = r.tvHeld || (!r.tvReleasedAt.IsZero() && !c.At.After(r.tvReleasedAt.Add(3*time.Second)))
		}
	}
	r.updateLive(t, c.Kind, c.Fields)
	if t.Role == "speaker" {
		state, _ := c.Fields["state"].(string)
		switch {
		case c.Kind == "player-state" && (state == "PLAYING" || state == "BUFFERING") && !r.loadAt.IsZero() && !c.At.Before(r.loadAt):
			r.playing = true
		case r.playing && r.playEndAt.IsZero() && ((c.Kind == "player-state" && state == "IDLE") || c.Kind == "app-closed"):
			r.playEndAt = c.At
		}
	}
	r.mu.Unlock()

	if external {
		ev.External, ev.Level = true, "warn"
		ev.Text += " — not caused by the agent"
	}
	r.add(ev)
}

// monitor ends the session by itself: after the playback is over (plus the tail),
// when nothing started in time, or at the time limit.
func (r *Run) monitor() {
	hasSpeaker := false
	for _, t := range r.targets {
		if t.Role == "speaker" {
			hasSpeaker = true
		}
	}
	limit := r.spec.MaxDuration
	if limit <= 0 {
		limit = defaultCap
		if r.spec.Kind == "quran" {
			limit = defaultQuranCap
		}
	}
	began := time.Now()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-tick.C:
		}
		if time.Since(began) > limit {
			go r.End("time limit")
			return
		}
		if !r.spec.ExpectPlayback || !hasSpeaker {
			continue
		}
		r.mu.Lock()
		evs := append([]trace.Event(nil), r.events...)
		loadAt := r.loadAt
		r.mu.Unlock()
		res := Analyze(r.spec, evs, nil, false)
		switch {
		case res.Facts.Started && !res.Facts.PlayedTo.IsZero() && time.Since(res.Facts.PlayedTo) >= r.j.o.Tail:
			go r.End("playback ended")
			return
		case !res.Facts.Started && !loadAt.IsZero() && r.j.o.Now().Sub(loadAt) > r.j.o.StartTimeout:
			go r.End("nothing started playing")
			return
		case !res.Facts.Started && loadAt.IsZero() && r.noRequestDeadline().Before(r.j.o.Now()):
			go r.End("the agent never asked the speaker to play")
			return
		}
	}
}

// noRequestDeadline is when a session in which the agent never asked the speaker
// to play gives up: the scheduled time (or the start) plus StartTimeout.
func (r *Run) noRequestDeadline() time.Time {
	base := r.started
	if r.spec.ScheduledFor.After(base) {
		base = r.spec.ScheduledFor
	}
	return base.Add(r.j.o.StartTimeout)
}
