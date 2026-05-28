package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"azan-agent/internal/chromecast"
	"azan-agent/internal/config"
	"azan-agent/internal/prayer"

	"github.com/gorilla/websocket"
)

//go:embed templates
var templateFS embed.FS

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Server is the web dashboard + REST API + WebSocket server.
type Server struct {
	cfg       *config.Config
	fetcher   *prayer.Fetcher
	scheduler *prayer.Scheduler
	castMgr   *chromecast.Manager
	tmpl      *template.Template
	port      int

	mu      sync.Mutex
	clients map[*websocket.Conn]struct{}

	httpSrv *http.Server
}

// NewServer constructs the web server.
func NewServer(
	cfg *config.Config,
	fetcher *prayer.Fetcher,
	scheduler *prayer.Scheduler,
	castMgr *chromecast.Manager,
) (*Server, error) {
	tmpl, err := loadTemplates()
	if err != nil {
		return nil, fmt.Errorf("load templates: %w", err)
	}
	return &Server{
		cfg:       cfg,
		fetcher:   fetcher,
		scheduler: scheduler,
		castMgr:   castMgr,
		tmpl:      tmpl,
		clients:   make(map[*websocket.Conn]struct{}),
	}, nil
}

// Start begins serving on cfg.Web.Host:cfg.Web.Port.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Pages
	mux.HandleFunc("/", s.handleDashboard)
	mux.HandleFunc("/scheduler", s.handleScheduler)
	mux.HandleFunc("/chromecasts", s.handleChromecasts)
	mux.HandleFunc("/settings", s.handleSettings)
	mux.HandleFunc("/logs", s.handleLogs)

	// WebSocket
	mux.HandleFunc("/ws", s.handleWS)

	// REST API
	mux.HandleFunc("/api/config", s.handleAPIConfig)
	mux.HandleFunc("/api/prayer-times", s.handleAPIPrayerTimes)
	mux.HandleFunc("/api/scheduler-status", s.handleAPISchedulerStatus)
	mux.HandleFunc("/api/devices", s.handleAPIDevices)
	mux.HandleFunc("/api/discover-devices", s.handleAPIDiscoverDevices)
	mux.HandleFunc("/api/play-athan", s.handleAPIPlayAthan)
	mux.HandleFunc("/api/stop-playback", s.handleAPIStopPlayback)
	mux.HandleFunc("/api/next-prayer", s.handleAPINextPrayer)

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
	s.httpSrv = &http.Server{Handler: mux}

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
	s.renderPage(w, "dashboard.html", map[string]interface{}{
		"config":       s.cfg.AsWebDict(),
		"prayer_times": times,
		"next_prayer":  map[string]interface{}{"name": name, "time": at.Format("15:04")},
		"jobs":         s.scheduler.Status(),
	})
}

func (s *Server) handleScheduler(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, "scheduler.html", map[string]interface{}{
		"config": s.cfg.AsWebDict(),
		"jobs":   s.scheduler.Status(),
	})
}

func (s *Server) handleChromecasts(w http.ResponseWriter, r *http.Request) {
	devs := s.castMgr.Devices()
	s.renderPage(w, "chromecasts.html", map[string]interface{}{
		"config":  s.cfg.AsWebDict(),
		"devices": devs,
	})
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, "settings.html", map[string]interface{}{
		"config": s.cfg.AsWebDict(),
	})
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	tail := readLogTail(s.cfg.Log.FilePath, 200)
	s.renderPage(w, "logs.html", map[string]interface{}{
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

	log.Printf("[web/ws] client connected from %s", r.RemoteAddr)
	defer func() {
		s.mu.Lock()
		delete(s.clients, conn)
		s.mu.Unlock()
		conn.Close()
		log.Printf("[web/ws] client disconnected from %s", r.RemoteAddr)
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
		if v, ok := body["speakers_group_name"].(string); ok {
			s.cfg.Speaker.GroupName = v
		}
		if v, ok := body["location"].(string); ok {
			s.cfg.Prayer.Location = v
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
		if err := s.cfg.Save(); err != nil {
			writeJSON(w, errResp(err))
			return
		}
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
		Prayer string `json:"prayer"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Prayer == "" {
		body.Prayer = "Dhuhr"
	}
	if err := s.castMgr.PlayAthan(body.Prayer); err != nil {
		writeJSON(w, errResp(err))
		return
	}
	writeJSON(w, map[string]interface{}{"success": true, "prayer": body.Prayer})
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

func (s *Server) handleAPINextPrayer(w http.ResponseWriter, r *http.Request) {
	name, at, ok := s.scheduler.NextPrayer()
	writeJSON(w, map[string]interface{}{
		"success": true,
		"found":   ok,
		"prayer":  name,
		"time":    at.Format("15:04"),
	})
}

// --- helpers ---

func (s *Server) renderPage(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("[web] template error (%s): %v", name, err)
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

func loadTemplates() (*template.Template, error) {
	sub, err := fs.Sub(templateFS, "templates")
	if err != nil {
		return nil, err
	}
	return template.New("").ParseFS(sub, "*.html")
}
