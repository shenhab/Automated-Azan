package chromecast

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"azan-agent/internal/fakecast"
)

// tvRig is the Athan speaker plus any number of TVs, all found through
// AZAN_STATIC_DEVICES.
type tvRig struct {
	m     *Manager
	tvp   *TVPauseManager
	athan *fakecast.Speaker
	tvs   map[string]*fakecast.Speaker
}

func newTVRig(t *testing.T, tvNames ...string) *tvRig {
	t.Helper()
	oldMDNS, oldPause := mdnsTimeout, connectRetryPause
	mdnsTimeout, connectRetryPause = 300*time.Millisecond, 100*time.Millisecond
	t.Cleanup(func() { mdnsTimeout, connectRetryPause = oldMDNS, oldPause })

	mux := http.NewServeMux()
	mux.HandleFunc("/media/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write(append([]byte("ID3"), make([]byte, 4096)...))
	})
	media := httptest.NewServer(mux)
	t.Cleanup(media.Close)

	r := &tvRig{athan: fakecast.New("Test Speaker"), tvs: map[string]*fakecast.Speaker{}}
	var spec []string
	for _, sp := range append([]*fakecast.Speaker{r.athan}, func() []*fakecast.Speaker {
		var out []*fakecast.Speaker
		for _, n := range tvNames {
			tv := fakecast.New(n)
			r.tvs[n] = tv
			out = append(out, tv)
		}
		return out
	}()...) {
		if err := sp.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(sp.Close)
		h, p := sp.Addr()
		spec = append(spec, fmt.Sprintf("%s=%s:%d", sp.Name, h, p))
	}
	t.Setenv("AZAN_DISABLE_MDNS", "1") // tests must never reach real devices on the LAN
	t.Setenv("AZAN_STATIC_DEVICES", strings.Join(spec, ";"))
	r.m = NewManager("Test Speaker")
	r.m.SetMediaBaseURL(media.URL + "/media/")
	r.tvp = NewTVPauseManager(r.m)
	return r
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for: %s", what)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func muted(sp *fakecast.Speaker) bool { _, m := sp.Volume(); return m }

func TestIdleTVIsMutedForTheAthanThenUnmuted(t *testing.T) {
	r := newTVRig(t, "Living Room TV")
	tv := r.tvs["Living Room TV"]

	r.tvp.PauseForAthan(nil, "Test Speaker")
	eventually(t, "TV muted", func() bool { return muted(tv) })
	if r.tvp.PausedCount() != 1 {
		t.Errorf("paused count = %d, want 1", r.tvp.PausedCount())
	}
	if len(r.athan.EventsOfKind("mute")) != 0 {
		t.Error("the Athan speaker itself was muted")
	}

	r.tvp.ResumeAfterAthan()
	eventually(t, "TV unmuted", func() bool { return !muted(tv) })
	if r.tvp.IsPaused() {
		t.Error("still reports paused after resume")
	}
}

func TestTVThatIsPlayingIsPausedThenResumed(t *testing.T) {
	r := newTVRig(t, "Living Room TV")
	tv := r.tvs["Living Room TV"]
	tv.SimulateCast("http://example/show.mp4")

	r.tvp.PauseForAthan(nil, "Test Speaker")
	eventually(t, "TV paused", func() bool { return tv.PlayerState() == "PAUSED" })
	if muted(tv) {
		t.Error("a TV that was playing should be paused, not muted")
	}

	r.tvp.ResumeAfterAthan()
	eventually(t, "TV playing again", func() bool { return tv.PlayerState() == "PLAYING" })
}

// The owner had muted the TV (or paused the show) themselves: the Athan must not
// change that, and afterwards must not undo it.
func TestWhatTheOwnerAlreadyMutedStaysMuted(t *testing.T) {
	r := newTVRig(t, "Living Room TV")
	tv := r.tvs["Living Room TV"]
	tv.SetVolumeState(0.4, true)

	r.tvp.PauseForAthan(nil, "Test Speaker")
	r.tvp.ResumeAfterAthan()
	time.Sleep(500 * time.Millisecond)
	if !muted(tv) {
		t.Error("the TV the owner had muted was unmuted by the Athan flow")
	}
	if lvl, _ := tv.Volume(); lvl != 0.4 {
		t.Errorf("volume changed to %v, want 0.4", lvl)
	}
}

