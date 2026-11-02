package journal

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// scriptedHealth answers each host's status page from a queue of snapshots.
type scriptedHealth struct {
	mu    sync.Mutex
	queue map[string][]HealthSnapshot
	calls map[string]int
}

func (s *scriptedHealth) fetch(host string) (HealthSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls[host]++
	q := s.queue[host]
	if len(q) == 0 {
		return HealthSnapshot{Host: host}, fmt.Errorf("unreachable")
	}
	h := q[0]
	if len(q) > 1 {
		s.queue[host] = q[1:]
	}
	h.Host, h.Reachable = host, true
	return h, nil
}

func healthy(uptime float64, build string) HealthSnapshot {
	return HealthSnapshot{Name: "Nest display", Uptime: uptime, Build: build, Connected: true, SSID: "Home"}
}

func healthJournal(t *testing.T, s *scriptedHealth) *Journal {
	j, _ := newJournal(t, func(o *Options) {
		o.Health = s.fetch
		o.Targets = func(Spec) []Target {
			return []Target{{Name: "athan", Host: "10.0.0.5", Port: 32067, Role: "speaker"}, {Name: "Nest display", Host: "10.0.0.5", Port: 8009, Role: "member"}}
		}
		o.PollEvery = 50 * time.Millisecond
	})
	return j
}

func runAthan(t *testing.T, j *Journal) Summary {
	t.Helper()
	r := j.Begin(athanSpec)
	playedAthan(r)
	time.Sleep(150 * time.Millisecond) // let the start snapshot arrive
	r.End("x")
	return j.Sessions(time.Time{})[0]
}

func TestHealthIsReadOncePerHostAtTheStartAndAtTheEnd(t *testing.T) {
	s := &scriptedHealth{queue: map[string][]HealthSnapshot{"10.0.0.5": {healthy(1000, "A")}}, calls: map[string]int{}}
	j := healthJournal(t, s)
	got := runAthan(t, j)
	if s.calls["10.0.0.5"] != 2 {
		t.Errorf("status page read %d times, want 2 (the group and its display share a host)", s.calls["10.0.0.5"])
	}
	if got.Verdict != VerdictOK {
		t.Errorf("verdict = %s (%s)", got.Verdict, got.Headline)
	}
	d, _ := j.Detail(got.ID)
	if len(d.Health) != 2 {
		t.Errorf("stored %d snapshots, want start+end", len(d.Health))
	}
	phases := ""
	for _, e := range d.Events {
		if e.Source == "health" {
			phases += e.Fields["phase"].(string) + " "
		}
	}
	if phases != "start end " {
		t.Errorf("health events on the timeline = %q, want start and end", phases)
	}
}

func TestADeviceThatRestartedSinceTheLastPrayerIsReported(t *testing.T) {
	s := &scriptedHealth{queue: map[string][]HealthSnapshot{"10.0.0.5": {healthy(50000, "A"), healthy(50003, "A"), healthy(120, "A"), healthy(125, "A")}}, calls: map[string]int{}}
	j := healthJournal(t, s)
	runAthan(t, j) // first prayer: nothing to compare with
	got := runAthan(t, j)
	if got.Verdict != VerdictWarning || !strings.Contains(got.Headline, "restarted since the last") {
		t.Errorf("%s: %q", got.Verdict, got.Headline)
	}
}

func TestADeviceThatRestartedDuringTheRecordingIsReported(t *testing.T) {
	s := &scriptedHealth{queue: map[string][]HealthSnapshot{"10.0.0.5": {healthy(5000, "A"), healthy(40, "A")}}, calls: map[string]int{}}
	got := runAthan(t, healthJournal(t, s))
	if got.Verdict != VerdictWarning || !strings.Contains(got.Headline, "restarted during") {
		t.Errorf("%s: %q", got.Verdict, got.Headline)
	}
}

func TestAFirmwareChangeIsReported(t *testing.T) {
	s := &scriptedHealth{queue: map[string][]HealthSnapshot{"10.0.0.5": {healthy(5000, "1.68"), healthy(5003, "1.68"), healthy(9000, "1.69"), healthy(9003, "1.69")}}, calls: map[string]int{}}
	j := healthJournal(t, s)
	runAthan(t, j)
	got := runAthan(t, j)
	if !strings.Contains(got.Headline, "firmware changed") || !strings.Contains(got.Headline, "1.68") {
		t.Errorf("%q", got.Headline)
	}
}

func TestADeviceWithNoStatusPageIsNotAProblem(t *testing.T) {
	s := &scriptedHealth{queue: map[string][]HealthSnapshot{}, calls: map[string]int{}} // never answers
	got := runAthan(t, healthJournal(t, s))
	if got.Verdict != VerdictOK {
		t.Errorf("a device that never had a status page made the Athan a %s: %q", got.Verdict, got.Headline)
	}
}

func TestADeviceThatStoppedAnsweringIsReported(t *testing.T) {
	s := &scriptedHealth{queue: map[string][]HealthSnapshot{"10.0.0.5": {healthy(5000, "A"), healthy(5003, "A")}}, calls: map[string]int{}}
	j := healthJournal(t, s)
	runAthan(t, j)
	s.mu.Lock()
	s.queue["10.0.0.5"] = nil // now unreachable
	s.mu.Unlock()
	got := runAthan(t, j)
	if !strings.Contains(got.Headline, "stopped answering") {
		t.Errorf("%q", got.Headline)
	}
}

func TestTheRealStatusPageIsParsedWithoutKeepingSecrets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/setup/eureka_info" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"name":"Nest display","uptime":894415.4,"cast_build_revision":"3.80.555360","connected":true,"has_update":false,
		  "ssid":"Home","ethernet_connected":false,"mac_address":"D8:8C:79:6E:AC:6B","public_key":"MIIB...","hotspot_bssid":"FA:8F:CA:5E:51:88","ip_address":"192.168.86.187"}`)
	}))
	defer srv.Close()
	h, err := fetchHealth(srv.URL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if h.Name != "Nest display" || h.Uptime < 894000 || h.Build != "3.80.555360" || !h.Connected || h.SSID != "Home" {
		t.Errorf("parsed = %+v", h)
	}
	if b := fmt.Sprintf("%+v", h); strings.Contains(b, "D8:8C") || strings.Contains(b, "MIIB") || strings.Contains(b, "FA:8F") {
		t.Errorf("secrets kept: %s", b)
	}
}
