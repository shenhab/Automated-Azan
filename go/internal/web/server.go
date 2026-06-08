package web

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"azan-agent/internal/chromecast"
	"azan-agent/internal/config"
	"azan-agent/internal/hijri"
	"azan-agent/internal/media"
	"azan-agent/internal/prayer"
	"azan-agent/internal/quran"

	"github.com/gorilla/websocket"
)

//go:embed templates
var templateFS embed.FS

//go:embed favicon.ico
var faviconData []byte

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// QuranController controls Quran streaming from the web API layer.
// The concrete implementation lives in internal/quran to avoid import cycles.
type QuranController interface {
	StartSpeaker(dur time.Duration) error
	StartSpeakerOnDevice(deviceName string, dur time.Duration) error
	StopSpeaker()
	StartLocal(dur time.Duration) error
	StopLocal()
	Stop()
	Status() (speakerActive, localActive bool)
}

// Server is the web dashboard + REST API + WebSocket server.
type Server struct {
	cfg       *config.Config
	fetcher   *prayer.Fetcher
	scheduler *prayer.Scheduler
	castMgr   *chromecast.Manager
	quranCtrl QuranController
	tvPause   *chromecast.TVPauseManager
	mediaDir  string // local directory for user media files
	port      int

	mu      sync.Mutex
	clients map[*websocket.Conn]struct{}

	httpSrv *http.Server

	// Geo cache — proxied from CountriesNow
	geoMu         sync.Mutex
	geoCountries  []map[string]string // [{name, Iso2, Iso3}]
	geoCountriesAt time.Time
	geoCities      map[string][]string // country → city list
	geoCitiesAt    map[string]time.Time
}

// NewServer constructs the web server.
func NewServer(
	cfg *config.Config,
	fetcher *prayer.Fetcher,
	scheduler *prayer.Scheduler,
	castMgr *chromecast.Manager,
	quranCtrl QuranController,
	tvPause *chromecast.TVPauseManager,
	mediaDir string,
) (*Server, error) {
	srv := &Server{
		cfg:         cfg,
		fetcher:     fetcher,
		scheduler:   scheduler,
		castMgr:     castMgr,
		quranCtrl:   quranCtrl,
		tvPause:     tvPause,
		mediaDir:    mediaDir,
		clients:     make(map[*websocket.Conn]struct{}),
		geoCities:   make(map[string][]string),
		geoCitiesAt: make(map[string]time.Time),
	}
	// Sync fetcher with current config on startup.
	fetcher.SetAladhan(
		cfg.Prayer.Location == "aladhan",
		cfg.Prayer.AladhanCity,
		cfg.Prayer.AladhanCountry,
		cfg.Prayer.AladhanMethod,
	)
	return srv, nil
}

