package quran

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeCastPlayer implements castPlayer without any real network or
// Chromecast I/O, so tests can drive Controller's state machine
// deterministically.
type fakeCastPlayer struct {
	playURLFunc func(url, contentType string) error

	mu        sync.Mutex
	stopCalls int
}

var _ castPlayer = (*fakeCastPlayer)(nil)

func (f *fakeCastPlayer) PlayURL(url, contentType string) error {
	if f.playURLFunc != nil {
		return f.playURLFunc(url, contentType)
	}
	return nil
}

func (f *fakeCastPlayer) PlayURLOnDevice(deviceName, url, contentType string) error {
	return f.PlayURL(url, contentType)
}

func (f *fakeCastPlayer) StopPlayback() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopCalls++
	return nil
}

func (f *fakeCastPlayer) StopCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stopCalls
}

// waitFor polls cond until it returns true or the timeout elapses, failing
// the test in the latter case. Used only to observe the async cleanup a
// streaming goroutine performs after a context is cancelled.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("condition not met within %v", timeout)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestSuperseded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if superseded(ctx) {
		t.Fatal("superseded(ctx) = true before cancel, want false")
	}

	cancel()

	if !superseded(ctx) {
		t.Fatal("superseded(ctx) = false after cancel, want true")
	}
}

func TestSupersededAfterTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	<-ctx.Done()

	if !superseded(ctx) {
		t.Fatal("superseded(ctx) = false after deadline expired, want true")
	}
}

func TestStatusReportsCurrentFlags(t *testing.T) {
	c := &Controller{castMgr: &fakeCastPlayer{}}

	if sa, la := c.Status(); sa || la {
		t.Fatalf("Status() = (%v, %v), want (false, false) for a fresh controller", sa, la)
	}

	c.speakerActive = true
	if sa, la := c.Status(); !sa || la {
		t.Fatalf("Status() = (%v, %v), want (true, false)", sa, la)
	}

	c.localActive = true
	if sa, la := c.Status(); !sa || !la {
		t.Fatalf("Status() = (%v, %v), want (true, true)", sa, la)
	}

	c.speakerActive = false
	if sa, la := c.Status(); sa || !la {
		t.Fatalf("Status() = (%v, %v), want (false, true)", sa, la)
	}
}

// TestClearSpeakerIfGenIgnoresStaleGeneration verifies the core invariant
// behind speaker generation counting: a goroutine belonging to an older
// StartSpeaker call must not clear state that a newer call already set up.
func TestClearSpeakerIfGenIgnoresStaleGeneration(t *testing.T) {
	c := &Controller{castMgr: &fakeCastPlayer{}}

	c.speakerGen = 1
	c.speakerActive = true
	_, newerCancel := context.WithCancel(context.Background())
	c.speakerCancel = newerCancel
	c.speakerGen = 2 // a second StartSpeaker call bumped the generation

	// The stale (gen-1) goroutine finishes and runs its deferred cleanup.
	c.clearSpeakerIfGen(1)

	if !c.speakerActive {
		t.Error("clearSpeakerIfGen(stale gen) cleared speakerActive; the newer stream's state must survive")
	}
	if c.speakerCancel == nil {
		t.Error("clearSpeakerIfGen(stale gen) cleared speakerCancel; the newer stream's cancel func must survive")
	}
}

func TestClearSpeakerIfGenClearsCurrentGeneration(t *testing.T) {
	c := &Controller{castMgr: &fakeCastPlayer{}}

	c.speakerGen = 3
	c.speakerActive = true
	_, cancel := context.WithCancel(context.Background())
	c.speakerCancel = cancel

	c.clearSpeakerIfGen(3)

	if c.speakerActive {
		t.Error("clearSpeakerIfGen(current gen) left speakerActive true, want false")
	}
	if c.speakerCancel != nil {
		t.Error("clearSpeakerIfGen(current gen) left speakerCancel non-nil, want nil")
	}
}

func TestStopSpeakerCancelsContextAndCallsStopPlayback(t *testing.T) {
	fake := &fakeCastPlayer{}
	c := &Controller{castMgr: fake}

	c.speakerGen = 5
	ctx, cancel := context.WithCancel(context.Background())
	c.speakerCancel = cancel
	c.speakerActive = true

	c.StopSpeaker()

	if ctx.Err() == nil {
		t.Error("StopSpeaker() did not cancel the speaker context")
	}
	if c.speakerCancel != nil {
		t.Error("StopSpeaker() left speakerCancel non-nil, want nil")
	}
	if got := fake.StopCalls(); got != 1 {
		t.Errorf("StopSpeaker() called StopPlayback %d times, want 1", got)
	}

	// The streaming goroutine notices ctx.Done() and runs its deferred cleanup.
	c.clearSpeakerIfGen(5)

	if sa, _ := c.Status(); sa {
		t.Error("speakerActive still true after StopSpeaker + goroutine cleanup, want false")
	}
}

