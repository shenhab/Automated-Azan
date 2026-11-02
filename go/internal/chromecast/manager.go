package chromecast

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vishen/go-chromecast/application"
	castdns "github.com/vishen/go-chromecast/dns"

	"azan-agent/internal/trace"
)

const (
	discoveryCooldown = 60 * time.Second
	connectTimeout    = 10 * time.Second
	maxRetries        = 3
)

// Timing knobs. They are variables only so tests can shorten them; production
// code never changes them.
var (
	backoffRetryInterval = 30 * time.Second // wait between backoff attempts
	backoffMaxWait       = 3 * time.Minute  // total time to keep retrying
	connectRetryPause    = 2 * time.Second  // pause between connect attempts
	mdnsTimeout          = 8 * time.Second  // how long one mDNS scan listens

	// Playback verification (see verifyPlayback).
	verifyStartTimeout = 10 * time.Second       // how long the speaker has to start playing
	verifyStablePeriod = 2 * time.Second        // it must keep playing this long to count
	verifyPollEvery    = 500 * time.Millisecond // how often its state is read
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

	mu             sync.Mutex
	knownMu        sync.Mutex
	known          map[string][]knownMember // who has been seen in each group (see known_members.go)
	lastMembership Membership
	hasMembership  bool
	devices        map[string]Device // uuid → Device
	lastDiscovery  time.Time
	app            *application.Application
	currentDevice  *Device
	playingDev     Device // the speaker the current playback is on

	// Pre-connect cache populated by PreConnect; consumed by the next playURL.
	preApp *application.Application
	preDev *Device

	athanPlaying bool
	quranPlaying bool

	lastPlayback Playback // what the speaker reported after the latest play request

	// static devices are always merged into discovery results (see
	// parseStaticDevices); they let speakers be addressed directly where mDNS
	// does not work (other VLANs, containers, CI).
	static []Device
}

// NewManager creates a Manager targeting the named device.
func NewManager(deviceName string) *Manager {
	return &Manager{
		deviceName: deviceName,
		devices:    make(map[string]Device),
		static:     parseStaticDevices(os.Getenv("AZAN_STATIC_DEVICES")),
	}
}

// parseStaticDevices parses AZAN_STATIC_DEVICES: a ';'-separated list of
// "Name=host:port" entries, e.g. "Kitchen=192.168.2.10:8009;Hall=10.0.0.5:8009".
// Malformed entries are logged and skipped.
func parseStaticDevices(spec string) []Device {
	var out []Device
	for _, entry := range strings.Split(spec, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		name, addr, ok := strings.Cut(entry, "=")
		name = strings.TrimSpace(name)
		host, portStr, err := net.SplitHostPort(strings.TrimSpace(addr))
		port, perr := strconv.Atoi(portStr)
		if !ok || name == "" || err != nil || perr != nil || port <= 0 || port > 65535 {
			log.Printf("[chromecast] ignoring malformed AZAN_STATIC_DEVICES entry %q (want Name=host:port)", entry)
			continue
		}
		out = append(out, Device{
			UUID:      "static-" + strings.ToLower(strings.ReplaceAll(name, " ", "-")),
			Name:      name,
			Host:      host,
			Port:      port,
			ModelName: "Static device",
		})
	}
	return out
}

// SetCacheDir sets the directory used to persist the device cache.
// Call before LoadCache and Discover.
func (m *Manager) SetCacheDir(dir string) {
	m.mu.Lock()
	m.cacheDir = dir
	m.mu.Unlock()
}

// StartHealthMonitor starts a background goroutine that probes the configured
// speaker's TCP port every interval. When the port is unreachable (common for
// Cast Groups whose dynamic port goes stale), it triggers a forced mDNS
// rediscovery so the cache always holds a live address before prayer time.
func (m *Manager) StartHealthMonitor(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			m.probeAndRefresh()
		}
	}()
	log.Printf("[chromecast/health] monitor started (interval %v)", interval)
}