// Start begins serving on cfg.Web.Host:cfg.Web.Port.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Auth (unprotected)
	mux.HandleFunc("/setup", s.handleSetup)
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/logout", s.handleLogout)

	// Favicon — same icon as the system tray (unprotected so browsers can load it)
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/x-icon")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Write(faviconData)
	})

	// Pages (auth-protected)
	mux.HandleFunc("/", s.requireAuth(s.handleDashboard))
	mux.HandleFunc("/scheduler", s.requireAuth(s.handleScheduler))
	mux.HandleFunc("/chromecasts", s.requireAuth(s.handleChromecasts))
	mux.HandleFunc("/settings", s.requireAuth(s.handleSettings))
	mux.HandleFunc("/logs", s.requireAuth(s.handleLogs))

	// WebSocket (auth-protected)
	mux.HandleFunc("/ws", s.requireAuth(s.handleWS))

	// REST API (auth-protected)
	mux.HandleFunc("/api/config", s.requireAPIAuth(s.handleAPIConfig))
	mux.HandleFunc("/api/prayer-times", s.requireAPIAuth(s.handleAPIPrayerTimes))
	mux.HandleFunc("/api/scheduler-status", s.requireAPIAuth(s.handleAPISchedulerStatus))
	mux.HandleFunc("/api/devices", s.requireAPIAuth(s.handleAPIDevices))
	mux.HandleFunc("/api/discover-devices", s.requireAPIAuth(s.handleAPIDiscoverDevices))
	mux.HandleFunc("/api/play-athan", s.requireAPIAuth(s.handleAPIPlayAthan))
	mux.HandleFunc("/api/stop-playback", s.requireAPIAuth(s.handleAPIStopPlayback))
	mux.HandleFunc("/api/next-prayer", s.requireAPIAuth(s.handleAPINextPrayer))
	mux.HandleFunc("/api/quran/status", s.requireAPIAuth(s.handleAPIQuranStatus))
	mux.HandleFunc("/api/quran/play", s.requireAPIAuth(s.handleAPIQuranPlay))
	mux.HandleFunc("/api/quran/stop", s.requireAPIAuth(s.handleAPIQuranStop))
	mux.HandleFunc("/api/quran/stream-url", s.requireAPIAuth(s.handleAPIQuranStreamURL))
	mux.HandleFunc("/api/quran/stream", s.requireAPIAuth(s.handleAPIQuranStream))
	mux.HandleFunc("/api/media/files", s.requireAPIAuth(s.handleAPIMediaFiles))
	mux.HandleFunc("/api/media/upload", s.requireAPIAuth(s.handleAPIMediaUpload))
	mux.HandleFunc("/media/", s.requireAPIAuth(s.handleMediaServe))
	mux.HandleFunc("/api/hijri", s.requireAPIAuth(s.handleAPIHijri))
	mux.HandleFunc("/api/aladhan/methods", s.requireAPIAuth(s.handleAPIAladhanMethods))
	mux.HandleFunc("/api/geo/countries", s.requireAPIAuth(s.handleAPIGeoCountries))
	mux.HandleFunc("/api/geo/cities", s.requireAPIAuth(s.handleAPIGeoCities))
	mux.HandleFunc("/api/tv-pause/status", s.requireAPIAuth(s.handleAPITVPauseStatus))
	mux.HandleFunc("/api/tv-pause/config", s.requireAPIAuth(s.handleAPITVPauseConfig))
	mux.HandleFunc("/api/tv-pause/pause", s.requireAPIAuth(s.handleAPITVPausePause))
	mux.HandleFunc("/api/tv-pause/resume", s.requireAPIAuth(s.handleAPITVPauseResume))

	// Static files served from ../static relative to binary
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Try configured port, then fall back to the next 10 ports if occupied
	port := s.cfg.Web.Port
	var ln net.Listener
	for i := 0; i < 10; i++ {
		addr := fmt.Sprintf("%s:%d", s.cfg.Web.Host, port+i)
		var err error
		ln, err = net.Listen("tcp", addr)
		if err == nil {
			port = port + i
			break
		}
		log.Printf("[web] port %d in use, trying %d...", port+i, port+i+1)
	}
	if ln == nil {
		return fmt.Errorf("could not bind to any port in range %d-%d", s.cfg.Web.Port, s.cfg.Web.Port+9)
	}

	s.port = port
	s.httpSrv = &http.Server{Handler: accessLogger(mux)}

	go func() {
		log.Printf("[web] dashboard at http://localhost:%d", port)
		if err := s.httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("[web] server error: %v", err)
		}
	}()

	// Background goroutine: push updates to WebSocket clients every 30 seconds
	go s.broadcastLoop()
	return nil
}

// Port returns the actual port the server bound to (may differ from cfg if there was a conflict).
func (s *Server) Port() int {
	return s.port
}

// Stop shuts down the HTTP server.
func (s *Server) Stop() {
	if s.httpSrv != nil {
		s.httpSrv.Close()
	}
}

// Broadcast sends a JSON message to all connected WebSocket clients.
// BroadcastPrayerFired sends a prayer_fired WebSocket event to all connected
// browser clients so they can show a notification or play audio if enabled.
func (s *Server) BroadcastPrayerFired(prayerName string, browserNotify, browserAthan bool, audioURL string) {
	s.Broadcast("prayer_fired", map[string]interface{}{
		"prayer":         prayerName,
		"browser_notify": browserNotify,
		"browser_athan":  browserAthan,
		"audio_url":      audioURL,
	})
}

func (s *Server) Broadcast(event string, data interface{}) {
	msg, err := json.Marshal(map[string]interface{}{"event": event, "data": data})
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for conn := range s.clients {
		conn.WriteMessage(websocket.TextMessage, msg)
	}
}

// --- Page handlers ---

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	now := time.Now().In(s.fetcher.Timezone())
	times, _ := s.fetcher.Fetch(s.cfg.Prayer.Location, now, false)
	name, at, _ := s.scheduler.NextPrayer()

	hd, _ := hijri.Today() // nil on first-ever network failure; template handles nil

	s.renderPage(w, "dashboard.html", map[string]interface{}{
		"page":            "dashboard",
		"config":          s.cfg.AsWebDict(),
		"prayer_times":    times,
		"next_prayer":     map[string]interface{}{"name": name, "time": at.Format("15:04")},
		"jobs":            s.scheduler.Status(),
		"hijri":           hd,
		"quran_stream_url": quran.StreamURL,
	})
}

