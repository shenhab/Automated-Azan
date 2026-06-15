package web

import (
	"crypto/rand"
	"encoding/base64"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookie = "azan_session"
	sessionTTL    = 24 * time.Hour
)

type sessionEntry struct {
	expiry time.Time
}

var (
	sessionsMu sync.Mutex
	sessions   = map[string]sessionEntry{}
)

// HashPassword returns a bcrypt hash of the plain-text password.
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func pruneExpired() {
	now := time.Now()
	for tok, s := range sessions {
		if now.After(s.expiry) {
			delete(sessions, tok)
		}
	}
}

func (s *Server) authEnabled() bool {
	return s.cfg.Web.Auth.Username != ""
}

func (s *Server) isAuthenticated(r *http.Request) bool {
	if !s.authEnabled() {
		return true
	}
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	sess, ok := sessions[cookie.Value]
	if !ok || time.Now().After(sess.expiry) {
		delete(sessions, cookie.Value)
		return false
	}
	return true
}

// requireAuth wraps a page handler.
// - No credentials configured → redirect to /setup (first-run wizard)
// - Credentials set but not authenticated → redirect to /login
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authEnabled() {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		if s.isAuthenticated(r) {
			next(w, r)
			return
		}
		http.Redirect(w, r, "/login?next="+r.URL.RequestURI(), http.StatusSeeOther)
	}
}

// requireAPIAuth wraps an API handler: unauthenticated requests get 401 JSON.
func (s *Server) requireAPIAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authEnabled() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"success":false,"error":"setup required: visit the dashboard to create credentials"}`))
			return
		}
		if s.isAuthenticated(r) {
			next(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"success":false,"error":"unauthorized"}`))
	}
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	// Once credentials exist, setup is closed.
	if s.authEnabled() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if r.Method == http.MethodGet {
		s.renderPage(w, "setup.html", map[string]interface{}{})
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	confirm := r.FormValue("confirm")

	renderErr := func(msg string) {
		s.renderPage(w, "setup.html", map[string]interface{}{"error": msg})
	}

	if username == "" {
		renderErr("Username is required.")
		return
	}
	if len(password) < 8 {
		renderErr("Password must be at least 8 characters.")
		return
	}
	if password != confirm {
		renderErr("Passwords do not match.")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		renderErr("Failed to hash password: " + err.Error())
		return
	}

	s.cfg.Web.Auth.Username = username
	s.cfg.Web.Auth.PasswordHash = string(hash)
	if err := s.cfg.Save(); err != nil {
		renderErr("Failed to save credentials: " + err.Error())
		return
	}

	log.Printf("[auth] first-run credentials created for user: %s", username)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.isAuthenticated(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if r.Method == http.MethodGet {
		s.renderPage(w, "login.html", map[string]interface{}{
			"next": r.URL.Query().Get("next"),
		})
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	next := r.FormValue("next")

	authFail := func() {
		// Small delay to slow brute-force attempts.
		time.Sleep(500 * time.Millisecond)
		s.renderPage(w, "login.html", map[string]interface{}{
			"error": "Invalid username or password",
			"next":  next,
		})
	}

	if username != s.cfg.Web.Auth.Username {
		authFail()
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(s.cfg.Web.Auth.PasswordHash), []byte(password)); err != nil {
		authFail()
		return
	}

	tok, err := newSessionToken()
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	sessionsMu.Lock()
	pruneExpired()
	sessions[tok] = sessionEntry{expiry: time.Now().Add(sessionTTL)}
	sessionsMu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})

	if next == "" || next == "/login" {
		next = "/"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		sessionsMu.Lock()
		delete(sessions, cookie.Value)
		sessionsMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:   sessionCookie,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
