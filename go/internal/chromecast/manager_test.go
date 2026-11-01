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

// newRig starts a fake speaker and a media server, and returns a Manager that
// finds the speaker through AZAN_STATIC_DEVICES.
func newRig(t *testing.T) (*Manager, *fakecast.Speaker) {
	t.Helper()
	// Speed up waits that exist for real-world networks (the code under test is
	// unchanged; only how long it pauses between attempts).
	oldMDNS, oldPause, oldInt, oldMax := mdnsTimeout, connectRetryPause, backoffRetryInterval, backoffMaxWait
	oldVS, oldVT, oldVP := verifyStartTimeout, verifyStablePeriod, verifyPollEvery
	mdnsTimeout, connectRetryPause, backoffRetryInterval, backoffMaxWait =
		300*time.Millisecond, 100*time.Millisecond, 500*time.Millisecond, 8*time.Second
	verifyStartTimeout, verifyStablePeriod, verifyPollEvery = 1500*time.Millisecond, 300*time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() {
		mdnsTimeout, connectRetryPause, backoffRetryInterval, backoffMaxWait = oldMDNS, oldPause, oldInt, oldMax
		verifyStartTimeout, verifyStablePeriod, verifyPollEvery = oldVS, oldVT, oldVP
	})
	sp := fakecast.New("Test Speaker")
	if err := sp.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sp.Close)

	mux := http.NewServeMux()
	mux.HandleFunc("/media/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write(append([]byte("ID3"), make([]byte, 8192)...))
	})
	media := httptest.NewServer(mux)
	t.Cleanup(media.Close)

	host, port := sp.Addr()
	t.Setenv("AZAN_DISABLE_MDNS", "1") // tests must never reach real devices on the LAN
	t.Setenv("AZAN_STATIC_DEVICES", fmt.Sprintf("Test Speaker=%s:%d", host, port))
	m := NewManager("Test Speaker")
	m.SetMediaBaseURL(media.URL + "/media/")
	return m, sp
}