func (m *Manager) probeAndRefresh() {
	m.mu.Lock()
	target := m.deviceName
	m.mu.Unlock()
	if target == "" {
		return
	}

	var found *Device
	for _, d := range m.Devices() {
		d := d
		if equalFold(d.Name, target) {
			found = &d
			break
		}
	}
	if found == nil {
		log.Printf("[chromecast/health] %q not in cache — skipping probe", target)
		return
	}

	if IsAvailable(found.Host, found.Port) {
		return // port is alive, nothing to do
	}

	log.Printf("[chromecast/health] %q at %s:%d unreachable — forcing rediscovery",
		found.Name, found.Host, found.Port)
	devs, err := m.Discover(true)
	if err != nil {
		log.Printf("[chromecast/health] rediscovery error: %v", err)
		return
	}
	for _, d := range devs {
		if equalFold(d.Name, target) {
			if d.Host != found.Host || d.Port != found.Port {
				log.Printf("[chromecast/health] %q updated: %s:%d → %s:%d",
					target, found.Host, found.Port, d.Host, d.Port)
			} else {
				log.Printf("[chromecast/health] %q still at %s:%d after rediscovery",
					target, d.Host, d.Port)
			}
			return
		}
	}
	log.Printf("[chromecast/health] %q not found after rediscovery", target)
}

// PreConnect establishes a connection to the configured speaker and caches it
// so the next playURL call can skip the connection step entirely.
// Called by the scheduler ~60 s before each prayer.
func (m *Manager) PreConnect() error {
	dev, err := m.resolveDevice()
	if err != nil {
		return fmt.Errorf("pre-connect resolve: %w", err)
	}
	app, err := m.connectWithRetry(dev)
	if err != nil {
		return fmt.Errorf("pre-connect to %q: %w", dev.Name, err)
	}
	m.mu.Lock()
	m.preApp = app
	devCopy := dev
	m.preDev = &devCopy
	m.mu.Unlock()
	log.Printf("[chromecast] pre-connected to %q (%s:%d)", dev.Name, dev.Host, dev.Port)
	trace.Agent("pre-connect", dev.Name, fmt.Sprintf("connected to %s (%s:%d) ahead of the Athan", dev.Name, dev.Host, dev.Port),
		"host", dev.Host, "port", dev.Port)
	return nil
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
	ctx, cancel := context.WithTimeout(context.Background(), mdnsTimeout)
	defer cancel()

	found := make(map[string]Device)
	var ch <-chan castdns.CastEntry
	var err error
	if os.Getenv("AZAN_DISABLE_MDNS") != "" {
		// Static devices only: never scan the network (used by tests, and by
		// anyone who wants the agent to talk to exactly the speakers they list).
		err = fmt.Errorf("mDNS disabled by AZAN_DISABLE_MDNS")
	} else {
		ch, err = castdns.DiscoverCastDNSEntries(ctx, nil)
	}
	if err != nil {
		if len(m.static) == 0 {
			return nil, fmt.Errorf("mDNS discover: %w", err)
		}
		log.Printf("[chromecast] mDNS discover skipped (%v) — using static devices only", err)
	} else {
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
	}
	for _, d := range m.static {
		found[d.UUID] = d
		log.Printf("[chromecast] static device: %s at %s:%d", d.Name, d.Host, d.Port)
	}

	m.mu.Lock()
	m.devices = found
	m.lastDiscovery = time.Now()
	cacheDir := m.cacheDir
	m.mu.Unlock()

	go m.saveCache(cacheDir)

	return m.deviceList(), nil
}