func (s *Server) handleScheduler(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, "scheduler.html", map[string]interface{}{
		"page":   "scheduler",
		"config": s.cfg.AsWebDict(),
		"jobs":   s.scheduler.Status(),
	})
}

func (s *Server) handleChromecasts(w http.ResponseWriter, r *http.Request) {
	devs := s.castMgr.Devices()
	s.renderPage(w, "chromecasts.html", map[string]interface{}{
		"page":         "devices",
		"config":       s.cfg.AsWebDict(),
		"devices":      devs,
		"tv_paused":    s.tvPause.IsPaused(),
		"tv_paused_n":  s.tvPause.PausedCount(),
	})
}

// notifRow describes one row in the Prayer Notifications table.
type notifRow struct {
	Key        string // config key prefix (e.g. "fajr", "pre_fajr")
	Label      string // display name
	HasEnabled bool   // show the Active toggle
	HasMedia   bool   // show the audio file select
	AudioLabel string // static label when HasMedia is false
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	rows := []notifRow{
		{Key: "fajr",        Label: "Fajr",               HasEnabled: true,  HasMedia: true},
		{Key: "dhuhr",       Label: "Dhuhr",              HasEnabled: true,  HasMedia: true},
		{Key: "asr",         Label: "Asr",                HasEnabled: true,  HasMedia: true},
		{Key: "maghrib",     Label: "Maghrib",            HasEnabled: true,  HasMedia: true},
		{Key: "isha",        Label: "Isha",               HasEnabled: true,  HasMedia: true},
		{Key: "pre_fajr",    Label: "Pre-Fajr Quran",     HasEnabled: false, HasMedia: false, AudioLabel: "Quran stream"},
		{Key: "friday_kahf", Label: "Friday Surah Al-Kahf", HasEnabled: false, HasMedia: false, AudioLabel: "Kahf (built-in)"},
	}
	s.renderPage(w, "settings.html", map[string]interface{}{
		"page":            "settings",
		"config":          s.cfg.AsWebDict(),
		"notifRows":       rows,
		"aladhan_methods": prayer.AladhanMethods,
		"tv_paused":       s.tvPause.IsPaused(),
		"tv_paused_n":     s.tvPause.PausedCount(),
	})
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	tail := readLogTail(s.cfg.Log.FilePath, 200)
	s.renderPage(w, "logs.html", map[string]interface{}{
		"page":   "logs",
		"config": s.cfg.AsWebDict(),
		"logs":   tail,
	})
}

// --- WebSocket ---

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[web/ws] upgrade error: %v", err)
		return
	}

	s.mu.Lock()
	s.clients[conn] = struct{}{}
	s.mu.Unlock()

	log.Printf("[web/ws] client connected from %s", realIP(r))
	defer func() {
		s.mu.Lock()
		delete(s.clients, conn)
		s.mu.Unlock()
		conn.Close()
		log.Printf("[web/ws] client disconnected from %s", realIP(r))
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (s *Server) broadcastLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		name, at, _ := s.scheduler.NextPrayer()
		s.Broadcast("scheduler_update", map[string]interface{}{
			"jobs":       s.scheduler.Status(),
			"next_prayer": map[string]interface{}{"name": name, "time": at.Format("15:04")},
		})
	}
}

// --- REST API ---

