package fakecast_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vishen/go-chromecast/application"

	"azan-agent/internal/fakecast"
)

// These tests drive the fake with the same client library the agent uses
// (go-chromecast), which is what makes it a faithful stand-in for a speaker.

func mediaServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/media/ok.mp3", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write(append([]byte("ID3"), make([]byte, 4096)...))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// eventually polls cond: the client's SetVolume/SetMuted/Stop are
// fire-and-forget, so the speaker applies them a moment after the call returns.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for: %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func startSpeaker(t *testing.T) (*fakecast.Speaker, *application.Application) {
	t.Helper()
	sp := fakecast.New("Test Speaker")
	if err := sp.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sp.Close)
	host, port := sp.Addr()
	app := application.NewApplication()
	if err := app.Start(host, port); err != nil {
		t.Fatalf("real go-chromecast client could not connect to the fake: %v", err)
	}
	t.Cleanup(func() { app.Close(false) })
	return sp, app
}

func TestRealClientLoadsMediaAndSpeakerFetchesIt(t *testing.T) {
	media := mediaServer(t)
	sp, app := startSpeaker(t)

	url := media.URL + "/media/ok.mp3"
	if err := app.Load(url, 0, "audio/mpeg", false, true, false); err != nil {
		t.Fatalf("Load: %v", err)
	}
	loads, err := sp.WaitForLoads(1, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	l := loads[0]
	if l.URL != url || l.ContentType != "audio/mpeg" {
		t.Errorf("load = %+v, want url %s type audio/mpeg", l, url)
	}
	f := l.Fetch
	if !f.Done || f.Status != 200 || f.ContentType != "audio/mpeg" || f.Bytes != 4099 || !f.LooksLikeAudio {
		t.Errorf("speaker's download of the media URL = %+v", f)
	}
	if sp.PlayerState() != "PLAYING" {
		t.Errorf("player state = %s, want PLAYING", sp.PlayerState())
	}
	trace := sp.Trace()
	want := map[string]bool{"connection CONNECT": false, "receiver GET_STATUS": false, "receiver LAUNCH": false, "media LOAD": false}
	for _, m := range trace {
		if _, ok := want[m]; ok {
			want[m] = true
		}
	}
	for m, seen := range want {
		if !seen {
			t.Errorf("speaker never saw %q; trace: %v", m, trace)
		}
	}
}

func TestVolumeMuteAndStop(t *testing.T) {
	media := mediaServer(t)
	sp, app := startSpeaker(t)
	if err := app.Load(media.URL+"/media/ok.mp3", 0, "audio/mpeg", false, true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := sp.WaitForLoads(1, 5*time.Second); err != nil {
		t.Fatal(err)
	}

	if err := app.SetMuted(true); err != nil {
		t.Fatalf("SetMuted: %v", err)
	}
	eventually(t, "speaker muted", func() bool { _, m := sp.Volume(); return m })
	if err := app.SetMuted(false); err != nil {
		t.Fatalf("SetMuted(false): %v", err)
	}
	eventually(t, "speaker unmuted", func() bool { _, m := sp.Volume(); return !m })
	if err := app.SetVolume(0.8); err != nil {
		t.Fatalf("SetVolume: %v", err)
	}
	eventually(t, "volume 0.8", func() bool { l, _ := sp.Volume(); return l > 0.79 && l < 0.81 })
	if err := app.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	eventually(t, "player IDLE after Stop", func() bool { return sp.PlayerState() == "IDLE" })
}

func TestSpeakerRecordsUnreachableMedia(t *testing.T) {
	sp, app := startSpeaker(t)
	// Nothing listens here: a speaker given an unreachable URL must say so.
	if err := app.Load("http://127.0.0.1:1/media/missing.mp3", 0, "audio/mpeg", false, true, false); err != nil {
		t.Fatal(err)
	}
	loads, err := sp.WaitForLoads(1, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if loads[0].Fetch.Err == "" || loads[0].Fetch.Status != 0 {
		t.Errorf("expected a recorded fetch error, got %+v", loads[0].Fetch)
	}
}
