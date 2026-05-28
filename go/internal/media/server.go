package media

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Server serves MP3 files from a directory over HTTP so Chromecast can fetch them.
type Server struct {
	dir      string
	port     int
	localIP  string
	httpSrv  *http.Server
}

// NewServer creates a media server that serves files from dir on the given port.
func NewServer(dir string, port int) *Server {
	return &Server{dir: dir, port: port, localIP: detectLANIP()}
}

// Start begins serving in a background goroutine.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/media/", s.handleMedia)

	s.httpSrv = &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", s.port),
		Handler: mux,
	}

	ln, err := net.Listen("tcp", s.httpSrv.Addr)
	if err != nil {
		return fmt.Errorf("media server listen on %s: %w", s.httpSrv.Addr, err)
	}

	go func() {
		log.Printf("[media] serving %s on %s", s.dir, s.httpSrv.Addr)
		if err := s.httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("[media] server error: %v", err)
		}
	}()
	return nil
}

// Stop shuts down the HTTP server gracefully.
func (s *Server) Stop() {
	if s.httpSrv != nil {
		s.httpSrv.Close()
	}
}

// BaseURL returns the LAN-accessible URL prefix for media files.
// e.g. http://192.168.1.50:8080/media/
func (s *Server) BaseURL() string {
	return fmt.Sprintf("http://%s:%d/media/", s.localIP, s.port)
}

// LocalIP returns the detected LAN IP.
func (s *Server) LocalIP() string {
	return s.localIP
}

func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request) {
	// Strip /media/ prefix, prevent path traversal
	name := strings.TrimPrefix(r.URL.Path, "/media/")
	name = filepath.Base(name) // strip any directory components

	path := filepath.Join(s.dir, name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeFile(w, r, path)
}

// detectLANIP finds the outbound LAN IP by probing Google DNS.
func detectLANIP() string {
	conn, err := net.DialTimeout("udp", "8.8.8.8:80", 3*time.Second)
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}