func (s *Server) handleAPIConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]interface{}{"success": true, "config": s.cfg.AsWebDict()})

	case http.MethodPost:
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, errResp(err))
			return
		}
		// Main config fields
		if v, ok := body["speakers_group_name"].(string); ok {
			s.cfg.Speaker.GroupName = v
		}
		if v, ok := body["location"].(string); ok {
			s.cfg.Prayer.Location = v
		}
		if v, ok := body["aladhan_city"].(string); ok {
			s.cfg.Prayer.AladhanCity = v
		}
		if v, ok := body["aladhan_country"].(string); ok {
			s.cfg.Prayer.AladhanCountry = v
		}
		if v, ok := body["aladhan_method"].(float64); ok {
			s.cfg.Prayer.AladhanMethod = int(v)
		}
		if v, ok := body["pre_fajr_enabled"].(bool); ok {
			s.cfg.Prayer.PreFajrEnabled = v
		}
		if v, ok := body["pre_fajr_minutes"].(float64); ok {
			s.cfg.Prayer.PreFajrMinutes = int(v)
		}
		if v, ok := body["friday_kahf_enabled"].(bool); ok {
			s.cfg.Prayer.FridayKahfEnabled = v
		}
		// Per-type speaker overrides (empty string = use default)
		if v, ok := body["athan_speaker"].(string); ok {
			s.cfg.Speaker.AthanSpeaker = v
		}
		if v, ok := body["pre_fajr_speaker"].(string); ok {
			s.cfg.Speaker.PreFajrSpeaker = v
		}
		if v, ok := body["friday_kahf_speaker"].(string); ok {
			s.cfg.Speaker.FridayKahfSpeaker = v
		}
		if v, ok := body["quran_speaker"].(string); ok {
			s.cfg.Speaker.QuranSpeaker = v
		}
		// Per-prayer enabled flags
		setBool := func(field *bool, key string) {
			if v, ok := body[key].(bool); ok {
				*field = v
			}
		}
		setBool(&s.cfg.Prayer.Enabled.Fajr,    "fajr_enabled")
		setBool(&s.cfg.Prayer.Enabled.Dhuhr,   "dhuhr_enabled")
		setBool(&s.cfg.Prayer.Enabled.Asr,     "asr_enabled")
		setBool(&s.cfg.Prayer.Enabled.Maghrib, "maghrib_enabled")
		setBool(&s.cfg.Prayer.Enabled.Isha,    "isha_enabled")
		// Per-prayer media files
		setStr := func(field *string, key string) {
			if v, ok := body[key].(string); ok {
				*field = v
			}
		}
		setStr(&s.cfg.Prayer.Media.Fajr,    "fajr_media")
		setStr(&s.cfg.Prayer.Media.Dhuhr,   "dhuhr_media")
		setStr(&s.cfg.Prayer.Media.Asr,     "asr_media")
		setStr(&s.cfg.Prayer.Media.Maghrib, "maghrib_media")
		setStr(&s.cfg.Prayer.Media.Isha,    "isha_media")
		// Per-job notification channels (ch_ prefix avoids collision with speaker-device overrides)
		setBool(&s.cfg.Prayer.Channels.Fajr.Speaker,              "ch_fajr_speaker")
		setBool(&s.cfg.Prayer.Channels.Fajr.Local,                "ch_fajr_local")
		setBool(&s.cfg.Prayer.Channels.Fajr.Notify,               "ch_fajr_notify")
		setBool(&s.cfg.Prayer.Channels.Fajr.BrowserNotify,        "ch_fajr_browser")
		setBool(&s.cfg.Prayer.Channels.Fajr.BrowserAthan,         "ch_fajr_browser_athan")
		setBool(&s.cfg.Prayer.Channels.Dhuhr.Speaker,             "ch_dhuhr_speaker")
		setBool(&s.cfg.Prayer.Channels.Dhuhr.Local,               "ch_dhuhr_local")
		setBool(&s.cfg.Prayer.Channels.Dhuhr.Notify,              "ch_dhuhr_notify")
		setBool(&s.cfg.Prayer.Channels.Dhuhr.BrowserNotify,       "ch_dhuhr_browser")
		setBool(&s.cfg.Prayer.Channels.Dhuhr.BrowserAthan,        "ch_dhuhr_browser_athan")
		setBool(&s.cfg.Prayer.Channels.Asr.Speaker,               "ch_asr_speaker")
		setBool(&s.cfg.Prayer.Channels.Asr.Local,                 "ch_asr_local")
		setBool(&s.cfg.Prayer.Channels.Asr.Notify,                "ch_asr_notify")
		setBool(&s.cfg.Prayer.Channels.Asr.BrowserNotify,         "ch_asr_browser")
		setBool(&s.cfg.Prayer.Channels.Asr.BrowserAthan,          "ch_asr_browser_athan")
		setBool(&s.cfg.Prayer.Channels.Maghrib.Speaker,           "ch_maghrib_speaker")
		setBool(&s.cfg.Prayer.Channels.Maghrib.Local,             "ch_maghrib_local")
		setBool(&s.cfg.Prayer.Channels.Maghrib.Notify,            "ch_maghrib_notify")
		setBool(&s.cfg.Prayer.Channels.Maghrib.BrowserNotify,     "ch_maghrib_browser")
		setBool(&s.cfg.Prayer.Channels.Maghrib.BrowserAthan,      "ch_maghrib_browser_athan")
		setBool(&s.cfg.Prayer.Channels.Isha.Speaker,              "ch_isha_speaker")
		setBool(&s.cfg.Prayer.Channels.Isha.Local,                "ch_isha_local")
		setBool(&s.cfg.Prayer.Channels.Isha.Notify,               "ch_isha_notify")
		setBool(&s.cfg.Prayer.Channels.Isha.BrowserNotify,        "ch_isha_browser")
		setBool(&s.cfg.Prayer.Channels.Isha.BrowserAthan,         "ch_isha_browser_athan")
		setBool(&s.cfg.Prayer.Channels.PreFajr.Speaker,           "ch_pre_fajr_speaker")
		setBool(&s.cfg.Prayer.Channels.PreFajr.Local,             "ch_pre_fajr_local")
		setBool(&s.cfg.Prayer.Channels.PreFajr.Notify,            "ch_pre_fajr_notify")
		setBool(&s.cfg.Prayer.Channels.PreFajr.BrowserNotify,     "ch_pre_fajr_browser")
		setBool(&s.cfg.Prayer.Channels.PreFajr.BrowserAthan,      "ch_pre_fajr_browser_athan")
		setBool(&s.cfg.Prayer.Channels.FridayKahf.Speaker,        "ch_friday_kahf_speaker")
		setBool(&s.cfg.Prayer.Channels.FridayKahf.Local,          "ch_friday_kahf_local")
		setBool(&s.cfg.Prayer.Channels.FridayKahf.Notify,         "ch_friday_kahf_notify")
		setBool(&s.cfg.Prayer.Channels.FridayKahf.BrowserNotify,  "ch_friday_kahf_browser")
		setBool(&s.cfg.Prayer.Channels.FridayKahf.BrowserAthan,   "ch_friday_kahf_browser_athan")
		if err := s.cfg.Save(); err != nil {
			writeJSON(w, errResp(err))
			return
		}
		// Keep fetcher in sync with any Aladhan setting changes.
		s.fetcher.SetAladhan(
			s.cfg.Prayer.Location == "aladhan",
			s.cfg.Prayer.AladhanCity,
			s.cfg.Prayer.AladhanCountry,
			s.cfg.Prayer.AladhanMethod,
		)
		writeJSON(w, map[string]interface{}{"success": true, "config": s.cfg.AsWebDict()})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAPIPrayerTimes(w http.ResponseWriter, r *http.Request) {
	now := time.Now().In(s.fetcher.Timezone())
	force := r.URL.Query().Get("force") == "true"
	times, err := s.fetcher.Fetch(s.cfg.Prayer.Location, now, force)
	if err != nil {
		writeJSON(w, errResp(err))
		return
	}
	writeJSON(w, map[string]interface{}{
		"success":      true,
		"prayer_times": times,
		"location":     s.cfg.Prayer.Location,
		"date":         now.Format("2006-01-02"),
	})
}

