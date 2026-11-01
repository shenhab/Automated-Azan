package chromecast

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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
	mdnsTimeout, connectRetryPause, backoffRetryInterval, backoffMaxWait =
		300*time.Millisecond, 100*time.Millisecond, 500*time.Millisecond, 8*time.Second
	t.Cleanup(func() {
		mdnsTimeout, connectRetryPause, backoffRetryInterval, backoffMaxWait = oldMDNS, oldPause, oldInt, oldMax
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
