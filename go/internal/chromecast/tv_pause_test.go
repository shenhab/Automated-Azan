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

// The next tests are about a TV like the NVIDIA SHIELD, whose Cast mute is a
// blind "toggle mute" key press (measured live, 2026-11-02): when the owner
// unmutes with the remote during the Athan, the Cast session still says "muted",
// and the old end-of-Athan "unmute" silenced the TV again.

func TestRemoteUnmutedTVIsNotSilencedWhenTheAthanEnds(t *testing.T) {
	r := newTVRig(t, "Living Room TV")
	tv := r.tvs["Living Room TV"]
	tv.ToggleMuteTV()
	tv.SetVolumeState(0.13, false)

	r.tvp.PauseForAthan(nil, "Test Speaker")
	eventually(t, "TV silenced", func() bool { return !tv.Audible() })

	tv.RemoteMuteButton() // the owner presses unmute on the remote mid-Athan
	if !tv.Audible() {
		t.Fatal("test set-up: the remote should have made the TV audible")
	}

	r.tvp.ResumeAfterAthan()
	time.Sleep(1500 * time.Millisecond) // let every release command go out
	if !tv.Audible() {
		t.Error("the end-of-Athan release silenced a TV its owner had unmuted")
	}
}

func TestToggleTVStaysSilentUntilReleaseThenIsAudible(t *testing.T) {
	r := newTVRig(t, "Living Room TV")
	tv := r.tvs["Living Room TV"]
	tv.ToggleMuteTV()
	tv.SetVolumeState(0.13, false)

	r.tvp.PauseForAthan(nil, "Test Speaker")
	eventually(t, "TV silenced", func() bool { return !tv.Audible() })

	r.tvp.ResumeAfterAthan()
	eventually(t, "TV audible again", func() bool { return tv.Audible() })
}

func TestReleaseRestoresTheExactVolume(t *testing.T) {
	for _, level := range []float64{0.13, 0.5, 0.99, 1.0, 0.0} {
		t.Run(fmt.Sprint(level), func(t *testing.T) {
			r := newTVRig(t, "Living Room TV")
			tv := r.tvs["Living Room TV"]
			tv.ToggleMuteTV()
			tv.SetVolumeState(level, false)

			r.tvp.PauseForAthan(nil, "Test Speaker")
			eventually(t, "TV silenced", func() bool { return !tv.Audible() })
			r.tvp.ResumeAfterAthan()
			eventually(t, "TV audible again", func() bool { return tv.Audible() })
			time.Sleep(1500 * time.Millisecond)

			got, _ := tv.Volume()
			if d := got - level; d > 0.001 || d < -0.001 {
				t.Errorf("volume after release = %.3f, want %.3f", got, level)
			}
		})
	}
}

func TestOrdinaryTVStillGetsAPlainUnmute(t *testing.T) {
	r := newTVRig(t, "Living Room TV")
	tv := r.tvs["Living Room TV"]
	r.tvp.PauseForAthan(nil, "Test Speaker")
	eventually(t, "TV muted", func() bool { return muted(tv) })
	r.tvp.ResumeAfterAthan()
	eventually(t, "TV unmuted", func() bool { return !muted(tv) })
	// The release nudges the volume for a moment and puts it straight back, so
	// wait for the end state, then make sure it stays there.
	eventually(t, "volume back at 0.5", func() bool { got, _ := tv.Volume(); return got == 0.5 })
	time.Sleep(700 * time.Millisecond)
	if got, _ := tv.Volume(); got != 0.5 {
		t.Errorf("volume = %v, want it untouched at 0.5", got)
	}
}

// --- release driven by the speaker's playback, not by a timer ---

func fastRelease(t *testing.T, settle, hold time.Duration) {
	t.Helper()
	oldSettle, oldPoll, oldHold, oldTimeout := releaseSettle, releasePoll, maxAthanHold, pollTimeout
	releaseSettle, releasePoll, maxAthanHold, pollTimeout = settle, 100*time.Millisecond, hold, time.Second
	t.Cleanup(func() {
		releaseSettle, releasePoll, maxAthanHold, pollTimeout = oldSettle, oldPoll, oldHold, oldTimeout
	})
}

