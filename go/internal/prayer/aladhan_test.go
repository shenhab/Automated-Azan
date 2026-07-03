package prayer

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStripTZ(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"with timezone suffix", "05:30 (IST)", "05:30"},
		{"without timezone suffix", "05:30", "05:30"},
		{"different suffix", "18:45 (GMT)", "18:45"},
		{"empty string", "", ""},
		{"only suffix marker with no closing paren", "05:30 (", "05:30"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripTZ(tt.input)
			if got != tt.want {
				t.Errorf("stripTZ(%q) = %q; want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAladhanBackupPath(t *testing.T) {
	dataDir := t.TempDir()
	f, err := NewFetcher(dataDir)
	if err != nil {
		t.Fatalf("NewFetcher: %v", err)
	}

	want := filepath.Join(dataDir, "aladhan_backup.json")
	got := f.aladhanBackupPath()
	if got != want {
		t.Errorf("aladhanBackupPath() = %q; want %q", got, want)
	}

	// Stable across repeated calls.
	if got2 := f.aladhanBackupPath(); got2 != got {
		t.Errorf("aladhanBackupPath() not stable: %q then %q", got, got2)
	}
}

// withAladhanTestServer points aladhanBase at an httptest.Server for the
// duration of the test and restores it afterwards.
func withAladhanTestServer(t *testing.T, handler http.HandlerFunc) *Fetcher {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	original := aladhanBase
	aladhanBase = srv.URL
	t.Cleanup(func() { aladhanBase = original })

	f, err := NewFetcher(t.TempDir())
	if err != nil {
		t.Fatalf("NewFetcher: %v", err)
	}
	return f
}

func TestFetchAladhanAPI_ValidResponse(t *testing.T) {
	f := withAladhanTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{
			"data": {
				"timings": {
					"Fajr": "05:30 (IST)",
					"Dhuhr": "13:05 (IST)",
					"Asr": "16:45 (IST)",
					"Maghrib": "19:50 (IST)",
					"Isha": "21:15 (IST)"
				}
			}
		}`)
	})

	got, err := f.fetchAladhanAPI("Dublin", "Ireland", 3, time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("fetchAladhanAPI() unexpected error: %v", err)
	}

	want := Times{Fajr: "05:30", Dhuhr: "13:05", Asr: "16:45", Maghrib: "19:50", Isha: "21:15"}
	if got != want {
		t.Errorf("fetchAladhanAPI() = %+v; want %+v", got, want)
	}
}

func TestFetchAladhanAPI_MalformedJSON(t *testing.T) {
	f := withAladhanTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{not valid json`)
	})

	_, err := f.fetchAladhanAPI("Dublin", "Ireland", 3, time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("fetchAladhanAPI() error = nil; want error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("fetchAladhanAPI() error = %v; want error mentioning decode", err)
	}
}

func TestFetchAladhanAPI_NonOKStatus(t *testing.T) {
	f := withAladhanTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := f.fetchAladhanAPI("Dublin", "Ireland", 3, time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("fetchAladhanAPI() error = nil; want error for non-200 status")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("fetchAladhanAPI() error = %v; want error mentioning HTTP 500", err)
	}
}
