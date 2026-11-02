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

	"azan-agent/internal/trace"
)

//go:embed embedded
var embeddedMedia embed.FS

// EmbeddedFS exposes the embedded media filesystem for use by other packages.
var EmbeddedFS = &embeddedMedia

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

	// Report every fetch to the Journal: who asked, how much was sent, whether
	// the whole file went out. (A speaker that fetched the file but never played
	// it looks very different from one that never fetched it.)
	cw := &countingWriter{ResponseWriter: w, status: http.StatusOK}
	w = cw
	source, total := "", int64(0)
	start := time.Now()
	defer func() { traceFetch(r, name, source, total, cw, time.Since(start)) }()

	// Try local directory first
	localPath := filepath.Join(s.dir, name)
	if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
		source, total = "local", info.Size()
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
	source = "embedded"
	if st, err := f.Stat(); err == nil {
		total = st.Size()
	}

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

// countingWriter remembers the status and how many bytes were written.
type countingWriter struct {
	http.ResponseWriter
	status int
	n      int64
}

func (c *countingWriter) WriteHeader(code int) { c.status = code; c.ResponseWriter.WriteHeader(code) }
func (c *countingWriter) Write(b []byte) (int, error) {
	n, err := c.ResponseWriter.Write(b)
	c.n += int64(n)
	return n, err
}

func traceFetch(r *http.Request, name, source string, total int64, cw *countingWriter, took time.Duration) {
	remote, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remote = r.RemoteAddr
	}
	fields := map[string]any{"name": name, "remote": remote, "status": cw.status, "bytes": cw.n, "total": total,
		"source": source, "ms": took.Milliseconds(), "range": r.Header.Get("Range")}
	if source == "" || cw.status >= 400 {
		trace.Warn(trace.SourceMedia, "fetch-failed", "", fmt.Sprintf("%s asked for %s but it could not be served (HTTP %d)", remote, name, cw.status), mapToKV(fields)...)
		return
	}
	complete := cw.status == http.StatusOK && cw.n == total
	fields["complete"] = complete
	what := "all of it"
	if !complete {
		what = fmt.Sprintf("%d of %d bytes", cw.n, total)
		if cw.status == http.StatusPartialContent {
			what += " (a range)"
		}
	}
	trace.From(trace.SourceMedia, "fetch", "", fmt.Sprintf("%s downloaded %s — %s in %dms", remote, name, what, took.Milliseconds()), mapToKV(fields)...)
}

func mapToKV(m map[string]any) []any {
	kv := make([]any, 0, 2*len(m))
	for k, v := range m {
		kv = append(kv, k, v)
	}
	return kv
}
