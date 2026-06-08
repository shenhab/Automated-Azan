package chromecast

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/vishen/go-chromecast/application"
	castdns "github.com/vishen/go-chromecast/dns"
)

const (
	discoveryCooldown = 60 * time.Second
	connectTimeout    = 10 * time.Second
	maxRetries        = 3
)

// QuranStation is the default live Quran recitation stream.
var QuranStation = map[string]string{
	"name": "Mahmoud Khalil Al-Hussary (Warsh)",
	"url":  "https://backup.qurango.net/radio/mahmoud_khalil_alhussary_warsh",
}

// Device holds discovered Chromecast device information.
type Device struct {
	UUID      string `json:"uuid"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	ModelName string `json:"model_name"`
}

// Manager discovers Chromecast devices and plays audio on them.
type Manager struct {
	deviceName   string // configured target speaker name
	mediaBaseURL string // e.g. http://192.168.1.x:5000/media/
	cacheDir     string // data directory for devices.json cache

	mu            sync.Mutex
	devices       map[string]Device // uuid → Device
	lastDiscovery time.Time
	app           *application.Application
	currentDevice *Device

	athanPlaying bool
	quranPlaying bool
}

// NewManager creates a Manager targeting the named device.
func NewManager(deviceName string) *Manager {
	return &Manager{
		deviceName: deviceName,
		devices:    make(map[string]Device),
	}
}

// SetCacheDir sets the directory used to persist the device cache.
// Call before LoadCache and Discover.
func (m *Manager) SetCacheDir(dir string) {
	m.mu.Lock()
	m.cacheDir = dir
	m.mu.Unlock()
}

// SetMediaBaseURL sets the HTTP base URL used to construct media URLs for Chromecast.
func (m *Manager) SetMediaBaseURL(u string) {
	m.mu.Lock()
	m.mediaBaseURL = u
	m.mu.Unlock()
}

// Discover finds Chromecast devices on the local network via mDNS.
// Uses a 60-second cooldown cache to avoid redundant scans.
func (m *Manager) Discover(force bool) ([]Device, error) {
	m.mu.Lock()
	if !force && time.Since(m.lastDiscovery) < discoveryCooldown && len(m.devices) > 0 {
		devs := m.deviceList()
		m.mu.Unlock()
		return devs, nil
	}
	m.mu.Unlock()

	log.Println("[chromecast] discovering devices via mDNS...")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	ch, err := castdns.DiscoverCastDNSEntries(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("mDNS discover: %w", err)
	}

	found := make(map[string]Device)
	for entry := range ch {
		d := Device{
			UUID:      entry.UUID,
			Name:      entry.DeviceName,
			Host:      entry.AddrV4.String(),
			Port:      entry.Port,
			ModelName: entry.Device,
		}
		found[entry.UUID] = d
		log.Printf("[chromecast] found: %s (%s) at %s:%d", d.Name, d.ModelName, d.Host, d.Port)
	}

	m.mu.Lock()
	m.devices = found
	m.lastDiscovery = time.Now()
	cacheDir := m.cacheDir
	m.mu.Unlock()

	go m.saveCache(cacheDir)

	return m.deviceList(), nil
}

// Devices returns the currently cached device list without re-scanning.
func (m *Manager) Devices() []Device {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.deviceList()
}

// PlayAthan plays the specified audio file on the configured device.
// filename is the MP3 filename (e.g. "media_Athan.mp3"); if empty the
// default for that prayer is used.
func (m *Manager) PlayAthan(prayer, filename string) error {
	m.mu.Lock()
	if m.athanPlaying {
		m.mu.Unlock()
		log.Println("[chromecast] athan already playing, skipping")
		return nil
	}
	m.athanPlaying = true
	baseURL := m.mediaBaseURL
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.athanPlaying = false
		m.mu.Unlock()
	}()

	if filename == "" {
		if prayer == "Fajr" {
			filename = "media_adhan_al_fajr.mp3"
		} else {
			filename = "media_Athan.mp3"
		}
	}

	if baseURL == "" {
		return fmt.Errorf("media base URL not set — call SetMediaBaseURL first")
	}

	url := baseURL + filename
	log.Printf("[chromecast] PlayAthan: prayer=%s file=%s full_url=%s", prayer, filename, url)
	return m.playURL(url, "audio/mpeg")
}

// PlayURL plays any URL on the configured Chromecast device.
func (m *Manager) PlayURL(url, contentType string) error {
	return m.playURL(url, contentType)
}

// PlayURLOnDevice plays url on the named device regardless of the Manager's
// configured target. The device must already be in the cached device list or
// reachable via mDNS. Updates m.app so StopPlayback() stops this device.
func (m *Manager) PlayURLOnDevice(deviceName, url, contentType string) error {
	devs, err := m.Discover(false)
	if err != nil {
		return fmt.Errorf("discover: %w", err)
	}
	var target *Device
	for i := range devs {
		if equalFold(devs[i].Name, deviceName) {
			target = &devs[i]
			break
		}
	}
	if target == nil {
		// Try a forced re-scan once.
		devs, _ = m.Discover(true)
		for i := range devs {
			if equalFold(devs[i].Name, deviceName) {
				target = &devs[i]
				break
			}
		}
	}
	if target == nil {
		return fmt.Errorf("device %q not found", deviceName)
	}

	app, err := m.connectWithRetry(*target)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", target.Name, err)
	}
	m.mu.Lock()
	m.app = app
	m.currentDevice = target
	m.mu.Unlock()

	log.Printf("[chromecast] PlayURLOnDevice: %s url=%s", target.Name, url)
	if err := app.Load(url, 0, contentType, false, true, false); err != nil {
		return fmt.Errorf("load on %s: %w", target.Name, err)
	}
	return nil
}

// PlayQuranStream plays the default live Quran recitation stream.
func (m *Manager) PlayQuranStream() error {
	log.Printf("[chromecast] starting Quran stream: %s", QuranStation["url"])
	return m.playURL(QuranStation["url"], "audio/mpeg")
}

// StopPlayback stops currently playing media.
func (m *Manager) StopPlayback() error {
	m.mu.Lock()
	app := m.app
	m.mu.Unlock()

	if app == nil {
		return nil
	}
	return app.Stop()
}

// IsAthanPlaying reports whether an Athan is currently playing.
func (m *Manager) IsAthanPlaying() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.athanPlaying
}

// --- internal ---

func (m *Manager) playURL(url, contentType string) error {
	dev, err := m.resolveDevice()
	if err != nil {
		return fmt.Errorf("resolve device: %w", err)
	}

	app, err := m.connectWithRetry(dev)
	if err != nil {
		// Cast Group ports are dynamic and can change between discovery and
		// connection. Force a fresh scan and retry once with the new address.
		log.Printf("[chromecast] connect to %q failed, rediscovering...", dev.Name)
		if fresh, ferr := m.rediscoverByName(dev.Name); ferr == nil {
			log.Printf("[chromecast] retrying %q at %s:%d", fresh.Name, fresh.Host, fresh.Port)
			app, err = m.connectWithRetry(fresh)
			dev = fresh
		}
		// If the Cast Group port is still unreachable, fall back to the host
		// device's standard port (8009). Cast Groups have dynamic ports; the
		// physical Nest Hub / speaker at the same IP is usually still reachable.
		if err != nil && dev.Port != 8009 {
			fallback := Device{
				UUID:      dev.UUID,
				Name:      dev.Name,
				Host:      dev.Host,
				Port:      8009,
				ModelName: dev.ModelName,
			}
			log.Printf("[chromecast] Cast Group unreachable — falling back to %s:8009", dev.Host)
			app, err = m.connectWithRetry(fallback)
			if err == nil {
				dev = fallback
			}
		}
		if err != nil {
			return fmt.Errorf("connect to %s: %w", dev.Name, err)
		}
	}

	m.mu.Lock()
	m.app = app
	m.mu.Unlock()

	log.Printf("[chromecast] sending Load to %s (%s:%d) url=%s", dev.Name, dev.Host, dev.Port, url)
	// detach=true so we return immediately without waiting for playback to finish
	if err := app.Load(url, 0, contentType, false, true, false); err != nil {
		return fmt.Errorf("load media on %s: %w", dev.Name, err)
	}
	log.Printf("[chromecast] Load command accepted by %s", dev.Name)
	return nil
}

// rediscoverByName forces a fresh mDNS scan and returns the device matching
// name. Used to recover from stale Cast Group ports after a connect failure.
func (m *Manager) rediscoverByName(name string) (Device, error) {
	devs, err := m.Discover(true)
	if err != nil {
		return Device{}, err
	}
	for _, d := range devs {
		if equalFold(d.Name, name) {
			return d, nil
		}
	}
	return Device{}, fmt.Errorf("device %q not found after rediscovery", name)
}

func (m *Manager) resolveDevice() (Device, error) {
	m.mu.Lock()
	target := m.deviceName
	m.mu.Unlock()

	// Use the in-memory cache first so we never block on an mDNS scan at
	// prayer time. Only scan if the cache is empty or the target isn't found.
	devs := m.Devices()
	if len(devs) == 0 {
		var err error
		devs, err = m.Discover(false)
		if err != nil {
			return Device{}, err
		}
		if len(devs) == 0 {
			devs, err = m.Discover(true)
			if err != nil || len(devs) == 0 {
				return Device{}, fmt.Errorf("no Chromecast devices found")
			}
		}
	}

	// Match configured name (case-insensitive)
	if target != "" {
		for _, d := range devs {
			if equalFold(d.Name, target) {
				return d, nil
			}
		}
		// Not in cache — force a fresh scan once
		devs, _ = m.Discover(true)
		for _, d := range devs {
			if equalFold(d.Name, target) {
				return d, nil
			}
		}
		return Device{}, fmt.Errorf("device %q not found", target)
	}

	// No name configured: prefer "Adahn", then first available
	for _, d := range devs {
		if equalFold(d.Name, "Adahn") {
			return d, nil
		}
	}
	return devs[0], nil
}

func (m *Manager) connectWithRetry(dev Device) (*application.Application, error) {
	for i := 0; i < maxRetries; i++ {
		app := application.NewApplication()
		if err := app.Start(dev.Host, dev.Port); err == nil {
			return app, nil
		} else {
			log.Printf("[chromecast] connect attempt %d/%d failed: %v", i+1, maxRetries, err)
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("failed to connect after %d attempts", maxRetries)
}

func (m *Manager) deviceList() []Device {
	out := make([]Device, 0, len(m.devices))
	for _, d := range m.devices {
		out = append(out, d)
	}
	return out
}

// IsAvailable does a TCP probe to check if a device is reachable.
func IsAvailable(host string, port int) bool {
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func equalFold(a, b string) bool {
	return len(a) == len(b) && foldEqual(a, b)
}

func foldEqual(a, b string) bool {
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}