func TestPlayAthanPicksTheRightFileAndSpeakerCanFetchIt(t *testing.T) {
	for _, tc := range []struct{ prayer, wantFile string }{
		{"Fajr", "media_adhan_al_fajr.mp3"},
		{"Dhuhr", "media_Athan.mp3"},
		{"Isha", "media_Athan.mp3"},
	} {
		t.Run(tc.prayer, func(t *testing.T) {
			m, sp := newRig(t)
			if err := m.PlayAthan(tc.prayer, ""); err != nil {
				t.Fatalf("PlayAthan: %v", err)
			}
			loads, err := sp.WaitForLoads(1, 10*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			l := loads[0]
			if want := m.mediaBaseURL + tc.wantFile; l.URL != want {
				t.Errorf("speaker was told to play %q, want %q", l.URL, want)
			}
			if l.ContentType != "audio/mpeg" || l.Fetch.Status != 200 || !l.Fetch.LooksLikeAudio {
				t.Errorf("speaker could not fetch/play it: type=%q fetch=%+v", l.ContentType, l.Fetch)
			}
		})
	}
}

// The scheduler pre-connects ~60 s before an Athan. That connection must be
// reused, not re-created, so the Athan starts immediately.
func TestPreConnectedSessionIsReused(t *testing.T) {
	m, sp := newRig(t)
	if err := m.PreConnect(); err != nil {
		t.Fatalf("PreConnect: %v", err)
	}
	if err := m.PlayAthan("Dhuhr", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := sp.WaitForLoads(1, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if n := sp.Connections(); n != 1 {
		t.Errorf("speaker saw %d connections, want 1 (the pre-connected one)", n)
	}
}

// Real-world failure: between the pre-connect (60 s before) and the Athan the
// speaker reboots or drops off Wi-Fi, so the cached connection is dead. The
// Athan must still reach the speaker (by reconnecting), and PlayAthan must not
// report success if it did not.
func TestAthanStillPlaysIfSpeakerDropsThePreConnectedSession(t *testing.T) {
	m, sp := newRig(t)
	if err := m.PreConnect(); err != nil {
		t.Fatalf("PreConnect: %v", err)
	}
	sp.DropConnections() // speaker blips after the warm-up

	err := m.PlayAthan("Dhuhr", "")
	loads, waitErr := sp.WaitForLoads(1, 25*time.Second)
	if waitErr != nil {
		t.Fatalf("the Athan never reached the speaker (PlayAthan returned %v): %v", err, waitErr)
	}
	if err != nil {
		t.Errorf("PlayAthan returned an error although the Athan was delivered: %v", err)
	}
	if !loads[0].Fetch.LooksLikeAudio {
		t.Errorf("speaker did not get playable audio: %+v", loads[0].Fetch)
	}
}

// The speaker is off when the Athan fires but comes back within the retry
// window (e.g. it was rebooting). The Athan must still be delivered.
func TestAthanIsDeliveredIfSpeakerComesBackDuringRetries(t *testing.T) {
	m, sp := newRig(t)
	_, port := sp.Addr()
	sp.Close() // speaker is down

	result := make(chan error, 1)
	go func() { result <- m.PlayAthan("Dhuhr", "") }()

	time.Sleep(1500 * time.Millisecond)
	back := fakecast.New("Test Speaker")
	if err := back.StartOnPort(port); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(back.Close)

	if _, err := back.WaitForLoads(1, 20*time.Second); err != nil {
		t.Fatalf("Athan never reached the speaker after it came back: %v", err)
	}
	if err := <-result; err != nil {
		t.Errorf("PlayAthan reported %v although the Athan was delivered", err)
	}
}

// The speaker never comes back. PlayAthan must say so; reporting success when
// nothing played would hide a missed prayer.
func TestPlayAthanReportsFailureWhenSpeakerStaysDown(t *testing.T) {
	m, sp := newRig(t)
	sp.Close()

	start := time.Now()
	err := m.PlayAthan("Dhuhr", "")
	if err == nil {
		t.Fatal("PlayAthan returned nil although the speaker is unreachable")
	}
	if elapsed := time.Since(start); elapsed > 30*time.Second {
		t.Errorf("gave up only after %v", elapsed)
	}
}

// A reply from the speaker can be lost (the client library has a race that drops
// replies arriving before it is ready, and networks lose packets). The Athan must
// still be delivered, by trying again, instead of failing after one 5-second wait.
func TestAthanIsDeliveredEvenIfTheSpeakersReplyIsLost(t *testing.T) {
	m, sp := newRig(t)
	sp.DropReplies("LAUNCH", 1)

	err := m.PlayAthan("Dhuhr", "")
	if err != nil {
		t.Fatalf("PlayAthan failed after a lost reply: %v", err)
	}
	loads, werr := sp.WaitForLoads(1, 20*time.Second)
	if werr != nil {
		t.Fatalf("the Athan never reached the speaker: %v", werr)
	}
	if !loads[0].Fetch.LooksLikeAudio {
		t.Errorf("speaker did not get playable audio: %+v", loads[0].Fetch)
	}
}

// The pre-connected session (opened 60 s before the Athan) must survive a lost
// reply to the "launch app" request: the app is already launched on the speaker, so
// asking again on the SAME session works. Reconnecting still delivers the Athan,
// but needlessly throws away the warm connection — and this race (the client
// library registers its listener after sending) is exactly what showed up as a
// flaky "saw 2 connections" on a busy CI runner.
func TestPreConnectedSessionIsKeptWhenALaunchReplyIsLost(t *testing.T) {
	m, sp := newRig(t)
	if err := m.PreConnect(); err != nil {
		t.Fatalf("PreConnect: %v", err)
	}
	sp.DropReplies("LAUNCH", 1)

	if err := m.PlayAthan("Dhuhr", ""); err != nil {
		t.Fatalf("PlayAthan: %v", err)
	}
	loads, err := sp.WaitForLoads(1, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !loads[0].Fetch.LooksLikeAudio {
		t.Errorf("speaker did not get playable audio: %+v", loads[0].Fetch)
	}
	if n := sp.Connections(); n != 1 {
		t.Errorf("speaker saw %d connections, want 1: the warm session should have been retried, not dropped", n)
	}
}

// ── the agent must know whether the speaker really started playing ───────────
//
// "The speaker accepted the request" is not "the Athan is playing": a real device
// can accept a LOAD and then do nothing, report LOAD_FAILED, or start and stop with
// an error — and the agent used to log success for all of them. After every play
// request it now asks the speaker what it is doing, retries if playback did not
// start, reports a real failure if it never does, and flags a speaker that is
// playing but muted.

func loadCount(sp *fakecast.Speaker) int { return len(sp.Loads()) }

func speakerPlaying(sp *fakecast.Speaker) bool { return sp.PlayerState() == "PLAYING" }

func TestPlaybackIsVerifiedAndReported(t *testing.T) {
	m, sp := newRig(t)
	if err := m.PlayAthan("Dhuhr", ""); err != nil {
		t.Fatal(err)
	}
	pb := m.LastPlayback()
	if !pb.Started || pb.State != "PLAYING" || pb.Speaker != "Test Speaker" {
		t.Errorf("LastPlayback = %+v, want a verified PLAYING on Test Speaker", pb)
	}
	if pb.Muted || pb.Level != 0.5 || pb.Warning != "" {
		t.Errorf("volume report wrong: %+v", pb)
	}
	if !speakerPlaying(sp) || loadCount(sp) != 1 {
		t.Errorf("speaker playing=%v after %d LOADs, want playing after exactly 1", speakerPlaying(sp), loadCount(sp))
	}
}

func TestALoadTheSpeakerIgnoresIsRetried(t *testing.T) {
	m, sp := newRig(t)
	sp.IgnoreNextLoads(1)
	if err := m.PlayAthan("Dhuhr", ""); err != nil {
		t.Fatalf("PlayAthan: %v", err)
	}
	if !speakerPlaying(sp) || loadCount(sp) != 2 {
		t.Errorf("playing=%v after %d LOADs, want playing after 2 (the first was ignored)", speakerPlaying(sp), loadCount(sp))
	}
	if !m.LastPlayback().Started {
		t.Errorf("playback not recorded as started: %+v", m.LastPlayback())
	}
}

func TestLoadFailedIsRetriedAtOnce(t *testing.T) {
	m, sp := newRig(t)
	sp.FailNextLoads(1)
	start := time.Now()
	if err := m.PlayAthan("Dhuhr", ""); err != nil {
		t.Fatalf("PlayAthan: %v", err)
	}
	if !speakerPlaying(sp) || loadCount(sp) != 2 {
		t.Errorf("playing=%v after %d LOADs, want playing after 2", speakerPlaying(sp), loadCount(sp))
	}
	// A speaker that says it failed is retried immediately, not after waiting out the
	// whole start-up timeout (which, at Athan time, would be seconds of silence).
	if took := time.Since(start); took >= verifyStartTimeout {
		t.Errorf("recovering from LOAD_FAILED took %v, as long as the start timeout (%v): the failure was not noticed straight away", took, verifyStartTimeout)
	}
}

func TestPlaybackThatStopsStraightAfterStartingIsRetried(t *testing.T) {
	m, sp := newRig(t)
	sp.FailPlaybackAfterStart(1, 120*time.Millisecond, "ERROR") // the device gives up shortly after starting
	if err := m.PlayAthan("Dhuhr", ""); err != nil {
		t.Fatalf("PlayAthan: %v", err)
	}
	if !speakerPlaying(sp) || loadCount(sp) != 2 {
		t.Errorf("playing=%v after %d LOADs, want playing after 2", speakerPlaying(sp), loadCount(sp))
	}
}

func TestARealFailureIsReportedWhenTheSpeakerNeverPlays(t *testing.T) {
	m, sp := newRig(t)
	sp.IgnoreNextLoads(100)
	err := m.PlayAthan("Dhuhr", "")
	if err == nil {
		t.Fatal("PlayAthan reported success although the speaker never played anything")
	}
	if !strings.Contains(err.Error(), "did not start playing") {
		t.Errorf("error %q does not say the speaker did not start playing", err)
	}
	if n := loadCount(sp); n != loadAttempts {
		t.Errorf("tried %d times, want %d", n, loadAttempts)
	}
	if m.LastPlayback().Started {
		t.Errorf("a failed attempt was recorded as started: %+v", m.LastPlayback())
	}
}

func TestAStalePlayingSessionDoesNotCountAsSuccess(t *testing.T) {
	// The speaker is still playing the previous Athan's session. If the next LOAD is
	// ignored, that old PLAYING must not be mistaken for the new request working.
	m, sp := newRig(t)
	if err := m.PlayAthan("Dhuhr", ""); err != nil {
		t.Fatal(err)
	}
	sp.IgnoreNextLoads(1)
	if err := m.PlayAthan("Dhuhr", ""); err != nil {
		t.Fatalf("second PlayAthan: %v", err)
	}
	if n := loadCount(sp); n != 3 {
		t.Errorf("%d LOADs in total, want 3 (first ok, second ignored and retried)", n)
	}
}

func TestAMutedOrSilentSpeakerIsFlagged(t *testing.T) {
	for name, tc := range map[string]struct {
		level float64
		muted bool
		want  string
	}{
		"muted":    {0.5, true, "muted"},
		"volume 0": {0, false, "volume"},
		"audible":  {0.5, false, ""},
	} {
		t.Run(name, func(t *testing.T) {
			m, sp := newRig(t)
			sp.SetVolumeState(tc.level, tc.muted)
			if err := m.PlayAthan("Dhuhr", ""); err != nil {
				t.Fatal(err)
			}
			pb := m.LastPlayback()
			if !pb.Started {
				t.Fatalf("not started: %+v", pb)
			}
			if tc.want == "" && pb.Warning != "" {
				t.Errorf("warned about an audible speaker: %q", pb.Warning)
			}
			if tc.want != "" && !strings.Contains(strings.ToLower(pb.Warning), tc.want) {
				t.Errorf("warning %q does not mention %q", pb.Warning, tc.want)
			}
		})
	}
}

func TestTheWarmSessionIsKeptWhenALoadIsIgnored(t *testing.T) {
	m, sp := newRig(t)
	if err := m.PreConnect(); err != nil {
		t.Fatal(err)
	}
	sp.IgnoreNextLoads(1)
	if err := m.PlayAthan("Dhuhr", ""); err != nil {
		t.Fatal(err)
	}
	if !speakerPlaying(sp) || loadCount(sp) != 2 || sp.Connections() != 1 {
		t.Errorf("playing=%v, %d LOADs, %d connections; want playing after 2 LOADs on the 1 warm connection",
			speakerPlaying(sp), loadCount(sp), sp.Connections())
	}
}
