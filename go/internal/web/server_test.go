package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRealIP(t *testing.T) {
	tests := []struct {
		name       string
		realIP     string
		forwardFor string
		remoteAddr string
		want       string
	}{
		{
			name:       "X-Real-IP wins over other sources",
			realIP:     "203.0.113.1",
			forwardFor: "198.51.100.1, 198.51.100.2",
			remoteAddr: "192.0.2.1:12345",
			want:       "203.0.113.1",
		},
		{
			name:       "X-Forwarded-For returns first trimmed entry",
			forwardFor: "198.51.100.1,  198.51.100.2",
			remoteAddr: "192.0.2.1:12345",
			want:       "198.51.100.1",
		},
		{
			name:       "X-Forwarded-For single value",
			forwardFor: "198.51.100.9",
			remoteAddr: "192.0.2.1:12345",
			want:       "198.51.100.9",
		},
		{
			name:       "falls back to RemoteAddr host when no headers set",
			remoteAddr: "192.0.2.1:12345",
			want:       "192.0.2.1",
		},
		{
			name:       "falls back to raw RemoteAddr when it has no port",
			remoteAddr: "not-a-host-port",
			want:       "not-a-host-port",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remoteAddr
			if tc.realIP != "" {
				r.Header.Set("X-Real-IP", tc.realIP)
			}
			if tc.forwardFor != "" {
				r.Header.Set("X-Forwarded-For", tc.forwardFor)
			}

			if got := realIP(r); got != tc.want {
				t.Errorf("realIP() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReadLogTail(t *testing.T) {
	t.Run("returns full content when under limit", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "log.txt")
		content := "line1\nline2\nline3"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		if got := readLogTail(path, 10); got != content {
			t.Errorf("readLogTail() = %q, want %q", got, content)
		}
	})

	t.Run("truncates to last N lines when over limit", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "log.txt")
		content := "line1\nline2\nline3\nline4\nline5"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		want := "line3\nline4\nline5"
		if got := readLogTail(path, 3); got != want {
			t.Errorf("readLogTail() = %q, want %q", got, want)
		}
	})

	t.Run("returns empty string for nonexistent file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "does-not-exist.txt")

		if got := readLogTail(path, 10); got != "" {
			t.Errorf("readLogTail() = %q, want empty string", got)
		}
	})
}
