package config

import (
	"os"
	"sync"
	"testing"
	"time"
)

// TestWatcher_ReloadFiresOnValidChange writes a temp config, starts a
// Watcher on it, edits the file, and asserts the OnChange callback fires
// with the previous and newly-parsed values.
func TestWatcher_ReloadFiresOnValidChange(t *testing.T) {
	path := writeTemp(t, validTOML)

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom returned unexpected error: %v", err)
	}

	w := NewWatcher(cfg)
	w.DebounceInterval = 20 * time.Millisecond

	type change struct{ oldC, newC *Config }
	changes := make(chan change, 1)
	w.OnChange(func(oldC, newC *Config) {
		changes <- change{oldC, newC}
	})

	if err := w.Start(); err != nil {
		t.Fatalf("Start() returned unexpected error: %v", err)
	}
	defer w.Stop()

	if err := os.WriteFile(path, []byte(`
[prayer]
location = "naas"

[web]
port = 12345
`), 0o644); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}

	select {
	case c := <-changes:
		if c.oldC.Prayer.Location != "icci" {
			t.Errorf("old.Prayer.Location = %q, want %q", c.oldC.Prayer.Location, "icci")
		}
		if c.newC.Prayer.Location != "naas" {
			t.Errorf("new.Prayer.Location = %q, want %q", c.newC.Prayer.Location, "naas")
		}
		if c.newC.Web.Port != 12345 {
			t.Errorf("new.Web.Port = %d, want 12345", c.newC.Web.Port)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("OnChange handler did not fire within 5s of the config file changing")
	}

	if cfg.Prayer.Location != "naas" {
		t.Errorf("cfg.Prayer.Location = %q after reload, want %q", cfg.Prayer.Location, "naas")
	}
	if cfg.Web.Port != 12345 {
		t.Errorf("cfg.Web.Port = %d after reload, want 12345", cfg.Web.Port)
	}
}

// TestWatcher_InvalidEditDoesNotCorruptLiveConfig mirrors the safe-reload
// pattern verified for Config.Reload() directly: an on-disk edit that fails
// validation must not fire the OnChange callback or mutate the
// already-loaded live config.
func TestWatcher_InvalidEditDoesNotCorruptLiveConfig(t *testing.T) {
	path := writeTemp(t, validTOML)

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom returned unexpected error: %v", err)
	}

	w := NewWatcher(cfg)
	w.DebounceInterval = 20 * time.Millisecond

	var mu sync.Mutex
	fired := false
	w.OnChange(func(oldC, newC *Config) {
		mu.Lock()
		fired = true
		mu.Unlock()
	})

	if err := w.Start(); err != nil {
		t.Fatalf("Start() returned unexpected error: %v", err)
	}
	defer w.Stop()

	if err := os.WriteFile(path, []byte(`
[prayer]
location = "atlantis"
`), 0o644); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}

	// The watcher debounces reloads for a quiet period before acting; wait
	// comfortably past that so reload() has a chance to run before we assert
	// nothing changed.
	time.Sleep(3 * w.DebounceInterval)

	mu.Lock()
	defer mu.Unlock()
	if fired {
		t.Errorf("OnChange handler fired for an invalid config edit, want no callback")
	}
	if cfg.Prayer.Location != "icci" {
		t.Errorf("cfg.Prayer.Location = %q after invalid edit, want unchanged %q", cfg.Prayer.Location, "icci")
	}
	if cfg.Web.Port != 9090 {
		t.Errorf("cfg.Web.Port = %d after invalid edit, want unchanged 9090", cfg.Web.Port)
	}
}