func (s *Server) handleAPISchedulerStatus(w http.ResponseWriter, r *http.Request) {
	name, at, ok := s.scheduler.NextPrayer()
	writeJSON(w, map[string]interface{}{
		"success":     true,
		"jobs":        s.scheduler.Status(),
		"next_prayer": map[string]interface{}{"name": name, "time": at.Format("15:04"), "found": ok},
	})
}

func (s *Server) handleAPIDevices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"success": true,
		"devices": s.castMgr.Devices(),
	})
}

func (s *Server) handleAPIDiscoverDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	devs, err := s.castMgr.Discover(true)
	if err != nil {
		writeJSON(w, errResp(err))
		return
	}
	writeJSON(w, map[string]interface{}{"success": true, "devices": devs, "count": len(devs)})
}

func (s *Server) handleAPIPlayAthan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Prayer   string `json:"prayer"`
		Filename string `json:"filename"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Prayer == "" {
		body.Prayer = "Dhuhr"
	}
	// Use configured media file if not explicitly overridden in request
	if body.Filename == "" {
		body.Filename = s.cfg.Prayer.Media.FileFor(body.Prayer)
	}
	if err := s.castMgr.PlayAthan(body.Prayer, body.Filename); err != nil {
		writeJSON(w, errResp(err))
		return
	}
	writeJSON(w, map[string]interface{}{"success": true, "prayer": body.Prayer, "filename": body.Filename})
}

func (s *Server) handleAPIStopPlayback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.castMgr.StopPlayback(); err != nil {
		writeJSON(w, errResp(err))
		return
	}
	writeJSON(w, map[string]interface{}{"success": true})
}

func (s *Server) handleAPIQuranStatus(w http.ResponseWriter, r *http.Request) {
	if s.quranCtrl == nil {
		writeJSON(w, map[string]interface{}{"success": true, "speaker_active": false, "local_active": false})
		return
	}
	spk, loc := s.quranCtrl.Status()
	writeJSON(w, map[string]interface{}{"success": true, "speaker_active": spk, "local_active": loc})
}

func (s *Server) handleAPIQuranStreamURL(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{"success": true, "url": quran.StreamURL})
}

// handleAPIQuranStream proxies the Quran radio stream through the local server
// so the browser can play it without CORS issues.
func (s *Server) handleAPIQuranStream(w http.ResponseWriter, r *http.Request) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, quran.StreamURL, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")
	if rng := r.Header.Get("Range"); rng != "" {
		req.Header.Set("Range", rng)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "stream unavailable: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Forward content-type and status; strip CORS headers so ours apply.
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "audio/mpeg")
	}
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body) //nolint:errcheck
}

func (s *Server) handleAPIQuranPlay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Target string `json:"target"` // "speaker"|"device"|"local"|"both" (default "speaker")
		Device string `json:"device"` // Chromecast device name when target=="device"
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Target == "" {
		body.Target = "speaker"
	}

	if s.quranCtrl == nil {
		writeJSON(w, map[string]interface{}{"success": false, "error": "quran controller not available"})
		return
	}
	switch body.Target {
	case "speaker":
		if err := s.quranCtrl.StartSpeaker(0); err != nil {
			writeJSON(w, errResp(err))
			return
		}
	case "device":
		if body.Device == "" {
			// No device name — fall back to default speaker.
			if err := s.quranCtrl.StartSpeaker(0); err != nil {
				writeJSON(w, errResp(err))
				return
			}
		} else {
			if err := s.quranCtrl.StartSpeakerOnDevice(body.Device, 0); err != nil {
				writeJSON(w, errResp(err))
				return
			}
		}
	case "local":
		if err := s.quranCtrl.StartLocal(0); err != nil {
			writeJSON(w, errResp(err))
			return
		}
	case "both":
		if err := s.quranCtrl.StartSpeaker(0); err != nil {
			writeJSON(w, errResp(err))
			return
		}
		if err := s.quranCtrl.StartLocal(0); err != nil {
			writeJSON(w, errResp(err))
			return
		}
	default:
		writeJSON(w, map[string]interface{}{"success": false, "error": "unknown target"})
		return
	}
	writeJSON(w, map[string]interface{}{"success": true, "target": body.Target, "device": body.Device})
}

func (s *Server) handleAPIQuranStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Target string `json:"target"` // "speaker", "local", or "all" (default "all")
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Target == "" {
		body.Target = "all"
	}

	if s.quranCtrl == nil {
		// Fallback: just stop all Chromecast playback
		s.castMgr.StopPlayback()
		writeJSON(w, map[string]interface{}{"success": true})
		return
	}
	switch body.Target {
	case "speaker":
		s.quranCtrl.StopSpeaker()
	case "local":
		s.quranCtrl.StopLocal()
	default:
		s.quranCtrl.Stop()
	}
	writeJSON(w, map[string]interface{}{"success": true, "target": body.Target})
}

// athanExclusions lists embedded files that are NOT Athan audio and should
// not appear in the per-prayer media selection dropdown.
var athanExclusions = map[string]bool{
	"kahf.mp3": true, // Friday Surah Al-Kahf — used internally, not for Athan
}

// handleMediaServe serves media files from the local media directory with embedded fallback.
// Used by browser-side Athan playback.
func (s *Server) handleMediaServe(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/media/")
	if name == "" || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	localPath := filepath.Join(s.mediaDir, name)
	if _, err := os.Stat(localPath); err == nil {
		http.ServeFile(w, r, localPath)
		return
	}
	f, err := media.EmbeddedFS.Open("embedded/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	stat, _ := f.Stat()
	rs, ok := f.(io.ReadSeeker)
	if !ok || stat == nil {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, name, stat.ModTime(), rs)
}

func (s *Server) handleAPIMediaFiles(w http.ResponseWriter, r *http.Request) {
	seen := map[string]bool{}
	var files []string

	addFile := func(name string) {
		if athanExclusions[name] || seen[name] {
			return
		}
		seen[name] = true
		files = append(files, name)
	}

	// List all embedded MP3 files (excluding internal-only ones)
	entries, _ := media.EmbeddedFS.ReadDir("embedded")
	for _, e := range entries {
		if !e.IsDir() && (strings.HasSuffix(e.Name(), ".mp3") || strings.HasSuffix(e.Name(), ".m4a")) {
			addFile(e.Name())
		}
	}

	// Add any MP3s from the local media directory (user-uploaded)
	if dirEntries, err := os.ReadDir(s.mediaDir); err == nil {
		for _, e := range dirEntries {
			if !e.IsDir() && (strings.HasSuffix(e.Name(), ".mp3") || strings.HasSuffix(e.Name(), ".m4a")) {
				addFile(e.Name())
			}
		}
	}

	writeJSON(w, map[string]interface{}{"success": true, "files": files})
}

func (s *Server) handleAPIMediaUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.ParseMultipartForm(50 << 20) // 50 MB limit
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, errResp(fmt.Errorf("no file in request: %w", err)))
		return
	}
	defer file.Close()

	name := filepath.Base(header.Filename)
	if !strings.HasSuffix(name, ".mp3") && !strings.HasSuffix(name, ".m4a") {
		writeJSON(w, errResp(fmt.Errorf("only .mp3 and .m4a files are allowed")))
		return
	}

	if err := os.MkdirAll(s.mediaDir, 0o755); err != nil {
		writeJSON(w, errResp(fmt.Errorf("could not create media dir: %w", err)))
		return
	}

	dest, err := os.Create(filepath.Join(s.mediaDir, name))
	if err != nil {
		writeJSON(w, errResp(fmt.Errorf("could not save file: %w", err)))
		return
	}
	defer dest.Close()

	n, err := io.Copy(dest, file)
	if err != nil {
		writeJSON(w, errResp(fmt.Errorf("upload failed: %w", err)))
		return
	}

	log.Printf("[web] media file uploaded: %s (%d bytes)", name, n)
	writeJSON(w, map[string]interface{}{"success": true, "filename": name, "bytes": n})
}

func (s *Server) handleAPIAladhanMethods(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"success": true,
		"methods": prayer.AladhanMethods,
	})
}

const countriesNowBase = "https://countriesnow.space/api/v0.1"
const geoCacheTTL = 24 * time.Hour

func (s *Server) handleAPIGeoCountries(w http.ResponseWriter, r *http.Request) {
	s.geoMu.Lock()
	if s.geoCountries != nil && time.Since(s.geoCountriesAt) < geoCacheTTL {
		countries := s.geoCountries
		s.geoMu.Unlock()
		writeJSON(w, map[string]interface{}{"success": true, "data": countries})
		return
	}
	s.geoMu.Unlock()

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(countriesNowBase + "/countries/iso")
	if err != nil {
		writeJSON(w, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	defer resp.Body.Close()

	var result struct {
		Data []map[string]string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		writeJSON(w, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	s.geoMu.Lock()
	s.geoCountries = result.Data
	s.geoCountriesAt = time.Now()
	s.geoMu.Unlock()

	writeJSON(w, map[string]interface{}{"success": true, "data": result.Data})
}

func (s *Server) handleAPIGeoCities(w http.ResponseWriter, r *http.Request) {
	country := strings.TrimSpace(r.URL.Query().Get("country"))
	if country == "" {
		writeJSON(w, map[string]interface{}{"success": false, "error": "country required"})
		return
	}

	s.geoMu.Lock()
	if cities, ok := s.geoCities[country]; ok && time.Since(s.geoCitiesAt[country]) < geoCacheTTL {
		s.geoMu.Unlock()
		writeJSON(w, map[string]interface{}{"success": true, "data": cities})
		return
	}
	s.geoMu.Unlock()

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(countriesNowBase + "/countries/cities/q?country=" + strings.ReplaceAll(country, " ", "%20"))
	if err != nil {
		writeJSON(w, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	defer resp.Body.Close()

	var result struct {
		Data []string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		writeJSON(w, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	s.geoMu.Lock()
	s.geoCities[country] = result.Data
	s.geoCitiesAt[country] = time.Now()
	s.geoMu.Unlock()

	writeJSON(w, map[string]interface{}{"success": true, "data": result.Data})
}

func (s *Server) handleAPIHijri(w http.ResponseWriter, r *http.Request) {
	hd, err := hijri.Today()
	if err != nil {
		writeJSON(w, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{
		"success":     true,
		"date":        hd.String(),
		"day":         hd.Day,
		"month":       hd.Month,
		"month_en":    hd.MonthEN,
		"month_ar":    hd.MonthAR,
		"year":        hd.Year,
		"weekday":     hd.Weekday,
		"is_ramadan":  hd.IsRamadan(),
		"ramadan_day": hd.RamadanDay(),
		"special_day": hd.SpecialDay(),
	})
}

func (s *Server) handleAPINextPrayer(w http.ResponseWriter, r *http.Request) {
	name, at, ok := s.scheduler.NextPrayer()
	writeJSON(w, map[string]interface{}{
		"success": true,
		"found":   ok,
		"prayer":  name,
		"time":    at.Format("15:04"),
	})
}

// --- TV pause API ---

func (s *Server) handleAPITVPauseStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"success":          true,
		"enabled":          s.cfg.TVPause.Enabled,
		"paused":           s.tvPause.IsPaused(),
		"paused_count":     s.tvPause.PausedCount(),
		"resume_delay_secs": s.cfg.TVPause.ResumeDelaySecs,
		"devices":          s.cfg.TVPause.Devices,
	})
}

func (s *Server) handleAPITVPauseConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Enabled         bool     `json:"enabled"`
		ResumeDelaySecs int      `json:"resume_delay_secs"`
		Devices         []string `json:"devices"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, errResp(err))
		return
	}
	s.cfg.TVPause.Enabled = body.Enabled
	if body.ResumeDelaySecs > 0 {
		s.cfg.TVPause.ResumeDelaySecs = body.ResumeDelaySecs
	}
	s.cfg.TVPause.Devices = body.Devices
	if err := s.cfg.Save(); err != nil {
		writeJSON(w, errResp(err))
		return
	}
	writeJSON(w, map[string]interface{}{"success": true})
}

