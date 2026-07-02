package chromecast

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestLoadCacheSeedsDevices verifies LoadCache reads a valid cache file and
// populates the in-memory device map so playback can start before the
// mDNS scan finishes.
func TestLoadCacheSeedsDevices(t *testing.T) {
	dir := t.TempDir()
	want := []Device{
		{UUID: "uuid-1", Name: "Kitchen Speaker", Host: "192.168.1.10", Port: 8009, ModelName: "Chromecast Audio"},
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, cacheFileName), data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	m := NewManager("Kitchen Speaker")
	m.LoadCache(dir)

	got := m.Devices()
	if len(got) != 1 {
		t.Fatalf("Devices() = %v, want 1 device", got)
	}
	if got[0] != want[0] {
		t.Errorf("Devices()[0] = %+v, want %+v", got[0], want[0])
	}
}

// TestLoadCacheMissingFile verifies LoadCache is a no-op when the cache
// file does not exist yet (e.g. first run).
func TestLoadCacheMissingFile(t *testing.T) {
	dir := t.TempDir()

	m := NewManager("Kitchen Speaker")
	m.LoadCache(dir)

	if got := m.Devices(); len(got) != 0 {
		t.Errorf("Devices() = %v, want empty", got)
	}
}

// TestLoadCacheMalformedJSON verifies LoadCache skips gracefully without
// seeding any devices when the cache file contains invalid JSON.
func TestLoadCacheMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, cacheFileName), []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	m := NewManager("Kitchen Speaker")
	m.LoadCache(dir)

	if got := m.Devices(); len(got) != 0 {
		t.Errorf("Devices() = %v, want empty after malformed cache", got)
	}
}

// TestSaveCacheRoundTrip verifies saveCache writes a file that LoadCache can
// read back into an equivalent device map.
func TestSaveCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()

	m1 := NewManager("Living Room Speaker")
	m1.devices["uuid-2"] = Device{UUID: "uuid-2", Name: "Living Room Speaker", Host: "192.168.1.20", Port: 8009, ModelName: "Chromecast"}
	m1.saveCache(dir)

	if _, err := os.Stat(filepath.Join(dir, cacheFileName)); err != nil {
		t.Fatalf("expected cache file to exist: %v", err)
	}

	m2 := NewManager("Living Room Speaker")
	m2.LoadCache(dir)

	got := m2.Devices()
	if len(got) != 1 {
		t.Fatalf("Devices() = %v, want 1 device", got)
	}
	if got[0] != m1.devices["uuid-2"] {
		t.Errorf("Devices()[0] = %+v, want %+v", got[0], m1.devices["uuid-2"])
	}
}

// TestSaveCacheEmptyDataDir verifies saveCache does nothing when dataDir is
// empty, rather than attempting to write relative to the working directory.
func TestSaveCacheEmptyDataDir(t *testing.T) {
	m := NewManager("Kitchen Speaker")
	m.devices["uuid-3"] = Device{UUID: "uuid-3", Name: "Kitchen Speaker"}

	m.saveCache("")

	if _, err := os.Stat(cacheFileName); err == nil {
		os.Remove(cacheFileName)
		t.Fatalf("saveCache(\"\") wrote %q in the working directory, want no-op", cacheFileName)
	}
}