func TestStopLocalCancelsContext(t *testing.T) {
	c := &Controller{castMgr: &fakeCastPlayer{}}

	ctx, cancel := context.WithCancel(context.Background())
	c.localCancel = cancel
	c.localActive = true

	c.StopLocal()

	if ctx.Err() == nil {
		t.Fatal("StopLocal() did not cancel the local context")
	}

	// clearLocal is invoked by the streaming goroutine's defer once it
	// observes ctx.Done() — StopLocal itself does not reset the flags.
	c.clearLocal()

	if _, la := c.Status(); la {
		t.Error("localActive still true after StopLocal + goroutine cleanup, want false")
	}
	if c.localCancel != nil {
		t.Error("localCancel not nil after clearLocal, want nil")
	}
}

func TestStopCancelsBothStreamsAndCallsStopPlayback(t *testing.T) {
	fake := &fakeCastPlayer{}
	c := &Controller{castMgr: fake}

	c.speakerGen = 7
	speakerCtx, speakerCancel := context.WithCancel(context.Background())
	c.speakerCancel = speakerCancel
	c.speakerActive = true

	localCtx, localCancel := context.WithCancel(context.Background())
	c.localCancel = localCancel
	c.localActive = true

	c.Stop()

	if speakerCtx.Err() == nil {
		t.Error("Stop() did not cancel the speaker context")
	}
	if localCtx.Err() == nil {
		t.Error("Stop() did not cancel the local context")
	}
	if c.speakerCancel != nil {
		t.Error("Stop() left speakerCancel non-nil, want nil")
	}
	if c.localCancel != nil {
		t.Error("Stop() left localCancel non-nil, want nil")
	}
	if got := fake.StopCalls(); got != 1 {
		t.Errorf("Stop() called StopPlayback %d times, want 1", got)
	}

	// Simulate both streaming goroutines noticing cancellation and clearing state.
	c.clearSpeakerIfGen(7)
	c.clearLocal()

	if sa, la := c.Status(); sa || la {
		t.Errorf("Status() = (%v, %v) after full cleanup, want (false, false)", sa, la)
	}
}

// TestStartSpeakerSupersedesInFlightCall drives the real StartSpeaker method
// twice, using a fake castPlayer to prove that a second call supersedes an
// older in-flight one: the stale goroutine's deferred cleanup must not clear
// the state the newer call set up, and only the newer stream ends up active
// once both goroutines settle.
func TestStartSpeakerSupersedesInFlightCall(t *testing.T) {
	started := make(chan struct{}, 2)
	proceed := make(chan struct{})
	fake := &fakeCastPlayer{
		playURLFunc: func(url, contentType string) error {
			started <- struct{}{}
			<-proceed
			return nil
		},
	}
	c := &Controller{castMgr: fake}

	if err := c.StartSpeaker(0); err != nil {
		t.Fatalf("first StartSpeaker: %v", err)
	}
	<-started // first goroutine is now blocked inside PlayURL (in-flight)

	firstGen := c.speakerGen

	if err := c.StartSpeaker(0); err != nil {
		t.Fatalf("second StartSpeaker: %v", err)
	}

	if c.speakerGen == firstGen {
		t.Fatalf("second StartSpeaker did not bump speakerGen (still %d)", c.speakerGen)
	}
	secondGen := c.speakerGen

	close(proceed) // release both goroutines' blocked PlayURL calls

	// Give the stale (first) goroutine time to run its deferred cleanup and
	// confirm it left the newer generation's state untouched.
	waitFor(t, time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.speakerGen == secondGen
	})
	time.Sleep(20 * time.Millisecond)

	if sa, _ := c.Status(); !sa {
		t.Fatal("speakerActive is false after stale goroutine cleanup; the newer (second) stream should still be active")
	}
	c.mu.Lock()
	staleCancelCleared := c.speakerCancel == nil
	c.mu.Unlock()
	if staleCancelCleared {
		t.Fatal("speakerCancel was cleared by the stale goroutine; the newer stream's cancel func should survive")
	}

	// Now stop the (second) stream and confirm it fully clears.
	c.StopSpeaker()
	waitFor(t, time.Second, func() bool {
		sa, _ := c.Status()
		return !sa
	})
}
