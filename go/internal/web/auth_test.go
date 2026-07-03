package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"azan-agent/internal/config"
)

func newTestServer(authUsername string) *Server {
	return &Server{
		cfg: &config.Config{
			Web: config.WebConfig{
				Auth: config.AuthConfig{
					Username: authUsername,
				},
			},
		},
	}
}

func addSession(tok string, expiry time.Time) {
	sessionsMu.Lock()
	sessions[tok] = sessionEntry{expiry: expiry}
	sessionsMu.Unlock()
}

func clearSessions() {
	sessionsMu.Lock()
	for k := range sessions {
		delete(sessions, k)
	}
	sessionsMu.Unlock()
}

func TestIsAuthenticated_AuthDisabled(t *testing.T) {
	clearSessions()
	s := newTestServer("")
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if !s.isAuthenticated(r) {
		t.Fatal("expected isAuthenticated to return true when auth is disabled")
	}
}

func TestIsAuthenticated_NoCookie(t *testing.T) {
	clearSessions()
	s := newTestServer("admin")
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if s.isAuthenticated(r) {
		t.Fatal("expected isAuthenticated to return false with no cookie")
	}
}

func TestIsAuthenticated_UnknownToken(t *testing.T) {
	clearSessions()
	s := newTestServer("admin")
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "does-not-exist"})
	if s.isAuthenticated(r) {
		t.Fatal("expected isAuthenticated to return false for unknown token")
	}
}

func TestIsAuthenticated_ExpiredSession(t *testing.T) {
	clearSessions()
	defer clearSessions()
	s := newTestServer("admin")
	tok := "expired-token"
	addSession(tok, time.Now().Add(-time.Hour))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})
	if s.isAuthenticated(r) {
		t.Fatal("expected isAuthenticated to return false for expired session")
	}
}

func TestIsAuthenticated_ValidSession(t *testing.T) {
	clearSessions()
	defer clearSessions()
	s := newTestServer("admin")
	tok := "valid-token"
	addSession(tok, time.Now().Add(time.Hour))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})
	if !s.isAuthenticated(r) {
		t.Fatal("expected isAuthenticated to return true for valid unexpired session")
	}
}

func TestPruneExpired(t *testing.T) {
	clearSessions()
	defer clearSessions()

	sessionsMu.Lock()
	sessions["expired-1"] = sessionEntry{expiry: time.Now().Add(-time.Minute)}
	sessions["expired-2"] = sessionEntry{expiry: time.Now().Add(-time.Hour)}
	sessions["valid-1"] = sessionEntry{expiry: time.Now().Add(time.Hour)}
	pruneExpired()
	defer sessionsMu.Unlock()

	if _, ok := sessions["expired-1"]; ok {
		t.Error("expected expired-1 to be pruned")
	}
	if _, ok := sessions["expired-2"]; ok {
		t.Error("expected expired-2 to be pruned")
	}
	if _, ok := sessions["valid-1"]; !ok {
		t.Error("expected valid-1 to remain after pruning")
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 remaining session, got %d", len(sessions))
	}
}

func TestRequireAuth_AuthDisabled_RedirectsToSetup(t *testing.T) {
	clearSessions()
	s := newTestServer("")
	called := false
	handler := s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler(w, r)

	if called {
		t.Fatal("expected wrapped handler not to be called when auth is disabled")
	}
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/setup" {
		t.Fatalf("expected redirect to /setup, got %q", loc)
	}
}

func TestRequireAuth_Unauthenticated_RedirectsToLogin(t *testing.T) {
	clearSessions()
	s := newTestServer("admin")
	called := false
	handler := s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	r := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	w := httptest.NewRecorder()
	handler(w, r)

	if called {
		t.Fatal("expected wrapped handler not to be called when unauthenticated")
	}
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, w.Code)
	}
	loc := w.Header().Get("Location")
	if loc == "" || loc[:len("/login")] != "/login" {
		t.Fatalf("expected redirect to /login*, got %q", loc)
	}
}

func TestRequireAuth_Authenticated_CallsNext(t *testing.T) {
	clearSessions()
	defer clearSessions()
	s := newTestServer("admin")
	tok := "valid-token"
	addSession(tok, time.Now().Add(time.Hour))

	called := false
	handler := s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})
	w := httptest.NewRecorder()
	handler(w, r)

	if !called {
		t.Fatal("expected wrapped handler to be called when authenticated")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestRequireAPIAuth_AuthDisabled_Returns503(t *testing.T) {
	clearSessions()
	s := newTestServer("")
	called := false
	handler := s.requireAPIAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	r := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	w := httptest.NewRecorder()
	handler(w, r)

	if called {
		t.Fatal("expected wrapped handler not to be called when auth is disabled")
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse JSON body: %v", err)
	}
	if success, _ := body["success"].(bool); success {
		t.Error("expected success=false in JSON body")
	}
}

func TestRequireAPIAuth_Unauthenticated_Returns401(t *testing.T) {
	clearSessions()
	s := newTestServer("admin")
	called := false
	handler := s.requireAPIAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	r := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	w := httptest.NewRecorder()
	handler(w, r)

	if called {
		t.Fatal("expected wrapped handler not to be called when unauthenticated")
	}
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse JSON body: %v", err)
	}
	if success, _ := body["success"].(bool); success {
		t.Error("expected success=false in JSON body")
	}
}

func TestRequireAPIAuth_Authenticated_CallsNext(t *testing.T) {
	clearSessions()
	defer clearSessions()
	s := newTestServer("admin")
	tok := "valid-token"
	addSession(tok, time.Now().Add(time.Hour))

	called := false
	handler := s.requireAPIAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})
	w := httptest.NewRecorder()
	handler(w, r)

	if !called {
		t.Fatal("expected wrapped handler to be called when authenticated")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}