// SpeakerDevice returns the speaker the manager plays on, from its device cache
// when it can (no network scan at prayer time).
func (m *Manager) SpeakerDevice() (Device, error) { return m.resolveDevice() }

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
	dev := m.playingDev
	m.mu.Unlock()

	if app == nil {
		return nil
	}
	trace.Agent("stop-sent", dev.Name, "asked "+dev.Name+" to stop", "host", dev.Host)
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
	// Use the pre-cached connection if one was established by PreConnect.
	m.mu.Lock()
	preApp := m.preApp
	preDev := m.preDev
	m.preApp = nil
	m.preDev = nil
	m.mu.Unlock()

	if preApp != nil && preDev != nil {
		log.Printf("[chromecast] using pre-connected session for %q (%s:%d) url=%s",
			preDev.Name, preDev.Host, preDev.Port, url)
		// A lost reply (see loadWithRetry) does not make the session stale: the speaker
		// has already done what was asked, so asking again on the same warm session
		// normally works. Only if that fails too is it replaced.
		for attempt := 1; attempt <= preConnectAttempts; attempt++ {
			if attempt > 1 {
				trace.Warn(trace.SourceAgent, "retry", preDev.Name, "asking again on the pre-connected session", "host", preDev.Host)
			}
			pb, err := m.loadAndVerify(preApp, *preDev, url, contentType)
			if err == nil {
				m.mu.Lock()
				m.app = preApp
				m.playingDev = *preDev
				m.mu.Unlock()
				m.recordPlayback(pb, "pre-connect")
				return nil
			}
			log.Printf("[chromecast] playback on the pre-connected session failed (attempt %d/%d): %v", attempt, preConnectAttempts, err)
		}
		preApp.Close(false)
		log.Printf("[chromecast] pre-connect session stale for %q — reconnecting", preDev.Name)
		trace.Warn(trace.SourceAgent, "retry", preDev.Name, "the pre-connected session did not work — reconnecting", "host", preDev.Host)
	}

	dev, err := m.resolveDevice()
	if err != nil {
		trace.Error(trace.SourceAgent, "play-failed", "", "cannot find the speaker: "+err.Error(), "error", err.Error())
		return fmt.Errorf("resolve device: %w", err)
	}

	app, err := m.connectWithRetry(dev)
	if err != nil {
		// Cast Group ports are dynamic and can go stale. Enter a backoff retry
		// loop with forced rediscovery between attempts so the Athan plays on
		// the full group once the group's port recovers (usually within 1–2 min).
		log.Printf("[chromecast] connect to %q failed — entering backoff retry loop (max %v)",
			dev.Name, backoffMaxWait)
		var backoffDev Device
		app, backoffDev, err = m.connectWithBackoff(dev.Name)
		if err != nil {
			trace.Error(trace.SourceAgent, "play-failed", dev.Name, "cannot connect to the speaker: "+err.Error(), "error", err.Error(), "host", dev.Host)
			return fmt.Errorf("connect to %s: %w", dev.Name, err)
		}
		dev = backoffDev
	}

	log.Printf("[chromecast] sending Load to %s (%s:%d) url=%s", dev.Name, dev.Host, dev.Port, url)
	app, pb, err := m.loadWithRetry(dev, app, url, contentType)
	if err != nil {
		m.mu.Lock()
		m.lastPlayback = pb
		m.mu.Unlock()
		trace.Error(trace.SourceAgent, "play-failed", dev.Name, "could not play on "+dev.Name+": "+err.Error(), "error", err.Error(), "host", dev.Host)
		return fmt.Errorf("load media on %s: %w", dev.Name, err)
	}
	m.mu.Lock()
	m.app = app
	m.playingDev = dev
	m.mu.Unlock()
	m.recordPlayback(pb, "")
	return nil
}

// loadAttempts is how many times a LOAD is tried before giving up.
const loadAttempts = 3

// preConnectAttempts is how many times a LOAD is tried on the warm pre-connected
// session before it is replaced by a fresh connection.
const preConnectAttempts = 2

// loadWithRetry plays url on the connected speaker and checks that it really
// started (see verifyPlayback). If a request is lost, ignored, rejected or the
// playback dies at once, it reconnects and tries again, up to loadAttempts times.
// (A lost reply is a known race in the client library, which registers its
// listener after sending; it surfaces as a 5-second timeout and would otherwise
// cost a whole Athan.) It returns the connection that succeeded and what the
// speaker reported.
func (m *Manager) loadWithRetry(dev Device, app *application.Application, url, contentType string) (*application.Application, Playback, error) {
	var (
		pb  Playback
		err error
	)
	for attempt := 1; attempt <= loadAttempts; attempt++ {
		if attempt > 1 {
			trace.Warn(trace.SourceAgent, "retry", dev.Name, fmt.Sprintf("trying again (attempt %d of %d)", attempt, loadAttempts), "host", dev.Host)
		}
		if pb, err = m.loadAndVerify(app, dev, url, contentType); err == nil {
			return app, pb, nil
		}
		log.Printf("[chromecast] play attempt %d/%d on %s failed: %v", attempt, loadAttempts, dev.Name, err)
		if attempt == loadAttempts {
			break
		}
		app.Close(false)
		time.Sleep(connectRetryPause)
		fresh, cerr := m.connectWithRetry(dev)
		if cerr != nil {
			return nil, pb, fmt.Errorf("%w (and reconnecting failed: %v)", err, cerr)
		}
		app = fresh
	}
	return nil, pb, err
}

