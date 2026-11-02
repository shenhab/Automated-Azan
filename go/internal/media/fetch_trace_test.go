package media

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"azan-agent/internal/trace"
)

type sinkList struct {
	mu  sync.Mutex
	evs []trace.Event
}

func (s *sinkList) Event(e trace.Event) { s.mu.Lock(); s.evs = append(s.evs, e); s.mu.Unlock() }

func capture(t *testing.T) *sinkList {
	s := &sinkList{}
	trace.SetSink(s)
	t.Cleanup(func() { trace.SetSink(nil) })
	return s
}

func fetch(t *testing.T, s *Server, path, rng string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req.RemoteAddr = "192.168.86.187:51234"
	if rng != "" {
		req.Header.Set("Range", rng)
	}
	rec := httptest.NewRecorder()
	s.handleMedia(rec, req)
	return rec
}

func TestAFetchByASpeakerIsOnTheTimelineWithItsSizeAndWhoAsked(t *testing.T) {
	c := capture(t)
	s := NewServer(t.TempDir(), 0)
	rec := fetch(t, s, "/media/media_Athan.mp3", "")
	if rec.Code != 200 || rec.Body.Len() == 0 {
		t.Fatalf("served %d with %d bytes", rec.Code, rec.Body.Len())
	}
	if len(c.evs) != 1 {
		t.Fatalf("got %d events", len(c.evs))
	}
	e := c.evs[0]
	if e.Source != trace.SourceMedia || e.Kind != "fetch" || e.Fields["remote"] != "192.168.86.187" ||
		e.Fields["name"] != "media_Athan.mp3" || e.Fields["source"] != "embedded" {
		t.Fatalf("event = %+v", e)
	}
	if int(e.Fields["bytes"].(int64)) != rec.Body.Len() || e.Fields["complete"] != true {
		t.Errorf("bytes/complete wrong: %+v (body %d)", e.Fields, rec.Body.Len())
	}
	if !strings.Contains(e.Text, "192.168.86.187") || !strings.Contains(e.Text, "media_Athan.mp3") {
		t.Errorf("text = %q", e.Text)
	}
}

func TestALocalFileAndARangeRequestAreDescribed(t *testing.T) {
	c := capture(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "mine.mp3"), []byte(strings.Repeat("x", 1000)), 0o644)
	s := NewServer(dir, 0)
	rec := fetch(t, s, "/media/mine.mp3", "bytes=0-99")
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status %d", rec.Code)
	}
	e := c.evs[0]
	if e.Fields["source"] != "local" || e.Fields["status"] != 206 || e.Fields["bytes"].(int64) != 100 || e.Fields["complete"] != false {
		t.Errorf("event = %+v", e.Fields)
	}
}

func TestAMissingFileIsAWarning(t *testing.T) {
	c := capture(t)
	s := NewServer(t.TempDir(), 0)
	fetch(t, s, "/media/nope.mp3", "")
	e := c.evs[0]
	if e.Kind != "fetch-failed" || e.Level != "warn" || e.Fields["status"] != 404 {
		t.Errorf("event = %+v", e)
	}
}