func TestShowThePersonPausedIsNotStartedByTheAthanFlow(t *testing.T) {
	r := newTVRig(t, "Living Room TV")
	tv := r.tvs["Living Room TV"]
	tv.SimulateCast("http://example/show.mp4")
	tv.SetPlayerStateForTest("PAUSED") // the person paused it

	r.tvp.PauseForAthan(nil, "Test Speaker")
	r.tvp.ResumeAfterAthan()
	time.Sleep(500 * time.Millisecond)
	if st := tv.PlayerState(); st != "PAUSED" {
		t.Errorf("TV state = %s, want PAUSED (as the person left it)", st)
	}
}

// Pressing "test" while a TV is already muted by the previous run must still
// end with the TV unmuted — the second call must not mistake our own mute for the
// owner's.
func TestOverlappingPausesStillRestoreTheTV(t *testing.T) {
	r := newTVRig(t, "Living Room TV")
	tv := r.tvs["Living Room TV"]

	r.tvp.PauseForAthan(nil, "Test Speaker")
	eventually(t, "TV muted", func() bool { return muted(tv) })
	r.tvp.PauseForAthan(nil, "Test Speaker") // e.g. the test button, 1 minute later
	r.tvp.ResumeAfterAthan()
	eventually(t, "TV unmuted", func() bool { return !muted(tv) })
}

func TestOnlyTheChosenTVsAreAffected(t *testing.T) {
	r := newTVRig(t, "Living Room TV", "Bedroom TV")
	r.tvp.PauseForAthan([]string{"static-living-room-tv"}, "Test Speaker")
	eventually(t, "chosen TV muted", func() bool { return muted(r.tvs["Living Room TV"]) })
	time.Sleep(300 * time.Millisecond)
	if muted(r.tvs["Bedroom TV"]) {
		t.Error("a TV that was not selected got muted")
	}
}

func TestAnUnreachableTVDoesNotStopTheOthers(t *testing.T) {
	r := newTVRig(t, "Living Room TV", "Bedroom TV")
	r.tvs["Bedroom TV"].Close() // off

	done := make(chan struct{})
	go func() { r.tvp.PauseForAthan(nil, "Test Speaker"); close(done) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("PauseForAthan hung on the unreachable TV")
	}
	eventually(t, "reachable TV muted", func() bool { return muted(r.tvs["Living Room TV"]) })
}

func TestAutoResumeUnmutesAfterTheDelay(t *testing.T) {
	r := newTVRig(t, "Living Room TV")
	tv := r.tvs["Living Room TV"]
	r.tvp.PauseForAthan(nil, "Test Speaker")
	eventually(t, "TV muted", func() bool { return muted(tv) })
	r.tvp.ScheduleResume(400 * time.Millisecond)
	eventually(t, "TV unmuted by the timer", func() bool { return !muted(tv) })
}

// The TV reboots or loses Wi-Fi during the minutes it is muted. It must still be
// unmuted afterwards, not left silent.
func TestTVIsUnmutedEvenIfItsConnectionDroppedMeanwhile(t *testing.T) {
	r := newTVRig(t, "Living Room TV")
	tv := r.tvs["Living Room TV"]
	r.tvp.PauseForAthan(nil, "Test Speaker")
	eventually(t, "TV muted", func() bool { return muted(tv) })

	tv.DropConnections()
	time.Sleep(300 * time.Millisecond)
	r.tvp.ResumeAfterAthan()
	eventually(t, "TV unmuted after reconnecting", func() bool { return !muted(tv) })
}

func TestResumeWithNothingPausedDoesNothing(t *testing.T) {
	r := newTVRig(t, "Living Room TV")
	r.tvp.ResumeAfterAthan()
	time.Sleep(200 * time.Millisecond)
	if len(r.tvs["Living Room TV"].Events()) != 0 {
		t.Error("resume touched a TV that was never paused")
	}
}

// Safety net for the whole test suite: tests must only ever see the fake devices
// they create, never real speakers or TVs on the developer's or CI's network.
func TestTestRigsNeverSeeRealDevices(t *testing.T) {
	r := newTVRig(t, "Living Room TV")
	devs, err := r.m.Discover(true)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, d := range devs {
		names[d.Name] = true
		if d.Host != "127.0.0.1" {
			t.Errorf("test discovery returned a non-local device: %+v", d)
		}
	}
	if len(devs) != 2 || !names["Test Speaker"] || !names["Living Room TV"] {
		t.Errorf("discovery returned %+v, want exactly the two fakes", devs)
	}
}
