package media

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"testing"
)

func portOf(t *testing.T, s *Server) int {
	t.Helper()
	u, err := url.Parse(s.BaseURL())
	if err != nil {
		t.Fatal(err)
	}
	p, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("BaseURL %q has no port: %v", s.BaseURL(), err)
	}
	return p
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestTheMediaServerUsesTheRequestedPortWhenItIsFree(t *testing.T) {
	want := freePort(t)
	s := NewServer(t.TempDir(), want)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	if got := portOf(t, s); got != want {
		t.Errorf("port = %d, want %d", got, want)
	}
}

func TestATakenMediaPortFallsBackToARandomOneThatWorks(t *testing.T) {
	taken, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	busy := taken.Addr().(*net.TCPAddr).Port

	s := NewServer(t.TempDir(), busy)
	if err := s.Start(); err != nil {
		t.Fatalf("a taken port must not stop the agent: %v", err)
	}
	defer s.Stop()
	got := portOf(t, s)
	if got == busy || got == 0 {
		t.Fatalf("port = %d, the taken port was %d", got, busy)
	}
	// BaseURL must point at where it really listens: speakers fetch from there.
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/media/does-not-exist.mp3", got))
	if err != nil {
		t.Fatalf("nothing is listening on the advertised port: %v", err)
	}
	resp.Body.Close()
}
