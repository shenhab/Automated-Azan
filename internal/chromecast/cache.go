package chromecast

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

const cacheFileName = "devices.json"

// LoadCache reads the device cache from the data directory and seeds the
// in-memory device map so the app can play immediately on startup without
// waiting for an mDNS scan. lastDiscovery is intentionally left at zero so
// the background scan still runs and refreshes the cache.
func (m *Manager) LoadCache(dataDir string) {
	path := filepath.Join(dataDir, cacheFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[chromecast] cache read error: %v", err)
		}
		return
	}

	var devices []Device
	if err := json.Unmarshal(data, &devices); err != nil {
		log.Printf("[chromecast] cache parse error: %v", err)
		return
	}

	m.mu.Lock()
	for _, d := range devices {
		m.devices[d.UUID] = d
	}
	m.mu.Unlock()
	log.Printf("[chromecast] loaded %d device(s) from cache", len(devices))
}

// saveCache writes the current device list to the data directory.
func (m *Manager) saveCache(dataDir string) {
	if dataDir == "" {
		return
	}
	devs := m.Devices()
	data, err := json.MarshalIndent(devs, "", "  ")
	if err != nil {
		log.Printf("[chromecast] cache marshal error: %v", err)
		return
	}
	path := filepath.Join(dataDir, cacheFileName)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("[chromecast] cache write error: %v", err)
		return
	}
	log.Printf("[chromecast] saved %d device(s) to cache", len(devs))
}
