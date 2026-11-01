package fakecast_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vishen/go-chromecast/application"
	"github.com/vishen/go-chromecast/cast"

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

// A real speaker is reached over Wi-Fi: it never answers in microseconds. A fake
// that does exposes a race in the client library (it registers its listener after
// sending a request, so an instant reply can be lost; CI hit this as "unable to
// change to appID ... context deadline exceeded"). So the fake answers with a small,
// realistic delay by default.

func TestTheSpeakerAnswersLikeARealDeviceOverANetwork(t *testing.T) {
	sp, app := startSpeaker(t)

	start := time.Now()
	if err := app.Update(); err != nil { // one request/response round trip
		t.Fatal(err)
	}
	if took := time.Since(start); took < 2*time.Millisecond {
		t.Errorf("a round trip took %v; a real device over a network takes longer than that", took)
	}

	sp.SetReplyDelay(60 * time.Millisecond)
	start = time.Now()
	if err := app.Update(); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took < 60*time.Millisecond {
		t.Errorf("with a 60 ms reply delay a round trip took only %v", took)
	}
}

// ── misbehaviour the fake can simulate (used to test the agent's playback checks) ──

func mediaState(app interface {
	Update() error
	Media() *cast.Media
}) (state, reason string) {
	app.Update()
	if m := app.Media(); m != nil {
		return m.PlayerState, m.IdleReason
	}
	return "none", ""
}

func TestFakeCanAcceptALoadAndDoNothing(t *testing.T) {
	media := mediaServer(t)
	sp, app := startSpeaker(t)
	sp.IgnoreNextLoads(1)
	if err := app.Load(media.URL+"/media/ok.mp3", 0, "audio/mpeg", false, true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := sp.WaitForLoads(1, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if st, _ := mediaState(app); st != "none" {
		t.Errorf("an ignored LOAD produced media state %q, want none", st)
	}
	// The next one works again.
	if err := app.Load(media.URL+"/media/ok.mp3", 0, "audio/mpeg", false, true, false); err != nil {
		t.Fatal(err)
	}
	if st, _ := mediaState(app); st != "PLAYING" {
		t.Errorf("after the misbehaviour ended, state = %q, want PLAYING", st)
	}
}

func TestFakeCanReportLoadFailed(t *testing.T) {
	media := mediaServer(t)
	sp, app := startSpeaker(t)
	sp.FailNextLoads(1)
	if err := app.Load(media.URL+"/media/ok.mp3", 0, "audio/mpeg", false, true, false); err != nil {
		t.Fatal(err)
	}
	if st, reason := mediaState(app); st != "IDLE" || reason != "ERROR" {
		t.Errorf("after LOAD_FAILED state = %s/%s, want IDLE/ERROR", st, reason)
	}
}

func TestFakeCanStartPlayingAndThenStopWithAnError(t *testing.T) {
	media := mediaServer(t)
	sp, app := startSpeaker(t)
	sp.FailPlaybackAfterStart(1, 400*time.Millisecond, "ERROR")
	if err := app.Load(media.URL+"/media/ok.mp3", 0, "audio/mpeg", false, true, false); err != nil {
		t.Fatal(err)
	}
	if st, _ := mediaState(app); st != "PLAYING" {
		t.Errorf("right after the LOAD state = %q, want PLAYING", st)
	}
	time.Sleep(700 * time.Millisecond)
	if st, reason := mediaState(app); st != "IDLE" || reason != "ERROR" {
		t.Errorf("later state = %s/%s, want IDLE/ERROR", st, reason)
	}
}