// Playback is what a speaker reported after being asked to play something.
type Playback struct {
	Speaker    string  `json:"speaker"`
	Started    bool    `json:"started"`     // it reported PLAYING/BUFFERING and kept playing
	State      string  `json:"state"`       // PLAYING, BUFFERING, IDLE, ...
	IdleReason string  `json:"idle_reason"` // e.g. ERROR, when State is IDLE
	Muted      bool    `json:"muted"`
	Level      float32 `json:"level"` // 0..1
	Warning    string  `json:"warning,omitempty"`
	// Members are the devices the speaker (a Cast group) is made of, each with
	// its own volume; empty for a plain speaker.
	Members []GroupMember `json:"members,omitempty"`
	At      time.Time     `json:"at"`
	Session int           `json:"-"` // the media session that is ours; 0 if unknown
}

// LastPlayback returns what the speaker reported after the latest play request.
func (m *Manager) LastPlayback() Playback {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastPlayback
}

func (m *Manager) recordPlayback(pb Playback, via string) {
	m.mu.Lock()
	m.lastPlayback = pb
	m.mu.Unlock()
	if via != "" {
		via = " (" + via + ")"
	}
	log.Printf("[chromecast] %s is %s%s — volume %.2f, muted=%v", pb.Speaker, pb.State, via, pb.Level, pb.Muted)
	if pb.Warning != "" {
		log.Printf("[chromecast] WARNING: %s", pb.Warning)
	}
}

// loadAndVerify asks the speaker to play url and then checks that it did.
func (m *Manager) loadAndVerify(app *application.Application, dev Device, url, contentType string) (Playback, error) {
	speaker := dev.Name
	baseline := 0 // the media session already on the speaker, if any, is not ours
	if md := app.Media(); md != nil {
		baseline = md.MediaSessionId
	}
	trace.Agent("load-sent", speaker, "asked "+speaker+" to play "+path.Base(url), "host", dev.Host, "port", dev.Port, "url", url)
	// detach=true so we return immediately without waiting for playback to finish
	if err := app.Load(url, 0, contentType, false, true, false); err != nil {
		trace.Warn(trace.SourceAgent, "attempt-failed", speaker, "the request was not accepted: "+err.Error(), "error", err.Error(), "host", dev.Host)
		return Playback{Speaker: speaker}, err
	}
	pb, err := verifyPlayback(app, speaker, baseline)
	if err != nil {
		trace.Warn(trace.SourceAgent, "attempt-failed", speaker, "the speaker accepted the request but did not start playing: "+err.Error(), "error", err.Error(), "host", dev.Host)
		return pb, fmt.Errorf("the speaker accepted the request but did not start playing: %w", err)
	}
	level := trace.Event{}.Level
	if pb.Warning != "" {
		level = "warn"
	}
	trace.Emit(trace.Event{Source: trace.SourceAgent, Kind: "verified", Device: speaker, Level: level,
		Text:   fmt.Sprintf("%s is %s — volume %.2f%s", speaker, pb.State, pb.Level, mutedWord(pb.Muted)),
		Fields: map[string]any{"state": pb.State, "volume": float64(pb.Level), "muted": pb.Muted, "warning": pb.Warning, "host": dev.Host}})
	return pb, nil
}

// checkMembers looks at the devices a Cast group is made of. The group's own
// volume says nothing about them: one member can be muted or at volume 0 while
// the group reports itself audible, and the Athan then plays everywhere but
// there. Best effort: a group that cannot be asked adds nothing.
//
// It is a separate step, run after a verified play (CheckGroupMembers), so the
// Athan's own path keeps its single warm connection.
func (m *Manager) checkMembers(dev Device, pb *Playback) {
	members, err := GroupMembers(dev.Host, dev.Port)
	if err != nil {
		log.Printf("[chromecast] could not list the members of %q: %v", dev.Name, err)
		return
	}
	if len(members) == 1 && strings.EqualFold(members[0].Name, dev.Name) {
		return // a plain device lists itself
	}
	pb.Members = members
	var warns []string
	for _, mb := range members {
		switch {
		case mb.Muted:
			warns = append(warns, fmt.Sprintf("%s (in %s) is MUTED — it will not be heard", mb.Name, dev.Name))
		case mb.Level < 0.02:
			warns = append(warns, fmt.Sprintf("%s (in %s) has its volume at %.2f — it will not be heard", mb.Name, dev.Name, mb.Level))
		}
	}
	if len(warns) > 0 {
		if pb.Warning != "" {
			warns = append([]string{pb.Warning}, warns...)
		}
		pb.Warning = strings.Join(warns, "; ")
	}
}