// athanPlaying mutes the TVs and starts the Athan on the test speaker, then arms
// the playback-driven release, exactly as the scheduler does.
func athanPlaying(t *testing.T, r *tvRig) {
	t.Helper()
	r.tvp.PauseForAthan(nil, "Test Speaker")
	if err := r.m.PlayAthan("Dhuhr", ""); err != nil {
		t.Fatalf("PlayAthan: %v", err)
	}
	r.tvp.ScheduleResume(5 * time.Minute) // what the scheduler arms as a fallback
	r.tvp.ReleaseWhenAthanEnds()
}

func TestTVStaysMutedWhileTheAthanPlaysAndIsReleasedSecondsAfterItEnds(t *testing.T) {
	fastRelease(t, 400*time.Millisecond, time.Minute)
	r := newTVRig(t, "Living Room TV")
	tv := r.tvs["Living Room TV"]
	athanPlaying(t, r)

	time.Sleep(1200 * time.Millisecond)
	if !muted(tv) {
		t.Fatal("the TV was unmuted while the Athan was still playing")
	}

	ended := time.Now()
	r.athan.FinishPlayback()
	time.Sleep(150 * time.Millisecond)
	if !muted(tv) {
		t.Error("the TV was unmuted at once; it should wait a moment after the Athan ends")
	}
	eventually(t, "TV unmuted after the Athan ended", func() bool { return !muted(tv) })
	if d := time.Since(ended); d < 400*time.Millisecond {
		t.Errorf("TV unmuted %v after the end, before the %v settle time", d, 400*time.Millisecond)
	}
	if r.tvp.IsPaused() {
		t.Error("still holding the TV after release")
	}
}

func TestTVIsReleasedWhenTheAthanIsStoppedEarly(t *testing.T) {
	fastRelease(t, 200*time.Millisecond, time.Minute)
	r := newTVRig(t, "Living Room TV")
	tv := r.tvs["Living Room TV"]
	athanPlaying(t, r)
	time.Sleep(600 * time.Millisecond)

	if err := r.m.StopPlayback(); err != nil {
		t.Fatal(err)
	}
	eventually(t, "TV unmuted after a stop", func() bool { return !muted(tv) })
}

func TestTVIsReleasedAfterAConnectionDropMidAthan(t *testing.T) {
	fastRelease(t, 200*time.Millisecond, time.Minute)
	r := newTVRig(t, "Living Room TV")
	tv := r.tvs["Living Room TV"]
	athanPlaying(t, r)
	time.Sleep(500 * time.Millisecond)

	r.athan.DropConnections() // Wi-Fi blip: the watcher must reconnect and keep watching
	time.Sleep(800 * time.Millisecond)
	if !muted(tv) {
		t.Fatal("a dropped connection released the TV while the Athan was still playing")
	}
	r.athan.FinishPlayback()
	eventually(t, "TV unmuted after the Athan ended", func() bool { return !muted(tv) })
}

func TestTVIsNeverLeftMutedIfThePlaybackNeverEnds(t *testing.T) {
	fastRelease(t, 100*time.Millisecond, 1500*time.Millisecond)
	r := newTVRig(t, "Living Room TV")
	tv := r.tvs["Living Room TV"]
	athanPlaying(t, r)

	start := time.Now()
	eventually(t, "TV unmuted at the hold cap", func() bool { return !muted(tv) })
	if d := time.Since(start); d < 1200*time.Millisecond {
		t.Errorf("released after %v, before the hold cap", d)
	}
}

func TestReleaseWhenAthanEndsWithNothingHeldDoesNothing(t *testing.T) {
	fastRelease(t, 100*time.Millisecond, time.Minute)
	r := newTVRig(t, "Living Room TV")
	r.tvp.ReleaseWhenAthanEnds() // must not panic or start anything
	time.Sleep(300 * time.Millisecond)
	if r.tvp.IsPaused() {
		t.Error("nothing was paused")
	}
}

func TestTheVolumeNudgeIsBiggerThanAnyKnownVolumeStep(t *testing.T) {
	// The nudge goes to every TV, so it cannot be tuned to one model: a change
	// smaller than a device's step may be rounded away and not give the sound back.
	for model, step := range map[string]float64{"Nest speaker": 0.02, "Chromecast": 0.05, "SHIELD": 0.0667} {
		if volumeNudge <= step {
			t.Errorf("nudge %.3f is not bigger than the %.4f step of a %s", volumeNudge, step, model)
		}
	}
}
