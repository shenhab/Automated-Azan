package media

import (
	"embed"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed embedded
var embeddedMedia embed.FS

// Server serves MP3 files from a directory over HTTP so Chromecast can fetch them.
// When a requested file is not found in the local directory, it falls back to the
// MP3 files embedded in the binary at build time.
type Server struct {
	dir     string
	port    int
	localIP string
	httpSrv *http.Server
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
		log.Printf("[media] serving %s on %s (embedded fallback enabled)", s.dir, s.httpSrv.Addr)
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
func (s *Server) BaseURL() string {
	return fmt.Sprintf("http://%s:%d/media/", s.localIP, s.port)
}

// LocalIP returns the detected LAN IP.
func (s *Server) LocalIP() string {
	return s.localIP
}

func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/media/")
	name = filepath.Base(name) // prevent path traversal

	// Try local directory first
	localPath := filepath.Join(s.dir, name)
	if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
		log.Printf("[media] serving local file: %s", name)
		w.Header().Set("Accept-Ranges", "bytes")
		http.ServeFile(w, r, localPath)
		return
	}

	// Fall back to embedded binary MP3
	embeddedPath := "embedded/" + name
	f, err := embeddedMedia.Open(embeddedPath)
	if err != nil {
		log.Printf("[media] not found locally or embedded: %s", name)
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	log.Printf("[media] serving embedded file: %s", name)
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Accept-Ranges", "bytes")
	io.Copy(w, f)
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
