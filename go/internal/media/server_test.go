package media

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestServer(t *testing.T, dir string) *Server {
	t.Helper()
	return &Server{dir: dir, port: 8080, localIP: "192.168.1.50"}
}

func TestHandleMedia_LocalFileHit(t *testing.T) {
	dir := t.TempDir()
	want := "local file contents"
	if err := os.WriteFile(filepath.Join(dir, "custom.mp3"), []byte(want), 0644); err != nil {
		t.Fatalf("write local file: %v", err)
	}

	s := newTestServer(t, dir)
	req := httptest.NewRequest("GET", "/media/custom.mp3", nil)
	rec := httptest.NewRecorder()

	s.handleMedia(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestHandleMedia_EmbeddedFallbackHit(t *testing.T) {
	// dir has no files, so a request for an embedded-only file must fall back.
	dir := t.TempDir()
	s := newTestServer(t, dir)

	req := httptest.NewRequest("GET", "/media/default.mp3", nil)
	rec := httptest.NewRecorder()

	s.handleMedia(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("expected non-empty embedded file body")
	}
	if ct := rec.Header().Get("Content-Type"); ct != "audio/mpeg" {
		t.Fatalf("Content-Type = %q, want audio/mpeg", ct)
	}
}

func TestHandleMedia_NotFound(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(t, dir)

	req := httptest.NewRequest("GET", "/media/does-not-exist.mp3", nil)
	rec := httptest.NewRecorder()

	s.handleMedia(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandleMedia_PathTraversalBlocked(t *testing.T) {
	dir := t.TempDir()
	// Sensitive file that must NEVER be reachable via traversal - it lives outside dir.
	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, "passwd"), []byte("secret"), 0644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	s := newTestServer(t, dir)
	req := httptest.NewRequest("GET", "/media/"+strings.Repeat("../", 6)+"etc/passwd", nil)
	rec := httptest.NewRecorder()

	s.handleMedia(rec, req)

	// filepath.Base reduces the traversal to just "passwd", which doesn't
	// exist in dir or embedded, and http.ServeFile additionally rejects any
	// request whose raw URL path still contains "..". Either way the
	// outside file's contents must never be leaked.
	if rec.Code == 200 && rec.Body.String() == "secret" {
		t.Fatalf("path traversal served file outside dir: %q", rec.Body.String())
	}
	if rec.Code != 400 && rec.Code != 404 {
		t.Fatalf("status = %d, want 400 or 404 for blocked traversal", rec.Code)
	}
}

func TestBaseURL(t *testing.T) {
	s := &Server{port: 8080, localIP: "192.168.1.50"}
	want := "http://192.168.1.50:8080/media/"
	if got := s.BaseURL(); got != want {
		t.Fatalf("BaseURL() = %q, want %q", got, want)
	}
}