func (s *Server) handleAPITVPausePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	delay := time.Duration(s.cfg.TVPause.ResumeDelaySecs) * time.Second
	if delay == 0 {
		delay = 5 * time.Minute
	}
	excludeSpeaker := s.cfg.Speaker.Resolve("athan")
	go func() {
		s.tvPause.PauseForAthan(s.cfg.TVPause.Devices, excludeSpeaker)
		s.tvPause.ScheduleResume(delay)
	}()
	writeJSON(w, map[string]interface{}{"success": true})
}

func (s *Server) handleAPITVPauseResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	go s.tvPause.ResumeAfterAthan()
	writeJSON(w, map[string]interface{}{"success": true})
}

// --- helpers ---

// realIP returns the client IP, preferring X-Real-IP then X-Forwarded-For
// (first entry) over the raw RemoteAddr, so logs show the true source IP
// when the app sits behind a reverse proxy.
func realIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if i := strings.Index(fwd, ","); i >= 0 {
			return strings.TrimSpace(fwd[:i])
		}
		return strings.TrimSpace(fwd)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.status = code
	rec.ResponseWriter.WriteHeader(code)
}

func (rec *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := rec.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not implement http.Hijacker")
	}
	return h.Hijack()
}

func accessLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("[web] %s %s %s %d %s", realIP(r), r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}