// CheckGroupMembers looks at the members of the speaker's group after the
// Athan started, adds them to the last playback report and warns (in the log,
// the timeline and the report) about a member that is muted or at volume 0.
// Best effort and read-only; call it in the background.
func (m *Manager) CheckGroupMembers() {
	dev, err := m.resolveDevice()
	if err != nil {
		return
	}
	m.mu.Lock()
	pb := m.lastPlayback
	m.mu.Unlock()
	before := pb.Warning
	m.checkMembers(dev, &pb)
	if pb.Members == nil {
		return
	}
	m.mu.Lock()
	m.lastPlayback.Members, m.lastPlayback.Warning = pb.Members, pb.Warning
	m.mu.Unlock()
	if pb.Warning != before {
		log.Printf("[chromecast] WARNING: %s", pb.Warning)
		trace.Warn(trace.SourceAgent, "member-silent", dev.Name, pb.Warning, "host", dev.Host)
	}
}

// verifyPlayback asks the speaker, until it answers, what it is doing. Playback
// counts as started once a NEW media session (not the one that was there before)
// reports PLAYING or BUFFERING and keeps doing so for verifyStablePeriod. It fails
// at once if the speaker reports an error, stops, or pauses, and after
// verifyStartTimeout if nothing starts. A speaker that is playing but muted, or at
// volume 0, is flagged in the report (the Athan will start but not be heard).
func verifyPlayback(app *application.Application, speaker string, baseline int) (Playback, error) {
	pb := Playback{Speaker: speaker}
	deadline := time.Now().Add(verifyStartTimeout)
	var playingSince time.Time
	for {
		if err := app.Update(); err != nil {
			return pb, fmt.Errorf("could not read the speaker's state: %w", err)
		}
		pb.At = time.Now()
		pb.State, pb.IdleReason = "", ""
		if v := app.Volume(); v != nil {
			pb.Muted, pb.Level = v.Muted, v.Level
		}
		if md := app.Media(); md != nil && md.MediaSessionId != baseline {
			pb.State, pb.IdleReason = md.PlayerState, md.IdleReason
			pb.Session = md.MediaSessionId
		}

		switch pb.State {
		case "PLAYING", "BUFFERING":
			if playingSince.IsZero() {
				playingSince = time.Now()
			}
			if time.Since(playingSince) >= verifyStablePeriod {
				pb.Started = true
				pb.Warning = silenceWarning(speaker, pb)
				return pb, nil
			}
		case "IDLE":
			switch {
			case pb.IdleReason == "FINISHED" && !playingSince.IsZero():
				pb.Started = true // it played, and was short enough to be over already
				pb.Warning = silenceWarning(speaker, pb)
				return pb, nil
			case pb.IdleReason != "":
				return pb, fmt.Errorf("the speaker went idle (%s)", pb.IdleReason)
			}
		case "PAUSED":
			return pb, fmt.Errorf("the speaker is paused")
		}

		if time.Now().After(deadline) {
			state := pb.State
			if state == "" {
				state = "no media"
			}
			return pb, fmt.Errorf("nothing was playing after %v (speaker state: %s)", verifyStartTimeout, state)
		}
		time.Sleep(verifyPollEvery)
	}
}

func mutedWord(m bool) string {
	if m {
		return " (muted)"
	}
	return ""
}

// silenceWarning explains why a speaker that is playing may still be inaudible.
func silenceWarning(speaker string, pb Playback) string {
	switch {
	case pb.Muted:
		return fmt.Sprintf("%s is playing but it is MUTED — the Athan will not be heard", speaker)
	case pb.Level < 0.02:
		return fmt.Sprintf("%s is playing but its volume is %.2f — the Athan will not be heard", speaker, pb.Level)
	}
	return ""
}

// connectWithBackoff retries connecting to the named device with forced mDNS
// rediscovery between each attempt, blocking for up to backoffMaxWait. Used
// when Cast Group dynamic ports go stale and refuse connections — the group
// typically recovers within 1–2 minutes.
func (m *Manager) connectWithBackoff(name string) (*application.Application, Device, error) {
	deadline := time.Now().Add(backoffMaxWait)
	for attempt := 1; ; attempt++ {
		devs, err := m.Discover(true)
		if err != nil {
			log.Printf("[chromecast] backoff attempt %d: discover error: %v", attempt, err)
		} else {
			for _, d := range devs {
				if equalFold(d.Name, name) {
					app, err := m.connectWithRetry(d)
					if err == nil {
						log.Printf("[chromecast] backoff attempt %d: connected to %q at %s:%d",
							attempt, d.Name, d.Host, d.Port)
						return app, d, nil
					}
					log.Printf("[chromecast] backoff attempt %d: %q at %s:%d unreachable: %v",
						attempt, d.Name, d.Host, d.Port, err)

					// Kick the host device at port 8009 — sending Stop may
					// force the Nest Hub to reinitialise the Cast Group receiver
					// on a new port, which the next discovery will find.
					if d.Port != 8009 {
						m.kickHost(d.Host)
					}
					break
				}
			}
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		wait := backoffRetryInterval
		if wait > remaining {
			wait = remaining
		}
		log.Printf("[chromecast] backoff: waiting %v before attempt %d (%.0fs remaining)",
			wait, attempt+1, remaining.Seconds())
		time.Sleep(wait)
	}
	return nil, Device{}, fmt.Errorf("could not connect to %q after %v", name, backoffMaxWait)
}

// kickHost connects to the physical device at host:8009 and sends a Stop
// command. This can revive a crashed Cast Group receiver on the same host.
func (m *Manager) kickHost(host string) {
	log.Printf("[chromecast] kicking host %s:8009 to revive Cast Group", host)
	hostDev := Device{Host: host, Port: 8009, Name: host + ":8009"}
	app := application.NewApplication()
	if err := app.Start(host, 8009); err != nil {
		log.Printf("[chromecast] kick connect to %s:8009 failed: %v", host, err)
		return
	}
	if err := app.Stop(); err != nil {
		log.Printf("[chromecast] kick Stop on %s:8009 failed: %v", hostDev.Name, err)
		return
	}
	log.Printf("[chromecast] kick sent to %s:8009 — waiting for Cast Group to reinitialise", host)
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
		time.Sleep(connectRetryPause)
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

// WaitForPlaybackEnd blocks until the playback described by pb is over, or until
// hold has passed, and says why it returned. It watches the speaker over a
// connection of its own, so it never disturbs the one that started the Athan,
// and it reconnects if that connection drops (a Wi-Fi blip is not the end of the
// Athan). The playback is over when the speaker has no app, no media, an idle
// player (finished, stopped, error) or a different media session (something
// else took over).
func (m *Manager) WaitForPlaybackEnd(pb Playback, hold time.Duration) string {
	deadline := time.Now().Add(hold)
	var app *application.Application
	defer func() {
		if app != nil {
			app.Close(false)
		}
	}()
	for time.Now().Before(deadline) {
		if app == nil {
			dev, err := m.resolveDevice()
			if err == nil {
				app, err = m.connectWithRetry(dev)
			}
			if err != nil {
				time.Sleep(releasePoll)
				continue
			}
		} else if err := updateWithin(app, pollTimeout); err != nil {
			// A stopped app answers nothing, and so does a dropped connection:
			// look again over a fresh connection, which sees the truth.
			app.Close(false)
			app = nil
			continue
		}
		if why := playbackOver(app, pb); why != "" {
			return why
		}
		time.Sleep(releasePoll)
	}
	return fmt.Sprintf("the %v hold limit was reached", hold)
}

func playbackOver(app *application.Application, pb Playback) string {
	if a := app.App(); a == nil || a.IsIdleScreen {
		return "the speaker has no app running"
	}
	md := app.Media()
	switch {
	case md == nil:
		return "the speaker has nothing playing"
	case pb.Session != 0 && md.MediaSessionId != pb.Session:
		return "something else started playing on the speaker"
	case md.PlayerState == "IDLE":
		if md.IdleReason != "" {
			return "playback ended (" + md.IdleReason + ")"
		}
		return "playback ended"
	}
	return ""
}

// pollTimeout bounds one status poll. The library retries a failing status
// request for ~10 s, far too long to sit on a dropped connection.
var pollTimeout = 4 * time.Second

// updateWithin is app.Update() with a time limit. On timeout the connection is
// closed, which also unblocks the abandoned call.
func updateWithin(app *application.Application, limit time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- app.Update() }()
	select {
	case err := <-done:
		return err
	case <-time.After(limit):
		app.Close(false)
		return fmt.Errorf("no answer from the speaker within %v", limit)
	}
}