// renderPage parses base.html + the named page together each time.
// This avoids Go's "last define wins" problem — when all pages are parsed
// into one template set, the last {{define "content"}} overwrites all others,
// so every route renders the same page. Parsing per-request keeps exactly
// one "content" block in scope.
var tmplFuncs = template.FuncMap{
	"contains": strings.Contains,
	"lower":    strings.ToLower,
	"list": func(items ...string) []string { return items },
	// colorize wraps log lines in colour spans based on log level keywords
	"colorize": func(raw string) template.HTML {
		var sb strings.Builder
		for _, line := range strings.Split(raw, "\n") {
			lower := strings.ToLower(line)
			class := ""
			switch {
			case strings.Contains(lower, "error") || strings.Contains(lower, "fatal"):
				class = "log-error"
			case strings.Contains(lower, "warn"):
				class = "log-warn"
			case strings.Contains(lower, "✓") || strings.Contains(lower, "success") ||
				strings.Contains(lower, "started") || strings.Contains(lower, "saved"):
				class = "log-success"
			case strings.Contains(lower, "info") || strings.Contains(lower, "[main]") ||
				strings.Contains(lower, "[web]") || strings.Contains(lower, "[scheduler]"):
				class = "log-info"
			}
			escaped := template.HTMLEscapeString(line)
			if class != "" {
				sb.WriteString(`<span class="` + class + `">` + escaped + "</span>\n")
			} else {
				sb.WriteString(escaped + "\n")
			}
		}
		return template.HTML(sb.String())
	},
}

func (s *Server) renderPage(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	sub, err := fs.Sub(templateFS, "templates")
	if err != nil {
		http.Error(w, "template FS error", http.StatusInternalServerError)
		return
	}
	tmpl, err := template.New("").Funcs(tmplFuncs).ParseFS(sub, "base.html", name)
	if err != nil {
		log.Printf("[web] template parse error (%s): %v", name, err)
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("[web] template execute error (%s): %v", name, err)
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func errResp(err error) map[string]interface{} {
	return map[string]interface{}{"success": false, "error": err.Error()}
}

func readLogTail(path string, lines int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	all := strings.Split(string(data), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n")
}

